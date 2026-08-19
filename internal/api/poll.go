package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/notify"
	"github.com/junaid51/job-pulse/internal/poll"
)

// triggerPoll starts a cycle and returns immediately. A full cycle can take
// tens of seconds, which is far too long to hold a pull-to-refresh open, so the
// app refetches /api/jobs once this returns.
func triggerPoll(pool *pgxpool.Pool, notifier *notify.Notifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Detached from the request: the cycle must outlive this response.
		ctx := context.WithoutCancel(r.Context())
		go func() {
			if _, err := poll.Cycle(ctx, pool, notifier); err != nil && !errors.Is(err, poll.ErrBusy) {
				slog.Error("manual poll failed", "error", err)
			}
		}()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "polling"})
	}
}
