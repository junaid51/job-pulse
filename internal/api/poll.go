package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/junaid51/job-pulse/internal/notify"
	"github.com/junaid51/job-pulse/internal/poll"

	"github.com/jackc/pgx/v5/pgxpool"
)

// reviveAfter is how stale the poller has to be before any inbound request
// starts a cycle. This service only runs while something is talking to it, so
// "someone touched the API" is the most reliable clock available, and it does
// not matter which URL, method or token the ping used — only that one arrived.
//
// Four minutes rather than five, deliberately: the pings arrive five minutes
// apart and a cycle takes half a minute, so each ping lands about four and a
// half minutes after the last cycle finished. Measured against five, every
// other ping would decline to poll and the real cadence would be ten minutes.
const reviveAfter = 4 * time.Minute

// startPoll runs a cycle detached from whatever request asked for it.
//
// This is the whole lesson of one nineteen-hour outage: the cycle used to run
// inside the request, on the request's context. The cron driving it gives up
// after thirty seconds and a pass over two hundred boards takes minutes, so
// every single run was cancelled halfway through — the boards it had reached
// looked fresh, the rest silently rotted, and the cron's own dashboard called
// each aborted call a success until the host started answering 503 and it
// disabled itself.
func startPoll(pool *pgxpool.Pool, notifier *notify.Notifier, trigger string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		start := time.Now()
		stats, err := poll.Cycle(ctx, pool, notifier)
		switch {
		case errors.Is(err, poll.ErrBusy):
			// Another cycle is mid-flight; its results are just as good.
		case err != nil:
			slog.Error("poll cycle failed", "trigger", trigger, "error", err)
		default:
			slog.Info("poll cycle",
				"trigger", trigger,
				"companies", stats.Companies,
				"failed", stats.Failed,
				"new_jobs", stats.NewJobs,
				"new_matches", stats.NewMatches,
				"removed", stats.Removed,
				"pruned", stats.Pruned,
				"duration", time.Since(start).Round(time.Millisecond).String(),
			)
		}
	}()
}

// revivePoller starts a cycle after serving any request, if none has finished
// recently. The response is already on its way out by then, so a caller never
// waits for the boards.
func revivePoller(pool *pgxpool.Pool, notifier *notify.Notifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			if poll.Since() >= reviveAfter {
				startPoll(pool, notifier, "traffic")
			}
		})
	}
}

// triggerPoll answers immediately and polls in the background.
func triggerPoll(pool *pgxpool.Pool, notifier *notify.Notifier) http.HandlerFunc {
	// POLL_TOKEN is not user auth — the API deliberately has none. It stops a
	// stranger who finds a public deployment from making this server hammer the
	// job boards. Unset (local development) means the endpoint stays open.
	token := os.Getenv("POLL_TOKEN")

	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
				writeError(w, http.StatusUnauthorized, "poll requires the token")
				return
			}
		}
		startPoll(pool, notifier, "trigger")
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "polling"})
	}
}
