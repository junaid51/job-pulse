package match

import (
	"testing"

	"github.com/junaid51/job-pulse/internal/providers"
)

func TestMatches(t *testing.T) {
	backendGo := Criteria{Keywords: []string{"go", "backend"}, Locations: []string{"berlin", "remote"}}

	tests := []struct {
		name     string
		criteria Criteria
		job      providers.Job
		want     bool
	}{
		{
			name:     "keyword and location both hit",
			criteria: backendGo,
			job:      providers.Job{Title: "Backend Engineer", Location: "Berlin, Germany"},
			want:     true,
		},
		{
			name:     "keyword hits but location does not",
			criteria: backendGo,
			job:      providers.Job{Title: "Backend Engineer", Location: "Paris, France"},
			want:     false,
		},
		{
			name:     "location hits but keyword does not",
			criteria: backendGo,
			job:      providers.Job{Title: "Account Executive", Location: "Berlin, Germany"},
			want:     false,
		},
		{
			name:     "matching is case insensitive",
			criteria: Criteria{Keywords: []string{"GOLANG"}},
			job:      providers.Job{Title: "Senior golang developer"},
			want:     true,
		},
		{
			name:     "keywords match anywhere in the title, not just the start",
			criteria: Criteria{Keywords: []string{"platform"}},
			job:      providers.Job{Title: "Engineer, Core Platform Team"},
			want:     true,
		},
		{
			name:     "no keywords means any title",
			criteria: Criteria{Locations: []string{"lisbon"}},
			job:      providers.Job{Title: "Anything At All", Location: "Lisbon"},
			want:     true,
		},
		{
			name:     "no locations means anywhere",
			criteria: Criteria{Keywords: []string{"rust"}},
			job:      providers.Job{Title: "Rust Engineer", Location: "Ulaanbaatar"},
			want:     true,
		},
		{
			name:     "empty criteria matches everything",
			criteria: Criteria{},
			job:      providers.Job{Title: "Barista", Location: "Nowhere"},
			want:     true,
		},
		{
			name:     "remote only rejects an onsite job",
			criteria: Criteria{RemoteOnly: true},
			job:      providers.Job{Title: "Backend Engineer", Location: "Berlin", Remote: false},
			want:     false,
		},
		{
			name:     "remote only accepts a remote job",
			criteria: Criteria{RemoteOnly: true},
			job:      providers.Job{Title: "Backend Engineer", Location: "Remote", Remote: true},
			want:     true,
		},
		{
			name:     "blank keywords are ignored rather than matching everything",
			criteria: Criteria{Keywords: []string{"  ", ""}},
			job:      providers.Job{Title: "Anything"},
			want:     false,
		},
		{
			name:     "a keyword is a substring, so short ones match broadly",
			criteria: Criteria{Keywords: []string{"go"}},
			job:      providers.Job{Title: "Head of Diversity, Golang not required"},
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Matches(tc.criteria, tc.job); got != tc.want {
				t.Errorf("Matches(%+v, %q/%q) = %v, want %v",
					tc.criteria, tc.job.Title, tc.job.Location, got, tc.want)
			}
		})
	}
}

// Boards write "Dubai", "Dubai, United Arab Emirates" and "Abu Dhabi, UAE" for
// the same place; typing "uae" has to find all of them. Discovered the hard way
// by the first real profile matching nothing.
func TestLocationAliases(t *testing.T) {
	uae := Criteria{Locations: []string{"UAE"}}
	for _, location := range []string{
		"Dubai",
		"Dubai, United Arab Emirates",
		"Abu Dhabi, UAE",
		"Dubai & Sharjah",
		"UAE, Dubai",
	} {
		if !Matches(uae, providers.Job{Title: "Engineer", Location: location}) {
			t.Errorf("locations=[UAE] should match %q", location)
		}
	}
	if Matches(uae, providers.Job{Title: "Engineer", Location: "London"}) {
		t.Error("locations=[UAE] must not match London")
	}

	if !Matches(Criteria{Locations: []string{"ksa"}},
		providers.Job{Title: "Engineer", Location: "Riyadh, Saudi Arabia"}) {
		t.Error(`locations=[ksa] should match "Riyadh, Saudi Arabia"`)
	}

	// Aliases are for locations only; a keyword named like one stays literal.
	if Matches(Criteria{Keywords: []string{"uae"}},
		providers.Job{Title: "Engineer, Dubai team", Location: ""}) {
		t.Error("keyword aliases must not expand")
	}
}
