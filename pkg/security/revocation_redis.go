package security

import (
	"context"
	"errors"
	"time"

	"github.com/felipedenardo/chameleon-common/pkg/metrics"
	"github.com/redis/go-redis/v9"
)

// Chaves gravadas pelo auth-api. São o contrato entre ele e os demais
// serviços: um jti na blacklist, um inteiro por usuário na versão.
const (
	blacklistKeyPrefix    = "auth:blacklist:"
	tokenVersionKeyPrefix = "auth:token_version:"
)

// revocationOpTimeout é curto de propósito. Esta leitura está no caminho de
// TODA requisição autenticada: se o Redis engasgar, é melhor degradar rápido
// (ver RedisRevocationChecker) do que segurar o request.
const revocationOpTimeout = 500 * time.Millisecond

// revocationDegraded conta as vezes em que a checagem não pôde ser feita e a
// requisição passou assim mesmo. É o que dá base ao alerta: sem ela, o sistema
// fica sem revogação e ninguém descobre.
var revocationDegraded = metrics.NewCounter(
	"auth_revocation_check_degraded_total",
	"Checagens de revogação que falharam e resultaram em liberação da requisição.",
	"service", "check",
)

// RedisRevocationChecker responde as duas perguntas de revogação lendo o Redis
// que o auth-api escreve: este token foi deslogado, e esta versão ainda vale.
//
// # Por que ler direto, em vez de perguntar ao auth-api
//
// Uma chamada HTTP por requisição autenticada, em seis serviços, seria o hop
// mais caro do sistema. O Redis é infraestrutura compartilhada de propósito
// aqui — o contrato é uma chave com um inteiro, somente-leitura deste lado.
//
// # Por que liberar quando o Redis falha
//
// Falha de leitura libera a requisição e incrementa revocationDegraded. Não é
// concessão: é o comportamento que estes serviços já tinham antes de existir
// checagem alguma, então indisponibilidade do Redis degrada para o estado
// anterior em vez de derrubar o sistema inteiro. O alerta sobre a métrica é o
// que impede isso de virar permanente e silencioso.
//
// Ausência de chave NÃO é falha: significa que não há revogação registrada, e
// liberar é a resposta correta. O auth-api grava a versão a cada incremento
// com TTL maior que a vida máxima do token, então a ausência é informação, não
// ignorância.
type RedisRevocationChecker struct {
	client  *redis.Client
	service string
}

// NewRedisRevocationChecker liga o verificador ao Redis do auth. service
// rotula a métrica de degradação.
func NewRedisRevocationChecker(client *redis.Client, service string) *RedisRevocationChecker {
	return &RedisRevocationChecker{client: client, service: service}
}

// IsTokenBlacklisted diz se aquele token específico foi deslogado.
//
// Devolve sempre erro nil: o middleware trata erro como motivo para recusar, e
// aqui a decisão de degradar é desta camada, que sabe distinguir "não há
// revogação" de "não consegui perguntar".
func (c *RedisRevocationChecker) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	opCtx, cancel := context.WithTimeout(ctx, revocationOpTimeout)
	defer cancel()

	found, err := c.client.Exists(opCtx, blacklistKeyPrefix+jti).Result()
	if err != nil {
		c.degraded("blacklist")
		return false, nil
	}
	return found == 1, nil
}

// GetUserTokenVersion devolve a versão mínima que um token daquele usuário
// precisa carregar para ainda valer.
//
// Chave ausente devolve 0, que nenhuma versão de token reprova — usuário que
// nunca teve token revogado passa direto.
func (c *RedisRevocationChecker) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	opCtx, cancel := context.WithTimeout(ctx, revocationOpTimeout)
	defer cancel()

	version, err := c.client.Get(opCtx, tokenVersionKeyPrefix+userID).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		c.degraded("token_version")
		return 0, nil
	}
	return version, nil
}

func (c *RedisRevocationChecker) degraded(check string) {
	revocationDegraded.WithLabelValues(c.service, check).Inc()
}
