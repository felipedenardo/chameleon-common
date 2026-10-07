package middleware

import (
	"net/http"
	"slices"

	"github.com/felipedenardo/chameleon-common/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Situação do plano da unidade, como o auth-api a põe no token ao emitir uma
// sessão com unidade (claims plan_status e plan_modules, vindas do
// billing-api). Os serviços só leem: nenhum dos middlewares abaixo faz I/O.
const (
	PlanStatusKey  = "plan_status"
	PlanModulesKey = "plan_modules"

	// PlanStatusOK cobre teste, em dia e atrasado dentro da carência.
	PlanStatusOK = "ok"
	// PlanStatusBlocked é o atraso depois da carência: bloqueio total.
	PlanStatusBlocked = "blocked"
	// PlanStatusNoContract é a unidade cujo dono ainda não contratou (ou não
	// configurou o pagamento).
	PlanStatusNoContract = "no_contract"
)

// Códigos de erro que o web reconhece para mandar a pessoa à tela certa.
const (
	CodePlanBlocked     = "plan_blocked"
	CodePlanNoContract  = "plan_no_contract"
	CodeModuleNotInPlan = "module_not_in_plan"
)

// planResponse é o corpo padrão com o código que o web usa para decidir a
// tela ("Plano suspenso", "Escolha seu plano", "Não incluído no seu plano").
type planResponse struct {
	response.Standard
	Code string `json:"code"`
}

// setPlanClaims guarda a situação do plano que veio no token. Claim ausente
// não grava nada: é token sem unidade, do admin, de antes do billing, ou
// emitido com o billing fora do ar — e token sem situação libera.
func setPlanClaims(c *gin.Context, claims jwt.MapClaims) {
	if status, ok := claims["plan_status"].(string); ok && status != "" {
		c.Set(PlanStatusKey, status)
	}
	if raw, ok := claims["plan_modules"]; ok {
		modules := extractStringSlice(raw)
		if modules == nil {
			modules = []string{}
		}
		c.Set(PlanModulesKey, modules)
	}
}

// GetPlanStatus devolve a situação do plano do token, se veio.
func GetPlanStatus(c *gin.Context) (string, bool) {
	value, ok := c.Get(PlanStatusKey)
	if !ok {
		return "", false
	}
	status, ok := value.(string)
	return status, ok && status != ""
}

// GetPlanModules devolve os módulos liberados do token, se vieram.
func GetPlanModules(c *gin.Context) ([]string, bool) {
	value, ok := c.Get(PlanModulesKey)
	if !ok {
		return nil, false
	}
	modules, ok := value.([]string)
	return modules, ok
}

// RequireActivePlan recusa a unidade bloqueada ou sem contrato, com 402. Vai
// no grupo tenant de cada serviço, logo depois do RequireEstablishmentContext.
//
// Passam sempre: o admin da plataforma, e o token sem situação (ver
// setPlanClaims). Quem é cliente final lê "agenda indisponível", sem menção a
// cobrança.
func RequireActivePlan() gin.HandlerFunc {
	return func(c *gin.Context) {
		if hasPlatformAccess(c) {
			c.Next()
			return
		}
		status, ok := GetPlanStatus(c)
		if !ok || status == PlanStatusOK {
			c.Next()
			return
		}

		switch status {
		case PlanStatusBlocked:
			abortPlan(c, http.StatusPaymentRequired, CodePlanBlocked, blockedMessage(c))
		case PlanStatusNoContract:
			abortPlan(c, http.StatusPaymentRequired, CodePlanNoContract, noContractMessage(c))
		default:
			// Situação que este serviço ainda não conhece: libera, como o
			// token sem situação. Bloquear por desconhecimento derrubaria
			// clientes pagantes a cada valor novo.
			c.Next()
		}
	}
}

// RequireModule recusa, com 403, a rota de um módulo fora do plano. Passam o
// admin da plataforma e o token sem a lista de módulos.
func RequireModule(module string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hasPlatformAccess(c) {
			c.Next()
			return
		}
		modules, ok := GetPlanModules(c)
		if !ok || slices.Contains(modules, module) {
			c.Next()
			return
		}
		abortPlan(c, http.StatusForbidden, CodeModuleNotInPlan, "Este recurso não está incluído no plano da unidade.")
	}
}

func abortPlan(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, planResponse{
		Standard: response.Standard{Status: "fail", Message: message},
		Code:     code,
	})
}

func isCustomer(c *gin.Context) bool {
	role, _ := c.Get("role")
	return role == "customer"
}

func blockedMessage(c *gin.Context) string {
	if isCustomer(c) {
		return "A agenda desta unidade está indisponível no momento."
	}
	if role, _ := c.Get("role"); role == "owner" {
		return "O plano desta unidade está suspenso por falta de pagamento. Regularize em Meu plano."
	}
	return "O acesso a esta unidade está suspenso. Fale com o responsável pela unidade."
}

func noContractMessage(c *gin.Context) string {
	if isCustomer(c) {
		return "A agenda desta unidade está indisponível no momento."
	}
	return "Esta unidade ainda não tem um plano ativo."
}
