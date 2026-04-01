# chameleon-common

Biblioteca Go compartilhada pelos microservicos do ecossistema Chameleon.

Ela existe para evitar duplicacao de infraestrutura entre APIs e manter um comportamento padrao em autenticacao, contexto tenant, respostas HTTP e validacao.

## Quando usar

Use esta lib quando o microservico precisar de pelo menos um destes pontos:

- validar JWT e expor dados do usuario no `gin.Context`
- proteger rotas por role ou permissao
- trabalhar com rotas por `:slug` de estabelecimento
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

Se um fluxo precisa transformar `slug -> establishment_id`, por exemplo, o servico consumidor injeta essa regra. A lib so define o contrato e o fluxo.

## O que ela entrega

Pacotes principais:

- `pkg/middleware`: autenticacao JWT, autorizacao, contexto tenant e logging de request
- `pkg/http`: helpers para respostas HTTP em handlers Gin
- `pkg/response`: estrutura padrao de sucesso, erro e paginacao
- `pkg/validation`: validacao de payloads e traducao de erros
- `pkg/security`: interfaces para blacklist de token e versionamento
- `pkg/base`: modelo base e DTO base para entidades GORM

## Como usar

Instalacao:

```bash
go get github.com/felipedenardo/chameleon-common
```

Fluxo mais comum em um microservico Gin:

1. adicionar `RequestLogger`
2. proteger a API com `AuthMiddleware`
3. aplicar `RequireRole` e `RequirePermission` nas rotas necessarias
4. usar `RequireEstablishmentSlug` nas rotas multi-tenant por `:slug`
5. responder handlers com `pkg/http`
6. validar payloads com `pkg/validation`

Exemplo enxuto:

```go
package main

import (
	"context"

	httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
	"github.com/felipedenardo/chameleon-common/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
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
	r.Use(middleware.RequestLogger(zerolog.Nop()))

	auth := middleware.AuthMiddleware(
		"secret",
		blacklistChecker{},
		tokenVersionChecker{},
	)

	api := r.Group("/api").Use(auth)

	api.GET("/me", func(c *gin.Context) {
		userID, _ := middleware.GetUserID(c)
		httphelpers.RespondOK(c, gin.H{"user_id": userID})
	})

	tenant := api.Group("/establishments/:slug").Use(
		middleware.RequireEstablishmentSlug(),
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

## Como pensar o uso no dia a dia

`AuthMiddleware`:

- valida JWT do tipo `access`
- injeta no contexto dados como `userID`, `role`, `permissions`, `establishment_id` e `establishment_slug`
- pode consultar blacklist e versao de token se o servico fornecer as interfaces de `pkg/security`

`RequireRole` e `RequirePermission`:

- use `RequireRole` quando a rota depende de perfil fixo
- use `RequirePermission` quando a protecao precisa ser mais granular
- permissoes aceitam match exato e wildcards como `*` e `appointments.*`

`RequireEstablishmentSlug`:

- use em rotas como `/:slug/...`
- garante que o usuario so atue dentro do tenant permitido
- para usuarios com acesso global, o servico pode injetar um resolver para materializar o `establishment_id`

Exemplo com resolver:

```go
tenant := api.Group("/establishments/:slug").Use(
	middleware.RequireEstablishmentSlugWithResolver(
		middleware.EstablishmentResolverFunc(func(ctx context.Context, slug string) (string, error) {
			return establishmentService.ResolveIDBySlug(ctx, slug)
		}),
	),
)
```

`pkg/http` e `pkg/validation`:

- use `RespondOK`, `RespondCreated`, `RespondValidation`, `RespondInternalError` e afins para manter o mesmo contrato HTTP entre servicos
- use `ValidateRequest` para devolver erros de payload com nomes de campo consistentes

Exemplo:

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

## Contratos esperados

Se o servico quiser revogar token ou validar versao, ele implementa as interfaces abaixo:

```go
type BlacklistTokenChecker interface {
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type TokenVersionChecker interface {
	GetUserTokenVersion(ctx context.Context, userID string) (int, error)
}
```

Variaveis de ambiente suportadas no JWT:

- `JWT_ISSUER`: valida `iss` quando configurado
- `JWT_AUDIENCE`: valida `aud` quando configurado
- `JWT_LEEWAY_SECONDS`: tolerancia para clock skew

## Para quem evoluir a lib

Mantenha a biblioteca pequena, previsivel e compartilhavel:

- preserve compatibilidade sempre que possivel
- prefira contratos pequenos e injetaveis
- nao mova regra de negocio para ca
- adicione apenas o que realmente fizer sentido para mais de um servico

## Versionamento

O projeto segue SemVer.
