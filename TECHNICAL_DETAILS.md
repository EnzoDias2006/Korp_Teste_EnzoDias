# Consolidated Technical Details

This document describes the implementation frozen for the Korp challenge. It consolidates the
required frontend and backend decisions without claiming behavior outside the published source.

## 1. System boundaries

The solution contains an Angular web application, a Stock microservice, a Billing microservice,
and two PostgreSQL databases.

```text
Angular Web
  ├── product HTTP operations ───────> Stock Service ─────> stock_db
  └── invoice HTTP operations ───────> Billing Service ───> billing_db
                                              │
                                              └───────────> Stock Service
```

Stock owns products, balances, atomic consumption, and consumption idempotency. Billing owns
invoices, item snapshots, sequential numbers, status, and finalization identity. The browser owns
interaction and presentation only. It never implements authoritative stock rules or optimistic
invoice closure.

## 2. Angular architecture

The frontend uses Angular 22 standalone components. `app.config.ts` registers the Router,
`HttpClient`, and the request ID interceptor. There is no application NgModule, `zone.js`, or
zone-based provider; Angular runs zoneless. Route arrays and pages are lazy-loaded with
`loadChildren` and `loadComponent`.

Feature ownership is explicit:

- `core` contains shared API models and the request interceptor;
- `features/products` contains product pages, models, and the focused API service;
- `features/invoices` contains invoice pages, models, and the focused API service;
- application components use `ChangeDetectionStrategy.OnPush`.

No global state library is present. The application does not need NgRx because route pages own
small, local state and the backend remains authoritative.

## 3. Angular lifecycle APIs actually used

Only lifecycle behavior present in the final source is listed:

| Component | Lifecycle behavior and reason |
|---|---|
| `ProductList` | Calls `loadProducts()` from its constructor; it does not implement `OnInit`. |
| `ProductCreate` | Uses no lifecycle hook. |
| `InvoiceList` | `ngOnInit()` loads invoice summaries when the route page initializes. |
| `InvoiceCreate` | `ngOnInit()` creates the first item row and loads selectable products. |
| `InvoiceDetail` | `ngOnInit()` connects the route ID and explicit retry trigger to detail loading. |

Each page that owns a subscription injects `DestroyRef` and uses `takeUntilDestroyed`. No component
implements `OnDestroy` or maintains a manual subscription collection.

## 4. Signals

Signals represent synchronous page state:

- product and invoice collections;
- loading, saving, and printing flags;
- current invoice and safe error state;
- product-load recovery state;
- request references;
- computed decisions such as `hasData` and `canPrint`.

`canPrint` is true only for a backend-confirmed `OPEN` invoice while no print request is in flight.
Signals are updated from HTTP subscriptions and keep templates declarative.

## 5. RxJS

RxJS represents asynchronous HTTP and route flows, not synchronous global state.

| API/operator | Concrete use |
|---|---|
| `HttpClient` Observables | Product and invoice requests. |
| `map` | DTO-to-domain mapping and route parameter conversion. |
| `catchError` | Safe error mapping and intentional recovery. |
| `finalize` | Clear loading, saving, and printing on success or failure. |
| `switchMap` | Replace an obsolete invoice-detail request when route/retry input changes. |
| `combineLatest` | Combine invoice ID and the manual retry trigger. |
| `filter` | Reject unusable route IDs. |
| `tap` | Reset detail state immediately before loading. |
| `BehaviorSubject` | Seed and trigger the explicit detail reload. |
| `takeUntilDestroyed` | Bind subscriptions to the page lifecycle. |

The code avoids nested subscriptions and blind automatic retries. A potentially ambiguous
finalization failure is retried only through a visible user action.

## 6. Typed Reactive Forms

Product creation uses typed controls for code, description, and balance. Required text rejects
whitespace-only values, text is trimmed before transport, and balance must be a non-negative
integer. Saving disables the action and a second in-flight submit is ignored. A recoverable server
error preserves the entered values.

Invoice creation uses a typed `FormArray` whose rows contain `productId` and `quantity`. Quantity
must be a positive integer and a form-level validator rejects duplicate products. The form is not
available until products load successfully. Empty and failed product-loading states provide clear
next actions. Creating an invoice disables the form, prevents duplicate requests, and preserves rows
after failure.

Frontend validation improves feedback; Stock and Billing still validate every authoritative rule.

## 7. Angular Material and other frontend libraries

Angular Material is the visual library. The final source uses toolbar/navigation, cards, form
fields, inputs, selects, buttons, icons, progress spinners, snackbars, tooltips, tables, and status
treatment. Focused SCSS supplies layout, narrow-screen table overflow, responsive form wrapping,
and print media rules. Status and errors are not communicated by color alone.

Other deliberate frontend tools are:

| Tool | Purpose |
|---|---|
| Angular CDK | Material foundation and peer dependency; no application CDK API is imported directly. |
| RxJS | HTTP and route composition, error recovery, finalization, and lifecycle cleanup. |
| Vitest | Unit and component/spec execution through Angular's test builder. |
| TestBed and `HttpTestingController` | Deterministic component and HTTP contract tests. |
| Angular ESLint | TypeScript and template linting, including accessibility rules. |
| Lefthook | Local pre-commit lint and pre-push test/build gates. |
| Prettier | Source and Markdown formatting. |
| Bun | Dependency installation, lockfile resolution, and scripts. |

No third-party form, state, toast, spinner, HTTP wrapper, or CSS framework was added.

## 8. Frontend API and finalization behavior

Focused services are the only frontend classes that call `HttpClient`. They map snake-case transport
fields to explicit domain models. Invoice item code and description snapshots are preserved; the
detail page does not query current Product text to reconstruct history.

Invoice status crosses an untrusted HTTP boundary. The runtime parser accepts only `OPEN` or
`CLOSED`. A print response must be `CLOSED` with a non-empty `closed_at`; otherwise the UI rejects it
and never invokes `window.print()`.

The finalization sequence is:

1. expose the action only for `OPEN`;
2. show `Finalizando...` and disable repeated clicks;
3. post to Billing and wait;
4. validate the confirmed `CLOSED` response;
5. replace local state with that response;
6. invoke browser printing.

On Stock unavailability, processing ends, the invoice stays visibly `OPEN`, a safe message and
request reference are displayed, and retry remains available. `INVOICE_NOT_OPEN` causes an
authoritative detail reload.

## 9. Go frameworks and dependency management

The backend uses Go 1.26. Stock and Billing are independent modules:

```text
api/services/stock/go.mod
api/services/billing/go.mod
```

The root `go.work` makes repository-wide development convenient without merging ownership. Each
module declares Gin and pgx as direct runtime dependencies.

- Gin 1.12 handles routing, middleware, binding, and JSON transport.
- pgx/v5 and pgxpool provide context-aware PostgreSQL access and pooling.
- standard `net/http` implements the small Billing-to-Stock client.
- standard `log/slog` emits structured JSON logs.
- golang-migrate applies versioned SQL outside application startup.

There is no ORM. Explicit SQL makes conditional updates, row locks, affected-row checks, and
transaction boundaries inspectable. Resty, Zap, and Logrus were unnecessary overlaps with the
standard library.

## 10. Backend error and exception handling

Gin handlers remain transport adapters. They strictly decode JSON, reject incorrect content type,
unknown fields, multiple values, oversized bodies, and invalid transport semantics. Domain and
dependency errors are mapped to stable status/code pairs.

The public envelope is:

```json
{
  "error": {
    "code": "STOCK_SERVICE_UNAVAILABLE",
    "message": "Could not update product stock.",
    "details": null,
    "request_id": "request-reference"
  }
}
```

The API distinguishes bad transport (`400`), missing resources (`404`), conflicts (`409`), semantic
validation (`422`), dependency unavailability (`503`), and unexpected failures (`500`). Panic
recovery is middleware-owned. Unexpected errors are logged with safe structured fields, but client
responses never reveal SQL, connection strings, passwords, or stack traces.

The frontend treats `code` as the machine contract and chooses fixed Portuguese messages. It never
renders arbitrary backend or transport text. Malformed responses fall back conservatively; status
zero and `503` map to Stock unavailable when no known nested code is available.

## 11. Transactions and concurrency

Product codes are unique and balances have a non-negative database check. Multi-product consumption
sorts product IDs and locks rows with `SELECT ... ORDER BY id FOR UPDATE`. Every row is validated
before conditional updates. Missing products, insufficient balance, or any write failure rolls back
the entire Stock transaction.

Invoice number is backed by a unique identity sequence. Status is constrained to `OPEN` or
`CLOSED`, item quantities are positive, one product may appear only once, and snapshots are
nonblank. Invoice creation and all items commit in one Billing transaction.

For two distinct operations against balance one, the Product row lock serializes access. One
conditional decrement succeeds; the other observes insufficient stock. Final balance is zero,
never negative. The database guarantee works across goroutines, processes, and replicas.

## 12. Durable idempotency and lost-response recovery

Billing persists one UUID operation for the logical finalization and reuses it while the invoice is
open. Stock computes a canonical SHA-256 fingerprint from invoice ID plus sorted product IDs and
their quantities.

In one Stock transaction, the first request claims the unique operation ID, decrements every item,
and stores the exact response balance for each product. A retry behaves as follows:

- same operation ID and fingerprint: replay the stored original result without another decrement;
- same operation ID and different fingerprint: return `409 IDEMPOTENCY_CONFLICT`;
- incomplete stored result: fail safely instead of reconstructing from mutable balances.

This covers the critical window in which Stock commits but Billing loses the response. Billing keeps
the invoice open. Manual retry reuses the same operation; Stock replays the committed result and
Billing can then close the invoice exactly once. No transaction remains open across the service HTTP
call.

## 13. Billing-to-Stock HTTP hardening

Billing uses a dedicated `http.Client` with a five-second total timeout, bounded idle connections,
redirects disabled, incoming context propagation, JSON content negotiation, and `X-Request-ID`
propagation. Response bodies are always closed and limited to 1 MiB.

Billing validates content type, exact JSON shape, status, error envelope, and DTO completeness. A
consume success must contain exactly one non-negative balance for every requested product. Network
errors, timeouts, Stock 5xx, redirects, malformed bodies, unexpected status, or incomplete results
become `503 STOCK_SERVICE_UNAVAILABLE`.

## 14. Operations

Both binaries validate configuration and ping only their owned database before serving. Health
endpoints expose process liveness and owned-database readiness. Compose starts PostgreSQL, Stock,
and Billing in health order. Runtime images execute as non-root users.

Migrations are versioned SQL and applied explicitly with `make migrate-up`; application startup does
not change schema. The CI pipeline creates clean PostgreSQL state, applies all migrations, checks
formatting, runs `go vet`, tests and builds, builds both Docker images, and removes disposable
infrastructure.

## 15. Request correlation

The Angular functional interceptor generates a new UUID v4 for every HTTP request and preserves all
other headers and bodies. Stock and Billing echo `X-Request-ID`; Billing also propagates it to Stock.
Safe error bodies include the request reference, while structured logs include it alongside service,
operation, invoice, product, and durable operation identifiers where applicable.

Request ID is for observability. Durable operation ID is the idempotency guarantee; the two concepts
are intentionally separate.

## 16. Verification evidence

The frozen frontend candidate passed:

- `bun install --frozen-lockfile`;
- `bun run lint`;
- 117 Vitest/TestBed tests;
- `bun run build`;
- pull-request and post-merge GitHub Actions.

The frozen backend candidate passed:

- clean PostgreSQL startup and all five migrations;
- `gofmt` checks and `go vet`;
- Stock and Billing tests, including PostgreSQL integration tests;
- both binary builds and Docker image builds;
- pull-request GitHub Actions.

The final browser regression used a new disposable database and the published Web/API `main`
commits. It verified product persistence, integer validation, duplicate conflict, two-item invoice
snapshots, sequential number, processing feedback, confirmed closure, real print dialog, exact
balance updates, and absence of a reprint action. Two simultaneous finalizations against balance one
produced one `CLOSED`, one `OPEN` with `INSUFFICIENT_STOCK`, and final balance zero. With Stock
stopped and Billing healthy, the UI showed `STOCK_SERVICE_UNAVAILABLE`, kept the invoice open, and
preserved request ID and retry; after recovery, one retry closed the invoice and consumed once.

## 17. Deliberate scope boundaries

- No dashboard, authentication, AI, global state framework, ORM, or distributed transaction was
  added.
- The repository does not include a permanent browser E2E runner; the final real-service browser
  regression is delivery evidence, while focused behavior remains covered by unit/component tests.
- Production's relative frontend API URLs require a deployment gateway or reverse proxy; this local
  challenge repository does not prescribe a cloud platform.

For command-level reproduction and the exact DTOs, see [README.md](./README.md),
[api/docs/BOOTSTRAP.md](./api/docs/BOOTSTRAP.md), and
[api/docs/HTTP_CONTRACT.md](./api/docs/HTTP_CONTRACT.md).
