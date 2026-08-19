package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Oracle Recruiting Cloud runs the career sites of Gulf enterprises that the
// startup ATSes never touch — hotel groups, holdings, telcos. Its REST API is
// public by design; the awkward part is the host, which is per-tenant. The slug
// is therefore "host|siteNumber" (esbe.fa.em8.oraclecloud.com|CX_1001), since
// neither can be guessed from the other.
type oracleResponse struct {
	Items []struct {
		TotalJobsCount  int `json:"TotalJobsCount"`
		RequisitionList []struct {
			ID              string `json:"Id"`
			Title           string `json:"Title"`
			PrimaryLocation string `json:"PrimaryLocation"`
			Country         string `json:"PrimaryLocationCountry"`
			PostedDate      string `json:"PostedDate"` // "2026-08-19"
			WorkplaceType   string `json:"WorkplaceType"`
		} `json:"requisitionList"`
	} `json:"items"`
}

const (
	oraclePageSize   = 50
	oracleMaxPages   = 10
	oracleTimeLayout = "2006-01-02"
)

func fetchOracle(ctx context.Context, slug string) ([]Job, error) {
	host, site, ok := strings.Cut(slug, "|")
	if !ok || host == "" || site == "" {
		return nil, fmt.Errorf("oracle slug %q: want host|siteNumber", slug)
	}

	var jobs []Job
	for page := 0; page < oracleMaxPages; page++ {
		// The finder's semicolon and commas are Oracle's own syntax and must not
		// be percent-encoded, so the query is assembled by hand rather than with
		// url.Values.
		endpoint := fmt.Sprintf(
			"https://%s/hcmRestApi/resources/latest/recruitingCEJobRequisitions"+
				"?onlyData=true&expand=requisitionList"+
				"&finder=findReqs;siteNumber=%s,limit=%d,offset=%d,sortBy=POSTING_DATES_DESC",
			host, site, oraclePageSize, page*oraclePageSize)

		var result oracleResponse
		if err := oracleGet(ctx, endpoint, &result); err != nil {
			return nil, err
		}
		if len(result.Items) == 0 {
			return jobs, nil
		}
		jobs = append(jobs, result.jobs(host, site)...)
		if len(jobs) >= result.Items[0].TotalJobsCount || len(result.Items[0].RequisitionList) == 0 {
			return jobs, nil
		}
	}
	return jobs, nil
}

func oracleGet(ctx context.Context, endpoint string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	// A browser User-Agent: some Oracle edges reject the default Go one.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; jobpulse/0.1; +https://github.com/junaid51/job-pulse)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError{code: resp.StatusCode, url: endpoint}
	}
	return decodeJSON(resp, v)
}

func (r oracleResponse) jobs(host, site string) []Job {
	if len(r.Items) == 0 {
		return nil
	}
	source := r.Items[0].RequisitionList
	jobs := make([]Job, 0, len(source))
	for _, j := range source {
		location := j.PrimaryLocation
		if location == "" {
			location = j.Country
		}
		jobs = append(jobs, Job{
			ExternalID: j.ID,
			// Company left empty; the poller fills the display name.
			Title:    strings.TrimSpace(j.Title),
			Location: location,
			Remote:   strings.EqualFold(j.WorkplaceType, "Remote") || looksRemote(location),
			URL: fmt.Sprintf("https://%s/hcmUI/CandidateExperience/en/sites/%s/job/%s",
				host, site, j.ID),
			PostedAt: parseTime(oracleTimeLayout, j.PostedDate),
		})
	}
	return jobs
}
