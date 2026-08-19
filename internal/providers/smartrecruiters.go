package providers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// smartRecruitersPage is one page of
// https://api.smartrecruiters.com/v1/companies/{slug}/postings
//
// This is the only provider that pages, and the only one whose list response has
// no apply URL — the posting URL is built from the slug and id instead of
// fetching each posting's detail endpoint.
type smartRecruitersPage struct {
	Offset     int `json:"offset"`
	Limit      int `json:"limit"`
	TotalFound int `json:"totalFound"`
	Content    []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		ReleasedDate string `json:"releasedDate"`
		Company      struct {
			Name string `json:"name"`
		} `json:"company"`
		Location struct {
			City         string `json:"city"`
			Region       string `json:"region"`
			Country      string `json:"country"`
			FullLocation string `json:"fullLocation"`
			Remote       bool   `json:"remote"`
		} `json:"location"`
	} `json:"content"`
}

const smartRecruitersPageSize = 100

func fetchSmartRecruiters(ctx context.Context, slug string) ([]Job, error) {
	var jobs []Job
	for offset := 0; ; offset += smartRecruitersPageSize {
		url := fmt.Sprintf(
			"https://api.smartrecruiters.com/v1/companies/%s/postings?limit=%d&offset=%d",
			slug, smartRecruitersPageSize, offset,
		)
		var page smartRecruitersPage
		if err := getJSON(ctx, url, &page); err != nil {
			return nil, err
		}
		if len(page.Content) == 0 {
			return jobs, nil
		}
		jobs = append(jobs, page.jobs(slug)...)
		if page.TotalFound > 0 && len(jobs) >= page.TotalFound {
			return jobs, nil
		}
	}
}

func (p smartRecruitersPage) jobs(slug string) []Job {
	jobs := make([]Job, 0, len(p.Content))
	for _, j := range p.Content {
		location := j.Location.FullLocation
		if location == "" {
			location = joinNonEmpty(j.Location.City, j.Location.Region, j.Location.Country)
		}
		jobs = append(jobs, Job{
			ExternalID: j.ID,
			Company:    j.Company.Name,
			Title:      strings.TrimSpace(j.Name),
			Location:   location,
			Remote:     j.Location.Remote || looksRemote(location),
			URL:        fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s", slug, j.ID),
			PostedAt:   parseTime(time.RFC3339, j.ReleasedDate),
		})
	}
	return jobs
}
