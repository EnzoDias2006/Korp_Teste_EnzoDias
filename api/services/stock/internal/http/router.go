// Package httpapi provides HTTP router setup and health endpoints for the stock service.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/EnzoDias2006/korp-api/services/stock/internal/http/middleware"
	"github.com/EnzoDias2006/korp-api/services/stock/internal/product"
	"github.com/gin-gonic/gin"
)

type readinessChecker interface {
	Ping(context.Context) error
}

// productService defines the interface needed from product.Service for HTTP handlers.
type productService interface {
	Create(ctx context.Context, input product.CreateInput) (product.Product, error)
	List(ctx context.Context) ([]product.Product, error)
	GetByID(ctx context.Context, id int64) (product.Product, error)
	Resolve(ctx context.Context, input product.ResolveInput) (product.ResolveResult, error)
	Consume(ctx context.Context, input product.ConsumeInput) (product.ConsumeResult, bool, error)
}

// NewRouter creates and configures the Gin router with health endpoints, middleware, and product routes.
func NewRouter(database readinessChecker, logger *slog.Logger, corsAllowedOrigins []string, productService productService) *gin.Engine {
	router := gin.New()

	// Add request ID middleware first to ensure it runs before CORS
	router.Use(middleware.RequestIDMiddleware())

	// Add CORS middleware with configuration
	router.Use(middleware.CORSMiddleware(corsAllowedOrigins))

	// Add recovery middleware with structured logging
	router.Use(middleware.RecoverMiddleware(logger))

	// Health endpoints
	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "live"})
	})

	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := database.Ping(ctx); err != nil {
			middleware.WriteError(c, http.StatusServiceUnavailable, middleware.NewErrorResponse(
				"DATABASE_UNAVAILABLE",
				"Database is not ready",
				nil,
				middleware.GetRequestID(c),
			))
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// API route groups
	apiV1 := router.Group("/api/v1")
	if productService != nil {
		productHandlers := NewProductHandlers(productService, logger)
		apiV1.POST("/products", productHandlers.CreateProduct)
		apiV1.GET("/products", productHandlers.ListProducts)
		apiV1.GET("/products/:id", productHandlers.GetProduct)

		// Internal API for inter-service communication
		internalV1 := router.Group("/internal/v1")
		internalV1.POST("/products/resolve", productHandlers.ResolveProducts)
		internalV1.POST("/stock/consume", productHandlers.ConsumeStock)
	}

	return router
}
