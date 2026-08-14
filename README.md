# checkout-go

Serviço de checkout em Go com suporte a **processamento tradicional** e **orquestração via padrão Saga**, garantindo consistência distribuída, idempotência e compensação de operações em caso de falha.

## Visão Geral

O serviço expõe uma API REST para processar pedidos de checkout. Cada requisição passa por validação de idempotência e, em seguida, é processada por um pipeline de etapas. Quando o processamento usa o **padrão Saga**, cada etapa é executada com uma contrapartida de compensação, permitindo reverter operações já concluídas caso uma etapa posterior falhe.

## Arquitetura

```
┌─────────────┐     ┌──────────────┐     ┌───────────────┐     ┌──────────────┐
│   Handler   │ ──▶ │    Service   │ ──▶ │ SagaCoordinator│ ──▶ │  Repository  │
│  (HTTP/Gin) │     │ (orquestra)  │     │ (coordenação) │     │    (GORM)    │
└─────────────┘     └──────────────┘     └───────────────┘     └──────────────┘
                          │  │                                        │
                          ▼  ▼                                        ▼
                     ┌──────────────┐                          ┌──────────────┐
                     │    Redis     │                          │   SQLite     │
                     │ (idempotência)│                         │ (checkouts)  │
                     └──────────────┘                          └──────────────┘
```

### Camadas

| Camada | Diretório | Responsabilidade |
|--------|-----------|------------------|
| Transporte | `internal/handlers` | Recebe HTTP, valida body, responde JSON |
| Middleware | `internal/middleware` | Valida e injeta o `Idempotency-Key` |
| Serviço | `internal/service` | Regras de negócio, orquestra o processamento |
| Saga | `internal/saga` | Coordenador e steps (execução + compensação) |
| Repositório | `internal/repository` | Persistência via GORM |
| Modelos | `internal/models` | Entidades e DTOs |

## Fluxo Sem Saga (Processamento Tradicional)

No modo tradicional, o checkout é processado de forma **sequencial e síncrona** sem mecanismo de compensação. Se qualquer etapa falhar, o fluxo é abortado e os dados já gravados permanecem.

```mermaid
sequenceDiagram
    autonumber
    participant Client as Cliente
    participant Handler as Handler HTTP
    participant Service as CheckoutService
    participant Repo as Repository
    participant DB as SQLite

    Client->>Handler: POST /api/v1/checkout/process
    Handler->>Handler: Middleware valida Idempotency-Key
    Handler->>Service: ProcessCheckout(req)

    Service->>Service: 1. Verifica cache (Redis)
    alt Cache hit
        Service-->>Handler: Resposta idempotente (cached)
    else Cache miss
        Service->>Repo: FindByIdempotencyKey(key)
        alt Pedido já processado
            Service-->>Handler: Resposta idempotente (DB)
        else Pedido novo
            Service->>Service: 2. Gera checkoutID + sagaID
            Service->>Service: 3. Calcula total dos itens
            Service->>Repo: Create(checkout)
            Repo->>DB: INSERT checkouts
            Service->>Service: 4. Grava resposta no cache
            Service-->>Handler: 200 OK (response)
        end
    end

    Handler-->>Client: JSON { id, status, total, saga_id }
```

### Passos (Sem Saga)

| # | Passo | Descrição |
|---|-------|-----------|
| 1 | **Idempotência** | Verifica cache Redis; se presente, retorna resposta armazenada |
| 2 | **Busca no DB** | Procura checkout pelo `idempotency_key`; se existir, reutiliza |
| 3 | **Geração de IDs** | Cria `checkoutID` (UUID) e `sagaID` (UUID) |
| 4 | **Cálculo do total** | Soma `price × quantity` de todos os itens |
| 5 | **Persistência** | Grava **uma única linha** com os itens em JSON |
| 6 | **Cache** | Armazena a resposta no Redis com TTL de 24h |

> **Característica-chave**: persistência de **1 pedido = 1 linha**, com `items` serializados como JSON — evita violação de constraint `UNIQUE` em `id_external`.

## Fluxo Com Saga (Orquestrado)

O padrão **Saga** coordena múltiplas etapas distribuídas. Cada step possui:

- `Execute(ctx, data)` — ação principal
- `Compensate(ctx, data)` — reversão da ação em caso de falha

Se a etapa `N` falhar, o coordenador executa `Compensate` de todas as etapas anteriores **em ordem reversa**, garantindo consistência.

```mermaid
sequenceDiagram
    autonumber
    participant Client as Cliente
    participant Service as CheckoutService
    participant Coord as SagaCoordinator
    participant Step1 as StepValidateStock
    participant Step2 as StepProcessPayment
    participant Step3 as StepSaveCheckout
    participant Repo as Repository
    participant DB as SQLite

    Client->>Service: ProcessCheckout(req)
    Service->>Coord: ExecuteSaga(steps, sagaData)
    Note over Coord: registra saga como ACTIVE

    rect rgb(230, 245, 255)
        Note over Coord,Step1: STEP 1/3 — validate_stock
        Coord->>Step1: Execute()
        Step1-->>Coord: valida estoque de cada item
    end

    rect rgb(230, 255, 235)
        Note over Coord,Step2: STEP 2/3 — process_payment
        Coord->>Step2: Execute()
        Step2-->>Coord: payment_id, amount_paid, status=approved
    end

    rect rgb(255, 250, 230)
        Note over Coord,Step3: STEP 3/3 — save_checkout
        Coord->>Step3: Execute()
        Step3->>Repo: Create(checkout)
        Repo->>DB: INSERT checkouts (items JSON)
        Step3-->>Coord: checkout_saved
    end

    alt Todas as etapas OK
        Coord->>Coord: move saga para COMPLETED
        Coord-->>Service: nil (sucesso)
        Service-->>Client: 200 OK
    else Etapa falhou
        Coord->>Coord: compensa etapas anteriores (ordem reversa)
        Note over Coord: Step2.Compensate → reversão do pagamento<br/>Step1.Compensate → liberação do estoque
        Coord->>Coord: move saga para FAILED
        Coord-->>Service: erro
        Service-->>Client: 500 Internal Server Error
    end
```

### Steps do Saga

| Ordem | Step | Execute | Compensate |
|-------|------|---------|------------|
| 1 | **validate_stock** | Verifica `stock >= quantity` para cada item | Libera o estoque reservado |
| 2 | **process_payment** | Calcula total, gera `payment_id`, aprova pagamento | Reverte o pagamento (`reverse_status`) |
| 3 | **save_checkout** | Grava checkout no banco (1 linha, items JSON) | Marca checkout como removido |

### Estado do Saga

O `SagaCoordinator` mantém três mapas de estado:

| Mapa | Estado | Significado |
|------|--------|-------------|
| `activeSagas` | `processing` | Saga em execução |
| `completedSagas` | `completed` | Todas as etapas concluídas |
| `failedSagas` | `failed` | Alguma etapa falhou, compensações executadas |

> O estado fica **em memória**. Em produção, o estado do saga deve ser persistido (ex.: tabela própria, event sourcing) para recuperação entre instâncias.

## API

### `POST /api/v1/checkout/process`

Processa um checkout.

**Headers**

| Header | Obrigatório | Descrição |
|--------|:-----------:|-----------|
| `Idempotency-Key` | Sim | Chave de idempotência (letras, números e hífens) |

**Request Body**

```json
{
  "user_id": "user-123",
  "items": [
    { "product_id": "prod-001", "quantity": 2, "price": 29.90, "stock": 10 },
    { "product_id": "prod-002", "quantity": 1, "price": 59.90, "stock": 5 }
  ]
}
```

**Response 200**

```json
{
  "id": "a7fbd198-c8cf-4afd-ad1b-c522d0f60ccb",
  "status": "processed",
  "total": 119.70,
  "processe_in": "2026-08-14T14:55:37.515Z",
  "idempotency_key": "teste-123-abc",
  "saga_id": "e8d1e3aa-9b7f-4c8e-9f2a-1b2c3d4e5f60"
}
```

### `GET /api/v1/checkout/:id`

Busca checkout pelo `id_external`.

### `GET /api/v1/saga/:saga_id/status`

Consulta o estado de um saga (`processing`, `completed` ou `failed`).

```json
{ "saga_id": "e8d1e3aa-...", "status": "completed", "step": 3 }
```

### `GET /api/v1/health`

Health check.

```json
{ "status": "ok", "message": "service of checkout is running" }
```

## Idempotência

A idempotência é garantida em **três camadas**:

1. **Redis** — chave `idempotency:<key>` com resposta em cache (TTL 24h)
2. **Banco de dados** — lookup por `idempotency_key` (única por pedido)
3. **Constraint** — `uniqueIndex` em `id_external` e `idempotency_key`

```mermaid
flowchart TD
    A[POST /checkout/process] --> B{Chave no Redis?}
    B -- Sim --> C[Retorna resposta cached]
    B -- Não --> D{Busca no DB por idempotency_key?}
    D -- Sim --> E[Retorna resposta existente]
    D -- Não --> F[Processa checkout]
    F --> G[Grava no DB]
    G --> H[Grava no Redis]
```

## Erros

Todos os erros retornam mensagens em **inglês**.

| Status | Cenário |
|--------|---------|
| `400` | Body inválido ou `Idempotency-Key` ausente/formatada incorretamente |
| `404` | Checkout ou saga não encontrado |
| `500` | Falha no processamento (ex.: estoque insuficiente, erro no saga) |

## Como Executar

### Docker Compose

```bash
docker compose up --build
```

### Local

```bash
# Redis rodando em localhost:6379
go run main.go
```

Serviço disponível em `http://localhost:8082`.

> **Atenção**: se a estrutura do banco mudou (ex.: adição da coluna `items`), apague o arquivo `checkout.db` antes de subir para evitar dados legados.

## Testes

```bash
go test ./...
```

Cobertura atual:

- Repositório: lookup por `idempotency_key`
- Serviço: pedido multi-item cria **1 única linha**
- Serviço: cache corrompido cai no fallback do DB
- Serviço: idempotência (retry retorna o mesmo `id`)

## Tecnologias

- [Go](https://go.dev) 1.26
- [Gin](https://gin-gonic.com) — HTTP framework
- [GORM](https://gorm.io) + SQLite — persistência
- [Redis](https://redis.io) — cache de idempotência
- [go-redis](https://github.com/redis/go-redis) — client Redis
- [google/uuid](https://github.com/google/uuid) — geração de IDs
