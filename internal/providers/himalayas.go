package providers

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// himalayas.app: remote jobs with location and timezone restrictions spelled
// out, via a keyless public API. Twenty newest per request regardless of the
// limit asked for — a window, so no absence-deletion; the slug is only a label.
type himalayasFeed struct {
	Jobs []struct {
		GUID         string   `json:"guid"`
		Title        string   `json:"title"`
		Company      string   `json:"companyName"`
		Application  string   `json:"applicationLink"`
		Restrictions []string `json:"locationRestrictions"`
		// Floats, not integers: an hourly posting reads 20.88, and an int64
		// here aborts the decode for the whole board — one contractor rate
		// used to take the entire feed down.
		MinSalary    float64 `json:"minSalary"`
		MaxSalary    float64 `json:"maxSalary"`
		Currency     string  `json:"currency"`
		SalaryPeriod string  `json:"salaryPeriod"`
		PubDate      int64   `json:"pubDate"`
	} `json:"jobs"`
}

func fetchHimalayas(ctx context.Context, _ string) ([]Job, error) {
	var feed himalayasFeed
	if err := getJSON(ctx, "https://himalayas.app/jobs/api?limit=100", &feed); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(feed.Jobs))
	for _, j := range feed.Jobs {
		location := strings.Join(j.Restrictions, ", ")
		if location == "" {
			location = "Worldwide"
		}
		var salary string
		if j.MinSalary > 0 && j.MaxSalary > 0 {
			salary = strings.TrimSpace(fmt.Sprintf("%s %s–%s / %s", j.Currency,
				money(j.MinSalary), money(j.MaxSalary), j.SalaryPeriod))
		}
		jobs = append(jobs, Job{
			ExternalID: j.GUID,
			Company:    strings.TrimSpace(j.Company),
			Title:      strings.TrimSpace(j.Title),
			Location:   location,
			Remote:     true,
			URL:        j.Application,
			Salary:     salary,
			PostedAt:   time.Unix(j.PubDate, 0).UTC(),
		})
	}
	return jobs, nil
}

// money prints a salary figure the way the posting wrote it: whole numbers for
// annual bands, cents for the hourly rates that come as fractions.
func money(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}
