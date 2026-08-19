// Package config reads the handful of settings JobPulse needs from the
// environment. Every setting has a working default so that `go run` against a
// freshly started `docker compose up` needs no setup at all.
package config

import "os"

type Config struct {
	// DatabaseURL is a libpq/pgx connection string.
	DatabaseURL string
	// Addr is the TCP address the HTTP server listens on.
	Addr string
}

// Load reads the environment. It never fails: a missing variable means "use
// the local development default", which is what running from a clone should do.
//
// TODO(M2): POLL_INTERVAL and COMPANIES_FILE.
// TODO(M4): GOOGLE_APPLICATION_CREDENTIALS.
func Load() Config {
	return Config{
		DatabaseURL: env("DATABASE_URL", "postgres://jobpulse:jobpulse@localhost:5432/jobpulse?sslmode=disable"),
		Addr:        ":" + env("PORT", "8080"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
