// Package config provides configuration loading and validation for the billing service.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config holds the service configuration.
type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	StockServiceURL    string
	CORSAllowedOrigins []string
}

// Load loads configuration from environment variables.
// It validates required fields and parses URLs without logging sensitive data.
// CORS_ALLOWED_ORIGINS is optional (comma-separated list of explicit http/https origins).
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        os.Getenv("HTTP_ADDR"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		StockServiceURL: os.Getenv("STOCK_SERVICE_URL"),
	}

	// Set defaults
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8082"
	}

	// Validate required fields
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.StockServiceURL == "" {
		return Config{}, errors.New("STOCK_SERVICE_URL is required")
	}

	// Validate URLs are parseable (secret-safe: don't log the actual URLs)
	if err := validateURL(cfg.DatabaseURL, "postgres", "postgresql"); err != nil {
		return Config{}, errors.New("DATABASE_URL must be a valid PostgreSQL URL")
	}
	if err := validateURL(cfg.StockServiceURL, "http", "https"); err != nil {
		return Config{}, errors.New("STOCK_SERVICE_URL must be a valid HTTP URL")
	}

	var err error
	cfg.CORSAllowedOrigins, err = parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateURL(rawURL string, allowedSchemes ...string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return errors.New("invalid URL")
	}

	for _, scheme := range allowedSchemes {
		if u.Scheme == scheme {
			return nil
		}
	}

	return errors.New("unsupported URL scheme")
}

func parseAllowedOrigins(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if err := validateOrigin(origin); err != nil {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS contains invalid origin: %w", err)
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}

	return origins, nil
}

func validateOrigin(origin string) error {
	if origin == "" {
		return errors.New("origin must not be empty")
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return errors.New("origin must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("origin scheme must be http or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return errors.New("origin must include a host")
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return errors.New("origin must contain only scheme, host, and optional port")
	}

	return nil
}
