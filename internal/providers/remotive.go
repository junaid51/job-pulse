package providers

import (
	"context"
	"strconv"
	"strings"
)

// remotive.com: a curated remote-jobs board with a keyless public API. It
// answers with only its freshest few listings — a window, not a full board —
// so it lives in minPollInterval and its jobs leave through the age sweep.
//
// The slug is a Remotive category ("software-dev", "design", …); "all" fetches
// the whole feed.
type remotiveFeed struct {
	Jobs []struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Company   string `json:"company_name"`
		Location  string `json:"candidate_required_location"`
		Salary    string `json:"salary"`
		URL       string `json:"url"`
		Published string `json:"publication_date"`
	} `json:"jobs"`
}

func fetchRemotive(ctx context.Context, slug string) ([]Job, error) {
	url := "https://remotive.com/api/remote-jobs"
	if slug != "" && slug != "all" {
		url += "?category=" + slug
	}
	var feed remotiveFeed
	if err := getJSON(ctx, url, &feed); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(feed.Jobs))
	for _, j := range feed.Jobs {
		jobs = append(jobs, Job{
			ExternalID: strconv.FormatInt(j.ID, 10),
			Company:    strings.TrimSpace(j.Company),
			Title:      strings.TrimSpace(j.Title),
			Location:   j.Location,
			Remote:     true,
			URL:        j.URL,
			Salary:     strings.TrimSpace(j.Salary),
			// No timezone in the timestamp; Remotive is a Europe-run board and
			// a few hours of skew does not matter at a 45-day horizon.
			PostedAt: parseTime("2006-01-02T15:04:05", j.Published),
		})
	}
	return jobs, nil
}
