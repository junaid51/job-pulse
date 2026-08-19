package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// registerDevice stores an FCM registration token so the poller knows where to
// send. The app calls this on every start: tokens rotate, and re-registering the
// same one is a no-op.
func registerDevice(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Token    string `json:"token"`
			Platform string `json:"platform"`
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
			insert into devices (token, platform, owner) values ($1, $2, $3)
			on conflict (token) do update
			set platform = excluded.platform, owner = excluded.owner`,
			in.Token, in.Platform, deviceID(r))
		if err != nil {
			serverError(w, "registering device", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
