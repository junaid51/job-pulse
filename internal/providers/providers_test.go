package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The fixtures in testdata are real responses from each provider, captured from
// a live board, with long description strings truncated and only the first two
// postings kept. Parsing is what breaks when a provider changes its payload, so
// that is what these tests pin down.
func TestParseBoards(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		parse   func(t *testing.T, raw []byte) []Job
		want    Job
		count   int
	}{
		{
			name:    "greenhouse",
			fixture: "greenhouse.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[greenhouseBoard](t, raw).jobs() },
			count:   2,
			want: Job{
				ExternalID: "8077887",
				Company:    "Stripe",
				Title:      "Account Executive, Bridge", // trailing space trimmed
				Location:   "SF, NYC, SEA, CHI",
				Remote:     false,
				URL:        "https://stripe.com/jobs/search?gh_jid=8077887",
				PostedAt:   mustTime(t, time.RFC3339, "2026-07-22T13:15:53-04:00"),
			},
		},
		{
			name:    "lever",
			fixture: "lever.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[leverPostings](t, raw).jobs() },
			count:   2,
			want: Job{
				ExternalID: "890b2c0f-f46f-4a4b-bb73-3a6af6e0edd5",
				Company:    "", // Lever does not report it; the poller fills it in
				Title:      "Advertiser Solutions Vendor Lead - Programmatic and Direct Support",
				Location:   "London",
				Remote:     false, // workplaceType is "hybrid"
				URL:        "https://jobs.lever.co/spotify/890b2c0f-f46f-4a4b-bb73-3a6af6e0edd5",
				// createdAt is epoch milliseconds, not a timestamp string.
				PostedAt: mustTime(t, time.RFC3339, "2026-07-20T17:49:59.619Z"),
			},
		},
		{
			name:    "ashby",
			fixture: "ashby.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[ashbyBoard](t, raw).jobs() },
			count:   2,
			want: Job{
				ExternalID: "8fb1615c-34bf-47c4-a1d1-b7b2f836bbd3",
				Title:      "Technical Program Manager, Compute Infrastructure",
				Location:   "San Francisco",
				Remote:     false, // isRemote is null
				URL:        "https://jobs.ashbyhq.com/openai/8fb1615c-34bf-47c4-a1d1-b7b2f836bbd3",
				PostedAt:   mustTime(t, time.RFC3339, "2026-03-12T16:38:15.322+00:00"),
			},
		},
		{
			name:    "smartrecruiters",
			fixture: "smartrecruiters.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[smartRecruitersPage](t, raw).jobs("Visa") },
			count:   2,
			want: Job{
				ExternalID: "744000133907678",
				Company:    "Visa",
				Title:      "Sr. Manager",
				Location:   "Austin, TX, United States",
				Remote:     false,
				// The list response has no apply URL, so it is constructed.
				URL:      "https://jobs.smartrecruiters.com/Visa/744000133907678",
				PostedAt: mustTime(t, time.RFC3339, "2026-06-24T10:00:11.853Z"),
			},
		},
		{
			name:    "workable",
			fixture: "workable.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[workableAccount](t, raw).jobs() },
			count:   2,
			want: Job{
				ExternalID: "0FD01ABC66",
				Company:    "Blueground",
				Title:      "Business Development Representative",
				Location:   "United States", // blank city and state dropped
				Remote:     true,            // telecommuting
				URL:        "https://apply.workable.com/j/0FD01ABC66",
				PostedAt:   mustTime(t, workableDateLayout, "2026-08-18"),
			},
		},
		{
			name:    "teamtailor",
			fixture: "teamtailor.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[teamtailorFeed](t, raw).jobs() },
			count:   2,
			want: Job{
				ExternalID: "7ac6b47c-c2d1-48b5-803a-2d84bb3762fe",
				Company:    "Property Finder",
				Title:      "Sales Team Leader (B2B SaaS)",
				Location:   "Cairo, Egypt", // ISO "EG" expanded to what a profile matches
				Remote:     false,
				URL:        "https://propertyfinder.teamtailor.com/jobs/7982798-sales-team-leader-b2b-saas",
				PostedAt:   mustTime(t, time.RFC3339, "2026-06-29T09:37:39+04:00"),
			},
		},
		{
			name:    "manatal",
			fixture: "manatal.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[manatalPage](t, raw).jobs("nathanhr") },
			count:   2,
			want: Job{
				ExternalID: "X9583V6Y",
				Company:    "", // the payload's organization_name is a department; the poller fills the company
				Title:      "Content Creator",
				Location:   "Dubai, United Arab Emirates",
				Remote:     false, // is_remote is null
				URL:        "https://www.careers-page.com/nathanhr/job/X9583V6Y",
				// No posting date in the payload: ages from first sight.
			},
		},
		{
			name:    "recruitee",
			fixture: "recruitee.json",
			parse:   func(t *testing.T, raw []byte) []Job { return decode[recruiteeOffers](t, raw).jobs() },
			count:   2,
			want: Job{
				ExternalID: "2697907",
				Company:    "Channable",
				Title:      "Technical Customer Support DACH - German speaking",
				Location:   "Utrecht, Utrecht, Netherlands",
				Remote:     false,
				URL:        "https://jobs.channable.com/o/technical-customer-support-dach-german-speaking-2",
				PostedAt:   mustTime(t, recruiteeTimeLayout, "2026-08-03 15:40:43 UTC"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}

			jobs := tc.parse(t, raw)

			if len(jobs) != tc.count {
				t.Fatalf("parsed %d jobs, want %d", len(jobs), tc.count)
			}
			if !equalJob(jobs[0], tc.want) {
				t.Errorf("first job mismatch\n got: %+v\nwant: %+v", jobs[0], tc.want)
			}
			for i, j := range jobs {
				if j.ExternalID == "" || j.Title == "" || j.URL == "" {
					t.Errorf("job %d is missing an id, title or url: %+v", i, j)
				}
			}
		})
	}
}

// Both of these boards return postings that are not open, and shipping those as
// alerts would be worse than missing them.
func TestParseSkipsClosedPostings(t *testing.T) {
	t.Run("recruitee keeps only published", func(t *testing.T) {
		raw := []byte(`{"offers":[
			{"id":1,"title":"Open","status":"published","careers_url":"https://x/1"},
			{"id":2,"title":"Draft","status":"draft","careers_url":"https://x/2"},
			{"id":3,"title":"Closed","status":"closed","careers_url":"https://x/3"}
		]}`)
		jobs := decode[recruiteeOffers](t, raw).jobs()
		if len(jobs) != 1 || jobs[0].Title != "Open" {
			t.Fatalf("got %+v, want only the published offer", jobs)
		}
	})

	t.Run("ashby keeps only listed", func(t *testing.T) {
		raw := []byte(`{"jobs":[
			{"id":"a","title":"Listed","isListed":true,"jobUrl":"https://x/a"},
			{"id":"b","title":"Unlisted","isListed":false,"jobUrl":"https://x/b"},
			{"id":"c","title":"Unspecified","jobUrl":"https://x/c"}
		]}`)
		jobs := decode[ashbyBoard](t, raw).jobs()
		if len(jobs) != 2 {
			t.Fatalf("got %d jobs, want 2 (listed and unspecified)", len(jobs))
		}
		for _, j := range jobs {
			if j.Title == "Unlisted" {
				t.Error("unlisted job was kept")
			}
		}
	})
}

// Teamtailor postings can name several cities, or none at all — both appear on
// real boards (dubizzle has postings with no jobLocation).
func TestTeamtailorLocations(t *testing.T) {
	raw := []byte(`{"items":[
		{"id":"a","title":"Multi","url":"https://x/a","_jobposting":{"jobLocation":[
			{"address":{"addressLocality":"Dubai","addressCountry":"AE"}},
			{"address":{"addressLocality":"Riyadh","addressCountry":"SA"}}]}},
		{"id":"b","title":"Nowhere","url":"https://x/b","_jobposting":{}},
		{"id":"c","title":"Anywhere","url":"https://x/c","_jobposting":{"jobLocation":[
			{"address":{"addressLocality":"Remote","addressCountry":"XX"}}]}}
	]}`)
	jobs := decode[teamtailorFeed](t, raw).jobs()
	if got, want := jobs[0].Location, "Dubai, United Arab Emirates / Riyadh, Saudi Arabia"; got != want {
		t.Errorf("multi-location = %q, want %q", got, want)
	}
	if jobs[1].Location != "" {
		t.Errorf("missing jobLocation should give an empty location, got %q", jobs[1].Location)
	}
	if got, want := jobs[2].Location, "Remote, XX"; got != want {
		t.Errorf("unknown country code should pass through, got %q want %q", got, want)
	}
	if !jobs[2].Remote {
		t.Error(`a location saying "Remote" should set the flag`)
	}
}

func TestRemoteDetection(t *testing.T) {
	// Greenhouse has no remote flag at all, so the location text is all there is.
	raw := []byte(`{"jobs":[
		{"id":1,"title":"A","location":{"name":"Remote - US"},"absolute_url":"https://x/1"},
		{"id":2,"title":"B","location":{"name":"New York"},"absolute_url":"https://x/2"}
	]}`)
	jobs := decode[greenhouseBoard](t, raw).jobs()
	if !jobs[0].Remote {
		t.Error(`"Remote - US" should be remote`)
	}
	if jobs[1].Remote {
		t.Error(`"New York" should not be remote`)
	}
}

func TestParseTimeIsLenient(t *testing.T) {
	// A posting with an unreadable date is still a posting.
	for _, value := range []string{"", "not a date", "2026-13-45"} {
		if got := parseTime(time.RFC3339, value); !got.IsZero() {
			t.Errorf("parseTime(%q) = %v, want zero time", value, got)
		}
	}
}

func TestAllProvidersRegistered(t *testing.T) {
	want := []string{"greenhouse", "lever", "ashby", "smartrecruiters", "workable", "recruitee", "teamtailor", "careerjet", "manatal"}
	if len(All) != len(want) {
		t.Errorf("All has %d providers, want %d", len(All), len(want))
	}
	for _, name := range want {
		if All[name] == nil {
			t.Errorf("provider %q is not registered", name)
		}
	}
}

func decode[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	return v
}

func mustTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(layout, value)
	if err != nil {
		t.Fatalf("bad expected time %q: %v", value, err)
	}
	return parsed
}

// equalJob compares instants with Equal rather than ==, since two times can name
// the same moment in different zones.
func equalJob(got, want Job) bool {
	return got.ExternalID == want.ExternalID &&
		got.Company == want.Company &&
		got.Title == want.Title &&
		got.Location == want.Location &&
		got.Remote == want.Remote &&
		got.URL == want.URL &&
		got.PostedAt.Equal(want.PostedAt)
}
