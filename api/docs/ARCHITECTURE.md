# Architecture

## System boundary

```text
Angular Web
  ├──HTTP──> Stock Service ──pgxpool──> stock_db
  └──HTTP──> Billing Service ──pgxpool──> billing_db
                    └──net/http──> Stock Service
```

Billing-to-Stock is the only service dependency. Cross-service information moves over HTTP. There are no cross-database queries, shared persistence packages or distributed transactions.

## Ownership

Stock owns:

- product code, description and current balance;
- product validation and uniqueness;
- read-only product resolution for Billing snapshots;
- atomic multi-product consumption;
- product row locking and non-negative balance enforcement;
- durable consumption operation fingerprints and stored replay results.

Billing owns:

- invoice number, status and timestamps;
- invoice items and immutable product code/description snapshots;
- the `OPEN -> CLOSED` lifecycle;
- one durable finalization operation per invoice;
- orchestration and HTTP communication with Stock.

Stock does not know invoice state. Billing never reads or writes Stock tables. The PostgreSQL instance is shared only as local infrastructure:

```text
PostgreSQL 16
├── stock_db   <- stock_user
└── billing_db <- billing_user
```

The initialization script creates separate roles/databases and revokes public connection privileges. Compose supplies each container only its owned database URL.

## Modules and package direction

`services/stock` and `services/billing` each have a `go.mod`. Root `go.work` makes both modules available to local workspace commands without turning them into one deployable unit or shared domain module.

Within each service:

- `cmd/api` validates configuration, wires concrete dependencies and owns server lifecycle;
- `internal/config` parses environment values;
- `internal/database` creates and closes `pgxpool`;
- `internal/http` owns Gin routes, middleware and transport DTOs;
- `internal/product` owns Stock business rules and explicit SQL persistence;
- `internal/invoice` owns Billing business rules, transaction boundaries and explicit SQL persistence;
- Billing `internal/stock` is the consumer-defined `net/http` boundary to Stock;
- `migrations` is owned only by its service.

Gin handlers bind and validate transport input, invoke business operations, map known failures, log unexpected failures and render DTOs. They contain neither SQL nor transaction orchestration.

## Runtime and cross-cutting behavior

Each executable validates configuration, opens and pings its own pool, starts an `http.Server`, and performs bounded graceful shutdown after `SIGINT`/`SIGTERM`.

Both services expose `GET /health/live` and `GET /health/ready`. Readiness has a two-second database ping deadline and returns `503 DATABASE_UNAVAILABLE` when the owned database cannot be used.

Middleware order is request ID, CORS, then panic recovery. `X-Request-ID` is accepted or generated, returned to the caller and propagated by Billing to Stock. It correlates logs and errors but is never an idempotency key. CORS accepts only configured explicit HTTP(S) origins, and allowed preflights return `204`.

Errors use the nested envelope documented in [HTTP_CONTRACT.md](./HTTP_CONTRACT.md). Expected failures are intentionally classified; unexpected failures are logged through `slog` and exposed only as safe messages.

## Invoice creation

```text
POST /api/v1/invoices
  -> validate JSON and item semantics
  -> one POST /internal/v1/products/resolve
  -> validate the complete Stock response
  -> begin Billing transaction
  -> insert OPEN invoice plus trusted snapshots
  -> commit and return 201
```

The Stock call is outside the Billing transaction. Resolution is read-only and never reserves or consumes balance. Billing persists `product_code_snapshot` and `product_description_snapshot`, so historical invoices do not depend on later Stock mutations.

Invoice numbers use PostgreSQL `GENERATED ALWAYS AS IDENTITY` plus a unique constraint. Concurrent creates therefore receive unique increasing values without unsafe `MAX(number) + 1`. Gaps after rolled-back transactions are allowed.

## Atomic stock consumption

Stock handles the entire command inside one local transaction:

1. insert the durable operation claim;
2. on an existing operation, compare its fingerprint and load its stored results;
3. sort product IDs and lock all existing rows with `FOR UPDATE` in that order;
4. verify that all products exist;
5. conditionally decrement each row with `balance >= quantity`;
6. persist every resulting balance in `consumption_operation_results`;
7. commit all changes together.

Missing or insufficient stock rolls back the claim and every decrement. The database `CHECK (balance >= 0)` remains the final invariant. Deterministic lock order reduces deadlock risk, and PostgreSQL serialization—not an in-memory mutex—controls competing consumers.

## Finalization and recovery

```text
POST /api/v1/invoices/:id/print
  -> verify invoice is OPEN and load persisted quantities
  -> lock invoice and claim/reuse invoice_finalizations.operation_id
  -> commit Billing claim
  -> POST /internal/v1/stock/consume with that operation_id
  -> receive and validate a complete Stock result
  -> Billing transaction:
       conditional OPEN -> CLOSED + closed_at
       invoice_finalizations.completed_at
       load response items
     commit
```

No Billing transaction stays open during the HTTP call. Billing closes only after a usable Stock confirmation. Closing the invoice and marking its matching operation complete occur in the same Billing transaction; a failure loading the response or completing the operation rolls the close back.

Stock fingerprints the invoice ID and sorted product/quantity pairs with SHA-256. The operation claim, decrements and resulting balances commit together. A later identical request returns those stored original balances even if another command has since changed current stock. A different payload under the same operation ID returns `409 IDEMPOTENCY_CONFLICT`.

If Stock is unreachable, times out, returns 5xx, wrong content type, malformed/oversized JSON or an incomplete balance set, Billing returns `503 STOCK_SERVICE_UNAVAILABLE` and leaves the invoice `OPEN`. If Stock committed but its response was lost, retry reuses the durable operation ID; Stock replays the stored success without another decrement, then Billing can close safely. Billing performs no blind automatic retry.

## Deliberate exclusions

There is no ORM, sqlx, Resty, external logging framework, Redis, broker, two-phase commit, shared database, DI container or generic application framework. The challenge needs explicit ownership, PostgreSQL guarantees and one recoverable HTTP workflow rather than additional infrastructure.
