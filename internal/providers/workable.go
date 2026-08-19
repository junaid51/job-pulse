package providers

import (
	"context"
	"strings"
)

// workableAccount is the shape of
// https://apply.workable.com/api/v1/widget/accounts/{slug}?details=true
//
// published_on is a date with no time, so postings made on the same day cannot
// be ordered against each other.
type workableAccount struct {
	Name string `json:"name"`
	Jobs []struct {
		Shortcode     string `json:"shortcode"`
		Title         string `json:"title"`
		Telecommuting bool   `json:"telecommuting"`
		City          string `json:"city"`
		State         string `json:"state"`
		Country       string `json:"country"`
		URL           string `json:"url"`
		PublishedOn   string `json:"published_on"`
	} `json:"jobs"`
}

const workableDateLayout = "2006-01-02"

func fetchWorkable(ctx context.Context, slug string) ([]Job, error) {
	var account workableAccount
	url := "https://apply.workable.com/api/v1/widget/accounts/" + slug + "?details=true"
	if err := getJSON(ctx, url, &account); err != nil {
		return nil, err
	}
	return account.jobs(), nil
}

func (a workableAccount) jobs() []Job {
	jobs := make([]Job, 0, len(a.Jobs))
	for _, j := range a.Jobs {
		location := joinNonEmpty(j.City, j.State, j.Country)
		jobs = append(jobs, Job{
			ExternalID: j.Shortcode,
			Company:    a.Name,
			Title:      strings.TrimSpace(j.Title),
			Location:   location,
			Remote:     j.Telecommuting || looksRemote(location),
			URL:        j.URL,
			PostedAt:   parseTime(workableDateLayout, j.PublishedOn),
		})
	}
	return jobs
}
