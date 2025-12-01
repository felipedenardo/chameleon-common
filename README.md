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

### 1. Padronização de Respostas (pkg/response)
A biblioteca fornece Atalhos (mensagens automáticas) e métodos Customizados (mensagens manuais).

```go
import "github.com/felipedenardo/chameleon-common/pkg/response"

func MyHandler(c *gin.Context) {
    c.JSON(201, response.NewCreated(data))
    c.JSON(200, response.NewOk(data))
     	
    c.JSON(200, response.NewSuccessCustom("Login realizado com sucesso", token))
    c.JSON(400, response.NewFailCustom("Saldo insuficiente para transação", nil))
}
```

### 2. Interface HTTP (pkg/http)

Este pacote encapsula todas as chamadas c.JSON() e a lógica de tradução de erros. É o ponto de saída final da sua API.

```go
import httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"

func GetProfile(c *gin.Context) {
	// Retorna 404
	httphelpers.RespondNotFound(c)
	
	// Retorna 500
	httphelpers.HandleInternalError(c, err)
}
```

### 3. Validação Automática

Wrapper que traduz erros técnicos do go-playground/validator para o formato response.FieldError, amigável para o Frontend.

```go
import (
    "github.com/felipedenardo/chameleon-common/pkg/validation"
    httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
)

type LoginRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=6"`
}

func Login(c *gin.Context) {
    var req LoginRequest
    if errs := validation.ValidateRequest(req); errs != nil {
    httphelpers.RespondValidation(c, errs)
        return
    }
}
```

### 4. Base Model com UUID

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
