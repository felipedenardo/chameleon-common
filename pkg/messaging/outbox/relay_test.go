package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// fakeStore é o outbox em memória. Guarda cada escrita como veio: o relay
// precisa fazer uma por desfecho, não uma por evento.
type fakeStore struct {
	pending   []Event
	published [][]uuid.UUID
	failed    [][]uuid.UUID
	attempts  [][]uuid.UUID
	fail      map[string]error
	counts    [2]int64
}

func (s *fakeStore) FetchPending(_ context.Context, limit int) ([]Event, error) {
	if err := s.fail["fetch"]; err != nil {
		return nil, err
	}
	return s.pending[:min(limit, len(s.pending))], nil
}

func (s *fakeStore) MarkPublished(_ context.Context, ids []uuid.UUID) error {
	s.published = append(s.published, ids)
	return s.fail["published"]
}

func (s *fakeStore) MarkFailed(_ context.Context, ids []uuid.UUID) error {
	s.failed = append(s.failed, ids)
	return s.fail["failed"]
}

func (s *fakeStore) IncrementAttempts(_ context.Context, ids []uuid.UUID) error {
	s.attempts = append(s.attempts, ids)
	return s.fail["attempts"]
}

func (s *fakeStore) CountPending(context.Context) (int64, error) { return s.counts[0], s.fail["count"] }
func (s *fakeStore) CountFailed(context.Context) (int64, error)  { return s.counts[1], s.fail["count"] }

// fakeQueue aceita tudo, menos o que estiver em refuse (true = culpa do
// corpo, false = culpa da fila).
type fakeQueue struct {
	calls  [][]string
	refuse map[string]bool
	err    error
}

func (q *fakeQueue) SendMessageBatch(_ context.Context, params *sqs.SendMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	var bodies []string
	out := &sqs.SendMessageBatchOutput{}
	for _, entry := range params.Entries {
		body := aws.ToString(entry.MessageBody)
		bodies = append(bodies, body)
		if senderFault, refused := q.refuse[body]; refused {
			out.Failed = append(out.Failed, types.BatchResultErrorEntry{Id: entry.Id, Code: aws.String("InvalidMessageContents"), SenderFault: senderFault})
			continue
		}
		out.Successful = append(out.Successful, types.SendMessageBatchResultEntry{Id: entry.Id})
	}
	q.calls = append(q.calls, bodies)
	if q.err != nil {
		return nil, q.err
	}
	return out, nil
}

func events(bodies ...string) []Event {
	out := make([]Event, 0, len(bodies))
	for _, body := range bodies {
		out = append(out, Event{ID: uuid.New(), Type: "sale.created", Body: body})
	}
	return out
}

func idsOf(evs ...Event) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(evs))
	for _, event := range evs {
		out = append(out, event.ID)
	}
	return out
}

func newRelay(store *fakeStore, queue *fakeQueue) *Relay {
	return New(store, queue, Options{QueueURL: "http://fila", Enabled: true}, zerolog.Nop())
}

func TestDrain_PublicaOLoteNumEnvioEMarcaNumaEscrita(t *testing.T) {
	evs := events("a", "b", "c")
	store, queue := &fakeStore{pending: evs}, &fakeQueue{}

	result, err := newRelay(store, queue).drain(context.Background())

	require.NoError(t, err)
	require.Equal(t, [][]string{{"a", "b", "c"}}, queue.calls, "um envio para o lote")
	require.Equal(t, [][]uuid.UUID{idsOf(evs...)}, store.published, "uma escrita para todos")
	require.Empty(t, store.failed)
	require.Empty(t, store.attempts)
	require.Equal(t, 3, result.handled)
	require.False(t, result.more, "esvaziou")
}

func TestDrain_LoteCheioSegueSemEsperar(t *testing.T) {
	store := &fakeStore{pending: events("1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11")}

	result, err := newRelay(store, &fakeQueue{}).drain(context.Background())

	require.NoError(t, err)
	require.Equal(t, 10, result.handled)
	require.True(t, result.more)
}

// O corpo maior que a fila aceita nem vai: vira FAILED, e os outros seguem.
func TestDrain_CorpoGrandeDemaisViraFailedSemEnviar(t *testing.T) {
	big := strings.Repeat("x", maxMessageBytes+1)
	evs := events("a", big, "c")
	store, queue := &fakeStore{pending: evs}, &fakeQueue{}

	_, err := newRelay(store, queue).drain(context.Background())

	require.NoError(t, err)
	require.Equal(t, [][]string{{"a", "c"}}, queue.calls)
	require.Equal(t, [][]uuid.UUID{idsOf(evs[1])}, store.failed)
	require.Equal(t, [][]uuid.UUID{idsOf(evs[0], evs[2])}, store.published)
}

// A soma dos corpos de um envio também tem teto: o que não cabe fica para a
// próxima passada, em vez de um segundo envio na mesma.
func TestDrain_OQueNaoCabeNoEnvioFicaParaAProxima(t *testing.T) {
	half := strings.Repeat("x", maxBatchBytes/2)
	evs := events(half, half, "c")
	store, queue := &fakeStore{pending: evs}, &fakeQueue{}

	result, err := newRelay(store, queue).drain(context.Background())

	require.NoError(t, err)
	require.Len(t, queue.calls, 1, "um envio por passada")
	require.Equal(t, [][]uuid.UUID{idsOf(evs[0], evs[1])}, store.published)
	require.True(t, result.more)
}

// Culpa do corpo (SenderFault) é recusa de vez; culpa da fila vai de novo.
func TestDrain_RecusaPorEntradaSeparaCulpaDoCorpoDaCulpaDaFila(t *testing.T) {
	evs := events("ok", "ruim", "depois")
	store := &fakeStore{pending: evs}
	queue := &fakeQueue{refuse: map[string]bool{"ruim": true, "depois": false}}

	_, err := newRelay(store, queue).drain(context.Background())

	require.NoError(t, err)
	require.Equal(t, [][]uuid.UUID{idsOf(evs[0])}, store.published)
	require.Equal(t, [][]uuid.UUID{idsOf(evs[1])}, store.failed)
	require.Equal(t, [][]uuid.UUID{idsOf(evs[2])}, store.attempts)
}

// Fila fora: o envio inteiro falha, todos ganham uma tentativa e o erro sobe
// para o relay esperar antes da próxima passada.
func TestDrain_FilaForaContaTentativaEDevolveOErro(t *testing.T) {
	evs := events("a", "b")
	store := &fakeStore{pending: evs}
	boom := errors.New("fila fora")

	_, err := newRelay(store, &fakeQueue{err: boom}).drain(context.Background())

	require.ErrorIs(t, err, boom)
	require.Equal(t, [][]uuid.UUID{idsOf(evs...)}, store.attempts)
	require.Empty(t, store.published)
}

func TestDrain_OutboxVazioNaoEnvia(t *testing.T) {
	queue := &fakeQueue{}

	result, err := newRelay(&fakeStore{}, queue).drain(context.Background())

	require.NoError(t, err)
	require.Empty(t, queue.calls)
	require.False(t, result.more)
}

func TestDrain_PropagaErrosDoOutbox(t *testing.T) {
	for _, step := range []string{"fetch", "published"} {
		t.Run(step, func(t *testing.T) {
			boom := errors.New("banco fora")
			store := &fakeStore{pending: events("a"), fail: map[string]error{step: boom}}

			_, err := newRelay(store, &fakeQueue{}).drain(context.Background())

			require.ErrorIs(t, err, boom)
		})
	}
}

func TestObserve_AtualizaPendentesERecusadosDoDestino(t *testing.T) {
	store := &fakeStore{counts: [2]int64{3, 1}}
	relay := New(store, &fakeQueue{}, Options{QueueURL: "http://fila", Enabled: true, Stream: "teste"}, zerolog.Nop())

	relay.observe(context.Background())

	require.Equal(t, float64(3), testutil.ToFloat64(pendingGauge.WithLabelValues("teste")))
	require.Equal(t, float64(1), testutil.ToFloat64(failedGauge.WithLabelValues("teste")))
}

func TestStart_DesligadoNaoPublica(t *testing.T) {
	relay := New(&fakeStore{}, nil, Options{Enabled: true}, zerolog.Nop())
	require.NoError(t, relay.Start(context.Background()))

	relay = New(&fakeStore{}, &fakeQueue{}, Options{Enabled: false}, zerolog.Nop())
	require.NoError(t, relay.Start(context.Background()))
}

func TestStart_PublicaAteOContextoAcabar(t *testing.T) {
	store := &fakeStore{pending: events("a")}
	queue := &fakeQueue{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	require.NoError(t, New(store, queue, Options{QueueURL: "http://fila", Enabled: true, PollInterval: time.Hour}, zerolog.Nop()).Start(ctx))
	require.Len(t, queue.calls, 1)
}
