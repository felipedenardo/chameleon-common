# 🦎 Chameleon Common Lib

[![Go Version](https://img.shields.io/github/go-mod/go-version/felipedenardo/chameleon-common)](https://golang.org/)

Biblioteca de utilitários compartilhados (**Shared Kernel**) para o ecossistema de microsserviços Chameleon (Auth, CRM, Agent).

## 📖 Sumário

- [Visão Geral](#-visão-geral)
- [Estrutura do Projeto](#-estrutura-do-projeto)
- [📦 Instalação](#-instalação)
- [Como Usar](#-como-usar)
    - [1. Interface HTTP (`pkg/http`)](#1-interface-http-pkghtp)
    - [2. Autenticação (`pkg/middleware`)](#2-autenticação-e-autorização-pkgmiddleware)
    - [3. Respostas da API (`pkg/response`)](#3-padronização-de-respostas-pkgresponse)
    - [4. Validação (`pkg/validation`)](#4-validação-automática)
    - [5. Modelos de Base (`pkg/base`)](#5-base-model-com-uuid)
- [Versionamento](#-versionamento)

---

## 🎯 Visão Geral

O objetivo desta biblioteca é padronizar todo o código comum para apoiar **Arquiteturas Multi-Tenant e Cross-Services**:
- **Segurança Multi-Tenant**: Middleware JWT focado em RBAC (funções e permissões complexas com wildcards) e extração nativa de Tenant (`establishment_id` e `establishment_slug`).
- **Prevenção Cross-Tenant**: Middleware nativo para bloquear acessos indevidos fora do escopo do Tenant (`RequireEstablishmentSlug`).
- **Respostas da API**: Padronização baseada no formato JSEND, idêntica em todos os serviços.
- **Validação de Dados**: Wrapper amigável para o `validator v10`.
- **Modelos de Banco**: Implementação de UUID v4 & Soft Delete nativos para GORM.

## 📂 Estrutura do Projeto

```text
pkg/
├── base/        # Modelos base e DTOs (GORM, UUID)
├── http/        # Helpers para o framework Gin (Respostas rápidas)
├── middleware/  # Middlewares de segurança (JWT)
├── response/    # Estruturas JSEND e mensagens padrão
├── security/    # Interfaces de Blacklist e Validadores de Versão
└── validation/  # Lógica de validação e tradução de erros
```

---

## 📦 Instalação

No seu microserviço, execute:

```bash
go get github.com/felipedenardo/chameleon-common
```

---

## 🚀 Como Usar

### 1. Interface HTTP (`pkg/http`)
Este pacote encapsula chamadas `c.JSON()` e centraliza a lógica de erros.

| Método | Descrição | Status HTTP |
| :--- | :--- | :---: |
| `RespondOK` | Sucesso padrão | 200 |
| `RespondCreated` | Recurso criado | 201 |
| `RespondUpdated` | Recurso atualizado | 200 |
| `RespondDeleted` | Recurso removido | 200 |
| `RespondPaged` | Lista paginada | 200 |
| `RespondNotFound` | Recurso não encontrado | 404 |
| `RespondInternalError` | Erro interno (Logs automáticos) | 500 |

```go
import httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"

func GetProfile(c *gin.Context) {
    // Sucesso 201
    httphelpers.RespondCreated(c, data)
    
    // Erro 500 (Gera log interno com o erro original)
    httphelpers.RespondInternalError(c, err)
}
```

### 2. Autenticação e Autorização (`pkg/middleware`)
O Middleware verifica o Token JWT fornecido pelo Auth API e injeta diretamente no contexto do Gin (`userID`, `role`, `permissions`, `establishment_id` e `establishment_slug`), blindando completamente a arquitetura de **Multi-Tenant**.

```go
import (
    "github.com/felipedenardo/chameleon-common/pkg/middleware"
    "github.com/felipedenardo/chameleon-common/pkg/security"
)

func SetupRoutes(r *gin.Engine, blacklist security.BlacklistTokenChecker, versioning security.TokenVersionChecker) {
    authMiddleware := middleware.AuthMiddleware("sua-secret", blacklist, versioning)
    
    api := r.Group("/api/v1").Use(authMiddleware)
    {
        // 🔒 Rotas Genéricas (Sem estabelecimento/tenant definido)
        api.GET("/me", handler.Me) 

        // 🏢 Rotas Multi-Tenant (Cross-Tenant Middleware bloqueia tentativas indevidas)
        tenant := api.Group("/establishments/:slug").Use(middleware.RequireEstablishmentSlug())
        {
            // Validação exata da role
            tenant.GET("/stats", middleware.RequireRole("admin", "manager"), handler.Stats)

            // 🌟 Validação de Permissões com suporte a Wildcards (* e module.*)
            tenant.POST("/appointments", middleware.RequirePermission("appointments.create"), handler.CreateAppt)
        }
    }
}
```

Variáveis de ambiente esperadas pelo `AuthMiddleware` (Lidas apenas uma vez no momento do Setup da rota, garantindo máxima performance):

| Variável | Obrigatória | Descrição |
| :--- | :---: | :--- |
| `JWT_ISSUER` | Sim | Valor esperado no claim `iss`. |
| `JWT_AUDIENCE` | Sim | Valor esperado no claim `aud`. |
| `JWT_LEEWAY_SECONDS` | Não | Tolerância de clock skew para `exp/nbf/iat` (segundos). |

Exemplo (em `.env` ou no ambiente do serviço consumidor):

```bash
JWT_ISSUER=chameleon-auth-api
JWT_AUDIENCE=chameleon-services
JWT_LEEWAY_SECONDS=30
```

### 3. Padronização de Respostas (`pkg/response`)
Estructuras prontas para retornar JSON no formato JSEND.

```go
import "github.com/felipedenardo/chameleon-common/pkg/response"

// Resposta manual se necessário
c.JSON(200, response.NewPaged(lista, page, perPage, total))
```

### 4. Validação Automática
Tradução de erros do `go-playground/validator` para mensagens amigáveis.

```go
import (
    "github.com/felipedenardo/chameleon-common/pkg/validation"
    httphelpers "github.com/felipedenardo/chameleon-common/pkg/http"
)

func Login(c *gin.Context) {
    if errs := validation.ValidateRequest(req); errs != nil {
        httphelpers.RespondValidation(c, errs) // Retorna 400 com detalhes
        return
    }
}
```

### 5. Base Model e DTOs (`pkg/base`)
Padronização de IDs e auditoria para GORM.

```go
import "github.com/felipedenardo/chameleon-common/pkg/base"
 
type User struct {
    base.Model // Inclui ID (UUID), CreatedAt, UpdatedAt, DeletedAt
    Username string
}

// Transformação para DTO
userDTO := base.ToDTO(user)
```

---

## 🏷️ Versionamento

Este projeto utiliza **SemVer (Semantic Versioning)**.
As releases são controladas por **Git Tags**.

- **v0.x.x** — Desenvolvimento / Beta
- **v1.0.0** — Estável para Produção
