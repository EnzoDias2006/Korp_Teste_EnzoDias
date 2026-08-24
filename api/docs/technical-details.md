# Backend technical details

This document is ready to accompany the backend when the repository contents are copied to `Korp_Teste_SeuNome/api`.

## Stack

| Concern | Implemented choice | Role |
|---|---|---|
| Language | Go 1.26 | Explicit services, concurrency-safe standard library and small binaries |
| HTTP server | Gin 1.12 | Routing, middleware and JSON transport |
| Database | PostgreSQL 16 | Persistence, constraints, sequences, transactions and row locks |
| Driver/pool | `pgx/v5` + `pgxpool` 5.10 | Context-aware PostgreSQL access and pooling |
| Service HTTP client | standard `net/http` | Billing-to-Stock calls with context, timeout and defensive response handling |
| Logging | standard `log/slog` | Structured JSON logs |
| Migrations | golang-migrate container 4.17 | Versioned SQL applied outside application startup |
| Local infrastructure | Docker Compose | PostgreSQL, services, networks and health ordering |
| Quality | `gofmt`, `go vet`, `testing`, Lefthook and GitHub Actions | Repeatable formatting, tests, build and CI |

## Go dependency management

Stock and Billing are independent modules:

```text
services/stock/go.mod
services/billing/go.mod
```

Each module declares only Gin and pgx as direct runtime dependencies. Root `go.work` references both modules so repository tooling can operate from one checkout; it does not merge dependency ownership or create a runtime coupling.

Dependency cleanup must be run in both modules because the repository root intentionally has no `go.mod`:

```bash
(cd services/stock && go mod tidy)
(cd services/billing && go mod tidy)
git diff -- services/stock/go.mod services/stock/go.sum \
  services/billing/go.mod services/billing/go.sum
```

No ORM is used because the invariants depend on visible conditional updates, row locks, affected-row counts and transaction boundaries. pgx and explicit parameterized SQL expose those details directly. Resty would duplicate `net/http` for a small client. Zap/Logrus would duplicate `log/slog`. Avoiding those dependencies keeps ownership and failure semantics easier to inspect.

## HTTP layer and error handling

Gin is limited to transport concerns. Handlers:

1. parse paths and strictly decode JSON;
2. reject wrong content types, unknown fields, multiple JSON values and oversized bodies;
3. validate transport semantics;
4. call Product or Invoice logic;
5. map known domain/dependency errors to stable status/code pairs;
6. log unexpected failures and return a safe response DTO.

Handlers do not execute SQL or begin transactions. Panic recovery is middleware-owned. Client responses never contain SQL, connection strings, passwords or stack traces.

Errors use a nested envelope:

```json
{
  "error": {
    "code": "STOCK_SERVICE_UNAVAILABLE",
    "message": "Could not update product stock.",
    "details": null,
    "request_id": "demo-request-1"
  }
}
```

Transport errors (`400`), missing resources (`404`), business conflicts (`409`), semantic validation (`422`), dependency unavailability (`503`) and unexpected errors (`500`) remain distinguishable. Request IDs are returned in `X-Request-ID`, included in error bodies and propagated Billing-to-Stock.

## Structured logging

Both services emit JSON logs through `slog`. Operation failures include stable fields where applicable, including `service`, `request_id`, `operation`, `invoice_id`, `product_id`, `operation_id` and `error`. Configuration errors are secret-safe and normal startup logs do not print database URLs. Request IDs provide correlation only; durable operation IDs provide idempotency.

## Service and database ownership

Stock owns `stock_db`, Product rows, current balance and stock idempotency. Billing owns `billing_db`, Invoice rows, snapshots, status and finalization state. Their database roles are separate, and neither service receives the other's connection string.

Billing resolves or consumes Stock only over HTTP. This prevents accidental cross-database joins and makes the required unavailable-service behavior observable. A single HTTP operation can never atomically commit both databases, so the design uses local transactions plus durable idempotency instead of pretending there is a distributed transaction.

## PostgreSQL constraints and transactions

Product guarantees include:

- unique product `code`;
- `CHECK (balance >= 0)`;
- conditional decrement with `WHERE balance >= quantity`;
- row locks ordered by product ID for multi-item consumption.

Invoice guarantees include:

- identity-backed unique sequential `number`;
- status limited to `OPEN` or `CLOSED`;
- positive item product IDs and quantities;
- one product per invoice;
- nonblank historical snapshots;
- one finalization row per invoice and unique operation ID.

Creating an invoice and all items is one Billing transaction. Consuming all Stock items, recording the operation, and storing its results is one Stock transaction. Closing an invoice and setting the matching finalization's `completed_at` is one Billing transaction. No transaction is held open across the Billing-to-Stock HTTP call.

## Concurrency

For a multi-product command, Stock sorts IDs and locks matching Product rows with `SELECT ... ORDER BY id FOR UPDATE`. It verifies every row before conditional updates. Any missing row, insufficient balance or write failure rolls the transaction back.

For balance `1`, two distinct operations consuming `1` serialize on the Product row. One conditional update succeeds; the other observes insufficient stock. The final balance is `0`, never negative. Database locks and constraints work across goroutines, processes and service replicas; an in-memory mutex would not.

Billing locks an invoice while claiming/reusing its finalization identity, and the final close is conditional on `status = 'OPEN'`. Concurrent duplicate print requests therefore cannot both perform the one winning close transition.

## Durable idempotency

Billing creates one UUID in `invoice_finalizations.operation_id` for the logical print and reuses it while the invoice remains `OPEN`. Stock computes a canonical SHA-256 fingerprint from:

- invoice ID;
- product IDs sorted ascending;
- the quantity paired with every product.

`consumption_operations.operation_id` is unique. The first request claims it in the same transaction as all decrements. `consumption_operation_results` stores the exact post-consumption balance for every product in that response.

On retry:

- same operation ID + same fingerprint returns the stored original result without updating Product rows;
- same operation ID + different fingerprint returns `409 IDEMPOTENCY_CONFLICT`;
- a partial/missing stored result is treated as an internal failure, never reconstructed from mutable current balances.

Persisting the result matters because a replay may happen after another valid consumption. Returning current balance would change the established response and could hide an incomplete prior outcome.

## Billing-to-Stock client

Billing uses a dedicated `http.Client` with a five-second total timeout, bounded idle connections and redirects disabled. Every request is created with the incoming context, sends JSON content negotiation, propagates `X-Request-ID`, and closes every obtained response body.

Responses are limited to 1 MiB. Billing validates JSON content type, exact JSON decoding, expected status, stable error envelope and DTO completeness. Original 3xx responses remain at the client boundary and are rejected as unavailable instead of being followed to a potentially unrelated success response. For consume success it requires exactly one non-negative balance for each requested product. Network errors, timeouts, Stock 5xx, malformed bodies, unexpected status or incomplete data become `ErrUnavailable` and map to `503`.

Billing deliberately does not retry automatically. Once the request outcome may be ambiguous, only a retry using the already persisted operation ID is safe; the API leaves that explicit retry to the caller.

## Lost-response recovery

The critical window is:

```text
Billing -> Stock: consume(operation A)
Stock: decrement + operation A + stored results commit
Billing <- response is lost
Billing invoice remains OPEN
```

The client receives `503 STOCK_SERVICE_UNAVAILABLE`; Billing does not infer whether Stock committed. A later print reuses operation A. Stock finds the same fingerprint and replays the stored success without a second decrement. Billing then atomically closes the invoice and records completion.

Repeating print after the invoice is `CLOSED` returns `409 INVOICE_NOT_OPEN` before Stock is called. This is a lifecycle guard, while the Stock operation record is the guarantee that covers the ambiguous window before Billing closes.

## Operations and delivery

Application binaries validate required configuration and ping their owned databases before serving. Health endpoints expose process liveness and owned-database readiness. Compose starts PostgreSQL, then Stock, then Billing using health conditions rather than sleeps. Both runtime images execute as non-root users.

Migrations remain versioned SQL and are applied explicitly with `make migrate-up`; service startup never mutates schema. CI creates a clean PostgreSQL volume, applies both service migrations, runs formatting/vet/tests/build, builds both Docker images and always removes its disposable infrastructure.

The complete local commands and destructive-data warnings are in [BOOTSTRAP.md](./BOOTSTRAP.md). The exact route DTOs and codes are in [HTTP_CONTRACT.md](./HTTP_CONTRACT.md).
