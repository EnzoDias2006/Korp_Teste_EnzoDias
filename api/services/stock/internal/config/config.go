// Package config provides service configuration loading from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config holds service configuration from environment variables.
type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	CORSAllowedOrigins []string
}

// Load loads configuration from environment variables.
// HTTP_ADDR defaults to ":8081" if not set.
// DATABASE_URL is required and will cause an error if not set.
// CORS_ALLOWED_ORIGINS is optional (comma-separated list of explicit http/https origins).
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr: os.Getenv("HTTP_ADDR"),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8081"
	}

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return Config{}, errors.New("DATABASE_URL must be a valid PostgreSQL URL")
	}

	cfg.CORSAllowedOrigins, err = parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
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
