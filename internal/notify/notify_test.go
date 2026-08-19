package notify

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/junaid51/job-pulse/internal/providers"
)

func TestSummarize(t *testing.T) {
	at := func(companies ...string) []providers.Job {
		jobs := make([]providers.Job, 0, len(companies))
		for _, company := range companies {
			jobs = append(jobs, providers.Job{Company: company, Title: "Engineer"})
		}
		return jobs
	}

	tests := []struct {
		name      string
		profile   string
		jobs      []providers.Job
		wantTitle string
		wantBody  string
	}{
		{
			name:      "one job is singular",
			profile:   "Backend Go",
			jobs:      at("Stripe"),
			wantTitle: "1 new job · Backend Go",
			wantBody:  "Stripe",
		},
		{
			name:      "several companies are listed",
			profile:   "Backend Go",
			jobs:      at("Stripe", "Spotify", "OpenAI"),
			wantTitle: "3 new jobs · Backend Go",
			wantBody:  "Stripe, Spotify, OpenAI",
		},
		{
			name:      "repeated companies are named once",
			profile:   "Remote only",
			jobs:      at("Stripe", "Stripe", "Stripe"),
			wantTitle: "3 new jobs · Remote only",
			wantBody:  "Stripe",
		},
		{
			name:      "a long list is truncated",
			profile:   "ML",
			jobs:      at("Stripe", "Spotify", "OpenAI", "Visa", "Channable"),
			wantTitle: "5 new jobs · ML",
			wantBody:  "Stripe, Spotify, OpenAI and 2 more",
		},
		{
			name:      "no company names still says something useful",
			profile:   "ML",
			jobs:      at("", ""),
			wantTitle: "2 new jobs · ML",
			wantBody:  "Open JobPulse to see them.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, body := Summarize(tc.profile, tc.jobs)
			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

// A Notifier with no credentials must be usable rather than nil, so the poller
// never has to check, and it must not touch the database on the way to doing
// nothing — hence the nil pool.
func TestNotifierWithoutCredentialsIsUsable(t *testing.T) {
	notifier := New(t.Context(), nil, "")
	if notifier.client != nil {
		t.Error("there should be no HTTP client without credentials")
	}
	notifier.Notify(t.Context(), "device-1", "Backend Go", []providers.Job{{Company: "Stripe"}})
}

// Bad credentials must degrade to logging rather than stop the process.
//
// A structurally valid service account with a garbage private key is NOT in this
// list: the oauth2 library parses lazily, so that failure only surfaces on the
// first send — where it is logged and retried the next cycle.
func TestNotifierWithUnusableCredentials(t *testing.T) {
	for name, contents := range map[string]string{
		"missing file":  "",
		"not json":      "this is not json",
		"no project_id": `{"type":"service_account"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := name + "-does-not-exist.json"
			if contents != "" {
				path = t.TempDir() + "/creds.json"
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			notifier := New(t.Context(), nil, path)
			if notifier.client != nil {
				t.Error("unusable credentials should leave push disabled")
			}
			notifier.Notify(t.Context(), "device-1", "Backend Go", []providers.Job{{Company: "Stripe"}})
		})
	}
}

func TestIsTokenDead(t *testing.T) {
	tests := map[int]bool{
		http.StatusNotFound:            true,  // UNREGISTERED: the app is gone
		http.StatusBadRequest:          true,  // INVALID_ARGUMENT: malformed token
		http.StatusUnauthorized:        false, // our credentials, not the token
		http.StatusInternalServerError: false, // FCM is having a moment
		http.StatusTooManyRequests:     false,
	}
	for status, want := range tests {
		if got := isTokenDead(&sendError{status: status}); got != want {
			t.Errorf("isTokenDead(%d) = %v, want %v", status, got, want)
		}
	}
	if isTokenDead(errors.New("connection refused")) {
		t.Error("a transport error must not delete a token")
	}
}

// Quiet hours are the device's night, not the server's.
func TestIsQuietHours(t *testing.T) {
	noonUTC := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)    // 16:00 in Dubai
	nightUTC := time.Date(2026, 8, 19, 20, 0, 0, 0, time.UTC)   // 00:00 in Dubai
	morningUTC := time.Date(2026, 8, 19, 4, 30, 0, 0, time.UTC) // 08:30 in Dubai

	if isQuietHours("Asia/Dubai", noonUTC) {
		t.Error("4pm in Dubai is not quiet hours")
	}
	if !isQuietHours("Asia/Dubai", nightUTC) {
		t.Error("midnight in Dubai is quiet hours")
	}
	if isQuietHours("Asia/Dubai", morningUTC) {
		t.Error("08:30 in Dubai is past quiet hours")
	}
	if isQuietHours("", nightUTC) {
		t.Error("no timezone means never quiet")
	}
	if isQuietHours("Not/AZone", nightUTC) {
		t.Error("an unparseable timezone must not eat notifications")
	}
}
