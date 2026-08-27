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

// startPoll runs a cycle detached from whatever request asked for it.
//
// This is the lesson of a nineteen-hour outage: the cycle used to run inside
// the request, on the request's context. The scheduler driving it gives up
// after thirty seconds and a pass over two hundred boards takes minutes, so
// every run was cancelled halfway — the boards it had reached looked fresh, the
// rest silently rotted, and each aborted call was logged as a success.
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
