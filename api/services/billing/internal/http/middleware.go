// Package httpapi provides HTTP middleware and handlers for the billing service.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// requestIDKey is the context key for request ID.
type requestIDKey struct{}

// RequestIDMiddleware adds a request ID to each request, injects it into the context,
// and sets the X-Request-ID response header.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		c.Set("request_id", requestID)
		ctx := context.WithValue(c.Request.Context(), requestIDKey{}, requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "billing-" + hex.EncodeToString(random[:])
	}

	return fmt.Sprintf("billing-%d-%d", time.Now().UnixNano(), requestIDSequence.Add(1))
}

var requestIDSequence atomic.Uint64

// GetRequestID extracts the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if v := ctx.Value(requestIDKey{}); v != nil {
		if rid, ok := v.(string); ok {
			return rid
		}
	}
	return "unknown"
}

// ErrorResponse represents the stable nested JSON error envelope.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the stable machine-readable error fields.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details"`
	RequestID string `json:"request_id"`
}

func newErrorResponse(code, message string, details any, requestID string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: requestID,
		},
	}
}

// StructuredRecovery is a Gin recovery middleware that logs panics with structure.
// It recovers from panics and returns a 500 error response with nested error envelope.
func StructuredRecovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				requestID := GetRequestID(c.Request.Context())
				if logger != nil {
					logger.Error("panic recovered",
						"error", err,
						"request_id", requestID,
						"path", c.Request.URL.Path,
						"method", c.Request.Method,
					)
				}
				c.JSON(http.StatusInternalServerError, newErrorResponse(
					"INTERNAL_SERVER_ERROR",
					"Internal server error",
					nil,
					requestID,
				))
				c.Abort()
			}
		}()
		c.Next()
	}
}

// ErrorHandler is a centralized error to HTTP response mapper.
// It returns a consistent nested error envelope.
func ErrorHandler(c *gin.Context, _ error, code string, message string, httpStatus int) {
	requestID := c.GetString("request_id")
	if requestID == "" {
		requestID = GetRequestID(c.Request.Context())
	}

	c.JSON(httpStatus, newErrorResponse(code, message, nil, requestID))
}
