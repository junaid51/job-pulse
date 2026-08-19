package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Jobven is a market-wide aggregator that crawls employer career sites
// directly: applyUrl points at the company's own ATS posting, which is also
// why cross-provider URL dedup exists (see poll.insertJobs) — a board watched
// directly and a Jobven search can surface the same posting.
//
// The API is keyed (JOBVEN_API_KEY) and metered — a few hundred calls per
// month on the free tier — so it sits in poll.minPollInterval like careerjet.
// The "slug" is a saved search: a location with '+' for spaces ("dubai",
// "saudi+arabia"), or the word "remote" for remote-anywhere. Each call asks
// only for postings newer than three days, far wider than the poll gap.
type jobvenPage struct {
	Data []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		ApplyURL  string `json:"applyUrl"`
		PostedAt  int64  `json:"postedAt"`
		Remote    string `json:"remoteType"`
		Locations []struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
			Country  string `json:"addressCountry"`
		} `json:"locations"`
		Companies []struct {
			Name string `json:"name"`
		} `json:"companies"`
	} `json:"data"`
}

func fetchJobven(ctx context.Context, slug string) ([]Job, error) {
	key := os.Getenv("JOBVEN_API_KEY")
	if key == "" {
		return nil, errors.New("jobven needs JOBVEN_API_KEY")
	}

	query := url.Values{"limit": {"100"}}
	query.Set("postedAfter", fmt.Sprint(time.Now().Add(-72*time.Hour).Unix()))
	if slug == "remote" {
		// The API validates remoteType as an array; [] is its spelling.
		query.Set("remoteType[]", "remote")
	} else {
		query.Set("location", strings.ReplaceAll(slug, "+", " "))
	}

	request, err := http.NewRequestWithContext(ctx,
		http.MethodGet, "https://api.jobven.com/v1/public/jobs?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-API-Key", key)
	request.Header.Set("User-Agent", userAgent)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("jobven query: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jobven query: unexpected status %d", response.StatusCode)
	}
	var page jobvenPage
	if err := decodeJSON(response, &page); err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(page.Data))
	for _, j := range page.Data {
		var location string
		if len(j.Locations) > 0 {
			l := j.Locations[0]
			parts := make([]string, 0, 2)
			if l.Locality != "" {
				parts = append(parts, l.Locality)
			}
			if l.Region != "" && l.Region != l.Locality {
				parts = append(parts, l.Region)
			}
			location = strings.Join(parts, ", ")
		}
		var company string
		if len(j.Companies) > 0 {
			company = j.Companies[0].Name
		}
		jobs = append(jobs, Job{
			ExternalID: j.ID,
			Company:    strings.TrimSpace(company),
			Title:      strings.TrimSpace(j.Title),
			Location:   location,
			Remote:     j.Remote == "remote" || looksRemote(location),
			URL:        j.ApplyURL,
			PostedAt:   time.Unix(j.PostedAt, 0).UTC(),
		})
	}
	return jobs, nil
}
