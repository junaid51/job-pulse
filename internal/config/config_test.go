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

// The poller cannot be switched off any more: "nothing polls unless something
// external says so" is the arrangement three outages came from.
func TestPollIntervalRefusesToBeDisabled(t *testing.T) {
	for _, value := range []string{"0", "0s", "-5m", "nonsense", ""} {
		t.Setenv("POLL_INTERVAL", value)
		if got := Load().PollInterval; got < time.Minute {
			t.Errorf("POLL_INTERVAL=%q gave %v, want at least a minute", value, got)
		}
	}
	t.Setenv("POLL_INTERVAL", "30s")
	if got := Load().PollInterval; got != time.Minute {
		t.Errorf("a sub-minute interval should be raised to a minute, got %v", got)
	}
	t.Setenv("POLL_INTERVAL", "15m")
	if got := Load().PollInterval; got != 15*time.Minute {
		t.Errorf("an explicit interval should be honoured, got %v", got)
	}
}
