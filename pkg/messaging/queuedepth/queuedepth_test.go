package queuedepth

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// fakeQueue responde os atributos por URL e guarda quais foram lidas.
type fakeQueue struct {
	attributes map[string]map[string]string
	read       []string
	err        error
}

func (q *fakeQueue) GetQueueAttributes(_ context.Context, params *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	url := aws.ToString(params.QueueUrl)
	q.read = append(q.read, url)
	if q.err != nil {
		return nil, q.err
	}
	return &sqs.GetQueueAttributesOutput{Attributes: q.attributes[url]}, nil
}

const (
	queueURL = "http://localstack:4566/000000000000/teste-fila"
	dlqURL   = "http://localstack:4566/000000000000/teste-fila-dlq"
)

func TestPoll_MedeAFilaEDescobreADLQPeloRedrivePolicy(t *testing.T) {
	queue := &fakeQueue{attributes: map[string]map[string]string{
		queueURL: {
			"ApproximateNumberOfMessages":           "4",
			"ApproximateNumberOfMessagesNotVisible": "2",
			"RedrivePolicy":                         `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:000000000000:teste-fila-dlq","maxReceiveCount":"5"}`,
		},
		dlqURL: {"ApproximateNumberOfMessages": "1"},
	}}
	poller := New(queue, queueURL, zerolog.Nop())

	poller.poll(context.Background())
	poller.poll(context.Background())

	require.Equal(t, float64(4), testutil.ToFloat64(sqsMessages.WithLabelValues("teste-fila", "visible")))
	require.Equal(t, float64(2), testutil.ToFloat64(sqsMessages.WithLabelValues("teste-fila", "in_flight")))
	require.Equal(t, float64(1), testutil.ToFloat64(sqsMessages.WithLabelValues("teste-fila-dlq", "visible")))
	require.Equal(t, []string{queueURL, dlqURL, queueURL, dlqURL}, queue.read, "a DLQ é descoberta uma vez e lida a cada passada")
}

func TestPoll_SemRedrivePolicyNaoLeDLQ(t *testing.T) {
	queue := &fakeQueue{attributes: map[string]map[string]string{queueURL: {"ApproximateNumberOfMessages": "0"}}}

	New(queue, queueURL, zerolog.Nop()).poll(context.Background())

	require.Equal(t, []string{queueURL}, queue.read)
}

func TestPoll_FalhaNaLeituraNaoPara(t *testing.T) {
	queue := &fakeQueue{err: errors.New("fila fora")}

	New(queue, queueURL, zerolog.Nop()).poll(context.Background())

	require.Equal(t, []string{queueURL}, queue.read)
}

func TestStart_SemFilaConfiguradaNaoMede(t *testing.T) {
	New(nil, "", zerolog.Nop()).Start(context.Background())
}
