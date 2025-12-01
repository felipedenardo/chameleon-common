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

### 1. Respostas Padronizadas

Use este pacote nos seus Handlers (Controllers) para garantir que todas as APIs retornem o mesmo formato JSON.

```go
import "github.com/felipedenardo/chameleon-common/pkg/http_api"
// Atalhos (Recomendado)
// Usam mensagens padrão como "success", "created successfully", etc.
c.JSON(200, response.NewOk(data))
c.JSON(200, response.NewPaged(lista, page, perPage, total))
c.JSON(200, response.NewDeleted())
c.JSON(200, response.NewUpdated(data))
c.JSON(201, response.NewCreated(data))
c.JSON(400, response.NewValidationErr(errors))
c.JSON(404, response.NewNotFound())
c.JSON(500, response.NewError(response.MsgInternalErr))
c.JSON(500, response.NewInternalErr())

// Customizados
// Use quando precisar de uma mensagem de negócio específica.
c.JSON(200, response.NewSuccess("Custom message 1", token))
c.JSON(400, response.NewFail("Custom message 2", nil))
c.JSON(500, response.NewError("Custom message 3"))
```

#### 2. Helpers de API (pkg/http_api)
Este pacote atua como uma camada de cola (infraestrutura) que lida com o Gin/Validator. Use-o para evitar escrever a lógica de conversão de erros em todos os seus Handlers.

```go
import "https://github.com/felipedenardo/chameleon-common/pkg/http_api"

func RegisterUser(c *gin.Context) {
    var req RegisterRequest

    if err := c.ShouldBindJSON(&req); err != nil {
        http_api.HandleBindingError(c, err)
        return
    }

    userID := c.Param("id")
    if userID == "" {
        http_api.HandleParamError(c, "id", "Invalid user id")
        return
    }
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


