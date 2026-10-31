package security

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// newChecker liga o verificador a um Redis em memória. O nome do serviço é
// único por teste para a métrica de degradação não somar entre testes.
func newChecker(t *testing.T) (*RedisRevocationChecker, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisRevocationChecker(client, t.Name()), server
}

func degradedCount(t *testing.T, check string) float64 {
	t.Helper()
	return testutil.ToFloat64(revocationDegraded.WithLabelValues(t.Name(), check))
}

// As chaves são as que o auth-api grava: mudar o prefixo aqui sem mudar lá
// desliga a revogação em silêncio.
func TestIsTokenBlacklisted_LeAChaveDoAuth(t *testing.T) {
	checker, server := newChecker(t)
	require.NoError(t, server.Set("auth:blacklist:jti-deslogado", "1"))

	revoked, err := checker.IsTokenBlacklisted(context.Background(), "jti-deslogado")
	require.NoError(t, err)
	require.True(t, revoked)

	revoked, err = checker.IsTokenBlacklisted(context.Background(), "jti-valido")
	require.NoError(t, err)
	require.False(t, revoked)
	require.Zero(t, degradedCount(t, "blacklist"), "chave ausente não é falha")
}

func TestGetUserTokenVersion_LeAVersaoDoAuth(t *testing.T) {
	checker, server := newChecker(t)
	require.NoError(t, server.Set("auth:token_version:user-1", "3"))

	version, err := checker.GetUserTokenVersion(context.Background(), "user-1")

	require.NoError(t, err)
	require.Equal(t, 3, version)
}

// Usuário que nunca teve token revogado não tem chave: versão 0 libera qualquer
// token dele, sem contar como degradação.
func TestGetUserTokenVersion_SemChaveEZero(t *testing.T) {
	checker, _ := newChecker(t)

	version, err := checker.GetUserTokenVersion(context.Background(), "user-novo")

	require.NoError(t, err)
	require.Zero(t, version)
	require.Zero(t, degradedCount(t, "token_version"))
}

// Valor que não é inteiro é falha de leitura, não ausência: libera e conta.
func TestGetUserTokenVersion_ValorInvalidoDegrada(t *testing.T) {
	checker, server := newChecker(t)
	require.NoError(t, server.Set("auth:token_version:user-1", "abc"))

	version, err := checker.GetUserTokenVersion(context.Background(), "user-1")

	require.NoError(t, err)
	require.Zero(t, version)
	require.Equal(t, float64(1), degradedCount(t, "token_version"))
}

// Redis fora libera a requisição (o middleware recusaria com erro) e conta na
// métrica que dispara o alerta, para a liberação não virar permanente.
func TestRevocation_RedisForaLiberaEConta(t *testing.T) {
	checker, server := newChecker(t)
	server.SetError("ERR fora do ar")

	revoked, err := checker.IsTokenBlacklisted(context.Background(), "jti-1")
	require.NoError(t, err)
	require.False(t, revoked)

	version, err := checker.GetUserTokenVersion(context.Background(), "user-1")
	require.NoError(t, err)
	require.Zero(t, version)

	require.Equal(t, float64(1), degradedCount(t, "blacklist"))
	require.Equal(t, float64(1), degradedCount(t, "token_version"))
}
