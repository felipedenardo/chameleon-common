package security

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SignServiceToken minta um token curto (claim typ=service) assinado com o
// secret compartilhado entre os serviços — a credencial pra chamada
// service-to-service (nunca um usuário consegue forjar, pois não tem o
// secret). subject identifica o serviço chamador (ex.: nome do serviço).
// Validado por middleware.ServiceTokenMiddleware do lado que recebe.
func SignServiceToken(secret, subject string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"typ": "service",
		"sub": subject,
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}
