// Package metrics concentra a instrumentação Prometheus dos serviços
// Chameleon: o registry, os coletores de runtime e os construtores usados
// pelos serviços para declarar métricas próprias.
//
// O registry é próprio do pacote, e não o global do client_golang. Isso
// mantém o que é exposto sob controle desta biblioteca e evita que duas
// registrações do mesmo nome derrubem o processo.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Path é onde o handler de métricas é exposto, tanto nos serviços HTTP
// quanto nos workers.
const Path = "/metrics"

var registry = prometheus.NewRegistry()

func init() {
	// Runtime do Go (goroutines, heap, GC) e do processo (CPU, file
	// descriptors). Saem de graça e respondem boa parte das perguntas de
	// saúde sem nenhuma instrumentação manual.
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Handler serve o endpoint de exposição das métricas.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		// Erro na coleta vira 500 com o motivo no corpo, em vez de resposta
		// parcial que o Prometheus registraria como sucesso.
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
}

// NewCounter declara um contador no registry do pacote. Use para grandezas
// que só crescem: eventos processados, falhas, mensagens publicadas.
func NewCounter(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	registry.MustRegister(c)
	return c
}

// NewGauge declara um medidor no registry do pacote. Use para grandezas que
// sobem e descem e cujo valor instantâneo importa: itens pendentes numa
// fila, conexões abertas.
func NewGauge(name, help string, labels ...string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	registry.MustRegister(g)
	return g
}

// NewHistogram declara um histograma no registry do pacote. buckets vazio
// usa os buckets default do client_golang, adequados a latência em segundos.
func NewHistogram(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: buckets,
	}, labels)
	registry.MustRegister(h)
	return h
}
