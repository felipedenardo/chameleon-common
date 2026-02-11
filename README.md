# 🦎 Chameleon Common Lib

Biblioteca de utilitários compartilhados (**Shared Kernel**) para o ecossistema de microsserviços Chameleon (Auth, CRM, Agent).

O objetivo desta biblioteca é padronizar:
- **Segurança e Acesso** (Middleware JWT)
- **Respostas da API** (Padrão JSEND)
- **Validação de Dados** (Wrapper do Validator v10)
- **Modelos de Banco** (UUID v4 & Soft Delete)

---

## 📦 Instalação

No seu microserviço (ex: `auth-api`), execute:

```bash
go get github.com/felipedenardo/chameleon-common
```

## Como Usar

### 1. Interface HTTP (pkg/http)
Este pacote encapsula todas as chamadas c.JSON() e a lógica de tradução de erros. É o ponto de saída final da sua API.

```go
import httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"

func GetProfile(c *gin.Context) {
    // Retorna 404
    httphelpers.RespondNotFound(c)
    
    // Retorna 500
    httphelpers.HandleInternalError(c, err)
    
    // Sucesso 201
    httphelpers.RespondCreated(c, data)
}
```

### 2. Autenticação e Autorização (pkg/middleware)
O Middleware verifica o Token JWT emitido pelo auth-api e injeta userID e role no contexto do Gin. Agora com suporte nativo a **Blacklist** e **Token Versioning**.

```go
import (
    "github.com/felipedenardo/chameleon-common/pkg/middleware"
    "github.com/felipedenardo/chameleon-common/pkg/security"
)

func SetupProtectedRoutes(r *gin.Engine, cfg *Config, blacklist security.BlacklistTokenChecker, versioning security.TokenVersionChecker) {
    // Agora o middleware exige checkers de segurança
    authMiddleware := middleware.AuthMiddleware(cfg.JWTSecret, blacklist, versioning)
    
    protectedRoutes := r.Group("/api/v1/profiles").Use(authMiddleware)
    {
        protectedRoutes.GET("/me", userHandler.GetProfile) 
    }
}
```

#### Segurança Adicional
- **Blacklist Check:** Verifica se o `jti` do token foi revogado (Logout).
- **Token Versioning:** Valida se a versão do token (`token_version` claim) é inferior à versão atual do usuário no banco/cache (Global Logout).

### 3. Padronização de Respostas (pkg/response)
A biblioteca fornece Atalhos (mensagens automáticas) e métodos Customizados.

```go
import "github.com/felipedenardo/chameleon-common/pkg/response"

func MyHandler(c *gin.Context) {
    c.JSON(201, response.NewCreated(data))
    c.JSON(200, response.NewPaged(lista, page, perPage, total))
    c.JSON(400, response.NewFailCustom("Resposta customizada", nil))
}
```

### 4. Validação Automática
Wrapper que traduz erros técnicos do go-playground/validator para o formato response.FieldError

```go
import (
    "github.com/felipedenardo/chameleon-common/pkg/validation"
    httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
)

func Login(c *gin.Context) {
    if errs := validation.ValidateRequest(req); errs != nil {
        httphelpers.RespondValidation(c, errs)
        return
    }
}
```

### 5. Base Model com UUID

```go
import "github.com/felipedenardo/chameleon-common/pkg/base"
 
type Professional struct {
    base.Model
    Name string `json:"name"`
}
// ...
```

### Versionamento

Este projeto utiliza **SemVer (Semantic Versioning)**.
As releases são controladas por **Git Tags**.

- **v0.x.x** — Desenvolvimento
- **v1.0.0** — Estável
