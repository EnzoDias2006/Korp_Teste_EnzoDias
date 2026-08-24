package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRecoverMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Set up default logger for recovery middleware
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	t.Run("recovers from panic", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware())
		router.Use(RecoverMiddleware(logger))
		router.GET("/panic", func(c *gin.Context) {
			panic("test panic")
		})

		req := httptest.NewRequest("GET", "/panic", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}

		// Check that the error response is written with nested error envelope
		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json; charset=utf-8" {
			t.Errorf("expected Content-Type to be JSON, got %q", contentType)
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
		if body.Error.RequestID == "" || body.Error.RequestID == "unknown" {
			t.Errorf("request_id = %q, want generated request ID", body.Error.RequestID)
		}
	})

	t.Run("panic recovery includes nested error with request ID", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware())
		router.Use(RecoverMiddleware(logger))
		router.GET("/panic", func(c *gin.Context) {
			panic("test panic")
		})

		req := httptest.NewRequest("GET", "/panic", nil)
		req.Header.Set("X-Request-ID", "test-panic-req-123")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}

		// Check X-Request-ID is preserved in response
		if w.Header().Get("X-Request-ID") != "test-panic-req-123" {
			t.Errorf("expected X-Request-ID header %q, got %q", "test-panic-req-123", w.Header().Get("X-Request-ID"))
		}

		var body ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.Error.RequestID != "test-panic-req-123" {
			t.Errorf("request_id = %q, want %q", body.Error.RequestID, "test-panic-req-123")
		}
		if body.Error.Details != nil {
			t.Errorf("details = %#v, want nil", body.Error.Details)
		}
		if !bytes.Contains(w.Body.Bytes(), []byte(`"details":null`)) {
			t.Errorf("response does not contain explicit null details: %s", w.Body.String())
		}
	})

	t.Run("does not recover from normal flow", func(t *testing.T) {
		router := gin.New()
		router.Use(RecoverMiddleware(logger))
		router.GET("/ok", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest("GET", "/ok", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		if w.Body.String() != "ok" {
			t.Errorf("expected body %q, got %q", "ok", w.Body.String())
		}
	})
}
