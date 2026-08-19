package config

import "testing"

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
