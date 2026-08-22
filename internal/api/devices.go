package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/notify"
)

// registerDevice stores an FCM registration token so the poller knows where to
// send. The app calls this on every start: tokens rotate, and re-registering the
// same one is a no-op.
func registerDevice(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Token    string `json:"token"`
			Platform string `json:"platform"`
			Timezone string `json:"timezone"` // IANA name; quiet hours run on it
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		in.Token = strings.TrimSpace(in.Token)
		if in.Token == "" {
			writeError(w, http.StatusBadRequest, "token is required")
			return
		}
		if in.Platform = strings.TrimSpace(in.Platform); in.Platform == "" {
			in.Platform = "unknown"
		}

		_, err := pool.Exec(r.Context(), `
			insert into devices (token, platform, owner, timezone) values ($1, $2, $3, $4)
			on conflict (token) do update
			set platform = excluded.platform, owner = excluded.owner,
			    timezone = excluded.timezone`,
			in.Token, in.Platform, deviceID(r), strings.TrimSpace(in.Timezone))
		if err != nil {
			serverError(w, "registering device", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// deviceStatus answers what the server knows about this browser's push setup.
// The app could only ever report the browser's own permission, which says
// nothing about whether a usable token reached us — so a token that expired
// weeks ago was indistinguishable from a quiet week.
func deviceStatus(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			count    int
			timezone *string
			last     *time.Time
		)
		err := pool.QueryRow(r.Context(), `
			select count(*), max(timezone), max(last_notified_at)
			from devices where owner = $1`, deviceID(r)).Scan(&count, &timezone, &last)
		if err != nil {
			serverError(w, "reading device status", err)
			return
		}
		body := map[string]any{"registered": count > 0, "devices": count}
		if timezone != nil {
			body["timezone"] = *timezone
		}
		if last != nil {
			body["last_notified_at"] = *last
		}
		writeJSON(w, http.StatusOK, body)
	}
}

// testDevice pushes one message to this browser's devices, proving the chain
// end to end without waiting for a job to appear.
func testDevice(notifier *notify.Notifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sent, err := notifier.SendTest(r.Context(), deviceID(r))
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sent": sent})
	}
}
