// Package config reads the handful of settings JobPulse needs from the
// environment. Every setting has a working default so that `go run` against a
// freshly started `docker compose up` needs no setup at all.
package config

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	// DatabaseURL is a libpq/pgx connection string.
	DatabaseURL string
	// Addr is the TCP address the HTTP server listens on.
	Addr string
	// PollInterval is how often every board is fetched. There is no way to set
	// it to zero: "the poller is off and something external drives it" was the
	// arrangement behind three outages, the last of which read no boards for
	// nine hours because nothing external could wake the host. A value at or
	// below zero is ignored in favour of the default.
	PollInterval time.Duration
	// CompaniesFile lists the boards to poll.
	CompaniesFile string
	// FirebaseCredentials is a service account JSON file. Empty means push
	// notifications are logged rather than sent.
	FirebaseCredentials string
}

// Load reads the environment. It never fails: a missing variable means "use
// the local development default", which is what running from a clone should do.
func Load() Config {
	return Config{
		DatabaseURL:   env("DATABASE_URL", "postgres://jobpulse:jobpulse@localhost:5432/jobpulse?sslmode=disable"),
		Addr:          ":" + env("PORT", "8080"),
		PollInterval:  pollInterval(),
		CompaniesFile: env("COMPANIES_FILE", "companies.txt"),
		// The same variable the Google libraries read, so there is one name for it.
		FirebaseCredentials: strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
	}
}

// env reads a variable, trimming surrounding whitespace. The trim is not
// fussiness: pasting a connection string into a hosting dashboard picks up a
// trailing newline easily, and Go's URL parser rejects the control character —
// which takes the whole deployment down with an error that names everything
// except the invisible character at fault.
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// duration accepts Go duration strings such as "15m" or "90s", and "0" to mean
// disabled. An unparseable value is a typo worth mentioning rather than a
// reason to refuse to start.
func duration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if raw == "0" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		slog.Warn("ignoring invalid duration", "key", key, "value", raw, "using", fallback.String())
		return fallback
	}
	return d
}

// pollInterval refuses to be turned off, and refuses to be so short that
// cycles overlap — one pass over two hundred boards takes about thirty seconds.
func pollInterval() time.Duration {
	const fallback = 5 * time.Minute
	interval := duration("POLL_INTERVAL", fallback)
	switch {
	case interval <= 0:
		slog.Warn("POLL_INTERVAL is not a usable interval; polling anyway",
			"ignored", os.Getenv("POLL_INTERVAL"), "using", fallback.String())
		return fallback
	case interval < time.Minute:
		slog.Warn("POLL_INTERVAL is shorter than a cycle takes; raising it",
			"asked", interval.String(), "using", time.Minute.String())
		return time.Minute
	}
	return interval
}
