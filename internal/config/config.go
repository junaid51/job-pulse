// Package config reads the handful of settings JobPulse needs from the
// environment. Every setting has a working default so that `go run` against a
// freshly started `docker compose up` needs no setup at all.
package config

import (
	"log/slog"
	"os"
	"time"
)

type Config struct {
	// DatabaseURL is a libpq/pgx connection string.
	DatabaseURL string
	// Addr is the TCP address the HTTP server listens on.
	Addr string
	// PollInterval is how often every board is fetched.
	PollInterval time.Duration
	// CompaniesFile lists the boards to poll.
	CompaniesFile string
}

// Load reads the environment. It never fails: a missing variable means "use
// the local development default", which is what running from a clone should do.
//
// TODO(M4): GOOGLE_APPLICATION_CREDENTIALS.
func Load() Config {
	return Config{
		DatabaseURL:   env("DATABASE_URL", "postgres://jobpulse:jobpulse@localhost:5432/jobpulse?sslmode=disable"),
		Addr:          ":" + env("PORT", "8080"),
		PollInterval:  duration("POLL_INTERVAL", 15*time.Minute),
		CompaniesFile: env("COMPANIES_FILE", "companies.txt"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// duration accepts Go duration strings such as "15m" or "90s". An unparseable
// value is a typo worth mentioning rather than a reason to refuse to start.
func duration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("ignoring invalid duration", "key", key, "value", raw, "using", fallback.String())
		return fallback
	}
	return d
}
