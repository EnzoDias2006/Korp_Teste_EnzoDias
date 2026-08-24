# AGENTS.md — Go Backend

## 1. Scope and Precedence

This file applies to every file under `api/`, including both backend microservices.

The repository-root `../AGENTS.md` remains the project-wide source of truth. This file narrows those rules to the Go backend and adds backend-specific guidance. If the two files appear to conflict, stop and surface the conflict before changing architecture, service ownership, public API contracts, persistence, or locked technologies.

The challenge specification remains the primary product source of truth.

---

## 2. Backend Mission

Build two small, production-minded Go microservices that persist products and invoices and make invoice finalization recoverable, concurrency-safe, and idempotent.

Prioritize, in order:

1. business and data correctness;
2. strict service and database ownership;
3. recoverable distributed failures;
4. database-enforced concurrency guarantees;
5. idempotent side effects;
6. idiomatic, explicit Go;
7. predictable HTTP errors;
8. focused tests at high-risk boundaries;
9. minimal dependencies and easy local evaluation.

This is a technical challenge, not a production ERP. Do not add enterprise infrastructure for appearance.

---

## 3. Locked Backend Stack

Do not replace or bypass these choices without explicit project-owner approval:

- Go 1.26;
- Gin as the HTTP layer;
- PostgreSQL;
- `github.com/jackc/pgx/v5`;
- `pgxpool`;
- standard library `net/http` for service-to-service calls;
- standard library `log/slog` for structured logging;
- `golang-migrate` for SQL migrations;
- separate Go modules for Stock and Billing;
- root `go.work` referencing both modules;
- Docker Compose for local infrastructure;
- Lefthook;
- `gofmt`, `go vet`, and Go's standard `testing` package.

Do not introduce:

- GORM or another ORM;
- sqlx;
- Resty;
- Zap or Logrus;
- a DI container;
- repository/application frameworks;
- generic frameworks layered on Gin;
- Redis;
- Kafka, RabbitMQ, or another message broker;
- two-phase commit;
- Kubernetes, service mesh, tracing stack, or API gateway products;
- AI functionality.

Before proposing any new dependency, answer all four questions:

1. Which concrete requirement does it solve?
2. Why are the standard library, Gin, pgx, PostgreSQL, or an installed package insufficient?
3. Does it make the final technical explanation harder?
4. Does it overlap an existing responsibility?

If the case is not compelling, do not add it. Stop for approval before adding any runtime dependency.

---

## 4. Intended `api/` Layout

Keep Stock and Billing as independent Go modules:

```text
api/
├── stock/
│   ├── cmd/
│   │   └── api/
│   │       └── main.go
│   ├── internal/
│   │   ├── product/
│   │   ├── http/
│   │   └── database/
│   ├── migrations/
│   └── go.mod
└── billing/
    ├── cmd/
    │   └── api/
    │       └── main.go
    ├── internal/
    │   ├── invoice/
    │   ├── stock/
    │   ├── http/
    │   └── database/
    ├── migrations/
    └── go.mod
```

The repository-root `go.work` should reference:

```go
use (
    ./api/stock
    ./api/billing
)
```

Do not merge the services into one module for convenience. Do not create layers or packages merely to mirror a Clean Architecture diagram. Add structure only when it improves ownership, cohesion, testing, or readability.

---

## 5. Service Boundaries

System flow:

```text
Angular Web
  ├──> Stock Service
  └──> Billing Service
              └──> Stock Service
```

### Stock Service owns

- products;
- unique product codes and descriptions;
- current stock balances;
- stock validation;
- atomic stock decrement;
- concurrency protection around stock;
- durable stock-side idempotency records where required.

Stock Service must not own:

- invoices;
- invoice status or lifecycle;
- invoice numbering;
- Billing Service tables.

### Billing Service owns

- invoices;
- sequential invoice numbering;
- invoice status;
- invoice items and historical snapshots;
- finalization orchestration;
- durable finalization state where required;
- communication with Stock Service.

Billing Service must not:

- read or write Stock Service tables;
- update stock directly;
- infer stock success after an HTTP failure;
- mark an invoice `CLOSED` before Stock Service confirms the idempotent stock operation.

Cross-service information flows only over HTTP. Never share tables, database credentials, or persistence packages between services.

---

## 6. Database Ownership and Schema

Use one PostgreSQL server locally with logically separate databases:

```text
PostgreSQL instance
├── stock_db
└── billing_db
```

Rules:

- Stock Service connects only to `stock_db`;
- Billing Service connects only to `billing_db`;
- no cross-database reads or writes;
- every schema change has a committed migration;
- startup and tests must not rely on manually created schema state;
- database constraints enforce invariants where appropriate.

At minimum, Stock data needs:

- internal product ID;
- unique code;
- description;
- non-negative balance;
- timestamps when useful;
- durable idempotency data for applied stock commands.

At minimum, Billing data needs:

- internal invoice ID;
- unique sequential invoice number;
- status;
- creation timestamp;
- closing timestamp when useful;
- invoice items with product identity, quantity, and the snapshot needed for historical rendering;
- durable operation identity/state needed for recoverable finalization.

Do not make historical invoices depend on mutable Stock Service descriptions.

---

## 7. Idiomatic Go Rules

Prefer:

- small, explicit types;
- concrete dependencies;
- constructors only when they clarify required dependencies or invariants;
- `context.Context` propagation;
- returned and wrapped errors;
- early returns;
- standard library functionality;
- explicit SQL;
- narrow interfaces defined at the consumer only when they improve testing or design.

Avoid Java/C# patterns translated mechanically into Go.

Do not create:

- `IProductRepository`-style interfaces by convention;
- `BaseService` or inheritance-like abstractions;
- generic service/repository layers;
- factories for trivial constructors;
- a dependency injection container;
- an interface with one implementation and no testing or boundary benefit;
- `Helper`, `Util`, `Manager`, `Processor`, or `Common` dumping grounds.

Keep `main.go` focused on configuration, dependency wiring, server lifecycle, and graceful shutdown. Keep business logic out of `main.go` and Gin handlers.

Use English for packages, identifiers, API paths, database schema, tests, logs, commits when reasonable, and technical documentation.

---

## 8. Gin HTTP Layer

Handlers should:

1. parse path/query parameters and bind JSON;
2. validate transport input;
3. call domain/application logic;
4. translate known errors to the shared HTTP error contract;
5. log unexpected errors safely;
6. return the response with the intended status code.

Handlers must not:

- contain raw SQL;
- orchestrate database transactions directly;
- leak pgx/database errors;
- return stack traces;
- collapse every failure into `500`;
- silently swallow bind, validation, or encoding errors.

Use JSON and clear REST-oriented nouns. Preferred public namespaces are:

```text
/products
/invoices
```

An explicit internal stock action endpoint is acceptable when finalization semantics require a command rather than CRUD-shaped behavior.

Do not silently change an established public API contract. Stop for approval first.

---

## 9. HTTP Status and Error Contract

Use meaningful status codes:

- `200 OK` for a successful read or action with a body;
- `201 Created` for resource creation;
- `204 No Content` for a successful command with no body;
- `400 Bad Request` for malformed transport input;
- `404 Not Found` for a missing resource;
- `409 Conflict` for domain conflicts such as insufficient stock, invalid state, or idempotency conflict;
- `422 Unprocessable Entity` for valid JSON with invalid domain input when appropriate;
- `503 Service Unavailable` when Billing cannot reach or obtain a usable response from Stock Service;
- `500 Internal Server Error` only for unexpected server failures.

All APIs must converge on this predictable shape:

```json
{
  "code": "STOCK_SERVICE_UNAVAILABLE",
  "message": "Could not update product stock.",
  "details": null
}
```

Rules:

- `code` is stable and machine-readable;
- `message` is safe for presentation or translation;
- `details` is optional and safe;
- internal SQL, stack traces, credentials, and network details never reach the client;
- transport errors, business rejection, dependency unavailability, and unexpected failures remain distinguishable.

Centralize error rendering only as far as it remains explicit and easy to follow.

---

## 10. PostgreSQL and Transactions

Use `pgx/v5` and `pgxpool`. Prefer explicit SQL.

Transactions belong around operations that must be atomic. The code that defines the atomic application operation should define the transaction boundary; handlers must not coordinate it.

Use database constraints and conditional writes as the final authority. Check and translate:

- `pgx.ErrNoRows`;
- unique constraint violations;
- transaction failures;
- context cancellation and deadline expiry;
- affected-row counts for conditional writes.

Never construct SQL by interpolating user-controlled values. Use parameters.

Do not hold a database transaction open across a service-to-service HTTP call unless a documented design explicitly justifies the consequences. There is no distributed transaction between databases.

---

## 11. Product and Stock Rules

Product registration includes:

- code;
- description/name;
- current stock balance.

Validate input at the HTTP boundary and enforce authoritative invariants in the service/database. Product code uniqueness and non-negative balance must not depend only on pre-checks in Go.

Stock consumption must be atomic and concurrency-safe. Prefer a conditional update or an appropriately locked transaction, for example:

```sql
UPDATE products
SET balance = balance - $1
WHERE id = $2
  AND balance >= $1
RETURNING balance;
```

For a product with balance `1` and two competing operations consuming `1`, exactly one may succeed and the balance must never become negative.

Do not use an in-memory mutex as the stock guarantee. It fails across processes and instances.

For a multi-product stock command, define atomic semantics explicitly: either all required stock changes apply once or none apply. Lock/order rows deterministically where needed to reduce deadlock risk, and translate insufficient stock intentionally.

---

## 12. Invoice Rules

Invoice registration includes:

- a backend-generated sequential number;
- initial status `OPEN`;
- one or more invoice items;
- a positive quantity per item;
- enough product snapshot data for historical rendering.

The database must enforce number uniqueness. Generate sequential numbers using a PostgreSQL-backed mechanism safe under concurrency; do not calculate `MAX(number) + 1` without a concurrency-safe design.

Validate duplicate product items and invalid quantities explicitly according to the established API contract.

Only an `OPEN` invoice may enter finalization. A successfully finalized invoice becomes `CLOSED`. If a small durable intermediate state such as `FINALIZING` materially closes a correctness gap, it may be introduced and documented; do not add states for aesthetics.

---

## 13. Finalization Workflow

Treat “printing” as the business operation that finalizes the invoice.

Conceptual flow:

```text
Frontend
  |
  | POST finalization/print
  v
Billing Service
  | validate invoice state
  | load item snapshots
  | send durable operation ID
  v
Stock Service
  | atomically validate/decrement once
  v
Billing Service
  | mark invoice CLOSED
  v
Frontend
```

Critical invariant:

> Billing must not mark an invoice `CLOSED` unless the required stock operation succeeded.

Billing must never update the Stock database directly. Stock must never update the Billing database directly. Do not attempt two-phase commit.

Keep the workflow explicit, durable, retryable, and easy to explain.

---

## 14. Service-to-Service HTTP

Billing calls Stock with standard library `net/http`.

Requirements:

- propagate the incoming `context.Context`;
- configure explicit client/request timeouts;
- construct requests with context;
- set and validate JSON content types as appropriate;
- close response bodies on every obtained response;
- limit/handle response bodies defensively;
- treat non-2xx responses intentionally;
- distinguish Stock business rejection from Stock unavailability;
- preserve stable error codes across the Billing boundary where appropriate;
- do not retry blindly.

A timeout, connection failure, malformed response, or unusable Stock response must not be treated as stock success.

Automatic retries are unsafe without an idempotent operation identifier. Prefer returning a recoverable `503` unless the established workflow explicitly makes a bounded retry safe.

Do not add Resty.

---

## 15. Failure Recovery

Stock Service unavailability during invoice finalization is a required, demonstrable behavior:

```text
POST invoice finalization
        |
        v
Billing Service
        |
        X Stock Service unavailable
        |
        v
503 Service Unavailable
```

Required guarantees:

- the client receives a safe, meaningful `503` error;
- the invoice does not falsely become `CLOSED`;
- the workflow remains recoverable;
- after Stock Service returns, retry can complete;
- stock is decremented exactly once.

Do not hide this path as an incidental network error. Test and document it as a first-class feature.

When the outcome of the Stock request is unknown, do not assume it failed before side effects. Retry with the same durable operation identifier so Stock can return the previously applied result without decrementing twice.

---

## 16. Idempotency

Idempotency is required for invoice finalization.

At minimum:

- repeating a completed invoice finalization must not decrement stock again;
- concurrent duplicate requests must not duplicate effects;
- Billing must use a stable operation/idempotency identifier for the logical finalization;
- Stock must durably record whether that operation was applied;
- reuse of an identifier with a different payload must be rejected as an idempotency conflict;
- a retry after a lost response must return the previously established outcome.

The design must handle this failure window:

```text
Billing -> Stock decrement succeeds
Billing <- response is lost or Billing fails before invoice close
Client retries finalization
```

The retry must use the same operation identity and must not decrement stock twice.

Checking only `invoice.status == CLOSED` is not sufficient because Billing may still show `OPEN` after Stock committed. Do not claim idempotency is complete until this window has a durable, tested answer.

Use PostgreSQL uniqueness and transactions to serialize competing commands for the same idempotency key. Do not rely on process memory.

---

## 17. Logging and Request Context

Use `log/slog` for structured logs.

Include relevant fields when available:

```text
service
request_id
invoice_id
product_id
operation
status
error
duration
```

Keep field names consistent. Log enough context to diagnose the required failure flow without logging:

- secrets or credentials;
- complete database connection strings;
- authorization headers;
- sensitive request bodies;
- raw internal errors in client responses.

Propagate a request/correlation ID across Billing-to-Stock calls when the established HTTP contract supports it. Correlation IDs aid diagnosis; they are not a substitute for idempotency keys.

Respect context cancellation in database and HTTP work. Use graceful HTTP server shutdown.

---

## 18. Configuration

Use environment variables for runtime configuration.

Stock Service categories:

```text
HTTP_ADDR
DATABASE_URL
```

Billing Service categories:

```text
HTTP_ADDR
DATABASE_URL
STOCK_SERVICE_URL
```

Add narrowly named timeout settings only when needed and document defaults.

Requirements:

- validate required configuration at startup;
- fail fast with a useful, secret-safe log message;
- never commit secrets;
- provide `.env.example` files for required variables;
- never log the complete database URL;
- keep service configuration independent.

---

## 19. Migrations

Each service owns its migrations:

```text
api/stock/migrations/
├── 000001_create_products.up.sql
└── 000001_create_products.down.sql

api/billing/migrations/
├── 000001_create_invoices.up.sql
└── 000001_create_invoices.down.sql
```

Rules:

- use `golang-migrate`;
- commit every migration;
- name migrations by intent;
- keep ownership within the service;
- make a clean database reproducible without manual SQL;
- apply extra care to destructive down migrations and data changes;
- keep schema constraints aligned with domain invariants.

Do not edit an already-applied migration casually once it is part of shared history; add the next migration unless the project is explicitly still resetting unreleased history.

---

## 20. Backend Testing

Use Go's standard `testing` package and `httptest` for HTTP boundaries.

Prioritize:

- product creation and validation;
- unique product codes;
- invoice creation and validation;
- concurrency-safe sequential numbering;
- invalid invoice state;
- atomic multi-item stock decrement;
- insufficient stock;
- competing stock consumption;
- successful finalization;
- Stock Service business rejection mapping;
- Stock Service unavailable/timeout behavior;
- safe API error mapping;
- idempotent repeated and concurrent requests;
- lost-response retry behavior.

Use integration tests with real PostgreSQL behavior for guarantees that mocks cannot establish, especially:

- transactions;
- unique and check constraints;
- sequences;
- conditional updates;
- row locking and concurrency;
- idempotency records.

Do not mock PostgreSQL behavior that is itself the subject of the test. Keep unit tests focused where a small fake or `httptest.Server` genuinely isolates application or HTTP-client behavior.

Expected module checks:

```bash
go test ./...
go vet ./...
```

Run them from each module or through existing root tooling. Format all touched Go files with `gofmt`. Never claim a check passed unless it was executed.

---

## 21. Docker Compose and Local Evaluation

Repository-level Docker Compose must provide at least the PostgreSQL resources needed by both services, with simple health checks.

The target local experience is:

```bash
docker compose up -d
```

It may also run the application services when that improves demo/evaluation simplicity, but do not introduce production orchestration concerns.

Keep database initialization, health checks, ports, and environment examples easy to understand. Verify migrations from empty databases before delivery.

---

## 22. Change Discipline

Before editing:

1. inspect the existing service and module conventions;
2. confirm service and database ownership;
3. understand the established HTTP/error contract;
4. identify the smallest valid change;
5. reason about transactions, concurrency, and retry windows;
6. avoid unrelated refactors.

After editing:

1. run `gofmt` on touched Go files;
2. run relevant focused tests;
3. run applicable `go test ./...` and `go vet ./...` checks;
4. verify migrations/integration behavior when schema or database guarantees changed;
5. update architecture/technical documentation when behavior changed;
6. report exactly what changed and what was verified;
7. explicitly name checks that were not run.

Do not weaken validation, skip tests, fake persistence, or rewrite unrelated files to make a check pass.

---

## 23. Backend Definition of Done

A backend feature is done only when all applicable items are true:

- the business requirement is implemented;
- transport and domain validation are intentional;
- persistence is real and migration-backed;
- service/database ownership is preserved;
- errors map to the safe shared contract;
- context, timeouts, and response bodies are handled correctly;
- atomicity and concurrency risks are handled by PostgreSQL;
- duplicate effects are prevented across the lost-response failure window;
- important behavior has focused unit/integration tests;
- formatting, tests, and vet pass as applicable;
- no unnecessary dependency was introduced;
- documentation is updated when architecture/API behavior changed;
- the feature is easy to demonstrate in the final video.

Project-level backend acceptance includes:

- persist and retrieve products;
- generate unique sequential invoice numbers;
- create `OPEN` invoices with multiple valid items;
- reject finalization for invalid states;
- atomically decrement stock and close the invoice only after success;
- return `503` and preserve recoverability while Stock Service is unavailable;
- complete exactly once on retry after Stock returns;
- allow only one of two competing consumers when balance is `1`;
- reproduce both schemas from empty databases.

---

## 24. Technical Explanation Readiness

As important decisions are implemented, update `../docs/technical-details.md` and, when appropriate, `../docs/architecture.md` with concise, current notes covering:

- why Gin was chosen and what belongs in handlers;
- how separate Go modules and root `go.work` manage dependencies;
- why pgx and explicit SQL were chosen instead of an ORM;
- how errors are classified and mapped to HTTP;
- how `slog` fields support diagnosis safely;
- how Billing uses `net/http` and timeouts for Stock calls;
- why Stock and Billing have separate ownership/databases;
- how unavailable Stock Service remains recoverable;
- how PostgreSQL enforces concurrency;
- how durable idempotency prevents duplicate stock effects after a lost response.

Do not leave these explanations to memory at the end of the challenge.

---

## 25. Stop Conditions

Stop and ask the project owner before:

- adding a runtime dependency;
- replacing any locked technology;
- changing Stock/Billing service boundaries;
- sharing a database or tables between services;
- introducing another database, Redis, or a message broker;
- changing an established public API contract;
- adding a new invoice state without a demonstrated correctness need;
- implementing automatic cross-service retries without proven idempotency;
- implementing AI;
- making a large unrelated architectural refactor.

Always prefer explicit Go, PostgreSQL guarantees, one clear service boundary, one useful behavioral test, and a recoverable HTTP workflow over generic abstractions or framework accumulation.
