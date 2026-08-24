// Package httpapi provides HTTP handlers for the billing service.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/invoice"
	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
	"github.com/gin-gonic/gin"
)

// readinessCheckTimeout is the timeout for database ping during readiness check.
const readinessCheckTimeout = 2 * time.Second

type readinessChecker interface {
	Ping(context.Context) error
}

type invoiceService interface {
	Create(context.Context, invoice.CreateInput) (invoice.Invoice, error)
	List(context.Context) ([]invoice.Invoice, error)
	GetByID(context.Context, int64) (invoice.Invoice, error)
	Print(context.Context, int64, invoice.StockConsumer, string) (invoice.Invoice, [16]byte, error)
}

type productResolver interface {
	ResolveProducts(context.Context, []int64, string) (map[int64]stock.ResolvedProduct, error)
}

// HealthHandlers provides health check endpoints.
type HealthHandlers struct {
	db     readinessChecker
	logger *slog.Logger
}

// NewRouter creates the Billing HTTP transport with health and invoice routes.
func NewRouter(
	db readinessChecker,
	logger *slog.Logger,
	corsAllowedOrigins []string,
	invoices invoiceService,
	products productResolver,
) *gin.Engine {
	router := gin.New()
	// Add request ID middleware first to ensure it runs before CORS
	router.Use(RequestIDMiddleware())
	// Add CORS middleware with configuration
	router.Use(CORSMiddleware(corsAllowedOrigins))
	// Add recovery middleware
	router.Use(StructuredRecovery(logger))

	health := NewHealthHandlers(db, logger)
	router.GET("/health/live", health.LiveHandler)
	router.GET("/health/ready", health.ReadyHandler)

	if invoices != nil && products != nil {
		invoiceHandlers := NewInvoiceHandlers(invoices, products, logger)
		apiV1 := router.Group("/api/v1")
		apiV1.POST("/invoices", invoiceHandlers.Create)
		apiV1.POST("/invoices/:id/print", invoiceHandlers.Print)
		apiV1.GET("/invoices", invoiceHandlers.List)
		apiV1.GET("/invoices/:id", invoiceHandlers.GetByID)
	}

	return router
}

// NewHealthHandlers creates a new HealthHandlers instance.
func NewHealthHandlers(db readinessChecker, logger *slog.Logger) *HealthHandlers {
	return &HealthHandlers{
		db:     db,
		logger: logger,
	}
}

// LiveHandler returns a simple liveness check handler.
func (h *HealthHandlers) LiveHandler(c *gin.Context) {
	requestID := GetRequestID(c.Request.Context())
	h.logger.Debug("health live check", "request_id", requestID)
	c.JSON(http.StatusOK, gin.H{"status": "live", "request_id": requestID})
}

// ReadyHandler returns a readiness check handler.
// It checks database connectivity with a timeout.
func (h *HealthHandlers) ReadyHandler(c *gin.Context) {
	requestID := GetRequestID(c.Request.Context())

	// Create a context with timeout for the readiness check
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessCheckTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.logger.Error("readiness check failed", "error", err, "request_id", requestID)
		ErrorHandler(c, err, "DATABASE_UNAVAILABLE", "Database is not ready", http.StatusServiceUnavailable)
		return
	}

	h.logger.Debug("health ready check", "request_id", requestID)
	c.JSON(http.StatusOK, gin.H{"status": "ready", "request_id": requestID})
}
