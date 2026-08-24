package stock

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (body *closeTrackingBody) Close() error {
	body.closed = true
	return nil
}

func TestNewClient(t *testing.T) {
	client := NewClient(ClientConfig{BaseURL: "http://stock.example/", Timeout: 0})
	if client.GetBaseURL() != "http://stock.example" {
		t.Fatalf("base URL = %q, want trailing slash removed", client.GetBaseURL())
	}
	if client.GetTimeout() != DefaultClientTimeout {
		t.Fatalf("timeout = %v, want %v", client.GetTimeout(), DefaultClientTimeout)
	}
}

func TestClientDoRejectsRelativeURL(t *testing.T) {
	client := NewClient(ClientConfig{BaseURL: "http://stock.example"})
	request, err := http.NewRequest(http.MethodGet, "/relative", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if _, err := client.Do(context.Background(), request); err == nil {
		t.Fatal("Do() expected relative URL error")
	}
}

func TestResolveProductsUsesStockContractAndPropagatesRequestID(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/internal/v1/products/resolve" {
			t.Errorf("request = %s %s, want POST /internal/v1/products/resolve", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := request.Header.Get("X-Request-ID"); got != "request-123" {
			t.Errorf("X-Request-ID = %q, want request-123", got)
		}

		var body ResolveRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.IDs) != 2 || body.IDs[0] != 7 || body.IDs[1] != 9 {
			t.Fatalf("ids = %#v, want [7 9]", body.IDs)
		}

		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(response).Encode(ResolveResponse{
			Products: map[int64]ResolvedProduct{
				7: {ID: 7, Code: "SKU-7", Description: "Seven", Balance: 4},
				9: {ID: 9, Code: "SKU-9", Description: "Nine", Balance: 0},
			},
			Missing: []int64{},
		})
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, Timeout: time.Second})
	products, err := client.ResolveProducts(context.Background(), []int64{7, 9}, "request-123")
	if err != nil {
		t.Fatalf("ResolveProducts() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("Stock calls = %d, want 1", calls)
	}
	if products[7].Description != "Seven" || products[9].Balance != 0 {
		t.Fatalf("products = %#v", products)
	}
}

func TestResolveProductsDoesNotFollowRedirects(t *testing.T) {
	var redirectTargetCalls int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/internal/v1/products/resolve":
			response.Header().Set("Location", "/redirected-resolve-success")
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusTemporaryRedirect)
			_, _ = io.WriteString(response, `{}`)
		case "/redirected-resolve-success":
			redirectTargetCalls++
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(ResolveResponse{
				Products: map[int64]ResolvedProduct{
					7: {ID: 7, Code: "SKU-7", Description: "Seven", Balance: 4},
				},
				Missing: []int64{},
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL})
	products, err := client.ResolveProducts(context.Background(), []int64{7}, "redirect-resolve")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("ResolveProducts() error = %v, want original 307 classified as ErrUnavailable", err)
	}
	if products != nil {
		t.Fatalf("ResolveProducts() products = %#v, want nil", products)
	}
	if redirectTargetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", redirectTargetCalls)
	}
}

func TestResolveProductsPreservesBusinessRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/problem+json")
		response.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(response).Encode(ErrorResponse{Error: ErrorBody{
			Code:      "PRODUCT_NOT_FOUND",
			Message:   "One or more requested products were not found.",
			Details:   nil,
			RequestID: "request-404",
		}})
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL})
	_, err := client.ResolveProducts(context.Background(), []int64{99}, "request-404")
	var serviceError *ServiceError
	if !errors.As(err, &serviceError) {
		t.Fatalf("error = %v, want ServiceError", err)
	}
	if serviceError.Status != http.StatusNotFound || serviceError.Code != "PRODUCT_NOT_FOUND" ||
		serviceError.RequestID != "request-404" || serviceError.Details != nil {
		t.Fatalf("ServiceError = %#v", serviceError)
	}
}

func TestResolveProductsClassifiesUnusableResponses(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		status      int
		body        string
	}{
		{
			name:        "non JSON content type",
			contentType: "text/plain",
			status:      http.StatusOK,
			body:        `{"products":{},"missing":[]}`,
		},
		{
			name:        "malformed JSON",
			contentType: "application/json",
			status:      http.StatusOK,
			body:        "{",
		},
		{
			name:        "unexpected product set",
			contentType: "application/json",
			status:      http.StatusOK,
			body:        `{"products":{"8":{"id":8,"code":"SKU-8","description":"Eight","balance":1}},"missing":[]}`,
		},
		{
			name:        "malformed error envelope",
			contentType: "application/json",
			status:      http.StatusInternalServerError,
			body:        `{"message":"database failure"}`,
		},
		{
			name:        "valid 5xx error envelope",
			contentType: "application/json",
			status:      http.StatusInternalServerError,
			body:        `{"error":{"code":"INTERNAL_ERROR","message":"failed","details":null,"request_id":"server-500"}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", test.contentType)
				response.WriteHeader(test.status)
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()

			client := NewClient(ClientConfig{BaseURL: server.URL})
			_, err := client.ResolveProducts(context.Background(), []int64{7}, "")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error = %v, want ErrUnavailable", err)
			}
			var serviceError *ServiceError
			if errors.As(err, &serviceError) {
				t.Fatalf("error = %#v, unusable response must not be ServiceError", serviceError)
			}
		})
	}
}

func TestResolveProductsRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, strings.Repeat("x", maxResponseBodyBytes+1))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL})
	_, err := client.ResolveProducts(context.Background(), []int64{1}, "")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want oversized ErrUnavailable", err)
	}
}

func TestConsumeUsesStockContractAndPropagatesRequestID(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/internal/v1/stock/consume" {
			t.Errorf("request = %s %s, want POST /internal/v1/stock/consume", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := request.Header.Get("X-Request-ID"); got != "consume-123" {
			t.Errorf("X-Request-ID = %q, want consume-123", got)
		}

		var body ConsumeRequest
		decoder := json.NewDecoder(request.Body)
		if err := decoder.Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		want := ConsumeRequest{
			InvoiceID:   42,
			OperationID: UUID{1},
			Items:       []ConsumeItem{{ProductID: 7, Quantity: 2}, {ProductID: 9, Quantity: 1}},
		}
		if body.OperationID != want.OperationID || body.InvoiceID != want.InvoiceID || len(body.Items) != 2 ||
			body.Items[0] != want.Items[0] || body.Items[1] != want.Items[1] {
			t.Fatalf("items = %#v, want %#v", body.Items, want.Items)
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{"balances": []map[string]any{
			{"product_id": 7, "balance": 8},
			{"product_id": 9, "balance": 0},
		}})
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, Timeout: time.Second})
	err := client.Consume(context.Background(), ConsumeRequest{
		InvoiceID:   42,
		OperationID: UUID{1},
		Items:       []ConsumeItem{{ProductID: 7, Quantity: 2}, {ProductID: 9, Quantity: 1}},
	}, "consume-123")
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("Stock calls = %d, want exactly one with no automatic retry", calls)
	}
}

func TestConsumeDoesNotFollowRedirects(t *testing.T) {
	var redirectTargetCalls int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/internal/v1/stock/consume":
			response.Header().Set("Location", "/redirected-consume-success")
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusTemporaryRedirect)
			_, _ = io.WriteString(response, `{}`)
		case "/redirected-consume-success":
			redirectTargetCalls++
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"balances":[{"product_id":7,"balance":8}]}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL})
	err := client.Consume(context.Background(), ConsumeRequest{
		InvoiceID:   42,
		OperationID: UUID{1},
		Items:       []ConsumeItem{{ProductID: 7, Quantity: 2}},
	}, "redirect-consume")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("Consume() error = %v, want original 307 classified as ErrUnavailable", err)
	}
	if redirectTargetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", redirectTargetCalls)
	}
}

func TestConsumePreservesBusinessRejections(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "insufficient stock", code: "INSUFFICIENT_STOCK"},
		{name: "idempotency conflict", code: "IDEMPOTENCY_CONFLICT"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(response).Encode(ErrorResponse{Error: ErrorBody{
					Code:      test.code,
					Message:   "Stock rejected the command.",
					Details:   map[string]any{"product_id": float64(7)},
					RequestID: "business-123",
				}})
			}))
			defer server.Close()

			client := NewClient(ClientConfig{BaseURL: server.URL})
			err := client.Consume(context.Background(), ConsumeRequest{}, "business-123")
			var serviceError *ServiceError
			if !errors.As(err, &serviceError) {
				t.Fatalf("error = %v (%T), want ServiceError", err, err)
			}
			if errors.Is(err, ErrUnavailable) {
				t.Fatalf("error = %v, business rejection must not be ErrUnavailable", err)
			}
			if serviceError.Status != http.StatusConflict || serviceError.Code != test.code ||
				serviceError.Message != "Stock rejected the command." || serviceError.RequestID != "business-123" {
				t.Fatalf("ServiceError = %#v", serviceError)
			}
			if serviceError.Details == nil {
				t.Fatalf("ServiceError details = nil, want preserved details")
			}
		})
	}
}

func TestConsumeRejectsSemanticallyInvalidSuccessResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty balances", body: `{"balances":[]}`},
		{name: "missing requested product", body: `{"balances":[{"product_id":7,"balance":8}]}`},
		{name: "extra product", body: `{"balances":[{"product_id":7,"balance":8},{"product_id":9,"balance":9},{"product_id":11,"balance":4}]}`},
		{name: "duplicate product", body: `{"balances":[{"product_id":7,"balance":8},{"product_id":7,"balance":8}]}`},
		{name: "unexpected product", body: `{"balances":[{"product_id":7,"balance":8},{"product_id":11,"balance":4}]}`},
		{name: "negative balance", body: `{"balances":[{"product_id":7,"balance":-1},{"product_id":9,"balance":9}]}`},
		{name: "missing balance", body: `{"balances":[{"product_id":7},{"product_id":9,"balance":9}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()

			client := NewClient(ClientConfig{BaseURL: server.URL})
			err := client.Consume(context.Background(), ConsumeRequest{Items: []ConsumeItem{
				{ProductID: 7, Quantity: 2},
				{ProductID: 9, Quantity: 1},
			}}, "unusable-123")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Consume() error = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestConsumeClassifiesUnusableHTTPResponses(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		status      int
		body        string
	}{
		{name: "non JSON success", contentType: "text/plain", status: http.StatusOK, body: `{"balances":[{"product_id":7,"balance":8}]}`},
		{name: "malformed success", contentType: "application/json", status: http.StatusOK, body: "{"},
		{name: "wrong balance type", contentType: "application/json", status: http.StatusOK, body: `{"balances":[{"product_id":7,"balance":"bad"}]}`},
		{name: "unknown success field", contentType: "application/json", status: http.StatusOK, body: `{"balances":[{"product_id":7,"balance":8}],"replayed":false}`},
		{name: "multiple JSON values", contentType: "application/json", status: http.StatusOK, body: `{"balances":[{"product_id":7,"balance":8}]} {}`},
		{name: "204 no content", contentType: "application/json", status: http.StatusNoContent, body: ""},
		{name: "malformed 4xx error", contentType: "application/json", status: http.StatusConflict, body: "{bad"},
		{name: "code-less 4xx error", contentType: "application/json", status: http.StatusConflict, body: `{"error":{"message":"conflict"}}`},
		{name: "valid 5xx error envelope", contentType: "application/json", status: http.StatusInternalServerError, body: `{"error":{"code":"INTERNAL_ERROR","message":"failed","details":null,"request_id":"server-500"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", test.contentType)
				response.WriteHeader(test.status)
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()

			client := NewClient(ClientConfig{BaseURL: server.URL})
			err := client.Consume(context.Background(), ConsumeRequest{
				Items: []ConsumeItem{{ProductID: 7, Quantity: 2}},
			}, "unusable-123")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Consume() error = %v, want ErrUnavailable", err)
			}
			var serviceError *ServiceError
			if errors.As(err, &serviceError) {
				t.Fatalf("Consume() error = %#v, unusable response must not be ServiceError", serviceError)
			}
		})
	}
}

func TestConsumeRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, strings.Repeat("x", maxResponseBodyBytes+1))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL})
	err := client.Consume(context.Background(), ConsumeRequest{
		Items: []ConsumeItem{{ProductID: 7, Quantity: 2}},
	}, "oversized-123")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Consume() error = %v, want oversized ErrUnavailable", err)
	}
}

func TestResolveProductsClassifiesNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	client := NewClient(ClientConfig{BaseURL: baseURL, Timeout: 100 * time.Millisecond})
	_, err := client.ResolveProducts(context.Background(), []int64{1}, "")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestResolveProductsClosesObtainedResponseBody(t *testing.T) {
	body := &closeTrackingBody{Reader: strings.NewReader(
		`{"products":{"1":{"id":1,"code":"SKU-1","description":"One","balance":1}},"missing":[]}`,
	)}
	client := NewClient(ClientConfig{BaseURL: "http://stock.example"})
	client.client.Transport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
		}, nil
	})

	if _, err := client.ResolveProducts(context.Background(), []int64{1}, ""); err != nil {
		t.Fatalf("ResolveProducts() error = %v", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}
