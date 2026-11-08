package middleware

import (
	"strings"

	httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// CustomerIDClaim é o claim da sessão de cliente numa unidade: o id do cadastro
// dele NAQUELA barbearia, que é o customer_id que agenda, clube, vendas e
// pagamento anotam. Diferente do sub, que é o login: a conta tem um cadastro
// por barbearia, e o da recepção nasce antes de a pessoa ter conta.
const CustomerIDClaim = "customer_id"

const customerIDKey = "customerID"

func setCustomerClaim(c *gin.Context, claims jwt.MapClaims) {
	if customerID, ok := claims[CustomerIDClaim].(string); ok && strings.TrimSpace(customerID) != "" {
		c.Set(customerIDKey, customerID)
	}
}

// GetCustomerID devolve o cadastro de cliente da sessão. Sem o claim (token de
// antes dele, ou de quem não é cliente da unidade), vale o sub: era assim que
// toda rota do próprio cliente lia o customer_id, e é o mesmo id quando a conta
// entrou na barbearia sem cadastro anterior.
func GetCustomerID(c *gin.Context) (string, bool) {
	if value, exists := c.Get(customerIDKey); exists {
		if customerID, ok := value.(string); ok && customerID != "" {
			return customerID, true
		}
	}
	return GetUserID(c)
}

// RequireCustomerID é o GetCustomerID das rotas do próprio cliente ("meus
// agendamentos", "meu clube", "minha cobrança"): sem sessão, 401.
func RequireCustomerID(c *gin.Context) (string, bool) {
	customerID, ok := GetCustomerID(c)
	if !ok {
		httphelpers.RespondUnauthorized(c, "Sua sessão expirou. Entre de novo.")
		c.Abort()
		return "", false
	}
	return customerID, true
}
