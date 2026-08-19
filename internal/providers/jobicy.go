package providers

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// jobicy.com: remote jobs with a per-region geo field, keyless public API.
// Fifty newest per request — a window, so no absence-deletion. The slug is a
// Jobicy industry filter ("dev", "design", …); "all" fetches everything.
type jobicyFeed struct {
	Jobs []struct {
		ID      int64  `json:"id"`
		Title   string `json:"jobTitle"`
		Company string `json:"companyName"`
		Geo     string `json:"jobGeo"`
		URL     string `json:"url"`
		PubDate string `json:"pubDate"`
	} `json:"jobs"`
}

func fetchJobicy(ctx context.Context, slug string) ([]Job, error) {
	url := "https://jobicy.com/api/v2/remote-jobs?count=50"
	if slug != "" && slug != "all" {
		url += "&industry=" + slug
	}
	var feed jobicyFeed
	if err := getJSON(ctx, url, &feed); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(feed.Jobs))
	for _, j := range feed.Jobs {
		posted := parseTime(time.RFC3339, j.PubDate)
		if posted.IsZero() {
			// Jobicy has been seen writing both RFC3339 and a space-separated
			// variant of it.
			posted = parseTime("2006-01-02 15:04:05", j.PubDate)
		}
		jobs = append(jobs, Job{
			ExternalID: strconv.FormatInt(j.ID, 10),
			Company:    strings.TrimSpace(j.Company),
			Title:      strings.TrimSpace(j.Title),
			Location:   j.Geo,
			Remote:     true,
			URL:        j.URL,
			PostedAt:   posted,
		})
	}
	return jobs, nil
}
