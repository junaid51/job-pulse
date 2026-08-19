package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Phenom powers the career sites of large enterprises — the employers whose
// ATSes (Workday, SAP) are otherwise walled off. The slug is the careers host
// itself (careers.majidalfuttaim.com), and the site's own search runs on a
// public POST /widgets endpoint that pages with from/size and reports
// totalHits, so the full listing is reachable and absence means closed.
type phenomPage struct {
	RefineSearch struct {
		TotalHits int `json:"totalHits"`
		Data      struct {
			Jobs []struct {
				JobSeqNo         string `json:"jobSeqNo"`
				Title            string `json:"title"`
				Company          string `json:"company"` // unreliable: sometimes a bare number
				CityStateCountry string `json:"cityStateCountry"`
				PostedDate       string `json:"postedDate"` // 2025-12-20T15:09:43.000+0000
			} `json:"jobs"`
		} `json:"data"`
	} `json:"refineSearch"`
}

const (
	phenomPageSize   = 50
	phenomMaxPages   = 10 // 500 postings bounds any board this tool watches
	phenomTimeLayout = "2006-01-02T15:04:05.000-0700"
)

func fetchPhenom(ctx context.Context, slug string) ([]Job, error) {
	var jobs []Job
	for page := 0; page < phenomMaxPages; page++ {
		var result phenomPage
		if err := phenomSearch(ctx, slug, page*phenomPageSize, &result); err != nil {
			return nil, err
		}
		jobs = append(jobs, result.jobs(slug)...)
		if len(jobs) >= result.RefineSearch.TotalHits || len(result.RefineSearch.Data.Jobs) == 0 {
			return jobs, nil
		}
	}
	return jobs, nil
}

// phenomSearch posts the widget query the career site's own search page makes.
func phenomSearch(ctx context.Context, host string, from int, v any) error {
	body, err := json.Marshal(map[string]any{
		"lang": "en_us", "deviceType": "desktop", "country": "us",
		"pageName": "search-results", "ddoKey": "refineSearch",
		"sortBy": "", "subsearch": "", "from": from, "jobs": true,
		"counts": true, "all_fields": []string{"category", "country", "city"},
		"size": phenomPageSize, "clearAll": false, "jdsource": "facets",
		"isSliderEnable": false, "siteType": "external", "keywords": "",
		"global": true, "selected_fields": map[string]any{}, "locationData": map[string]any{},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+host+"/widgets", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError{code: resp.StatusCode, url: host + "/widgets"}
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode phenom %s: %w", host, err)
	}
	return nil
}

func (p phenomPage) jobs(host string) []Job {
	source := p.RefineSearch.Data.Jobs
	jobs := make([]Job, 0, len(source))
	for _, j := range source {
		// The company field sometimes holds a bare brand number; anything that
		// short is noise, and the poller fills the display name instead.
		company := strings.TrimSpace(j.Company)
		if len(company) < 3 {
			company = ""
		}
		jobs = append(jobs, Job{
			ExternalID: j.JobSeqNo,
			Company:    company,
			Title:      strings.TrimSpace(j.Title),
			Location:   j.CityStateCountry,
			Remote:     looksRemote(j.CityStateCountry),
			URL:        "https://" + host + "/global/en/job/" + j.JobSeqNo,
			PostedAt:   parseTime(phenomTimeLayout, j.PostedDate),
		})
	}
	return jobs
}
