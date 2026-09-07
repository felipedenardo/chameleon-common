package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/felipedenardo/chameleon-common/pkg/security"
	"github.com/gin-gonic/gin"
)

type (
	BlacklistCheckerParaTeste = security.BlacklistTokenChecker
	VersionCheckerParaTeste   = security.TokenVersionChecker
)

// Seis serviços subiram com AuthMiddleware(nil, nil) e ninguém percebeu:
// deslogar não deslogava, e trocar a senha não derrubava sessão. Nada disso
// falhava — só não acontecia. Estes testes garantem que a próxima vez seja
// impossível por distração e explícita quando intencional.

type checkerFalso struct {
	blacklisted bool
	version     int
}

func (c checkerFalso) IsTokenBlacklisted(context.Context, string) (bool, error) {
	return c.blacklisted, nil
}
func (c checkerFalso) GetUserTokenVersion(context.Context, string) (int, error) {
	return c.version, nil
}

func TestAuthMiddleware_RecusaVerificadoresNulos(t *testing.T) {
	casos := []struct {
		nome      string
		blacklist BlacklistCheckerParaTeste
		versao    VersionCheckerParaTeste
	}{
		{"ambos nulos", nil, nil},
		{"so blacklist", nil, checkerFalso{}},
		{"so versao", checkerFalso{}, nil},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("subir sem verificador de revogacao precisa quebrar o boot, nao passar batido")
				}
			}()
			AuthMiddleware(caso.blacklist, caso.versao)
		})
	}
}

// A saída explícita continua existindo — só não se alcança por engano.
func TestAuthMiddlewareWithoutRevocationChecks_Monta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mw := AuthMiddlewareWithoutRevocationChecks()
	if mw == nil {
		t.Fatal("o construtor explicito precisa devolver um middleware utilizavel")
	}

	r := gin.New()
	r.GET("/x", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sem Authorization deveria dar 401, deu %d", w.Code)
	}
}
