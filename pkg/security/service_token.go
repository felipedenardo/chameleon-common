package security

import (
	"crypto/rsa"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SignServiceToken minta um token curto (claim typ=service) assinado com a
// chave PRIVADA RSA do serviço chamador — a credencial pra chamada
// service-to-service. subject identifica o serviço chamador (ex.: nome do
// serviço), e é o que o lado que recebe usa pra escolher a chave pública
// correspondente. Validado por middleware.ServiceTokenMiddleware.
func SignServiceToken(privateKey *rsa.PrivateKey, subject string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"typ": "service",
		"sub": subject,
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(privateKey)
}

// ParseRSAPrivateKeyPEM decodifica uma chave privada RSA em PEM (PKCS#1 ou
// PKCS#8) pro tipo que SignServiceToken espera.
func ParseRSAPrivateKeyPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	return jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
}

// ParseRSAPublicKeyPEM decodifica uma chave pública RSA em PEM — usada por
// quem recebe chamada de serviço, pra validar a assinatura.
func ParseRSAPublicKeyPEM(pemBytes []byte) (*rsa.PublicKey, error) {
	return jwt.ParseRSAPublicKeyFromPEM(pemBytes)
}

// LoadRSAPrivateKeyFile lê e decodifica a chave privada RSA de um arquivo PEM.
func LoadRSAPrivateKeyFile(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key %s: %w", path, err)
	}
	return ParseRSAPrivateKeyPEM(data)
}

// LoadRSAPublicKeyFile lê e decodifica a chave pública RSA de um arquivo PEM.
func LoadRSAPublicKeyFile(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key %s: %w", path, err)
	}
	return ParseRSAPublicKeyPEM(data)
}
