# Frontend Technical Details

This document describes the implementation currently present in this repository. It deliberately
separates automated evidence from checks that require running browsers and backend services.

## 1. Runtime architecture

The application uses Angular 22 standalone components. `app.config.ts` provides the Router and
`HttpClient`; no application NgModule exists. Application components use
`ChangeDetectionStrategy.OnPush`.

There is no `zone.js` package and no zone-based provider in the app configuration. The application
therefore uses Angular 22's default zoneless runtime. State changes are explicit through Signals,
reactive-form controls, router state, and HTTP subscriptions.

The frontend boundary is intentionally narrow:

- product reads and writes go directly to Stock Service;
- invoice reads, writes, and print/finalize requests go to Billing Service;
- Billing, not the browser, coordinates stock updates during finalization;
- the browser does not calculate authoritative stock or invoice status.

## 2. Lazy routes

`app.routes.ts` redirects the root to products and lazy-loads two feature route arrays with
`loadChildren`. Each feature route lazy-loads its standalone page with `loadComponent`.

| URL             | Standalone page |
| --------------- | --------------- |
| `/products`     | `ProductList`   |
| `/products/new` | `ProductCreate` |
| `/invoices`     | `InvoiceList`   |
| `/invoices/new` | `InvoiceCreate` |
| `/invoices/:id` | `InvoiceDetail` |

No wildcard route is configured.

## 3. Lifecycle and subscription cleanup

The code uses lifecycle APIs only where initialization work needs them.

| Component       | Actual lifecycle behavior                                                     |
| --------------- | ----------------------------------------------------------------------------- |
| `ProductList`   | Calls `loadProducts()` from its constructor; it does not implement `OnInit`   |
| `ProductCreate` | Uses no lifecycle hook                                                        |
| `InvoiceList`   | `ngOnInit()` loads invoice summaries                                          |
| `InvoiceCreate` | `ngOnInit()` creates the initial item row and loads products                  |
| `InvoiceDetail` | `ngOnInit()` connects route parameters, the retry trigger, and detail loading |

All five pages subscribe to asynchronous work. Each injects `DestroyRef`, and each lifecycle-bound
chain uses `takeUntilDestroyed(this.destroyRef)`. No component implements `OnDestroy` or maintains a
manual subscription collection.

## 4. Signals

Signals represent synchronous screen state and derived UI decisions.

| Page            | Signal state                                                                                           |
| --------------- | ------------------------------------------------------------------------------------------------------ |
| `ProductList`   | `loading`, `products`, `error`                                                                         |
| `ProductCreate` | `saving`, `submitInFlight`, `errorMessage`                                                             |
| `InvoiceList`   | `loading`, `error`, `requestId`, `invoices`; computed `hasData`                                        |
| `InvoiceCreate` | `products`, `isLoadingProducts`, `isSaving`, `productLoadError`, `errorMessage`                        |
| `InvoiceDetail` | `loading`, `invoice`, `notFound`, `error`, `printing`, `actionError`, `requestId`; computed `canPrint` |

`hasData` centralizes the invoice-list success-state decision. `canPrint` is true only for an
`OPEN` invoice while no print request is in flight. HTTP and router subscriptions update Signals
directly; the templates do not use the async pipe.

## 5. RxJS

RxJS represents asynchronous HTTP and route workflows rather than global state.

| Operator or type     | Purpose in this application                                                  |
| -------------------- | ---------------------------------------------------------------------------- |
| `map`                | Map service DTOs and route parameter values                                  |
| `catchError`         | Convert failures into safe page state or a fallback observable               |
| `finalize`           | Clear loading, saving, and printing flags on success or error                |
| `switchMap`          | Cancel the prior invoice-detail request when route/retry input changes       |
| `combineLatest`      | Combine the current invoice ID with the manual retry trigger                 |
| `filter`             | Reject unusable route IDs before loading                                     |
| `tap`                | Reset invoice-detail state before a request                                  |
| `of` and `EMPTY`     | Complete an intentionally recovered chain without throwing into the template |
| `BehaviorSubject`    | Seed and trigger explicit invoice-detail reloads                             |
| `takeUntilDestroyed` | End subscriptions with the owning page                                       |

The flows avoid nested subscriptions and do not add automatic global retries. Retry is a visible,
user-controlled operation.

## 6. Typed Reactive Forms

### Product creation

`ProductCreate` uses typed controls for `code`, `description`, and `balance`. Code and description
are non-nullable, required, and reject whitespace-only input. Balance is required, has a minimum of
zero, and uses an integer validator. Text is trimmed before the request. Saving and in-flight guards
disable the submit button and reject repeated submit calls; values remain available after a
recoverable error.

### Invoice creation

`InvoiceCreate` uses a typed `FormArray` of item groups containing `productId` and `quantity`.
Quantity is required, has a minimum of one, and rejects fractional input. A form-level validator
rejects duplicate product selections.

The first item row and product load start in `ngOnInit`. While products are loading, or after a load
failure, the invoice form is unavailable. A failure shows the safe shared message and request
reference plus an in-page retry. Retrying clears the stale load error first. A successful response
with no products shows an explicit empty state and links to `/products/new`.

During create, the form and submit controls are disabled. A failed create preserves item values, and
a later manual submit sends the values again without duplicating an in-flight request.

## 7. API services and DTO mapping

Both feature services inject Angular `HttpClient` and build URLs from the selected environment. No
component calls `fetch` or talks to a database.

`ProductApiService` maps snake-case timestamps to camel-case fields and converts numeric product IDs
to the domain model's string IDs. It exposes create, list, and detail operations against Stock.

`InvoiceApiService` maps summary, detail, item, and print responses from Billing. It preserves the
item code and description snapshots returned with an invoice; detail rendering does not query Stock
for a current description. Nullable `closed_at` becomes an optional `closedAt` only when present.

Invoice status crosses an untrusted HTTP boundary, so a private runtime parser accepts exactly
`OPEN` or `CLOSED`. List, detail, and print mappings reject an unknown status rather than coercing it
to `CLOSED`. The print response must additionally be `CLOSED` with a non-empty `closed_at`; otherwise
the service throws and the page never invokes browser printing.

## 8. Request identification

`requestIdInterceptor` is a functional `HttpInterceptorFn`. For each request it:

1. generates a value with `crypto.randomUUID()`;
2. clones the request with `X-Request-ID` set to that value;
3. forwards the clone without changing other headers or the body.

`app.config.ts` registers it with `provideHttpClient(withInterceptors([requestIdInterceptor]))`.
Focused tests cover registration, HTTP methods, UUID shape, per-request uniqueness, and preservation
of other headers.

## 9. Safe backend errors

The expected response shape is nested:

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

The shared `extractSafeHttpApiError` helper treats a known nested code as authoritative and selects
a fixed Portuguese message. It never displays arbitrary backend `message`, database, stack-trace,
or transport text. A valid nested `request_id` is retained so the page can show a support reference.

Unknown nested codes and malformed payloads fall back conservatively. HTTP status `0` and `503`
map to `STOCK_SERVICE_UNAVAILABLE`, while still preserving a valid nested request reference, unless
a known nested code already explains the failure. Page-specific transport overrides are not needed;
product creation and invoice loading/list/detail/finalization all use the shared mapper.

## 10. Backend-confirmed finalization

The UI treats “print” as the operation that finalizes an invoice:

1. the action exists only for an `OPEN` invoice;
2. `printing` immediately disables another click and shows progress;
3. the client posts to Billing at `/invoices/:id/print`;
4. the service validates the returned invoice as `CLOSED` with `closed_at`;
5. the page replaces its invoice Signal with that confirmed response;
6. only then does it call `window.print()`.

No optimistic mutation sets `CLOSED`. On a Stock unavailable error, progress stops, the invoice
remains visibly `OPEN`, the request reference is shown when present, and manual retry stays
available. `INVOICE_NOT_OPEN` triggers an authoritative detail reload. An idempotency conflict is
reported without a blind automatic retry.

## 11. Angular Material and SCSS

The application imports Material by page, keeping each standalone component's dependency list
explicit.

| Material area                | Purpose                                                     |
| ---------------------------- | ----------------------------------------------------------- |
| Toolbar                      | Primary application navigation                              |
| Cards                        | Group route content and states                              |
| Form fields, inputs, selects | Labels, validation, product selection, and quantities       |
| Buttons and icons            | Navigation, add/remove, submit, retry, and finalize actions |
| Progress spinners            | Perceivable loading/saving/finalizing state                 |
| Snackbars                    | Short operation feedback                                    |
| Tooltips                     | Accessible context for icon actions where used              |
| Tables                       | Product, invoice, and invoice-item display                  |

The tables are display-only; there is no `MatSort` integration. Angular CDK is installed because it
underpins Material, but application source does not import a CDK API directly. Focused SCSS handles
layout, narrow-screen overflow/wrapping, status treatment, and invoice `@media print` output without
adding another styling framework.

## 12. Environment configuration

| Build configuration | Stock base URL                 | Billing base URL               |
| ------------------- | ------------------------------ | ------------------------------ |
| Development         | `http://localhost:8081/api/v1` | `http://localhost:8082/api/v1` |
| Production          | `/api/v1`                      | `/api/v1`                      |

Development uses direct origins, not an Angular proxy. Production's relative URLs require an
external reverse proxy or gateway to route product endpoints to Stock and invoice endpoints to
Billing. That deployment rule is not implemented inside the Angular repository.

## 13. Package purposes

| Package or tool                             | Concrete purpose                                                                                 |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Angular Material                            | Accessible application controls and feedback components                                          |
| Angular CDK                                 | Foundation and peer dependency for Material; no direct application API use                       |
| RxJS                                        | HTTP/route composition, error recovery, finalization, and subscription lifecycle                 |
| Vitest                                      | Fast unit/component/spec execution through Angular's unit-test builder                           |
| Angular TestBed and `HttpTestingController` | Component rendering and deterministic HTTP contract tests                                        |
| Angular ESLint                              | TypeScript and Angular-template linting, including configured accessibility rules                |
| Lefthook                                    | Local pre-commit lint and pre-push test/build commands                                           |
| Prettier                                    | Explicit source and Markdown formatting from `.prettierrc`; no package script/hook is configured |
| Bun                                         | Dependency installation, committed lockfile resolution, and project script execution             |

No E2E package or Angular E2E target is configured.

## 14. Quality and evidence boundary

The reproducible project checks are:

```bash
bun install --frozen-lockfile
bun run lint
bun run test -- --watch=false
bun run build
```

The automated suite uses Vitest, TestBed, and `HttpTestingController` for forms, screen states,
request/response mapping, request IDs, malformed responses, safe errors, duplicate-action guards,
and recovery. TypeScript compiler options are the repository's current settings; this document does
not claim that every strictness flag is enabled.

There is no browser E2E runner. On 24/08/2026, a Playwright CLI pass against the real
Stage-4-complete services on ports `8081` and `8082` verified product persistence and duplicate-code
feedback, fractional-balance validation, a two-item invoice with snapshots and sequential number,
backend-confirmed closure, print invocation and print-media cleanup, exact balance updates, and the
absence of a reprint action. At a 390 px viewport, product and invoice pages had no document-level
horizontal overflow, and keyboard navigation reached the labeled product combobox.

The same pass finalized two invoices concurrently against a product with balance `1`: one became
`CLOSED`, the other remained `OPEN` with `INSUFFICIENT_STOCK`, and the final balance was `0`. With
only Stock stopped, Billing returned `503 STOCK_SERVICE_UNAVAILABLE`, the invoice stayed `OPEN`, the
request ID and retry remained visible, and no print occurred; after Stock was healthy again, one
manual retry closed the invoice and decremented stock exactly once. This is manual evidence, not a
checked-in E2E suite. It must be repeated against the frozen Stage 5 backend and the final deployment;
production reverse-proxy routing also remains a deployment check.
