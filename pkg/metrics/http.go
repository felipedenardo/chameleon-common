package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Nomes padrão da convenção Prometheus, sem prefixo de vendor: dashboards e
// alertas prontos da comunidade funcionam sem tradução.
var (
	httpRequests = NewCounter(
		"http_requests_total",
		"Total de requisições HTTP atendidas.",
		"service", "method", "route", "status",
	)

	httpDuration = NewHistogram(
		"http_request_duration_seconds",
		"Duração das requisições HTTP em segundos.",
		nil,
		// Sem o rótulo status de propósito: ele multiplicaria cada bucket
		// pelo número de códigos observados. A taxa de erro sai do contador.
		"service", "method", "route",
	)
)

// HTTPMiddleware instrumenta toda requisição com as três grandezas do RED:
// taxa e erros pelo contador, duração pelo histograma.
func HTTPMiddleware(service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		route := c.FullPath()

		// O próprio endpoint de métricas não entra na conta: o scrape do
		// Prometheus inflaria a taxa de requisições com tráfego interno.
		if route == Path {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		// FullPath é o template da rota registrada ("/products/:id"), não a
		// URL recebida ("/products/9f3a-..."). Usar a URL faria cada ID virar
		// uma série temporal nova.
		//
		// Requisição que não casou com rota nenhuma tem FullPath vazio: vira
		// um rótulo fixo, senão qualquer varredura de scanner criaria séries.
		if route == "" {
			route = "unmatched"
		}

		httpRequests.WithLabelValues(
			service, c.Request.Method, route, strconv.Itoa(c.Writer.Status()),
		).Inc()

		httpDuration.WithLabelValues(
			service, c.Request.Method, route,
		).Observe(time.Since(start).Seconds())
	}
}
