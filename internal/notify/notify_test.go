package notify

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/junaid51/job-pulse/internal/providers"
)

// An announcement is one posting, however many saved searches caught it and
// however many cities it was listed in. Over a week, 294 alerts went out for
// 173 distinct jobs: three overlapping devops searches meant one Riyadh role
// buzzed the same phone three times.
func TestSummarizeOnePostingIsActionableFromTheLockScreen(t *testing.T) {
	title, body := Summarize([]Announcement{{
		Job:       providers.Job{Title: "React Native Developer", Company: "Tawantech"},
		Locations: []string{"Riyadh"},
		Searches:  []string{"Frontend"},
	}})
	if title != "React Native Developer · Tawantech" {
		t.Errorf("title = %q", title)
	}
	if body != "Riyadh — caught by Frontend" {
		t.Errorf("body = %q", body)
	}
}

// Pay is the first thing this reader checks and the commonest reason they do
// not apply. One in five matched postings carries it — "$20 - $50/hour" — and
// until now you had to open the app to find that out.
func TestSummarizeShowsThePayWhenTheBoardStatesIt(t *testing.T) {
	_, body := Summarize([]Announcement{{
		Job: providers.Job{
			Title: "Senior Backend Engineer", Company: "Quik Hire Staffing",
			Salary: "$20 - $50/hour",
		},
		Locations: []string{"United Arab Emirates"},
		Searches:  []string{"Backend · Dubai"},
	}})
	if body != "United Arab Emirates · $20 - $50/hour — caught by Backend · Dubai" {
		t.Errorf("body = %q", body)
	}
}

// Most boards say nothing about pay, and an alert must not imply they did.
func TestSummarizeSaysNothingAboutPayWhenTheBoardDidNot(t *testing.T) {
	_, body := Summarize([]Announcement{{
		Job:       providers.Job{Title: "DevOps Engineer", Company: "Devoteam"},
		Locations: []string{"Riyadh"},
		Searches:  []string{"Devops · Gulf"},
	}})
	if body != "Riyadh — caught by Devops · Gulf" {
		t.Errorf("body = %q", body)
	}
}

func TestSummarizeNamesEverySearchThatCaughtIt(t *testing.T) {
	_, body := Summarize([]Announcement{{
		Job:       providers.Job{Title: "DevOps Engineer", Company: "Devoteam"},
		Locations: []string{"Riyadh"},
		Searches:  []string{"Devops · Gulf", "sam", "Salim Jobs"},
	}})
	if body != "Riyadh — caught by Devops · Gulf, sam and Salim Jobs" {
		t.Errorf("body = %q", body)
	}
}

// One role listed in nine cities is one role. Elastic's vector-search opening
// is stored nine times for that reason.
func TestSummarizeJoinsTheCitiesOfOnePosting(t *testing.T) {
	_, body := Summarize([]Announcement{{
		Job:       providers.Job{Title: "Senior Software Engineer", Company: "Elastic"},
		Locations: []string{"Dubai", "Bengaluru", "Remote"},
		Searches:  []string{"Backend"},
	}})
	if body != "Dubai, Bengaluru and Remote — caught by Backend" {
		t.Errorf("body = %q", body)
	}
}

func TestSummarizeSeveralPostingsCountsThem(t *testing.T) {
	at := func(company string) Announcement {
		return Announcement{
			Job:      providers.Job{Company: company, Title: "Engineer"},
			Searches: []string{"Backend"},
		}
	}
	title, body := Summarize([]Announcement{at("Stripe"), at("Tawantech"), at("Aldar")})
	if title != "3 new roles" {
		t.Errorf("title = %q", title)
	}
	if body != "Stripe, Tawantech, Aldar" {
		t.Errorf("body = %q", body)
	}
}

func TestSummarizeCapsALongCompanyList(t *testing.T) {
	var many []Announcement
	for _, c := range []string{"A", "B", "C", "D", "E", "F"} {
		many = append(many, Announcement{Job: providers.Job{Company: c, Title: "Engineer"}})
	}
	title, body := Summarize(many)
	if title != "6 new roles" {
		t.Errorf("title = %q", title)
	}
	if body != "A, B, C and 3 more" {
		t.Errorf("body = %q", body)
	}
}

func TestSummarizeSurvivesAPostingWithNothingToSay(t *testing.T) {
	title, body := Summarize([]Announcement{{Job: providers.Job{Title: "Engineer"}}})
	if title == "" || body == "" {
		t.Errorf("title = %q, body = %q; both must say something", title, body)
	}
}

func TestNotifierWithoutCredentialsIsUsable(t *testing.T) {
	notifier := New(t.Context(), nil, "")
	if notifier.client != nil {
		t.Error("there should be no HTTP client without credentials")
	}
	notifier.Notify(t.Context(), "device-1", []Announcement{{
		Job:      providers.Job{Company: "Stripe", Title: "Engineer"},
		Searches: []string{"Backend Go"},
	}})
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
			notifier.Notify(t.Context(), "device-1", []Announcement{{
				Job:      providers.Job{Company: "Stripe", Title: "Engineer"},
				Searches: []string{"Backend Go"},
			}})
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

	if isQuietHours("Asia/Dubai", 22, 8, noonUTC) {
		t.Error("4pm in Dubai is not quiet hours")
	}
	if !isQuietHours("Asia/Dubai", 22, 8, nightUTC) {
		t.Error("midnight in Dubai is quiet hours")
	}
	if isQuietHours("Asia/Dubai", 22, 8, morningUTC) {
		t.Error("08:30 in Dubai is past quiet hours")
	}
	if isQuietHours("", 22, 8, nightUTC) {
		t.Error("no timezone means never quiet")
	}
	if isQuietHours("Not/AZone", 22, 8, nightUTC) {
		t.Error("an unparseable timezone must not eat notifications")
	}
}

// A push that says "3 new jobs" has to open the screen listing them.
func TestBuildMessageLinksToNotifications(t *testing.T) {
	raw, err := json.Marshal(buildMessage("tok", "1 new job · UAE", "Careem",
		"https://example.test/#/notifications"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Message struct {
			Token        string            `json:"token"`
			Notification map[string]string `json:"notification"`
			Webpush      struct {
				FCMOptions map[string]string `json:"fcm_options"`
			} `json:"webpush"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if link := got.Message.Webpush.FCMOptions["link"]; link != "https://example.test/#/notifications" {
		t.Errorf("webpush link = %q, want the notifications screen", link)
	}
	if got.Message.Notification["title"] != "1 new job · UAE" || got.Message.Token != "tok" {
		t.Errorf("title and token must survive the build: %s", raw)
	}
}

func TestAppURLTrimsTrailingSlash(t *testing.T) {
	t.Setenv("APP_URL", "https://fork.example/")
	if got := appURL(); got != "https://fork.example" {
		t.Errorf("appURL() = %q, want no trailing slash", got)
	}
}

// The window is the reader's to choose, including choosing not to have one.
func TestQuietHoursWindows(t *testing.T) {
	// 06:45 in Dubai, the case that motivated this: silenced by the old fixed
	// 22-08, and by a 23-07 window too, but audible once the reader narrows it
	// to 23-06. That choice is now theirs to make.
	early := time.Date(2026, 8, 23, 2, 45, 0, 0, time.UTC)
	if !isQuietHours("Asia/Dubai", 22, 8, early) {
		t.Error("06:45 is inside 22-08")
	}
	if !isQuietHours("Asia/Dubai", 23, 7, early) {
		t.Error("06:45 is still inside 23-07")
	}
	if isQuietHours("Asia/Dubai", 23, 6, early) {
		t.Error("06:45 is outside 23-06")
	}
	// Equal hours disable the window entirely.
	for _, at := range []time.Time{early, time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)} {
		if isQuietHours("Asia/Dubai", 0, 0, at) {
			t.Error("from == to means never quiet")
		}
	}
	// A window that does not wrap midnight reads literally.
	afternoon := time.Date(2026, 8, 23, 9, 30, 0, 0, time.UTC) // 13:30 in Dubai
	if !isQuietHours("Asia/Dubai", 13, 14, afternoon) {
		t.Error("13:30 is inside 13-14")
	}
	if isQuietHours("Asia/Dubai", 14, 15, afternoon) {
		t.Error("13:30 is outside 14-15")
	}
}
