package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Careerjet is not a company board but a market-wide aggregator, which changes
// three things. The "slug" is a saved search, "keywords|location|locale" with
// '+' for spaces (e.g. software+engineer|dubai|en_AE). Postings carry no stable
// id and their URLs are per-request tracking tokens, so identity is a hash of
// the fields that do not change. And the API meters requests: it is IP-locked
// (declared in the publisher dashboard), wants the publisher site as Referer,
// and the poller only visits it every few hours (see poll.minPollInterval).
type careerjetPage struct {
	Type  string `json:"type"` // "JOBS" or "ERROR"
	Error string `json:"error"`
	Hits  int    `json:"hits"`
	Pages int    `json:"pages"`
	Jobs  []struct {
		Title     string `json:"title"`
		Company   string `json:"company"`
		Locations string `json:"locations"`
		Salary    string `json:"salary"`
		URL       string `json:"url"`
		Date      string `json:"date"` // RFC 1123: "Tue, 18 Aug 2026 07:45:48 GMT"
	} `json:"jobs"`
}

// careerjetMaxPages bounds each search to the newest sixty postings (the API
// serves twenty per page regardless of page_size). Sorted by date, that is
// far more than arrives between two polls.
const careerjetMaxPages = 3

func fetchCareerjet(ctx context.Context, slug string) ([]Job, error) {
	key := os.Getenv("CAREERJET_API_KEY")
	site := os.Getenv("CAREERJET_SITE")
	if key == "" || site == "" {
		return nil, errors.New("careerjet needs CAREERJET_API_KEY and CAREERJET_SITE")
	}

	keywords, location, locale, err := parseCareerjetSearch(slug)
	if err != nil {
		return nil, err
	}

	var jobs []Job
	for page := 1; page <= careerjetMaxPages; page++ {
		query := url.Values{
			"keywords":    {keywords},
			"locale_code": {locale},
			"sort":        {"date"},
			"page":        {fmt.Sprint(page)},
			// Both are required by the API. The poller is its own visitor.
			"user_ip":    {"127.0.0.1"},
			"user_agent": {userAgent},
		}
		if location != "" {
			query.Set("location", location)
		}

		var result careerjetPage
		if err := careerjetGet(ctx, key, site, query, &result); err != nil {
			return nil, err
		}
		if result.Type == "ERROR" {
			return nil, errors.New("careerjet: " + result.Error)
		}
		jobs = append(jobs, result.jobs()...)
		if page >= result.Pages || len(result.Jobs) == 0 {
			break
		}
	}
	return jobs, nil
}

// parseCareerjetSearch splits "keywords|location|locale"; location may be empty
// for country-wide searches, locale defaults to the API's own en_GB.
func parseCareerjetSearch(slug string) (keywords, location, locale string, err error) {
	parts := strings.Split(slug, "|")
	if len(parts) < 1 || parts[0] == "" || len(parts) > 3 {
		return "", "", "", fmt.Errorf("careerjet search %q: want keywords|location|locale", slug)
	}
	keywords = strings.ReplaceAll(parts[0], "+", " ")
	if len(parts) > 1 {
		location = strings.ReplaceAll(parts[1], "+", " ")
	}
	locale = "en_GB"
	if len(parts) > 2 && parts[2] != "" {
		locale = parts[2]
	}
	return keywords, location, locale, nil
}

// careerjetGet is getJSON's sibling with the extra headers this API demands.
// Twenty duplicated lines beat threading header options through every provider.
func careerjetGet(ctx context.Context, key, site string, query url.Values, v any) error {
	endpoint := "https://search.api.careerjet.net/v4/query?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(key, "")
	req.Header.Set("Referer", site)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError{code: resp.StatusCode, url: "careerjet query"}
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode careerjet: %w", err)
	}
	return nil
}

func (p careerjetPage) jobs() []Job {
	jobs := make([]Job, 0, len(p.Jobs))
	for _, j := range p.Jobs {
		title := strings.TrimSpace(j.Title)
		jobs = append(jobs, Job{
			ExternalID: careerjetID(title, j.Company, j.Locations),
			Company:    strings.TrimSpace(j.Company),
			Title:      title,
			Salary:     strings.TrimSpace(j.Salary),
			Location:   strings.TrimSpace(j.Locations),
			Remote:     looksRemote(j.Locations),
			URL:        j.URL,
			PostedAt:   parseTime(time.RFC1123, j.Date),
		})
	}
	return jobs
}

// careerjetID builds a stable identity for a posting whose URL changes on
// every request. The posting date is deliberately NOT part of it: aggregators
// bump ads by reposting them with a fresh date, and a bumped ad is the same
// job — including the date turned every bump into a "new" job and a duplicate
// notification. The cost is that N identical openings collapse into one row,
// which for a personal alert tool is the right trade.
func careerjetID(title, company, locations string) string {
	sum := sha256.Sum256([]byte(title + "|" + company + "|" + locations))
	return hex.EncodeToString(sum[:12])
}
