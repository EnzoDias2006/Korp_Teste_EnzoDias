# Bootstrap and evaluation guide

All commands in this guide run from the `api/` root unless a command changes directory explicitly.

## Prerequisites

- Go 1.26;
- Docker Engine with Docker Compose v2 and support for `docker compose up --wait`;
- Make;
- Git;
- `curl` for health and HTTP examples;
- `jq` for the executable smoke/concurrency scripts;
- Lefthook only when local Git hooks are desired.

`golang-migrate` does not need a host installation. Make uses the pinned `migrate/migrate:v4.17.0` image. Gin `v1.12.0` and pgx `v5.10.0` are resolved through each service's committed `go.mod`/`go.sum`.

## Clean start

```bash
cp .env.example .env
docker compose --env-file .env.example config
docker compose up -d --wait postgres
make migrate-up
docker compose up -d --build --wait
docker compose ps
```

This order is intentional:

1. validate the committed example configuration;
2. start a healthy PostgreSQL instance and create the two databases on the first volume start;
3. apply **all Stock and Billing migrations**;
4. build/start Stock and wait for readiness;
5. start Billing after PostgreSQL and Stock are healthy.

Neither service applies SQL migrations during startup. Compose ordering uses health conditions, not fixed sleeps.

Verify health:

```bash
curl -i http://localhost:8081/health/live
curl -i http://localhost:8081/health/ready
curl -i http://localhost:8082/health/live
curl -i http://localhost:8082/health/ready
```

Liveness proves that the HTTP process is serving. Readiness pings only the service-owned database with a two-second deadline. An unavailable database returns `503 DATABASE_UNAVAILABLE` in the safe error envelope.

Stop without deleting data:

```bash
docker compose down
```

`docker compose down --volumes --remove-orphans` deletes both local databases and all their data. Use it only for an intentional disposable reset.

## Ports

| Component | Host | Compose network |
|---|---:|---|
| PostgreSQL | `localhost:5432` | `postgres:5432` |
| Stock | `localhost:8081` | `stock:8081` |
| Billing | `localhost:8082` | `billing:8082` |

The default allowed browser origin is `http://localhost:4200`; Angular is not part of this backend Compose file.

## Environment

`.env.example` contains local-only sample credentials. Copy it to the ignored `.env`; never commit a real `.env`.

| Root variable | Purpose | Example |
|---|---|---|
| `POSTGRES_USER` | PostgreSQL bootstrap administrator | `postgres` |
| `POSTGRES_PASSWORD` | Local administrator password | `postgres` |
| `POSTGRES_DB` | Initial administrative database | `postgres` |
| `POSTGRES_PORT` | Published PostgreSQL port | `5432` |
| `STOCK_DB_PASSWORD` | Password created for `stock_user` | `stock_pass` |
| `BILLING_DB_PASSWORD` | Password created for `billing_user` | `billing_pass` |
| `CORS_ALLOWED_ORIGINS` | Explicit comma-separated origins for both APIs | `http://localhost:4200` |
| `STOCK_HTTP_ADDR` | Stock host-run listen address | `:8081` |
| `STOCK_DATABASE_URL` | Stock host-run URL | `stock_db` on localhost |
| `STOCK_MIGRATION_DATABASE_URL` | Stock golang-migrate URL | `stock_db` on `postgres` |
| `BILLING_HTTP_ADDR` | Billing host-run listen address | `:8082` |
| `BILLING_DATABASE_URL` | Billing host-run URL | `billing_db` on localhost |
| `BILLING_MIGRATION_DATABASE_URL` | Billing golang-migrate URL | `billing_db` on `postgres` |
| `STOCK_SERVICE_URL` | Stock URL used by host-run Billing | `http://localhost:8081` |

Compose maps root names to each process's service-local contract:

- Stock: `HTTP_ADDR`, `DATABASE_URL`, `CORS_ALLOWED_ORIGINS`;
- Billing: `HTTP_ADDR`, `DATABASE_URL`, `STOCK_SERVICE_URL`, `CORS_ALLOWED_ORIGINS`.

`DATABASE_URL` is required by both services; `STOCK_SERVICE_URL` is additionally required by Billing. URLs are validated before serving and are not printed by normal startup logs. `HTTP_ADDR` defaults to `:8081`/`:8082` in the binaries. Billing's Stock client timeout is a fixed five seconds in the current implementation.

`CORS_ALLOWED_ORIGINS` is optional at the binary boundary. When set, each entry must be a complete `http://` or `https://` origin with no path, query, fragment, credentials, empty item or wildcard:

```dotenv
CORS_ALLOWED_ORIGINS=http://localhost:4200,https://app.example.com
```

An invalid origin fails startup. An allowed preflight receives the configured origin, `GET, POST, OPTIONS`, `Content-Type, X-Request-ID`, exposed `X-Request-ID`, `Vary: Origin`, and status `204`. A nonmatching preflight receives no allow headers and does not reach the route handler.

## Database ownership and migrations

The first initialization of the PostgreSQL volume creates:

- `stock_db`, used only by `stock_user`;
- `billing_db`, used only by `billing_user`.

Public database connection privileges are revoked. The service roles cannot connect to each other's database.

Migration ownership is independent:

```text
services/stock/migrations/
  000001_create_products
  000002_create_consumption_operations
  000003_create_consumption_operation_results

services/billing/migrations/
  000001_create_invoices
  000002_create_invoice_finalizations
```

Common commands:

```bash
make migrate-up-stock
make migrate-up-billing
make migrate-up
make migrate-down-stock     # exactly one Stock version
make migrate-down-billing   # exactly one Billing version
make migrate-down           # exactly one Billing and one Stock version
```

### Clean migration and safe down validation

The following procedure intentionally destroys the Compose volume. Run it only when the environment contains no data that must be kept:

```bash
docker compose down --volumes --remove-orphans
cp -n .env.example .env
docker compose up -d --wait postgres
make migrate-up

docker compose exec -T postgres psql -U postgres -d stock_db -c '\dt'
docker compose exec -T postgres psql -U postgres -d billing_db -c '\dt'

make migrate-down-billing
make migrate-down-billing
make migrate-down-stock
make migrate-down-stock
make migrate-down-stock

docker compose exec -T postgres psql -U postgres -d stock_db -c '\dt'
docker compose exec -T postgres psql -U postgres -d billing_db -c '\dt'

make migrate-up
docker compose up -d --build --wait
```

The first inspection must show Product/consumption tables in Stock and Invoice/finalization tables in Billing. After all one-step downs, only golang-migrate bookkeeping may remain; application tables must be gone. The final `make migrate-up` proves that both schemas reproduce from zero. Down migrations are destructive and are validation tools for disposable databases, not a production rollback recommendation.

## Developer and quality commands

```bash
make help
make infra-up
make services-up
make services-down
make run-stock
make run-billing
make fmt
make check-fmt
make test
make test-stock
make test-billing
make vet
make build
make build-stock
make build-billing
make docker-build
```

For host execution, start PostgreSQL and apply migrations, then run each service in its own terminal:

```bash
make infra-up
make migrate-up
make run-stock
```

```bash
make run-billing
```

The Makefile loads `.env` for these targets.

### Real PostgreSQL integration tests

Without test URLs, the PostgreSQL-specific packages explicitly skip their integration cases while unit/HTTP-client tests still run. To exercise the real locks, constraints and transactions locally, use disposable databases.

Stock tests create and remove isolated databases, so their URL must use an administrator role. Billing tests reset the schema named by their URL. Create a dedicated Billing test database rather than pointing them at `billing_db` containing useful data:

```bash
docker compose up -d --wait postgres
docker compose exec -T postgres psql -U postgres -d postgres \
  -c 'DROP DATABASE IF EXISTS billing_test WITH (FORCE)'
docker compose exec -T postgres psql -U postgres -d postgres \
  -c 'CREATE DATABASE billing_test'

STOCK_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' \
BILLING_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/billing_test?sslmode=disable' \
make test

docker compose exec -T postgres psql -U postgres -d postgres \
  -c 'DROP DATABASE IF EXISTS billing_test WITH (FORCE)'
```

Adjust the administrator password/port if `.env` differs. The explicit `billing_test` name bounds the destructive reset.

To maintain dependency metadata, run tidy in each module, not at the repository root:

```bash
(cd services/stock && go mod tidy)
(cd services/billing && go mod tidy)
git diff -- services/stock/go.mod services/stock/go.sum \
  services/billing/go.mod services/billing/go.sum
```

## Quality hooks and CI

Install local hooks when Lefthook is available:

```bash
lefthook install
```

Pre-commit checks `gofmt`; pre-push runs `make vet` and `make test`.

`.github/workflows/backend.yml` runs on pull requests and pushes to `main`. It starts a disposable healthy PostgreSQL instance, applies both services' migrations, executes:

```bash
make check-fmt
make vet
make test
make build
make docker-build
```

and always removes the CI volume. A local pass does not prove that a later commit's GitHub Actions run is green; verify the candidate commit's workflow separately.

## HTTP evaluation procedures

The commands below assume the complete stack is healthy and both migrations are current. They use only registered endpoints. Use a disposable local database because they create durable records.

Set reusable URLs:

```bash
STOCK_API='http://localhost:8081'
BILLING_API='http://localhost:8082'
```

### Product, invoice and finalization smoke test

Create two products and capture their IDs:

```bash
PRODUCT_A_JSON="$(curl -fsS -X POST "$STOCK_API/api/v1/products" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: smoke-product-a' \
  -d '{"code":"SMOKE-A","description":"Smoke product A","balance":10}')"
PRODUCT_B_JSON="$(curl -fsS -X POST "$STOCK_API/api/v1/products" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: smoke-product-b' \
  -d '{"code":"SMOKE-B","description":"Smoke product B","balance":4}')"
PRODUCT_A_ID="$(jq -r '.id' <<<"$PRODUCT_A_JSON")"
PRODUCT_B_ID="$(jq -r '.id' <<<"$PRODUCT_B_JSON")"
printf '%s\n%s\n' "$PRODUCT_A_JSON" "$PRODUCT_B_JSON" | jq .

curl -fsS "$STOCK_API/api/v1/products" | jq .
curl -fsS "$STOCK_API/api/v1/products/$PRODUCT_A_ID" | jq .
```

Prove duplicate code and invalid balance:

```bash
curl -sS -o /tmp/korp-duplicate-product.json -w 'duplicate status=%{http_code}\n' \
  -X POST "$STOCK_API/api/v1/products" -H 'Content-Type: application/json' \
  -d '{"code":"smoke-a","description":"Duplicate","balance":1}'
jq . /tmp/korp-duplicate-product.json

curl -sS -o /tmp/korp-invalid-product.json -w 'invalid status=%{http_code}\n' \
  -X POST "$STOCK_API/api/v1/products" -H 'Content-Type: application/json' \
  -d '{"code":"INVALID","description":"Invalid","balance":-1}'
jq . /tmp/korp-invalid-product.json
```

Expected: `409 PRODUCT_CODE_CONFLICT` and `422 VALIDATION_ERROR`.

Create a multi-item invoice, then list and inspect its persisted snapshots:

```bash
INVOICE_JSON="$(curl -fsS -X POST "$BILLING_API/api/v1/invoices" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: smoke-invoice' \
  -d "{\"items\":[{\"product_id\":$PRODUCT_A_ID,\"quantity\":2},{\"product_id\":$PRODUCT_B_ID,\"quantity\":1}]}")"
INVOICE_ID="$(jq -r '.id' <<<"$INVOICE_JSON")"
jq . <<<"$INVOICE_JSON"

curl -fsS "$BILLING_API/api/v1/invoices" | jq .
curl -fsS "$BILLING_API/api/v1/invoices/$INVOICE_ID" | jq .
```

Expected: status `OPEN`, `closed_at: null`, two item snapshots and no stock decrement yet.

Finalize and verify the lifecycle and balances:

```bash
curl -fsS -X POST "$BILLING_API/api/v1/invoices/$INVOICE_ID/print" \
  -H 'X-Request-ID: smoke-print' | jq .
curl -fsS "$BILLING_API/api/v1/invoices/$INVOICE_ID" | jq .
curl -fsS "$STOCK_API/api/v1/products/$PRODUCT_A_ID" | jq .
curl -fsS "$STOCK_API/api/v1/products/$PRODUCT_B_ID" | jq .

curl -sS -o /tmp/korp-print-closed.json -w 'repeat status=%{http_code}\n' \
  -X POST "$BILLING_API/api/v1/invoices/$INVOICE_ID/print"
jq . /tmp/korp-print-closed.json
```

Expected: the first print returns `CLOSED` with non-null `closed_at`; balances become `8` and `3`; the second print returns `409 INVOICE_NOT_OPEN` without another decrement.

### Concurrency: balance 1, two invoices

```bash
RACE_PRODUCT_JSON="$(curl -fsS -X POST "$STOCK_API/api/v1/products" \
  -H 'Content-Type: application/json' \
  -d '{"code":"RACE-ONE","description":"Concurrency product","balance":1}')"
RACE_PRODUCT_ID="$(jq -r '.id' <<<"$RACE_PRODUCT_JSON")"

RACE_INVOICE_A="$(curl -fsS -X POST "$BILLING_API/api/v1/invoices" \
  -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":$RACE_PRODUCT_ID,\"quantity\":1}]}")"
RACE_INVOICE_B="$(curl -fsS -X POST "$BILLING_API/api/v1/invoices" \
  -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":$RACE_PRODUCT_ID,\"quantity\":1}]}")"
RACE_INVOICE_A_ID="$(jq -r '.id' <<<"$RACE_INVOICE_A")"
RACE_INVOICE_B_ID="$(jq -r '.id' <<<"$RACE_INVOICE_B")"

RACE_OUTPUT_DIR="$(mktemp -d)"
curl -sS -o "$RACE_OUTPUT_DIR/a.json" -w '%{http_code}' \
  -X POST "$BILLING_API/api/v1/invoices/$RACE_INVOICE_A_ID/print" \
  > "$RACE_OUTPUT_DIR/a.status" &
RACE_PID_A=$!
curl -sS -o "$RACE_OUTPUT_DIR/b.json" -w '%{http_code}' \
  -X POST "$BILLING_API/api/v1/invoices/$RACE_INVOICE_B_ID/print" \
  > "$RACE_OUTPUT_DIR/b.status" &
RACE_PID_B=$!
wait "$RACE_PID_A" "$RACE_PID_B"

printf 'A=%s B=%s\n' "$(<"$RACE_OUTPUT_DIR/a.status")" "$(<"$RACE_OUTPUT_DIR/b.status")"
jq . "$RACE_OUTPUT_DIR/a.json"
jq . "$RACE_OUTPUT_DIR/b.json"
curl -fsS "$STOCK_API/api/v1/products/$RACE_PRODUCT_ID" | jq .
```

Expected: one status `200` with a `CLOSED` invoice, one status `409` with `INSUFFICIENT_STOCK`, and final balance `0`. The losing invoice remains `OPEN`.

### Durable consume replay and conflict

This internal route is documented because it is Billing's actual command boundary. It also gives a direct, deterministic idempotency demonstration.

```bash
IDEMP_PRODUCT_JSON="$(curl -fsS -X POST "$STOCK_API/api/v1/products" \
  -H 'Content-Type: application/json' \
  -d '{"code":"IDEMP-ONE","description":"Idempotency product","balance":5}')"
IDEMP_PRODUCT_ID="$(jq -r '.id' <<<"$IDEMP_PRODUCT_JSON")"
IDEMP_OPERATION='11111111-1111-4111-8111-111111111111'

curl -fsS -X POST "$STOCK_API/internal/v1/stock/consume" \
  -H 'Content-Type: application/json' \
  -d "{\"invoice_id\":9001,\"operation_id\":\"$IDEMP_OPERATION\",\"items\":[{\"product_id\":$IDEMP_PRODUCT_ID,\"quantity\":2}]}" | jq .

curl -fsS -X POST "$STOCK_API/internal/v1/stock/consume" \
  -H 'Content-Type: application/json' \
  -d "{\"invoice_id\":9001,\"operation_id\":\"$IDEMP_OPERATION\",\"items\":[{\"product_id\":$IDEMP_PRODUCT_ID,\"quantity\":2}]}" | jq .

curl -fsS "$STOCK_API/api/v1/products/$IDEMP_PRODUCT_ID" | jq .

curl -sS -o /tmp/korp-idempotency-conflict.json -w 'conflict status=%{http_code}\n' \
  -X POST "$STOCK_API/internal/v1/stock/consume" \
  -H 'Content-Type: application/json' \
  -d "{\"invoice_id\":9001,\"operation_id\":\"$IDEMP_OPERATION\",\"items\":[{\"product_id\":$IDEMP_PRODUCT_ID,\"quantity\":1}]}"
jq . /tmp/korp-idempotency-conflict.json
```

Expected: both identical calls return the originally stored balance `3`, current balance remains `3`, and the changed payload returns `409 IDEMPOTENCY_CONFLICT`.

### Stock offline and recoverable retry

Use fresh records so previous examples do not affect the expected balance:

```bash
RECOVERY_PRODUCT_JSON="$(curl -fsS -X POST "$STOCK_API/api/v1/products" \
  -H 'Content-Type: application/json' \
  -d '{"code":"RECOVERY-ONE","description":"Recovery product","balance":2}')"
RECOVERY_PRODUCT_ID="$(jq -r '.id' <<<"$RECOVERY_PRODUCT_JSON")"
RECOVERY_INVOICE_JSON="$(curl -fsS -X POST "$BILLING_API/api/v1/invoices" \
  -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":$RECOVERY_PRODUCT_ID,\"quantity\":1}]}")"
RECOVERY_INVOICE_ID="$(jq -r '.id' <<<"$RECOVERY_INVOICE_JSON")"

docker compose stop stock
curl -sS -o /tmp/korp-stock-offline.json -w 'offline status=%{http_code}\n' \
  -X POST "$BILLING_API/api/v1/invoices/$RECOVERY_INVOICE_ID/print" \
  -H 'X-Request-ID: recovery-offline'
jq . /tmp/korp-stock-offline.json
curl -fsS "$BILLING_API/api/v1/invoices/$RECOVERY_INVOICE_ID" | jq .

docker compose start stock
until curl -fsS "$STOCK_API/health/ready" >/dev/null; do sleep 1; done

curl -fsS -X POST "$BILLING_API/api/v1/invoices/$RECOVERY_INVOICE_ID/print" \
  -H 'X-Request-ID: recovery-retry' | jq .
curl -fsS "$STOCK_API/api/v1/products/$RECOVERY_PRODUCT_ID" | jq .

curl -sS -o /tmp/korp-recovery-repeat.json -w 'repeat status=%{http_code}\n' \
  -X POST "$BILLING_API/api/v1/invoices/$RECOVERY_INVOICE_ID/print"
jq . /tmp/korp-recovery-repeat.json
curl -fsS "$STOCK_API/api/v1/products/$RECOVERY_PRODUCT_ID" | jq .
```

Expected: offline print returns `503 STOCK_SERVICE_UNAVAILABLE`; the invoice remains `OPEN`; after Stock returns, retry closes it; balance changes from `2` to `1` exactly once; a later print returns `409 INVOICE_NOT_OPEN` and balance stays `1`.

Stopping Stock before the request proves the explicit unavailable path. The harder ambiguous window—Stock commits but Billing loses the response—is covered by Billing/Stock client and PostgreSQL replay tests. Those tests verify reuse of the same operation ID, stored-result replay and no second decrement.

## Delivery copy check

Before copying this directory as `Korp_Teste_SeuNome/api`, inspect versioned and local files:

```bash
git status --short
git ls-files | sort
find . -maxdepth 3 -type f \( -name '.env' -o -name '*.log' -o -name '*.test' \)
```

Do not copy `.env`, local binaries under `bin/`, logs, caches, database volumes or Git metadata into the final directory. Keep `.env.example`, both `go.mod`/`go.sum` pairs, `go.work`, migrations, Compose, Makefile, hooks, workflow and documentation.
