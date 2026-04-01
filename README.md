# chameleon-common

Biblioteca Go compartilhada pelos microserviços do ecossistema Chameleon.

Ela concentra código transversal que não deveria ficar duplicado em cada serviço:
- autenticação JWT e autorização por role/permissão
- contexto multi-tenant por `establishment_id` e `slug`
- contratos de segurança para integração com blacklist e versionamento de token
- helpers de resposta HTTP em Gin
- estrutura padrão de payload de resposta
- validação de requests
- modelos base para GORM

## O que esta lib é

`chameleon-common` é uma shared lib de apoio para os serviços da plataforma. Ela define contratos, middlewares e utilitários reutilizáveis para manter comportamento consistente entre APIs.

O foco aqui é padronização:
- mesma forma de validar token
- mesma forma de expor contexto autenticado
- mesma estrutura de resposta HTTP
- mesma base de modelos e validação

## O que esta lib não é

Esta lib não conhece regra de negócio do domínio de cada serviço.

Ela também não deve:
- acessar banco diretamente
- conhecer repositories concretos
- resolver dependências específicas de um microserviço

Quando algum fluxo precisa de dados externos, como resolver `slug -> establishment_id`, a responsabilidade é do serviço consumidor via interface/função injetada.

## Instalação

```bash
go get github.com/felipedenardo/chameleon-common
```

## Estrutura

```text
pkg/
├── base/        Modelos base e DTOs para GORM
├── http/        Helpers HTTP para Gin
├── middleware/  Auth, autorização, contexto tenant e request logging
├── response/    Estruturas e mensagens de resposta
├── security/    Contratos para blacklist e token version
└── validation/  Validação de structs e tradução de erros
```

## Pacotes

### `pkg/middleware`

Pacote com os middlewares centrais da lib.

Disponível hoje:
- `AuthMiddleware(secretKey, blacklistChecker, tokenVersionChecker)`
- `RequireEstablishmentSlug()`
- `RequireEstablishmentSlugWithResolver(resolver)`
- `RequireEstablishmentSlugFunc(fn)`
- `RequireRole(roles...)`
- `RequirePermission(permissions...)`
- `RequestLogger(logger)`

Também expõe helpers para ler contexto do Gin:
- `GetUserID`
- `GetRawToken`
- `GetEstablishmentID`
- `GetEstablishmentUUID`
- `GetEstablishmentSlug`
- `GetEstablishmentIDs`
- `GetEstablishmentSlugs`
- `GetPermissions`

#### `AuthMiddleware`

Valida o JWT e injeta no contexto do Gin:
- `userID`
- `role`
- `permissions`
- `establishment_id`
- `establishment_slug`
- `establishment_ids`
- `establishment_slugs`
- `rawTokenString`

Também suporta duas verificações opcionais, implementadas pelo microserviço:
- blacklist de token via `security.BlacklistTokenChecker`
- versionamento/revogação via `security.TokenVersionChecker`

Claims relevantes esperadas no token:
- `sub`
- `typ=access`
- `jti`
- `role`
- `permissions`
- `establishment_id`
- `establishment_slug`
- `establishment_ids`
- `establishment_slugs`
- `token_version` quando houver checker de versão

Variáveis de ambiente lidas no setup do middleware:

| Variável | Obrigatória | Uso |
| :--- | :---: | :--- |
| `JWT_ISSUER` | Não | Valida o claim `iss` quando configurado |
| `JWT_AUDIENCE` | Não | Valida o claim `aud` quando configurado |
| `JWT_LEEWAY_SECONDS` | Não | Tolerância para clock skew |

#### Contexto tenant por `slug`

Para rotas como `/:slug/...`, a lib oferece middleware cross-tenant.

Comportamento:
- usuário comum: compara o `slug` da rota com `establishment_slug`
- owner/multi-establishment: procura o `slug` em `establishment_slugs` e ativa o `establishment_id` correspondente da mesma posição em `establishment_ids`
- admin/global: pode acessar qualquer `slug`; se faltar `establishment_id`, o serviço pode injetar um resolver para materializar o tenant ativo

Uso legado, sem resolver:

```go
tenant := api.Group("/establishments/:slug").Use(
    middleware.RequireEstablishmentSlug(),
)
```

Uso recomendado para rotas que precisam de `establishment_id` garantido no contexto:

```go
package routes

import (
    "context"

    "github.com/felipedenardo/chameleon-common/pkg/middleware"
)

func tenantResolver(ctx context.Context, slug string) (string, error) {
    return establishmentService.ResolveIDBySlug(ctx, slug)
}

func register(api *gin.RouterGroup) {
    tenant := api.Group("/establishments/:slug").Use(
        middleware.RequireEstablishmentSlugWithResolver(
            middleware.EstablishmentResolverFunc(tenantResolver),
        ),
    )

    tenant.GET("/stats", handler.Stats)
}
```

Esse resolver é intencionalmente responsabilidade do microserviço consumidor. A shared lib só define o fluxo.

#### Roles e permissões

`RequireRole` faz validação exata de role.

`RequirePermission` aceita:
- match exato, ex: `appointments.create`
- wildcard global: `*`
- wildcard por módulo: `appointments.*`

Exemplo:

```go
tenant.GET(
    "/appointments",
    middleware.RequireRole("admin", "manager"),
    middleware.RequirePermission("appointments.read"),
    handler.ListAppointments,
)
```

#### Request logging

`RequestLogger` registra:
- método
- path
- status
- latência
- IP
- user agent

E ajusta o nível do log:
- `info` para sucesso
- `warn` para 4xx
- `error` para 5xx

### `pkg/security`

Contém apenas contratos usados pelo middleware de autenticação:

```go
type BlacklistTokenChecker interface {
    IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type TokenVersionChecker interface {
    GetUserTokenVersion(ctx context.Context, userID string) (int, error)
}
```

Essas interfaces devem ser implementadas no microserviço consumidor.

### `pkg/http`

Helpers para respostas HTTP em handlers Gin, sempre usando o formato padronizado do pacote `response`.

Principais funções:
- `RespondOK`
- `RespondCreated`
- `RespondUpdated`
- `RespondDeleted`
- `RespondPaged`
- `RespondValidation`
- `RespondUnauthorized`
- `RespondForbidden`
- `RespondNotFound`
- `RespondInternalError`
- `RespondBindingError`
- `RespondParamError`

Exemplo:

```go
import httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"

func GetProfile(c *gin.Context) {
    profile, err := service.GetProfile(c.Request.Context())
    if err != nil {
        httphelpers.RespondInternalError(c, err)
        return
    }

    httphelpers.RespondOK(c, profile)
}
```

### `pkg/response`

Define o shape de resposta usado pelos helpers HTTP.

Estrutura base:

```json
{
  "status": "success",
  "message": "success",
  "data": {},
  "meta": {},
  "errors": []
}
```

Tipos disponíveis:
- `Standard`
- `FieldError`
- `PaginationMeta`

Factories disponíveis:
- `NewSuccess`
- `NewCreated`
- `NewUpdated`
- `NewDeleted`
- `NewPaged`
- `NewValidationErr`
- `NewInternalErr`
- `NewNotFound`
- `NewFailCustom`
- `NewErrorCustom`

### `pkg/validation`

Responsável por validar structs e traduzir erros do `go-playground/validator`.

Funções principais:
- `ValidateRequest`
- `FromValidationErrors`
- `SetupCustomValidator`

`SetupCustomValidator()` integra o validator com o binder do Gin para usar os nomes do campo definidos na tag `json`.

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
}
```

### `pkg/base`

Ajuda a padronizar modelos e DTOs compartilhados.

`base.Model` inclui:
- `ID uuid.UUID`
- `CreatedAt time.Time`
- `UpdatedAt *time.Time`
- `DeletedAt gorm.DeletedAt`

`base.ModelDTO` é a versão voltada para saída/serialização.

`base.ToDTO(model)` converte `base.Model` em `base.ModelDTO`.

Exemplo:

```go
type User struct {
    base.Model
    Name string `json:"name"`
}

func toResponse(user User) any {
    return struct {
        base.ModelDTO
        Name string `json:"name"`
    }{
        ModelDTO: base.ToDTO(user.Model),
        Name:     user.Name,
    }
}
```

## Exemplo de uso completo

```go
package main

import (
    "context"

    httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
    "github.com/felipedenardo/chameleon-common/pkg/middleware"
    "github.com/felipedenardo/chameleon-common/pkg/security"
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
    logger := zerolog.Nop()

    r.Use(middleware.RequestLogger(logger))

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

## Notas para quem for evoluir a lib

- preserve compatibilidade sempre que possível, porque vários serviços podem depender da mesma API
- prefira contratos pequenos e injeção de dependência em vez de acoplamento com implementações concretas
- evite colocar regra de negócio específica de um serviço aqui
- mantenha esta lib focada em infraestrutura compartilhada, contexto e padronização

## Versionamento

O projeto segue SemVer.
