package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// hideJob removes a job from this device's feeds for good. The state is keyed
// on (device, job) rather than on a match, because the reader dismissing a row
// is not making a statement about one of their saved searches — and a job found
// through the search bar has no match to write on, which is why this used to
// answer 404 for every search result.
func hideJob(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "id must be a number")
			return
		}
		_, err = pool.Exec(r.Context(), `
			insert into job_state (owner, job_id, hidden_at)
			values ($1, $2, $3)
			on conflict (owner, job_id) do update set hidden_at = excluded.hidden_at
			where job_state.hidden_at is null`,
			deviceID(r), jobID, time.Now())
		if isMissingJob(err) {
			writeError(w, http.StatusNotFound, "no such job")
			return
		}
		if err != nil {
			serverError(w, "hiding job", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// unhideJob is hide's undo, which the toast offers for a few seconds.
func unhideJob(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "id must be a number")
			return
		}
		// Clearing the timestamp is the whole undo. An emptied row is left
		// behind on purpose: deleting it in the same statement is not possible
		// anyway (a data-modifying CTE cannot delete the row it just updated,
		// and does so silently), and one row per undone gesture is nothing.
		if _, err := pool.Exec(r.Context(), `
			update job_state set hidden_at = null
			where owner = $1 and job_id = $2`,
			deviceID(r), jobID); err != nil {
			serverError(w, "unhiding job", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// isMissingJob reports whether an insert failed because the job id does not
// exist — the foreign key is what validates the path parameter.
func isMissingJob(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation
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
			insert into job_state (owner, job_id, applied_at)
			values ($1, $2, $3)
			on conflict (owner, job_id) do update set applied_at =
				case when job_state.applied_at is null then excluded.applied_at end
			returning applied_at`,
			deviceID(r), jobID, time.Now()).Scan(&applied)
		if isMissingJob(err) {
			writeError(w, http.StatusNotFound, "no such job")
			return
		}
		if err != nil {
			serverError(w, "toggling applied", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"applied": applied != nil})
	}
}
