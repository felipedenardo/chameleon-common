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
const establishmentIDKey = "establishment_id"
const establishmentSlugKey = "establishment_slug"

func AuthMiddleware(secretKey string, blacklistTokenChecker security.BlacklistTokenChecker, tokenVersionChecker security.TokenVersionChecker) gin.HandlerFunc {
	issuer := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
	audience := strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))
	leeway := getJWTLeeway()

	var parserOpts []jwt.ParserOption
	parserOpts = append(parserOpts, jwt.WithLeeway(leeway))
	if issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(issuer))
	}
	if audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(audience))
	}

	parser := jwt.NewParser(parserOpts...)

	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			httphelpers.RespondUnauthorized(c, "auth header is empty")
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

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
			if estID, ok := claims["establishment_id"].(string); ok {
				c.Set(establishmentIDKey, estID)
			}
			if estSlug, ok := claims["establishment_slug"].(string); ok {
				c.Set(establishmentSlugKey, estSlug)
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

// GetEstablishmentID retrieves the establishment_id from the context
func GetEstablishmentID(c *gin.Context) (string, bool) {
	estID, exists := c.Get(establishmentIDKey)
	if !exists {
		return "", false
	}
	estIDStr, ok := estID.(string)
	if !ok || estIDStr == "" {
		return "", false
	}
	return estIDStr, true
}

// RequireEstablishmentID retrieves the establishment_id from the context or responds with 401 Unauthorized
func RequireEstablishmentID(c *gin.Context) (string, bool) {
	estIDStr, ok := GetEstablishmentID(c)
	if !ok {
		httphelpers.RespondUnauthorized(c, "Establishment context missing")
		c.Abort()
		return "", false
	}
	return estIDStr, true
}

// GetEstablishmentSlug retrieves the establishment_slug from the context
func GetEstablishmentSlug(c *gin.Context) (string, bool) {
	estSlug, exists := c.Get(establishmentSlugKey)
	if !exists {
		return "", false
	}
	estSlugStr, ok := estSlug.(string)
	if !ok || estSlugStr == "" {
		return "", false
	}
	return estSlugStr, true
}

// RequireEstablishmentSlugContext retrieves the establishment_slug from the context or responds with 401 Unauthorized
func RequireEstablishmentSlugContext(c *gin.Context) (string, bool) {
	estSlugStr, ok := GetEstablishmentSlug(c)
	if !ok {
		httphelpers.RespondUnauthorized(c, "Establishment slug context missing")
		c.Abort()
		return "", false
	}
	return estSlugStr, true
}

// RequireEstablishmentSlug is a middleware that enforces Cross-Tenant boundaries.
// It retrieves the "slug" param from the URL path and compares it exactly with
// the token's establishment_slug. If no slug param is present, allows generic routes.
func RequireEstablishmentSlug() gin.HandlerFunc {
	return func(c *gin.Context) {
		routeSlug := c.Param("slug")
		if routeSlug == "" {
			c.Next()
			return
		}

		// Allow platform administrators to bypass slug validation
		if granted, ok := GetPermissions(c); ok {
			for _, g := range granted {
				if g == "*" || g == "platform.*" {
					c.Next()
					return
				}
			}
		}

		tokenSlug, ok := GetEstablishmentSlug(c)
		if !ok || tokenSlug != routeSlug {
			httphelpers.RespondForbidden(c, "Cross-Tenant access denied")
			c.Abort()
			return
		}

		c.Next()
	}
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

	values, ok := permissions.([]string)
	if !ok || len(values) == 0 {
		return nil, false
	}

	return values, true
}

// RequirePermission validates whether the user has at least one of the allowed permissions.
// Supports exact match and wildcards in the granted permissions (e.g. "*" or "appointments.*").
func RequirePermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		granted, ok := GetPermissions(c)
		if !ok {
			httphelpers.RespondForbidden(c, "Insufficient permission")
			c.Abort()
			return
		}

		for _, allowed := range permissions {
			for _, g := range granted {
				if g == "*" || g == allowed {
					c.Next()
					return
				}
				if strings.HasSuffix(g, ".*") {
					prefix := strings.TrimSuffix(g, ".*")
					if allowed == prefix || strings.HasPrefix(allowed, prefix+".") {
						c.Next()
						return
					}
				}
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
