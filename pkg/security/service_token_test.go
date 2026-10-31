package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

// O token de serviço é o contrato com o ServiceTokenMiddleware de quem recebe:
// RS256, typ=service, o nome de quem chama no sub, e vida de 2 minutos.
func TestSignServiceToken_AssinaRS256ComOContrato(t *testing.T) {
	key := newKey(t)
	before := time.Now().Unix()

	raw, err := SignServiceToken(key, "chameleon-purchase-api")
	require.NoError(t, err)

	token, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
	require.NoError(t, err)
	require.Equal(t, jwt.SigningMethodRS256.Alg(), token.Method.Alg())
	claims := token.Claims.(jwt.MapClaims)
	require.Equal(t, "service", claims["typ"])
	require.Equal(t, "chameleon-purchase-api", claims["sub"])
	iat, exp := int64(claims["iat"].(float64)), int64(claims["exp"].(float64))
	require.GreaterOrEqual(t, iat, before)
	require.Equal(t, int64(120), exp-iat, "vida de 2 minutos")
}

// Assinado com a chave de um serviço, não vale contra a de outro: é o que
// impede um serviço comprometido de falar em nome de outro.
func TestSignServiceToken_NaoValeContraAChaveDeOutroServico(t *testing.T) {
	raw, err := SignServiceToken(newKey(t), "chameleon-sales-api")
	require.NoError(t, err)

	other := newKey(t)
	_, err = jwt.Parse(raw, func(*jwt.Token) (any, error) { return &other.PublicKey, nil })

	require.ErrorIs(t, err, jwt.ErrTokenSignatureInvalid)
}

func TestSignServiceToken_SemChaveEErro(t *testing.T) {
	_, err := SignServiceToken(nil, "chameleon-sales-api")

	require.Error(t, err)
}

func privatePEM(t *testing.T, key *rsa.PrivateKey, pkcs8 bool) []byte {
	t.Helper()
	if !pkcs8 {
		return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func publicPEM(t *testing.T, key *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// O openssl gera PKCS#1 ou PKCS#8 conforme a versão; as duas formas valem.
func TestParseRSAPrivateKeyPEM_AceitaPKCS1EPKCS8(t *testing.T) {
	key := newKey(t)

	for name, pkcs8 := range map[string]bool{"PKCS#1": false, "PKCS#8": true} {
		t.Run(name, func(t *testing.T) {
			parsed, err := ParseRSAPrivateKeyPEM(privatePEM(t, key, pkcs8))

			require.NoError(t, err)
			require.True(t, key.Equal(parsed))
		})
	}
}

func TestParseRSAPublicKeyPEM_LeAChavePublica(t *testing.T) {
	key := newKey(t)

	parsed, err := ParseRSAPublicKeyPEM(publicPEM(t, &key.PublicKey))

	require.NoError(t, err)
	require.True(t, key.PublicKey.Equal(parsed))
}

func TestParseRSAKeyPEM_ConteudoInvalidoEErro(t *testing.T) {
	_, err := ParseRSAPrivateKeyPEM([]byte("não é uma chave"))
	require.Error(t, err)

	_, err = ParseRSAPublicKeyPEM([]byte("não é uma chave"))
	require.Error(t, err)

	// Trocar as chaves de lugar no deploy é o erro mais provável: a pública no
	// lugar da privada não pode passar.
	key := newKey(t)
	_, err = ParseRSAPrivateKeyPEM(publicPEM(t, &key.PublicKey))
	require.Error(t, err)
}

func TestLoadRSAKeyFile_LeOArquivoPEM(t *testing.T) {
	key := newKey(t)
	dir := t.TempDir()
	privatePath, publicPath := filepath.Join(dir, "private.pem"), filepath.Join(dir, "public.pem")
	require.NoError(t, os.WriteFile(privatePath, privatePEM(t, key, false), 0o600))
	require.NoError(t, os.WriteFile(publicPath, publicPEM(t, &key.PublicKey), 0o600))

	private, err := LoadRSAPrivateKeyFile(privatePath)
	require.NoError(t, err)
	require.True(t, key.Equal(private))

	public, err := LoadRSAPublicKeyFile(publicPath)
	require.NoError(t, err)
	require.True(t, key.PublicKey.Equal(public))
}

// Arquivo ausente diz qual caminho faltou: é o que aparece no log quando o
// serviço não sobe por falta de chave.
func TestLoadRSAKeyFile_ArquivoAusenteDizOCaminho(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "sumiu.pem")

	_, err := LoadRSAPrivateKeyFile(missing)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Contains(t, err.Error(), missing)

	_, err = LoadRSAPublicKeyFile(missing)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Contains(t, err.Error(), missing)
}
