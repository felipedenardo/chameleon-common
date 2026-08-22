package httpserver

import (
	"net/http"
	"time"

	"github.com/felipedenardo/chameleon-common/pkg/metrics"
	"github.com/felipedenardo/chameleon-common/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Options configura o servidor HTTP padrão dos serviços Chameleon.
type Options struct {
	ServiceName  string
	Port         string
	BasePath     string
	MaxBodyBytes int64
	Swagger      bool
}

// New monta um *http.Server com o stack padrão dos serviços: recovery,
// request logger, limite de corpo, security headers, métricas, swagger
// opcional e endpoint de health. O registro das rotas de negócio fica a cargo do
// callback register, que recebe o grupo /api/v1 já criado — assim cada
// serviço só se preocupa com as próprias rotas e seus middlewares.
func New(logger zerolog.Logger, opts Options, register func(api *gin.RouterGroup)) *http.Server {
	r := gin.New()
	r.Use(
		gin.Recovery(),
		middleware.RequestLogger(logger),
		middleware.MaxBodyBytes(opts.MaxBodyBytes),
		middleware.SecurityHeaders(),
		metrics.HTTPMiddleware(opts.ServiceName),
	)

	// Fora do BasePath de propósito: o Kong roteia por /chameleon-<serviço>,
	// então /metrics na raiz não casa com rota nenhuma e fica inalcançável
	// pela borda. O Prometheus raspa pela rede interna, pelo nome do container.
	r.GET(metrics.Path, gin.WrapH(metrics.Handler()))

	basePath := r.Group(opts.BasePath)
	if opts.Swagger {
		basePath.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	api := basePath.Group("/api/v1")
	register(api)
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": opts.ServiceName})
	})

	return &http.Server{
		Addr:              ":" + opts.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
