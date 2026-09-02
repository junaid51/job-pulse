package providers

import (
	"context"
	"sort"
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

// jobs merges the entries that share a shortcode.
//
// Workable returns one entry per location, all carrying the same shortcode: a
// posting open in Mexico, Colombia and Brazil arrives three times, and one
// staffing board sent 2,077 entries for 805 actual jobs. Storing them as-is
// meant the copies fought over one row every cycle — the last writer set the
// location, so a role open in the UAE could be stored as Bangladesh and vanish
// from an in-market feed, and the row was rewritten on every poll for ever.
//
// One row per shortcode, then, whose location names every place it is open.
// That is what a substring location filter needs to see, and it is stable
// between polls because the places are sorted rather than left in API order.
func (a workableAccount) jobs() []Job {
	order := make([]string, 0, len(a.Jobs))
	merged := make(map[string]*Job, len(a.Jobs))
	places := make(map[string][]string, len(a.Jobs))

	for _, j := range a.Jobs {
		location := joinNonEmpty(j.City, j.State, j.Country)
		remote := j.Telecommuting || looksRemote(location)
		if job, seen := merged[j.Shortcode]; seen {
			// A posting is remote if any of its locations is.
			job.Remote = job.Remote || remote
			places[j.Shortcode] = appendUnique(places[j.Shortcode], location)
			continue
		}
		order = append(order, j.Shortcode)
		merged[j.Shortcode] = &Job{
			ExternalID: j.Shortcode,
			Company:    a.Name,
			Title:      strings.TrimSpace(j.Title),
			Remote:     remote,
			URL:        j.URL,
			PostedAt:   parseTime(workableDateLayout, j.PublishedOn),
		}
		places[j.Shortcode] = appendUnique(nil, location)
	}

	jobs := make([]Job, 0, len(order))
	for _, shortcode := range order {
		job := merged[shortcode]
		list := places[shortcode]
		sort.Strings(list)
		job.Location = strings.Join(list, " · ")
		jobs = append(jobs, *job)
	}
	return jobs
}

func appendUnique(list []string, value string) []string {
	if value = strings.TrimSpace(value); value == "" {
		return list
	}
	for _, existing := range list {
		if strings.EqualFold(existing, value) {
			return list
		}
	}
	return append(list, value)
}
