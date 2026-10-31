package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/felipedenardo/chameleon-common/pkg/middleware"
	"github.com/felipedenardo/chameleon-common/pkg/security"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const caller = "chameleon-purchase-api"

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func serviceClaims(sub, typ string, exp time.Time) jwt.MapClaims {
	return jwt.MapClaims{"typ": typ, "sub": sub, "iat": time.Now().Unix(), "exp": exp.Unix()}
}

func signRS256(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)
	return raw
}

// callInternal chama uma rota interna protegida pelo middleware, que confia só
// em chameleon-purchase-api com a chave trusted, e devolve o status e a
// mensagem de recusa.
func callInternal(t *testing.T, trusted *rsa.PublicKey, authorization string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/internal", middleware.ServiceTokenMiddleware(map[string]*rsa.PublicKey{caller: trusted}),
		func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/internal", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		Message string `json:"message"`
	}
	if w.Code != http.StatusNoContent {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	}
	return w.Code, body.Message
}

// O token que o security.SignServiceToken emite é o que o middleware aceita:
// os dois lados do mesmo contrato.
func TestServiceTokenMiddleware_AceitaOTokenDoServicoConfiavel(t *testing.T) {
	key := rsaKey(t)
	raw, err := security.SignServiceToken(key, caller)
	require.NoError(t, err)

	status, _ := callInternal(t, &key.PublicKey, "Bearer "+raw)

	require.Equal(t, http.StatusNoContent, status)
}

func TestServiceTokenMiddleware_Recusa(t *testing.T) {
	key, other := rsaKey(t), rsaKey(t)
	valid := time.Now().Add(time.Minute)

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	// Troca de algoritmo: HS256 com a chave pública (que é pública) como
	// segredo. Passaria se o middleware aceitasse qualquer método.
	hmacWithPublicKey, err := jwt.NewWithClaims(jwt.SigningMethodHS256, serviceClaims(caller, "service", valid)).
		SignedString(publicPEM)
	require.NoError(t, err)
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, serviceClaims(caller, "service", valid)).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	cases := []struct {
		name          string
		authorization string
		message       string
	}{
		{"sem cabeçalho", "", "service token required"},
		{"esquema que não é Bearer", "Basic dXNlcjpzZW5oYQ==", "service token required"},
		{"lixo no lugar do token", "Bearer nao-e-um-jwt", "invalid service token"},
		{"serviço fora do mapa", "Bearer " + signRS256(t, other, serviceClaims("chameleon-intruso-api", "service", valid)), "untrusted service"},
		{"token sem sub", "Bearer " + signRS256(t, key, jwt.MapClaims{"typ": "service", "exp": valid.Unix()}), "untrusted service"},
		{"nome de serviço confiável assinado com outra chave", "Bearer " + signRS256(t, other, serviceClaims(caller, "service", valid)), "invalid service token"},
		{"alg none", "Bearer " + unsigned, "invalid service token"},
		{"HS256 com a chave pública como segredo", "Bearer " + hmacWithPublicKey, "invalid service token"},
		{"vencido", "Bearer " + signRS256(t, key, serviceClaims(caller, "service", time.Now().Add(-time.Minute))), "invalid service token"},
		{"token de usuário com a chave certa", "Bearer " + signRS256(t, key, serviceClaims(caller, "access", valid)), "not a service token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, message := callInternal(t, &key.PublicKey, tc.authorization)

			require.Equal(t, http.StatusUnauthorized, status)
			require.Equal(t, tc.message, message)
		})
	}
}
