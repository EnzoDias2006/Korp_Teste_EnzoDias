# AGENTS.md — Web Frontend

## 1. Scope and Precedence

This file applies to every file under `web/`.

The repository-root `../AGENTS.md` remains the project-wide source of truth. This file narrows those rules to the Angular frontend and adds frontend-specific guidance. If the two files appear to conflict, stop and surface the conflict before changing architecture, public API contracts, or locked technologies.

The challenge specification remains the primary product source of truth.

---

## 2. Frontend Mission

Build a professional, easy-to-demonstrate Angular application for:

- registering and viewing products;
- registering invoices with multiple products and quantities;
- viewing invoice number and status;
- triggering the invoice print/finalization operation;
- showing immediate processing feedback;
- presenting safe, actionable backend errors;
- allowing recovery and retry when Stock Service is unavailable.

Prioritize, in order:

1. correct representation of confirmed backend state;
2. clear forms and validation;
3. recoverable failure UX;
4. modern, idiomatic Angular;
5. accessible Angular Material UI;
6. explicit, explainable code;
7. focused behavioral tests;
8. minimal dependencies.

This is a technical challenge, not a production ERP. Do not add enterprise complexity for appearance.

---

## 3. Locked Frontend Stack

Do not replace or bypass these choices without explicit project-owner approval:

- Angular 22;
- Standalone Components;
- Angular Router;
- Angular `HttpClient`;
- Angular Signals;
- RxJS;
- Typed Reactive Forms;
- Angular Material;
- SCSS;
- Vitest;
- Angular TestBed;
- Bun for dependency management and scripts.

Do not create NgModules for feature organization.

Do not introduce:

- NgRx or another state-management library;
- Tailwind, Bootstrap, or another component/styling system;
- third-party toast, spinner, HTTP-wrapper, or form libraries;
- npm, pnpm, or Yarn lockfiles;
- AI functionality.

Before proposing any new dependency, answer all four questions:

1. Which concrete requirement does it solve?
2. Why are Angular, Angular Material, RxJS, the browser platform, or an installed package insufficient?
3. Does it make the final technical explanation harder?
4. Does it overlap an existing responsibility?

If the case is not compelling, do not add it. Stop for approval before adding any runtime dependency.

---

## 4. Intended `web/` Layout

Prefer this layout, adapting only when actual cohesion or existing conventions justify it:

```text
web/
├── src/
│   ├── app/
│   │   ├── core/
│   │   │   ├── api/
│   │   │   ├── interceptors/
│   │   │   └── models/
│   │   ├── features/
│   │   │   ├── products/
│   │   │   │   ├── pages/
│   │   │   │   ├── components/
│   │   │   │   └── services/
│   │   │   └── invoices/
│   │   │       ├── pages/
│   │   │       ├── components/
│   │   │       └── services/
│   │   ├── shared/
│   │   │   └── components/
│   │   ├── app.config.ts
│   │   └── app.routes.ts
│   ├── environments/
│   └── styles.scss
├── package.json
└── bun.lock
```

Create a directory or extracted component only when it improves ownership, reuse, testing, or readability. Do not reproduce architecture diagrams as empty layers.

---

## 5. Frontend Boundary

The frontend owns:

- user interaction and navigation;
- typed client-side form validation;
- loading, saving, and finalizing UI state;
- presenting backend-confirmed products and invoices;
- presenting safe backend errors;
- feature composition and route-level orchestration.

The frontend must never:

- implement authoritative stock rules;
- access PostgreSQL or share backend persistence concerns;
- decrement or infer stock locally;
- generate authoritative invoice numbers;
- mark an invoice `CLOSED` optimistically;
- report finalization success before Billing Service confirms it;
- hide an unavailable-service error behind a false success state.

System flow:

```text
Angular Web
  ├──> Stock Service: product operations
  └──> Billing Service: invoice operations
                              └──> Stock Service: stock finalization
```

Billing Service, not the browser, orchestrates invoice finalization with Stock Service.

---

## 6. Domain and API Models

Use explicit domain vocabulary:

```text
Product
Invoice
InvoiceItem
Stock
Balance
Quantity
Open
Closed
Finalize
```

Keep source code, identifiers, API paths, tests, and technical documentation in English. The visible UI may be in Portuguese.

Avoid vague names such as `Data`, `Manager`, `Helper`, `Util`, `Processor`, and `Common` unless the responsibility is genuinely generic.

Model API payloads explicitly. Do not use `any`. Preserve invoice item snapshots returned by Billing Service; historical invoice display must not depend on the current product description.

The backend error contract is:

```json
{
  "code": "STOCK_SERVICE_UNAVAILABLE",
  "message": "Could not update product stock.",
  "details": null
}
```

Frontend handling rules:

- treat `code` as the stable machine-readable value;
- display a safe, understandable message;
- never render raw database, stack-trace, or transport internals;
- distinguish validation/conflict errors from service unavailability;
- provide a conservative fallback for malformed or unexpected error responses;
- preserve form data and retryability when the operation can recover.

Do not silently change an established public API contract. Stop for approval first.

---

## 7. Modern Angular Rules

Use:

- Standalone Components;
- `app.config.ts` for application providers;
- `app.routes.ts` for routing;
- `inject()` when it keeps dependency use concise and clear;
- modern template control flow: `@if`, `@for`, and `@switch` where appropriate;
- `ChangeDetectionStrategy.OnPush` for application components unless there is a concrete reason not to;
- `DestroyRef` and `takeUntilDestroyed` for lifecycle-bound subscriptions.

Use lifecycle APIs deliberately and keep their purpose explainable for the final video. Do not add lifecycle hooks merely to demonstrate that they exist.

Prefer route-level lazy loading for feature pages when it stays simple. Keep route configuration explicit and easy to follow.

Templates must remain readable. Move non-trivial transformation or orchestration to component code, computed signals, or feature services rather than embedding complex expressions in HTML.

---

## 8. Component Responsibilities

Prefer:

- page components for route-level loading and feature orchestration;
- feature-local API/domain services for HTTP interaction;
- presentational components when reuse or genuine view complexity justifies extraction;
- shared components only for proven cross-feature reuse.

A page component may coordinate form state, API calls, and feedback, but must not become a container for unrelated UI sections, transport details, and complex transformations.

Avoid both extremes:

- giant components that own every concern;
- excessive fragmentation into components with no meaningful ownership or reuse.

Components must not call `fetch` directly. Use Angular `HttpClient` through focused services.

---

## 9. Signals and RxJS

Use Signals for synchronous UI and application state, including:

- loading and saving flags;
- invoice finalization state;
- selected product or invoice;
- current product and invoice collections;
- UI errors;
- derived values via `computed`.

Use RxJS where it naturally models asynchronous work, especially `HttpClient` flows.

Good, explainable operators include:

- `catchError` for intentional error mapping or recovery;
- `finalize` for resetting processing state;
- `switchMap` for a dependent request or route-driven flow;
- `forkJoin` only for genuinely independent requests whose combined result is needed;
- `takeUntilDestroyed` for lifecycle-bound subscriptions.

Avoid:

- nested subscriptions;
- complex operator chains for synchronous state;
- subscriptions used only to copy values without a reason;
- converting every signal to an observable or every observable to a signal;
- manual subscription cleanup when Angular's lifecycle utilities fit.

Every RxJS chain should have a clear asynchronous purpose that can be explained in the final technical video.

---

## 10. Typed Reactive Forms

Use Typed Reactive Forms for product and invoice input.

Requirements:

- controls are strongly typed;
- validators are explicit;
- required text is trimmed or rejected intentionally;
- product codes and quantities follow the backend contract;
- balance and quantity reject invalid negative, zero, fractional, or out-of-range values as the contract requires;
- an invoice supports multiple items;
- duplicate product selections are prevented or handled explicitly;
- the form is disabled while a destructive/finalizing action is in flight;
- server validation errors are mapped clearly without destroying entered values.

Do not adopt Signal Forms unless the project owner explicitly changes the locked decision.

Frontend validation improves UX; it never replaces backend validation.

---

## 11. Angular Material and UX

Use Angular Material for UI needs it already covers, such as:

- tables;
- form fields and inputs;
- selects;
- buttons;
- dialogs;
- snackbars;
- progress spinners;
- icons;
- toolbars and cards.

Use SCSS for focused application styling. The UI should look intentional and professional without decorative complexity.

Accessibility expectations:

- every control has an accessible label;
- validation errors are associated with their controls;
- keyboard interaction remains usable;
- icon-only actions have accessible names/tooltips;
- status is not communicated by color alone;
- progress and disabled states are perceivable;
- destructive or finalizing actions are unambiguous.

Prefer responsive layouts that remain usable at common laptop and narrow viewport widths.

---

## 12. Invoice Print/Finalize UX

Treat “print” as the business operation that finalizes an invoice.

When the user triggers it:

1. expose the action only for an `OPEN` invoice;
2. show processing feedback immediately;
3. disable repeated clicks while the request is running;
4. call Billing Service and wait for confirmation;
5. on success, replace local data with the confirmed `CLOSED` invoice and show success feedback;
6. on failure, clear processing state, keep the invoice recoverable, show a meaningful message, and allow retry when appropriate.

When Stock Service is unavailable, Billing Service should return a safe `503` error. The UI must:

- stop the progress indicator;
- explain that finalization could not complete;
- keep the invoice visibly `OPEN` or reload its authoritative state;
- preserve a safe retry path.

Never infer that stock was changed. Never show a closed/success state from an optimistic mutation.

Repeated user actions and retry flows must not create duplicate frontend requests while an earlier request is still pending. Backend idempotency remains the authority for duplicated network requests and lost responses.

---

## 13. Configuration and Local Development

Configure Stock and Billing API base URLs through Angular environment/configuration or a development proxy. Prefer same-origin proxy routing for local development when convenient.

Never:

- hard-code secrets;
- commit credentials;
- expose backend database URLs;
- place server-only values in frontend configuration.

Use Bun consistently:

```bash
bun install
bun run start
bun run lint
bun run test
bun run build
```

Commit `bun.lock`. Do not introduce `package-lock.json`, `pnpm-lock.yaml`, or Yarn lockfiles.

Use the existing scripts before adding aliases or new tooling.

---

## 14. Frontend Testing

Use Vitest, Angular TestBed, and `HttpTestingController` where appropriate.

Prioritize behavior over implementation details:

- product form validation;
- invoice form validation and multiple items;
- product API request/response behavior;
- invoice API request/response behavior;
- rendering safe API errors;
- finalization processing state;
- prevention of repeated clicks;
- successful finalization and confirmed `CLOSED` state;
- unavailable Stock Service feedback;
- retry behavior;
- disabled/hidden print action for non-open invoices.

Tests must not assert optimistic closure or mock away the behavior under test. Use focused fixtures and avoid brittle assertions against unrelated markup.

For each frontend change, run the narrowest relevant tests first, then the applicable project checks.

Expected full checks:

```bash
bun run lint
bun run test
bun run build
```

Run formatting checks if configured. Never claim a check passed unless it was executed.

---

## 15. Change Discipline

Before editing:

1. inspect the existing implementation and scripts;
2. understand local conventions;
3. identify the smallest valid change;
4. check the established API contract;
5. avoid unrelated refactors.

After editing:

1. format touched files;
2. run relevant focused tests;
3. run applicable lint/build checks;
4. update documentation when UI/API behavior or an architectural decision changed;
5. report exactly what changed and what was verified;
6. explicitly name checks that were not run.

Do not weaken validation, skip tests, or rewrite unrelated files to make a check pass.

---

## 16. Frontend Definition of Done

A frontend feature is done only when all applicable items are true:

- the business flow is implemented against real API boundaries;
- forms are typed and validated;
- loading, empty, success, and error states are handled;
- authoritative backend state is preserved;
- finalization cannot be triggered for a non-open invoice;
- repeated finalization clicks are prevented;
- unavailable-service failure is understandable and retryable;
- important behavior has focused tests;
- formatting, linting, tests, and production build pass as applicable;
- no unnecessary dependency was introduced;
- technical documentation is updated when decisions changed;
- the behavior is easy to demonstrate in the final video.

Project-level acceptance from the frontend perspective includes:

- create and list a persisted product;
- create an invoice with multiple valid items;
- display its backend-generated sequential number and `OPEN` status;
- finalize it with visible processing feedback;
- display `CLOSED` only after backend success;
- show a recoverable error while Stock Service is down;
- succeed on retry after Stock Service returns;
- never imply duplicated stock effects.

---

## 17. Technical Explanation Readiness

As important decisions are implemented, update `../docs/technical-details.md` with concise, current notes covering:

- why Standalone Components were used;
- which lifecycle APIs/hooks are used and why;
- where Signals represent synchronous state;
- where RxJS represents asynchronous flows;
- why Typed Reactive Forms were chosen;
- why Angular Material was chosen;
- how backend errors and unavailable-service recovery appear in the UI;
- why invoice closure is never optimistic.

Do not leave these explanations to memory at the end of the challenge.

---

## 18. Stop Conditions

Stop and ask the project owner before:

- adding a runtime dependency;
- replacing any locked technology;
- adding a state-management or UI library;
- adopting Signal Forms;
- changing an established public API contract;
- moving authoritative business rules into the frontend;
- changing Stock/Billing service boundaries;
- implementing AI;
- making a large unrelated architectural refactor.

Always prefer explicit code, focused components, existing Angular capabilities, one useful behavioral test, and a recoverable UI flow over generic abstractions or framework accumulation.
