package providers

import (
	"context"
	"strings"
)

// manatalPage is one page of
// https://api.manatal.com/open/v3/career-page/{slug}/jobs/
//
// Manatal is the ATS of choice for Gulf recruitment agencies, which makes its
// boards the closest thing to agency inventory a polite poller can reach. The
// payload has no posting date, so PostedAt stays zero and the job ages from
// first sight; organization_name is the department, not the company, so the
// company comes from companies.txt.
type manatalPage struct {
	Count   int    `json:"count"`
	Next    string `json:"next"`
	Results []struct {
		Hash            string `json:"hash"`
		PositionName    string `json:"position_name"`
		LocationDisplay string `json:"location_display"`
		IsRemote        *bool  `json:"is_remote"` // frequently null
	} `json:"results"`
}

// manatalMaxPages bounds the walk the same way teamtailor's does; these boards
// hold dozens of postings, not thousands.
const manatalMaxPages = 10

func fetchManatal(ctx context.Context, slug string) ([]Job, error) {
	var jobs []Job
	url := "https://api.manatal.com/open/v3/career-page/" + slug + "/jobs/"
	for range manatalMaxPages {
		var page manatalPage
		if err := getJSON(ctx, url, &page); err != nil {
			return nil, err
		}
		jobs = append(jobs, page.jobs(slug)...)
		if page.Next == "" {
			return jobs, nil
		}
		url = page.Next
	}
	return jobs, nil
}

func (p manatalPage) jobs(slug string) []Job {
	jobs := make([]Job, 0, len(p.Results))
	for _, j := range p.Results {
		remote := j.IsRemote != nil && *j.IsRemote
		jobs = append(jobs, Job{
			ExternalID: j.Hash,
			// Company deliberately left empty; the poller fills the display name.
			Title:    strings.TrimSpace(j.PositionName),
			Location: j.LocationDisplay,
			Remote:   remote || looksRemote(j.LocationDisplay),
			// careers-page.com is Manatal's hosted careers domain.
			URL: "https://www.careers-page.com/" + slug + "/job/" + j.Hash,
		})
	}
	return jobs
}
