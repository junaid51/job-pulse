// Package providers fetches job postings from public company job boards.
//
// Every one of these APIs is unauthenticated and scoped to a single company's
// board: none of them offer search across companies, which is why JobPulse works
// from a list of boards rather than a query.
//
// Adding a provider means writing one file and adding one line to All.
package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Job is a posting normalized across providers. Company is left empty by boards
// that do not report it; the poller fills those in from companies.txt.
type Job struct {
	ExternalID string
	Company    string
	Title      string
	Location   string
	Remote     bool
	URL        string
	// Salary is display text, kept only when a board publishes it.
	Salary   string
	PostedAt time.Time
}

// A Provider fetches every currently open posting on one company's board.
//
// This is a function rather than an interface because providers hold no state
// and there is only one thing to do with them.
type Provider func(ctx context.Context, slug string) ([]Job, error)

// All is the registry. The keys are the provider names used in companies.txt.
var All = map[string]Provider{
	"greenhouse":      fetchGreenhouse,
	"lever":           fetchLever,
	"ashby":           fetchAshby,
	"smartrecruiters": fetchSmartRecruiters,
	"workable":        fetchWorkable,
	"recruitee":       fetchRecruitee,
	"teamtailor":      fetchTeamtailor,
	"careerjet":       fetchCareerjet,
	"manatal":         fetchManatal,
	"phenom":          fetchPhenom,
	"oracle":          fetchOracle,
}

const userAgent = "jobpulse/0.1 (+https://github.com/junaid51/job-pulse)"

var client = &http.Client{Timeout: 30 * time.Second}

// getJSON fetches url and decodes the body into v, retrying once on a 5xx or a
// network error: boards blip, and the next poll is fifteen minutes away.
func getJSON(ctx context.Context, url string, v any) error {
	err := get(ctx, url, v)
	if err == nil || !retryable(err) {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}
	return get(ctx, url, v)
}

// decodeJSON decodes a successful response body into v.
func decodeJSON(resp *http.Response, v any) error {
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", resp.Request.URL, err)
	}
	return nil
}

func get(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return statusError{code: resp.StatusCode, url: url}
	}
	return decodeJSON(resp, v)
}

type statusError struct {
	code int
	url  string
}

func (e statusError) Error() string {
	return fmt.Sprintf("%s: unexpected status %d", e.url, e.code)
}

func retryable(err error) bool {
	var se statusError
	if errors.As(err, &se) {
		return se.code >= 500
	}
	var ne net.Error
	return errors.As(err, &ne)
}

// parseTime is lenient on purpose: a posting with an unreadable date is still a
// posting, and the zero time sorts it last.
func parseTime(layout, value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(layout, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

// looksRemote is the fallback for boards that have no remote flag and only say
// so in the location text.
func looksRemote(location string) bool {
	return strings.Contains(strings.ToLower(location), "remote")
}

// joinNonEmpty builds "City, State, Country" while skipping the blanks that
// several boards leave in those fields.
func joinNonEmpty(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}
