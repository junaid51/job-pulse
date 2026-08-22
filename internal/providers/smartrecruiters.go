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

// fetchSmartRecruiters reads one company's postings, optionally narrowed to
// countries at the source.
//
// The slug is "Company" or "Company|ae,sa,in" with ISO alpha-2 codes. That
// filter is what makes global mass-hirers usable: Accor publishes 5,898
// postings worldwide and 532 across the UAE and Saudi, so asking for the two
// countries costs six requests instead of sixty and brings back no hotel jobs
// in Alberta. The API ignores a second country parameter rather than OR-ing
// them, hence one pass per country.
func fetchSmartRecruiters(ctx context.Context, slug string) ([]Job, error) {
	company, countryList, filtered := strings.Cut(slug, "|")
	countries := []string{""}
	if filtered {
		countries = nil
		for _, c := range strings.Split(countryList, ",") {
			if c = strings.TrimSpace(c); c != "" {
				countries = append(countries, c)
			}
		}
	}

	var jobs []Job
	for _, country := range countries {
		found, err := fetchSmartRecruitersCountry(ctx, company, country)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, found...)
	}
	return jobs, nil
}

func fetchSmartRecruitersCountry(ctx context.Context, company, country string) ([]Job, error) {
	var jobs []Job
	for offset := 0; ; offset += smartRecruitersPageSize {
		url := fmt.Sprintf(
			"https://api.smartrecruiters.com/v1/companies/%s/postings?limit=%d&offset=%d",
			company, smartRecruitersPageSize, offset,
		)
		if country != "" {
			url += "&country=" + country
		}
		var page smartRecruitersPage
		if err := getJSON(ctx, url, &page); err != nil {
			return nil, err
		}
		if len(page.Content) == 0 {
			return jobs, nil
		}
		jobs = append(jobs, page.jobs(company)...)
		if page.TotalFound > 0 && len(jobs) >= page.TotalFound {
			return jobs, nil
		}
	}
}

// jobs takes the company id, never the configured slug: a slug may carry a
// country filter ("AccorHotel|ae,sa") and the apply URL must not.
func (p smartRecruitersPage) jobs(company string) []Job {
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
			URL:        fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s", company, j.ID),
			PostedAt:   parseTime(time.RFC3339, j.ReleasedDate),
		})
	}
	return jobs
}
