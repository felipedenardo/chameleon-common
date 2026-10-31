package servicecall

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

// O destino decide em quem confia pelo subject do token: quem chama assina com
// o próprio nome.
func TestDo_AssinaComOSubjectDeQuemChama(t *testing.T) {
	key := newKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		token, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
		require.NoError(t, err)
		subject, err := token.Claims.GetSubject()
		require.NoError(t, err)
		require.Equal(t, "chameleon-purchase-api", subject)
		require.Equal(t, "/base/api/v1/x", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer server.Close()

	status, body, err := New("x", "chameleon-purchase-api", server.URL+"/base/", key, time.Second).Do(context.Background(), http.MethodGet, "/api/v1/x", nil)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	data, err := Decode[struct {
		OK bool `json:"ok"`
	}](body)
	require.NoError(t, err)
	require.True(t, data.OK)
}

func TestDo_EnviaOCorpoComoJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	status, _, err := New("x", "s", server.URL, newKey(t), time.Second).Do(context.Background(), http.MethodPost, "/", map[string]int{"a": 1})

	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
}

// 4xx é resposta do destino: volta com o status, sem erro, e não abre o
// circuito.
func TestDo_4xxVoltaComoResposta(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	caller := New("x", "s", server.URL, newKey(t), time.Second)

	for range 10 {
		status, _, err := caller.Do(context.Background(), http.MethodGet, "/", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, status)
	}
	require.Equal(t, 10, calls, "o circuito segue fechado")
}

func TestDo_5xxEIndisponivel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, _, err := New("x", "s", server.URL, newKey(t), time.Second).Do(context.Background(), http.MethodGet, "/", nil)

	require.ErrorIs(t, err, ErrUnavailable)
}

func TestDo_DestinoForaDoArEIndisponivel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()

	_, _, err := New("x", "s", server.URL, newKey(t), time.Second).Do(context.Background(), http.MethodGet, "/", nil)

	require.ErrorIs(t, err, ErrUnavailable)
}

// Cinco falhas seguidas abrem o circuito: a sexta chamada nem sai.
func TestDo_CircuitoAbertoNaoChamaEEIndisponivel(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	caller := New("x", "s", server.URL, newKey(t), time.Second)

	for range 6 {
		_, _, err := caller.Do(context.Background(), http.MethodGet, "/", nil)
		require.ErrorIs(t, err, ErrUnavailable)
	}
	require.Equal(t, 5, calls)
}

// Quem chamou desistiu: não é culpa do destino, e o erro é o do contexto.
func TestDo_ChamadaAbandonadaNaoEIndisponivel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := New("x", "s", server.URL, newKey(t), time.Second).Do(ctx, http.MethodGet, "/", nil)

	require.Error(t, err)
	require.False(t, errors.Is(err, ErrUnavailable))
	require.ErrorIs(t, err, context.Canceled)
}
