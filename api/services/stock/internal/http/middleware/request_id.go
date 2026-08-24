package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// requestIDContextKey is the context key for the request ID.
type requestIDContextKey struct{}

// RequestIDMiddleware generates or accepts X-Request-ID header and adds it to context.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for existing request ID in header
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			// Generate new request ID
			requestID = generateRequestID()
		}

		// Set request ID in response header
		c.Header("X-Request-ID", requestID)

		// Add request ID to context
		ctx := context.WithValue(c.Request.Context(), requestIDContextKey{}, requestID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(c *gin.Context) string {
	if rid, ok := c.Request.Context().Value(requestIDContextKey{}).(string); ok {
		return rid
	}
	return ""
}

// generateRequestID generates a random 16-byte request ID.
func generateRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "stock-" + hex.EncodeToString(random[:])
	}

	return fmt.Sprintf("stock-%d-%d", time.Now().UnixNano(), requestIDSequence.Add(1))
}

var requestIDSequence atomic.Uint64
