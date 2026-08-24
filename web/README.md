# Korp Web

Angular frontend for the Korp technical challenge. It manages products through the Stock
Service and invoices through the Billing Service, including multi-item creation, invoice
history, backend-confirmed finalization, safe errors, and manual recovery when Stock is
unavailable.

## Requirements

- Bun 1.3.14 (the version declared in `package.json`)
- Stock Service available for product operations
- Billing Service available for invoice operations

Install the exact dependency graph committed in `bun.lock`:

```bash
bun install --frozen-lockfile
```

Do not create npm, pnpm, or Yarn lockfiles for this project.

## API configuration

Angular selects the environment file for the requested build configuration.

| Configuration | Stock API                      | Billing API                    |
| ------------- | ------------------------------ | ------------------------------ |
| Development   | `http://localhost:8081/api/v1` | `http://localhost:8082/api/v1` |
| Production    | `/api/v1`                      | `/api/v1`                      |

Development calls the two API origins directly; this repository does not configure an Angular
development proxy. The relative production URLs require the deployment environment to provide
reverse-proxy or gateway routing for Stock and Billing. Because both use `/api/v1`, that external
routing must distinguish their endpoints; the Angular application does not do it.

## Run locally

Start both backend services on the origins above, then run:

```bash
bun run start
```

Open `http://localhost:4200/`. The application routes are:

- `/products` and `/products/new`
- `/invoices`, `/invoices/new`, and `/invoices/:id`

## Quality commands

```bash
bun run lint
bun run test -- --watch=false
bun run build
```

`bun run test` starts the Angular/Vitest unit runner in its default mode. Passing
`-- --watch=false` makes the quality check reproducible and non-interactive. The production build
is written under `dist/`.

Prettier is installed and configured, but there is no package script or Lefthook command for it.
Format an explicit file set when needed, for example:

```bash
bun x prettier --write README.md docs/*.md
```

Lefthook runs lint before commits and runs tests plus the production build before pushes. Those
hooks support local checks; they do not replace running the commands above before delivery.

## Manual integration evidence and limits

The Vitest/TestBed suite verifies components, HTTP request/response mapping, request IDs, safe
error handling, retry behavior, and non-optimistic invoice closure with mocked HTTP responses. It
does not start the Stock or Billing services, control containers, exercise a deployed reverse
proxy, or provide browser end-to-end automation.

On 24/08/2026, a manual Playwright CLI pass against real local Stock and Billing services verified:

1. product validation, creation, reload persistence, and duplicate-code feedback;
2. a multi-item invoice with snapshots, sequential number, and confirmed closure;
3. print invocation, print-media cleanup, closed-invoice blocking, and exact balance updates;
4. last-unit concurrency with one `CLOSED`, one `OPEN`, and final balance `0`;
5. Stock-down `503`, visible request ID, preserved `OPEN` state, restore, retry, and single consume;
6. keyboard focus and document-width checks at a 390 px viewport.

This manual run is not a checked-in E2E suite. Repeat it after freezing the Stage 5 backend and in
the final deployment, including production reverse-proxy routing and the real browser print dialog.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Technical details](docs/technical-details.md)
- [Final demonstration script](docs/demo-script.md)
- [Implementation roadmap](docs/ROADMAP.md)
