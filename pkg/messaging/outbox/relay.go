// Package outbox publica na fila SQS os eventos que um serviço gravou no seu
// outbox transacional, na mesma transação do dado: existe evento se, e somente
// se, o dado foi gravado, e o relay garante a entrega mesmo que a fila esteja
// fora na hora da gravação.
//
// Cada passada é uma leitura de até 10 eventos, um envio em lote e uma escrita
// por desfecho (publicados, recusados, a tentar de novo) — sem banco nem fila
// dentro de laço.
package outbox

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/felipedenardo/chameleon-common/pkg/metrics"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	// batchSize é o máximo do SQS por envio em lote: uma passada, um envio.
	batchSize = 10
	// maxMessageBytes é o maior corpo que a fila aceita; maxBatchBytes, a
	// soma dos corpos de um envio.
	maxMessageBytes = 256 * 1024
	maxBatchBytes   = 256 * 1024

	defaultPollInterval = 5 * time.Second
	minBackoff          = 2 * time.Second
	maxBackoff          = 30 * time.Second
	sendTimeout         = 5 * time.Second

	metricsJob = "outbox_relay"
)

// Eventos esperando a fila. É o sinal mais honesto do padrão: se o relay
// parar, nada dá erro — a tabela só cresce em silêncio.
var pendingGauge = metrics.NewGauge(
	"chameleon_outbox_pending_events",
	"Eventos no outbox aguardando publicacao, por destino.",
	"stream",
)

// Eventos que a fila recusou de vez. Saíram do caminho dos outros, mas quem
// consome nunca soube deles: acima de zero, alguém precisa olhar.
var failedGauge = metrics.NewGauge(
	"chameleon_outbox_failed_events",
	"Eventos no outbox que a fila recusou de vez, por destino.",
	"stream",
)

// Event é um evento do outbox como o relay precisa dele.
type Event struct {
	ID uuid.UUID
	// Type aparece no log do evento recusado.
	Type string
	// Body é publicado como veio.
	Body string
}

// Store é o outbox de um serviço, sempre em lote: cada método é uma consulta
// ou uma escrita, qualquer que seja o tamanho do lote.
type Store interface {
	// FetchPending traz até limit eventos que esperam a fila, do mais antigo.
	FetchPending(ctx context.Context, limit int) ([]Event, error)
	// MarkPublished marca os eventos que a fila aceitou.
	MarkPublished(ctx context.Context, ids []uuid.UUID) error
	// MarkFailed tira da fila de publicação os que ela recusa de vez (corpo
	// grande demais ou inválido): tentar de novo daria o mesmo, e esperar
	// por eles travaria os seguintes.
	MarkFailed(ctx context.Context, ids []uuid.UUID) error
	// IncrementAttempts conta mais uma tentativa dos que vão de novo.
	IncrementAttempts(ctx context.Context, ids []uuid.UUID) error
	CountPending(ctx context.Context) (int64, error)
	CountFailed(ctx context.Context) (int64, error)
}

// Client é a parte do SQS que o relay usa.
type Client interface {
	SendMessageBatch(ctx context.Context, params *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
}

// Options configura um relay.
type Options struct {
	QueueURL string
	// Enabled desliga o relay quando a fila não está configurada (dev sem
	// LocalStack): os eventos esperam no banco.
	Enabled bool
	// Stream rotula as métricas quando o serviço publica em mais de uma fila
	// (o payment: clube e plataforma). Vazio quando há uma só.
	Stream string
	// PollInterval é a espera entre passadas quando o outbox esvaziou.
	PollInterval time.Duration
}

// Relay publica o outbox de uma fila.
type Relay struct {
	store  Store
	client Client
	opts   Options
	logger zerolog.Logger
}

func New(store Store, client Client, opts Options, logger zerolog.Logger) *Relay {
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultPollInterval
	}
	return &Relay{store: store, client: client, opts: opts, logger: logger.With().Str("stream", opts.Stream).Logger()}
}

// Start publica até o contexto acabar.
func (r *Relay) Start(ctx context.Context) error {
	if !r.opts.Enabled || r.client == nil {
		r.logger.Info().Msg("Outbox relay disabled (SQS not configured)")
		return nil
	}

	r.logger.Info().Msg("Starting outbox relay")
	backoff := minBackoff
	for {
		if ctx.Err() != nil {
			return nil
		}
		var result pass
		err := metrics.Observe(metricsJob, func() error {
			var drainErr error
			result, drainErr = r.drain(ctx)
			return drainErr
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.logger.Error().Err(err).Msg("Outbox relay pass failed")
			if !sleep(ctx, backoff) {
				return nil
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = minBackoff
		r.observe(ctx)

		// Sobrou evento: segue sem esperar. Esvaziou: espera a próxima passada.
		if result.more {
			continue
		}
		if !sleep(ctx, r.opts.PollInterval) {
			return nil
		}
	}
}

// pass é o desfecho de uma passada.
type pass struct {
	handled int
	// more diz que ainda há evento esperando.
	more bool
}

// drain publica um lote. Falha do envio inteiro (fila fora, credencial) volta
// como erro, e os eventos ganham mais uma tentativa; a recusa de uma entrada
// é por evento: a culpa do corpo (SenderFault) o marca FAILED, a da fila o
// deixa para a próxima passada.
func (r *Relay) drain(ctx context.Context) (pass, error) {
	events, err := r.store.FetchPending(ctx, batchSize)
	if err != nil || len(events) == 0 {
		return pass{}, err
	}

	batch, oversized, left := split(events)
	refused := make([]uuid.UUID, 0, len(oversized))
	for _, event := range oversized {
		r.logRefused(event, "MessageTooLong", "body larger than the queue takes")
		refused = append(refused, event.ID)
	}

	var published, retry []uuid.UUID
	var sendErr error
	if len(batch) > 0 {
		var outcome sendOutcome
		outcome, sendErr = r.send(ctx, batch)
		published, retry = outcome.published, outcome.retry
		refused = append(refused, outcome.refused...)
	}

	if err := r.mark(ctx, published, refused, retry); err != nil && sendErr == nil {
		return pass{}, err
	}
	return pass{handled: len(published) + len(refused), more: len(events) == batchSize || left > 0}, sendErr
}

// split separa, na ordem, o que cabe num envio (até 10 eventos e 256 KiB), o
// que não cabe na fila de jeito nenhum e quantos ficam para a próxima passada.
func split(events []Event) (batch, oversized []Event, left int) {
	size := 0
	for _, event := range events {
		switch {
		case len(event.Body) > maxMessageBytes:
			oversized = append(oversized, event)
		case len(batch) < batchSize && size+len(event.Body) <= maxBatchBytes:
			batch = append(batch, event)
			size += len(event.Body)
		default:
			left++
		}
	}
	return batch, oversized, left
}

type sendOutcome struct {
	published, refused, retry []uuid.UUID
}

func (r *Relay) send(ctx context.Context, batch []Event) (sendOutcome, error) {
	byID := make(map[string]Event, len(batch))
	entries := make([]types.SendMessageBatchRequestEntry, 0, len(batch))
	ids := make([]uuid.UUID, 0, len(batch))
	for _, event := range batch {
		byID[event.ID.String()] = event
		ids = append(ids, event.ID)
		entries = append(entries, types.SendMessageBatchRequestEntry{Id: aws.String(event.ID.String()), MessageBody: aws.String(event.Body)})
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	output, err := r.client.SendMessageBatch(sendCtx, &sqs.SendMessageBatchInput{QueueUrl: aws.String(r.opts.QueueURL), Entries: entries})
	cancel()
	if err != nil {
		return sendOutcome{retry: ids}, err
	}

	var outcome sendOutcome
	for _, ok := range output.Successful {
		if event, found := byID[aws.ToString(ok.Id)]; found {
			outcome.published = append(outcome.published, event.ID)
		}
	}
	for _, failed := range output.Failed {
		event, found := byID[aws.ToString(failed.Id)]
		if !found {
			continue
		}
		if failed.SenderFault {
			r.logRefused(event, aws.ToString(failed.Code), aws.ToString(failed.Message))
			outcome.refused = append(outcome.refused, event.ID)
			continue
		}
		r.logger.Warn().Str("event_id", event.ID.String()).Str("code", aws.ToString(failed.Code)).
			Msg("Outbox event not accepted by the queue this time")
		outcome.retry = append(outcome.retry, event.ID)
	}
	return outcome, nil
}

// mark grava os desfechos, uma escrita por desfecho.
func (r *Relay) mark(ctx context.Context, published, refused, retry []uuid.UUID) error {
	if len(published) > 0 {
		if err := r.store.MarkPublished(ctx, published); err != nil {
			return err
		}
	}
	if len(refused) > 0 {
		if err := r.store.MarkFailed(ctx, refused); err != nil {
			return err
		}
	}
	if len(retry) > 0 {
		return r.store.IncrementAttempts(ctx, retry)
	}
	return nil
}

func (r *Relay) logRefused(event Event, code, reason string) {
	metrics.Failure(metricsJob, "rejected")
	r.logger.Error().Str("event_id", event.ID.String()).Str("event_type", event.Type).
		Str("code", code).Str("reason", reason).
		Msg("Outbox event rejected by the queue; marked FAILED")
}

// observe atualiza as métricas de profundidade. Falha aqui não interrompe o
// relay: métrica indisponível é menos grave que parar de publicar.
func (r *Relay) observe(ctx context.Context) {
	pending, err := r.store.CountPending(ctx)
	if err != nil {
		metrics.Failure(metricsJob, "count_pending")
		r.logger.Warn().Err(err).Msg("Failed to count pending outbox events")
	} else {
		pendingGauge.WithLabelValues(r.opts.Stream).Set(float64(pending))
	}

	failed, err := r.store.CountFailed(ctx)
	if err != nil {
		metrics.Failure(metricsJob, "count_failed")
		r.logger.Warn().Err(err).Msg("Failed to count failed outbox events")
		return
	}
	failedGauge.WithLabelValues(r.opts.Stream).Set(float64(failed))
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
