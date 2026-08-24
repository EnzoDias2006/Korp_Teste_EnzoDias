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

	"github.com/EnzoDias2006/korp-api/services/stock/internal/http/middleware"
	"github.com/EnzoDias2006/korp-api/services/stock/internal/product"
	"github.com/gin-gonic/gin"
)

// mockProductService is a test double for product.Service.
type mockProductService struct {
	createFunc  func(ctx context.Context, input product.CreateInput) (product.Product, error)
	listFunc    func(ctx context.Context) ([]product.Product, error)
	getByIDFunc func(ctx context.Context, id int64) (product.Product, error)
	resolveFunc func(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error)
	consumeFunc func(ctx context.Context, input product.ConsumeInput) (product.ConsumeResult, error)
}

func (m *mockProductService) Create(ctx context.Context, input product.CreateInput) (product.Product, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, input)
	}
	return product.Product{}, nil
}

func (m *mockProductService) List(ctx context.Context) ([]product.Product, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return nil, nil
}

func (m *mockProductService) GetByID(ctx context.Context, id int64) (product.Product, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return product.Product{}, nil
}

func (m *mockProductService) Resolve(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, input)
	}
	return product.ResolveResult{
		Products: make(map[int64]product.Product),
		Missing:  []int64{},
	}, nil
}

func (m *mockProductService) Consume(ctx context.Context, input product.ConsumeInput) (product.ConsumeResult, bool, error) {
	if m.consumeFunc != nil {
		result, err := m.consumeFunc(ctx, input)
		return result, false, err
	}
	return product.ConsumeResult{}, false, nil
}

func newTestProductHandlers(service productService) *ProductHandlers {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewProductHandlers(service, logger)
}

func TestCreateProduct(t *testing.T) {
	// Setup fake service
	service := &mockProductService{
		createFunc: func(ctx context.Context, input product.CreateInput) (product.Product, error) {
			return product.Product{
				ID:          1,
				Code:        "TEST001",
				Description: "Test Product",
				Balance:     100,
			}, nil
		},
	}
	handlers := newTestProductHandlers(service)

	t.Run("valid creation returns 201", func(t *testing.T) {
		router := setupTestRouter(handlers)
		body := `{"code":"TEST001","description":"Test Product","balance":100}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-req-1")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
		}

		// Check X-Request-ID header is preserved
		if rec.Header().Get("X-Request-ID") != "test-req-1" {
			t.Errorf("X-Request-ID = %q, want %q", rec.Header().Get("X-Request-ID"), "test-req-1")
		}

		// Verify response structure
		var response map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Check all required fields are present
		requiredFields := []string{"id", "code", "description", "balance", "created_at", "updated_at"}
		for _, field := range requiredFields {
			if _, ok := response[field]; !ok {
				t.Errorf("response missing field: %s", field)
			}
		}
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {
		router := setupTestRouter(handlers)
		body := `{invalid json`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-req-2")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		// Verify error envelope
		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "MALFORMED_REQUEST" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "MALFORMED_REQUEST")
		}
		if errResp.Error.RequestID != "test-req-2" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-req-2")
		}
		if errResp.Error.Details != nil {
			t.Errorf("details = %#v, want nil", errResp.Error.Details)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"details":null`)) {
			t.Errorf("response does not contain explicit null details: %s", rec.Body.String())
		}
	})

	t.Run("missing balance field returns 422", func(t *testing.T) {
		router := setupTestRouter(handlers)
		body := `{"code":"TEST001","description":"Test Product"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-req-3")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
		}
	})

	t.Run("null balance field returns 422 without calling service", func(t *testing.T) {
		called := false
		service := &mockProductService{
			createFunc: func(context.Context, product.CreateInput) (product.Product, error) {
				called = true
				return product.Product{}, nil
			},
		}
		router := setupTestRouter(newTestProductHandlers(service))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(
			`{"code":"TEST001","description":"Test Product","balance":null}`,
		))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}
		if called {
			t.Fatal("service was called for a missing required balance")
		}
	})

	t.Run("unknown fields returns 400", func(t *testing.T) {
		router := setupTestRouter(handlers)
		body := `{"code":"TEST001","description":"Test Product","balance":100,"extra":"field"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-req-4")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "MALFORMED_REQUEST" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "MALFORMED_REQUEST")
		}
	})

	t.Run("non JSON content type returns 400", func(t *testing.T) {
		router := setupTestRouter(handlers)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(
			`{"code":"TEST001","description":"Test Product","balance":100}`,
		))
		req.Header.Set("Content-Type", "text/plain")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("oversized body returns 400", func(t *testing.T) {
		router := setupTestRouter(handlers)
		body := `{"code":"` + strings.Repeat("A", maxProductCreateBodyBytes) + `","description":"Test","balance":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("zero balance is valid", func(t *testing.T) {
		service := &mockProductService{
			createFunc: func(ctx context.Context, input product.CreateInput) (product.Product, error) {
				return product.Product{
					ID:          2,
					Code:        "TEST002",
					Description: "Zero Balance Product",
					Balance:     0,
				}, nil
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"code":"TEST002","description":"Zero Balance Product","balance":0}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
	})

	t.Run("semantic validation error returns 422", func(t *testing.T) {
		service := &mockProductService{
			createFunc: func(ctx context.Context, input product.CreateInput) (product.Product, error) {
				return product.Product{}, product.ErrValidation
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"code":"","description":"","balance":-1}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-req-5")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
		}
		if errResp.Error.RequestID != "test-req-5" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-req-5")
		}
	})

	t.Run("duplicate code returns 409", func(t *testing.T) {
		service := &mockProductService{
			createFunc: func(ctx context.Context, input product.CreateInput) (product.Product, error) {
				return product.Product{}, product.ErrCodeConflict
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"code":"DUP001","description":"Duplicate","balance":10}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-req-6")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "PRODUCT_CODE_CONFLICT" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "PRODUCT_CODE_CONFLICT")
		}
		if errResp.Error.RequestID != "test-req-6" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-req-6")
		}
	})

	t.Run("unexpected error returns 500", func(t *testing.T) {
		var logs bytes.Buffer
		service := &mockProductService{
			createFunc: func(ctx context.Context, input product.CreateInput) (product.Product, error) {
				return product.Product{}, errors.New("unexpected database error")
			},
		}
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		handlers := NewProductHandlers(service, logger)
		router := setupTestRouter(handlers)

		body := `{"code":"ERR001","description":"Error","balance":10}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-req-7")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "INTERNAL_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "INTERNAL_ERROR")
		}
		if errResp.Error.RequestID != "test-req-7" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-req-7")
		}
		if strings.Contains(rec.Body.String(), "database") {
			t.Fatalf("response leaked internal error: %s", rec.Body.String())
		}
		if !strings.Contains(logs.String(), `"request_id":"test-req-7"`) ||
			!strings.Contains(logs.String(), `"operation":"create_product"`) {
			t.Fatalf("unexpected structured log: %s", logs.String())
		}
	})
}

func TestListProducts(t *testing.T) {
	t.Run("returns 200 with product list", func(t *testing.T) {
		products := []product.Product{
			{ID: 1, Code: "P001", Description: "Product 1", Balance: 100},
			{ID: 2, Code: "P002", Description: "Product 2", Balance: 50},
		}
		service := &mockProductService{
			listFunc: func(ctx context.Context) ([]product.Product, error) {
				return products, nil
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
		req.Header.Set("X-Request-ID", "test-req-8")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		// Check X-Request-ID header is preserved
		if rec.Header().Get("X-Request-ID") != "test-req-8" {
			t.Errorf("X-Request-ID = %q, want %q", rec.Header().Get("X-Request-ID"), "test-req-8")
		}

		var response []productResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(response) != 2 {
			t.Errorf("products length = %d, want %d", len(response), 2)
		}
	})

	t.Run("unexpected error returns 500", func(t *testing.T) {
		service := &mockProductService{
			listFunc: func(ctx context.Context) ([]product.Product, error) {
				return nil, errors.New("unexpected error")
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
		req.Header.Set("X-Request-ID", "test-req-9")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "INTERNAL_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "INTERNAL_ERROR")
		}
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		service := &mockProductService{
			listFunc: func(ctx context.Context) ([]product.Product, error) {
				return []product.Product{}, nil
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var response []productResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if response == nil {
			t.Fatal("response is null, want an empty array")
		}
		if len(response) != 0 {
			t.Errorf("products length = %d, want %d", len(response), 0)
		}
	})
}

func TestGetProduct(t *testing.T) {
	t.Run("valid ID returns 200", func(t *testing.T) {
		service := &mockProductService{
			getByIDFunc: func(ctx context.Context, id int64) (product.Product, error) {
				return product.Product{
					ID:          1,
					Code:        "P001",
					Description: "Test Product",
					Balance:     100,
				}, nil
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products/1", nil)
		req.Header.Set("X-Request-ID", "test-req-10")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		// Check X-Request-ID header is preserved
		if rec.Header().Get("X-Request-ID") != "test-req-10" {
			t.Errorf("X-Request-ID = %q, want %q", rec.Header().Get("X-Request-ID"), "test-req-10")
		}

		// Verify response structure
		var response map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Check all required fields are present
		requiredFields := []string{"id", "code", "description", "balance", "created_at", "updated_at"}
		for _, field := range requiredFields {
			if _, ok := response[field]; !ok {
				t.Errorf("response missing field: %s", field)
			}
		}
	})

	t.Run("invalid ID returns 400", func(t *testing.T) {
		service := &mockProductService{}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products/invalid", nil)
		req.Header.Set("X-Request-ID", "test-req-11")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "INVALID_PRODUCT_ID" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "INVALID_PRODUCT_ID")
		}
		if errResp.Error.RequestID != "test-req-11" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-req-11")
		}
	})

	t.Run("non-positive ID returns 400", func(t *testing.T) {
		service := &mockProductService{}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		// Test with 0
		req := httptest.NewRequest(http.MethodGet, "/api/v1/products/0", nil)
		req.Header.Set("X-Request-ID", "test-req-12")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d (for ID 0)", rec.Code, http.StatusBadRequest)
		}

		// Test with negative
		req = httptest.NewRequest(http.MethodGet, "/api/v1/products/-1", nil)
		req.Header.Set("X-Request-ID", "test-req-13")
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d (for ID -1)", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		service := &mockProductService{
			getByIDFunc: func(ctx context.Context, id int64) (product.Product, error) {
				return product.Product{}, product.ErrNotFound
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products/9999", nil)
		req.Header.Set("X-Request-ID", "test-req-14")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "PRODUCT_NOT_FOUND" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "PRODUCT_NOT_FOUND")
		}
		if errResp.Error.RequestID != "test-req-14" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-req-14")
		}
	})

	t.Run("unexpected error returns 500", func(t *testing.T) {
		service := &mockProductService{
			getByIDFunc: func(ctx context.Context, id int64) (product.Product, error) {
				return product.Product{}, errors.New("unexpected error")
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/products/1", nil)
		req.Header.Set("X-Request-ID", "test-req-15")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "INTERNAL_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "INTERNAL_ERROR")
		}
	})
}

// setupTestRouter creates a test router with the given handlers.
func setupTestRouter(handlers *ProductHandlers) *gin.Engine {
	// We need to import gin here
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add request ID middleware
	router.Use(middleware.RequestIDMiddleware())

	// Add recovery middleware
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router.Use(middleware.RecoverMiddleware(logger))

	// Register product routes
	apiV1 := router.Group("/api/v1")
	apiV1.POST("/products", handlers.CreateProduct)
	apiV1.GET("/products", handlers.ListProducts)
	apiV1.GET("/products/:id", handlers.GetProduct)

	// Register internal routes
	internalV1 := router.Group("/internal/v1")
	internalV1.POST("/products/resolve", handlers.ResolveProducts)

	return router
}

func TestResolveProducts(t *testing.T) {
	t.Run("valid request with all found products returns 200", func(t *testing.T) {
		service := &mockProductService{
			resolveFunc: func(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error) {
				return product.ResolveResult{
					Products: map[int64]product.Product{
						1: {ID: 1, Code: "P001", Description: "Product 1", Balance: 100},
						2: {ID: 2, Code: "P002", Description: "Product 2", Balance: 200},
					},
					Missing: []int64{},
				}, nil
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"ids":[1,2]}`
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-resolve-1")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		if rec.Header().Get("X-Request-ID") != "test-resolve-1" {
			t.Errorf("X-Request-ID = %q, want %q", rec.Header().Get("X-Request-ID"), "test-resolve-1")
		}

		var response resolveResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response.Products) != 2 {
			t.Errorf("products length = %d, want %d", len(response.Products), 2)
		}

		prod1, exists := response.Products[1]
		if !exists {
			t.Fatal("expected product with ID 1 in response")
		}
		if prod1.Code != "P001" {
			t.Errorf("product 1 code = %q, want %q", prod1.Code, "P001")
		}
		if prod1.Balance != 100 {
			t.Errorf("product 1 balance = %d, want %d", prod1.Balance, 100)
		}

		if len(response.Missing) != 0 {
			t.Errorf("missing length = %d, want %d", len(response.Missing), 0)
		}
	})

	t.Run("request with missing products returns 404", func(t *testing.T) {
		service := &mockProductService{
			resolveFunc: func(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error) {
				return product.ResolveResult{
					Products: map[int64]product.Product{
						1: {ID: 1, Code: "P001", Description: "Product 1", Balance: 100},
					},
					Missing: []int64{999},
				}, nil
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"ids":[1,999]}`
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-resolve-2")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "PRODUCT_NOT_FOUND" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "PRODUCT_NOT_FOUND")
		}
		if errResp.Error.RequestID != "test-resolve-2" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-resolve-2")
		}
		if errResp.Error.Details != nil {
			t.Errorf("details = %#v, want nil", errResp.Error.Details)
		}
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {
		service := &mockProductService{}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{invalid json`
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-resolve-3")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "MALFORMED_REQUEST" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "MALFORMED_REQUEST")
		}
		if errResp.Error.RequestID != "test-resolve-3" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-resolve-3")
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"details":null`)) {
			t.Errorf("response does not contain explicit null details: %s", rec.Body.String())
		}
	})

	t.Run("duplicate IDs returns 422", func(t *testing.T) {
		service := &mockProductService{}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"ids":[1,1,2]}`
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-resolve-4")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
		}
	})

	t.Run("empty IDs list returns 200 with empty response", func(t *testing.T) {
		service := &mockProductService{
			resolveFunc: func(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error) {
				return product.ResolveResult{
					Products: make(map[int64]product.Product),
					Missing:  []int64{},
				}, nil
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"ids":[]}`
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-resolve-5")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var response resolveResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response.Products) != 0 {
			t.Errorf("products length = %d, want %d", len(response.Products), 0)
		}
		if len(response.Missing) != 0 {
			t.Errorf("missing length = %d, want %d", len(response.Missing), 0)
		}
	})

	t.Run("non JSON content type returns 400", func(t *testing.T) {
		service := &mockProductService{}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(`{"ids":[1]}`))
		req.Header.Set("Content-Type", "text/plain")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid ID in list returns 422", func(t *testing.T) {
		service := &mockProductService{
			resolveFunc: func(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error) {
				return product.ResolveResult{}, product.ErrInvalidID
			},
		}
		handlers := newTestProductHandlers(service)
		router := setupTestRouter(handlers)

		body := `{"ids":[0,-1]}`
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-resolve-6")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
		}
	})

	t.Run("service error returns 500", func(t *testing.T) {
		var logs bytes.Buffer
		service := &mockProductService{
			resolveFunc: func(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error) {
				return product.ResolveResult{}, errors.New("unexpected database error")
			},
		}
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		handlers := NewProductHandlers(service, logger)
		router := setupTestRouter(handlers)

		body := `{"ids":[1,2]}`
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/products/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "test-resolve-7")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}

		var errResp middleware.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error.Code != "INTERNAL_ERROR" {
			t.Errorf("error code = %q, want %q", errResp.Error.Code, "INTERNAL_ERROR")
		}
		if errResp.Error.RequestID != "test-resolve-7" {
			t.Errorf("request_id = %q, want %q", errResp.Error.RequestID, "test-resolve-7")
		}
		if strings.Contains(rec.Body.String(), "database") {
			t.Fatalf("response leaked internal error: %s", rec.Body.String())
		}
	})
}
