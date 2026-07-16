package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Rede de seguranca da fronteira de tenant.
//
// A autorizacao passou a comparar o establishment_id imutavel do token com o
// identificador de tenant que chega na rota. Estes testes travam os invariantes
// de seguranca (mesmo tenant permite, cross-tenant nega 403, ausencia de
// contexto nega) que valem independentemente de a rota ainda carregar um slug
// (fail-closed) ou ja carregar o id.

func init() {
	gin.SetMode(gin.TestMode)
}

// tenantContext simula o que o AuthMiddleware injeta no contexto a partir das
// claims do JWT.
type tenantContext struct {
	establishmentID string
	permissions     []string
}

func (tc tenantContext) inject() gin.HandlerFunc {
	return func(c *gin.Context) {
		if tc.establishmentID != "" {
			c.Set(establishmentIDKey, tc.establishmentID)
		}
		if len(tc.permissions) > 0 {
			c.Set(PermissionsKey, tc.permissions)
		}
		c.Next()
	}
}

// runBoundary monta a rota /:establishmentID/resource protegida pelo middleware
// sob teste e devolve o status HTTP resultante para o id de tenant informado na
// rota.
func runBoundary(mw gin.HandlerFunc, tc tenantContext, routeEstablishmentID string) int {
	r := gin.New()
	r.GET("/api/v1/:establishmentID/resource", tc.inject(), mw, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+routeEstablishmentID+"/resource", nil)
	r.ServeHTTP(w, req)
	return w.Code
}

const (
	idA = "11111111-1111-1111-1111-111111111111"
	idB = "22222222-2222-2222-2222-222222222222"
)

// Servicos downstream usam o middleware com resolver nil.
func TestTenantBoundary_AutorizacaoPorID(t *testing.T) {
	cases := []struct {
		name       string
		tc         tenantContext
		routeEstID string
		want       int
	}{
		{
			name:       "mesmo tenant (id do token == id da rota) permite",
			tc:         tenantContext{establishmentID: idA},
			routeEstID: idA,
			want:       http.StatusOK,
		},
		{
			name:       "cross-tenant (ids diferentes) nega com 403",
			tc:         tenantContext{establishmentID: idA},
			routeEstID: idB,
			want:       http.StatusForbidden,
		},
		{
			name:       "sem establishment no token nega",
			tc:         tenantContext{},
			routeEstID: idA,
			want:       http.StatusForbidden,
		},
		{
			name:       "rota ainda com slug nao bate com id do token e nega (fail-closed)",
			tc:         tenantContext{establishmentID: idA},
			routeEstID: "barbearia-a",
			want:       http.StatusForbidden,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := runBoundary(RequireEstablishmentContext(), tt.tc, tt.routeEstID)
			if got != tt.want {
				t.Fatalf("status = %d, esperado %d", got, tt.want)
			}
		})
	}
}

// Rota sem :slug (ex.: health) passa direto.
func TestTenantBoundary_SemParamPassaDireto(t *testing.T) {
	r := gin.New()
	r.GET("/api/v1/health", RequireEstablishmentContext(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", w.Code, http.StatusOK)
	}
}

// Platform admin (permissao "*" ou "platform.*") atua sobre o tenant da rota.
func TestTenantBoundary_PlatformAdmin(t *testing.T) {
	t.Run("admin acessa qualquer tenant da rota", func(t *testing.T) {
		got := runBoundary(RequireEstablishmentContext(),
			tenantContext{permissions: []string{"*"}}, idB)
		if got != http.StatusOK {
			t.Fatalf("status = %d, esperado %d", got, http.StatusOK)
		}
	})

	t.Run("admin com platform.* tambem acessa", func(t *testing.T) {
		got := runBoundary(RequireEstablishmentContext(),
			tenantContext{permissions: []string{"platform.*"}}, idA)
		if got != http.StatusOK {
			t.Fatalf("status = %d, esperado %d", got, http.StatusOK)
		}
	})
}
