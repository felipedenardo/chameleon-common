package middleware

import (
	"strings"

	httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
	"github.com/felipedenardo/chameleon-common/pkg/security"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const RawTokenKey = "rawTokenString"

func AuthMiddleware(secretKey string, blacklistTokenChecker security.BlacklistTokenChecker, tokenVersionChecker security.TokenVersionChecker) gin.HandlerFunc {
	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			httphelpers.RespondUnauthorized(c, "auth header is empty")
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		tokenUnverified, _, _ := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
		if claims, ok := tokenUnverified.Claims.(jwt.MapClaims); ok {
			jti, _ := claims["jti"].(string)

			isBlacklisted, err := blacklistTokenChecker.IsTokenBlacklisted(c.Request.Context(), jti)

			if err != nil || isBlacklisted {
				httphelpers.RespondUnauthorized(c, "Token revogado ou erro de segurança.")
				c.Abort()
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
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			userID, okUserID := claims["sub"].(string)
			if okUserID {
				c.Set("userID", userID)
			}
			if role, ok := claims["role"].(string); ok {
				c.Set("role", role)
			}

			if tokenVersionChecker != nil && okUserID {
				tokenVersionClaim := 0
				if tv, ok := claims["token_version"].(float64); ok {
					tokenVersionClaim = int(tv)
				} else if tv, ok := claims["token_version"].(int); ok {
					tokenVersionClaim = tv // In case the JWT library unmarshals as int
				}

				currentVersion, err := tokenVersionChecker.GetUserTokenVersion(c.Request.Context(), userID)
				if err != nil {
					httphelpers.RespondUnauthorized(c, "Error verifying token version")
					c.Abort()
					return
				}

				if tokenVersionClaim < currentVersion {
					httphelpers.RespondUnauthorized(c, "Token version mismatch (revoked)")
					c.Abort()
					return
				}
			}
		}

		c.Next()
	}
}
