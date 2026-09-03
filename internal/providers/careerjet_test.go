package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The careerjet fixture is a real response. Its URLs are per-request tracking
// tokens, so identity comes from a content hash — the property these tests pin
// down is that the hash is deterministic and built only from stable fields.
func TestCareerjetParse(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "careerjet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var page careerjetPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}

	jobs := page.jobs()
	if len(jobs) != 2 {
		t.Fatalf("parsed %d jobs, want 2", len(jobs))
	}

	first := jobs[0]
	if first.Title != "Software Engineer III" || first.Company != "SBS" || first.Location != "Dubai" {
		t.Errorf("first job fields wrong: %+v", first)
	}
	want := time.Date(2026, 8, 18, 7, 45, 48, 0, time.UTC)
	if !first.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", first.PostedAt, want)
	}
	if len(first.ExternalID) != 24 {
		t.Errorf("ExternalID = %q, want a 24-hex-char hash", first.ExternalID)
	}

	// Parsing the same payload twice must yield the same identities even though
	// the URL field would differ between real requests.
	again := page.jobs()
	if again[0].ExternalID != first.ExternalID {
		t.Error("hash identity is not deterministic")
	}
	// A reposted ad (same job, bumped date) must keep its identity, or every
	// bump becomes a duplicate row and a duplicate notification.
	if careerjetID("t", "c", "l") != careerjetID("t", "c", "l") {
		t.Error("identity must be stable")
	}
	if careerjetID("t", "c", "l") == careerjetID("t", "c", "elsewhere") {
		t.Error("location must distinguish postings")
	}
}

func TestParseCareerjetSearch(t *testing.T) {
	keywords, location, locale, err := parseCareerjetSearch("software+engineer|dubai|en_AE")
	if err != nil || keywords != "software engineer" || location != "dubai" || locale != "en_AE" {
		t.Errorf("got %q %q %q %v", keywords, location, locale, err)
	}

	// Country-wide search: empty location, default locale.
	keywords, location, locale, err = parseCareerjetSearch("designer")
	if err != nil || keywords != "designer" || location != "" || locale != "en_GB" {
		t.Errorf("got %q %q %q %v", keywords, location, locale, err)
	}

	if _, _, _, err := parseCareerjetSearch(""); err == nil {
		t.Error("empty search should be rejected")
	}
	if _, _, _, err := parseCareerjetSearch("a|b|c|d"); err == nil {
		t.Error("four parts should be rejected")
	}
}

// Without credentials the fetch must fail fast with a message that names the
// missing configuration, so the board's last_error says what to do.
func TestCareerjetWithoutCredentials(t *testing.T) {
	t.Setenv("CAREERJET_API_KEY", "")
	t.Setenv("CAREERJET_SITE", "")
	_, err := fetchCareerjet(t.Context(), "engineer|dubai|en_AE")
	if err == nil || !strings.Contains(err.Error(), "CAREERJET_API_KEY") {
		t.Errorf("err = %v, want a message naming the env vars", err)
	}
}
