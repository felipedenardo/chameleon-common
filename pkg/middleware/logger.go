package middleware

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// sensitiveName é o nome de parâmetro (da consulta ou da rota) cujo valor não
// vai para o log: o token de verificação do webhook da Meta
// (hub.verify_token), o do link de ativação e o do convite (/:token), o
// código do OAuth. O log sai do serviço (CloudWatch, Grafana) e é lido por
// quem não deveria ter esses valores.
var sensitiveName = regexp.MustCompile(`(?i)(token|secret|password|senha|code|key|signature|ticket)`)

const redacted = "<oculto>"

func RequestLogger(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Fill metrics
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		entry := logger.Info()
		if statusCode >= 400 && statusCode < 500 {
			entry = logger.Warn()
		} else if statusCode >= 500 {
			entry = logger.Error()
		}

		// Log entry with rich metadata
		entry.
			Str("method", method).
			Str("path", loggedPath(c)).
			Int("status", statusCode).
			Dur("latency", latency).
			Str("ip", clientIP).
			Str("user_agent", c.Request.UserAgent()).
			Msg("HTTP Request")
	}
}

// loggedPath é o endereço da chamada sem os segredos: o valor de parâmetro de
// rota com nome sensível vira <oculto> no caminho, e o de consulta também.
func loggedPath(c *gin.Context) string {
	path := c.Request.URL.Path
	for _, param := range c.Params {
		if param.Value != "" && sensitiveName.MatchString(param.Key) {
			path = strings.Replace(path, param.Value, redacted, 1)
		}
	}
	if raw := c.Request.URL.RawQuery; raw != "" {
		path += "?" + redactQuery(raw)
	}
	return path
}

// redactQuery remonta a consulta com os valores sensíveis escondidos. A
// consulta que não se lê sai inteira escondida: melhor perder o detalhe do
// que gravar o segredo que estava nela.
func redactQuery(raw string) string {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return redacted
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(values))
	for _, key := range keys {
		for _, value := range values[key] {
			if sensitiveName.MatchString(key) {
				value = redacted
			} else {
				value = url.QueryEscape(value)
			}
			parts = append(parts, url.QueryEscape(key)+"="+value)
		}
	}
	return strings.Join(parts, "&")
}
