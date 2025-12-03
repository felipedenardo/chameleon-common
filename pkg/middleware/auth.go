package middleware

import (
	httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
	"github.com/felipedenardo/chameleon-common/pkg/security"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"strings"
)

const RawTokenKey = "rawTokenString"

func AuthMiddleware(secretKey string, blacklistTokenChecker security.BlacklistTokenChecker) gin.HandlerFunc {
	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			httphelpers.RespondUnauthorized(c, "auth header is empty")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		tokenUnverified, _, _ := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
		if claims, ok := tokenUnverified.Claims.(jwt.MapClaims); ok {
			jti, _ := claims["jti"].(string)

			isBlacklisted, err := blacklistTokenChecker.IsTokenBlacklisted(c.Request.Context(), jti)

			if err != nil || isBlacklisted {
				httphelpers.RespondUnauthorized(c, "Token revogado ou erro de segurança.")
				return
			}
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secretKey), nil
		})

		c.Set(RawTokenKey, tokenString)

		if err != nil || !token.Valid {
			httphelpers.RespondUnauthorized(c, "Invalid or expired token")
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if userID, ok := claims["sub"].(string); ok {
				c.Set("userID", userID)
			}
			if role, ok := claims["role"].(string); ok {
				c.Set("role", role)
			}
		}

		c.Next()
	}
}
