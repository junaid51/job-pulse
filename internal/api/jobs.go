package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/junaid51/job-pulse/internal/match"

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
	Salary    string     `json:"salary"`
	PostedAt  *time.Time `json:"posted_at"`
	MatchedAt time.Time  `json:"matched_at"`
	SeenAt    *time.Time `json:"seen_at"`
	Applied   bool       `json:"applied"`
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
		if sort != "posted" && sort != "matched" && sort != "applied" {
			writeError(w, http.StatusBadRequest, "sort must be posted, matched or applied")
			return
		}
		search := strings.TrimSpace(r.URL.Query().Get("q"))
		locations := locationPatterns(r)

		// With a search term and no profile, the query runs over everything
		// stored — a search bar that silently hides jobs because they missed
		// your profile keywords answers a different question than the one asked.
		if r.URL.Query().Get("profile_id") == "" && (search != "" || locations != nil) {
			searchAllJobs(w, r, pool, search, locations, limit)
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

		// Every ordering paginates on a (timestamp, id) pair, because timestamps
		// tie constantly — a backfill stamps hundreds of matches with one now(),
		// and boards without posting dates coalesce to the epoch. Filtering on
		// the timestamp alone would drop every tied row. Each sort names the
		// column its cursor compares.
		cursorExpr := `coalesce(j.posted_at, 'epoch'::timestamptz)`
		extraWhere := ``
		switch sort {
		case "matched":
			cursorExpr = `m.created_at`
		case "applied":
			cursorExpr = `m.applied_at`
			extraWhere = ` and m.applied_at is not null`
		}
		rows, err := pool.Query(r.Context(), `
			select j.id, j.provider, j.company, j.title, j.location, j.remote, j.url,
			       j.salary, j.posted_at, m.created_at, m.seen_at,
			       m.applied_at is not null, `+cursorExpr+` as cursor_at
			from matches m
			join jobs j on j.id = m.job_id
			join profiles p on p.id = m.profile_id and p.owner = $6
			where m.profile_id = $1
			  and m.hidden_at is null`+extraWhere+`
			  and ($2::timestamptz is null or (`+cursorExpr+`, j.id) < ($2, $3::bigint))
			  and ($5 = '' or j.title ilike '%' || $5 || '%'
			       or j.company ilike '%' || $5 || '%'
			       or j.location ilike '%' || $5 || '%')
			  and ($7::text[] is null or j.location ilike any($7))
			order by `+cursorExpr+` desc, j.id desc
			limit $4`, profileID, at, atID, limit, search, deviceID(r), locations)
		if err != nil {
			serverError(w, "listing jobs", err)
			return
		}
		defer rows.Close()

		jobs := []job{}
		var lastCursorAt time.Time
		for rows.Next() {
			var j job
			if err := rows.Scan(&j.ID, &j.Provider, &j.Company, &j.Title, &j.Location,
				&j.Remote, &j.URL, &j.Salary, &j.PostedAt, &j.MatchedAt, &j.SeenAt,
				&j.Applied, &lastCursorAt); err != nil {
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
		// Only offer a cursor when there is plausibly another page.
		if len(jobs) == limit {
			body["next_cursor"] = fmt.Sprintf("%s,%d",
				lastCursorAt.Format(time.RFC3339Nano), jobs[len(jobs)-1].ID)
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

// locationPatterns turns location= params into ilike patterns, expanded through
// the same alias dictionary profile matching uses — "@uae" finds "Dubai" here
// for exactly the reason a profile's "uae" does.
func locationPatterns(r *http.Request) []string {
	var patterns []string
	for _, raw := range r.URL.Query()["location"] {
		for _, term := range match.LocationTerms(raw) {
			patterns = append(patterns, "%"+term+"%")
		}
	}
	return patterns
}

// searchAllJobs is the profile-free search: every live job, newest first.
func searchAllJobs(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, search string, locations []string, limit int) {
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
	rows, err := pool.Query(r.Context(), `
		select id, provider, company, title, location, remote, url,
		       salary, posted_at, first_seen_at, null::timestamptz, false,
		       coalesce(posted_at, 'epoch'::timestamptz) as cursor_at
		from jobs
		where ($1 = '' or title ilike '%' || $1 || '%'
		   or company ilike '%' || $1 || '%'
		   or location ilike '%' || $1 || '%')
		  and ($5::text[] is null or location ilike any($5))
		  and ($3::timestamptz is null
		       or (coalesce(posted_at, 'epoch'::timestamptz), id) < ($3, $4::bigint))
		order by coalesce(posted_at, 'epoch'::timestamptz) desc, id desc
		limit $2`, search, limit, at, atID, locations)
	if err != nil {
		serverError(w, "searching jobs", err)
		return
	}
	defer rows.Close()

	jobs := []job{}
	var lastCursorAt time.Time
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.ID, &j.Provider, &j.Company, &j.Title, &j.Location,
			&j.Remote, &j.URL, &j.Salary, &j.PostedAt, &j.MatchedAt, &j.SeenAt,
			&j.Applied, &lastCursorAt); err != nil {
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
	if len(jobs) == limit {
		body["next_cursor"] = fmt.Sprintf("%s,%d",
			lastCursorAt.Format(time.RFC3339Nano), jobs[len(jobs)-1].ID)
	}
	writeJSON(w, http.StatusOK, body)
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
