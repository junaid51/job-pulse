package providers

import (
	"context"
	"strconv"
	"strings"
)

// recruiteeOffers is the shape of https://{slug}.recruitee.com/api/offers/
//
// The response includes offers that are not open, so anything other than
// "published" is skipped. Timestamps are "2006-01-02 15:04:05 UTC", which is not
// any of the layouts in the time package.
type recruiteeOffers struct {
	Offers []struct {
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Location    string `json:"location"`
		Remote      bool   `json:"remote"`
		CareersURL  string `json:"careers_url"`
		PublishedAt string `json:"published_at"`
		CompanyName string `json:"company_name"`
	} `json:"offers"`
}

const recruiteeTimeLayout = "2006-01-02 15:04:05 MST"

func fetchRecruitee(ctx context.Context, slug string) ([]Job, error) {
	var offers recruiteeOffers
	if err := getJSON(ctx, "https://"+slug+".recruitee.com/api/offers/", &offers); err != nil {
		return nil, err
	}
	return offers.jobs(), nil
}

func (o recruiteeOffers) jobs() []Job {
	jobs := make([]Job, 0, len(o.Offers))
	for _, j := range o.Offers {
		if j.Status != "published" {
			continue
		}
		jobs = append(jobs, Job{
			ExternalID: strconv.FormatInt(j.ID, 10),
			Company:    j.CompanyName,
			Title:      strings.TrimSpace(j.Title),
			Location:   j.Location,
			Remote:     j.Remote || looksRemote(j.Location),
			URL:        j.CareersURL,
			PostedAt:   parseTime(recruiteeTimeLayout, j.PublishedAt),
		})
	}
	return jobs
}
