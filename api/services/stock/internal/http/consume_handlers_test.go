package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EnzoDias2006/korp-api/services/stock/internal/product"
)

func postConsume(t *testing.T, router http.Handler, contentType string, body any) (*httptest.ResponseRecorder, map[string]any) {
	return postConsumeWithRequestID(t, router, contentType, body, "")
}

func postConsumeWithRequestID(
	t *testing.T,
	router http.Handler,
	contentType string,
	body any,
	requestID string,
) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	request := httptest.NewRequest(http.MethodPost, "/internal/v1/stock/consume", reader)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var decoded map[string]any
	if recorder.Body.Len() > 0 && json.Unmarshal(recorder.Body.Bytes(), &decoded) != nil {
		t.Fatalf("decode response %q", recorder.Body.String())
	}
	return recorder, decoded
}

func TestConsumeStock_UnexpectedFailureLogsCorrelationFields(t *testing.T) {
	var logs bytes.Buffer
	serviceError := errors.New("consume persistence failed")
	service := &mockProductService{consumeFunc: func(context.Context, product.ConsumeInput) (product.ConsumeResult, error) {
		return product.ConsumeResult{}, serviceError
	}}
	router := NewRouter(nil, slog.New(slog.NewJSONHandler(&logs, nil)), nil, service)
	operationID := product.OperationID{9, 8, 7}
	body := stockConsumeInput{
		InvoiceID:   pointerTo(int64(42)),
		OperationID: pointerTo(operationID),
		Items:       []stockConsumeItem{{ProductID: pointerTo(int64(3)), Quantity: pointerTo(1)}},
	}

	recorder, response := postConsumeWithRequestID(t, router, "application/json", body, "consume-log-123")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	requireError(t, response, "INTERNAL_ERROR")

	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatalf("decode slog record: %v; log = %q", err, logs.String())
	}
	if record["service"] != "stock" || record["request_id"] != "consume-log-123" ||
		record["invoice_id"] != float64(42) || record["operation"] != "consume_stock" ||
		record["status"] != float64(http.StatusInternalServerError) || record["error"] != serviceError.Error() {
		t.Fatalf("unexpected slog record: %#v", record)
	}
	if record["operation_id"] != operationID.String() {
		t.Fatalf("operation_id = %#v, want %q", record["operation_id"], operationID.String())
	}
}

func requireError(t *testing.T, response map[string]any, code string) string {
	t.Helper()

	errorBody, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no nested error object: %#v", response)
	}
	if errorBody["code"] != code {
		t.Fatalf("error code = %#v, want %q", errorBody["code"], code)
	}
	if _, exists := errorBody["details"]; !exists {
		t.Fatal("error details field is absent")
	}
	requestID, _ := errorBody["request_id"].(string)
	if requestID == "" {
		t.Fatal("request_id is empty")
	}
	return requestID
}

func TestConsumeStock_StrictTransport(t *testing.T) {
	router := NewRouter(nil, slog.New(slog.DiscardHandler), nil, &mockProductService{})

	recorder, response := postConsume(t, router, "text/plain", stockConsumeInput{})
	if recorder.Code != http.StatusBadRequest || requireError(t, response, "MALFORMED_REQUEST") == "" {
		t.Fatalf("status = %d, response = %#v", recorder.Code, response)
	}

	recorder, response = postConsume(t, router, "application/json", map[string]any{"operation_id": "x"})
	if recorder.Code != http.StatusBadRequest || requireError(t, response, "MALFORMED_REQUEST") == "" {
		t.Fatalf("unknown field: status = %d, response = %#v", recorder.Code, response)
	}

	recorder, response = postConsume(t, router, "application/json", map[string]any{
		"invoice_id": 1,
		"items":      []map[string]any{},
	})
	if recorder.Code != http.StatusUnprocessableEntity || requireError(t, response, "VALIDATION_ERROR") == "" {
		t.Fatalf("empty items: status = %d, response = %#v", recorder.Code, response)
	}
}

func TestConsumeStock_SuccessAndErrors(t *testing.T) {
	service := &mockProductService{
		consumeFunc: func(_ context.Context, input product.ConsumeInput) (product.ConsumeResult, error) {
			if input.InvoiceID != 42 || len(input.Items) != 2 {
				t.Fatalf("unexpected domain input: %+v", input)
			}
			return product.ConsumeResult{Balances: map[int64]int{9: 1, 4: 0}}, nil
		},
	}
	router := NewRouter(nil, slog.New(slog.DiscardHandler), nil, service)
	body := stockConsumeInput{
		InvoiceID:   pointerTo(int64(42)),
		OperationID: pointerTo(product.OperationID{1}),
		Items: []stockConsumeItem{
			{ProductID: pointerTo(int64(9)), Quantity: pointerTo(2)},
			{ProductID: pointerTo(int64(4)), Quantity: pointerTo(3)},
		},
	}

	recorder, response := postConsume(t, router, "application/json", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	raw, _ := json.Marshal(response)
	var result stockConsumeResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode typed response: %v", err)
	}
	want := []stockBalanceResponse{{ProductID: 4, Balance: 0}, {ProductID: 9, Balance: 1}}
	if len(result.Balances) != len(want) || result.Balances[0] != want[0] || result.Balances[1] != want[1] {
		t.Fatalf("balances = %#v, want %#v", result.Balances, want)
	}

	service.consumeFunc = func(context.Context, product.ConsumeInput) (product.ConsumeResult, error) {
		return product.ConsumeResult{}, fmt.Errorf("wrapped: %w", product.ErrNotFound)
	}
	recorder, response = postConsume(t, router, "application/json", body)
	if recorder.Code != http.StatusNotFound || requireError(t, response, "PRODUCT_NOT_FOUND") == "" {
		t.Fatalf("not found: status = %d, response = %#v", recorder.Code, response)
	}

	service.consumeFunc = func(context.Context, product.ConsumeInput) (product.ConsumeResult, error) {
		return product.ConsumeResult{}, fmt.Errorf("wrapped: %w", product.ErrInsufficientStock)
	}
	recorder, response = postConsume(t, router, "application/json", body)
	if recorder.Code != http.StatusConflict || requireError(t, response, "INSUFFICIENT_STOCK") == "" {
		t.Fatalf("insufficient: status = %d, response = %#v", recorder.Code, response)
	}
}

func TestConsumeStock_ReplaysSameOperationOnce(t *testing.T) {
	if consumeTestDB == nil {
		t.Skip("STOCK_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	ctx := context.Background()
	if _, err := consumeTestDB.Exec(ctx, "TRUNCATE TABLE consumption_operations, products RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("reset idempotency test data: %v", err)
	}
	repository := product.NewRepository(consumeTestDB)
	createdProduct, err := repository.Create(ctx, "REPLAY", "Replay", 2)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	router := NewRouter(nil, slog.New(slog.DiscardHandler), nil, product.NewService(repository))

	request := stockConsumeInput{
		InvoiceID:   pointerTo(int64(88)),
		OperationID: pointerTo(product.OperationID{9}),
		Items:       []stockConsumeItem{{ProductID: pointerTo(createdProduct.ID), Quantity: pointerTo(1)}},
	}
	for attempt := 0; attempt < 2; attempt++ {
		recorder, _ := postConsume(t, router, "application/json", request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, body = %s", attempt+1, recorder.Code, recorder.Body.String())
		}
	}

	var balance int
	if err := consumeTestDB.QueryRow(ctx,
		`SELECT balance FROM products WHERE id = $1`, createdProduct.ID,
	).Scan(&balance); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if balance != 1 {
		t.Fatalf("balance = %d, want exactly one decrement", balance)
	}

	conflicting := request
	conflicting.Items = []stockConsumeItem{{ProductID: pointerTo(createdProduct.ID), Quantity: pointerTo(2)}}
	recorder, response := postConsume(t, router, "application/json", conflicting)
	if recorder.Code != http.StatusConflict || requireError(t, response, "IDEMPOTENCY_CONFLICT") == "" {
		t.Fatalf("conflict status = %d, response = %#v", recorder.Code, response)
	}
}

func pointerTo[T any](value T) *T {
	return &value
}
