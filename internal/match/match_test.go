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

	// "United Kingdom" does not contain the letters "uk"; the alias has to
	// bridge it, exactly as with the Emirates.
	for _, location := range []string{"Remote, United Kingdom", "London", "London, UK"} {
		if !Matches(Criteria{Locations: []string{"UK"}},
			providers.Job{Title: "Engineer", Location: location}) {
			t.Errorf("locations=[UK] should match %q", location)
		}
	}
	if !Matches(Criteria{Locations: []string{"usa"}},
		providers.Job{Title: "Engineer", Location: "Remote, United States"}) {
		t.Error(`locations=[usa] should match "Remote, United States"`)
	}

	// Aliases are for locations only; a keyword named like one stays literal.
	if Matches(Criteria{Keywords: []string{"uae"}},
		providers.Job{Title: "Engineer, Dubai team", Location: ""}) {
		t.Error("keyword aliases must not expand")
	}
}

// "Frontend" and "React Engineer" are the same job wearing different titles;
// the keyword dictionary bridges them. See aliases.go.
func TestKeywordAliases(t *testing.T) {
	frontend := Criteria{Keywords: []string{"Frontend"}}
	for _, title := range []string{
		"Frontend Developer",
		"Front-End Engineer",
		"Senior React Engineer",
		"Software Engineer, Payments",
		"TypeScript Developer",
	} {
		if !Matches(frontend, providers.Job{Title: title}) {
			t.Errorf("keywords=[Frontend] should match %q", title)
		}
	}
	if Matches(frontend, providers.Job{Title: "Accountant"}) {
		t.Error("keywords=[Frontend] must not match Accountant")
	}

	backend := Criteria{Keywords: []string{"backend"}}
	for _, title := range []string{"Golang Engineer", "Java Spring Developer", "Node Engineer"} {
		if !Matches(backend, providers.Job{Title: title}) {
			t.Errorf("keywords=[backend] should match %q", title)
		}
	}

	for _, keyword := range []string{"full stack", "full-stack", "fullstack"} {
		if !Matches(Criteria{Keywords: []string{keyword}},
			providers.Job{Title: "Full-Stack Engineer"}) {
			t.Errorf("keywords=[%s] should match Full-Stack Engineer", keyword)
		}
	}

	// Role aliases apply to keywords only; a location named like one stays literal.
	if Matches(Criteria{Locations: []string{"frontend"}},
		providers.Job{Title: "Engineer", Location: "React Office Park"}) {
		t.Error("keyword aliases must not leak into location matching")
	}
}

// "-senior" is the entire exclusion syntax: literal, case-insensitive, and it
// always wins over a positive match.
func TestNegativeKeywords(t *testing.T) {
	criteria := Criteria{Keywords: []string{"engineer", "-senior", "-intern"}}
	if !Matches(criteria, providers.Job{Title: "Backend Engineer"}) {
		t.Error("plain engineer should match")
	}
	for _, title := range []string{"Senior Backend Engineer", "Engineering Intern", "SENIOR Engineer"} {
		if Matches(criteria, providers.Job{Title: title}) {
			t.Errorf("%q should be excluded", title)
		}
	}

	// Only negatives: everything except the excluded.
	onlyNegative := Criteria{Keywords: []string{"-manager"}}
	if !Matches(onlyNegative, providers.Job{Title: "Engineer"}) {
		t.Error("only-negatives should match any non-excluded title")
	}
	if Matches(onlyNegative, providers.Job{Title: "Engineering Manager"}) {
		t.Error("only-negatives should still exclude")
	}

	// Negatives are literal — no alias expansion. "-frontend" must not exclude
	// a "Software Engineer" merely because the alias dictionary links them.
	literal := Criteria{Keywords: []string{"-frontend"}}
	if !Matches(literal, providers.Job{Title: "Software Engineer"}) {
		t.Error("negative keywords must not expand through aliases")
	}
	if Matches(literal, providers.Job{Title: "Frontend Developer"}) {
		t.Error("literal negative should still exclude")
	}

	// A lone minus is garbage-only input, and garbage keyword lists match
	// nothing — same contract as a list of blanks.
	if Matches(Criteria{Keywords: []string{"-"}}, providers.Job{Title: "Anything"}) {
		t.Error("a bare minus should match nothing, like a blank")
	}
}
