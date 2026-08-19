package providers

import (
	"context"
	"strings"
	"time"
)

// leverPostings is the shape of
// https://api.lever.co/v0/postings/{slug}?mode=json
//
// The response is a bare array. An unknown slug returns 404; a real company
// with nothing open returns an empty array.
type leverPostings []struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// CreatedAt is milliseconds since the epoch, not a timestamp string.
	CreatedAt     int64  `json:"createdAt"`
	HostedURL     string `json:"hostedUrl"`
	WorkplaceType string `json:"workplaceType"`
	Categories    struct {
		Location string `json:"location"`
	} `json:"categories"`
}

func fetchLever(ctx context.Context, slug string) ([]Job, error) {
	var postings leverPostings
	if err := getJSON(ctx, "https://api.lever.co/v0/postings/"+slug+"?mode=json", &postings); err != nil {
		return nil, err
	}
	return postings.jobs(), nil
}

func (p leverPostings) jobs() []Job {
	jobs := make([]Job, 0, len(p))
	for _, j := range p {
		var posted time.Time
		if j.CreatedAt > 0 {
			posted = time.UnixMilli(j.CreatedAt).UTC()
		}
		jobs = append(jobs, Job{
			ExternalID: j.ID,
			// Lever does not report the company name; the poller fills it in.
			Title:    strings.TrimSpace(j.Text),
			Location: j.Categories.Location,
			Remote:   strings.EqualFold(j.WorkplaceType, "remote") || looksRemote(j.Categories.Location),
			URL:      j.HostedURL,
			PostedAt: posted,
		})
	}
	return jobs
}
