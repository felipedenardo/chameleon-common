package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	contaID    = "11111111-1111-1111-1111-111111111111"
	cadastroID = "33333333-3333-3333-3333-333333333333"
)

// customerFromToken passa o token pelo AuthMiddleware de verdade e devolve o
// que RequireCustomerID leu, e o status.
func customerFromToken(t *testing.T, claims jwt.MapClaims) (string, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	base := jwt.MapClaims{
		"jti":           "22222222-2222-2222-2222-222222222222",
		"typ":           "access",
		"exp":           time.Now().Add(time.Hour).Unix(),
		"token_version": float64(0),
	}
	for k, v := range claims {
		base[k] = v
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, base).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	var got string
	r := gin.New()
	r.GET("/x", AuthMiddleware(&key.PublicKey, checkerFalso{}, checkerFalso{}), func(c *gin.Context) {
		id, ok := RequireCustomerID(c)
		if !ok {
			return
		}
		got = id
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)
	return got, w.Code
}

// O cadastro do balcão ligado à conta: o cliente é o cadastro, não o login.
func TestRequireCustomerID_UsaOCadastroDaBarbearia(t *testing.T) {
	got, status := customerFromToken(t, jwt.MapClaims{"sub": contaID, CustomerIDClaim: cadastroID})
	if status != http.StatusOK || got != cadastroID {
		t.Fatalf("esperava o cadastro %s com 200, veio %q com %d", cadastroID, got, status)
	}
}

// Token de antes do claim, ou conta que entrou na barbearia sem cadastro
// anterior: o cliente é o próprio login, como sempre foi.
func TestRequireCustomerID_SemOClaimValeOSub(t *testing.T) {
	for _, claims := range []jwt.MapClaims{
		{"sub": contaID},
		{"sub": contaID, CustomerIDClaim: ""},
		{"sub": contaID, CustomerIDClaim: 42},
	} {
		got, status := customerFromToken(t, claims)
		if status != http.StatusOK || got != contaID {
			t.Fatalf("com %v esperava o sub %s, veio %q com %d", claims, contaID, got, status)
		}
	}
}

// Sem sessão nenhuma, 401: RequireCustomerID não inventa cliente.
func TestRequireCustomerID_SemSessao(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(c *gin.Context) {
		if _, ok := RequireCustomerID(c); ok {
			c.Status(http.StatusOK)
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sem sessão devia dar 401, deu %d", w.Code)
	}
}
