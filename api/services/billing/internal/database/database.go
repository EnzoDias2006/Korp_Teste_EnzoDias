// Package database provides the database connection boundary for the billing service.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with service-specific boundary methods.
// It provides the database connection boundary for the billing service.
type DB struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// Open creates a new database connection and returns a DB wrapper.
// It opens a connection pool and verifies connectivity with a ping.
func Open(ctx context.Context, databaseURL string, logger *slog.Logger) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}

	// Configure connection pool settings for a billing service
	config.MaxConns = 5
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Ping to verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("database pool created")

	return &DB{
		pool:   pool,
		logger: logger,
	}, nil
}

// Pool returns the underlying pgxpool.Pool for direct access when needed.
// Prefer using boundary methods when possible.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Ping verifies the database connection is alive.
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Close closes the connection pool.
// This should be called during graceful shutdown.
func (db *DB) Close() {
	db.pool.Close()
	db.logger.Info("database pool closed")
}
