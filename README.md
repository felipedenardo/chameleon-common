# 🦎 Chameleon Common Lib

Biblioteca de utilitários compartilhados (**Shared Kernel**) para o ecossistema de microsserviços Chameleon (Auth, CRM, Agent).

O objetivo desta biblioteca é padronizar:
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

### 1. Interface HTTP 
Este pacote encapsula todas as chamadas `c.JSON()` e a lógica de tradução de erros para garantir que seus Handlers fiquem limpos.

```go
import (
    httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
    "github.com/felipedenardo/chameleon-common/pkg/response"
)

func RegisterUser(c *gin.Context) {
    var req RegisterRequest
    
    if err := c.ShouldBindJSON(&req); err != nil {
        httphelpers.HandleBindingError(c, err) 
        return
    }

    user, err := service.Register(...)

    if err != nil {
        if errors.Is(err, service.ErrEmailExists) {
            httphelpers.RespondDomainFail(c, err.Error()) 
            return
        }
        httphelpers.HandleInternalError(c, err) 
        return
    }

    httphelpers.RespondCreated(c, user) 
}

func GetProfile(c *gin.Context) {
    userID := c.Param("id")
    if userID == "" {
        httphelpers.HandleParamError(c, "id", "ID is required") 
        return
    }

    if profile == nil {
        httphelpers.RespondNotFound(c)
        return
    }
    
    httphelpers.RespondPaged(c, lista, page, perPage, total)
}
```

### 3. Validação Automática

Wrapper que traduz erros técnicos do `go-playground/validator` para o formato `response.FieldError`, amigável para o Frontend.  
Ele lê as tags JSON para nomear os campos corretamente.

```go
import (
"github.com/felipedenardo/chameleon-common/pkg/validation"
"github.com/felipedenardo/chameleon-common/pkg/response"
)

type LoginRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=6"`
}

if errs := validation.ValidateRequest(req); errs != nil {
    c.JSON(400, response.NewFail("Erro de validação", errs))
}
```

### 4. Base Model com UUID

Struct base para entidades GORM.  
Já vem configurada com **UUID v4 como Primary Key**, timestamps automáticos e helper para **DTO limpo** (sem campos internos do GORM).

```go
import "github.com/felipedenardo/chameleon-common/pkg/base"
 
type Professional struct {
    base.Model
    Name string `json:"name"`
}

func ToResponse(p *Professional) MyResponse {
    return MyResponse{
            ModelDTO: base.ToDTO(p.Model),
            Name:     p.Name,
    }
}
```

### Versionamento

Este projeto utiliza **SemVer (Semantic Versioning)**. 
As releases são controladas por **Git Tags**.

- **v0.x.x** — Desenvolvimento
- **v1.0.0** — Estável


