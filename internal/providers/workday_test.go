package providers

import (
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

// "3 Locations" is not a place. Storing it would hide the job from every region
// filter, including the one it is actually in.
func TestWorkdayLocationDropsTheSummary(t *testing.T) {
	for in, want := range map[string]string{
		"UAE, Dubai":      "UAE, Dubai",
		"3 Locations":     "",
		"1 Location":      "",
		"12 Locations":    "",
		"Location, Egypt": "Location, Egypt",
		"UK, Remote":      "UK, Remote",
	} {
		if got := workdayLocation(in); got != want {
			t.Errorf("workdayLocation(%q) = %q, want %q", in, got, want)
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
