package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
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

		writeJSON(w, http.StatusOK, map[string]any{
			"targets":    targets,
			"do_not_try": skip,
			"providers":  discoverable(),
		})
	}
}

// discoverable is the set of providers a board may be discovered on: one
// employer's own board, addressed by a slug you could guess from the employer's
// name. Two exclusions, both learned by watching a scout work:
//
// Aggregators ignore the slug and answer with their whole feed, so probing
// "himalayas:namshi" returned twenty postings that had nothing to do with
// Namshi — and the scout proposed it, because the gate only asked whether
// reachable postings came back. It would have polled the same feed twice under
// a made-up employer.
//
// Workday, Oracle and Phenom are addressed by a tenant URL with facet ids in
// it, which no amount of guessing from a company name will produce. The scout
// burned three turns discovering that "namshi" is not a hostname. Finding those
// tenants is real work and worth doing, but it is not this.
var discoverableProviders = []string{
	"ashby", "greenhouse", "lever", "recruitee", "smartrecruiters",
	"teamtailor", "workable",
}

func discoverable() []string {
	names := make([]string, 0, len(discoverableProviders))
	for _, name := range discoverableProviders {
		if _, known := providers.All[name]; known {
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
				" is not a provider a board can be discovered on: it is either a "+
				"search across many employers, or addressed by a tenant URL rather "+
				"than the employer's name")
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
		if err != nil || len(found) == 0 || reachable == 0 {
			why := "the board answered with no postings this hunt can reach"
			if err != nil {
				why = "the board could not be read: " + err.Error()
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
				"postings": len(found), "reachable": reachable,
			})
			return
		}

		name := in.Employer
		if name == "" {
			name = in.Slug
		}
		if _, err := pool.Exec(r.Context(), `
			insert into companies (provider, slug, name, origin, added_reason)
			values ($1, $2, $3, 'agent', $4)
			on conflict (provider, slug) do nothing`,
			in.Provider, in.Slug, name, strings.TrimSpace(in.Reason)); err != nil {
			serverError(w, "adding the board", err)
			return
		}
		if _, err := pool.Exec(r.Context(), `
			insert into board_candidates (provider, slug, employer, verdict, reason, postings, reachable)
			values ($1, $2, $3, 'added', $4, $5, $6)
			on conflict (provider, slug) do update
			set verdict = 'added', reason = excluded.reason, postings = excluded.postings,
			    reachable = excluded.reachable, decided_at = now()`,
			in.Provider, in.Slug, in.Employer, strings.TrimSpace(in.Reason),
			len(found), reachable); err != nil {
			serverError(w, "recording the addition", err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"status": "watching", "postings": len(found), "reachable": reachable,
			"probation_days": int(poll.ProbationPeriod.Hours() / 24),
		})
	}
}
