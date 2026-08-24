# Frontend Architecture

## System boundary

```text
Angular Web
  |-- product requests ----------> Stock Service
  `-- invoice requests ----------> Billing Service
                                      `-- finalization --> Stock Service
```

The browser never decrements stock, generates invoice numbers, or infers that an invoice closed.
Billing owns invoice finalization and its call to Stock. The UI replaces local invoice data only
with the response confirmed by Billing.

## Application structure

```text
src/app/
|-- core/
|   |-- interceptors/request-id.interceptor.ts
|   `-- models/api-error.model.ts
|-- features/
|   |-- products/
|   |   |-- pages/
|   |   |-- services/product-api.service.ts
|   |   `-- products.routes.ts
|   `-- invoices/
|       |-- pages/
|       |-- services/invoice-api.service.ts
|       `-- invoices.routes.ts
|-- app.config.ts
|-- app.routes.ts
`-- app.ts
```

All application components are standalone and use `ChangeDetectionStrategy.OnPush`. There are no
feature NgModules or global state libraries. The app has no `zone.js` dependency or zone-based
provider, so it runs with Angular 22's default zoneless behavior.

## Routing

The root route redirects to `/products`. `app.routes.ts` lazy-loads the product and invoice route
arrays with `loadChildren`; those arrays lazy-load each route page with `loadComponent`.

| Route           | Page responsibility                                    |
| --------------- | ------------------------------------------------------ |
| `/products`     | Load and display products from Stock                   |
| `/products/new` | Validate and create a product                          |
| `/invoices`     | Load and display invoice summaries                     |
| `/invoices/new` | Load products and create a multi-item invoice          |
| `/invoices/:id` | Load, display, finalize, recover, and print an invoice |

There is currently no wildcard/not-found route.

## Pages, services, and state

Route pages own form and screen state. `ProductApiService` and `InvoiceApiService` own `HttpClient`
calls and DTO-to-domain mapping. Neither service owns UI state or renders transport messages.

Signals hold synchronous state such as collections, loading/saving flags, safe errors, request
references, and the derived `InvoiceList.hasData` and `InvoiceDetail.canPrint` values. RxJS models
HTTP and route-param work; subscriptions update Signals directly, so the templates do not use the
async pipe.

Lifecycle use is intentionally small:

| Component       | Initialization                                                                  |
| --------------- | ------------------------------------------------------------------------------- |
| `ProductList`   | Its constructor starts the initial product load; it does not implement `OnInit` |
| `ProductCreate` | No lifecycle hook                                                               |
| `InvoiceList`   | `ngOnInit` loads invoices                                                       |
| `InvoiceCreate` | `ngOnInit` adds the first item and loads products                               |
| `InvoiceDetail` | `ngOnInit` subscribes to route changes and retry requests                       |

Every page with a subscription injects `DestroyRef` and applies `takeUntilDestroyed`; no page
implements manual `ngOnDestroy` cleanup.

## Forms and API contracts

Product and invoice entry use Typed Reactive Forms. Product fields reject missing and
whitespace-only text, while balance is non-negative and integral. Invoice items use a typed
`FormArray`, require positive integral quantities, and reject duplicate product selections.
Submitting forms preserves values on recoverable server errors.

API DTOs are declared explicitly and mapped at the service boundary. Snake-case timestamps become
camel-case model properties, numeric identifiers become frontend strings where the domain model
requires them, and invoice item descriptions remain Billing-provided snapshots. Historical invoice
details therefore do not depend on the current Product description.

Invoice status is parsed at runtime and accepts exactly `OPEN` or `CLOSED`. Unknown statuses reject
the response instead of becoming a false `CLOSED` state. The print mapping additionally requires a
confirmed `CLOSED` response with a non-empty `closed_at` before the browser print action can run.

## HTTP and errors

`app.config.ts` registers `HttpClient` with the functional `requestIdInterceptor`. Each outgoing
request is cloned with a generated UUID in the `X-Request-ID` header.

The shared safe error mapper reads the nested backend envelope, maps known codes to fixed Portuguese
messages, and ignores arbitrary backend/transport text. A valid nested `request_id` is preserved for
support. Unknown or malformed payloads use a conservative fallback; transport status `0` and `503`
map to `STOCK_SERVICE_UNAVAILABLE` unless a known nested code is authoritative.

Recoverable page flows keep entered values and expose manual retry. Invoice product loading shows
an in-page retry and keeps the form unavailable until products exist; a successful empty response
links to product creation. Invoice finalization disables duplicate submissions, keeps the invoice
`OPEN` on failure, and updates it only from the confirmed Billing response.

## UI stack

The application currently uses these Angular Material areas:

- toolbar, cards, buttons, icons, and tooltips;
- form fields, inputs, and selects;
- progress spinners and snackbars;
- display-only tables.

No `MatSort` integration or table sorting is implemented. SCSS remains local to components and
provides responsive overflow/wrapping plus print-specific invoice layout. Angular CDK is installed
as Material's foundation/peer dependency; application code does not import a CDK API directly.

## Environment and deployment

Development configuration calls Stock at `http://localhost:8081/api/v1` and Billing at
`http://localhost:8082/api/v1`. There is no Angular development proxy. Production uses the relative
base `/api/v1` for both services and therefore requires a deployment reverse proxy or gateway that
can route product and invoice endpoints to the correct backend.

## Verification boundary

Vitest, Angular TestBed, and `HttpTestingController` cover local behavior and API mapping. Angular
ESLint checks TypeScript and templates; production builds validate Angular compilation and bundle
budgets. There is no configured browser E2E runner, so real service availability, deployment
routing, browser printing, keyboard use, and viewport behavior remain manual integration checks.
