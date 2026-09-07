// Package redisclient monta o cliente do Redis compartilhado a partir das
// variáveis de ambiente que todos os serviços usam.
//
// Existe para que os seis serviços que só precisam LER a lista de revogação
// não repitam quarenta linhas de configuração cada um — a divergência entre
// essas cópias (um timeout diferente, um default trocado) é o tipo de coisa
// que só aparece em produção.
package redisclient

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config são os parâmetros de conexão. FromEnv preenche a partir das mesmas
// variáveis que o auth-api já usa.
type Config struct {
	Host           string
	Port           string
	Password       string
	DB             int
	DialTimeoutMs  int
	ReadTimeoutMs  int
	WriteTimeoutMs int
}

// FromEnv lê a configuração do ambiente, com os mesmos defaults do auth-api.
func FromEnv() Config {
	return Config{
		Host:           envOr("REDIS_HOST", "localhost"),
		Port:           envOr("REDIS_PORT", "6379"),
		Password:       os.Getenv("REDIS_PASSWORD"),
		DB:             envIntOr("REDIS_DB", 0),
		DialTimeoutMs:  envIntOr("REDIS_DIAL_TIMEOUT_MS", 2000),
		ReadTimeoutMs:  envIntOr("REDIS_READ_TIMEOUT_MS", 2000),
		WriteTimeoutMs: envIntOr("REDIS_WRITE_TIMEOUT_MS", 2000),
	}
}

// New abre a conexão e confirma que o Redis responde.
//
// Falha aqui derruba o boot de propósito: um serviço que não alcança a lista
// de revogação estaria aceitando token deslogado sem saber, e é melhor não
// subir do que subir cego. Depois de conectado, falha de leitura degrada em
// vez de derrubar — a decisão é outra, e mora no verificador.
func New(cfg Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  time.Duration(cfg.DialTimeoutMs) * time.Millisecond,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(cfg.WriteTimeoutMs) * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("conectar ao Redis (%s:%s): %w", cfg.Host, cfg.Port, err)
	}
	return client, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
