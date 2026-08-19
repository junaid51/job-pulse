// Package notify tells the phone when a poll has found something.
//
// One push per profile per cycle, never one per job: fifteen matches in a single
// cycle should not be fifteen buzzes.
//
// This talks to the FCM HTTP v1 API directly rather than through the Firebase
// Admin SDK. The SDK brings gRPC, OpenTelemetry, Firestore and Cloud Storage —
// fifty-seven indirect dependencies — to send one JSON POST. The only piece
// worth borrowing is the OAuth2 token exchange, which golang.org/x/oauth2 does.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // free hosts ship no zoneinfo; embed it

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/junaid51/job-pulse/internal/providers"
)

const messagingScope = "https://www.googleapis.com/auth/firebase.messaging"

// Notifier sends to every device that has registered a token. When Firebase is
// not configured it logs what it would have sent instead, so the whole project
// still runs from a clone with no Google account.
type Notifier struct {
	pool *pgxpool.Pool

	// client and projectID are empty when there are no credentials.
	client    *http.Client
	projectID string
}

// New builds a Notifier. Bad or missing credentials are not fatal: polling and
// the API are useful without push, and refusing to start would be a worse trade.
func New(ctx context.Context, pool *pgxpool.Pool, credentialsFile string) *Notifier {
	if credentialsFile == "" {
		slog.Info("push disabled; set GOOGLE_APPLICATION_CREDENTIALS to enable it")
		return &Notifier{pool: pool}
	}

	raw, err := os.ReadFile(credentialsFile)
	if err != nil {
		slog.Error("reading firebase credentials; push disabled", "error", err)
		return &Notifier{pool: pool}
	}
	// The service account file names its own project, so there is nothing else
	// to configure.
	var account struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &account); err != nil || account.ProjectID == "" {
		slog.Error("firebase credentials have no project_id; push disabled", "error", err)
		return &Notifier{pool: pool}
	}
	credentials, err := google.CredentialsFromJSON(ctx, raw, messagingScope)
	if err != nil {
		slog.Error("firebase credentials rejected; push disabled", "error", err)
		return &Notifier{pool: pool}
	}

	// The token source refreshes itself, so this client is good for the lifetime
	// of the process.
	client := oauth2.NewClient(ctx, credentials.TokenSource)
	client.Timeout = 20 * time.Second

	slog.Info("push enabled", "project", account.ProjectID)
	return &Notifier{pool: pool, projectID: account.ProjectID, client: client}
}

// Notify sends one summary for one profile, to the device that owns it —
// profiles belong to devices, and a match on your search should not buzz
// someone else's phone.
func (n *Notifier) Notify(ctx context.Context, owner, profileName string, jobs []providers.Job) {
	if len(jobs) == 0 {
		return
	}
	title, body := Summarize(profileName, jobs)

	if n.client == nil {
		slog.Info("notification (not sent: push disabled)", "title", title, "body", body)
		return
	}

	tokens, err := n.tokens(ctx, owner)
	if err != nil {
		slog.Error("reading device tokens", "error", err)
		return
	}
	if len(tokens) == 0 {
		slog.Info("notification (no devices registered)", "title", title, "body", body)
		return
	}

	// One user with one or two devices, so a loop is clearer than a batch API and
	// gives each token its own error to act on.
	sent := 0
	for _, device := range tokens {
		if isQuietHours(device.timezone, time.Now()) {
			// A 3am match waits for morning. The feed has it either way; only
			// the buzz is suppressed, so nothing needs a retry queue.
			slog.Info("notification held for quiet hours", "title", title, "tz", device.timezone)
			continue
		}
		token := device.token
		switch err := n.send(ctx, token, title, body); {
		case err == nil:
			sent++
		case isTokenDead(err):
			// The app was uninstalled or the token rotated. Forget it.
			n.forget(ctx, token)
		default:
			slog.Error("sending notification", "error", err)
		}
	}
	slog.Info("notification sent", "title", title, "devices", sent)
}

// send posts one message. The payload is deliberately just a title and a body:
// the app refetches from the API rather than trusting anything in a push.
func (n *Notifier) send(ctx context.Context, token, title, body string) error {
	payload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"token":        token,
			"notification": map[string]string{"title": title, "body": body},
		},
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", n.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	message := make([]byte, 512)
	read, _ := resp.Body.Read(message)
	return &sendError{status: resp.StatusCode, body: string(message[:read])}
}

// sendError carries the status code so a dead token can be told apart from a
// problem worth logging.
type sendError struct {
	status int
	body   string
}

func (e *sendError) Error() string {
	return fmt.Sprintf("fcm returned %d: %s", e.status, strings.TrimSpace(e.body))
}

// isTokenDead reports whether FCM is saying this registration will never work
// again. 404 UNREGISTERED means the app is gone; 400 INVALID_ARGUMENT means the
// token is malformed, and since the rest of the payload is fixed, the token is
// the only thing that can be wrong.
func isTokenDead(err error) bool {
	var sendErr *sendError
	if !errors.As(err, &sendErr) {
		return false
	}
	return sendErr.status == http.StatusNotFound || sendErr.status == http.StatusBadRequest
}

type deviceToken struct {
	token    string
	timezone string
}

func (n *Notifier) tokens(ctx context.Context, owner string) ([]deviceToken, error) {
	rows, err := n.pool.Query(ctx, `select token, timezone from devices where owner = $1`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []deviceToken
	for rows.Next() {
		var d deviceToken
		if err := rows.Scan(&d.token, &d.timezone); err != nil {
			return nil, err
		}
		tokens = append(tokens, d)
	}
	return tokens, rows.Err()
}

// isQuietHours reports whether it is night where the device lives: 22:00 to
// 08:00, fixed on purpose — a preference screen for two numbers is not worth
// its surface. No timezone means never quiet, which is what the row's default
// gives devices registered before timezones existed.
func isQuietHours(timezone string, now time.Time) bool {
	if timezone == "" {
		return false
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false
	}
	hour := now.In(location).Hour()
	return hour >= 22 || hour < 8
}

func (n *Notifier) forget(ctx context.Context, token string) {
	if _, err := n.pool.Exec(ctx, `delete from devices where token = $1`, token); err != nil {
		slog.Error("deleting stale device token", "error", err)
		return
	}
	slog.Info("removed a device token FCM rejected")
}

// maxCompanies is how many company names fit in a notification before it stops
// being readable on a lock screen.
const maxCompanies = 3

// Summarize writes the two lines a push notification shows: how many jobs and
// which companies they are at.
func Summarize(profileName string, jobs []providers.Job) (title, body string) {
	noun := "new jobs"
	if len(jobs) == 1 {
		noun = "new job"
	}
	title = fmt.Sprintf("%d %s · %s", len(jobs), noun, profileName)

	// Several postings at one company are common, and repeating the name tells
	// you nothing.
	var companies []string
	seen := map[string]bool{}
	for _, job := range jobs {
		if name := job.Company; name != "" && !seen[name] {
			seen[name] = true
			companies = append(companies, name)
		}
	}

	switch {
	case len(companies) == 0:
		body = "Open JobPulse to see them."
	case len(companies) <= maxCompanies:
		body = strings.Join(companies, ", ")
	default:
		body = fmt.Sprintf("%s and %d more",
			strings.Join(companies[:maxCompanies], ", "), len(companies)-maxCompanies)
	}
	return title, body
}
