package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/match"
	"github.com/junaid51/job-pulse/internal/providers"
)

type profile struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Keywords   []string  `json:"keywords"`
	Locations  []string  `json:"locations"`
	RemoteOnly bool      `json:"remote_only"`
	CreatedAt  time.Time `json:"created_at"`
	Unread     int       `json:"unread"`
}

func (p profile) criteria() match.Criteria {
	return match.Criteria{Keywords: p.Keywords, Locations: p.Locations, RemoteOnly: p.RemoteOnly}
}

func listProfiles(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `
			select p.id, p.name, p.keywords, p.locations, p.remote_only, p.created_at,
			       count(m.job_id) filter (where m.seen_at is null and m.hidden_at is null)
			from profiles p
			left join matches m on m.profile_id = p.id
			where p.owner = $1
			group by p.id order by p.id`, deviceID(r))
		if err != nil {
			serverError(w, "listing profiles", err)
			return
		}
		defer rows.Close()

		profiles := []profile{}
		for rows.Next() {
			var p profile
			if err := rows.Scan(&p.ID, &p.Name, &p.Keywords, &p.Locations, &p.RemoteOnly,
				&p.CreatedAt, &p.Unread); err != nil {
				serverError(w, "reading profiles", err)
				return
			}
			profiles = append(profiles, p)
		}
		if err := rows.Err(); err != nil {
			serverError(w, "reading profiles", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
	}
}

// profileInput is what the app sends. It is separate from profile because id and
// created_at are ours to set, not the client's.
type profileInput struct {
	Name       string   `json:"name"`
	Keywords   []string `json:"keywords"`
	Locations  []string `json:"locations"`
	RemoteOnly bool     `json:"remote_only"`
}

// clean trims the strings and drops the blanks a chip input tends to produce.
func (in *profileInput) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errors.New("name is required")
	}
	in.Keywords = cleanList(in.Keywords)
	in.Locations = cleanList(in.Locations)
	return nil
}

func cleanList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			cleaned = append(cleaned, v)
		}
	}
	return cleaned
}

func createProfile(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in profileInput
		if err := decodeJSON(w, r, &in); err != nil {
			return
		}

		var p profile
		err := pool.QueryRow(r.Context(), `
			insert into profiles (name, keywords, locations, remote_only, owner)
			values ($1, $2, $3, $4, $5)
			returning id, name, keywords, locations, remote_only, created_at`,
			in.Name, in.Keywords, in.Locations, in.RemoteOnly, deviceID(r),
		).Scan(&p.ID, &p.Name, &p.Keywords, &p.Locations, &p.RemoteOnly, &p.CreatedAt)
		if err != nil {
			serverError(w, "creating profile", err)
			return
		}

		matched, err := backfill(r, pool, p)
		if err != nil {
			serverError(w, "backfilling profile", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"profile": p, "matched": matched})
	}
}

func updateProfile(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "id must be a number")
			return
		}
		var in profileInput
		if err := decodeJSON(w, r, &in); err != nil {
			return
		}

		var p profile
		err = pool.QueryRow(r.Context(), `
			update profiles set name = $2, keywords = $3, locations = $4, remote_only = $5
			where id = $1 and owner = $6
			returning id, name, keywords, locations, remote_only, created_at`,
			id, in.Name, in.Keywords, in.Locations, in.RemoteOnly, deviceID(r),
		).Scan(&p.ID, &p.Name, &p.Keywords, &p.Locations, &p.RemoteOnly, &p.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no such profile")
			return
		}
		if err != nil {
			serverError(w, "updating profile", err)
			return
		}

		// Widened criteria should show results immediately. Matches that no longer
		// qualify are left alone: they were real when they were recorded.
		matched, err := backfill(r, pool, p)
		if err != nil {
			serverError(w, "backfilling profile", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profile": p, "matched": matched})
	}
}

func deleteProfile(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "id must be a number")
			return
		}
		// Matches go with it: the foreign key cascades. The owner clause means a
		// device can only delete what it created.
		tag, err := pool.Exec(r.Context(),
			`delete from profiles where id = $1 and owner = $2`, id, deviceID(r))
		if err != nil {
			serverError(w, "deleting profile", err)
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "no such profile")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// backfill makes a profile's matches agree with its criteria: every stored job
// that matches is added, and every match that no longer does is removed, so a
// new profile is not empty and an edited one is not haunted by what it used to
// look for. Jobs marked applied are never removed — that is the user's own
// record of having applied, not a match the profile happens to own.
//
// The matching itself is Go, not SQL, so this reads the jobs and filters them
// here — the same match.Matches the poller uses, which is the point.
func backfill(r *http.Request, pool *pgxpool.Pool, p profile) (int, error) {
	ctx := r.Context()
	rows, err := pool.Query(ctx, `select id, title, location, remote from jobs`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	criteria := p.criteria()
	ids := []int64{}
	for rows.Next() {
		var (
			id  int64
			job providers.Job
		)
		if err := rows.Scan(&id, &job.Title, &job.Location, &job.Remote); err != nil {
			return 0, err
		}
		if match.Matches(criteria, job) {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if _, err := pool.Exec(ctx, `
		delete from matches
		where profile_id = $1
		  and applied_at is null
		  and not (job_id = any($2::bigint[]))`, p.ID, ids); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	tag, err := pool.Exec(ctx, `
		insert into matches (profile_id, job_id)
		select $1, id from unnest($2::bigint[]) as id
		on conflict do nothing`, p.ID, ids)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, in *profileInput) error {
	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return err
	}
	if err := in.clean(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return err
	}
	return nil
}
