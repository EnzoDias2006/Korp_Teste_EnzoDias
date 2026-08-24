package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/invoice"
	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
)

type invoiceServiceStub struct {
	create      func(context.Context, invoice.CreateInput) (invoice.Invoice, error)
	list        func(context.Context) ([]invoice.Invoice, error)
	get         func(context.Context, int64) (invoice.Invoice, error)
	print       func(context.Context, int64, invoice.StockConsumer, string) (invoice.Invoice, error)
	operationID [16]byte
}

func (stub *invoiceServiceStub) Create(ctx context.Context, input invoice.CreateInput) (invoice.Invoice, error) {
	if stub.create == nil {
		return invoice.Invoice{}, errors.New("unexpected Create call")
	}
	return stub.create(ctx, input)
}

func (stub *invoiceServiceStub) List(ctx context.Context) ([]invoice.Invoice, error) {
	if stub.list == nil {
		return nil, errors.New("unexpected List call")
	}
	return stub.list(ctx)
}

func (stub *invoiceServiceStub) GetByID(ctx context.Context, id int64) (invoice.Invoice, error) {
	if stub.get == nil {
		return invoice.Invoice{}, errors.New("unexpected GetByID call")
	}
	return stub.get(ctx, id)
}

func (stub *invoiceServiceStub) Print(
	ctx context.Context,
	id int64,
	consumer invoice.StockConsumer,
	requestID string,
) (invoice.Invoice, [16]byte, error) {
	if stub.print == nil {
		return invoice.Invoice{}, [16]byte{}, errors.New("unexpected Print call")
	}
	found, err := stub.print(ctx, id, consumer, requestID)
	return found, stub.operationID, err
}

type productResolverStub struct {
	resolve func(context.Context, []int64, string) (map[int64]stock.ResolvedProduct, error)
	consume func(context.Context, stock.ConsumeRequest, string) error
}

func (stub *productResolverStub) Consume(ctx context.Context, request stock.ConsumeRequest, requestID string) error {
	if stub.consume == nil {
		return errors.New("unexpected Consume call")
	}
	return stub.consume(ctx, request, requestID)
}

func (stub *productResolverStub) ResolveProducts(
	ctx context.Context,
	ids []int64,
	requestID string,
) (map[int64]stock.ResolvedProduct, error) {
	return stub.resolve(ctx, ids, requestID)
}

func TestCreateInvoiceResolvesOnceAndPersistsTrustedSnapshots(t *testing.T) {
	createdAt := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	resolveCalls := 0
	resolver := &productResolverStub{resolve: func(_ context.Context, ids []int64, requestID string) (map[int64]stock.ResolvedProduct, error) {
		resolveCalls++
		if len(ids) != 2 || ids[0] != 9 || ids[1] != 4 {
			t.Fatalf("resolved IDs = %#v, want [9 4]", ids)
		}
		if requestID != "invoice-create-123" {
			t.Fatalf("request ID = %q, want invoice-create-123", requestID)
		}
		return map[int64]stock.ResolvedProduct{
			9: {ID: 9, Code: "TRUSTED-9", Description: "Trusted nine", Balance: 8},
			4: {ID: 4, Code: "TRUSTED-4", Description: "Trusted four", Balance: 3},
		}, nil
	}}
	service := &invoiceServiceStub{create: func(_ context.Context, input invoice.CreateInput) (invoice.Invoice, error) {
		want := []invoice.CreateItem{
			{ProductID: 9, ProductCode: "TRUSTED-9", ProductDescription: "Trusted nine", Quantity: 2},
			{ProductID: 4, ProductCode: "TRUSTED-4", ProductDescription: "Trusted four", Quantity: 1},
		}
		if len(input.Items) != len(want) {
			t.Fatalf("Create items = %#v, want %#v", input.Items, want)
		}
		for index := range want {
			if input.Items[index] != want[index] {
				t.Fatalf("Create item %d = %#v, want %#v", index, input.Items[index], want[index])
			}
		}
		return invoice.Invoice{
			ID:        11,
			Number:    42,
			Status:    invoice.StatusOpen,
			CreatedAt: createdAt,
			Items: []invoice.Item{
				{ProductID: 9, ProductCode: "TRUSTED-9", ProductDescription: "Trusted nine", Quantity: 2},
				{ProductID: 4, ProductCode: "TRUSTED-4", ProductDescription: "Trusted four", Quantity: 1},
			},
		}, nil
	}}

	response := performInvoiceRequest(
		t,
		service,
		resolver,
		http.MethodPost,
		"/api/v1/invoices",
		`{"items":[{"product_id":9,"quantity":2},{"product_id":4,"quantity":1}]}`,
		"invoice-create-123",
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if resolveCalls != 1 {
		t.Fatalf("resolve calls = %d, want 1", resolveCalls)
	}
	if got := response.Header().Get("X-Request-ID"); got != "invoice-create-123" {
		t.Fatalf("X-Request-ID = %q, want invoice-create-123", got)
	}

	var body invoiceResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Number != 42 || body.Status != "OPEN" || body.ClosedAt != nil || len(body.Items) != 2 {
		t.Fatalf("response = %#v", body)
	}
	if strings.Contains(response.Body.String(), "balance") {
		t.Fatalf("invoice response leaked current Stock balance: %s", response.Body.String())
	}
}

func TestCreateInvoiceRejectsSemanticInputBeforeDependencies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "items required", body: `{"items":[]}`},
		{name: "product required", body: `{"items":[{"quantity":1}]}`},
		{name: "positive quantity", body: `{"items":[{"product_id":1,"quantity":0}]}`},
		{name: "duplicate product", body: `{"items":[{"product_id":1,"quantity":1},{"product_id":1,"quantity":2}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolverCalled := false
			resolver := &productResolverStub{resolve: func(context.Context, []int64, string) (map[int64]stock.ResolvedProduct, error) {
				resolverCalled = true
				return nil, nil
			}}
			response := performInvoiceRequest(
				t,
				&invoiceServiceStub{},
				resolver,
				http.MethodPost,
				"/api/v1/invoices",
				test.body,
				"validation-123",
			)
			assertErrorResponse(t, response, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "validation-123")
			if resolverCalled {
				t.Fatal("Stock resolver was called for semantically invalid input")
			}
		})
	}
}

func TestCreateInvoiceRejectsMalformedTransport(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
	}{
		{name: "invalid JSON", body: "{", contentType: "application/json"},
		{name: "unknown field", body: `{"items":[],"number":7}`, contentType: "application/json"},
		{name: "multiple values", body: `{"items":[]} {"items":[]}`, contentType: "application/json"},
		{name: "wrong content type", body: `{"items":[]}`, contentType: "text/plain"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/invoices", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("X-Request-ID", "malformed-123")
			response := httptest.NewRecorder()
			newInvoiceRouter(&invoiceServiceStub{}, &productResolverStub{}).ServeHTTP(response, request)
			assertErrorResponse(t, response, http.StatusBadRequest, "MALFORMED_REQUEST", "malformed-123")
		})
	}
}

func TestCreateInvoiceMapsStockFailures(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "missing product",
			err:    &stock.ServiceError{Code: "PRODUCT_NOT_FOUND", Status: http.StatusNotFound},
			status: http.StatusNotFound,
			code:   "PRODUCT_NOT_FOUND",
		},
		{
			name:   "unavailable",
			err:    stock.ErrUnavailable,
			status: http.StatusServiceUnavailable,
			code:   "STOCK_SERVICE_UNAVAILABLE",
		},
		{
			name:   "stock internal failure",
			err:    &stock.ServiceError{Code: "INTERNAL_ERROR", Status: http.StatusInternalServerError},
			status: http.StatusServiceUnavailable,
			code:   "STOCK_SERVICE_UNAVAILABLE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serviceCalled := false
			service := &invoiceServiceStub{create: func(context.Context, invoice.CreateInput) (invoice.Invoice, error) {
				serviceCalled = true
				return invoice.Invoice{}, nil
			}}
			resolver := &productResolverStub{resolve: func(context.Context, []int64, string) (map[int64]stock.ResolvedProduct, error) {
				return nil, test.err
			}}
			response := performInvoiceRequest(
				t,
				service,
				resolver,
				http.MethodPost,
				"/api/v1/invoices",
				`{"items":[{"product_id":1,"quantity":1}]}`,
				"stock-error-123",
			)
			assertErrorResponse(t, response, test.status, test.code, "stock-error-123")
			if serviceCalled {
				t.Fatal("invoice persistence was called after Stock failure")
			}
		})
	}
}

func TestInvoiceQueryRoutes(t *testing.T) {
	createdAt := time.Date(2026, time.August, 21, 15, 0, 0, 0, time.UTC)
	want := invoice.Invoice{
		ID:        5,
		Number:    12,
		Status:    invoice.StatusOpen,
		CreatedAt: createdAt,
		Items: []invoice.Item{{
			ProductID: 3, ProductCode: "SKU-3", ProductDescription: "Three", Quantity: 2,
		}},
	}
	service := &invoiceServiceStub{
		list: func(context.Context) ([]invoice.Invoice, error) {
			return []invoice.Invoice{want}, nil
		},
		get: func(_ context.Context, id int64) (invoice.Invoice, error) {
			if id != want.ID {
				t.Fatalf("id = %d, want %d", id, want.ID)
			}
			return want, nil
		},
	}
	resolver := &productResolverStub{resolve: func(context.Context, []int64, string) (map[int64]stock.ResolvedProduct, error) {
		return nil, errors.New("unexpected resolve call")
	}}

	listResponse := performInvoiceRequest(t, service, resolver, http.MethodGet, "/api/v1/invoices", "", "list-123")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listResponse.Code, listResponse.Body.String())
	}
	var summaries []map[string]any
	if err := json.Unmarshal(listResponse.Body.Bytes(), &summaries); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(summaries) != 1 || summaries[0]["status"] != "OPEN" {
		t.Fatalf("list response = %#v", summaries)
	}
	if _, hasItems := summaries[0]["items"]; hasItems {
		t.Fatalf("list summary unexpectedly contains items: %#v", summaries[0])
	}

	detailResponse := performInvoiceRequest(t, service, resolver, http.MethodGet, "/api/v1/invoices/5", "", "detail-123")
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", detailResponse.Code, detailResponse.Body.String())
	}
	var detail invoiceResponse
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Items) != 1 || detail.Items[0].ProductDescription != "Three" {
		t.Fatalf("detail response = %#v", detail)
	}
}

func TestPrintInvoiceReturnsFinalizedInvoiceAfterOneStockCall(t *testing.T) {
	closedAt := time.Date(2026, time.August, 22, 11, 0, 0, 0, time.UTC)
	var consumeCalls int
	resolver := &productResolverStub{consume: func(_ context.Context, request stock.ConsumeRequest, requestID string) error {
		consumeCalls++
		if requestID != "print-123" || len(request.Items) != 1 || request.Items[0] != (stock.ConsumeItem{ProductID: 3, Quantity: 2}) {
			t.Fatalf("Consume(request = %#v, requestID = %q)", request, requestID)
		}
		return nil
	}}
	service := &invoiceServiceStub{print: func(_ context.Context, id int64, consumer invoice.StockConsumer, requestID string) (invoice.Invoice, error) {
		if id != 5 || requestID != "print-123" {
			t.Fatalf("Print(id = %d, requestID = %q)", id, requestID)
		}
		if err := consumer.Consume(context.Background(), stock.ConsumeRequest{Items: []stock.ConsumeItem{
			{ProductID: 3, Quantity: 2},
		}}, "print-123"); err != nil {
			return invoice.Invoice{}, err
		}
		return invoice.Invoice{
			ID: 5, Number: 2, Status: invoice.StatusClosed, ClosedAt: &closedAt,
			Items: []invoice.Item{{ProductID: 3, ProductCode: "SKU-3", ProductDescription: "Three", Quantity: 2}},
		}, nil
	}}

	response := performInvoiceRequest(t, service, resolver, http.MethodPost, "/api/v1/invoices/5/print", "", "print-123")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if consumeCalls != 1 {
		t.Fatalf("consume calls = %d, want one", consumeCalls)
	}
	var body invoiceResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "CLOSED" || body.ClosedAt == nil || len(body.Items) != 1 {
		t.Fatalf("response = %#v", body)
	}
}

func TestPrintInvoiceMapsDomainAndStockErrorsWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		code       string
		consumed   bool
		closeCalls int
	}{
		{name: "missing", err: invoice.ErrNotFound, status: http.StatusNotFound, code: "INVOICE_NOT_FOUND"},
		{name: "closed", err: invoice.ErrNotOpen, status: http.StatusConflict, code: "INVOICE_NOT_OPEN"},
		{
			name: "insufficient stock",
			err: &stock.ServiceError{
				Code: "INSUFFICIENT_STOCK", Message: "Insufficient stock.", Status: http.StatusConflict,
			},
			status: http.StatusConflict, code: "INSUFFICIENT_STOCK",
		},
		{
			name:   "stock unavailable",
			err:    stock.ErrUnavailable,
			status: http.StatusServiceUnavailable, code: "STOCK_SERVICE_UNAVAILABLE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &productResolverStub{consume: func(context.Context, stock.ConsumeRequest, string) error {
				return test.err
			}}
			service := &invoiceServiceStub{print: func(
				_ context.Context,
				_ int64,
				consumer invoice.StockConsumer,
				_ string,
			) (invoice.Invoice, error) {
				if err := consumer.Consume(context.Background(), stock.ConsumeRequest{}, "error-123"); err != nil {
					return invoice.Invoice{}, err
				}
				return invoice.Invoice{}, nil
			}}

			response := performInvoiceRequest(
				t, service, resolver, http.MethodPost, "/api/v1/invoices/5/print", "", "error-123",
			)
			assertErrorResponse(t, response, test.status, test.code, "error-123")
		})
	}
}

func TestPrintInvoiceFailureLogsStableOperationContext(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{name: "stock unavailable", err: stock.ErrUnavailable, status: http.StatusServiceUnavailable, message: "stock consumption unavailable"},
		{name: "unexpected", err: errors.New("complete finalization failed"), status: http.StatusInternalServerError, message: "invoice print failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			operationID := [16]byte{6, 5, 4}
			service := &invoiceServiceStub{
				operationID: operationID,
				print: func(context.Context, int64, invoice.StockConsumer, string) (invoice.Invoice, error) {
					return invoice.Invoice{}, test.err
				},
			}
			resolver := &productResolverStub{consume: func(context.Context, stock.ConsumeRequest, string) error {
				return nil
			}}
			logger := slog.New(slog.NewJSONHandler(&logs, nil))

			response := performInvoiceRequestWithLogger(
				t, service, resolver, logger, http.MethodPost, "/api/v1/invoices/5/print", "", "print-log-123",
			)
			assertErrorResponse(t, response, test.status, map[int]string{
				http.StatusServiceUnavailable:  "STOCK_SERVICE_UNAVAILABLE",
				http.StatusInternalServerError: "INTERNAL_ERROR",
			}[test.status], "print-log-123")

			var record map[string]any
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatalf("decode slog record: %v; log = %q", err, logs.String())
			}
			if record["msg"] != test.message || record["service"] != "billing" ||
				record["request_id"] != "print-log-123" || record["invoice_id"] != float64(5) ||
				record["operation"] != "print_invoice" || record["status"] != float64(test.status) ||
				record["error"] != test.err.Error() {
				t.Fatalf("unexpected slog record: %#v", record)
			}
			operationText, err := stock.UUID(operationID).MarshalText()
			if err != nil {
				t.Fatalf("format operation ID: %v", err)
			}
			if record["operation_id"] != string(operationText) {
				t.Fatalf("operation_id = %#v, want %q", record["operation_id"], operationText)
			}
		})
	}
}

func TestGetInvoiceMapsInvalidAndMissingIDs(t *testing.T) {
	resolver := &productResolverStub{resolve: func(context.Context, []int64, string) (map[int64]stock.ResolvedProduct, error) {
		return nil, nil
	}}
	service := &invoiceServiceStub{get: func(context.Context, int64) (invoice.Invoice, error) {
		return invoice.Invoice{}, invoice.ErrNotFound
	}}

	invalid := performInvoiceRequest(t, service, resolver, http.MethodGet, "/api/v1/invoices/not-a-number", "", "invalid-id-123")
	assertErrorResponse(t, invalid, http.StatusBadRequest, "INVALID_INVOICE_ID", "invalid-id-123")

	missing := performInvoiceRequest(t, service, resolver, http.MethodGet, "/api/v1/invoices/99", "", "missing-id-123")
	assertErrorResponse(t, missing, http.StatusNotFound, "INVOICE_NOT_FOUND", "missing-id-123")
}

func TestInvoiceUnexpectedPersistenceErrorsAreSafe(t *testing.T) {
	service := &invoiceServiceStub{
		create: func(context.Context, invoice.CreateInput) (invoice.Invoice, error) {
			return invoice.Invoice{}, errors.New("insert invoice: password=secret")
		},
		list: func(context.Context) ([]invoice.Invoice, error) {
			return nil, errors.New("list invoices: password=secret")
		},
		get: func(context.Context, int64) (invoice.Invoice, error) {
			return invoice.Invoice{}, errors.New("get invoice: password=secret")
		},
	}
	resolver := &productResolverStub{resolve: func(context.Context, []int64, string) (map[int64]stock.ResolvedProduct, error) {
		return map[int64]stock.ResolvedProduct{
			1: {ID: 1, Code: "SKU-1", Description: "One", Balance: 1},
		}, nil
	}}

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/invoices", body: `{"items":[{"product_id":1,"quantity":1}]}`},
		{method: http.MethodGet, path: "/api/v1/invoices"},
		{method: http.MethodGet, path: "/api/v1/invoices/1"},
	}
	for _, request := range requests {
		response := performInvoiceRequest(t, service, resolver, request.method, request.path, request.body, "internal-123")
		assertErrorResponse(t, response, http.StatusInternalServerError, "INTERNAL_ERROR", "internal-123")
		if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "insert") {
			t.Fatalf("response leaked internal error: %s", response.Body.String())
		}
	}
}

func performInvoiceRequest(
	t *testing.T,
	service invoiceService,
	resolver productResolver,
	method string,
	path string,
	body string,
	requestID string,
) *httptest.ResponseRecorder {
	return performInvoiceRequestWithLogger(t, service, resolver, slog.New(slog.NewTextHandler(io.Discard, nil)), method, path, body, requestID)
}

func performInvoiceRequestWithLogger(
	t *testing.T,
	service invoiceService,
	resolver productResolver,
	logger *slog.Logger,
	method string,
	path string,
	body string,
	requestID string,
) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody io.Reader
	if body != "" {
		requestBody = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, requestBody)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Request-ID", requestID)
	response := httptest.NewRecorder()
	NewRouter(pingStub{}, logger, nil, service, resolver).ServeHTTP(response, request)
	return response
}

func newInvoiceRouter(service invoiceService, resolver productResolver) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(pingStub{}, logger, nil, service, resolver)
}

func assertErrorResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	status int,
	code string,
	requestID string,
) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d: %s", response.Code, status, response.Body.String())
	}
	var body ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != code || body.Error.RequestID != requestID || body.Error.Details != nil {
		t.Fatalf("error response = %#v", body)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"details":null`)) {
		t.Fatalf("error response omits explicit null details: %s", response.Body.String())
	}
	if got := response.Header().Get("X-Request-ID"); got != requestID {
		t.Fatalf("X-Request-ID = %q, want %q", got, requestID)
	}
}
