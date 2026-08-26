package providers

import (
	"strings"
	"testing"
	"time"
)

func TestWorkdayPostedAt(t *testing.T) {
	now := time.Date(2026, 8, 25, 17, 42, 0, 0, time.UTC)
	today := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Time
	}{
		{"Posted Today", today},
		{"Posted Yesterday", today.AddDate(0, 0, -1)},
		{"Posted 3 Days Ago", today.AddDate(0, 0, -3)},
		{"Posted 30+ Days Ago", today.AddDate(0, 0, -30)},
		{"Posted 1 Day Ago", today.AddDate(0, 0, -1)},
		{"", time.Time{}},
		{"Posted a while back", time.Time{}},
	}
	for _, c := range cases {
		if got := workdayPostedAt(c.in, now); !got.Equal(c.want) {
			t.Errorf("workdayPostedAt(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// A listed place is used as listed; "3 Locations" is not a place, and the
// posting's URL is where the real one comes from. Half of the NVIDIA board
// arrived unplaceable before this.
func TestWorkdayLocation(t *testing.T) {
	cases := []struct{ text, path, want string }{
		{"UAE, Dubai", "/job/UAE-Dubai/Engineer_JR1", "UAE, Dubai"},
		{"UK, Remote", "/job/UK-Remote/Engineer_JR1", "UK, Remote"},
		{"Location, Egypt", "/job/x/y_JR1", "Location, Egypt"},
		// summarised: recover the primary location from the path
		{"2 Locations", "/job/India-Bengaluru/Senior-AI-ML-Software-Engineer_JR2024127", "India Bengaluru"},
		{"3 Locations", "/job/US-CA-Santa-Clara/Engineer_JR2", "US CA Santa Clara"},
		{"1 Location", "/job/SA---Riyadh/Senior-HSE-Manager_R180560", "SA Riyadh"},
		// the components run together on some tenants
		{"2 Locations", "/job/AEAbu-DhabiTrade-Center-and-West-Tower/Chef_R1", "AE Abu Dhabi Trade Center and West Tower"},
		// nothing to recover
		{"4 Locations", "/job/Engineer_JR3", ""},
		{"", "/job/India-Pune/Engineer_JR4", "India Pune"},
	}
	for _, c := range cases {
		if got := workdayLocation(c.text, c.path); got != c.want {
			t.Errorf("workdayLocation(%q, %q) = %q, want %q", c.text, c.path, got, c.want)
		}
	}
}

// The recovered string has to contain the words the matcher and the Where
// filter look for; it does not have to equal Workday's own formatting.
func TestRecoveredLocationIsSearchable(t *testing.T) {
	for path, needle := range map[string]string{
		"/job/AEAbu-DhabiTrade-Center-and-West-Tower/Chef_R1": "abu dhabi",
		"/job/India-Bengaluru/Engineer_JR1":                   "bengaluru",
		"/job/SA---Riyadh/Manager_R1":                         "riyadh",
		"/job/United-Arab-Emirates/Engineer_R1":               "united arab emirates",
	} {
		got := strings.ToLower(locationFromPath(path))
		if !strings.Contains(got, needle) {
			t.Errorf("locationFromPath(%q) = %q, want it to contain %q", path, got, needle)
		}
	}
}

func TestWorkdaySlugNeedsASite(t *testing.T) {
	if _, err := fetchWorkday(t.Context(), "acme.wd3.myworkdayjobs.com"); err == nil {
		t.Error("a slug without a site should be rejected before any request")
	}
}

// Parsons puts the location in the first bullet field and the requisition in
// the second. Taking the first collapsed 808 postings onto eighteen ids.
func TestWorkdayID(t *testing.T) {
	cases := []struct {
		fields []string
		path   string
		want   string
	}{
		{[]string{"R2128704"}, "/job/x_R2128704", "R2128704"},
		{[]string{"SA - Riyadh", "R180560"}, "/job/y_R180560", "R180560"},
		{[]string{"SA - Riyadh, Qiddiya", "R182411"}, "/job/z", "R182411"},
		{[]string{"Full time"}, "/job/no-id", "/job/no-id"},
		{nil, "/job/none", "/job/none"},
		{[]string{"", "  "}, "/job/blank", "/job/blank"},
	}
	for _, c := range cases {
		if got := workdayID(c.fields, c.path); got != c.want {
			t.Errorf("workdayID(%q) = %q, want %q", c.fields, got, c.want)
		}
	}
}

func TestWorkdayFacetsFromSlugQuery(t *testing.T) {
	got, err := workdayFacets("locationHierarchy1=aaa,bbb&workerSubType=ccc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got["locationHierarchy1"]) != 2 || got["locationHierarchy1"][1] != "bbb" {
		t.Errorf("locationHierarchy1 = %v", got["locationHierarchy1"])
	}
	if len(got["workerSubType"]) != 1 {
		t.Errorf("workerSubType = %v", got["workerSubType"])
	}
	empty, err := workdayFacets("")
	if err != nil || len(empty) != 0 {
		t.Errorf("empty query should give no facets: %v %v", empty, err)
	}
}
