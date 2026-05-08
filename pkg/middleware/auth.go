package middleware

import (
	"context"
	"github.com/google/uuid"
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

// EstablishmentResolver resolves the active establishment ID for slug-based routes.
// The concrete implementation lives in the consumer microservice.
type EstablishmentResolver interface {
	ResolveEstablishmentID(ctx context.Context, slug string) (string, error)
}

// EstablishmentResolverFunc adapts a function to EstablishmentResolver.
type EstablishmentResolverFunc func(ctx context.Context, slug string) (string, error)

func (f EstablishmentResolverFunc) ResolveEstablishmentID(ctx context.Context, slug string) (string, error) {
	return f(ctx, slug)
}

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

func GetEstablishmentUUID(c *gin.Context) (uuid.UUID, bool) {
	estID, ok := GetEstablishmentID(c)
	if !ok {
		return uuid.Nil, false
	}

	id, err := uuid.Parse(estID)
	if err != nil {
		return uuid.Nil, false
	}

	return id, true
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

func RequireUUIDParam(c *gin.Context, paramName string) (string, bool) {
	value := c.Param(paramName)
	if _, err := uuid.Parse(value); err != nil {
		httphelpers.RespondParamError(c, paramName, "invalid UUID")
		c.Abort()
		return "", false
	}

	return value, true
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

// RequireEstablishmentContext is a middleware that enforces tenant boundaries.
// Regular users must carry an establishment_id in the token and match the route slug.
// Platform admins may resolve the route slug dynamically when a resolver is provided.
func RequireEstablishmentContext() gin.HandlerFunc {
	return requireEstablishmentContext(nil)
}

// RequireEstablishmentContextWithResolver extends tenant validation with slug resolution.
func RequireEstablishmentContextWithResolver(resolver EstablishmentResolver) gin.HandlerFunc {
	return requireEstablishmentContext(resolver)
}

// RequireEstablishmentContextFunc is a convenience helper for function-based resolvers.
func RequireEstablishmentContextFunc(resolver func(ctx context.Context, slug string) (string, error)) gin.HandlerFunc {
	if resolver == nil {
		return requireEstablishmentContext(nil)
	}
	return requireEstablishmentContext(EstablishmentResolverFunc(resolver))
}

// RequireEstablishmentSlug keeps the old name as a compatibility alias.
func RequireEstablishmentSlug() gin.HandlerFunc {
	return RequireEstablishmentContext()
}

// RequireEstablishmentSlugWithResolver keeps the old name as a compatibility alias.
func RequireEstablishmentSlugWithResolver(resolver EstablishmentResolver) gin.HandlerFunc {
	return RequireEstablishmentContextWithResolver(resolver)
}

// RequireEstablishmentSlugFunc keeps the old name as a compatibility alias.
func RequireEstablishmentSlugFunc(resolver func(ctx context.Context, slug string) (string, error)) gin.HandlerFunc {
	return RequireEstablishmentContextFunc(resolver)
}

func requireEstablishmentContext(resolver EstablishmentResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		routeSlug := c.Param("slug")
		if routeSlug == "" {
			c.Next()
			return
		}

		allowed, err := resolveEstablishmentContextFromSlug(c, routeSlug, resolver)
		if err != nil {
			httphelpers.RespondInternalError(c, err)
			c.Abort()
			return
		}

		if allowed {
			c.Next()
			return
		}

		httphelpers.RespondForbidden(c, "Cross-Tenant access denied")
		c.Abort()
	}
}

func resolveEstablishmentContextFromSlug(c *gin.Context, routeSlug string, resolver EstablishmentResolver) (bool, error) {
	if hasPlatformAccess(c) {
		c.Set(establishmentSlugKey, routeSlug)
		if resolver == nil {
			_, ok := GetEstablishmentID(c)
			return ok, nil
		}

		establishmentID, err := resolver.ResolveEstablishmentID(c.Request.Context(), routeSlug)
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(establishmentID) == "" {
			return false, nil
		}

		c.Set(establishmentIDKey, establishmentID)
		return true, nil
	}

	tokenID, hasTokenID := GetEstablishmentID(c)
	if !hasTokenID {
		return false, nil
	}

	if tokenSlug, ok := GetEstablishmentSlug(c); ok && tokenSlug == routeSlug {
		c.Set(establishmentSlugKey, routeSlug)
		return true, nil
	}

	if resolver == nil {
		return false, nil
	}

	establishmentID, err := resolver.ResolveEstablishmentID(c.Request.Context(), routeSlug)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(establishmentID) == "" {
		return false, nil
	}
	if establishmentID != tokenID {
		return false, nil
	}

	c.Set(establishmentIDKey, establishmentID)
	c.Set(establishmentSlugKey, routeSlug)
	return true, nil
}

func hasPlatformAccess(c *gin.Context) bool {
	granted, ok := GetPermissions(c)
	if !ok {
		return false
	}

	for _, permission := range granted {
		if permission == "*" || permission == "platform.*" {
			return true
		}
	}

	return false
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

func extractStringSlice(raw interface{}) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []interface{}:
		slice := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if ok && strings.TrimSpace(text) != "" {
				slice = append(slice, text)
			}
		}
		return slice
	default:
		return nil
	}
}

func extractPermissions(raw interface{}) []string {
	return extractStringSlice(raw)
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
