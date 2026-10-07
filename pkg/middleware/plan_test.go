package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// runPlan monta uma rota com as claims do token já lidas e o middleware do
// plano na frente, e devolve o status e o corpo.
func runPlan(t *testing.T, claims jwt.MapClaims, guard gin.HandlerFunc) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(c *gin.Context) {
		if role, ok := claims["role"].(string); ok {
			c.Set("role", role)
		}
		if perms := extractPermissions(claims["permissions"]); len(perms) > 0 {
			c.Set(PermissionsKey, perms)
		}
		setPlanClaims(c, claims)
		c.Next()
	}, guard, func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w.Code, w.Body.String()
}

func TestRequireActivePlan(t *testing.T) {
	cases := []struct {
		name   string
		claims jwt.MapClaims
		status int
		code   string
	}{
		{"em dia passa", jwt.MapClaims{"plan_status": "ok"}, 200, ""},
		{"sem situação passa", jwt.MapClaims{}, 200, ""},
		{"bloqueado é 402", jwt.MapClaims{"plan_status": "blocked"}, 402, CodePlanBlocked},
		{"sem contrato é 402", jwt.MapClaims{"plan_status": "no_contract"}, 402, CodePlanNoContract},
		{"situação desconhecida passa", jwt.MapClaims{"plan_status": "novidade"}, 200, ""},
		{"admin passa bloqueado", jwt.MapClaims{"plan_status": "blocked", "permissions": []interface{}{"platform.*"}}, 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := runPlan(t, tc.claims, RequireActivePlan())
			if status != tc.status {
				t.Fatalf("status = %d, quer %d (%s)", status, tc.status, body)
			}
			if tc.code != "" && !strings.Contains(body, `"code":"`+tc.code+`"`) {
				t.Fatalf("corpo sem o código %s: %s", tc.code, body)
			}
		})
	}
}

// O cliente final não lê nada de cobrança: só que a agenda está indisponível.
func TestRequireActivePlan_ClienteNaoVeCobranca(t *testing.T) {
	_, body := runPlan(t, jwt.MapClaims{"plan_status": "blocked", "role": "customer"}, RequireActivePlan())
	if !strings.Contains(body, "agenda desta unidade está indisponível") {
		t.Fatalf("mensagem para cliente: %s", body)
	}
}

func TestRequireModule(t *testing.T) {
	cases := []struct {
		name   string
		claims jwt.MapClaims
		status int
	}{
		{"módulo no plano passa", jwt.MapClaims{"plan_modules": []interface{}{"inventory", "club"}}, 200},
		{"módulo fora do plano é 403", jwt.MapClaims{"plan_modules": []interface{}{"club"}}, 403},
		{"lista vazia recusa", jwt.MapClaims{"plan_modules": []interface{}{}}, 403},
		{"sem a lista passa", jwt.MapClaims{}, 200},
		{"admin passa", jwt.MapClaims{"plan_modules": []interface{}{}, "permissions": []interface{}{"*"}}, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := runPlan(t, tc.claims, RequireModule("inventory"))
			if status != tc.status {
				t.Fatalf("status = %d, quer %d (%s)", status, tc.status, body)
			}
			if status == 403 && !strings.Contains(body, CodeModuleNotInPlan) {
				t.Fatalf("corpo sem o código: %s", body)
			}
		})
	}
}
