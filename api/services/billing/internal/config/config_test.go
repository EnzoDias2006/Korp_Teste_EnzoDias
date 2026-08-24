package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name            string
		httpAddr        string
		databaseURL     string
		stockServiceURL string
		corsOrigins     string
		wantAddr        string
		wantOrigins     []string
		wantErr         string
	}{
		{
			name:            "uses default HTTP address",
			databaseURL:     "postgres://localhost:5432/billing_db",
			stockServiceURL: "http://localhost:8081",
			wantAddr:        ":8082",
		},
		{
			name:            "loads and normalizes explicit origins",
			httpAddr:        ":9090",
			databaseURL:     "postgres://localhost:5432/billing_db",
			stockServiceURL: "http://localhost:8081",
			corsOrigins:     " http://localhost:4200,https://example.com,http://localhost:4200 ",
			wantAddr:        ":9090",
			wantOrigins:     []string{"http://localhost:4200", "https://example.com"},
		},
		{
			name:            "requires database URL",
			stockServiceURL: "http://localhost:8081",
			wantErr:         "DATABASE_URL is required",
		},
		{
			name:        "requires stock service URL",
			databaseURL: "postgres://localhost:5432/billing_db",
			wantErr:     "STOCK_SERVICE_URL is required",
		},
		{
			name:            "rejects invalid database URL",
			databaseURL:     "not a URL",
			stockServiceURL: "http://localhost:8081",
			wantErr:         "DATABASE_URL must be a valid PostgreSQL URL",
		},
		{
			name:            "rejects invalid stock service URL",
			databaseURL:     "postgres://localhost:5432/billing_db",
			stockServiceURL: "ftp://localhost:8081",
			wantErr:         "STOCK_SERVICE_URL must be a valid HTTP URL",
		},
		{
			name:            "rejects wildcard origin",
			databaseURL:     "postgres://localhost:5432/billing_db",
			stockServiceURL: "http://localhost:8081",
			corsOrigins:     "*",
			wantErr:         "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
		{
			name:            "rejects non HTTP origin",
			databaseURL:     "postgres://localhost:5432/billing_db",
			stockServiceURL: "http://localhost:8081",
			corsOrigins:     "ftp://example.com",
			wantErr:         "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
		{
			name:            "rejects origin with query",
			databaseURL:     "postgres://localhost:5432/billing_db",
			stockServiceURL: "http://localhost:8081",
			corsOrigins:     "https://example.com?tenant=1",
			wantErr:         "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
		{
			name:            "rejects empty list entry",
			databaseURL:     "postgres://localhost:5432/billing_db",
			stockServiceURL: "http://localhost:8081",
			corsOrigins:     "https://example.com,",
			wantErr:         "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", tt.httpAddr)
			t.Setenv("DATABASE_URL", tt.databaseURL)
			t.Setenv("STOCK_SERVICE_URL", tt.stockServiceURL)
			t.Setenv("CORS_ALLOWED_ORIGINS", tt.corsOrigins)

			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.HTTPAddr != tt.wantAddr {
				t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, tt.wantAddr)
			}
			if cfg.DatabaseURL != tt.databaseURL {
				t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, tt.databaseURL)
			}
			if cfg.StockServiceURL != tt.stockServiceURL {
				t.Errorf("StockServiceURL = %q, want %q", cfg.StockServiceURL, tt.stockServiceURL)
			}
			if !reflect.DeepEqual(cfg.CORSAllowedOrigins, tt.wantOrigins) {
				t.Errorf("CORSAllowedOrigins = %#v, want %#v", cfg.CORSAllowedOrigins, tt.wantOrigins)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		schemes []string
		wantErr bool
	}{
		{name: "HTTP URL", rawURL: "http://localhost:8080", schemes: []string{"http", "https"}},
		{name: "HTTPS URL", rawURL: "https://localhost:443", schemes: []string{"http", "https"}},
		{name: "PostgreSQL URL", rawURL: "postgres://user:pass@localhost:5432/db", schemes: []string{"postgres", "postgresql"}},
		{name: "empty URL", schemes: []string{"http", "https"}, wantErr: true},
		{name: "invalid URL", rawURL: "not a url", schemes: []string{"http", "https"}, wantErr: true},
		{name: "unsupported scheme", rawURL: "ftp://localhost:21", schemes: []string{"http", "https"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.rawURL, tt.schemes...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateURL() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
