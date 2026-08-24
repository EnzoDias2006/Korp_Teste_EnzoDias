package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EnzoDias2006/korp-api/services/billing/internal/config"
	"github.com/EnzoDias2006/korp-api/services/billing/internal/database"
	httpapi "github.com/EnzoDias2006/korp-api/services/billing/internal/http"
	"github.com/EnzoDias2006/korp-api/services/billing/internal/invoice"
	"github.com/EnzoDias2006/korp-api/services/billing/internal/stock"
)

// setupLogger creates a structured JSON logger.
func setupLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: false,
	}))
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Fail fast with secret-safe error message
		return fmt.Errorf("configuration error: %w", err)
	}

	logger.Info("starting billing service",
		"http_addr", cfg.HTTPAddr,
		"service", "billing",
	)

	// Open database connection
	db, err := database.Open(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	logger.Info("database connected")

	// Create Stock Service HTTP client
	stockClient := stock.NewClient(stock.ClientConfig{
		BaseURL: cfg.StockServiceURL,
		Timeout: stock.DefaultClientTimeout,
	})
	defer stockClient.CloseIdleConnections()
	logger.Info("stock service client configured")

	invoiceRepository := invoice.NewRepository(db.Pool())
	invoiceService := invoice.NewService(invoiceRepository)

	router := httpapi.NewRouter(db, logger, cfg.CORSAllowedOrigins, invoiceService, stockClient)

	// Create HTTP server with timeouts
	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	}

	logger.Info("shutting down server")

	// Shutdown server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

func main() {
	logger := setupLogger()
	if err := run(logger); err != nil {
		logger.Error("billing service stopped", "error", err)
		os.Exit(1)
	}
}
