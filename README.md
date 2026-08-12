# chameleon-common

Biblioteca Go compartilhada pelos microsservicos do ecossistema Chameleon.

Ela existe para evitar duplicacao de infraestrutura entre APIs e manter um comportamento padrao em autenticacao, autenticacao service-to-service, contexto tenant, bootstrap HTTP, respostas HTTP e validacao.

## Quando usar

Use esta lib quando o microservico precisar de pelo menos um destes pontos:

- extrair e autorizar claims de um token de usuario (`typ=access`) no `gin.Context`
- proteger rotas por role ou permissao
- isolar rotas multi-tenant por `establishment_id`
- autenticar chamadas service-to-service com token de servico assinado por RSA
- montar o servidor HTTP padrao do servico (recovery, logging, security headers, swagger, health)
- registrar chamada HTTP externa com circuit breaker
- logger estruturado padronizado
- responder com o formato HTTP padrao da plataforma
- validar requests com mensagens consistentes
- reutilizar modelo base para entidades GORM

Em resumo: ela deve entrar quando o problema for infraestrutura compartilhada, nao regra de negocio.

## Quando nao usar

Esta lib nao deve:

- conhecer regra de negocio de um servico especifico
- acessar banco diretamente
- depender de repositories concretos
- resolver dependencias externas por conta propria

## O que ela entrega

Pacotes principais:

- `pkg/middleware`: extracao/autorizacao de claims de usuario, token de servico (RSA), contexto tenant, security headers e logging de request
- `pkg/security`: assinatura/validacao de token de servico via RSA, e interfaces para blacklist e versionamento de token de usuario
- `pkg/httpserver`: bootstrap do `*http.Server` padrao (recovery, logger, security headers, swagger opcional, health)
- `pkg/circuitbreaker`: circuit breaker generico para chamada HTTP sincrona entre servicos
- `pkg/log`: logger `zerolog` padronizado (nome do servico, ambiente, nivel)
- `pkg/http`: helpers para respostas HTTP em handlers Gin
- `pkg/response`: estrutura padrao de sucesso, erro e paginacao
- `pkg/validation`: validacao de payloads (inclusive documento/telefone/CEP BR) e traducao de erros
- `pkg/base`: modelo base e DTO base para entidades GORM

## Como usar

Instalacao:

```bash
go get github.com/felipedenardo/chameleon-common
```

### Bootstrap do servidor HTTP

`httpserver.New` monta o `*http.Server` com o stack padrao (recovery, request logger, limite de corpo, security headers, swagger opcional e `/health`) — o servico so registra as proprias rotas via callback:

```go
package main

import (
	"net/http"

	commonlog "github.com/felipedenardo/chameleon-common/pkg/log"
	"github.com/felipedenardo/chameleon-common/pkg/httpserver"
	"github.com/gin-gonic/gin"
)

func main() {
	logger := commonlog.New("meu-servico", "development", "info")

	srv := httpserver.New(logger, httpserver.Options{
		ServiceName:  "meu-servico",
		Port:         "8080",
		BasePath:     "/meu-servico",
		MaxBodyBytes: 1048576,
		Swagger:      true,
	}, func(api *gin.RouterGroup) {
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"pong": true})
		})
	})

	_ = srv.ListenAndServe()
}
```

### Autenticacao de usuario

`AuthMiddleware` **extrai e autoriza** as claims de um token `typ=access` — ele **nao reverifica assinatura nem expiracao**: isso e responsabilidade do Kong (plugin `jwt`) na borda, a unica porta de entrada externa dos servicos. Essa premissa so e segura enquanto nenhum servico publica porta pro host alem do Kong.

```go
package main

import (
	"context"

	httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
	"github.com/felipedenardo/chameleon-common/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type blacklistChecker struct{}

func (blacklistChecker) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	return false, nil
}

type tokenVersionChecker struct{}

func (tokenVersionChecker) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	return 1, nil
}

func main() {
	r := gin.New()

	auth := middleware.AuthMiddleware(blacklistChecker{}, tokenVersionChecker{})

	api := r.Group("/api").Use(auth)

	api.GET("/me", func(c *gin.Context) {
		userID, _ := middleware.GetUserID(c)
		httphelpers.RespondOK(c, gin.H{"user_id": userID})
	})

	tenant := api.Group("/:establishmentID").Use(
		middleware.RequireEstablishmentContext(),
	)

	tenant.GET(
		"/stats",
		middleware.RequirePermission("dashboard.read"),
		func(c *gin.Context) {
			establishmentID, _ := middleware.GetEstablishmentID(c)
			httphelpers.RespondOK(c, gin.H{"establishment_id": establishmentID})
		},
	)

	_ = r.Run(":8080")
}
```

`blacklistTokenChecker`/`tokenVersionChecker` sao opcionais — passe `nil` se o servico nao precisar revogar sessao.

`RequireRole` e `RequirePermission`:

- use `RequireRole` quando a rota depende de perfil fixo
- use `RequirePermission` quando a protecao precisa ser mais granular
- permissoes aceitam match exato e wildcards como `*` e `appointments.*`

`RequireEstablishmentContext`:

- usa em rotas como `/:establishmentID/...` — o parametro de rota **e sempre o id imutavel**
- usuario comum so passa se o `establishment_id` do token bater com o da rota
- platform admin (permissao `*` ou `platform.*`) atua sobre qualquer tenant indicado na rota

### Autenticacao service-to-service

Chamada HTTP sincrona entre servicos (nao iniciada por usuario) usa um token de servico curto, assinado com a chave **privada** RSA do servico chamador. Quem recebe valida contra a chave **publica** correspondente, mapeada por `sub` (subject) num allow-list — um `sub` fora do mapa e rejeitado mesmo com assinatura valida contra outra chave.

Lado que chama:

```go
privateKey, err := security.LoadRSAPrivateKeyFile(cfg.ServicePrivateKeyPath)
if err != nil {
	// tratar erro
}

token, err := security.SignServiceToken(privateKey, "chameleon-meu-servico")
```

Lado que recebe (rota interna, nunca exposta pelo Kong):

```go
trustedKeys := map[string]*rsa.PublicKey{
	"chameleon-outro-servico": outroServicoPublicKey,
}

internalGroup := api.Group("/internal", middleware.ServiceTokenMiddleware(trustedKeys))
```

### Circuit breaker

Para chamada HTTP sincrona a outro servico ou dependencia externa:

```go
cb := circuitbreaker.New(circuitbreaker.Settings{
	Name:             "meu-servico-outro-servico",
	MaxHalfOpenReqs:  1,
	Interval:         60 * time.Second,
	Timeout:          30 * time.Second,
	FailureThreshold: 5,
})

result, err := circuitbreaker.Execute(cb, func() (Result, error) {
	return doHTTPCall(ctx)
})
```

### Respostas HTTP e validacao

```go
type CreateUserRequest struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required,min=3"`
}

func CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httphelpers.RespondBindingError(c, err)
		return
	}

	if errs := validation.ValidateRequest(req); errs != nil {
		httphelpers.RespondValidation(c, errs)
		return
	}

	httphelpers.RespondCreated(c, gin.H{"created": true})
}
```

`validation.SetupCustomValidator()` registra, no boot do servico, os validators BR (`br_document`, `br_phone`, `br_zip`) no engine de binding do Gin e ajusta o nome dos campos nas mensagens de erro para o `json` tag — chamar uma vez no bootstrap:

```go
func main() {
	validation.SetupCustomValidator()
	// ...
}
```

## Contratos esperados

Se o servico quiser revogar token de usuario ou validar versao, implementa as interfaces abaixo (ambas opcionais em `AuthMiddleware`):

```go
type BlacklistTokenChecker interface {
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type TokenVersionChecker interface {
	GetUserTokenVersion(ctx context.Context, userID string) (int, error)
}
```

## Para quem evoluir a lib

Mantenha a biblioteca pequena, previsivel e compartilhavel:

- preserve compatibilidade sempre que possivel
- prefira contratos pequenos e injetaveis
- nao mova regra de negocio para ca
- adicione apenas o que realmente fizer sentido para mais de um servico

## Versionamento

O projeto segue SemVer, via tags Git (`v0.30.0`, etc.) — cada microsservico consumidor pina uma versao especifica no `go.mod`, nunca a branch principal.
