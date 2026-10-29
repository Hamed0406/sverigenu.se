// Package config loads application configuration from environment variables.
// Values are read from a .env file first (if one exists), then from the
// process environment. Process env vars always win over .env values, so
// Docker / Kubernetes injected vars are never overridden.
package config

import (
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL        string
	Port               string
	SyncInterval       time.Duration
	ContactEmail       string // included in the polisen.se User-Agent header as required by their terms
	SyncToken          string // bearer token for POST /sync; empty = unprotected (dev only)
	BaseURL            string // canonical base URL e.g. "https://sverigedata.nu"; used in sitemap/robots
	LogDir             string // directory for access.log; empty = file logging disabled
	Debug              bool   // enable debug-level logging (DEBUG=true in .env)
	TrafikverketAPIKey string // Trafikverket open data API key
}

// Load reads config in this order (last writer wins for conflicts):
//  1. .env file in the working directory (silently ignored if absent)
//  2. Actual process environment variables
//
// This means you can override any .env value by setting the env var directly,
// which is useful in Docker / CI without modifying the file.
func Load() Config {
	// godotenv.Load is a no-op when .env doesn't exist (production deployments
	// where env vars are injected by the container runtime or CI).
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Warn("could not load .env file", "err", err)
	}

	syncInterval := 15 * time.Minute
	if v := os.Getenv("SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			syncInterval = d
		} else {
			slog.Warn("invalid SYNC_INTERVAL, using default 15m", "value", v)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL environment variable is required but not set")
		os.Exit(1)
	}

	email := os.Getenv("CONTACT_EMAIL")
	if email == "" {
		slog.Warn("CONTACT_EMAIL not set — polisen.se User-Agent will lack a contact address")
	}

	syncToken := os.Getenv("SYNC_TOKEN")
	if syncToken == "" {
		slog.Warn("SYNC_TOKEN is not set — POST /api/v1/sync and GET /api/v1/admin/stats are UNAUTHENTICATED")
	} else if len(syncToken) < 16 {
		slog.Warn("SYNC_TOKEN is very short — use at least 16 random characters")
	}

	return Config{
		DatabaseURL:        dbURL,
		Port:               port,
		SyncInterval:       syncInterval,
		ContactEmail:       email,
		SyncToken:          syncToken,
		BaseURL:            os.Getenv("BASE_URL"),
		LogDir:             os.Getenv("LOG_DIR"),
		Debug:              os.Getenv("DEBUG") == "true",
		TrafikverketAPIKey: os.Getenv("TRAFIKVERKET_API_KEY"),
	}
}
