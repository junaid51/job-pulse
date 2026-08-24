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
	AppliedAt *time.Time `json:"applied_at"`
	// Which saved search caught this, filled in only by the mine=1 feed —
	// the one view that spans every profile and so has to say.
	MatchedBy string `json:"matched_by,omitempty"`
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

// listJobs returns a feed of jobs: one profile's matches, every profile's
// matches (mine=1), or the whole corpus (a search term with no profile).
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
		// The search term expands through the same role dictionary profile
		// matching uses, so "frontend" finds React roles in both places.
		titlePatterns := keywordPatterns(search)
		// Company and location are searched too, but only for terms long
		// enough to mean something there: "qa" would otherwise match every
		// job in Qatar and at DigitalQatalyst.
		rawPattern := ""
		if len(search) >= 3 {
			rawPattern = "%" + search + "%"
		}
		locations := locationPatterns(r)
		// market=1 hides postings nobody here could take. A company board is
		// chosen as a whole, so it brings its Ohio roles along with its Dubai
		// one; this is how a reader asks to see only the second kind.
		inMarket := r.URL.Query().Get("market") == "1"
		var marketPatterns []string
		if inMarket {
			marketPatterns = match.ReachablePatterns()
		}

		// With a search term and no profile, the query runs over everything
		// stored — a search bar that silently hides jobs because they missed
		// your profile keywords answers a different question than the one asked.
		// mine=1 runs the same query narrowed to jobs some profile of this
		// device caught: every saved search at once, which is what the feed
		// opens on.
		mine := r.URL.Query().Get("mine") == "1"
		if r.URL.Query().Get("profile_id") == "" && (mine || search != "" || locations != nil) {
			searchAllJobs(w, r, pool, titlePatterns, rawPattern, locations, marketPatterns, sort, limit, mine)
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
			       m.applied_at is not null, m.applied_at, `+cursorExpr+` as cursor_at
			from matches m
			join jobs j on j.id = m.job_id
			join profiles p on p.id = m.profile_id and p.owner = $6
			where m.profile_id = $1
			  and m.hidden_at is null`+extraWhere+`
			  and ($2::timestamptz is null or (`+cursorExpr+`, j.id) < ($2, $3::bigint))
			  and ($5::text[] is null or j.title ilike any($5)
			       or ($9 <> '' and (j.company ilike $9 or j.location ilike $9)))
			  and ($7::text[] is null or j.location ilike any($7))
			  and (not $8 or j.remote)
			  and ($10::text[] is null or j.location = '' or j.location ilike any($10))
			order by `+cursorExpr+` desc, j.id desc
			limit $4`, profileID, at, atID, limit, titlePatterns, deviceID(r), locations,
			r.URL.Query().Get("remote") == "1", rawPattern, marketPatterns)
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
				&j.Applied, &j.AppliedAt, &lastCursorAt); err != nil {
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
			body["next_cursor"] = formatCursor(lastCursorAt, jobs[len(jobs)-1].ID)
		}
		writeJSON(w, http.StatusOK, body)
	}
}

// A cursor is "<RFC3339 timestamp>,<job id>": the position of the last row
// returned, timestamped by whichever column the sort ordered on. Clients should
// treat it as opaque and pass back what they were given.
func formatCursor(at time.Time, id int64) string {
	return fmt.Sprintf("%s,%d", at.Format(time.RFC3339Nano), id)
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

// keywordPatterns turns a search term into ilike patterns, expanded through the
// role dictionary so the search bar and a profile keyed on the same word find
// the same jobs.
func keywordPatterns(search string) []string {
	var patterns []string
	for _, term := range match.KeywordTerms(search) {
		patterns = append(patterns, "%"+term+"%")
	}
	return patterns
}

// searchAllJobs is the profile-free search over every stored job.
//
// It still left-joins this device's matches, because hunt state has to survive
// a search: a job marked applied is marked applied wherever it appears. The
// join is aggregated per job — several profiles can match one posting, and the
// row must not multiply — which also makes sort=matched and sort=applied
// meaningful here: "the ones my searches caught" and "the ones I applied to",
// narrowed by whatever is typed.
func searchAllJobs(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool,
	titlePatterns []string, rawPattern string, locations, marketPatterns []string,
	sort string, limit int, mine bool) {
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

	cursorExpr := `coalesce(j.posted_at, 'epoch'::timestamptz)`
	extraWhere := ``
	switch sort {
	case "matched":
		// Arrival order across the whole corpus. Restricting this to matched
		// rows, as the profile feed does, silently turned the search bar into
		// a view of the reader's own matches — the opposite of what it is for.
		// A job nobody matched still arrived when we first saw it.
		cursorExpr = `coalesce(m.matched_at, j.first_seen_at)`
	case "applied":
		cursorExpr = `m.applied_at`
		extraWhere = ` and m.applied_at is not null`
	}

	rows, err := pool.Query(r.Context(), `
		select j.id, j.provider, j.company, j.title, j.location, j.remote, j.url,
		       j.salary, j.posted_at, coalesce(m.matched_at, j.first_seen_at),
		       m.seen_at, m.applied_at is not null, m.applied_at,
		       coalesce(m.names, ''), `+cursorExpr+` as cursor_at
		from jobs j
		left join (
			select mm.job_id,
			       max(mm.created_at) as matched_at,
			       max(mm.seen_at)    as seen_at,
			       max(mm.applied_at) as applied_at,
			       string_agg(distinct p.name, ', ') as names
			from matches mm
			join profiles p on p.id = mm.profile_id and p.owner = $7
			where mm.hidden_at is null
			group by mm.job_id
		) m on m.job_id = j.id
		where ($1::text[] is null or j.title ilike any($1)
		   or ($6 <> '' and (j.company ilike $6 or j.location ilike $6)))
		  and ($5::text[] is null or j.location ilike any($5))
		  and (not $8 or j.remote)
		  and ($9::text[] is null or j.location = '' or j.location ilike any($9))
		  and (not $10 or m.job_id is not null)`+extraWhere+`
		  and ($3::timestamptz is null
		       or (`+cursorExpr+`, j.id) < ($3, $4::bigint))
		order by `+cursorExpr+` desc, j.id desc
		limit $2`, titlePatterns, limit, at, atID, locations, rawPattern,
		deviceID(r), r.URL.Query().Get("remote") == "1", marketPatterns, mine)
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
			&j.Applied, &j.AppliedAt, &j.MatchedBy, &lastCursorAt); err != nil {
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
		body["next_cursor"] = formatCursor(lastCursorAt, jobs[len(jobs)-1].ID)
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
