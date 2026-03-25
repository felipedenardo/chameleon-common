package middleware

import (
	"os"
	"strconv"
	"strings"
	"time"

	httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
	"github.com/felipedenardo/chameleon-common/pkg/security"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const RawTokenKey = "rawTokenString"
const PermissionsKey = "permissions"
const userIDKey = "userID"

func AuthMiddleware(secretKey string, blacklistTokenChecker security.BlacklistTokenChecker, tokenVersionChecker security.TokenVersionChecker) gin.HandlerFunc {
	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			httphelpers.RespondUnauthorized(c, "auth header is empty")
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		parser := jwt.NewParser(jwt.WithLeeway(getJWTLeeway()))
		token, err := parser.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
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
			if !validateIssuer(claims) || !validateAudience(claims) {
				httphelpers.RespondUnauthorized(c, "Invalid token issuer or audience")
				c.Abort()
				return
			}

			if !validateTokenType(claims) {
				httphelpers.RespondUnauthorized(c, "Invalid token type")
				c.Abort()
				return
			}

			userID, okUserID := claims["sub"].(string)
			if !okUserID || strings.TrimSpace(userID) == "" {
				httphelpers.RespondUnauthorized(c, "Missing subject")
				c.Abort()
				return
			}
			c.Set(userIDKey, userID)
			if role, ok := claims["role"].(string); ok {
				c.Set("role", role)
			}
			if permissions := extractPermissions(claims["permissions"]); len(permissions) > 0 {
				c.Set(PermissionsKey, permissions)
			}

			jti, okJTI := claims["jti"].(string)
			if !okJTI || strings.TrimSpace(jti) == "" {
				httphelpers.RespondUnauthorized(c, "Missing token identifier")
				c.Abort()
				return
			}

			if blacklistTokenChecker != nil {
				isBlacklisted, err := blacklistTokenChecker.IsTokenBlacklisted(c.Request.Context(), jti)
				if err != nil || isBlacklisted {
					httphelpers.RespondUnauthorized(c, "Token revogado ou erro de segurança.")
					c.Abort()
					return
				}
			}

			if tokenVersionChecker != nil {
				if _, ok := claims["token_version"]; !ok {
					httphelpers.RespondUnauthorized(c, "Missing token version")
					c.Abort()
					return
				}

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

func validateTokenType(claims jwt.MapClaims) bool {
	typ, ok := claims["typ"].(string)
	if !ok {
		return false
	}
	return typ == "access"
}

func validateIssuer(claims jwt.MapClaims) bool {
	issuer := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
	iss, ok := claims["iss"].(string)
	if !ok {
		return false
	}
	return iss == issuer
}

func validateAudience(claims jwt.MapClaims) bool {
	audience := strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))

	switch aud := claims["aud"].(type) {
	case string:
		return aud == audience
	case []string:
		for _, a := range aud {
			if a == audience {
				return true
			}
		}
	case []interface{}:
		for _, a := range aud {
			if s, ok := a.(string); ok && s == audience {
				return true
			}
		}
	}

	return false
}

// GetUserID retrieves the userID from the context
func GetUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get(userIDKey)
	if !exists {
		return "", false
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		return "", false
	}
	return userIDStr, true
}

// GetRawToken retrieves the rawTokenString from the context
func GetRawToken(c *gin.Context) (string, bool) {
	token, exists := c.Get(RawTokenKey)
	if !exists {
		return "", false
	}
	tokenStr, ok := token.(string)
	if !ok || tokenStr == "" {
		return "", false
	}
	return tokenStr, ok
}

// RequireUserID retrieves the userID from the context or responds with 401 Unauthorized
func RequireUserID(c *gin.Context) (string, bool) {
	userIDStr, ok := GetUserID(c)
	if !ok {
		httphelpers.RespondUnauthorized(c, "Authentication context missing")
		c.Abort()
		return "", false
	}
	return userIDStr, true
}

// RequireRawToken retrieves the raw token from the context or responds with 401 Unauthorized
func RequireRawToken(c *gin.Context) (string, bool) {
	tokenStr, ok := GetRawToken(c)
	if !ok {
		httphelpers.RespondUnauthorized(c, "Authentication context missing")
		c.Abort()
		return "", false
	}
	return tokenStr, true
}

// RequireRole validates whether the user's role matches one of the allowed roles.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := c.Get("role")
		if !ok {
			httphelpers.RespondUnauthorized(c, "Authentication context missing")
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || roleStr == "" {
			httphelpers.RespondUnauthorized(c, "Authentication context missing")
			c.Abort()
			return
		}

		for _, allowed := range roles {
			if roleStr == allowed {
				c.Next()
				return
			}
		}

		httphelpers.RespondForbidden(c, "Insufficient role")
		c.Abort()
	}
}

// GetPermissions retrieves permissions from the context.
func GetPermissions(c *gin.Context) ([]string, bool) {
	permissions, exists := c.Get(PermissionsKey)
	if !exists {
		return nil, false
	}

	switch values := permissions.(type) {
	case []string:
		if len(values) == 0 {
			return nil, false
		}
		return values, true
	case []interface{}:
		normalized := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if ok && strings.TrimSpace(text) != "" {
				normalized = append(normalized, text)
			}
		}
		if len(normalized) == 0 {
			return nil, false
		}
		return normalized, true
	default:
		return nil, false
	}
}

// RequirePermission validates whether the user has at least one of the allowed permissions.
func RequirePermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		granted, ok := GetPermissions(c)
		if !ok {
			httphelpers.RespondForbidden(c, "Insufficient permission")
			c.Abort()
			return
		}

		permissionSet := make(map[string]struct{}, len(granted))
		for _, permission := range granted {
			permissionSet[permission] = struct{}{}
		}

		for _, allowed := range permissions {
			if _, exists := permissionSet[allowed]; exists {
				c.Next()
				return
			}
		}

		httphelpers.RespondForbidden(c, "Insufficient permission")
		c.Abort()
	}
}

func extractPermissions(raw interface{}) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []interface{}:
		permissions := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if ok && strings.TrimSpace(text) != "" {
				permissions = append(permissions, text)
			}
		}
		return permissions
	default:
		return nil
	}
}

func getJWTLeeway() time.Duration {
	value := strings.TrimSpace(os.Getenv("JWT_LEEWAY_SECONDS"))
	if value == "" {
		return 0
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
