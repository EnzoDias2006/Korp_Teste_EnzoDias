# HTTP contract

This document contains only routes registered by the current Stock and Billing routers. There are no filtering, pagination, update or delete endpoints.

## Conventions

- Stock base URL: `http://localhost:8081`.
- Billing base URL: `http://localhost:8082`.
- Domain payloads and responses are JSON.
- Every response carries `X-Request-ID`; an incoming value is preserved, otherwise the service generates one.
- Billing propagates `X-Request-ID` to Stock for resolve/consume calls.
- Timestamps are UTC RFC 3339 values.
- IDs are positive signed 64-bit integers unless stated otherwise.

Expected status meanings:

| Status | Meaning |
|---:|---|
| `200` | successful read or action with a body |
| `201` | resource created |
| `204` | allowed CORS preflight |
| `400` | malformed transport input or invalid path ID |
| `404` | missing Product or Invoice |
| `409` | domain or idempotency conflict |
| `422` | JSON is valid but domain input is invalid |
| `503` | owned database unavailable or Billing cannot obtain a usable Stock response |
| `500` | unexpected server failure |

## Error envelope

Every documented application and middleware error uses this nested shape:

```json
{
  "error": {
    "code": "MALFORMED_REQUEST",
    "message": "The request body is malformed or contains invalid JSON.",
    "details": null,
    "request_id": "request-abc123"
  }
}
```

`error.code` is stable and machine-readable. `message` and `details` are client-safe. `request_id` correlates the response with logs. SQL, stack traces, database URLs, credentials and raw network errors are never returned.

Implemented codes:

| Code | Status | Use |
|---|---:|---|
| `MALFORMED_REQUEST` | `400` | invalid JSON transport, content type, body size/shape or unknown fields |
| `INVALID_PRODUCT_ID` | `400` | invalid Product path ID |
| `INVALID_INVOICE_ID` | `400` | invalid Invoice path ID |
| `VALIDATION_ERROR` | `422` | semantically invalid body |
| `PRODUCT_NOT_FOUND` | `404` | requested Product is missing |
| `INVOICE_NOT_FOUND` | `404` | requested Invoice is missing |
| `PRODUCT_CODE_CONFLICT` | `409` | normalized Product code already exists |
| `INVOICE_NOT_OPEN` | `409` | print requires an `OPEN` Invoice |
| `INSUFFICIENT_STOCK` | `409` | at least one Product cannot satisfy the command |
| `IDEMPOTENCY_CONFLICT` | `409` | operation identity conflicts with another logical command |
| `STOCK_SERVICE_UNAVAILABLE` | `503` | Stock request failed or its response was unusable |
| `DATABASE_UNAVAILABLE` | `503` | service-owned database failed readiness ping |
| `INTERNAL_ERROR` | `500` | unexpected handler/application failure |
| `INTERNAL_SERVER_ERROR` | `500` | panic recovered by middleware |

## Health and CORS

Both services register:

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/health/live` | HTTP process is serving |
| `GET` | `/health/ready` | owned PostgreSQL database responds within two seconds |

Stock health success bodies are `{"status":"live"}` and `{"status":"ready"}`. Billing additionally includes `request_id` in its health success body. Readiness failure is `503 DATABASE_UNAVAILABLE`.

For an `Origin` present in `CORS_ALLOWED_ORIGINS`, both services return:

```text
Access-Control-Allow-Origin: <exact configured origin>
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Headers: Content-Type, X-Request-ID
Access-Control-Expose-Headers: X-Request-ID
Vary: Origin
```

An allowed `OPTIONS` preflight returns `204`. A nonmatching preflight also stops with `204` but receives no CORS allow headers. Credentials and wildcard origins are not enabled.

## Stock Product API

### POST /api/v1/products

Creates a Product. The body is limited to 64 KiB and must contain one JSON object with exactly the documented fields.

Request:

```json
{
  "code": "PROD001",
  "description": "Test Product",
  "balance": 100
}
```

Success — `201 Created`:

```json
{
  "id": 1,
  "code": "PROD001",
  "description": "Test Product",
  "balance": 100,
  "created_at": "2026-08-24T12:00:00Z",
  "updated_at": "2026-08-24T12:00:00Z"
}
```

`code` is trimmed and normalized to uppercase; `description` is trimmed. `balance` is required, may be zero and may not be negative.

Errors:

- `400 MALFORMED_REQUEST`: malformed JSON, non-JSON content type, body over 64 KiB, multiple JSON values or unknown field;
- `422 VALIDATION_ERROR`: empty code/description, absent/null balance or negative balance;
- `409 PRODUCT_CODE_CONFLICT`: normalized code already exists;
- `500 INTERNAL_ERROR`: unexpected persistence failure.

Duplicate example:

```json
{
  "error": {
    "code": "PRODUCT_CODE_CONFLICT",
    "message": "A product with the same code already exists.",
    "details": null,
    "request_id": "product-create-2"
  }
}
```

### GET /api/v1/products

Success — `200 OK`:

```json
[
  {
    "id": 1,
    "code": "PROD001",
    "description": "Test Product",
    "balance": 100,
    "created_at": "2026-08-24T12:00:00Z",
    "updated_at": "2026-08-24T12:00:00Z"
  }
]
```

The direct array is ordered by `id` ascending. Empty result is `[]`. Query parameters do not provide filtering or pagination.

### GET /api/v1/products/:id

Success is the complete Product representation above with `200 OK`.

Errors:

- `400 INVALID_PRODUCT_ID`: path ID is not a positive integer;
- `404 PRODUCT_NOT_FOUND`: no Product has that ID;
- `500 INTERNAL_ERROR`: unexpected persistence failure.

## Stock internal HTTP boundary

These routes are used by Billing but remain normal HTTP contracts; Billing never imports Stock persistence.

### POST /internal/v1/products/resolve

Resolves a batch without changing balance. The body is limited to 64 KiB.

Request:

```json
{
  "ids": [1, 2]
}
```

Success — `200 OK`:

```json
{
  "products": {
    "1": {
      "id": 1,
      "code": "PROD001",
      "description": "Test Product",
      "balance": 100
    },
    "2": {
      "id": 2,
      "code": "PROD002",
      "description": "Another Product",
      "balance": 50
    }
  },
  "missing": []
}
```

Map keys are decimal string forms of Product IDs in JSON. An empty `ids` array is valid and returns empty `products`/`missing`. If any requested Product is absent, the endpoint returns an error rather than partial success.

Errors:

- `400 MALFORMED_REQUEST`: strict transport failure;
- `422 VALIDATION_ERROR`: duplicate or non-positive ID;
- `404 PRODUCT_NOT_FOUND`: at least one Product is missing;
- `500 INTERNAL_ERROR`: unexpected failure.

### POST /internal/v1/stock/consume

Atomically consumes one or more distinct Product quantities.

Request:

```json
{
  "invoice_id": 1,
  "operation_id": "11111111-1111-4111-8111-111111111111",
  "items": [
    {"product_id": 1, "quantity": 2},
    {"product_id": 2, "quantity": 1}
  ]
}
```

`invoice_id`, Product IDs and quantities must be positive. `operation_id` must be a non-zero UUID. At least one item is required and Product IDs may not repeat.

Success — `200 OK`:

```json
{
  "balances": [
    {"product_id": 1, "balance": 98},
    {"product_id": 2, "balance": 49}
  ]
}
```

Balances are ordered by `product_id` ascending. The first successful command stores this exact per-product outcome. Repeating the same operation ID and canonical command returns the stored outcome with `200`, even if later operations changed current Product balances; it never decrements twice.

Stock fingerprints the invoice ID plus sorted Product/quantity pairs. Reordering identical items therefore identifies the same command. Reusing the operation ID with a different invoice, Product set or quantity is a conflict.

Errors:

- `400 MALFORMED_REQUEST`: strict transport failure or invalid UUID encoding;
- `422 VALIDATION_ERROR`: zero UUID, absent/non-positive invoice ID, empty items, duplicate/non-positive Product ID or non-positive quantity;
- `404 PRODUCT_NOT_FOUND`: at least one Product is missing; no balance changes;
- `409 INSUFFICIENT_STOCK`: at least one Product lacks balance; no balance changes;
- `409 IDEMPOTENCY_CONFLICT`: operation ID was used by another canonical command;
- `500 INTERNAL_ERROR`: unexpected transaction failure or incomplete legacy replay result.

Idempotency conflict example:

```json
{
  "error": {
    "code": "IDEMPOTENCY_CONFLICT",
    "message": "The operation ID was already used with a different command.",
    "details": null,
    "request_id": "consume-conflict-1"
  }
}
```

Insufficient Stock response from this route:

```json
{
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "message": "One or more products do not have enough stock.",
    "details": null,
    "request_id": "consume-2"
  }
}
```

## Billing Invoice API

### POST /api/v1/invoices

Request:

```json
{
  "items": [
    {"product_id": 1, "quantity": 2},
    {"product_id": 2, "quantity": 1}
  ]
}
```

The body is limited to 64 KiB and requires one or more distinct positive Product IDs with positive quantities. Callers cannot supply snapshot fields. Billing performs one Stock resolve call, validates the complete response, then stores trusted code/description snapshots in one local transaction. Creation never consumes stock.

Success — `201 Created`:

```json
{
  "id": 1,
  "number": 1,
  "status": "OPEN",
  "created_at": "2026-08-24T12:05:00Z",
  "closed_at": null,
  "items": [
    {
      "product_id": 1,
      "product_code": "PROD001",
      "product_description": "Test Product",
      "quantity": 2
    },
    {
      "product_id": 2,
      "product_code": "PROD002",
      "product_description": "Another Product",
      "quantity": 1
    }
  ]
}
```

Errors:

- `400 MALFORMED_REQUEST`: strict transport failure;
- `422 VALIDATION_ERROR`: no items, duplicate/non-positive Product ID or non-positive quantity;
- `404 PRODUCT_NOT_FOUND`: Stock reports a missing Product;
- `503 STOCK_SERVICE_UNAVAILABLE`: timeout/network failure, Stock 5xx or unusable Stock response;
- `500 INTERNAL_ERROR`: unexpected Billing persistence failure.

### GET /api/v1/invoices

Success — `200 OK`:

```json
[
  {
    "id": 1,
    "number": 1,
    "status": "OPEN",
    "created_at": "2026-08-24T12:05:00Z",
    "closed_at": null
  }
]
```

The direct summary array is ordered by `number`, then `id`, ascending. Empty result is `[]`. Items are intentionally omitted from summaries. Filtering and pagination are outside this challenge contract.

### GET /api/v1/invoices/:id

Success uses the complete Invoice representation shown for creation, including snapshots. It returns `200 OK`.

Errors:

- `400 INVALID_INVOICE_ID`: path ID is not a positive integer;
- `404 INVOICE_NOT_FOUND`: no Invoice has that ID;
- `500 INTERNAL_ERROR`: unexpected persistence failure.

### POST /api/v1/invoices/:id/print

The request needs no body. It finalizes an `OPEN` Invoice:

1. Billing loads the Invoice and persisted quantities;
2. Billing claims or reuses one durable operation UUID;
3. Stock atomically consumes or replays that command;
4. Billing validates Stock's complete success response;
5. Billing closes the Invoice and marks the operation complete in one local transaction.

Success — `200 OK`:

```json
{
  "id": 1,
  "number": 1,
  "status": "CLOSED",
  "created_at": "2026-08-24T12:05:00Z",
  "closed_at": "2026-08-24T12:06:00Z",
  "items": [
    {
      "product_id": 1,
      "product_code": "PROD001",
      "product_description": "Test Product",
      "quantity": 2
    },
    {
      "product_id": 2,
      "product_code": "PROD002",
      "product_description": "Another Product",
      "quantity": 1
    }
  ]
}
```

Errors:

- `400 INVALID_INVOICE_ID`: path ID is not a positive integer;
- `404 INVOICE_NOT_FOUND`: Invoice does not exist;
- `409 INVOICE_NOT_OPEN`: Invoice is already closed or this request lost the conditional close race;
- `409 INSUFFICIENT_STOCK`: Stock rejected the atomic command;
- `409 IDEMPOTENCY_CONFLICT`: durable operation conflicts with another Stock command;
- `503 STOCK_SERVICE_UNAVAILABLE`: no timely, valid and complete Stock confirmation;
- `500 INTERNAL_ERROR`: unexpected local finalization failure.

Required public error examples follow.

Insufficient Stock:

```json
{
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "message": "There is insufficient stock to print the invoice.",
    "details": null,
    "request_id": "print-insufficient-1"
  }
}
```

Already closed or lost close race:

```json
{
  "error": {
    "code": "INVOICE_NOT_OPEN",
    "message": "Only an OPEN invoice can be printed.",
    "details": null,
    "request_id": "print-repeat-1"
  }
}
```

Stock unavailable or unusable:

```json
{
  "error": {
    "code": "STOCK_SERVICE_UNAVAILABLE",
    "message": "Could not update product stock.",
    "details": null,
    "request_id": "print-offline-1"
  }
}
```

Finalization operation conflict:

```json
{
  "error": {
    "code": "IDEMPOTENCY_CONFLICT",
    "message": "The finalization operation conflicts with a prior command.",
    "details": null,
    "request_id": "print-conflict-1"
  }
}
```

If Stock commits but Billing does not receive a usable response, Billing returns `503` and the Invoice remains `OPEN`. A retry uses the same durable operation ID. Stock replays its stored outcome without a second decrement; Billing then closes. `X-Request-ID` may differ between retries because it is correlation metadata, not operation identity.

## Invoice item representation

An Invoice item contains exactly:

| Field | Meaning |
|---|---|
| `product_id` | Stock Product identity |
| `product_code` | historical code snapshot |
| `product_description` | historical description snapshot |
| `quantity` | positive invoiced quantity |

`unit_price`, `stock_balance`, `total_price` and `total_amount` are not part of this challenge's contract.

## Enforced invariants

- Product code is unique after normalization.
- Product balance never becomes negative.
- Invoice number is generated by PostgreSQL and unique.
- Invoice contains at least one item, with no duplicate Product ID.
- Item Product IDs and quantities are positive.
- Invoice is created `OPEN` with trusted historical snapshots.
- Creating an Invoice does not consume stock.
- Only Stock changes Product balance.
- Multi-item consumption is all-or-nothing.
- Only `OPEN` may transition to `CLOSED`; `CLOSED` is final.
- Billing closes only after usable Stock success.
- Identical retry consumes exactly once and replays the stored result.
- Two consumers cannot drive balance below zero.
