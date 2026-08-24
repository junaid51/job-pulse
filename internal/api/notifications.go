package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// markSeen marks every arrival as seen. There is one user with one phone, so
// "looked at the feed" means all of it — no ids to plumb through.
func markSeen(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag, err := pool.Exec(r.Context(), `
			update matches m set seen_at = $1
			from profiles p
			where p.id = m.profile_id and p.owner = $2 and m.seen_at is null`,
			time.Now(), deviceID(r))
		if err != nil {
			serverError(w, "marking seen", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"marked": tag.RowsAffected()})
	}
}
