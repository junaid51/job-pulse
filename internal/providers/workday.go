package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Workday runs the careers site of most large enterprises — the tier this tool
// could not read at all, and the one that matters most in the Gulf, where the
// big employers are banks, airlines, ports and holding groups rather than
// startups on Greenhouse.
//
// The site is a JavaScript app, but the endpoint behind it is public JSON:
// POST /wday/cxs/{tenant}/{site}/jobs with a limit and an offset. It reports a
// total, so absence means closed and the whole board is reachable.
//
// The slug is the host and the site exactly as they appear in the careers URL —
// "kbr.wd5.myworkdayjobs.com/KBR_Careers" — because those are the two things
// that vary and neither can be derived from a company name. The tenant is the
// first label of the host, which is how Workday derives it too.
//
// A slug may carry facet filters as a query string, which is how a global
// employer becomes a small board:
//
//	kbr.wd5.myworkdayjobs.com/KBR_Careers?locationHierarchy1=<uae>,<ksa>,<ind>
//
// KBR lists 1,675 jobs, 328 of them somewhere this hunt can work. Reading all
// 1,675 twenty at a time to find them is 84 requests a cycle; filtered, it is
// 17 and nothing irrelevant is stored. The ids are per-tenant, so they cannot
// be hardcoded — they come from the board's own facets, which every response
// carries:
//
//	curl -s -X POST .../wday/cxs/{tenant}/{site}/jobs \
//	  -H 'Content-Type: application/json' \
//	  -d '{"appliedFacets":{},"limit":1,"offset":0,"searchText":""}' | jq .facets
type workdayPage struct {
	Total       int `json:"total"`
	JobPostings []struct {
		Title         string   `json:"title"`
		ExternalPath  string   `json:"externalPath"`
		LocationsText string   `json:"locationsText"`
		PostedOn      string   `json:"postedOn"`
		BulletFields  []string `json:"bulletFields"`
	} `json:"jobPostings"`
}

const (
	// Workday rejects anything above twenty with a 400, so paging is the only
	// way through a board and the cap is what stops one employer with four
	// thousand openings from owning the cycle. Forty-five pages covers the
	// largest board here once its facets are applied; a board that needs more
	// wants a narrower filter, not a bigger cap.
	workdayPageSize = 20
	workdayMaxPages = 45
)

func fetchWorkday(ctx context.Context, slug string) ([]Job, error) {
	path, query, _ := strings.Cut(slug, "?")
	host, site, ok := strings.Cut(path, "/")
	if !ok {
		return nil, fmt.Errorf("workday slug %q: want \"host/site\", e.g. \"acme.wd3.myworkdayjobs.com/External\"", slug)
	}
	facets, err := workdayFacets(query)
	if err != nil {
		return nil, fmt.Errorf("workday slug %q: %w", slug, err)
	}
	tenant, _, _ := strings.Cut(host, ".")
	endpoint := "https://" + host + "/wday/cxs/" + tenant + "/" + site + "/jobs"

	var jobs []Job
	// Only the first page reports the total; every page after it says zero, so
	// trusting the per-page value ends the walk after forty postings.
	total := 0
	for page := 0; page < workdayMaxPages; page++ {
		var result workdayPage
		if err := workdaySearch(ctx, endpoint, page*workdayPageSize, facets, &result); err != nil {
			return nil, err
		}
		if page == 0 {
			total = result.Total
		}
		for _, p := range result.JobPostings {
			// One board returned a posting with no path at all, which stored a
			// row whose URL was the board's front page and whose id was empty.
			if strings.TrimSpace(p.ExternalPath) == "" {
				continue
			}
			jobs = append(jobs, Job{
				ExternalID: workdayID(p.BulletFields, p.ExternalPath),
				Title:      strings.TrimSpace(p.Title),
				Location:   workdayLocation(p.LocationsText, p.ExternalPath),
				Remote:     looksRemote(p.LocationsText),
				URL:        "https://" + host + "/en-US/" + site + p.ExternalPath,
				PostedAt:   workdayPostedAt(p.PostedOn, time.Now()),
			})
		}
		if len(result.JobPostings) < workdayPageSize || (total > 0 && len(jobs) >= total) {
			break
		}
	}
	return jobs, nil
}

// workdayFacets turns "locationHierarchy1=a,b&workerSubType=c" into the
// appliedFacets object the endpoint expects.
func workdayFacets(query string) (map[string][]string, error) {
	if query == "" {
		return map[string][]string{}, nil
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	facets := make(map[string][]string, len(values))
	for name, list := range values {
		var ids []string
		for _, value := range list {
			for _, id := range strings.Split(value, ",") {
				if id = strings.TrimSpace(id); id != "" {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) > 0 {
			facets[name] = ids
		}
	}
	return facets, nil
}

func workdaySearch(ctx context.Context, endpoint string, offset int, facets map[string][]string, v any) error {
	err := workdayPost(ctx, endpoint, offset, facets, v)
	if err == nil || !retryable(err) {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}
	return workdayPost(ctx, endpoint, offset, facets, v)
}

func workdayPost(ctx context.Context, endpoint string, offset int, facets map[string][]string, v any) error {
	body, err := json.Marshal(map[string]any{
		"appliedFacets": facets,
		"limit":         workdayPageSize,
		"offset":        offset,
		"searchText":    "",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
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
		return statusError{code: resp.StatusCode, url: endpoint}
	}
	return decodeJSON(resp, v)
}

// severalLocations is what Workday says instead of a place when a posting is
// open in more than one: "3 Locations".
var severalLocations = regexp.MustCompile(`^\d+ Locations?$`)

// workdayLocation is the listed text, except when Workday replaces it with a
// count — and it does that often enough to matter: half of one board's postings
// said "2 Locations" and nothing else. Storing that, or storing nothing, left
// those jobs unplaceable. No location word could match them, and a
// location-scoped search hid them without saying so.
//
// The posting's own URL spells the primary location out, so it comes from
// there: /job/India-Bengaluru/Senior-AI-ML-Software-Engineer_JR2024127. The
// other locations of a multi-location posting are genuinely not in the listing
// response, and asking per job would be one request each; the primary is where
// the work is based and it is what the filters need.
func workdayLocation(text, externalPath string) string {
	text = strings.TrimSpace(text)
	if text != "" && !severalLocations.MatchString(text) {
		return text
	}
	return locationFromPath(externalPath)
}

var (
	// "DhabiTrade" -> "Dhabi Trade"
	wordBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	// "AEAbu" -> "AE Abu"
	acronymBoundary = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	runOfSpaces     = regexp.MustCompile(`\s+`)
)

// locationFromPath reads the location out of /job/{location}/{title}_{req}.
//
// The slug is a lossy URL-ification and cannot be turned back into the exact
// string: spaces became dashes, and so did the separators between components
// ("US, CA, Santa Clara" and "SA - Riyadh" both). Worse, some tenants drop the
// separator entirely and run the parts together — "AEAbu-DhabiTrade-Center".
// Exactness is not the point: what matters is that the words are present and
// separated, so "abu dhabi" and "riyadh" can be found in it.
func locationFromPath(externalPath string) string {
	parts := strings.Split(strings.Trim(externalPath, "/"), "/")
	// Only the three-segment form carries a location. A posting whose path is
	// just /job/{title} has none to recover.
	if len(parts) < 3 || parts[0] != "job" {
		return ""
	}
	place := acronymBoundary.ReplaceAllString(parts[1], "$1 $2")
	place = wordBoundary.ReplaceAllString(place, "$1 $2")
	place = strings.ReplaceAll(place, "-", " ")
	return strings.TrimSpace(runOfSpaces.ReplaceAllString(place, " "))
}

var daysAgo = regexp.MustCompile(`(\d+)\+? Days? Ago`)

// workdayPostedAt reads the only date Workday's listing gives: relative English
// like "Posted Today" or "Posted 30+ Days Ago". It is rounded to the day on
// purpose — "Posted Today" resolved against the clock would rewrite every row
// on every cycle for no gain.
func workdayPostedAt(text string, now time.Time) time.Time {
	today := now.UTC().Truncate(24 * time.Hour)
	switch {
	case strings.Contains(text, "Today"):
		return today
	case strings.Contains(text, "Yesterday"):
		return today.AddDate(0, 0, -1)
	}
	if m := daysAgo.FindStringSubmatch(text); m != nil {
		if days, err := strconv.Atoi(m[1]); err == nil {
			return today.AddDate(0, 0, -days)
		}
	}
	return time.Time{}
}

// workdayID picks the requisition id out of bulletFields, which is stable when
// a posting is retitled. Boards do not agree on the order: KBR and JLL put the
// requisition first, Parsons puts the location there ("SA - Riyadh") and the
// requisition second — and taking the first field regardless collapsed 808
// Parsons postings onto eighteen ids, one per location, because the upsert keys
// on it. A requisition id has no spaces and contains a digit; a place has
// spaces. Anything else falls back to the path, which is unique by itself.
func workdayID(bulletFields []string, path string) string {
	for _, field := range bulletFields {
		field = strings.TrimSpace(field)
		if field == "" || strings.ContainsAny(field, " \t") || !strings.ContainsAny(field, "0123456789") {
			continue
		}
		return field
	}
	return path
}
