package log

import (
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

// New devolve um logger zerolog padronizado para os serviços Chameleon,
// já com o nome do serviço, o ambiente e o nível de log aplicados.
func New(serviceName, env, level string) zerolog.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	parsed, err := zerolog.ParseLevel(level)
	if err != nil {
		parsed = zerolog.InfoLevel
	}

	return zlog.With().
		Str("service", serviceName).
		Str("env", env).
		Logger().
		Level(parsed)
}
