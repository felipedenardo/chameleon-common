package metrics_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/felipedenardo/chameleon-common/pkg/metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(metrics.HTTPMiddleware("test-service"))
	r.GET(metrics.Path, gin.WrapH(metrics.Handler()))
	r.GET("/products/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/products", func(c *gin.Context) { c.Status(http.StatusCreated) })
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	return r
}

func do(t *testing.T, r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func scrape(t *testing.T, r *gin.Engine) string {
	t.Helper()
	w := do(t, r, http.MethodGet, metrics.Path)
	require.Equal(t, http.StatusOK, w.Code)
	return w.Body.String()
}

func TestRotaUsaTemplateNaoURL(t *testing.T) {
	r := newRouter()

	do(t, r, http.MethodGet, "/products/9f3a1b7c-0000-4000-8000-000000000001")
	do(t, r, http.MethodGet, "/products/9f3a1b7c-0000-4000-8000-000000000002")

	body := scrape(t, r)

	assert.Contains(t, body, `route="/products/:id"`,
		"deve rotular pelo template da rota, nao pela URL recebida")
	assert.NotContains(t, body, "9f3a1b7c",
		"ID na URL viraria uma serie temporal por request")
	assert.Contains(t, body, `http_requests_total{method="GET",route="/products/:id",service="test-service",status="200"} 2`,
		"os dois requests devem somar na mesma serie")
}

func TestStatusEMetodoViramRotulos(t *testing.T) {
	r := newRouter()

	do(t, r, http.MethodPost, "/products")
	do(t, r, http.MethodGet, "/boom")

	body := scrape(t, r)

	assert.Contains(t, body, `method="POST",route="/products",service="test-service",status="201"`)
	assert.Contains(t, body, `method="GET",route="/boom",service="test-service",status="500"`)
}

func TestRotaInexistenteNaoCriaSeriePorURL(t *testing.T) {
	r := newRouter()

	do(t, r, http.MethodGet, "/nao-existe-1")
	do(t, r, http.MethodGet, "/nao-existe-2")

	body := scrape(t, r)

	assert.Contains(t, body, `route="unmatched"`,
		"404 deve cair num rotulo fixo: varredura de scanner criaria series infinitas")
	assert.NotContains(t, body, "nao-existe")
}

func TestEndpointDeMetricasNaoSeAutoContabiliza(t *testing.T) {
	r := newRouter()

	scrape(t, r)
	body := scrape(t, r)

	assert.NotContains(t, body, `route="/metrics"`,
		"o scrape do Prometheus inflaria a taxa de requisicoes")
}

func TestRuntimeDoGoVemDeGraca(t *testing.T) {
	body := scrape(t, newRouter())

	assert.Contains(t, body, "go_goroutines")
	assert.Contains(t, body, "process_cpu_seconds_total")
}

func TestObserveClassificaPeloErro(t *testing.T) {
	require.NoError(t, metrics.Observe("job_ok", func() error { return nil }))

	err := metrics.Observe("job_ruim", func() error { return errors.New("falhou") })
	require.Error(t, err, "Observe deve propagar o erro, nao engolir")

	body := scrape(t, newRouter())

	assert.Contains(t, body, `chameleon_job_runs_total{job="job_ok",result="success"} 1`)
	assert.Contains(t, body, `chameleon_job_runs_total{job="job_ruim",result="error"} 1`)
	assert.Contains(t, body, `chameleon_job_duration_seconds_count{job="job_ok"} 1`)
}

func TestFailureRegistraFalhaForaDaUnidadeDeTrabalho(t *testing.T) {
	metrics.Failure("consumer_x", "receive")

	body := scrape(t, newRouter())

	assert.Contains(t, body, `chameleon_job_failures_total{job="consumer_x",stage="receive"} 1`)
}

func TestNewServerExpoeSomenteMetricas(t *testing.T) {
	srv := metrics.NewServer("9091")
	require.Equal(t, ":9091", srv.Addr)

	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, metrics.Path, nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "go_goroutines"))

	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/qualquer-outra", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}
