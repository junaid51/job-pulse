package poll

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/junaid51/job-pulse/internal/providers"
)

func TestParseCompanies(t *testing.T) {
	file := `
# a comment
greenhouse stripe Stripe
lever      spotify

  ashby openai OpenAI Inc
`
	companies, err := ParseCompanies(strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}

	want := []Company{
		{Provider: "greenhouse", Slug: "stripe", Name: "Stripe"},
		{Provider: "lever", Slug: "spotify", Name: ""},
		{Provider: "ashby", Slug: "openai", Name: "OpenAI Inc"}, // name may contain spaces
	}
	if len(companies) != len(want) {
		t.Fatalf("parsed %d companies, want %d: %+v", len(companies), len(want), companies)
	}
	for i := range want {
		if companies[i] != want[i] {
			t.Errorf("company %d = %+v, want %+v", i, companies[i], want[i])
		}
	}
}

func TestParseCompaniesRejectsBadLines(t *testing.T) {
	tests := map[string]string{
		"unknown provider": "monster stripe",
		"missing slug":     "greenhouse",
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCompanies(strings.NewReader(line)); err == nil {
				t.Errorf("ParseCompanies(%q) succeeded, want an error", line)
			}
		})
	}
}

// Metered providers sit out cycles until their interval passes; everything
// else is always due, including boards never polled before.
func TestDueNow(t *testing.T) {
	now := time.Now()
	recent, stale := now.Add(-time.Hour), now.Add(-7*time.Hour)
	companies := []Company{
		{Provider: "greenhouse", Slug: "always", LastPolledAt: &recent},
		{Provider: "careerjet", Slug: "fresh", LastPolledAt: &recent},
		{Provider: "careerjet", Slug: "stale", LastPolledAt: &stale},
		{Provider: "careerjet", Slug: "never"},
	}

	var slugs []string
	for _, c := range dueNow(companies, now) {
		slugs = append(slugs, c.Slug)
	}
	// A metered provider surfaces at most one due board per cycle — bursts
	// trip its rate limit — so "never" waits for the next cycle.
	want := []string{"always", "stale"}
	if fmt.Sprint(slugs) != fmt.Sprint(want) {
		t.Errorf("dueNow kept %v, want %v", slugs, want)
	}
}

func TestDisplayNameFallsBackToSlug(t *testing.T) {
	if got := (Company{Slug: "spotify"}).displayName(); got != "spotify" {
		t.Errorf("displayName() = %q, want %q", got, "spotify")
	}
	if got := (Company{Slug: "spotify", Name: "Spotify"}).displayName(); got != "Spotify" {
		t.Errorf("displayName() = %q, want %q", got, "Spotify")
	}
}

// The ingest filter and the age sweep must agree, or an old posting still on
// its board would be deleted and re-announced every cycle.
func TestYoungEnough(t *testing.T) {
	now := time.Now()
	jobs := []providers.Job{
		{Title: "fresh", PostedAt: now.Add(-24 * time.Hour)},
		{Title: "borderline", PostedAt: now.Add(-maxJobAge + time.Hour)},
		{Title: "ancient", PostedAt: now.Add(-maxJobAge - time.Hour)},
		{Title: "undated"}, // no posting date: ages from first sight, so kept
	}
	var kept []string
	for _, j := range youngEnough(jobs, now) {
		kept = append(kept, j.Title)
	}
	want := []string{"fresh", "borderline", "undated"}
	if fmt.Sprint(kept) != fmt.Sprint(want) {
		t.Errorf("youngEnough kept %v, want %v", kept, want)
	}
}

// Excluded locations are dropped at ingest and swept from storage; if only one
// side knew, a still-listed posting would be re-ingested every cycle.
func TestExcludedLocations(t *testing.T) {
	now := time.Now()
	jobs := []providers.Job{
		{Title: "Backend Engineer", Location: "Tel Aviv, Israel"},
		{Title: "Backend Engineer", Location: "Herzliya"},
		{Title: "Backend Engineer", Location: "Remote - Israel"},
		{Title: "Backend Engineer", Location: "Dubai, United Arab Emirates"},
		{Title: "Backend Engineer", Location: "Remote"},
	}
	var kept []string
	for _, j := range youngEnough(jobs, now) {
		kept = append(kept, j.Location)
	}
	want := []string{"Dubai, United Arab Emirates", "Remote"}
	if fmt.Sprint(kept) != fmt.Sprint(want) {
		t.Errorf("youngEnough kept %v, want %v", kept, want)
	}

	// Every ingest-filtered term must also be sweepable, or storage and ingest
	// disagree about the same posting.
	if len(excludedPatterns()) != len(excludedLocations) {
		t.Error("the sweep must cover every excluded term")
	}
	for i, pattern := range excludedPatterns() {
		if pattern != "%"+excludedLocations[i]+"%" {
			t.Errorf("pattern %q does not wrap %q", pattern, excludedLocations[i])
		}
	}
}
