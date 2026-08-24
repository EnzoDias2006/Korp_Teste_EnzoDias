package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allowedOrigins := []string{"http://localhost:4200", "https://example.com"}

	t.Run("allowed preflight returns 204 without reaching handler", func(t *testing.T) {
		handlerCalled := false
		router := gin.New()
		router.Use(RequestIDMiddleware(), CORSMiddleware(allowedOrigins))
		router.OPTIONS("/test", func(c *gin.Context) {
			handlerCalled = true
			c.Status(http.StatusTeapot)
		})

		request := httptest.NewRequest(http.MethodOptions, "/test", nil)
		request.Header.Set("Origin", "http://localhost:4200")
		request.Header.Set("Access-Control-Request-Method", http.MethodPost)
		request.Header.Set("X-Request-ID", "preflight-123")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if handlerCalled {
			t.Fatal("application handler was called")
		}
		assertHeader(t, response, "Access-Control-Allow-Origin", "http://localhost:4200")
		assertHeader(t, response, "Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		assertHeader(t, response, "Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
		assertHeader(t, response, "Access-Control-Expose-Headers", "X-Request-ID")
		assertHeader(t, response, "Vary", "Origin")
		assertHeader(t, response, "X-Request-ID", "preflight-123")
	})

	t.Run("nonmatching preflight has no allow headers and does not reach handler", func(t *testing.T) {
		handlerCalled := false
		router := gin.New()
		router.Use(RequestIDMiddleware(), CORSMiddleware(allowedOrigins))
		router.OPTIONS("/test", func(c *gin.Context) {
			handlerCalled = true
			c.Status(http.StatusTeapot)
		})

		request := httptest.NewRequest(http.MethodOptions, "/test", nil)
		request.Header.Set("Origin", "https://untrusted.example")
		request.Header.Set("X-Request-ID", "rejected-123")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if handlerCalled {
			t.Fatal("application handler was called")
		}
		assertHeader(t, response, "Access-Control-Allow-Origin", "")
		assertHeader(t, response, "Access-Control-Allow-Methods", "")
		assertHeader(t, response, "Access-Control-Allow-Headers", "")
		assertHeader(t, response, "Access-Control-Expose-Headers", "")
		assertHeader(t, response, "Vary", "Origin")
		assertHeader(t, response, "X-Request-ID", "rejected-123")
	})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run("allowed "+method+" reaches handler", func(t *testing.T) {
			router := gin.New()
			router.Use(RequestIDMiddleware(), CORSMiddleware(allowedOrigins))
			router.Handle(method, "/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			request := httptest.NewRequest(method, "/test", nil)
			request.Header.Set("Origin", "https://example.com")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			assertHeader(t, response, "Access-Control-Allow-Origin", "https://example.com")
			assertHeader(t, response, "Access-Control-Expose-Headers", "X-Request-ID")
			assertHeader(t, response, "Vary", "Origin")
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("X-Request-ID is empty")
			}
		})
	}

	t.Run("nonmatching simple request reaches handler without allow headers", func(t *testing.T) {
		router := gin.New()
		router.Use(RequestIDMiddleware(), CORSMiddleware(allowedOrigins))
		router.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		request := httptest.NewRequest(http.MethodGet, "/test", nil)
		request.Header.Set("Origin", "https://untrusted.example")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		assertHeader(t, response, "Access-Control-Allow-Origin", "")
		assertHeader(t, response, "Vary", "Origin")
	})
}

func assertHeader(t *testing.T, response *httptest.ResponseRecorder, name, want string) {
	t.Helper()
	if got := response.Header().Get(name); got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}
