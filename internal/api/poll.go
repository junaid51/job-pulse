package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"

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
