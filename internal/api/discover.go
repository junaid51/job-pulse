package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/match"
	"github.com/junaid51/job-pulse/internal/poll"
	"github.com/junaid51/job-pulse/internal/providers"
)

// Discovery: how a board found while the app is running gets into the corpus.
//
// The scout that proposes boards is a small language model, and a small
// language model will hand you a confident, plausible, entirely invented
// answer — asked about Air Arabia it proposed a Greenhouse board it had itself
// just probed and found empty, reasoning that "Air Arabia is a major company
// and may have Gulf postings". So the decision cannot live in the scout. It
// lives here: the server probes the board itself, with the same provider code
// the poller uses, and a proposal that does not independently answer with
// reachable postings is refused and remembered as refused.
//
// The scout holds no database credentials and gets no vote.

// ailingFor is how long a board must have been failing before it is worth
// investigating. Long enough that a Supabase connect wobble, a 502 from an
// aggregator, or a board being briefly unreachable has resolved itself; short
// enough that a renamed slug is chased while its postings are still fresh.
const ailingFor = 6 * time.Hour

// platformSized is where a "board" stops being an employer's and starts being a
// job platform's. Jobgether appears in aggregator results as a company name, so
// it reached the scout's warm list as an employer; its Lever board carries four
// thousand postings and weighs 39 MB, and polling it every five minutes spent a
// month of the host's bandwidth in four hours. The gate asked whether the
// postings were reachable. It did not ask how many there were.
const platformSized = 1500

// maxDiscoveredBoards caps what discovery may add. Cycle time is the real
// constraint: production polls 202 boards in 26 seconds against a five-minute
// interval, so the ceiling before cycles overlap is around a thousand — and
// overlapping cycles are how the poller died the first time. A hundred leaves
// the headroom untouched while being far more than a weekly scout can find.
const maxDiscoveredBoards = 100

func tokenGuard(w http.ResponseWriter, r *http.Request) bool {
	token := os.Getenv("POLL_TOKEN")
	if token == "" {
		return true // local development, as with /api/poll
	}
	presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
		writeError(w, http.StatusUnauthorized, "this endpoint requires the token")
		return false
	}
	return true
}

// discoveryTargets answers "who is worth looking for" from data the server
// already has: employers whose postings match a saved search but only ever
// arrive through an aggregator. Those jobs are already reaching this reader —
// hours late. A direct board turns a median lag of twenty-one hours into five
// minutes, which is the entire point of the app.
func discoveryTargets(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !tokenGuard(w, r) {
			return
		}
		limit := intParam(r, "limit", 25, 100)
		aggregators := poll.AggregatorProviders()

		rows, err := pool.Query(r.Context(), `
			select j.company, count(distinct m.job_id) as matches
			from matches m
			join jobs j on j.id = m.job_id
			where j.provider = any($1)
			  and lower(j.company) not in (
				select distinct lower(company) from jobs where not (provider = any($1)))
			  and j.company <> ''
			  -- Not staffing agencies. They are the top of this list by volume —
			  -- one of them accounts for seventeen matches on its own, and a
			  -- quarter of all matches come from firms like them — but a direct
			  -- board would only deliver the same reposted listings sooner. The
			  -- point of discovery is the employer behind the posting.
			  and j.company !~* '(staffing|recruit|manpower|placement|executive search|talent solutions|hr solutions|outsourc|\yhire\y|\yhiring\y)'
			group by j.company
			order by matches desc, j.company
			limit $2`, aggregators, limit)
		if err != nil {
			serverError(w, "listing discovery targets", err)
			return
		}
		defer rows.Close()

		type target struct {
			Employer string `json:"employer"`
			Matches  int    `json:"matches"`
		}
		targets := []target{}
		for rows.Next() {
			var t target
			if err := rows.Scan(&t.Employer, &t.Matches); err != nil {
				serverError(w, "reading discovery targets", err)
				return
			}
			targets = append(targets, t)
		}
		if err := rows.Err(); err != nil {
			serverError(w, "reading discovery targets", err)
			return
		}

		// Everything already judged, so a tireless scout stops re-probing the
		// same dead board every week — the job companies.txt did in comments.
		judged, err := pool.Query(r.Context(), `
			select provider, slug, 'watched' from companies
			union all
			select provider, slug, 'refused' from board_candidates
			where verdict = 'refused'`)
		if err != nil {
			serverError(w, "listing judged boards", err)
			return
		}
		defer judged.Close()

		type known struct {
			Provider string `json:"provider"`
			Slug     string `json:"slug"`
			Verdict  string `json:"verdict"`
		}
		skip := []known{}
		for judged.Next() {
			var k known
			if err := judged.Scan(&k.Provider, &k.Slug, &k.Verdict); err != nil {
				serverError(w, "reading judged boards", err)
				return
			}
			skip = append(skip, k)
		}
		if err := judged.Err(); err != nil {
			serverError(w, "reading judged boards", err)
			return
		}

		// Boards that have stopped answering. A dead one is worse than a missing
		// one: it costs no error anybody sees, its postings age out quietly, and
		// the notifications it would have sent never arrive. Transient failures
		// are excluded by time rather than by reading the error text, because
		// the text is the provider's and changes without notice.
		ailingRows, err := pool.Query(r.Context(), `
			select c.provider, c.slug, c.name, coalesce(c.last_error, ''),
			       c.failing_since, count(j.id)
			from companies c
			left join jobs j on j.provider = c.provider and j.slug = c.slug
			where c.active
			  and c.failing_since is not null
			  and c.failing_since < now() - $1::interval
			group by c.provider, c.slug, c.name, c.last_error, c.failing_since
			order by count(j.id) desc, c.failing_since
			limit $2`, ailingFor.String(), limit)
		if err != nil {
			serverError(w, "listing ailing boards", err)
			return
		}
		defer ailingRows.Close()

		type ailing struct {
			Provider     string    `json:"provider"`
			Slug         string    `json:"slug"`
			Employer     string    `json:"employer"`
			Error        string    `json:"error"`
			FailingSince time.Time `json:"failing_since"`
			PostingsHeld int       `json:"postings_still_held"`
		}
		broken := []ailing{}
		for ailingRows.Next() {
			var a ailing
			if err := ailingRows.Scan(&a.Provider, &a.Slug, &a.Employer, &a.Error,
				&a.FailingSince, &a.PostingsHeld); err != nil {
				serverError(w, "reading ailing boards", err)
				return
			}
			broken = append(broken, a)
		}
		if err := ailingRows.Err(); err != nil {
			serverError(w, "reading ailing boards", err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"targets":          targets,
			"ailing":           broken,
			"do_not_try":       skip,
			"providers":        discoverable(),
			"name_addressable": sweepable(),
		})
	}
}

// Two lists, because they answer different questions.
//
// nameAddressable is what a sweep over spellings of a company's name can reach:
// one employer's own board, addressed by something you could guess. Workday,
// Oracle and Phenom are not on it — they are addressed by a tenant host with
// site and facet ids, and a scout burned three turns discovering that "namshi"
// is not a hostname.
//
// discoverable is what may be *proposed*, which is wider, because a careers
// page will hand over a tenant that no guess produces: Etihad's SmartRecruiters
// id is "EtihadAirways5" and Emaar's Oracle tenant is
// "emhm.fa.em2.oraclecloud.com". Aggregators stay out of both — they ignore the
// slug and answer with their whole feed, so probing "himalayas:namshi" returned
// twenty postings that had nothing to do with Namshi.
var nameAddressable = []string{
	"ashby", "greenhouse", "lever", "recruitee", "smartrecruiters",
	"teamtailor", "workable",
}

func sweepable() []string {
	names := make([]string, 0, len(nameAddressable))
	for _, name := range nameAddressable {
		if _, known := providers.All[name]; known {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func discoverable() []string {
	aggregators := map[string]bool{}
	for _, name := range poll.AggregatorProviders() {
		aggregators[name] = true
	}
	names := make([]string, 0, len(providers.All))
	for name := range providers.All {
		if !aggregators[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func isDiscoverable(provider string) bool {
	for _, name := range discoverable() {
		if name == provider {
			return true
		}
	}
	return false
}

// addBoard verifies a proposal and, if it holds up, starts polling it.
func addBoard(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !tokenGuard(w, r) {
			return
		}
		var in struct {
			Provider string `json:"provider"`
			Slug     string `json:"slug"`
			Employer string `json:"employer"`
			Reason   string `json:"reason"`
			// Replaces names a board this one takes over from, as
			// "provider:slug". Set when a company has moved hiring systems.
			Replaces string `json:"replaces"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		in.Provider = strings.TrimSpace(in.Provider)
		in.Slug = strings.TrimSpace(in.Slug)
		in.Employer = strings.TrimSpace(in.Employer)
		fetch, known := providers.All[in.Provider]
		if known && !isDiscoverable(in.Provider) {
			writeError(w, http.StatusBadRequest, in.Provider+
				" is a search across many employers, not one employer's own board")
			return
		}
		if !known || in.Slug == "" {
			writeError(w, http.StatusBadRequest, "provider must be one this app can read, and slug is required")
			return
		}

		var exists bool
		if err := pool.QueryRow(r.Context(),
			`select exists (select 1 from companies where provider = $1 and slug = $2)`,
			in.Provider, in.Slug).Scan(&exists); err != nil {
			serverError(w, "checking the board", err)
			return
		}
		if exists {
			writeJSON(w, http.StatusOK, map[string]any{"status": "already watched"})
			return
		}

		var discovered int
		if err := pool.QueryRow(r.Context(),
			`select count(*) from companies where origin = 'agent'`).Scan(&discovered); err != nil {
			serverError(w, "counting discovered boards", err)
			return
		}
		if discovered >= maxDiscoveredBoards {
			writeError(w, http.StatusConflict, "the discovered-board budget is full")
			return
		}

		// The verification. Not the scout's word for it, and not its numbers.
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		found, err := fetch(ctx, in.Slug)
		reachable := 0
		for _, j := range found {
			if match.Reachable(j.Location) {
				reachable++
			}
		}
		// What the poller would actually keep today, which is not the same
		// number: a board of openings first published months ago is a real board
		// with nothing to contribute until it posts again. Acceptance still
		// turns on reachable postings — the board is genuine, and probation is
		// what decides whether it delivers — but the record has to say which.
		storable := poll.WouldStore(in.Provider, found)
		if err != nil || len(found) == 0 || reachable == 0 || len(found) > platformSized {
			why := "the board answered with no postings this hunt can reach"
			if err != nil {
				why = "the board could not be read: " + err.Error()
			}
			if err == nil && len(found) > platformSized {
				why = fmt.Sprintf("%d postings is a job platform rather than an employer, "+
					"and polling one costs more bandwidth than this deployment has",
					len(found))
			}
			if _, dberr := pool.Exec(r.Context(), `
				insert into board_candidates (provider, slug, employer, verdict, reason, postings, reachable)
				values ($1, $2, $3, 'refused', $4, $5, $6)
				on conflict (provider, slug) do update
				set verdict = 'refused', reason = excluded.reason,
				    postings = excluded.postings, reachable = excluded.reachable,
				    decided_at = now()`,
				in.Provider, in.Slug, in.Employer, why, len(found), reachable); dberr != nil {
				serverError(w, "recording the refusal", dberr)
				return
			}
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"status": "refused", "reason": why,
				"postings": len(found), "reachable": reachable, "storable": storable,
			})
			return
		}

		name := in.Employer
		if name == "" {
			name = in.Slug
		}
		reason := strings.TrimSpace(in.Reason)
		if storable == 0 {
			reason = strings.TrimSpace(reason + " — nothing fresh enough to store today; " +
				"every posting was first published over a fortnight ago, so it has " +
				"until the end of probation to post something new")
		}
		if _, err := pool.Exec(r.Context(), `
			insert into companies (provider, slug, name, origin, added_reason)
			values ($1, $2, $3, 'agent', $4)
			on conflict (provider, slug) do nothing`,
			in.Provider, in.Slug, name, reason); err != nil {
			serverError(w, "adding the board", err)
			return
		}
		if _, err := pool.Exec(r.Context(), `
			insert into board_candidates (provider, slug, employer, verdict, reason, postings, reachable)
			values ($1, $2, $3, 'added', $4, $5, $6)
			on conflict (provider, slug) do update
			set verdict = 'added', reason = excluded.reason, postings = excluded.postings,
			    reachable = excluded.reachable, decided_at = now()`,
			in.Provider, in.Slug, in.Employer, reason,
			len(found), reachable); err != nil {
			serverError(w, "recording the addition", err)
			return
		}

		answer := map[string]any{
			"status": "watching", "postings": len(found), "reachable": reachable,
			"storable":       storable,
			"probation_days": int(poll.ProbationPeriod.Hours() / 24),
		}
		if note := retireReplaced(r, pool, in.Replaces); note != "" {
			answer["replaced"] = note
		}
		writeJSON(w, http.StatusCreated, answer)
	}
}

// retireReplaced stops polling a board this one takes over from — but only a
// board that discovery added and that is genuinely failing. A board listed in
// companies.txt belongs to whoever wrote the file: deactivating it here would
// be overruled at the next boot anyway, and silently, so the answer says what
// to do instead.
func retireReplaced(r *http.Request, pool *pgxpool.Pool, replaces string) string {
	provider, slug, ok := strings.Cut(strings.TrimSpace(replaces), ":")
	if !ok || provider == "" || slug == "" {
		return ""
	}
	var origin string
	var failing *time.Time
	err := pool.QueryRow(r.Context(),
		`select origin, failing_since from companies where provider = $1 and slug = $2`,
		provider, slug).Scan(&origin, &failing)
	if err != nil {
		return ""
	}
	if failing == nil {
		return replaces + " is answering fine, so it stays"
	}
	if origin != "agent" {
		return replaces + " is listed in companies.txt; remove the line there"
	}
	if _, err := pool.Exec(r.Context(), `
		update companies set active = false
		where provider = $1 and slug = $2 and origin = 'agent'`,
		provider, slug); err != nil {
		return ""
	}
	return replaces + " retired: it stopped answering and this board took over"
}
