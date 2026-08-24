package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("generates request ID when not provided", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware())
		router.GET("/test", func(c *gin.Context) {
			requestID := GetRequestID(c)
			if requestID == "" {
				t.Error("expected request ID to be set")
			}
			c.String(http.StatusOK, requestID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check response header
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("expected X-Request-ID header to be set")
		}

		if len(requestID) != 38 || requestID[:6] != "stock-" {
			t.Errorf("unexpected generated request ID %q", requestID)
		}
	})

	t.Run("uses provided request ID", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware())
		router.GET("/test", func(c *gin.Context) {
			requestID := GetRequestID(c)
			if requestID != "test-request-id" {
				t.Errorf("expected request ID %q, got %q", "test-request-id", requestID)
			}
			c.String(http.StatusOK, requestID)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", "test-request-id")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check response header preserves the provided ID
		requestID := w.Header().Get("X-Request-ID")
		if requestID != "test-request-id" {
			t.Errorf("expected X-Request-ID header %q, got %q", "test-request-id", requestID)
		}
	})

	t.Run("GetRequestID returns empty string when not set", func(t *testing.T) {
		router := gin.New()
		router.GET("/test", func(c *gin.Context) {
			requestID := GetRequestID(c)
			if requestID != "" {
				t.Errorf("expected empty request ID, got %q", requestID)
			}
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}
