// Package keepalive stops a free-tier instance being put to sleep.
//
// The host spins an idle instance down after fifteen minutes and warns that
// waking one takes "50 seconds or more". Nothing outside could reliably do that
// waking: cron-job.org's free plan gives up after 30 seconds — and a request it
// abandons does not even start the instance, so its attempts returned 503 and
// it disabled itself twice — while GitHub's cron fired a */10 schedule every
// one to five hours, leaving a 5.1-hour gap in one night.
//
// Both of those are attempts to restore wakefulness from outside. This is the
// other half of the problem and the tractable one: while the process is alive,
// it asks its own public URL for /healthz on a timer. That request goes out
// through the edge and comes back as inbound traffic, which is what the host
// counts as activity — so the instance stays up, and the poller keeps running
// because any request revives one idle more than five minutes.
//
// It cannot wake a sleeping instance, only keep an awake one awake. The first
// wake still comes from outside: a scheduler, the GitHub workflow, or somebody
// opening the app.
package keepalive

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Interval is well inside the fifteen-minute idle window, and matches the
// cadence a cycle wants: every ping is also what triggers the next poll.
const Interval = 5 * time.Minute

// Run pings url until ctx is cancelled. Failures are logged and ignored: a
// missed ping costs one cycle, and the next one is five minutes away.
func Run(ctx context.Context, url string) {
	client := &http.Client{Timeout: 90 * time.Second}
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ping(ctx, client, url)
		}
	}
}

func ping(ctx context.Context, client *http.Client, url string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Warn("keepalive url is unusable", "url", url, "error", err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("keepalive ping failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		slog.Warn("keepalive ping unhealthy", "status", resp.StatusCode)
	}
}
