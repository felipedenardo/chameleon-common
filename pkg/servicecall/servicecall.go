// Package servicecall é o transporte das chamadas de um serviço a outro: assina
// o token de serviço, aplica timeout e passa tudo por um circuit breaker.
//
// Cada client de infra guarda o próprio Caller, com breaker nomeado por
// destino: um serviço fora do ar não abre o circuito de outro.
package servicecall

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/felipedenardo/chameleon-common/pkg/circuitbreaker"
	"github.com/felipedenardo/chameleon-common/pkg/security"
)

// ErrUnavailable é o destino que não respondeu: falha de transporte, timeout,
// 5xx ou circuito aberto. Tentar de novo mais tarde pode resolver. Um 4xx não
// entra aqui — é resposta do destino, devolvida com o status para quem chamou
// decidir.
var ErrUnavailable = errors.New("service unavailable")

// Caller fala com um destino.
type Caller struct {
	name       string
	subject    string
	baseURL    string
	privateKey *rsa.PrivateKey
	httpClient *http.Client
	cb         *circuitbreaker.CircuitBreaker
}

// New monta o Caller de um destino. name nomeia o breaker e os erros;
// subject é o "sub" do token, o serviço que chama, e precisa bater com a chave
// pública que o destino tem no seu mapa de confiança. O breaker segue o padrão
// do ecossistema: cinco falhas seguidas abrem o circuito, e um teste por vez
// decide reabrir.
func New(name, subject, baseURL string, privateKey *rsa.PrivateKey, timeout time.Duration) *Caller {
	return &Caller{
		name:       name,
		subject:    subject,
		baseURL:    strings.TrimRight(baseURL, "/"),
		privateKey: privateKey,
		httpClient: &http.Client{Timeout: timeout},
		cb: circuitbreaker.New(circuitbreaker.Settings{
			Name:             name,
			MaxHalfOpenReqs:  1,
			Interval:         60 * time.Second,
			Timeout:          30 * time.Second,
			FailureThreshold: 5,
		}),
	}
}

type outcome struct {
	status int
	body   []byte
	err    error
}

// Do envia a chamada e devolve o status e o corpo.
//
// Só falha de transporte e 5xx contam como falha da dependência, e voltam como
// ErrUnavailable. 4xx é resposta legítima — "essa pessoa não é de lá" —, e
// devolvê-lo como erro abriria o circuito por uso normal. Chamada abandonada
// por quem chamou (o contexto cancelado quando outra leitura em paralelo já
// falhou) também não é culpa do destino, e volta com o erro do contexto.
func (c *Caller) Do(ctx context.Context, method, path string, payload any) (int, []byte, error) {
	result, err := circuitbreaker.Execute(c.cb, func() (outcome, error) {
		status, body, sendErr := c.send(ctx, method, path, payload)
		if sendErr != nil {
			if ctx.Err() != nil {
				return outcome{err: sendErr}, nil
			}
			return outcome{}, sendErr
		}
		if status >= http.StatusInternalServerError {
			return outcome{}, fmt.Errorf("status %d", status)
		}
		return outcome{status: status, body: body}, nil
	})
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %s: %v", ErrUnavailable, c.name, err)
	}
	return result.status, result.body, result.err
}

func (c *Caller) send(ctx context.Context, method, path string, payload any) (int, []byte, error) {
	token, err := security.SignServiceToken(c.privateKey, c.subject)
	if err != nil {
		return 0, nil, err
	}

	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// envelope é o corpo padrão das respostas do ecossistema: o dado vem em "data".
type envelope[T any] struct {
	Data T `json:"data"`
}

// Decode lê o dado do envelope de resposta.
func Decode[T any](body []byte) (T, error) {
	var payload envelope[T]
	if err := json.Unmarshal(body, &payload); err != nil {
		var zero T
		return zero, err
	}
	return payload.Data, nil
}
