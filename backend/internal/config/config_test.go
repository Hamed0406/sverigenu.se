package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Unset all env vars that config.Load reads so we get predictable defaults.
	for _, k := range []string{
		"PORT", "SYNC_INTERVAL", "CONTACT_EMAIL",
		"SYNC_TOKEN", "LOG_DIR", "DEBUG", "TRAFIKVERKET_API_KEY",
	} {
		t.Setenv(k, "")
	}
	// DATABASE_URL is now required — must be set even for defaults test.
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")

	cfg := Load()

	if cfg.Port != "3000" {
		t.Errorf("default port: got %q, want %q", cfg.Port, "3000")
	}
	if cfg.SyncInterval != 15*time.Minute {
		t.Errorf("default sync interval: got %v, want 15m", cfg.SyncInterval)
	}
	if cfg.Debug {
		t.Error("debug should be false by default")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("SYNC_INTERVAL", "5m")
	t.Setenv("CONTACT_EMAIL", "test@example.com")
	t.Setenv("SYNC_TOKEN", "secret")
	t.Setenv("LOG_DIR", "/tmp/logs")
	t.Setenv("DEBUG", "true")
	t.Setenv("TRAFIKVERKET_API_KEY", "tkey")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")

	cfg := Load()

	checks := []struct{ got, want string }{
		{cfg.Port, "9090"},
		{cfg.ContactEmail, "test@example.com"},
		{cfg.SyncToken, "secret"},
		{cfg.LogDir, "/tmp/logs"},
		{cfg.TrafikverketAPIKey, "tkey"},
		{cfg.DatabaseURL, "postgres://u:p@localhost/db"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	if cfg.SyncInterval != 5*time.Minute {
		t.Errorf("sync interval: got %v, want 5m", cfg.SyncInterval)
	}
	if !cfg.Debug {
		t.Error("debug should be true when DEBUG=true")
	}
}

func TestLoadInvalidSyncInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("SYNC_INTERVAL", "notaduration")
	cfg := Load()
	if cfg.SyncInterval != 15*time.Minute {
		t.Errorf("invalid duration should fall back to 15m, got %v", cfg.SyncInterval)
	}
}

func TestLoadDotEnvIgnoredWhenAbsent(t *testing.T) {
	// Change to a temp dir where no .env file exists.
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Skip("cannot chdir:", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("PORT", "4567")
	cfg := Load()
	if cfg.Port != "4567" {
		t.Errorf("expected port 4567, got %q", cfg.Port)
	}
}
