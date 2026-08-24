# API Contract

## Expected Endpoints

### Stock API

- `POST /api/v1/products` - Create a product; request body uses `{ code, description, balance }`; response is a single product object with snake_case timestamps.
- `GET /api/v1/products` - List all products as a direct JSON array whose items use snake_case timestamps.
- `GET /api/v1/products/:id` - Get a single product by ID; response is a single product object with snake_case timestamps.

### Billing API

- `POST /api/v1/invoices`
- `GET /api/v1/invoices`
- `GET /api/v1/invoices/:id`
- `POST /api/v1/invoices/:id/print`

## Request/Response Examples

### POST /api/v1/products

**Request:**

```json
{
  "code": "KEYBOARD-001",
  "description": "Mechanical keyboard",
  "balance": 12
}
```

**Response (201 Created):**

```json
{
  "id": 17,
  "code": "KEYBOARD-001",
  "description": "Mechanical keyboard",
  "balance": 12,
  "created_at": "2026-08-20T12:00:00Z",
  "updated_at": "2026-08-20T12:30:00Z"
}
```

**Frontend mapping:** The `ProductApiService` maps each direct-array DTO from snake_case timestamps (`created_at`, `updated_at`) to camelCase domain fields (`createdAt`, `updatedAt`) and converts the numeric backend `id` to the Angular model's string `id`.

### POST /api/v1/invoices

**Request:**

```json
{
  "items": [
    {
      "product_id": 17,
      "quantity": 2
    },
    {
      "product_id": 23,
      "quantity": 1
    }
  ]
}
```

**Response (201 Created):**

```json
{
  "id": 42,
  "number": 1,
  "status": "OPEN",
  "items": [
    {
      "product_id": 17,
      "product_code": "KEYBOARD-001",
      "product_description": "Mechanical keyboard",
      "quantity": 2
    },
    {
      "product_id": 23,
      "product_code": "MOUSE-001",
      "product_description": "Wireless mouse",
      "quantity": 1
    }
  ],
  "created_at": "2026-08-20T14:00:00Z",
  "closed_at": null
}
```

**Frontend mapping:** The `InvoiceApiService` maps the DTO by: converting numeric `id` to string, preserving the numeric `number` field, casting `status` to `'OPEN' | 'CLOSED'`, mapping `created_at` to `createdAt`, mapping nullable `closed_at` to optional `closedAt` (undefined when null), and transforming each item to capture a **product snapshot** (`productId`, `productCode`, `productDescription`, `quantity`). POST and GET endpoints preserve these item snapshots in their responses.

### GET /api/v1/invoices

**Response (200 OK) - Summary list without items:**

```json
[
  {
    "id": 42,
    "number": 1,
    "status": "OPEN",
    "created_at": "2026-08-20T14:00:00Z",
    "closed_at": null
  },
  {
    "id": 43,
    "number": 2,
    "status": "CLOSED",
    "created_at": "2026-08-20T15:00:00Z",
    "closed_at": "2026-08-20T15:05:00Z"
  }
]
```

**Frontend mapping:** The list endpoint returns a direct JSON array of summary DTOs without `items`. The frontend maps these to a dedicated `InvoiceSummary` domain model so list rendering cannot depend on missing item snapshots. Full detail remains represented by `Invoice`.

### GET /api/v1/invoices/:id

**Response (200 OK):**

Same full-detail shape as the POST response above, including `items`.

**Frontend mapping:** Same full snapshot and field mapping as POST. Unlike the summary list, this endpoint preserves `items` for historical detail rendering.

## Error Envelope

The backend uses a standard nested error envelope:

```json
{
  "error": {
    "code": "string",
    "message": "string",
    "details": null,
    "request_id": "stock-request-id"
  }
}
```

All error responses use this nested `error` envelope with a stable machine-readable `code`, a safe `message`, explicit `details` (including `null`), and the request correlation ID in `request_id`. The frontend selects its visible messages from stable known codes and uses a conservative fallback for malformed or unexpected responses instead of rendering arbitrary transport text.

## Important Error Codes

The frontend must handle the following explicit error codes from both Stock and Billing services:

- `VALIDATION_ERROR` - Invalid input data
- `PRODUCT_CODE_CONFLICT` - Duplicate product code
- `PRODUCT_NOT_FOUND` - Product does not exist
- `INVOICE_NOT_FOUND` - Invoice does not exist
- `INSUFFICIENT_STOCK` - Not enough stock for operation
- `INVOICE_NOT_OPEN` - Invoice is not in OPEN state
- `STOCK_SERVICE_UNAVAILABLE` - Stock microservice is down
- `IDEMPOTENCY_CONFLICT` - Idempotency key conflict

The frontend presents safe, user-facing messages based on these codes and never exposes raw error details, stack traces, or database internals.

When the backend supplies `request_id`, the frontend preserves it in the relevant failure state and exposes it as secondary diagnostic text. Invoice finalization failures also offer copy support for this ID. An `IDEMPOTENCY_CONFLICT` is treated as a safe conflict: the UI asks the user to reload/sync authoritative data and does not automatically resend the print request.

## Invoice Domain Contract Details

### Numeric Invoice Number

The backend generates a sequential numeric `number` field for each invoice. The frontend preserves this as a `number` type in the Angular `Invoice` model, distinct from the string `id`. Display uses the sequential number; internal references use the `id`.

### Nullable closed_at

An OPEN invoice has `closed_at: null` in the response. The frontend maps this to `closedAt: undefined` in the domain model. A CLOSED invoice has a non-null ISO-8601 timestamp string. This nullable contract allows the frontend to distinguish OPEN from CLOSED without relying on the `status` field alone.

### Direct-Array List Assumption

Both `GET /api/v1/products` and `GET /api/v1/invoices` return a direct JSON array of DTOs (not a wrapped object). The frontend assumes this contract and maps directly from array elements to domain models.

### Product Snapshot Rule

Invoice responses include `product_code` and `product_description` in each `items` entry. The frontend maps these to `InvoiceItem.productCode` and `InvoiceItem.productDescription`, creating an immutable snapshot. Invoice display **never** queries Stock Service for current product details, ensuring historical accuracy even if product descriptions change later.

### No Stock Change / No Frontend Finalization

The frontend **does not** implement stock decrement logic. The frontend **does not** optimistically mark invoices as CLOSED. Invoice finalization (print) is only performed through `POST /api/v1/invoices/:id/print`, and the UI only updates to CLOSED after receiving backend confirmation with a non-null `closed_at` and `status: 'CLOSED'`.

### Safe Error Handling

When Stock Service is unavailable, Billing Service returns HTTP 503 with a `STOCK_SERVICE_UNAVAILABLE` error code. The frontend:

- Stops any progress indicator
- Displays a clear, user-friendly message
- Keeps the invoice in its authoritative OPEN state
- Clears its processing indicator
- Preserves form data and provides a retry path
- Never infers stock changes or shows false success
