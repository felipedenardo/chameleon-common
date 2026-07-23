package middleware

import (
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ServiceTokenMiddleware valida um token de serviço (claim typ=service, RS256)
// assinado pela chave PRIVADA do serviço chamador — protege rotas internas
// service-to-service. trustedKeys mapeia o subject esperado (ex.:
// "chameleon-auth-api") pra chave pública correspondente; um token cujo `sub`
// não estiver no mapa é rejeitado, mesmo que a assinatura seja válida contra
// outra chave qualquer. Emitido por security.SignServiceToken do lado que chama.
func ServiceTokenMiddleware(trustedKeys map[string]*rsa.PublicKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "service token required"})
			return
		}

		tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

		// Lê o subject sem verificar assinatura ainda — só pra escolher qual
		// chave pública usar na verificação real logo abaixo.
		unverifiedClaims := jwt.MapClaims{}
		if _, _, err := jwt.NewParser().ParseUnverified(tokenString, unverifiedClaims); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "invalid service token"})
			return
		}
		subject, _ := unverifiedClaims["sub"].(string)
		publicKey, trusted := trustedKeys[subject]
		if !trusted {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "untrusted service"})
			return
		}

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return publicKey, nil
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
