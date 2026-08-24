# Korp API

Backend for the Korp challenge, implemented as two independent Go microservices:

- **Stock** owns Products, balances, snapshot resolution and atomic/idempotent consumption;
- **Billing** owns Invoices, sequential numbering, historical snapshots and finalization orchestration.

The services use Gin, PostgreSQL, `pgx/v5` + `pgxpool`, explicit SQL, `net/http` and `log/slog`. Each service has its own Go module, database and migration directory. Billing reaches Stock only over HTTP; there is no ORM, Resty, shared database or distributed transaction.

## Quick start

Prerequisites: Go 1.26, Docker with Docker Compose, Make and `curl`. `jq` is recommended for the smoke procedures.

From this `api/` root:

```bash
cp .env.example .env
docker compose --env-file .env.example config
docker compose up -d --wait postgres
make migrate-up
docker compose up -d --build --wait
docker compose ps
curl http://localhost:8081/health/ready
curl http://localhost:8082/health/ready
```

`make migrate-up` applies **all Stock and Billing migrations** before either API is used. Application binaries never mutate schema at startup.

Default ports:

| Component | Host port |
|---|---:|
| PostgreSQL | `5432` |
| Stock API | `8081` |
| Billing API | `8082` |
| Allowed Angular development origin | `4200` |

Browser origins are configured through `CORS_ALLOWED_ORIGINS`; wildcards are not allowed. The database URLs and credentials in `.env.example` are local development examples only. `.env` is ignored by Git.

## Quality check

```bash
make check-fmt
make vet
make test
make build
make docker-build
```

The high-risk PostgreSQL tests are enabled by `STOCK_TEST_DATABASE_URL` and `BILLING_TEST_DATABASE_URL`. Read the [bootstrap guide](./docs/BOOTSTRAP.md) before setting them because test databases are disposable and their schema/data are reset.

## Main workflow

1. `POST /api/v1/products` creates Products in Stock.
2. `POST /api/v1/invoices` asks Stock for every Product snapshot and stores an `OPEN` Invoice in Billing.
3. `POST /api/v1/invoices/:id/print` obtains a durable operation, requests atomic Stock consumption, and only then closes the Invoice.
4. On timeout or an unusable Stock response, Billing returns `503 STOCK_SERVICE_UNAVAILABLE` and keeps the Invoice `OPEN` for manual retry.
5. Retry reuses the same `operation_id`. Stock compares the SHA-256 fingerprint and returns the originally stored result without decrementing again.

The [reproducible evaluation procedures](./docs/BOOTSTRAP.md#http-evaluation-procedures) cover Products, a multi-item Invoice, balance-`1` concurrency, direct idempotency replay/conflict, and recovery with Stock offline.

## Structure

```text
api/
├── go.work
├── docker-compose.yml
├── Makefile
├── infra/postgres/init/
├── services/
│   ├── stock/
│   │   ├── cmd/api/
│   │   ├── internal/{config,database,http,product}/
│   │   └── migrations/
│   └── billing/
│       ├── cmd/api/
│       ├── internal/{config,database,http,invoice,stock}/
│       └── migrations/
└── docs/
```

## Documentation

- [Bootstrap, migrations and evaluation procedures](./docs/BOOTSTRAP.md)
- [Architecture and ownership](./docs/ARCHITECTURE.md)
- [Final HTTP contract](./docs/HTTP_CONTRACT.md)
- [Technical details and decisions](./docs/technical-details.md)
- [Roadmap status and scope boundaries](./docs/ROADMAP.md)
- [Documentation index](./docs/README.md)

## Delivery placement

This is the backend working repository. Its content is organized to be copied, without renaming this repository, to:

```text
Korp_Teste_SeuNome/
└── api/
```

Copy versioned files, not a local `.env`, binaries, logs, caches or volumes. This repository's Git history is not required inside the final directory.
