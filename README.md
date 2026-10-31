# chameleon-common

Biblioteca Go compartilhada pelos microsserviços do ecossistema Chameleon. Guarda
a **infraestrutura** que todo serviço precisa igual — autenticação de usuário e
de serviço, servidor HTTP, respostas de erro, circuit breaker, transação,
cache, métricas — para que cada serviço só escreva a própria regra de negócio.

## O que entra aqui

1. **Mecanismo usado igual por dois ou mais serviços**: como falar com outro
   serviço, autenticar, gravar, ler fila, medir.
2. **Nunca decisão de domínio**: preço, status de negócio, regra de um serviço,
   texto de tela além das mensagens genéricas, acesso a banco de um domínio,
   repositório concreto. Duas exceções conscientes: `pkg/money`, que existe
   para o appointment e o sales calcularem exatamente o mesmo desconto, e o
   plano no token (`RequireActivePlan`, `RequireModule`), que é contrato do
   token como as permissões.
3. **Cada pacote se sustenta sozinho e entra com teste.** Os de `pkg/messaging`
   não são importados por nenhum outro pacote daqui: se um dia a parte de filas
   virar um módulo próprio, ela sai sem arrastar o resto.

## Sumário

- [O que entra aqui](#o-que-entra-aqui)
- [Pacotes](#pacotes)
- [Pipeline de uma requisição autenticada](#pipeline-de-uma-requisição-autenticada)
- [Autenticação de usuário](#autenticação-de-usuário)
- [Token de serviço (chamada interna)](#token-de-serviço-chamada-interna)
- [Chamar outro serviço: servicecall](#chamar-outro-serviço-servicecall)
- [Filas: consumidor, outbox e profundidade](#filas-consumidor-outbox-e-profundidade)
- [Circuit breaker](#circuit-breaker)
- [Transação: UnitOfWork](#transação-unitofwork)
- [Cache com invalidação por geração](#cache-com-invalidação-por-geração)
- [Respostas e erros](#respostas-e-erros)
- [Servidor HTTP e métricas](#servidor-http-e-métricas)
- [Versionamento e release](#versionamento-e-release)

## Pacotes

| pacote | o que entrega |
|---|---|
| `pkg/httpserver` | `*http.Server` padrão: recovery, request logger, limite de corpo, security headers, métricas, Swagger opcional e `/health` |
| `pkg/middleware` | autenticação de usuário (RS256 + revogação), papel, permissão, contexto do estabelecimento, token de serviço, security headers |
| `pkg/security` | assinatura e validação do token de serviço (RSA), carga de chaves PEM, verificador de revogação no Redis |
| `pkg/servicecall` | chamada a outro serviço: assina o token de serviço, timeout, circuit breaker por destino, `ErrUnavailable` para "fora do ar" |
| `pkg/messaging/consumer` | laço de leitura de uma fila SQS: espera longa, espera crescente na falha passageira, recusa permanente, confirmações em lote |
| `pkg/messaging/outbox` | relay do outbox transacional: lê, envia em lote e marca publicados e recusados (`FAILED`), uma escrita por desfecho |
| `pkg/messaging/queuedepth` | profundidade da fila e da DLQ para os alertas (`chameleon_sqs_messages`) |
| `pkg/circuitbreaker` | circuit breaker genérico para chamada HTTP síncrona |
| `pkg/persistence` | `UnitOfWork`: transação em `domain`/`application` sem expor GORM |
| `pkg/cache` | cache no Redis com chave versionada e invalidação explícita |
| `pkg/redisclient` | cliente Redis a partir das variáveis padrão (`REDIS_*`) |
| `pkg/metrics` | Prometheus: métricas HTTP, contadores/gauges, medição de jobs, servidor de métricas do worker |
| `pkg/http` · `pkg/response` | respostas padrão (sucesso, paginação, erro) — mensagens em português |
| `pkg/validation` | validação de payload, validadores BR (`br_document`, `br_phone`, `br_zip`) |
| `pkg/money` | arredondamento e desconto (percentual ou valor fixo) — o vocabulário de desconto da plataforma |
| `pkg/log` | logger `zerolog` padronizado (serviço, ambiente, nível) |
| `pkg/base` | modelo e DTO base para entidades GORM (id, timestamps) |

## Pipeline de uma requisição autenticada

```mermaid
flowchart LR
    req["requisição"] --> rec["recovery"] --> log["request logger"] --> body["limite de corpo"] --> sec["security headers"] --> met["métricas"]
    met --> auth["AuthMiddleware<br/>RS256 + revogação"]
    auth --> tenant["RequireEstablishmentContext<br/>establishment do token = da rota"]
    tenant --> perm["RequirePermission / RequireRole"]
    perm --> h["handler do serviço"]
```

Tudo até `métricas` vem do `httpserver.New`; os três de autorização o serviço
monta nas próprias rotas.

## Autenticação de usuário

`AuthMiddleware` **valida o token de verdade**: assinatura RS256 contra a chave
pública do auth-api, expiração, `typ=access`, e a revogação no Redis. O Kong
também valida na borda, mas não é a única barreira — confiar só no gateway
amarraria a segurança à topologia de rede.

```go
publicKey, _ := security.LoadRSAPublicKeyFile(cfg.UserTokenPublicKeyPath)
redisClient, _ := redisclient.New(redisclient.FromEnv())
revocation := security.NewRedisRevocationChecker(redisClient, "meu-servico")

auth := middleware.AuthMiddleware(publicKey, revocation, revocation)

tenant := api.Group("/:establishmentID", auth, middleware.RequireEstablishmentContext())
tenant.GET("/stats", middleware.RequirePermission("dashboard.read"), handler)
```

- **Os três parâmetros são obrigatórios.** A função entra em pânico no boot se algum vier nil: autorização pela metade precisa ser impossível de montar. (Uma versão anterior aceitava nil, e seis serviços subiram sem revogação — deslogar não deslogava.)
- **Revogação** (escrita pelo auth-api, lida aqui): blacklist do `jti` (logout de um dispositivo) e `token_version` do usuário (derruba todas as sessões). Se o Redis não responder, a requisição passa e a métrica `auth_revocation_check_degraded_total` sobe — cache fora custa segurança temporária, não disponibilidade, e a métrica é o alarme.
- **`RequireEstablishmentContext`**: o `establishment_id` do token tem que ser o da rota; o admin da plataforma (`*` ou `platform.*`) atua em qualquer unidade.
- **`RequirePermission`** aceita igualdade exata e curingas (`*`, `appointments.*`); `RequireRole` para perfil fixo (`owner`).
- Leitura do contexto no handler: `middleware.RequireUserID(c)`, `RequireEstablishmentID(c)`, `RequireUUIDParam(c, "id")`.

### Plano do Chameleon: `RequireActivePlan` e `RequireModule`

O auth-api põe no token de unidade a situação do plano (billing-api):
`plan_status` (`ok`, `blocked`, `no_contract`) e `plan_modules`. O
`AuthMiddleware` guarda os dois no contexto, e as guardas leem dali — **sem
I/O**:

```go
tenant := api.Group("/:establishmentID", auth,
    middleware.RequireEstablishmentContext(), middleware.RequireActivePlan())
stock := tenant.Group("/products", middleware.RequireModule("inventory"))
```

- `RequireActivePlan`: `blocked` → **402** `{"code": "plan_blocked"}`; `no_contract` → **402** `plan_no_contract`. O cliente final lê "agenda indisponível", sem menção a cobrança.
- `RequireModule(m)`: módulo fora do plano → **403** `module_not_in_plan`.
- Passam sempre: admin da plataforma, token **sem** a situação (sem unidade, emitido com o billing fora do ar, ou de unidade que o billing não conhece) e situação desconhecida. Bloquear por falta de dado derrubaria quem paga.
- O `code` é o que o web usa para mandar a pessoa a "Plano suspenso", "Escolha seu plano" ou "Não incluído no seu plano".

## Token de serviço (chamada interna)

```mermaid
sequenceDiagram
    participant A as serviço chamador
    participant B as serviço chamado
    A->>A: SignServiceToken(chave PRIVADA de A, sub="chameleon-a-api")<br/>JWT RS256, typ=service, 2 min
    A->>B: GET /internal/... Authorization: Bearer
    B->>B: lê o sub SEM validar ainda
    B->>B: sub está no mapa de confiança? (não = recusa)
    B->>B: valida a assinatura com a chave PÚBLICA de A, e typ=service
    B-->>A: resposta
```

```go
// quem chama
token, _ := security.SignServiceToken(privateKey, "chameleon-meu-servico")

// quem recebe
trusted := map[string]*rsa.PublicKey{"chameleon-outro-servico": outroPublicKey}
internal := api.Group("/internal", middleware.ServiceTokenMiddleware(trusted))
```

Cada serviço tem o seu par de chaves (`make service-keys` no `chameleon-stack`).
Um `sub` fora do mapa é recusado mesmo com assinatura válida contra outra chave;
token de usuário nunca serve no lugar do de serviço (`typ` diferente).

## Chamar outro serviço: servicecall

O lado de quem chama do token de serviço. Um `Caller` por destino, guardado no
client de infra do serviço:

```go
caller := servicecall.New("inventory-api-products", "chameleon-purchase-api", baseURL, privateKey, 3*time.Second)

status, body, err := caller.Do(ctx, http.MethodGet, "/api/v1/internal/"+id+"/products?product_ids=...", nil)
switch {
case errors.Is(err, servicecall.ErrUnavailable): // transporte, timeout, 5xx ou circuito aberto: tentar depois resolve
case err != nil:                                  // quem chamou desistiu (contexto cancelado)
case status != http.StatusOK:                     // 4xx: resposta do destino, quem chamou decide
}
products, err := servicecall.Decode[[]product](body) // lê o "data" do envelope
```

Só transporte e 5xx contam como falha para o circuit breaker; um 4xx volta como
resposta e não abre o circuito. Quem traduz o status para as sentinelas da
porta do domínio (recusa × fora do ar) é o client de infra de cada serviço.

## Filas: consumidor, outbox e profundidade

Nada aqui chama o banco ou a fila dentro de um laço.

**Consumidor** (`pkg/messaging/consumer`). O serviço diz o que fazer com as
mensagens; o pacote cuida do resto: espera longa de 20 s, espera crescente
quando receber falha, métricas, e as confirmações em lote — uma chamada apaga
as tratadas, outra adia as que falharam de passagem.

```go
// uma mensagem por vez, cada uma um fato na sua transação (uma venda, uma compra)
c := consumer.NewSingle(client, consumer.Options{
    Name: "sale_events_consumer", QueueURL: url, Enabled: cfg.SaleEventsEnabled(), VisibilityTimeout: 30,
}, func(ctx context.Context, m types.Message) error {
    event, err := parse(m)
    if err != nil {
        return consumer.Permanent(err) // mal formada: segue o tempo normal até a DLQ
    }
    return service.Apply(ctx, event)  // erro aqui adia a próxima entrega
}, logger)

// o lote inteiro de uma vez, quando o tratamento aceita lote (uma transação)
c := consumer.New(client, consumer.Options{Name: "notifications_consumer", MaxMessages: 10, ...}, handleBatch, logger)
```

Na falha passageira a mensagem fica escondida 30 s, 1 min, 2 min, 4 min, 8 min
e depois 15 min a cada entrega: com 5 entregas, uns 15 minutos de dependência
fora antes da DLQ; com 10, perto de 1h30. A recusa marcada com `Permanent` não
espera.

**Outbox** (`pkg/messaging/outbox`). O serviço implementa o `Store` do seu
outbox (leitura dos pendentes e as marcações, todas em lote); o relay lê até 10
eventos, envia num `SendMessageBatch` e grava os desfechos — publicados,
recusados e a tentar de novo —, uma escrita cada. O evento que a fila recusa de
vez (corpo acima de 256 KiB, ou entrada recusada com `SenderFault`) vira
`FAILED` e sai do caminho dos outros; fila fora conta mais uma tentativa e o
relay espera antes da próxima passada. Métricas: `chameleon_outbox_pending_events`
e `chameleon_outbox_failed_events`, rotuladas por `stream` quando o serviço
publica em mais de uma fila.

**Profundidade** (`pkg/messaging/queuedepth`). Lê a fila e a DLQ (descoberta
pelo `RedrivePolicy`) a cada 30 s e alimenta `chameleon_sqs_messages{queue,state}`,
que os alertas de fila parada e de mensagem na DLQ usam.

## Circuit breaker

```mermaid
stateDiagram-v2
    [*] --> fechado
    fechado --> aberto: FailureThreshold falhas seguidas
    aberto --> meio_aberto: depois de Timeout
    meio_aberto --> fechado: a requisição de teste passa
    meio_aberto --> aberto: a requisição de teste falha
```

```go
cb := circuitbreaker.New(circuitbreaker.Settings{
    Name: "sales-agenda", MaxHalfOpenReqs: 1,
    Interval: 60 * time.Second, Timeout: 30 * time.Second, FailureThreshold: 5,
})
result, err := circuitbreaker.Execute(cb, func() (Result, error) { return call(ctx) })
```

Um breaker **por destino**: um serviço fora do ar não pode abrir o circuito de
outro. Conte como falha só transporte e 5xx — 4xx é resposta legítima.

## Transação: UnitOfWork

`domain`/`application` nunca abrem transação GORM. Quem precisa gravar várias
coisas juntas usa `persistence.UnitOfWork`, que injeta a transação no
`context.Context`; o repositório pega a conexão certa com
`persistence.DB(ctx, r.db)`:

```go
return s.uow.Execute(ctx, func(ctx context.Context) error {
    if err := s.repo.Create(ctx, venda); err != nil {
        return err
    }
    return s.outbox.Create(ctx, evento) // mesma transação
})
```

Em teste, `persistence.NewNoopUnitOfWork()` roda a função direto, sem banco.

## Cache com invalidação por geração

`pkg/cache` guarda no Redis respostas caras de montar. Mora **no serviço dono do
dado**: ele sabe quando o dado muda, então invalidar é uma chamada local no
mesmo caminho da escrita.

```go
key := cache.NewKey("establishment", "members").With(establishmentID)
members, err := cache.Resolve(ctx, store, key, 5*time.Minute, func() ([]Member, error) {
    return repo.ListMembers(ctx, establishmentID) // só roda no miss
})
```

- **Geração:** para invalidar um conjunto inteiro de chaves de uma vez, a chave carrega a geração do escopo (`Generation` / `NextGeneration`). Avançar a geração torna todas as chaves antigas inalcançáveis, sem varrer o Redis.
- **Fora do caminho crítico:** o timeout da operação é curto, e estouro é tratado como ausência — o cache nunca atrasa a resposta.
- Uso hoje: establishment-api.

## Respostas e erros

`pkg/http` monta o corpo padrão `{status, message, data, meta, errors}`.
Todas as mensagens de erro são **frases em português prontas para a tela**,
sem detalhe técnico:

| helper | status | mensagem |
|---|---|---|
| `RespondInternalError` | 500 (ou 408/499 em timeout/cancelamento) | "Algo deu errado do nosso lado…" — o erro real vai para o log |
| `RespondNotFound` | 404 | "Não encontrado." |
| `RespondBindingError` | 400 | "Os dados enviados estão em um formato inválido." — o texto do parser JSON vai para o log, não para a resposta |
| `RespondValidation` | 400 | "Confira os dados informados." + erros por campo, em português |
| `RespondDomainFail` | 400 | a frase de domínio que o serviço escolheu |
| middleware de auth | 401 / 403 | "Sua sessão expirou…" / "Você não tem permissão…" |

A convenção de como cada serviço escolhe a frase de domínio está em
`chameleon-stack/.claude/conventions/go-errors.md`.

`validation.SetupCustomValidator()` registra os validadores BR no engine do Gin
— chamar uma vez no bootstrap.

## Servidor HTTP e métricas

```go
srv := httpserver.New(logger, httpserver.Options{
    ServiceName: "meu-servico", Port: "8080", BasePath: "/meu-servico",
    MaxBodyBytes: 1 << 20, Swagger: true,
}, func(api *gin.RouterGroup) {
    // rotas do serviço, já dentro de /meu-servico/api/v1
})
```

- `/meu-servico/api/v1/health` e, com `Swagger`, `/meu-servico/swagger/*`.
- `/metrics` na raiz, fora do `BasePath` — o Prometheus raspa pela rede interna; o Kong não roteia.
- Worker sem HTTP de negócio: `metrics.NewServer(port)` sobe só o `/metrics`. Laços periódicos medem cada rodada com `metrics.Observe("nome_do_job", fn)` e contam falhas parciais com `metrics.Failure`.

## Versionamento e release

SemVer por tag Git. Cada serviço fixa uma versão no `go.mod` — nunca a branch.
O `go.work` do `chameleon-stack` liga o código local para desenvolvimento, mas
o Docker não usa o workspace: **mudança aqui só chega aos serviços com tag
publicada**.

```bash
git tag -a v0.41.0 -m "..." && git push origin master v0.41.0
# em cada serviço:
GOPROXY=direct go get github.com/felipedenardo/chameleon-common@v0.41.0 && go mod tidy
```

`.github/workflows/ci.yml` roda lint, `govulncheck`, testes e build em todo pull
request.
