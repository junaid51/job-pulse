package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PORT", "")

	cfg := Load()

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want \":8080\"", cfg.Addr)
	}
	if cfg.DatabaseURL == "" {
		t.Error("DatabaseURL is empty; a clone with no env set must still connect locally")
	}
}

func TestLoadPrefersEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/jobpulse")
	t.Setenv("PORT", "9000")

	cfg := Load()

	if got, want := cfg.DatabaseURL, "postgres://user:pass@db:5432/jobpulse"; got != want {
		t.Errorf("DatabaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Addr, ":9000"; got != want {
		t.Errorf("Addr = %q, want %q", got, want)
	}
}

// A connection string pasted into a hosting dashboard often arrives with a
// trailing newline. Go's URL parser calls that an invalid control character
// and the process exits before serving a single request, so the trim here is
// load-bearing.
func TestLoadTrimsWhitespace(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pw@host:5432/db?sslmode=require\n")
	t.Setenv("PORT", " 9090 ")
	t.Setenv("POLL_INTERVAL", "  10m\n")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", " /etc/creds.json\n")

	cfg := Load()
	if strings.ContainsAny(cfg.DatabaseURL, "\n\r\t ") {
		t.Errorf("DatabaseURL still carries whitespace: %q", cfg.DatabaseURL)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want :9090", cfg.Addr)
	}
	if cfg.PollInterval != 10*time.Minute {
		t.Errorf("PollInterval = %v, want 10m", cfg.PollInterval)
	}
	if cfg.FirebaseCredentials != "/etc/creds.json" {
		t.Errorf("FirebaseCredentials = %q", cfg.FirebaseCredentials)
	}
}
