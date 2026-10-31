package consumer

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// fakeQueue entrega os lotes em ordem e, quando acabam, espera o fim do
// contexto. Guarda cada chamada de confirmação: o que importa é quantas
// chamadas saem e com o quê.
type fakeQueue struct {
	mu          sync.Mutex
	batches     [][]types.Message
	receiveErrs []error
	received    []*sqs.ReceiveMessageInput
	deletes     [][]string
	postpones   []map[string]int32
	drained     chan struct{}
	once        sync.Once
}

func newFakeQueue(batches ...[]types.Message) *fakeQueue {
	return &fakeQueue{batches: batches, drained: make(chan struct{})}
}

func (f *fakeQueue) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	f.mu.Lock()
	f.received = append(f.received, params)
	if len(f.receiveErrs) > 0 {
		err := f.receiveErrs[0]
		f.receiveErrs = f.receiveErrs[1:]
		f.mu.Unlock()
		return nil, err
	}
	if len(f.batches) > 0 {
		batch := f.batches[0]
		f.batches = f.batches[1:]
		f.mu.Unlock()
		return &sqs.ReceiveMessageOutput{Messages: batch}, nil
	}
	f.mu.Unlock()
	f.once.Do(func() { close(f.drained) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *fakeQueue) DeleteMessageBatch(_ context.Context, params *sqs.DeleteMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var handles []string
	for _, entry := range params.Entries {
		handles = append(handles, aws.ToString(entry.ReceiptHandle))
	}
	f.deletes = append(f.deletes, handles)
	return &sqs.DeleteMessageBatchOutput{}, nil
}

func (f *fakeQueue) ChangeMessageVisibilityBatch(_ context.Context, params *sqs.ChangeMessageVisibilityBatchInput, _ ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityBatchOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	timeouts := map[string]int32{}
	for _, entry := range params.Entries {
		timeouts[aws.ToString(entry.ReceiptHandle)] = entry.VisibilityTimeout
	}
	f.postpones = append(f.postpones, timeouts)
	return &sqs.ChangeMessageVisibilityBatchOutput{}, nil
}

func message(handle string, receives int) types.Message {
	return types.Message{
		ReceiptHandle: aws.String(handle), MessageId: aws.String(handle), Body: aws.String(handle),
		Attributes: map[string]string{"ApproximateReceiveCount": strconv.Itoa(receives)},
	}
}

func options() Options {
	return Options{Name: "test_consumer", QueueURL: "http://fila", Enabled: true, VisibilityTimeout: 30, MaxMessages: 10}
}

// run roda o consumidor até a fila esvaziar.
func run(t *testing.T, c *Consumer, queue *fakeQueue) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- c.Start(ctx) }()
	select {
	case <-queue.drained:
	case <-time.After(5 * time.Second):
		t.Fatal("o consumidor não voltou a pedir mensagens")
	}
	cancel()
	require.NoError(t, <-done)
}

func TestNew_ConfirmaOLoteNumaChamadaParaApagarENumaParaAdiar(t *testing.T) {
	queue := newFakeQueue([]types.Message{message("ok", 1), message("passageiro", 3), message("ruim", 1), message("ok2", 1)})
	handle := func(_ context.Context, messages []types.Message) []error {
		require.Len(t, messages, 4, "o handler recebe o lote inteiro")
		return []error{nil, errors.New("banco fora"), Permanent(errors.New("mal formada")), nil}
	}

	run(t, New(queue, options(), handle, zerolog.Nop()), queue)

	require.Equal(t, [][]string{{"ok", "ok2"}}, queue.deletes, "uma chamada apaga as tratadas")
	require.Equal(t, []map[string]int32{{"passageiro": 120}}, queue.postpones, "uma chamada adia a passageira, pela terceira entrega")
	require.Equal(t, int32(10), queue.received[0].MaxNumberOfMessages)
	require.Equal(t, int32(20), queue.received[0].WaitTimeSeconds)
	require.Equal(t, int32(30), queue.received[0].VisibilityTimeout)
	require.Equal(t, []types.MessageSystemAttributeName{types.MessageSystemAttributeNameApproximateReceiveCount},
		queue.received[0].MessageSystemAttributeNames, "pede a contagem de entregas para calcular a espera")
}

func TestNewSingle_RecebeUmaPorVez(t *testing.T) {
	queue := newFakeQueue([]types.Message{message("a", 1)}, []types.Message{message("b", 2)})
	var seen []string
	handle := func(_ context.Context, m types.Message) error {
		seen = append(seen, aws.ToString(m.Body))
		if aws.ToString(m.Body) == "b" {
			return errors.New("banco fora")
		}
		return nil
	}

	run(t, NewSingle(queue, options(), handle, zerolog.Nop()), queue)

	require.Equal(t, []string{"a", "b"}, seen)
	require.Equal(t, int32(1), queue.received[0].MaxNumberOfMessages)
	require.Equal(t, [][]string{{"a"}}, queue.deletes)
	require.Equal(t, []map[string]int32{{"b": 60}}, queue.postpones)
}

// O handler que devolve menos desfechos que mensagens não apaga a que ficou
// sem resposta: ela é adiada, como uma falha passageira.
func TestNew_MensagemSemDesfechoEAdiada(t *testing.T) {
	queue := newFakeQueue([]types.Message{message("a", 1), message("b", 1)})

	run(t, New(queue, options(), func(context.Context, []types.Message) []error { return []error{nil} }, zerolog.Nop()), queue)

	require.Equal(t, [][]string{{"a"}}, queue.deletes)
	require.Equal(t, []map[string]int32{{"b": 30}}, queue.postpones)
}

func TestStart_ErroAoReceberEsperaETentaDeNovo(t *testing.T) {
	queue := newFakeQueue([]types.Message{message("a", 1)})
	queue.receiveErrs = []error{errors.New("fila fora")}

	run(t, NewSingle(queue, options(), func(context.Context, types.Message) error { return nil }, zerolog.Nop()), queue)

	require.Equal(t, [][]string{{"a"}}, queue.deletes, "depois do erro, volta a ler")
}

func TestStart_DesligadoNaoLe(t *testing.T) {
	opts := options()
	opts.Enabled = false

	require.NoError(t, NewSingle(nil, opts, func(context.Context, types.Message) error { return nil }, zerolog.Nop()).Start(context.Background()))
}

func TestStart_LigadoSemClientEErro(t *testing.T) {
	require.Error(t, NewSingle(nil, options(), func(context.Context, types.Message) error { return nil }, zerolog.Nop()).Start(context.Background()))
}

func TestNew_LoteLimitadoA10(t *testing.T) {
	opts := options()
	opts.MaxMessages = 50
	require.Equal(t, int32(10), New(nil, opts, nil, zerolog.Nop()).opts.MaxMessages)
	opts.MaxMessages = 0
	require.Equal(t, int32(1), New(nil, opts, nil, zerolog.Nop()).opts.MaxMessages)
}

func TestRetryDelay_DobraACadaEntregaAte15Minutos(t *testing.T) {
	for count, want := range map[string]time.Duration{
		"": 30 * time.Second, "x": 30 * time.Second, "0": 30 * time.Second,
		"1": 30 * time.Second, "2": time.Minute, "3": 2 * time.Minute, "4": 4 * time.Minute,
		"5": 8 * time.Minute, "6": 15 * time.Minute, "999": 15 * time.Minute,
	} {
		m := types.Message{Attributes: map[string]string{"ApproximateReceiveCount": count}}
		require.Equal(t, want, retryDelay(m), "entrega %q", count)
	}
}

// Somando as esperas das 10 entregas das filas de cobrança: 1h30 de tolerância.
func TestRetryDelay_DezEntregasSomamUmaHoraEMeia(t *testing.T) {
	var total time.Duration
	for count := 1; count <= 10; count++ {
		total += retryDelay(message("x", count))
	}
	require.Equal(t, 90*time.Minute+30*time.Second, total)
}

func TestPermanent(t *testing.T) {
	require.Nil(t, Permanent(nil))
	cause := errors.New("mal formada")
	err := Permanent(cause)
	require.True(t, IsPermanent(err))
	require.True(t, IsPermanent(errors.Join(errors.New("contexto"), err)), "embrulhado continua permanente")
	require.ErrorIs(t, err, cause)
	require.False(t, IsPermanent(cause))
}
