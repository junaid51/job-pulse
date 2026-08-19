package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// hideJob removes a job from this device's feeds for good: every match the
// device's profiles have on it gets hidden_at. There is deliberately no unhide
// — hiding means "stop showing me this", and the job leaves the database
// anyway when its board closes it or it ages out.
func hideJob(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "id must be a number")
			return
		}
		tag, err := pool.Exec(r.Context(), `
			update matches m set hidden_at = $1
			from profiles p
			where p.id = m.profile_id and p.owner = $2
			  and m.job_id = $3 and m.hidden_at is null`,
			time.Now(), deviceID(r), jobID)
		if err != nil {
			serverError(w, "hiding job", err)
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "no visible match for that job")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// toggleApplied flips the applied state on this device's matches for a job and
// answers with the new state, so the row can render without a refetch.
func toggleApplied(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "id must be a number")
			return
		}
		var applied *time.Time
		err = pool.QueryRow(r.Context(), `
			update matches m set applied_at =
				case when m.applied_at is null then $1::timestamptz else null end
			from profiles p
			where p.id = m.profile_id and p.owner = $2 and m.job_id = $3
			returning m.applied_at`,
			time.Now(), deviceID(r), jobID).Scan(&applied)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no match for that job")
			return
		}
		if err != nil {
			serverError(w, "toggling applied", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"applied": applied != nil})
	}
}
