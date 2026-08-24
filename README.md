# Korp Technical Challenge — Enzo Dias

This public repository contains the frozen final delivery for the Korp technical challenge. It
combines the Angular frontend and both Go microservices at the exact commits used by the final
regression:

- Web: `b7efdbbb07f8f31c23aaf6a1526b6c60e2943c1a`
- API: `06fd227ff2d9d32ccd755942b0729a42f978251f`

## What the application demonstrates

- persisted product creation and listing;
- typed client and server validation;
- invoice creation with multiple product snapshots;
- backend-generated sequential invoice numbers;
- backend-confirmed invoice finalization and browser printing;
- atomic stock consumption under concurrency;
- durable idempotency for ambiguous or lost responses;
- safe Stock-unavailable feedback and manual recovery;
- end-to-end request correlation through `X-Request-ID`.

## Architecture

```text
Angular Web
  ├── products ──────────────> Stock Service ─────> stock_db
  └── invoices ──────────────> Billing Service ───> billing_db
                                      │
                                      └───────────> Stock Service
```

The browser never decrements stock, generates authoritative invoice numbers, or marks an invoice
closed optimistically. Billing owns invoice finalization and calls Stock over HTTP. Stock and
Billing own separate PostgreSQL databases and database roles.

## Repository layout

```text
Korp_Teste_EnzoDias/
├── README.md
├── TECHNICAL_DETAILS.md
├── web/
└── api/
```

The source repositories keep their own focused documentation:

- [Frontend README](./web/README.md)
- [Frontend technical details](./web/docs/technical-details.md)
- [Backend README](./api/README.md)
- [Backend bootstrap guide](./api/docs/BOOTSTRAP.md)
- [Backend HTTP contract](./api/docs/HTTP_CONTRACT.md)
- [Backend technical details](./api/docs/technical-details.md)

## Prerequisites

- Docker Engine with Docker Compose v2;
- Go 1.26 for local backend checks;
- Bun 1.3 or newer for the frontend;
- a current browser.

The normal application startup does not require a locally installed PostgreSQL or migration CLI;
Docker provides both.

## Start the backend

From the repository root:

```bash
cd api
cp .env.example .env
docker compose up -d --wait postgres
make migrate-up
docker compose up -d --build --wait
docker compose ps
```

Migrations are explicit and run before application traffic. Service startup never mutates the
schema. The default local endpoints are:

| Endpoint | URL |
|---|---|
| Stock readiness | `http://localhost:8081/health/ready` |
| Billing readiness | `http://localhost:8082/health/ready` |
| Stock API | `http://localhost:8081/api/v1` |
| Billing API | `http://localhost:8082/api/v1` |
| PostgreSQL host port | `localhost:5432` |

The checked-in `.env.example` contains development-only values. Never commit the generated `.env`.

## Start the frontend

In another terminal, from the repository root:

```bash
cd web
bun install --frozen-lockfile
bun run start
```

Open `http://localhost:4200`. The development configuration calls Stock on port `8081` and Billing
on port `8082`. The production build uses relative `/api/v1` URLs and therefore expects an external
gateway or reverse proxy to route product and invoice endpoints.

## Apply and inspect migrations

```bash
cd api
make migrate-up
docker compose exec -T postgres psql -U postgres -d stock_db -c '\dt'
docker compose exec -T postgres psql -U postgres -d billing_db -c '\dt'
```

Down migrations are destructive validation tools for disposable databases. Read
[the bootstrap guide](./api/docs/BOOTSTRAP.md) before using them.

## Run the checks

Frontend:

```bash
cd web
bun install --frozen-lockfile
bun run lint
bun run test -- --watch=false
bun run build
```

Backend:

```bash
cd api
make check-fmt
make vet
make test
make build
make docker-build
```

The original Web and API repositories also run these gates in GitHub Actions. The final candidate
passed 117 frontend tests, the production Angular build, both Go suites, `go vet`, binary builds,
Docker image builds, and clean-database migration checks.

## Demonstrate Stock failure and recovery

Create an `OPEN` invoice while all services are healthy, then stop only Stock:

```bash
cd api
docker compose stop stock
```

Keep Billing running and click `Finalizar` in the invoice detail. Expected behavior:

1. processing feedback stops;
2. the page shows a safe Stock-unavailable message and request reference;
3. the invoice remains `OPEN`;
4. manual retry remains available;
5. browser printing does not run.

Restore Stock and retry once:

```bash
docker compose start stock
curl --fail http://localhost:8081/health/ready
```

Billing reuses the persisted finalization operation. Stock either performs the first atomic consume
or replays the stored result, so an ambiguous earlier response cannot decrement inventory twice.

## Stop the local stack

```bash
cd api
docker compose down
```

This preserves the named database volume. `docker compose down --volumes` destroys both local
databases and should only be used for an intentional disposable reset.

## Final delivery documents

- [Consolidated technical details](./TECHNICAL_DETAILS.md)
- [Frontend final demonstration script](./web/docs/demo-script.md)
- [Exact HTTP contract](./api/docs/HTTP_CONTRACT.md)
- [Architecture details](./api/docs/ARCHITECTURE.md)

No real environment file, credential, token, dependency directory, database volume, or build
artifact is included in this repository.
