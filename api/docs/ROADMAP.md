# Roadmap and status

The functional flows required by the challenge are implemented. This document records repository capabilities. Quality commands and CI must be run again for every delivery candidate because release validation is not a permanent feature result.

## Stage 1 — Product vertical slice

- [x] Product migration with unique code and non-negative balance
- [x] pgx repository plus domain/transport validation
- [x] `POST /api/v1/products`
- [x] `GET /api/v1/products`
- [x] `GET /api/v1/products/:id`
- [x] `PRODUCT_CODE_CONFLICT`, `PRODUCT_NOT_FOUND` and focused tests

## Stage 2 — Invoice creation

- [x] `invoices` and `invoice_items` migrations
- [x] PostgreSQL-backed sequential unique number
- [x] `POST /internal/v1/products/resolve`
- [x] Billing Stock client through `net/http`
- [x] `POST /api/v1/invoices`
- [x] initial `OPEN` status and historical code/description snapshots
- [x] creation, constraint, numbering and HTTP-boundary tests

## Stage 3 — Invoice queries

- [x] `GET /api/v1/invoices`
- [x] `GET /api/v1/invoices/:id`
- [x] stable DTOs, deterministic ordering and detail snapshots
- [x] list, detail, empty-result and not-found tests

Filtering and pagination **do not exist**. They are not challenge requirements and remain outside this delivery's scope rather than being presented as completed work.

## Stage 4 — Atomic stock consumption

- [x] durable consumption operation/result schemas
- [x] `POST /internal/v1/stock/consume`
- [x] all-or-nothing PostgreSQL transaction
- [x] deterministic row-lock order
- [x] `PRODUCT_NOT_FOUND` and `INSUFFICIENT_STOCK`
- [x] PostgreSQL rollback and competing-consumer tests

## Stage 5 — Invoice print/finalization

- [x] durable `invoice_finalizations` schema
- [x] `POST /api/v1/invoices/:id/print`
- [x] Stock consumption without cross-database access
- [x] conditional `OPEN -> CLOSED` transition and `closed_at`
- [x] Invoice close plus operation completion in one Billing transaction
- [x] `INVOICE_NOT_OPEN` and race/rollback tests

## Stage 6 — Idempotency and ambiguous failure recovery

- [x] stable `operation_id` per Invoice
- [x] canonical SHA-256 fingerprint of Invoice and sorted items
- [x] stored original result for every consumed Product
- [x] durable replay without another decrement
- [x] `IDEMPOTENCY_CONFLICT` for changed payload under one identity
- [x] retry after a lost response with the same command
- [x] late-replay, conflict and concurrent-request tests

## Stage 7 — Required microservice failure scenario

- [x] explicit Billing HTTP timeout
- [x] defensive validation of Stock status, content type, body and DTO
- [x] `503 STOCK_SERVICE_UNAVAILABLE`
- [x] Invoice remains `OPEN` without usable Stock confirmation
- [x] manual retry after Stock recovery
- [x] request ID propagation for correlation
- [x] documented demonstration procedure

## Stage 8 — Hardening and delivery readiness

Present in the repository:

- [x] unit, HTTP and PostgreSQL tests at critical boundaries
- [x] CI workflow with clean PostgreSQL, both migrations, tests, vet, build and Docker build
- [x] Docker Compose health checks, separate databases and non-root containers
- [x] final bootstrap, architecture, contract and technical-decision documents
- [x] reproducible migration-down, smoke, concurrency, idempotency and offline-recovery procedures
- [x] layout ready to copy as `Korp_Teste_SeuNome/api`

Gate for each candidate commit:

- [ ] `make check-fmt`, `make vet`, `make test`, `make build` and `make docker-build` pass on that candidate
- [ ] GitHub Actions is green for that candidate
- [ ] Docker/HTTP smoke procedure is executed on a clean disposable database when freezing the delivery

These gates remain unchecked here because their result depends on the exact commit and environment. They do not represent missing functional features.

## Outside challenge scope

Filtering/pagination, authentication/authorization, prices/totals, update/delete operations, cache, broker, automatic retry, distributed tracing, API gateway, Kubernetes and AI are outside this delivery. None is needed to demonstrate the required ownership, persistence, concurrency, idempotency and recovery behavior.
