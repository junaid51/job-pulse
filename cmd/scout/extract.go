package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Reading a company's careers page for the hiring system behind it.
//
// This is the half that name-guessing cannot do. Etihad's SmartRecruiters
// identifier is "EtihadAirways5" — with a trailing digit — so every sensible
// spelling of the name missed it, while the careers page had it in an href.
// Ninety-eight postings, forty-eight of them in Abu Dhabi. Emaar's Oracle
// tenant is "emhm.fa.em2.oraclecloud.com", which no amount of guessing produces
// either.
//
// Extraction is regex over the page source, because that is what it is: a
// string in an attribute. What the model does is decide which pages to open —
// careers.x.com, x.com/careers, the "view all jobs" link — and judge whether
// what came back is really the employer.

type candidate struct {
	Provider string `json:"provider"`
	Slug     string `json:"slug"`
	Evidence string `json:"evidence"`
}

// Each pattern names a provider and how to build its slug from the match. The
// shapes come from real career sites, including the escaped forms a page emits
// inside JSON (&, &amp;).
var atsPatterns = []struct {
	provider string
	re       *regexp.Regexp
	slug     func([]string) string
}{
	{"smartrecruiters", regexp.MustCompile(`smartrecruiters\.com/([A-Za-z0-9_-]{2,60})`),
		func(m []string) string { return m[1] }},
	{"greenhouse", regexp.MustCompile(`(?:job-)?boards(?:\.eu)?\.greenhouse\.io/([a-z0-9_-]{2,60})`),
		func(m []string) string { return m[1] }},
	{"lever", regexp.MustCompile(`jobs\.(?:eu\.)?lever\.co/([a-z0-9_-]{2,60})`),
		func(m []string) string { return m[1] }},
	{"ashby", regexp.MustCompile(`jobs\.ashbyhq\.com/([a-z0-9.\-_]{2,60})`),
		func(m []string) string { return m[1] }},
	{"workable", regexp.MustCompile(`apply\.workable\.com/([a-z0-9-]{2,60})`),
		func(m []string) string { return m[1] }},
	{"teamtailor", regexp.MustCompile(`([a-z0-9-]{2,60})\.teamtailor\.com`),
		func(m []string) string { return m[1] }},
	{"recruitee", regexp.MustCompile(`([a-z0-9-]{2,60})\.recruitee\.com`),
		func(m []string) string { return m[1] }},
	// Workday is a host plus the name of a site on it, which is exactly the
	// shape companies.txt already carries.
	{"workday", regexp.MustCompile(`([a-z0-9-]+\.wd\d+\.myworkdayjobs\.com)/(?:en-US/)?([A-Za-z0-9_-]{2,60})`),
		func(m []string) string { return m[1] + "/" + m[2] }},
	{"workday", regexp.MustCompile(`([a-z0-9-]+\.wd\d+\.myworkdayjobs\.com)/wday/cxs/[a-z0-9-]+/([A-Za-z0-9_-]{2,60})`),
		func(m []string) string { return m[1] + "/" + m[2] }},
}

// oracleHost matches the tenant; the site number is not in the page, so both of
// the two forms in use are offered and the probe decides.
var oracleHost = regexp.MustCompile(`([a-z0-9-]+\.fa\.[a-z0-9]+\.oraclecloud\.com)`)

// phenomFingerprint is how a Phenom-hosted careers site gives itself away. The
// slug is the careers host itself, which is why the page's own URL is needed.
var phenomFingerprint = regexp.MustCompile(`phenompeople\.com|ph-static|phApp\.ddo`)

func atsCandidates(page, pageURL string) []candidate {
	// A page emits its own URLs escaped when they sit inside JSON or a script,
	// and Etihad's did exactly that: "https:\/\/jobs.smartrecruiters.com\/…".
	// Unescape first, or the pattern misses the very thing it is looking for.
	page = strings.NewReplacer(
		`\/`, "/", `\u002F`, "/", `\u002f`, "/", "&amp;", "&", `\u0026`, "&",
	).Replace(page)

	var found []candidate
	seen := map[string]bool{}
	add := func(provider, slug, evidence string) {
		slug = strings.Trim(slug, "-._")
		if slug == "" || seen[provider+":"+slug] {
			return
		}
		// Boilerplate that appears on pages of companies not using the system.
		switch strings.ToLower(slug) {
		case "www", "jobs", "careers", "api", "app", "help", "about", "support",
			"blog", "docs", "static", "assets", "partners", "developers":
			return
		}
		seen[provider+":"+slug] = true
		found = append(found, candidate{provider, slug, evidence})
	}

	for _, p := range atsPatterns {
		for _, m := range p.re.FindAllStringSubmatch(page, 12) {
			add(p.provider, p.slug(m), m[0])
		}
	}
	for _, m := range oracleHost.FindAllStringSubmatch(page, 6) {
		// Two site numbers cover every Oracle careers site seen so far, and a
		// probe settles which one this tenant uses.
		add("oracle", m[1]+"|CX_1001", m[0])
		add("oracle", m[1]+"|CX_1", m[0])
	}
	if phenomFingerprint.MatchString(page) {
		if u, err := url.Parse(pageURL); err == nil && u.Host != "" {
			add("phenom", u.Host, "a Phenom-hosted careers site")
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].Provider < found[j].Provider })
	return found
}

// careerLinks are the links on a page worth opening next, for when the ATS is
// one click further in — "View all jobs", "Search openings".
func careerLinks(page, pageURL string) []string {
	href := regexp.MustCompile(`(?i)href=["']([^"'#]{3,200})["']`)
	interesting := regexp.MustCompile(`(?i)(career|job|vacanc|opening|recruit|hiring|apply)`)
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range href.FindAllStringSubmatch(page, 400) {
		if !interesting.MatchString(m[1]) {
			continue
		}
		link, err := base.Parse(m[1])
		if err != nil || (link.Scheme != "http" && link.Scheme != "https") {
			continue
		}
		clean := link.String()
		if seen[clean] || len(out) >= 8 {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

// readCareersPage fetches one page and reports what hiring system it points at.
// It filters by employer for the same reason findHiringSystem does: the model
// followed this tool to careers.clickhouse.com, took Langfuse's board off it,
// and offered it as ClickHouse's.
func readCareersPage(ctx context.Context, rawURL, employer string) map[string]any {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") {
		return map[string]any{"error": "give me an http or https address"}
	}
	if isPrivateHost(target.Hostname()) {
		return map[string]any{"error": "that address is not on the public internet"}
	}
	page, status, err := fetchPage(ctx, target.String())
	if err != nil {
		return map[string]any{"fetched": false, "why": err.Error()}
	}
	if status != http.StatusOK {
		return map[string]any{"fetched": false, "http_status": status}
	}
	mine, others := []candidate{}, 0
	for _, c := range atsCandidates(page, target.String()) {
		if employer == "" || resemblesEmployer(c.Provider, c.Slug, employer) {
			mine = append(mine, c)
			continue
		}
		others++
	}
	return map[string]any{
		"fetched":                        true,
		"hiring_systems":                 mine,
		"other_companies_boards_ignored": others,
		"pages_worth_next":               careerLinks(page, target.String()),
	}
}

func isPrivateHost(host string) bool {
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
	}
	return false
}

// fetchPage is a plain GET with a browser's user agent, because a careers page
// served to something that looks like a crawler is often a different page.
func fetchPage(ctx context.Context, target string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "+
			"(KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	// A megabyte is more than enough to find an href, and less than enough to
	// matter if a page is hostile.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}

// careersURLs guesses where a company keeps its careers page. The model would
// not: asked for Etihad Airways it tried careers.etihadairways.com, which does
// not resolve, and gave up — while careers.etihad.com was sitting there with
// the SmartRecruiters id in an href. Guessing hostnames is a sweep, and sweeps
// belong in code.
//
// The .ae and .sa forms are here because this hunt is in the Gulf and plenty of
// employers there never registered the .com.
func careersURLs(employer string) []string {
	forms := slugVariants(employer)
	if len(forms) > 2 {
		forms = forms[:2] // the joined and hyphenated spellings
	}
	if first := strings.Fields(strings.ToLower(employer)); len(first) > 1 {
		forms = append(forms, strings.Trim(first[0], ",.|&"))
	}

	// Shapes outer, spellings inner: the first thing tried is
	// careers.<name>.com for every spelling of the name, which is where a
	// careers page most often is. Ordered the other way round, the cap below
	// spent all twelve slots on "etihadairways" and never reached "etihad".
	shapes := []string{
		"https://careers.%s.com",
		"https://www.%s.com/careers",
		"https://www.%s.com/en/careers",
		"https://careers.%s.ae",
		"https://www.%s.ae/careers",
		"https://%s.com/careers",
	}
	var out []string
	seen := map[string]bool{}
	for _, shape := range shapes {
		for _, name := range forms {
			if len(name) < 3 {
				continue
			}
			candidate := fmt.Sprintf(shape, name)
			if !seen[candidate] {
				seen[candidate] = true
				out = append(out, candidate)
			}
		}
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

// resemblesEmployer reports whether a slug plausibly belongs to this employer.
//
// A careers page mentions other companies' boards — partners, portfolio
// companies, integrations. Chasing ClickHouse's dead Greenhouse board, the
// scout read careers.clickhouse.com, picked "ashby:langfuse" off it, and
// proposed Langfuse's board as ClickHouse's new one; only the guard stopped it,
// and had Langfuse carried Gulf postings the gate would have accepted the wrong
// company's board outright.
//
// Applied to name-addressed systems only. A Workday or Oracle tenant host bears
// no relation to the company name — Emaar's is emhm.fa.em2.oraclecloud.com —
// and finding one on the company's own careers page is evidence enough.
func resemblesEmployer(provider, slug, employer string) bool {
	switch provider {
	case "oracle", "workday", "phenom":
		return true
	}
	bare := func(in string) string {
		var out []rune
		for _, r := range strings.ToLower(in) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				out = append(out, r)
			}
		}
		return string(out)
	}
	candidate := bare(slug)
	if candidate == "" {
		return false
	}
	for _, form := range append(slugVariants(employer), employer) {
		name := bare(form)
		if len(name) < 3 {
			continue
		}
		if strings.Contains(candidate, name) || strings.Contains(name, candidate) {
			return true
		}
	}
	return false
}

// findHiringSystem opens every plausible careers page for an employer at once
// and reports every hiring system any of them names.
func findHiringSystem(ctx context.Context, employer string) map[string]any {
	urls := careersURLs(employer)
	type reading struct {
		URL        string      `json:"page"`
		Candidates []candidate `json:"hiring_systems"`
	}
	var (
		mu       sync.Mutex
		readings []reading
		opened   []string
		ignored  int
		wg       sync.WaitGroup
	)
	gate := make(chan struct{}, 6)
	for _, target := range urls {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			page, status, err := fetchPage(ctx, target)
			if err != nil || status != http.StatusOK {
				return
			}
			mine, others := []candidate{}, 0
			for _, c := range atsCandidates(page, target) {
				if resemblesEmployer(c.Provider, c.Slug, employer) {
					mine = append(mine, c)
					continue
				}
				others++
			}
			mu.Lock()
			opened = append(opened, target)
			ignored += others
			if len(mine) > 0 {
				readings = append(readings, reading{target, mine})
			}
			mu.Unlock()
		}(target)
	}
	wg.Wait()
	sort.Strings(opened)
	return map[string]any{
		"pages_tried":  urls,
		"pages_opened": opened,
		"found":        readings,
		// Said out loud rather than dropped in silence: these were other
		// companies' boards mentioned on the page, not this employer's.
		"other_companies_boards_ignored": ignored,
	}
}
