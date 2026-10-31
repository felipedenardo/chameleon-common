// Package consumer é o laço de leitura de uma fila SQS: espera longa, espera
// crescente na falha passageira, recusa permanente seguindo para a DLQ e as
// confirmações feitas em lote. Cada serviço só diz o que fazer com as
// mensagens.
//
// Nada aqui chama o banco ou a fila dentro de um laço: o consumidor de uma
// mensagem por vez (NewSingle) recebe uma só, o de lote (New) entrega o lote
// inteiro ao handler, e as confirmações de cada recebida saem numa chamada
// para apagar e numa para adiar.
package consumer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/felipedenardo/chameleon-common/pkg/metrics"
	"github.com/rs/zerolog"
)

const (
	// waitTimeSeconds é a espera longa: sem ela o laço vira uma chamada por
	// milissegundo, cada uma cobrada.
	waitTimeSeconds = 20
	minPollBackoff  = 2 * time.Second
	maxPollBackoff  = 30 * time.Second
	// maxBatch é o máximo do SQS por recebimento e por confirmação em lote.
	maxBatch = 10
)

// Espera entre as entregas de uma mensagem que falhou por motivo passageiro
// (banco fora, outro serviço fora, evento que chegou antes da hora): dobra a
// cada entrega — 30 s, 1 min, 2 min, 4 min, 8 min — até 15 min. Com 5
// entregas a mensagem aguenta uns 15 minutos de dependência fora antes da DLQ;
// com 10, perto de 1h30. Com o visibility timeout fixo, seriam poucos minutos.
const (
	retryBaseDelay = 30 * time.Second
	retryMaxDelay  = 15 * time.Minute
)

// Client é a parte do SQS que o consumidor usa.
type Client interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageBatch(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
	ChangeMessageVisibilityBatch(ctx context.Context, params *sqs.ChangeMessageVisibilityBatchInput, optFns ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityBatchOutput, error)
}

// Handler trata o lote recebido e devolve, na mesma ordem, o desfecho de cada
// mensagem: nil apaga; Permanent(err) deixa a mensagem seguir o tempo normal
// até a DLQ; qualquer outro erro adia a próxima entrega com espera crescente.
type Handler func(ctx context.Context, messages []types.Message) []error

// Options configura um consumidor.
type Options struct {
	// Name nomeia o consumidor nas métricas (rótulo job) e nos logs.
	Name     string
	QueueURL string
	// Enabled desliga o consumidor quando a fila não está configurada (dev
	// sem LocalStack): Start só registra e sai.
	Enabled bool
	// VisibilityTimeout é quanto a mensagem fica escondida enquanto é
	// tratada, em segundos. Precisa cobrir o tratamento do lote: tratar além
	// dele faz a fila entregar de novo, em duplicidade.
	VisibilityTimeout int32
	// MaxMessages é o tamanho do lote recebido em New, de 1 a 10.
	MaxMessages int32
}

// Consumer lê uma fila.
type Consumer struct {
	client Client
	opts   Options
	handle Handler
	logger zerolog.Logger
}

// New monta o consumidor de lote: recebe até MaxMessages de uma vez e entrega
// todas ao handler, que as trata juntas (uma leitura, uma transação).
func New(client Client, opts Options, handle Handler, logger zerolog.Logger) *Consumer {
	opts.MaxMessages = max(1, min(opts.MaxMessages, maxBatch))
	return &Consumer{client: client, opts: opts, handle: handle, logger: logger.With().Str("consumer", opts.Name).Logger()}
}

// NewSingle monta o consumidor de uma mensagem por vez, para quando cada
// mensagem é um fato separado (uma venda, uma compra) tratado na sua própria
// transação.
func NewSingle(client Client, opts Options, handle func(ctx context.Context, message types.Message) error, logger zerolog.Logger) *Consumer {
	opts.MaxMessages = 1
	return New(client, opts, func(ctx context.Context, messages []types.Message) []error {
		return []error{handle(ctx, messages[0])}
	}, logger)
}

// permanentError marca a recusa que se repete a cada entrega (mensagem mal
// formada, tipo desconhecido): esperar não muda nada, e ela segue o tempo
// normal da fila até a DLQ, onde alguém vê.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marca err como recusa permanente.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// IsPermanent diz se err (ou algum erro que ele embrulha) é permanente.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

// Start lê a fila até o contexto acabar.
func (c *Consumer) Start(ctx context.Context) error {
	if !c.opts.Enabled {
		c.logger.Info().Msg("SQS consumer disabled")
		return nil
	}
	if c.client == nil {
		return fmt.Errorf("%s: SQS client is not configured", c.opts.Name)
	}

	c.logger.Info().Str("queue_url", c.opts.QueueURL).Msg("Starting SQS consumer")
	backoff := minPollBackoff
	for {
		if ctx.Err() != nil {
			return nil
		}
		output, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:                    aws.String(c.opts.QueueURL),
			MaxNumberOfMessages:         c.opts.MaxMessages,
			WaitTimeSeconds:             waitTimeSeconds,
			VisibilityTimeout:           c.opts.VisibilityTimeout,
			MessageSystemAttributeNames: []types.MessageSystemAttributeName{types.MessageSystemAttributeNameApproximateReceiveCount},
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Contador próprio: aqui o handler nem roda, então o contador dele
			// ficaria parado — e parado é indistinguível de fila vazia.
			metrics.Failure(c.opts.Name, "receive")
			c.logger.Error().Err(err).Msg("Failed to receive SQS messages")
			if !sleep(ctx, backoff) {
				return nil
			}
			backoff = min(backoff*2, maxPollBackoff)
			continue
		}
		backoff = minPollBackoff
		if len(output.Messages) > 0 {
			c.process(ctx, output.Messages)
		}
	}
}

// process trata o lote e confirma cada mensagem: as tratadas são apagadas
// numa chamada, as que falharam de passagem são adiadas noutra.
func (c *Consumer) process(ctx context.Context, messages []types.Message) {
	var results []error
	// O erro conjunto só classifica a execução nas métricas; cada mensagem é
	// conferida pelo próprio desfecho logo abaixo.
	_ = metrics.Observe(c.opts.Name, func() error {
		results = c.handle(ctx, messages)
		return errors.Join(results...)
	})

	var done []types.DeleteMessageBatchRequestEntry
	var later []types.ChangeMessageVisibilityBatchRequestEntry
	for i, message := range messages {
		err := errMissingResult
		if i < len(results) {
			err = results[i]
		}
		if message.ReceiptHandle == nil {
			continue
		}
		id := aws.String(strconv.Itoa(i))
		switch {
		case err == nil:
			done = append(done, types.DeleteMessageBatchRequestEntry{Id: id, ReceiptHandle: message.ReceiptHandle})
		case IsPermanent(err):
			c.logger.Error().Err(err).Str("message_id", aws.ToString(message.MessageId)).
				Msg("SQS message refused; it goes to the DLQ when its receives run out")
		default:
			c.logger.Error().Err(err).Str("message_id", aws.ToString(message.MessageId)).
				Msg("SQS message processing failed; postponed")
			later = append(later, types.ChangeMessageVisibilityBatchRequestEntry{
				Id: id, ReceiptHandle: message.ReceiptHandle, VisibilityTimeout: int32(retryDelay(message).Seconds()),
			})
		}
	}
	c.delete(ctx, done)
	c.postpone(ctx, later)
}

// errMissingResult é a mensagem para a qual o handler não devolveu desfecho:
// tratada como falha passageira, para não ser apagada sem ter sido tratada.
var errMissingResult = errors.New("handler returned no result for this message")

// delete apaga as tratadas. Se falhar, elas voltam depois do visibility
// timeout e são tratadas de novo — os consumidores são idempotentes.
func (c *Consumer) delete(ctx context.Context, entries []types.DeleteMessageBatchRequestEntry) {
	if len(entries) == 0 {
		return
	}
	output, err := c.client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{QueueUrl: aws.String(c.opts.QueueURL), Entries: entries})
	if err != nil {
		if ctx.Err() == nil {
			metrics.Failure(c.opts.Name, "delete")
			c.logger.Warn().Err(err).Int("messages", len(entries)).Msg("Failed to delete processed SQS messages")
		}
		return
	}
	if len(output.Failed) > 0 {
		metrics.Failure(c.opts.Name, "delete")
		c.logger.Warn().Int("messages", len(output.Failed)).Str("code", aws.ToString(output.Failed[0].Code)).
			Msg("SQS refused to delete processed messages")
	}
}

// postpone esconde as que falharam de passagem pela espera crescente. Se
// falhar, elas voltam no visibility timeout normal.
func (c *Consumer) postpone(ctx context.Context, entries []types.ChangeMessageVisibilityBatchRequestEntry) {
	if len(entries) == 0 || ctx.Err() != nil {
		return
	}
	output, err := c.client.ChangeMessageVisibilityBatch(ctx, &sqs.ChangeMessageVisibilityBatchInput{QueueUrl: aws.String(c.opts.QueueURL), Entries: entries})
	if err != nil {
		metrics.Failure(c.opts.Name, "postpone")
		c.logger.Warn().Err(err).Int("messages", len(entries)).Msg("Failed to postpone SQS messages")
		return
	}
	if len(output.Failed) > 0 {
		metrics.Failure(c.opts.Name, "postpone")
		c.logger.Warn().Int("messages", len(output.Failed)).Str("code", aws.ToString(output.Failed[0].Code)).
			Msg("SQS refused to postpone messages")
	}
}

// retryDelay é a espera antes da próxima entrega, pela contagem de entregas.
func retryDelay(message types.Message) time.Duration {
	count, err := strconv.Atoi(message.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)])
	if err != nil || count < 1 {
		count = 1
	}
	delay := retryBaseDelay
	for i := 1; i < count && delay < retryMaxDelay; i++ {
		delay *= 2
	}
	return min(delay, retryMaxDelay)
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
