package api

import (
	"fmt"
	"net/http"
	"regexp"
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

// jobColumns is the select list both feeds share, in the order scanJob reads
// them. The two queries are built separately and they drifted exactly once — a
// column added to one, its scan destination added to the other — and every
// feed scoped to a single saved search answered "reading jobs failed". They
// are one string now so that cannot happen again. The verbs differ: a profile
// feed knows when it matched and which search caught it, the corpus feed has to
// coalesce both.
func jobColumns(matchedAt, matchedBy string) string {
	return `j.id, j.provider, j.company, j.title, j.location, j.remote, j.url,
	        j.salary, j.posted_at, ` + matchedAt + `, m.seen_at,
	        m.applied_at is not null, m.applied_at, ` + matchedBy

}

// scanJob reads one row of jobColumns.
func scanJob(rows interface{ Scan(...any) error }, j *job, cursorAt *time.Time) error {
	return rows.Scan(&j.ID, &j.Provider, &j.Company, &j.Title, &j.Location,
		&j.Remote, &j.URL, &j.Salary, &j.PostedAt, &j.MatchedAt, &j.SeenAt,
		&j.Applied, &j.AppliedAt, &j.MatchedBy, cursorAt)
}

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
		// Every word of the query has to match something, in any order. See
		// searchSQL: as one substring, "engineer dubai" could never match.
		words, atPlaces, excluded := searchWords(search)
		// keyword= is repeatable and ORed: the keywords of a saved search,
		// asked of the whole corpus instead of its own match list.
		anyOf := r.URL.Query()["keyword"]
		locations := locationPatterns(append(r.URL.Query()["location"], atPlaces...))
		// market=1 hides postings nobody here could take. A company board is
		// chosen as a whole, so it brings its Ohio roles along with its Dubai
		// one; this is how a reader asks to see only the second kind.
		var marketPatterns []string
		if r.URL.Query().Get("market") == "1" {
			marketPatterns = match.ReachablePatterns()
		}
		remote := r.URL.Query().Get("remote") == "1"

		// With a search term and no profile, the query runs over everything
		// stored — a search bar that silently hides jobs because they missed
		// your profile keywords answers a different question than the one asked.
		// mine=1 runs the same query narrowed to jobs some profile of this
		// device caught: every saved search at once, which is what the feed
		// opens on.
		mine := r.URL.Query().Get("mine") == "1"
		// An exclusions-only query ("-civil") is a real question — everything
		// except that — so it routes like any other search.
		if r.URL.Query().Get("profile_id") == "" &&
			(mine || len(words) > 0 || len(excluded) > 0 || len(anyOf) > 0 || locations != nil) {
			searchAllJobs(w, r, pool, words, excluded, anyOf, locations, marketPatterns,
				sort, limit, remote, mine)
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
		var b binder
		owner, pid := b.add(deviceID(r)), b.add(profileID)
		cursorAt, cursorID := b.add(at), b.add(atID)
		// A single search's feed already knows which search caught these, so
		// matched_by is empty — cast, because a bare '' comes back as an
		// untyped unknown that will not scan into a string.
		query := `
			select ` + jobColumns("m.created_at", "''::text") + `, ` + cursorExpr + ` as cursor_at
			from matches m
			join jobs j on j.id = m.job_id
			join profiles p on p.id = m.profile_id and p.owner = ` + owner + `
			where m.profile_id = ` + pid + `
			  and m.hidden_at is null` + extraWhere + `
			  and (` + cursorAt + `::timestamptz is null
			       or (` + cursorExpr + `, j.id) < (` + cursorAt + `, ` + cursorID + `::bigint))
			  and (` + b.add(locations) + `::text[] is null or j.location ~* any(` + b.add(locations) + `))
			  and (not ` + b.add(remote) + ` or j.remote)
			  and (` + b.add(marketPatterns) + `::text[] is null or j.location = ''
			       or j.location ~* any(` + b.add(marketPatterns) + `))` +
			searchSQL(words, excluded, &b) + `
			order by ` + cursorExpr + ` desc, j.id desc
			limit ` + b.add(limit)
		rows, err := pool.Query(r.Context(), query, b.args()...)
		if err != nil {
			serverError(w, "listing jobs", err)
			return
		}
		defer rows.Close()

		jobs := []job{}
		var lastCursorAt time.Time
		for rows.Next() {
			var j job
			if err := scanJob(rows, &j, &lastCursorAt); err != nil {
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

// locationPatterns turns location= params into word-edge regexes, expanded
// through the same alias dictionary profile matching uses — "@uae" finds
// "Dubai" here for exactly the reason a profile's "uae" does.
//
// Word edges rather than "%india%": that pattern answered a request for India
// with Indianapolis, which is the same class of wrong as answering "gulf" with
// Gulfport. See match.containsWord.
func locationPatterns(raw []string) []string {
	var patterns []string
	for _, raw := range raw {
		for _, term := range match.LocationTerms(raw) {
			patterns = append(patterns, `\y`+regexp.QuoteMeta(term)+`\y`)
		}
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
	words, excluded, anyOf, locations, marketPatterns []string,
	sort string, limit int, remote, mine bool) {
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

	var b binder
	owner := b.add(deviceID(r))
	cursorAt, cursorID := b.add(at), b.add(atID)
	query := `
		select ` + jobColumns("coalesce(m.matched_at, j.first_seen_at)",
		"coalesce(m.names, '')") + `, ` + cursorExpr + ` as cursor_at
		from jobs j
		left join (
			-- Hidden matches are excluded from the labels but still counted, so
			-- that a job the reader dismissed everywhere stays dismissed here
			-- too. Anything one of their feeds still shows, search still finds.
			select mm.job_id,
			       max(mm.created_at) filter (where mm.hidden_at is null) as matched_at,
			       max(mm.seen_at)    filter (where mm.hidden_at is null) as seen_at,
			       max(mm.applied_at) filter (where mm.hidden_at is null) as applied_at,
			       string_agg(distinct p.name, ', ') filter (where mm.hidden_at is null) as names,
			       count(*) filter (where mm.hidden_at is null) as visible
			from matches mm
			join profiles p on p.id = mm.profile_id and p.owner = ` + owner + `
			group by mm.job_id
		) m on m.job_id = j.id
		where (` + b.add(locations) + `::text[] is null or j.location ~* any(` + b.add(locations) + `))
		  and (not ` + b.add(remote) + ` or j.remote)
		  and (` + b.add(marketPatterns) + `::text[] is null or j.location = ''
		       or j.location ~* any(` + b.add(marketPatterns) + `))
		  and (not ` + b.add(mine) + ` or m.job_id is not null)
		  and (m.job_id is null or m.visible > 0)` + extraWhere + `
		  and (` + cursorAt + `::timestamptz is null
		       or (` + cursorExpr + `, j.id) < (` + cursorAt + `, ` + cursorID + `::bigint))` +
		searchSQL(words, excluded, &b) + anySQL(anyOf, &b) + `
		order by ` + cursorExpr + ` desc, j.id desc
		limit ` + b.add(limit)

	rows, err := pool.Query(r.Context(), query, b.args()...)
	if err != nil {
		serverError(w, "searching jobs", err)
		return
	}
	defer rows.Close()

	jobs := []job{}
	var lastCursorAt time.Time
	for rows.Next() {
		var j job
		if err := scanJob(rows, &j, &lastCursorAt); err != nil {
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
