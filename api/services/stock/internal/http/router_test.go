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
	"testing"

	"github.com/EnzoDias2006/korp-api/services/stock/internal/http/middleware"
)

type pingStub struct {
	err error
}

func (stub pingStub) Ping(context.Context) error {
	return stub.err
}

func TestHealthRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("liveness ignores database availability", func(t *testing.T) {
		response := httptest.NewRecorder()
		NewRouter(pingStub{err: errors.New("database offline")}, logger, nil, nil).ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, "/health/live", nil),
		)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
	})

	t.Run("readiness succeeds when database responds", func(t *testing.T) {
		response := httptest.NewRecorder()
		NewRouter(pingStub{}, logger, nil, nil).ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, "/health/ready", nil),
		)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
	})

	t.Run("readiness failure uses stable envelope", func(t *testing.T) {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		request.Header.Set("X-Request-ID", "request-123")
		NewRouter(pingStub{err: errors.New("database offline")}, logger, nil, nil).ServeHTTP(response, request)

		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
		}

		var body middleware.ErrorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.Error.Code != "DATABASE_UNAVAILABLE" || body.Error.RequestID != "request-123" {
			t.Fatalf("unexpected response: %+v", body)
		}
		if body.Error.Message != "Database is not ready" {
			t.Errorf("message = %q, want %q", body.Error.Message, "Database is not ready")
		}
		if body.Error.Details != nil {
			t.Fatalf("details = %#v, want nil", body.Error.Details)
		}
		if !bytes.Contains(response.Body.Bytes(), []byte(`"details":null`)) {
			t.Fatalf("response does not contain explicit null details: %s", response.Body.String())
		}
	})

	t.Run("router applies request ID before CORS preflight", func(t *testing.T) {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodOptions, "/health/live", nil)
		request.Header.Set("Origin", "http://localhost:4200")
		request.Header.Set("X-Request-ID", "preflight-request-123")
		NewRouter(pingStub{}, logger, []string{"http://localhost:4200"}, nil).ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:4200" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:4200")
		}
		if got := response.Header().Get("X-Request-ID"); got != "preflight-request-123" {
			t.Errorf("X-Request-ID = %q, want %q", got, "preflight-request-123")
		}
	})
}
