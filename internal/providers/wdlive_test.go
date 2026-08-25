package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// A live read of one real board, which is the only way to know the adapter
// agrees with the API. Skipped when the network is unavailable.
func TestWorkdayLiveKBR(t *testing.T) {
	if os.Getenv("JOBPULSE_LIVE") == "" {
		t.Skip("set JOBPULSE_LIVE=1 to read a real board")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	jobs, err := fetchWorkday(ctx,
		"kbr.wd5.myworkdayjobs.com/KBR_Careers?locationHierarchy1=7d7dca02efe3019cb75ad7e7f401cc00,7d7dca02efe3013179cdc7e9f4016410")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("jobs: %d", len(jobs))
	if len(jobs) < 50 {
		t.Errorf("expected the UAE+Saudi facet to return a few hundred, got %d", len(jobs))
	}
	gulf, dated, withID := 0, 0, 0
	for _, j := range jobs {
		l := strings.ToLower(j.Location)
		if strings.Contains(l, "saudi") || strings.Contains(l, "arab") || strings.Contains(l, "riyadh") ||
			strings.Contains(l, "dubai") || strings.Contains(l, "abu dhabi") || j.Location == "" {
			gulf++
		}
		if !j.PostedAt.IsZero() {
			dated++
		}
		if j.ExternalID != "" {
			withID++
		}
		if !strings.HasPrefix(j.URL, "https://kbr.wd5.myworkdayjobs.com/en-US/KBR_Careers/job/") {
			t.Errorf("apply URL looks wrong: %s", j.URL)
			break
		}
	}
	t.Logf("in-market-looking: %d/%d  dated: %d  with an id: %d", gulf, len(jobs), dated, len(jobs))
	if withID != len(jobs) {
		t.Errorf("every posting should carry a requisition id, got %d/%d", withID, len(jobs))
	}
	if gulf*10 < len(jobs)*9 {
		t.Errorf("the facet should have kept this board in-market: %d/%d", gulf, len(jobs))
	}
}
