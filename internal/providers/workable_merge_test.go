package providers

import (
	"strings"
	"testing"
)

// Workable sends one entry per location under a single shortcode. Stored
// as-is, the copies overwrote each other's location on every poll — and a role
// open in the UAE could end up filed under Bangladesh.
func TestWorkableMergesLocationsOfOnePosting(t *testing.T) {
	account := workableAccount{Name: "Robusta"}
	for _, place := range []struct {
		city, country string
		remote        bool
	}{
		{"", "India", false},
		{"", "United Arab Emirates", false},
		{"", "Bangladesh", true},
	} {
		account.Jobs = append(account.Jobs, struct {
			Shortcode     string `json:"shortcode"`
			Title         string `json:"title"`
			Telecommuting bool   `json:"telecommuting"`
			City          string `json:"city"`
			State         string `json:"state"`
			Country       string `json:"country"`
			URL           string `json:"url"`
			PublishedOn   string `json:"published_on"`
		}{
			Shortcode: "ABC123", Title: "Senior Optimizely Developer",
			Telecommuting: place.remote, City: place.city, Country: place.country,
			URL: "https://apply.workable.com/robusta/j/ABC123", PublishedOn: "2026-09-01",
		})
	}

	jobs := account.jobs()
	if len(jobs) != 1 {
		t.Fatalf("want one job for one shortcode, got %d", len(jobs))
	}
	if got := jobs[0].Location; got != "Bangladesh · India · United Arab Emirates" {
		t.Errorf("location = %q, want every place, sorted", got)
	}
	if !strings.Contains(strings.ToLower(jobs[0].Location), "united arab") {
		t.Error("the in-market place must survive the merge")
	}
	if !jobs[0].Remote {
		t.Error("remote on any copy makes the posting remote")
	}

	// The same payload in a different order must produce the same row, or the
	// poller rewrites it on every cycle.
	shuffled := workableAccount{Name: account.Name}
	shuffled.Jobs = append(shuffled.Jobs, account.Jobs[2], account.Jobs[0], account.Jobs[1])
	if shuffled.jobs()[0].Location != jobs[0].Location {
		t.Error("merge must not depend on the order the API happens to use")
	}
}

func TestWorkableKeepsDistinctPostingsApart(t *testing.T) {
	account := workableAccount{Name: "Salla"}
	type entry = struct {
		Shortcode     string `json:"shortcode"`
		Title         string `json:"title"`
		Telecommuting bool   `json:"telecommuting"`
		City          string `json:"city"`
		State         string `json:"state"`
		Country       string `json:"country"`
		URL           string `json:"url"`
		PublishedOn   string `json:"published_on"`
	}
	account.Jobs = append(account.Jobs,
		entry{Shortcode: "A", Title: "UX Writer", City: "Jeddah", Country: "Saudi Arabia"},
		entry{Shortcode: "B", Title: "Backend Engineer", City: "Riyadh", Country: "Saudi Arabia"},
	)
	if got := account.jobs(); len(got) != 2 {
		t.Errorf("two shortcodes must stay two jobs, got %d", len(got))
	}
}
