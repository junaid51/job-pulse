package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// JobsPipe aggregates scraped boards (Indeed, LinkedIn, Glassdoor) alongside
// ATS crawls — its value here is the local employers that post only to those
// boards and are invisible to every other provider. The free tier meters jobs
// returned (1,000/month), which shapes everything: the slug is a deliberately
// narrow saved search and each call asks only for the last two days of
// postings. (discovered_at_gte would be cheaper — fetch each job exactly once
// — but that filter 502s their backend as of 2026-08; the two-day posted-at
// window re-returns each job ~4 times, ~480 credits/month for the shipped
// search, and the upsert makes the repeats no-ops.)
//
// Slug: "countries|titles" — ISO alpha-2 codes comma-separated, then title
// terms comma-separated with '+' for spaces and '-' prefixing exclusions:
// "ae,sa|frontend,full+stack,-civil". Auth: JOBSPIPE_API_KEY.
type jobsPipePage struct {
	Data []struct {
		ID         string `json:"id"`
		Title      string `json:"job_title"`
		Company    string `json:"company"`
		Location   string `json:"long_location"`
		ShortLoc   string `json:"location"`
		Remote     bool   `json:"remote"`
		URL        string `json:"url"`
		Salary     string `json:"salary_string"`
		DatePosted string `json:"date_posted"`
		// Liveness: this provider re-checks its sources and says so. A
		// posting it has seen close is worse than no posting at all.
		Status   string  `json:"status"`
		ClosedAt *string `json:"closed_at"`
	} `json:"data"`
}

func fetchJobsPipe(ctx context.Context, slug string) ([]Job, error) {
	key := os.Getenv("JOBSPIPE_API_KEY")
	if key == "" {
		return nil, errors.New("jobspipe needs JOBSPIPE_API_KEY")
	}

	countriesPart, titlesPart, found := strings.Cut(slug, "|")
	if !found {
		return nil, fmt.Errorf("jobspipe slug must be %q", "countries|titles")
	}
	var countries []string
	for _, c := range strings.Split(countriesPart, ",") {
		countries = append(countries, strings.ToUpper(strings.TrimSpace(c)))
	}
	var titles, exclude []string
	for _, t := range strings.Split(titlesPart, ",") {
		t = strings.ReplaceAll(strings.TrimSpace(t), "+", " ")
		if negated := strings.TrimPrefix(t, "-"); negated != t {
			exclude = append(exclude, negated)
		} else if t != "" {
			titles = append(titles, t)
		}
	}

	filters := map[string]any{
		"job_country_code_or":    countries,
		"job_title_or":           titles,
		"posted_at_max_age_days": 2,
		"limit":                  100,
	}
	if len(exclude) > 0 {
		filters["job_title_not"] = exclude
	}
	body, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx,
		http.MethodPost, "https://api.jobspipe.dev/v1/jobs/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("jobspipe query: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jobspipe query: unexpected status %d", response.StatusCode)
	}
	var page jobsPipePage
	if err := decodeJSON(response, &page); err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(page.Data))
	for _, j := range page.Data {
		if j.ClosedAt != nil || (j.Status != "" && j.Status != "active") {
			continue
		}
		location := j.Location
		if location == "" {
			location = j.ShortLoc
		}
		jobs = append(jobs, Job{
			ExternalID: j.ID,
			Company:    strings.TrimSpace(j.Company),
			Title:      strings.TrimSpace(j.Title),
			Location:   location,
			Remote:     j.Remote || looksRemote(location),
			URL:        j.URL,
			Salary:     strings.TrimSpace(j.Salary),
			PostedAt:   parseTime(time.RFC3339, j.DatePosted),
		})
	}
	return jobs, nil
}
