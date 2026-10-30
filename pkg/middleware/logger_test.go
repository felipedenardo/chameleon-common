package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/felipedenardo/chameleon-common/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func logDaChamada(t *testing.T, route, target string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var out bytes.Buffer
	router := gin.New()
	router.Use(middleware.RequestLogger(zerolog.New(&out)))
	router.GET(route, func(c *gin.Context) { c.Status(http.StatusOK) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	return out.String()
}

// O token de verificação do webhook da Meta vinha na consulta e ia inteiro
// para o log; o resto da consulta continua lá, para depurar.
func TestRequestLogger_EscondeSegredoDaConsulta(t *testing.T) {
	line := logDaChamada(t, "/webhooks/whatsapp", "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=s3cr3t&hub.challenge=4242")
	require.NotContains(t, line, "s3cr3t")
	require.Contains(t, line, "hub.verify_token=<oculto>")
	require.Contains(t, line, "hub.mode=subscribe")
	require.Contains(t, line, "hub.challenge=4242")
}

// O token do link de ativação e o do convite vêm no caminho.
func TestRequestLogger_EscondeSegredoDaRota(t *testing.T) {
	line := logDaChamada(t, "/links/:token", "/links/abc123token?establishment_id=e-1")
	require.NotContains(t, line, "abc123token")
	require.Contains(t, line, "/links/<oculto>?establishment_id=e-1")
}

// Id de rota que não é segredo continua no log.
func TestRequestLogger_MantemOResto(t *testing.T) {
	line := logDaChamada(t, "/customers/:customerID", "/customers/9a2899e9?page=2")
	require.Contains(t, line, "/customers/9a2899e9?page=2")
}
