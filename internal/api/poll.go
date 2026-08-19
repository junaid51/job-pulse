package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/notify"
	"github.com/junaid51/job-pulse/internal/poll"
)

// triggerPoll runs a cycle and returns its stats. It is deliberately
// synchronous: the caller is pull-to-refresh, and "fetch now" means the refetch
// that follows must actually see what the cycle found. A cycle over the
// configured boards takes a few seconds, which is exactly what a refresh
// spinner is for.
func triggerPoll(pool *pgxpool.Pool, notifier *notify.Notifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := poll.Cycle(r.Context(), pool, notifier)
		if errors.Is(err, poll.ErrBusy) {
			// The scheduled poller got there first; its results are just as good.
			writeJSON(w, http.StatusOK, map[string]string{"status": "already polling"})
			return
		}
		if err != nil {
			serverError(w, "polling", err)
			return
		}
		writeJSON(w, http.StatusOK, stats)
	}
}
