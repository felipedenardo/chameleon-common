package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ServiceTokenMiddleware valida um token de serviço (claim typ=service) assinado
// com o secret compartilhado entre os serviços — protege rotas internas
// chamadas service-to-service (ex.: a perninha de login consultando outro
// serviço, ou um serviço provisionando algo em outro). Tokens de usuário usam
// typ=access, e só quem tem o secret (os serviços) consegue assinar um
// typ=service — um usuário não forja essa credencial. Emitido por
// security.SignServiceToken do lado que chama.
func ServiceTokenMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "service token required"})
			return
		}

		tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "invalid service token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "invalid service token"})
			return
		}
		if typ, _ := claims["typ"].(string); typ != "service" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "not a service token"})
			return
		}

		c.Next()
	}
}
