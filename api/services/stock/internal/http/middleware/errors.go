package middleware

import "github.com/gin-gonic/gin"

// ErrorBody contains the stable machine-readable error fields.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details"`
	RequestID string `json:"request_id"`
}

// ErrorResponse represents the stable nested JSON error envelope.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// NewErrorResponse creates a new error response with the given parameters.
func NewErrorResponse(code, message string, details any, requestID string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: requestID,
		},
	}
}

// WriteError writes an error response with the appropriate status code.
// It maps error codes to HTTP status codes.
func WriteError(c *gin.Context, statusCode int, errResp ErrorResponse) {
	c.JSON(statusCode, errResp)
}
