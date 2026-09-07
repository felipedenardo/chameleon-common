package middleware

import (
	"crypto/rsa"
	"strings"

	"github.com/google/uuid"

	httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
	"github.com/felipedenardo/chameleon-common/pkg/security"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const RawTokenKey = "rawTokenString"
const PermissionsKey = "permissions"
const userIDKey = "userID"
const establishmentIDKey = "establishment_id"

// AuthMiddleware extrai e autoriza as claims de um token de usuário (typ=access):
// confere a assinatura RS256 com a chave pública do auth-api, recusa token
// deslogado (blacklist) e token emitido antes da última revogação
// (token_version).
//
// Os três parâmetros são obrigatórios, e é essa a razão de existir uma função
// só: já houve uma versão que aceitava nil nos verificadores, e seis serviços
// subiram assim — deslogar não deslogava, e trocar a senha não derrubava
// sessão. Nada falhava, só não acontecia. Aqui a única forma de montar a
// autorização é montá-la completa.
//
// Sem conferir a assinatura, a segurança dependeria de nenhum serviço publicar
// porta além do Kong: quem alcançasse a rede interna forjaria qualquer token,
// com qualquer sub e qualquer permissão. Só o auth-api tem a chave privada,
// então nenhum serviço que valida consegue emitir.
func AuthMiddleware(
	userTokenPublicKey *rsa.PublicKey,
	blacklistTokenChecker security.BlacklistTokenChecker,
	tokenVersionChecker security.TokenVersionChecker,
) gin.HandlerFunc {
	// Falha no boot, não em runtime: subir com autorização parcial precisa ser
	// impossível por distração, e um serviço que não alcança a lista de
	// revogação aceitaria token deslogado sem saber.
	if userTokenPublicKey == nil {
		panic("middleware.AuthMiddleware: chave pública do token de usuário é obrigatória")
	}
	if blacklistTokenChecker == nil || tokenVersionChecker == nil {
		panic("middleware.AuthMiddleware: verificadores de revogação são obrigatórios")
	}
	parser := jwt.NewParser()

	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			httphelpers.RespondUnauthorized(c, "auth header is empty")
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims := jwt.MapClaims{}
		// Método fixado em RSA: sem isso, alg=none ou um HS256 usando a própria
		// chave pública como segredo passariam pela verificação.
		_, err := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return userTokenPublicKey, nil
		})

		c.Set(RawTokenKey, tokenString)

		if err != nil {
			httphelpers.RespondUnauthorized(c, "Invalid or expired token")
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
		if estID, ok := claims["establishment_id"].(string); ok {
			c.Set(establishmentIDKey, estID)
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

// RequireEstablishmentContext is a middleware that enforces tenant boundaries.
// Regular users must carry an establishment_id in the token that matches the
// establishment_id in the route. Platform admins act on the tenant in the route.
func RequireEstablishmentContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Identificador de tenant na rota: o establishment_id imutavel. A
		// autorizacao compara SEMPRE por id.
		routeEstablishmentID := c.Param("establishmentID")
		if routeEstablishmentID == "" {
			c.Next()
			return
		}

		if authorizeEstablishmentByID(c, routeEstablishmentID) {
			c.Next()
			return
		}

		httphelpers.RespondForbidden(c, "Cross-Tenant access denied")
		c.Abort()
	}
}

// authorizeEstablishmentByID libera o acesso quando o establishment_id da rota
// bate com o do token. Platform admins atuam sobre o tenant indicado na rota.
func authorizeEstablishmentByID(c *gin.Context, routeEstablishmentID string) bool {
	if hasPlatformAccess(c) {
		c.Set(establishmentIDKey, routeEstablishmentID)
		return true
	}

	tokenID, ok := GetEstablishmentID(c)
	if !ok {
		return false
	}
	if tokenID != routeEstablishmentID {
		return false
	}

	c.Set(establishmentIDKey, tokenID)
	return true
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
