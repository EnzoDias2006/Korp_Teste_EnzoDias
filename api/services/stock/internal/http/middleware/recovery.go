package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// RecoverMiddleware recovers from panics and logs them with structured logging.
func RecoverMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				handlePanic(c, logger, err)
			}
		}()
		c.Next()
	}
}

// handlePanic handles a recovered panic.
func handlePanic(c *gin.Context, logger *slog.Logger, err any) {
	// Generate request ID for the error
	requestID := GetRequestID(c)
	if requestID == "" {
		requestID = "unknown"
	}

	// Log the panic with structured fields
	logger.Error(
		"panic recovered",
		"error", fmt.Sprintf("%v", err),
		"request_id", requestID,
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
		"stack", string(debug.Stack()),
	)

	// Write error response with nested error envelope
	c.JSON(http.StatusInternalServerError, NewErrorResponse(
		"INTERNAL_SERVER_ERROR",
		"Internal server error",
		nil,
		requestID,
	))
	c.Abort()
}
