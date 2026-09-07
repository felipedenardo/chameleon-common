package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestAuthMiddleware_RecusaConstrucaoIncompleta(t *testing.T) {
	chave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		nome      string
		chave     *rsa.PublicKey
		blacklist BlacklistCheckerParaTeste
		versao    VersionCheckerParaTeste
	}{
		{"tudo nulo", nil, nil, nil},
		{"sem chave publica", nil, checkerFalso{}, checkerFalso{}},
		{"sem blacklist", &chave.PublicKey, nil, checkerFalso{}},
		{"sem versao", &chave.PublicKey, checkerFalso{}, nil},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("montar autorizacao incompleta precisa quebrar o boot, nao passar batido")
				}
			}()
			AuthMiddleware(caso.chave, caso.blacklist, caso.versao)
		})
	}
}

// A variante que verifica assinatura existe porque a que nao verifica so e
// segura enquanto nenhum servico publicar porta alem do Kong. Quem alcancasse
// a rede interna forjaria token com qualquer sub e qualquer permissao.

func TestAuthMiddleware_RecusaTokenForjado(t *testing.T) {
	gin.SetMode(gin.TestMode)

	doServico, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	doAtacante, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	forjado := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "11111111-1111-1111-1111-111111111111",
		"jti": "22222222-2222-2222-2222-222222222222",
		"typ": "access",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	assinado, err := forjado.SignedString(doAtacante)
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/x", AuthMiddleware(&doServico.PublicKey, checkerFalso{}, checkerFalso{}),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+assinado)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("token assinado por chave desconhecida devia dar 401, deu %d", w.Code)
	}
}
