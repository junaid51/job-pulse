package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type job struct {
	ID        int64      `json:"id"`
	Provider  string     `json:"provider"`
	Company   string     `json:"company"`
	Title     string     `json:"title"`
	Location  string     `json:"location"`
	Remote    bool       `json:"remote"`
	URL       string     `json:"url"`
	PostedAt  *time.Time `json:"posted_at"`
	MatchedAt time.Time  `json:"matched_at"`
	SeenAt    *time.Time `json:"seen_at"`
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

// listJobs returns the jobs matched by one profile.
//
// profile_id is required: the Jobs screen always has a profile selected, and
// "every job we ever stored" is not a view anyone wants. sort=posted (the
// default) is freshness — when the job went up; sort=matched is when this
// profile first saw it. q narrows by a case-insensitive substring over title,
// company and location, server-side, so it searches every match rather than
// whatever page the client happens to hold.
func listJobs(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := intParam(r, "limit", defaultLimit, maxLimit)

		sort := r.URL.Query().Get("sort")
		if sort == "" {
			sort = "posted"
		}
		if sort != "posted" && sort != "matched" {
			writeError(w, http.StatusBadRequest, "sort must be posted or matched")
			return
		}
		search := strings.TrimSpace(r.URL.Query().Get("q"))

		// With a search term and no profile, the query runs over everything
		// stored — a search bar that silently hides jobs because they missed
		// your profile keywords answers a different question than the one asked.
		if r.URL.Query().Get("profile_id") == "" && search != "" {
			searchAllJobs(w, r, pool, search, limit)
			return
		}

		profileID, err := strconv.ParseInt(r.URL.Query().Get("profile_id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "profile_id is required unless q is set")
			return
		}

		// Offset pagination repeats rows on a feed that grows at the head, so this
		// is a cursor.
		var at any
		var atID any
		if raw := r.URL.Query().Get("cursor"); raw != "" {
			t, id, err := parseCursor(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			at, atID = t, id
		}

		// The cursor compares (created_at, id) as a pair because a great many
		// matches share a created_at — a profile backfill inserts all of its
		// matches in one statement, so they all carry the same now(). Filtering on
		// the timestamp alone would drop every tied row. The cursor belongs to the
		// matched ordering; posted ordering serves a single page.
		orderBy := `order by j.posted_at desc nulls last, j.id desc`
		if sort == "matched" {
			orderBy = `order by m.created_at desc, j.id desc`
		}
		rows, err := pool.Query(r.Context(), `
			select j.id, j.provider, j.company, j.title, j.location, j.remote, j.url,
			       j.posted_at, m.created_at, m.seen_at
			from matches m
			join jobs j on j.id = m.job_id
			join profiles p on p.id = m.profile_id and p.owner = $6
			where m.profile_id = $1
			  and ($2::timestamptz is null or (m.created_at, j.id) < ($2, $3::bigint))
			  and ($5 = '' or j.title ilike '%' || $5 || '%'
			       or j.company ilike '%' || $5 || '%'
			       or j.location ilike '%' || $5 || '%')
			`+orderBy+`
			limit $4`, profileID, at, atID, limit, search, deviceID(r))
		if err != nil {
			serverError(w, "listing jobs", err)
			return
		}
		defer rows.Close()

		jobs := []job{}
		for rows.Next() {
			var j job
			if err := rows.Scan(&j.ID, &j.Provider, &j.Company, &j.Title, &j.Location,
				&j.Remote, &j.URL, &j.PostedAt, &j.MatchedAt, &j.SeenAt); err != nil {
				serverError(w, "reading jobs", err)
				return
			}
			jobs = append(jobs, j)
		}
		if err := rows.Err(); err != nil {
			serverError(w, "reading jobs", err)
			return
		}

		body := map[string]any{"jobs": jobs}
		// Only offer a cursor when there is plausibly another page, and only for
		// the ordering the cursor encodes.
		if sort == "matched" && len(jobs) == limit {
			body["next_cursor"] = formatCursor(jobs[len(jobs)-1])
		}
		writeJSON(w, http.StatusOK, body)
	}
}

// A cursor is "<RFC3339 timestamp>,<job id>": the position of the last row
// returned. Clients should treat it as opaque and pass back what they were given.
func formatCursor(last job) string {
	return fmt.Sprintf("%s,%d", last.MatchedAt.Format(time.RFC3339Nano), last.ID)
}

func parseCursor(raw string) (time.Time, int64, error) {
	timestamp, id, found := strings.Cut(raw, ",")
	if !found {
		return time.Time{}, 0, fmt.Errorf("cursor must be %q", "timestamp,id")
	}
	at, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("cursor timestamp must be RFC3339: %w", err)
	}
	jobID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("cursor id must be a number: %w", err)
	}
	return at, jobID, nil
}

// searchAllJobs is the profile-free search: every live job, newest first.
func searchAllJobs(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, search string, limit int) {
	rows, err := pool.Query(r.Context(), `
		select id, provider, company, title, location, remote, url,
		       posted_at, first_seen_at, null::timestamptz
		from jobs
		where title ilike '%' || $1 || '%'
		   or company ilike '%' || $1 || '%'
		   or location ilike '%' || $1 || '%'
		order by posted_at desc nulls last, id desc
		limit $2`, search, limit)
	if err != nil {
		serverError(w, "searching jobs", err)
		return
	}
	defer rows.Close()

	jobs := []job{}
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.ID, &j.Provider, &j.Company, &j.Title, &j.Location,
			&j.Remote, &j.URL, &j.PostedAt, &j.MatchedAt, &j.SeenAt); err != nil {
			serverError(w, "reading jobs", err)
			return
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "reading jobs", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func intParam(r *http.Request, name string, def, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || v < 1 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
