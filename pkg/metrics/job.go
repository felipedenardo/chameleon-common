package metrics

import (
	"net/http"
	"time"
)

// Métricas dos processos que não atendem HTTP (consumidores de fila, relays).
// Prefixo chameleon_ porque são convenção nossa, não da comunidade.
var (
	jobRuns = NewCounter(
		"chameleon_job_runs_total",
		"Total de execuções de uma unidade de trabalho, por resultado.",
		"job", "result",
	)

	jobDuration = NewHistogram(
		"chameleon_job_duration_seconds",
		"Duração de uma unidade de trabalho em segundos.",
		nil,
		"job",
	)

	jobFailures = NewCounter(
		"chameleon_job_failures_total",
		"Falhas fora da unidade de trabalho, por etapa do ciclo.",
		"job", "stage",
	)
)

// Observe mede uma unidade de trabalho — processar uma mensagem, drenar um
// lote — e classifica o resultado pelo erro retornado. É o equivalente do
// middleware HTTP para quem não atende requisição: envolve a execução sem
// que a lógica de negócio saiba.
func Observe(job string, fn func() error) error {
	start := time.Now()
	err := fn()

	result := "success"
	if err != nil {
		result = "error"
	}

	jobRuns.WithLabelValues(job, result).Inc()
	jobDuration.WithLabelValues(job).Observe(time.Since(start).Seconds())

	return err
}

// Failure registra falha que acontece ANTES da unidade de trabalho e impede
// que ela rode — fila inalcançável, credencial expirada, consulta ao banco
// que falhou. Sem isso o contador do Observe fica parado, e parado é
// indistinguível de "não havia trabalho a fazer".
func Failure(job, stage string) {
	jobFailures.WithLabelValues(job, stage).Inc()
}

// NewServer monta o servidor HTTP mínimo que expõe /metrics num processo que
// não é servidor. O Prometheus é pull: quem quer ser medido precisa atender.
//
// Devolve o *http.Server para o chamador gerenciar o ciclo de vida, no mesmo
// formato de httpserver.New.
func NewServer(port string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(Path, Handler())

	return &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}
