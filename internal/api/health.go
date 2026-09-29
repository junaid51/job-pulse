package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/junaid51/job-pulse/internal/notify"
	"github.com/junaid51/job-pulse/internal/poll"

	"github.com/jackc/pgx/v5/pgxpool"
)

// staleAfter is when "the poller has not run" stops being a hiccup and starts
// being the outage that matters: the feed is only as fresh as the last cycle.
const staleAfter = 15 * time.Minute

// healthz reports whether the process can reach its database. It is what a
// free-tier host's health check and `make health` both call, so it does real
// work instead of returning a constant.
func healthz(pool *pgxpool.Pool, notifier *notify.Notifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// push is reported, not judged: a server with no credentials is still
		// polling and still useful, so it stays a 200. But it is the one
		// failure that is invisible from the app until a job is missed.
		body := map[string]any{"status": "ok", "database": "ok", "push": notifier.State()}
		status := http.StatusOK

		if err := pool.Ping(r.Context()); err != nil {
			body["status"] = "degraded"
			body["database"] = "unreachable"
			status = http.StatusServiceUnavailable
			slog.Error("health check failed", "error", err)
		}

		// How fresh the jobs are, which is the only freshness anyone cares
		// about. Deliberately not a 503: the host restarts a service whose
		// health check fails, and restarting is no cure for a poller that
		// nobody is triggering.
		age := poll.Since()
		last := poll.Last()
		body["poller"] = "ok"
		if last.At.IsZero() {
			body["poller"] = "never ran"
			body["poll_age_seconds"] = nil
		} else {
			body["last_poll_at"] = last.At.UTC().Format(time.RFC3339)
			body["poll_age_seconds"] = int(age.Seconds())
			if last.Failure != "" {
				body["poller"] = "failing"
				body["poll_error"] = last.Failure
			} else if age > staleAfter {
				body["poller"] = "stale"
			}
		}

		writeJSON(w, status, body)
	}
}
