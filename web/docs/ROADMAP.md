# Implementation Roadmap

The following stages describe the planned work post-bootstrap.

## Stage 1: Product vertical slice

- **Scope:** Product TypeScript models, Stock API client, Product list page, Angular Material table, Product creation page, Typed Reactive Form, Required validation, Loading state using Signals, HTTP handling using RxJS, Success/error feedback using MatSnackBar, Unit and HTTP tests.
- **Acceptance:** A user can create a product and immediately see persisted products loaded from the real Stock API.
- **Status:** ✅ **COMPLETED** - Verified on 21/08/2026 against the real Stock Service: the browser received `201 Created` for product creation, navigated to `/products`, loaded the persisted record with `200 OK`, and loaded the same record again after a full page reload. Focused tests, the full test suite, lint, production build, and diff checks also pass.

## Stage 2: Invoice creation

- **Scope:** Invoice models (numeric `number`, nullable `closedAt`, snapshot `InvoiceItem`), Billing API client (`InvoiceApiService` with create/list/get), Product selection from Stock, Invoice create page with FormArray, Dynamic Typed Reactive Form for multiple products, Quantity validation (required, min(1), integer-only), Prevent duplicate products validator, Create OPEN invoice, Success/error feedback with MatSnackBar, Tests.
- **Acceptance:** A user can select registered products, define quantities and create an OPEN invoice through the real Billing API.
- **Status:** ✅ **COMPLETED** - Verified on 23/08/2026 against the real Stock and Billing APIs: the browser created a multi-item invoice, received its sequential number, opened the persisted detail, and preserved the returned product snapshots.

## Stage 3: Invoice listing and detail

- **Scope:** Invoice list page, OPEN and CLOSED status presentation, Filtering, Pagination if exposed by the API, Invoice detail page, Product snapshot presentation, Tests.
- **Acceptance:** Invoices can be browsed and a complete invoice can be inspected without querying Stock for historical product descriptions.
- **Status:** ✅ **COMPLETED** - Verified on 23/08/2026 against the real Billing API: the browser listed persisted invoices with distinct `OPEN`/`CLOSED` states and opened complete details without querying current product descriptions.

## Stage 4: Print and finalization UX

- **Scope:** Print action visible only where appropriate, Disable print while processing, MatProgressSpinner or Material progress indicator, POST /api/v1/invoices/:id/print, Update UI from OPEN to CLOSED after success, Printable document layout, @media print, window.print only after backend confirmation, Tests.
- **Acceptance:** Printing visibly enters a processing state, successfully closes an OPEN invoice, updates UI state and opens a clean printable browser view.
- **Status:** ✅ **COMPLETED** - Verified on 23/08/2026 against the real APIs: a confirmed `200` changed the invoice from `OPEN` to `CLOSED`, removed the finalize action, rendered the read-only badge, and called `window.print()` exactly once. Product balances reflected one confirmed consumption.

## Stage 5: Failure and recovery UX

- **Scope:** Handle STOCK_SERVICE_UNAVAILABLE, Meaningful MatSnackBar feedback, Keep invoice OPEN after backend failure, Allow explicit retry, Do not perform blind global automatic retries, Display request_id when useful for troubleshooting, Tests.
- **Acceptance:** When Stock is intentionally unavailable, the user receives clear feedback and can retry after recovery without refreshing or corrupting state.
- **Status:** ✅ **COMPLETED** - Verified on 23/08/2026 by stopping the real Stock container: finalization returned `503 STOCK_SERVICE_UNAVAILABLE`, displayed a request ID, kept the invoice `OPEN`, and re-enabled manual retry. After Stock recovered, retry from the same page returned `200 CLOSED`, printed once, and decremented each item balance exactly once.

## Stage 6: Domain conflict UX

- **Scope:** Handle INSUFFICIENT_STOCK, Handle INVOICE_NOT_OPEN, Handle PRODUCT_CODE_CONFLICT, Handle validation errors, Map API errors to useful user-facing messages, Keep unexpected errors distinguishable from domain conflicts.
- **Acceptance:** Known domain errors have explicit, useful and deterministic frontend behavior.
- **Status:** ✅ **COMPLETED** - Focused tests cover every documented code. A real last-unit concurrency run finalized one of two competing invoices, kept the loser `OPEN` with explicit `409 INSUFFICIENT_STOCK` feedback and request ID, did not print the loser, and left the product balance at zero.

## Stage 7: Frontend hardening

- **Scope:** Route loading states, Empty states, Accessibility review, Keyboard usability, Responsive layout, Error-state consistency, Component tests, HttpClient tests, Build optimization verification.
- **Acceptance:** The frontend is stable, accessible enough for the challenge scope and all quality gates pass.
- **Status:** **FRONTEND HARDENING VALIDATED; FINAL BACKEND RECHECK PENDING** - Loading, empty, malformed-response, safe-error, request-ID, retry, duplicate-action, DTO/status, and non-optimistic finalization paths have focused component and HTTP tests. The frozen Bun install, Angular ESLint, 117-test Vitest suite, production build, Prettier, and diff checks pass. A real-service browser pass also covered keyboard focus, 390 px layouts, print media, persistence, multi-item closure, exact balance changes, concurrency, Stock-down feedback, and successful retry; the integration must be repeated after the Stage 5 backend freeze.

## Stage 8: Korp presentation preparation

- **Scope:** Document Angular lifecycle hooks actually used, Document where RxJS is used and why, Document where Signals are used and why, Document Angular Material components used, Document additional packages and justification, Prepare demonstration path for product registration, Prepare invoice creation demonstration, Prepare successful print demonstration, Prepare required Stock microservice failure demonstration, Prepare recovery demonstration.
- **Acceptance:** The project documentation contains enough accurate implementation detail to support the final technical presentation without inventing explanations after development.
- **Status:** **DOCUMENTATION AND SCRIPT READY; PRESENTATION EXECUTION PENDING** - The README, architecture, technical details, and final demonstration script describe the current lifecycle, Signals, RxJS, forms, HTTP mapping, Material usage, error recovery, finalization, environment setup, tool purposes, and reproducible quality commands. Recording the narrated video, validating a deployed reverse proxy, and completing the external public delivery remain pending.
