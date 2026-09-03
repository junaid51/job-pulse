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
	// Ten minutes is inside jobicy's hourly window and seven hours is well past
	// it, so "fresh" is skipped and "stale" is the first of that provider due.
	recent, stale := now.Add(-10*time.Minute), now.Add(-7*time.Hour)
	companies := []Company{
		{Provider: "greenhouse", Slug: "always", LastPolledAt: &recent},
		{Provider: "jobicy", Slug: "fresh", LastPolledAt: &recent},
		{Provider: "jobicy", Slug: "stale", LastPolledAt: &stale},
		{Provider: "jobicy", Slug: "never"},
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
	for _, j := range youngEnough(jobs, now, maxJobAge) {
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
	for _, j := range youngEnough(jobs, now, maxJobAge) {
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

// A remote job restricted to a country the reader cannot work in is noise, and
// on the live Himalayas feed that was nine listings in ten.
func TestReachable(t *testing.T) {
	for _, location := range []string{
		"Worldwide", "Anywhere", "EMEA", "India", "United Arab Emirates",
		"Saudi Arabia", "Global", "", "United States, India", "Middle East",
	} {
		if !reachable(location) {
			t.Errorf("reachable(%q) = false, want true", location)
		}
	}
	for _, location := range []string{
		"United States", "Canada", "United Kingdom", "China", "Macao",
		"Luxembourg", "Philippines", "Remote - US",
	} {
		if reachable(location) {
			t.Errorf("reachable(%q) = true, want false", location)
		}
	}
	// The ingest filter and the sweep patterns come from one list in match, so
	// they cannot drift apart.
	if len(reachablePatterns()) == 0 {
		t.Error("the sweep must have patterns to match on")
	}
}

// A board too expensive to fetch every cycle gets its own interval, without
// being treated as a metered provider (whose limit is per key, not per board).
func TestBoardInterval(t *testing.T) {
	if interval, limited := boardInterval(Company{Provider: "ashby", Slug: "snowflake"}); !limited || interval != time.Hour {
		t.Errorf("heavy board = (%v, %v), want (1h, true)", interval, limited)
	}
	if _, limited := boardInterval(Company{Provider: "ashby", Slug: "ziina"}); limited {
		t.Error("an ordinary ashby board should poll every cycle")
	}
	if interval, limited := boardInterval(Company{Provider: "jobven", Slug: "dubai"}); !limited || interval != 4*time.Hour {
		t.Errorf("metered provider = (%v, %v), want (4h, true)", interval, limited)
	}

	// Two heavy boards of the same provider are independent; two metered
	// boards of the same provider still take turns.
	now := time.Now()
	companies := []Company{
		{Provider: "ashby", Slug: "snowflake"},
		{Provider: "ashby", Slug: "cohere"},
		{Provider: "jobven", Slug: "dubai"},
		{Provider: "jobven", Slug: "india"},
	}
	var slugs []string
	for _, c := range dueNow(companies, now) {
		slugs = append(slugs, c.Slug)
	}
	if fmt.Sprint(slugs) != fmt.Sprint([]string{"snowflake", "cohere", "dubai"}) {
		t.Errorf("dueNow kept %v, want [snowflake cohere dubai]", slugs)
	}
}

// A metered board that fails must not keep its provider's one slot per cycle.
// Six searches once went live behind a bad credential and only the first was
// ever attempted, because a failure left its timestamp untouched and dueNow
// takes the first due board per provider.
func TestDueNowRotatesPastAFailingBoard(t *testing.T) {
	now := time.Now()
	justTried, longAgo := now.Add(-time.Minute), now.Add(-4*time.Hour)
	companies := []Company{
		// jobicy is metered, hourly. The first has just been attempted —
		// whether it succeeded or not — so the next one is the one due.
		{Provider: "jobicy", Slug: "first", LastPolledAt: &justTried},
		{Provider: "jobicy", Slug: "second", LastPolledAt: &longAgo},
		{Provider: "jobicy", Slug: "third", LastPolledAt: &longAgo},
	}
	var kept []string
	for _, c := range dueNow(companies, now) {
		kept = append(kept, c.Slug)
	}
	if len(kept) != 1 || kept[0] != "second" {
		t.Errorf("dueNow kept %v, want just [second]", kept)
	}
}
