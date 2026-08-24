package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	// Should generate unique IDs
	if id1 == id2 {
		t.Error("expected unique request IDs")
	}

	// Should have billing- prefix
	if len(id1) < 8 || id1[:8] != "billing-" {
		t.Errorf("expected request ID to start with 'billing-', got: %s", id1)
	}
}

func TestGetRequestID(t *testing.T) {
	ctx := context.Background()
	requestID := GetRequestID(ctx)
	if requestID != "unknown" {
		t.Errorf("expected 'unknown' for empty context, got: %s", requestID)
	}

	// Test with request ID in context
	ctx = context.WithValue(ctx, requestIDKey{}, "test-request-id")
	requestID = GetRequestID(ctx)
	if requestID != "test-request-id" {
		t.Errorf("expected 'test-request-id', got: %s", requestID)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("generates request ID when none provided", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware())
		router.GET("/test", func(c *gin.Context) {
			requestID := c.GetString("request_id")
			if requestID == "" {
				t.Error("expected request_id to be set in gin context")
			}
			if requestID[:8] != "billing-" {
				t.Errorf("expected request_id to start with 'billing-', got: %s", requestID)
			}
			c.String(http.StatusOK, requestID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got: %d", w.Code)
		}

		// Check X-Request-ID header is set
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("expected X-Request-ID header to be set")
		}
	})

	t.Run("uses provided X-Request-ID header", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware())
		router.GET("/test", func(c *gin.Context) {
			requestID := c.GetString("request_id")
			if requestID != "custom-request-id" {
				t.Errorf("expected custom request_id, got: %s", requestID)
			}
			c.String(http.StatusOK, requestID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", "custom-request-id")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got: %d", w.Code)
		}

		// Check X-Request-ID header is preserved
		requestID := w.Header().Get("X-Request-ID")
		if requestID != "custom-request-id" {
			t.Errorf("expected X-Request-ID header to be 'custom-request-id', got: %s", requestID)
		}
	})

	t.Run("request ID in context", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware())
		router.GET("/test", func(c *gin.Context) {
			requestID := GetRequestID(c.Request.Context())
			if requestID == "" {
				t.Error("expected request_id in context")
			}
			c.String(http.StatusOK, requestID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got: %d", w.Code)
		}
	})
}

func TestStructuredRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("recovers from panic", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware(), StructuredRecovery(nil))
		router.GET("/panic", func(c *gin.Context) {
			panic("test panic")
		})

		req := httptest.NewRequest("GET", "/panic", nil)
		req.Header.Set("X-Request-ID", "panic-request-123")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should recover and return 500
		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got: %d", w.Code)
		}

		var body ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.Error.Code != "INTERNAL_SERVER_ERROR" {
			t.Errorf("code = %q, want %q", body.Error.Code, "INTERNAL_SERVER_ERROR")
		}
		if body.Error.Message != "Internal server error" {
			t.Errorf("message = %q, want %q", body.Error.Message, "Internal server error")
		}
		if body.Error.Details != nil {
			t.Errorf("details = %#v, want nil", body.Error.Details)
		}
		if !bytes.Contains(w.Body.Bytes(), []byte(`"details":null`)) {
			t.Errorf("response does not contain explicit null details: %s", w.Body.String())
		}
		if body.Error.RequestID != "panic-request-123" {
			t.Errorf("request_id = %q, want %q", body.Error.RequestID, "panic-request-123")
		}
		if w.Header().Get("X-Request-ID") != "panic-request-123" {
			t.Errorf("X-Request-ID = %q, want %q", w.Header().Get("X-Request-ID"), "panic-request-123")
		}
	})
}

func TestErrorHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns error response", func(t *testing.T) {
		router := gin.New()
		router.GET("/error", func(c *gin.Context) {
			c.Set("request_id", "test-req-id")
			ErrorHandler(c, context.DeadlineExceeded, "TIMEOUT", "Request timeout", http.StatusRequestTimeout)
		})

		req := httptest.NewRequest("GET", "/error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusRequestTimeout {
			t.Errorf("expected status 408, got: %d", w.Code)
		}

		// Check response body is JSON
		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json; charset=utf-8" {
			t.Errorf("expected JSON content type, got: %s", contentType)
		}

		var body ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.Error.Code != "TIMEOUT" || body.Error.Message != "Request timeout" {
			t.Errorf("unexpected error: %+v", body.Error)
		}
		if body.Error.Details != nil {
			t.Errorf("details = %#v, want nil", body.Error.Details)
		}
		if !bytes.Contains(w.Body.Bytes(), []byte(`"details":null`)) {
			t.Errorf("response does not contain explicit null details: %s", w.Body.String())
		}
		if body.Error.RequestID != "test-req-id" {
			t.Errorf("request_id = %q, want %q", body.Error.RequestID, "test-req-id")
		}
	})

	t.Run("includes request ID in response", func(t *testing.T) {
		router := gin.New()
		router.GET("/error", func(c *gin.Context) {
			c.Set("request_id", "test-req-id")
			ErrorHandler(c, nil, "TEST_ERROR", "Test error", http.StatusBadRequest)
		})

		req := httptest.NewRequest("GET", "/error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var body ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.Error.RequestID != "test-req-id" {
			t.Errorf("request_id = %q, want %q", body.Error.RequestID, "test-req-id")
		}
	})
}
