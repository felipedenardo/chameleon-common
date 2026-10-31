// Package queuedepth mede, de tempos em tempos, quantas mensagens estão numa
// fila SQS e na DLQ dela. É o que os alertas de fila parada e de mensagem na
// DLQ leem.
package queuedepth

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/felipedenardo/chameleon-common/pkg/metrics"
	"github.com/rs/zerolog"
)

const (
	interval   = 30 * time.Second
	metricsJob = "queue_depth_poller"
)

// Mensagens na fila SQS, por estado. Rotulado por fila (queue) para cobrir a
// principal e a DLQ na mesma métrica; state separa o que aguarda consumo do que
// já foi recebido e está em processamento.
var sqsMessages = metrics.NewGauge(
	"chameleon_sqs_messages",
	"Mensagens na fila SQS, por estado.",
	"queue", "state",
)

// Client é a parte do SQS que a medição usa.
type Client interface {
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

// Poller lê a profundidade da fila e alimenta o gauge.
//
// Roda de preferência no produtor: se o consumidor cair e a fila empilhar — o
// cenário que se quer enxergar —, um poller no consumidor sumiria junto. Na
// fila com muitos produtores (notifications), roda no consumidor, e o worker
// caído já é o alerta WorkerFora.
//
// A DLQ é descoberta do próprio RedrivePolicy da fila principal, sem config
// extra. Qualquer valor acima de zero nela é sinal de que algo quebrou, não de
// backlog normal.
type Poller struct {
	client   Client
	queueURL string
	queue    string
	logger   zerolog.Logger

	// Descobertos na primeira leitura, a partir do RedrivePolicy.
	dlqURL   string
	dlqQueue string
}

func New(client Client, queueURL string, logger zerolog.Logger) *Poller {
	return &Poller{client: client, queueURL: queueURL, queue: queueName(queueURL), logger: logger}
}

// Start mede até o contexto acabar.
func (p *Poller) Start(ctx context.Context) {
	if p.client == nil || p.queueURL == "" {
		p.logger.Info().Msg("Queue depth poller disabled (SQS not configured)")
		return
	}

	p.logger.Info().Str("queue", p.queue).Msg("Starting queue depth poller")
	p.poll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	out, err := p.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(p.queueURL),
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameApproximateNumberOfMessages,
			types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			// Só para descobrir a DLQ; lido uma vez.
			types.QueueAttributeNameRedrivePolicy,
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		metrics.Failure(metricsJob, "get_attributes")
		p.logger.Warn().Err(err).Msg("Failed to read SQS queue attributes")
		return
	}

	// visible: aguardando consumo. in_flight: recebida por um consumidor, ainda
	// não apagada (em processamento ou adiada).
	set(p.queue, "visible", out.Attributes["ApproximateNumberOfMessages"])
	set(p.queue, "in_flight", out.Attributes["ApproximateNumberOfMessagesNotVisible"])

	p.discoverDLQ(out.Attributes["RedrivePolicy"])
	p.pollDLQ(ctx)
}

// discoverDLQ resolve a URL da DLQ a partir do RedrivePolicy da fila principal,
// uma vez. A DLQ mora no mesmo endpoint e conta, então basta trocar o último
// segmento da URL pelo nome dela — sem uma chamada extra a GetQueueUrl.
func (p *Poller) discoverDLQ(redrivePolicy string) {
	if p.dlqURL != "" || redrivePolicy == "" {
		return
	}
	var policy struct {
		DeadLetterTargetArn string `json:"deadLetterTargetArn"`
	}
	if err := json.Unmarshal([]byte(redrivePolicy), &policy); err != nil || policy.DeadLetterTargetArn == "" {
		return
	}
	// ARN: arn:aws:sqs:<region>:<account>:<queue-name>
	name := policy.DeadLetterTargetArn[strings.LastIndex(policy.DeadLetterTargetArn, ":")+1:]
	if name == "" {
		return
	}
	p.dlqURL = p.queueURL[:strings.LastIndex(p.queueURL, "/")+1] + name
	p.dlqQueue = name
	p.logger.Info().Str("dlq", name).Msg("DLQ discovered from the RedrivePolicy")
}

func (p *Poller) pollDLQ(ctx context.Context) {
	if p.dlqURL == "" {
		return
	}
	out, err := p.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(p.dlqURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		metrics.Failure(metricsJob, "get_dlq_attributes")
		p.logger.Warn().Err(err).Msg("Failed to read DLQ attributes")
		return
	}
	// A DLQ não é consumida no fluxo normal: só o total parado importa.
	set(p.dlqQueue, "visible", out.Attributes["ApproximateNumberOfMessages"])
}

func queueName(url string) string {
	return url[strings.LastIndex(url, "/")+1:]
}

func set(queue, state, raw string) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	sqsMessages.WithLabelValues(queue, state).Set(v)
}
