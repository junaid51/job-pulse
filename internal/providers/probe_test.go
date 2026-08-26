package providers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// A trimmed copy of the reachable set — match imports this package, so the
// probe cannot import match.
var probeMarket = []string{"united arab", "uae", "dubai", "abu dhabi", "sharjah",
	"saudi", "riyadh", "jeddah", "khobar", "dammam", "qatar", "doha", "kuwait",
	"bahrain", "manama", "oman", "muscat", "egypt", "cairo", "india",
	"bengaluru", "bangalore", "mumbai", "delhi", "gurgaon", "gurugram",
	"hyderabad", "pune", "chennai", "noida", "pakistan", "karachi", "lahore",
	"remote", "worldwide", "anywhere", "global", "emea", "middle east"}

func probeReachable(location string) bool {
	if strings.TrimSpace(location) == "" {
		return true
	}
	location = strings.ToLower(location)
	for _, term := range probeMarket {
		if strings.Contains(location, term) {
			return true
		}
	}
	return false
}

// A discovery probe, not a test: it tries candidate company slugs against the
// providers whose slug is just a company name, and reports which answer with
// jobs this hunt could take. Run with JOBPULSE_PROBE=1.
func TestProbeCandidates(t *testing.T) {
	if os.Getenv("JOBPULSE_PROBE") == "" {
		t.Skip("set JOBPULSE_PROBE=1")
	}
	candidates := strings.Fields(os.Getenv("JOBPULSE_PROBE_SLUGS"))
	providers := map[string]Provider{
		"greenhouse": fetchGreenhouse, "lever": fetchLever, "ashby": fetchAshby,
		"workable": fetchWorkable, "smartrecruiters": fetchSmartRecruiters,
	}
	type hit struct {
		provider, slug string
		total, market  int
		sample         string
	}
	var mu sync.Mutex
	var hits []hit
	gate := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, slug := range candidates {
		for name, fetch := range providers {
			wg.Add(1)
			go func(name, slug string, fetch Provider) {
				defer wg.Done()
				gate <- struct{}{}
				defer func() { <-gate }()
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				defer cancel()
				jobs, err := fetch(ctx, slug)
				if err != nil || len(jobs) == 0 {
					return
				}
				market, sample := 0, ""
				for _, j := range jobs {
					if probeReachable(j.Location) {
						market++
						if sample == "" {
							sample = fmt.Sprintf("%s — %s", j.Title, j.Location)
						}
					}
				}
				mu.Lock()
				hits = append(hits, hit{name, slug, len(jobs), market, sample})
				mu.Unlock()
			}(name, slug, fetch)
		}
	}
	wg.Wait()
	for _, h := range hits {
		if h.market == 0 {
			continue
		}
		t.Logf("%-16s %-22s jobs=%-4d in-market=%-4d  %s", h.provider, h.slug, h.total, h.market, h.sample)
	}
	t.Logf("--- %d boards answered with in-market jobs", len(hits))
}
