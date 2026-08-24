package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		httpAddr    string
		corsOrigins string
		wantAddr    string
		wantOrigins []string
		wantErr     string
	}{
		{
			name:        "uses default HTTP address",
			databaseURL: "postgres://localhost:5432/stock_db",
			wantAddr:    ":8081",
		},
		{
			name:        "loads and normalizes explicit origins",
			databaseURL: "postgres://localhost:5432/stock_db",
			httpAddr:    ":9090",
			corsOrigins: " http://localhost:4200,https://example.com,http://localhost:4200 ",
			wantAddr:    ":9090",
			wantOrigins: []string{"http://localhost:4200", "https://example.com"},
		},
		{
			name:    "requires database URL",
			wantErr: "DATABASE_URL is required",
		},
		{
			name:        "rejects invalid database URL",
			databaseURL: "not a URL",
			wantErr:     "DATABASE_URL must be a valid PostgreSQL URL",
		},
		{
			name:        "rejects wildcard origin",
			databaseURL: "postgres://localhost:5432/stock_db",
			corsOrigins: "*",
			wantErr:     "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
		{
			name:        "rejects non HTTP origin",
			databaseURL: "postgres://localhost:5432/stock_db",
			corsOrigins: "ftp://example.com",
			wantErr:     "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
		{
			name:        "rejects origin with path",
			databaseURL: "postgres://localhost:5432/stock_db",
			corsOrigins: "https://example.com/app",
			wantErr:     "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
		{
			name:        "rejects empty list entry",
			databaseURL: "postgres://localhost:5432/stock_db",
			corsOrigins: "https://example.com,",
			wantErr:     "CORS_ALLOWED_ORIGINS contains invalid origin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", tt.httpAddr)
			t.Setenv("DATABASE_URL", tt.databaseURL)
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
			if !reflect.DeepEqual(cfg.CORSAllowedOrigins, tt.wantOrigins) {
				t.Errorf("CORSAllowedOrigins = %#v, want %#v", cfg.CORSAllowedOrigins, tt.wantOrigins)
			}
		})
	}
}
