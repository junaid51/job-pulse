// Package poll fetches every configured board on a timer, stores what is new and
// records which search profiles it matched.
package poll

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/match"
	"github.com/junaid51/job-pulse/internal/notify"
	"github.com/junaid51/job-pulse/internal/providers"
)

// concurrency is how many boards are fetched at once. Raised from four when
// the watchlist passed a hundred: a cycle has five minutes and these are
// nearly all different hosts, so the politeness argument for a small number
// applies per host rather than in total. Boards of one provider still queue
// behind each other often enough in practice.
const concurrency = 8

// maxJobAge is how old a posting can be and still be worth applying to. Two
// weeks: past that the early applicants have been through a screen already,
// and this app exists to be early. It rules in two places that must agree —
// postings older than this never enter the database, and stored jobs are
// deleted once they cross it. If only the sweep existed, a still-listed old
// posting would be re-ingested next cycle and re-announced forever.
const maxJobAge = 14 * 24 * time.Hour

// maxAggregatorJobAge is the same rule, shortened again, for providers that
// serve a window rather than a board. Absence proves nothing there — a posting
// closed last week looks exactly like one still open — so the only defence
// against dead listings is to trust them for less time. One week costs almost
// nothing: these providers are asked for the last two or three days anyway.
const maxAggregatorJobAge = 7 * 24 * time.Hour

// ageLimit is how long a posting from this provider is worth keeping.
func ageLimit(provider string) time.Duration {
	if _, window := minPollInterval[provider]; window {
		return maxAggregatorJobAge
	}
	return maxJobAge
}

// windowProviders lists the providers whose postings age out early.
func windowProviders() []string {
	names := make([]string, 0, len(minPollInterval))
	for name := range minPollInterval {
		names = append(names, name)
	}
	return names
}

// excludedLocations are places the owner has taken out of their hunt. Enforced
// in the same two places maxJobAge is — the ingest filter and the sweep — and
// for the same reason: if only the sweep knew, a still-listed posting would be
// re-ingested and re-announced every cycle. Substring, case-insensitive, so
// city names are listed alongside the country for the boards that write only
// the city.
var excludedLocations = []string{
	"israel", "tel aviv", "tel-aviv", "jerusalem", "haifa", "herzliya",
	"ramat gan", "petah tikva", "netanya", "ra'anana", "raanana", "rehovot",
	"be'er sheva", "beer sheva", "yokneam", "caesarea",
}

// allowed reports whether a posting's location clears the exclusion list.
func allowed(location string) bool {
	location = strings.ToLower(location)
	for _, term := range excludedLocations {
		if strings.Contains(location, term) {
			return false
		}
	}
	return true
}

// excludedPatterns is the exclusion list as SQL ilike patterns, for the sweep.
func excludedPatterns() []string {
	patterns := make([]string, 0, len(excludedLocations))
	for _, term := range excludedLocations {
		patterns = append(patterns, "%"+term+"%")
	}
	return patterns
}

// remoteFeedProviders publish worldwide remote jobs, but most of those jobs
// carry a country restriction — measured on a live Himalayas page, eighteen of
// twenty were open only to the United States, the UK, Canada, China, Namibia,
// Macao or Luxembourg. A restriction the reader cannot satisfy is not a job
// offer, it is a row to scroll past, so these feeds are filtered on the way in.
//
// Company boards are exempt: those are in companies.txt because someone chose
// that employer, and their locations mean where the office is.
var remoteFeedProviders = map[string]bool{"himalayas": true, "jobicy": true}

// reachable reports whether a posting is open to this hunt. The region list
// lives in match, because the search bar filters on the same definition and two
// copies would drift.
func reachable(location string) bool {
	return match.Reachable(location)
}

// reachablePatterns is the same list as SQL ilike patterns, for the sweep.
func reachablePatterns() []string {
	return match.ReachablePatterns()
}

// minPollInterval slows selected providers down below the cycle cadence.
// Aggregator APIs meter requests, and a search index refreshes far slower than
// a company's own board; a cycle skips their boards until the interval passes.
var minPollInterval = map[string]time.Duration{
	// Careerjet is a market-wide search, not a board: absence proves nothing
	// (this map also gates deleteAbsent) and its quota is per publisher key.
	// Hourly, three pages of twenty per search — the newest sixty, which is far
	// more than arrives in an hour.
	"careerjet": time.Hour,
	// The remote-feed aggregators show only their newest window, so absence
	// proves nothing (this map also gates deleteAbsent) — and a window that
	// deep does not need five-minute polling.
	"himalayas": time.Hour,
	"jobicy":    time.Hour,
	// Metered at ~700 calls/month free, and worth spending down: measured
	// against production, jobven supplies most of the Gulf postings we detect
	// and does it with a five-hour median lag, the slowest thing feeding the
	// market this app is actually for. Dropping its "remote" search — remote
	// work arrives through himalayas, jobicy and a hundred direct boards
	// anyway — pays for the remaining three to run every four hours instead of
	// six: 3 searches x 6 polls x 30 days = 540 of 700.
	"jobven": 4 * time.Hour,
	// Metered in jobs returned, a thousand a month on the free tier. Four
	// searches hold roughly 220 fresh postings between them and a two-day
	// window re-fetches each about four times at this cadence, so twice-daily
	// costs around 880 of the 1,000 — chosen deliberately over the safer daily
	// poll, because a measured median lag of twenty-one hours was the worst
	// number in the system and headroom is worth less than being early. If the
	// meter runs hot, halve the window before halving the cadence.
	"jobspipe": 12 * time.Hour,
}

// Not done, deliberately: filtering company boards down to reachable postings
// at ingest. Two thirds of the corpus is work nobody here can take — Anthropic
// 170 postings for 4, Stripe 103 for 8 — and dropping it would cost more than
// it saves. Those boards are fetched whole either way, because their APIs take
// no country filter, so it saves no requests; the storage is three megabytes;
// the UI hides them behind the Gulf + India default already; and the search bar
// promises "every job from every board", which a filter would quietly break.
// A saved search here asks for the UK, which is out-of-market by this
// definition and would have gone silent.
//
// What was worth doing is removing the boards with no reachable postings at
// all — see companies.txt.

// heavyBoards are polled hourly rather than every cycle. Ashby inlines every
// description and offers no way to ask it not to, so these boards answer with
// megabytes: Snowflake is 4.7 MB, ElevenLabs 3.4, Cohere 2.4. At a five-minute
// cadence that is forty gigabytes a month for one company. An hour behind on a
// big employer is still hours ahead of the aggregator that used to carry them.
var heavyBoards = map[string]time.Duration{
	"ashby:snowflake":  time.Hour,
	"ashby:elevenlabs": time.Hour,
	"ashby:cohere":     time.Hour,
	// ServiceNow, Experian and Intuitive were throttled here for costing six
	// round trips and 1.4 MB each. Country-filtered on 2026-08-27 they are one
	// page per country, so they poll every cycle again — which is the point:
	// their Riyadh and Bengaluru postings are found in five minutes, not sixty.
	// Workable inlines descriptions too: Rentokil is 4.1 MB a fetch and
	// Apt Resources 1.5, for boards whose postings change slowly.
	"workable:rentokil-initial": time.Hour,
	"workable:apt-resources":    time.Hour,
	// Country-filtered but still several pages per country.
	"smartrecruiters:AccorHotel|ae,sa": time.Hour,
}

// retryAfterFailure caps the wait after a failed attempt, for providers whose
// meter counts the jobs they return rather than the calls they answer: a call
// that failed returned nothing, so trying again costs nothing. jobspipe is
// deliberately the only entry — jobven meters calls, so a spent call is spent
// whether it succeeded or not, and careerjet's own interval is an hour anyway.
//
// It was worth writing: two jobspipe searches took a 502 and were then not
// attempted again for five hours, with seven still to go, while the API had
// been healthy the whole time.
var retryAfterFailure = map[string]time.Duration{"jobspipe": time.Hour}

// boardInterval is the minimum gap between polls of one board: the provider's
// own limit, or a longer one for a board too expensive to fetch every cycle.
func boardInterval(c Company) (time.Duration, bool) {
	interval, metered := heavyBoards[c.Provider+":"+c.Slug]
	if !metered {
		interval, metered = minPollInterval[c.Provider]
	}
	if !metered {
		return 0, false
	}
	if retry, ok := retryAfterFailure[c.Provider]; ok && c.LastFailed && retry < interval {
		return retry, true
	}
	return interval, true
}

// reconcileEvery bounds how often matches are re-derived from scratch. The
// pass reads every stored job, which at five-minute polling came to four
// gigabytes a month of the same rows — real money on a metered database and
// seconds added to every cycle. It exists to heal criteria that changed, not
// to find new jobs (ingest does that on every cycle), so an hour is soon
// enough. Resetting on restart is deliberate: a deploy usually is the change.
const reconcileEvery = time.Hour

var lastReconcile time.Time

// ErrBusy is returned when a cycle is already running.
var ErrBusy = errors.New("poll already in progress")

// running makes cycles mutually exclusive, so a manual /api/poll during a
// scheduled cycle does not double-fetch every board.
var running sync.Mutex

// cycleDeadline caps a run so the mutex above can never be held forever by a
// board that stops answering mid-fetch. Two hundred boards at eight at a time
// is a couple of minutes; anything past this is broken, not slow.
const cycleDeadline = 8 * time.Minute

// The outcome of the last completed cycle. A poller that has stopped needs to
// be visible: this one went nineteen hours without a run while the cron driving
// it reported success, because each 30-second timeout aborted the cycle and
// nothing anywhere recorded that it had never finished.
var last struct {
	sync.Mutex
	at       time.Time
	duration time.Duration
	stats    Stats
	failure  string
}

// LastCycle is what the last completed run did. A zero At means no cycle has
// finished in this process yet.
type LastCycle struct {
	At       time.Time
	Duration time.Duration
	Stats    Stats
	Failure  string
}

func Last() LastCycle {
	last.Lock()
	defer last.Unlock()
	return LastCycle{At: last.at, Duration: last.duration, Stats: last.stats, Failure: last.failure}
}

// Since is how long ago a cycle last finished — a very long time if none has.
func Since() time.Duration {
	at := Last().At
	if at.IsZero() {
		return math.MaxInt64
	}
	return time.Since(at)
}

// Stats is what one cycle did, for the log line and the /api/poll response.
type Stats struct {
	Companies  int   `json:"companies"`
	Failed     int   `json:"failed"`
	NewJobs    int   `json:"new_jobs"`
	NewMatches int   `json:"new_matches"`
	Removed    int64 `json:"removed"`
	Pruned     int64 `json:"pruned"`
}

// Run polls immediately, then every interval until ctx is cancelled.
func Run(ctx context.Context, pool *pgxpool.Pool, notifier *notify.Notifier, interval time.Duration) {
	// Poll at startup so a fresh clone has jobs without waiting a quarter hour.
	logCycle(ctx, pool, notifier)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logCycle(ctx, pool, notifier)
		}
	}
}

func logCycle(ctx context.Context, pool *pgxpool.Pool, notifier *notify.Notifier) {
	start := time.Now()
	stats, err := Cycle(ctx, pool, notifier)
	if err != nil {
		if !errors.Is(err, ErrBusy) && !errors.Is(err, context.Canceled) {
			slog.Error("poll cycle failed", "error", err)
		}
		return
	}
	slog.Info("poll cycle",
		"companies", stats.Companies,
		"failed", stats.Failed,
		"new_jobs", stats.NewJobs,
		"new_matches", stats.NewMatches,
		"removed", stats.Removed,
		"pruned", stats.Pruned,
		"duration", time.Since(start).Round(time.Millisecond).String(),
	)
}

// warnUnmatchedOverrides reports throttles that no longer apply to anything.
//
// heavyBoards and minPollInterval are keyed by provider:slug, so renaming a
// board — adding "|ae,sa,in" to a SmartRecruiters slug, say — silently detaches
// its override. That happened to three of them and nothing said so.
func warnUnmatchedOverrides(companies []Company) {
	boards := make(map[string]bool, len(companies))
	providers := make(map[string]bool, len(companies))
	for _, c := range companies {
		boards[c.Provider+":"+c.Slug] = true
		providers[c.Provider] = true
	}
	for key := range heavyBoards {
		if !boards[key] {
			slog.Warn("heavyBoards entry matches no board", "key", key)
		}
	}
	for name := range minPollInterval {
		if !providers[name] {
			slog.Warn("minPollInterval entry matches no board", "provider", name)
		}
	}
}

// Cycle polls every company once. One board failing never stops the others: the
// error is recorded on the row and the next cycle tries again fifteen minutes
// later, which is why polling needs no retry queue.
func Cycle(ctx context.Context, pool *pgxpool.Pool, notifier *notify.Notifier) (stats Stats, err error) {
	if !running.TryLock() {
		return Stats{}, ErrBusy
	}
	defer running.Unlock()

	ctx, cancel := context.WithTimeout(ctx, cycleDeadline)
	defer cancel()

	start := time.Now()
	defer func() {
		last.Lock()
		defer last.Unlock()
		last.at, last.duration, last.stats = time.Now(), time.Since(start), stats
		last.failure = ""
		if err != nil {
			last.failure = err.Error()
		}
	}()

	companies, err := loadCompanies(ctx, pool)
	if err != nil {
		return Stats{}, err
	}
	warnUnmatchedOverrides(companies)
	companies = dueNow(companies, time.Now())
	profiles, err := loadProfiles(ctx, pool)
	if err != nil {
		return Stats{}, err
	}

	stats = Stats{Companies: len(companies)}

	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		gate = make(chan struct{}, concurrency)
	)
	for _, c := range companies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()

			newJobs, newMatches, err := pollCompany(ctx, pool, c, profiles)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				stats.Failed++
				return
			}
			stats.NewJobs += newJobs
			stats.NewMatches += len(newMatches)
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return stats, err
	}

	// The age sweep: past maxJobAge a posting is not worth applying to, so it
	// and its match history go. Jobs that never carried a posting date age from
	// when they were first seen.
	tag, err := pool.Exec(ctx, `
		delete from jobs
		where coalesce(posted_at, first_seen_at) < now() - $1::interval`,
		maxJobAge.String())
	if err != nil {
		return stats, err
	}
	stats.Removed += tag.RowsAffected()

	// The short sweep for window providers, paired with ageLimit at ingest.
	staleAggregated, err := pool.Exec(ctx, `
		delete from jobs
		where provider = any($1)
		  and coalesce(posted_at, first_seen_at) < now() - $2::interval`,
		windowProviders(), maxAggregatorJobAge.String())
	if err != nil {
		return stats, err
	}
	stats.Removed += staleAggregated.RowsAffected()

	// Orphan sweep: a board removed from companies.txt stops being polled, but
	// its jobs used to sit in the corpus until they aged out — up to a
	// fortnight of listings from a source we deliberately stopped trusting.
	// Rows from before the slug column are exempt; they are claimed by their
	// board when re-seen, and age out on their own if never claimed.
	orphans, err := pool.Exec(ctx, `
		delete from jobs j
		where j.slug <> ''
		  and not exists (
			select 1 from companies c
			where c.provider = j.provider and c.slug = j.slug)`)
	if err != nil {
		return stats, err
	}
	stats.Removed += orphans.RowsAffected()

	// The reachability sweep, paired with the remote-feed filter at ingest.
	unreachable, err := pool.Exec(ctx, `
		delete from jobs
		where provider = any($1) and location <> '' and not (location ilike any($2))`,
		[]string{"himalayas", "jobicy"}, reachablePatterns())
	if err != nil {
		return stats, err
	}
	stats.Removed += unreachable.RowsAffected()

	// The exclusion sweep, paired with the ingest filter above.
	excluded, err := pool.Exec(ctx, `delete from jobs where location ilike any($1)`,
		excludedPatterns())
	if err != nil {
		return stats, err
	}
	stats.Removed += excluded.RowsAffected()

	// Matches are derived data, so every cycle re-derives them: editing the
	// alias dictionary is meant to fix what a profile finds, and without this
	// it would only affect jobs arriving afterwards while the stale matches sat
	// in the feed forever.
	if time.Since(lastReconcile) >= reconcileEvery {
		pruned, err := reconcileMatches(ctx, pool, profiles)
		if err != nil {
			return stats, err
		}
		stats.Pruned = pruned
		lastReconcile = time.Now()
	}

	// A notification problem must not fail the cycle: the jobs are stored, and
	// the queue below retries the announcement next time round.
	if err := deliverNotifications(ctx, pool, notifier, profiles); err != nil {
		slog.Error("delivering notifications", "error", err)
	}

	return stats, nil
}

// reconcileMatches brings stored matches back in line with what the profiles
// mean today: matches whose job no longer satisfies its profile are dropped,
// and jobs that now qualify are added. Additions here are silent — a widened
// dictionary should not push a hundred notifications — and applied jobs are
// never dropped, because that record belongs to the user, not the query.
func reconcileMatches(ctx context.Context, pool *pgxpool.Pool, profiles []profile) (int64, error) {
	if len(profiles) == 0 {
		return 0, nil
	}
	rows, err := pool.Query(ctx, `select id, title, location, remote from jobs`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type stored struct {
		id  int64
		job providers.Job
	}
	var all []stored
	for rows.Next() {
		var s stored
		if err := rows.Scan(&s.id, &s.job.Title, &s.job.Location, &s.job.Remote); err != nil {
			return 0, err
		}
		all = append(all, s)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var pruned int64
	for _, p := range profiles {
		ids := []int64{}
		for _, s := range all {
			if match.Matches(p.criteria, s.job) {
				ids = append(ids, s.id)
			}
		}
		tag, err := pool.Exec(ctx, `
			delete from matches m
			where m.profile_id = $1
			  and not (m.job_id = any($2::bigint[]))
			  and not exists (
			    select 1 from job_state s
			    join profiles pp on pp.owner = s.owner
			    where pp.id = m.profile_id and s.job_id = m.job_id
			      and s.applied_at is not null)`, p.id, ids)
		if err != nil {
			return pruned, err
		}
		pruned += tag.RowsAffected()
		if len(ids) == 0 {
			continue
		}
		if _, err := pool.Exec(ctx, `
			insert into matches (profile_id, job_id, notified_at)
			select $1, id, now() from unnest($2::bigint[]) as id
			on conflict do nothing`, p.id, ids); err != nil {
			return pruned, err
		}
	}
	return pruned, nil
}

// dueNow drops boards whose provider has a minimum poll interval that has not
// passed yet. Everything else is always due.
func dueNow(companies []Company, now time.Time) []Company {
	due := make([]Company, 0, len(companies))
	// At most one board per metered provider per cycle: firing a provider's
	// searches back to back trips burst limits (jobven answers 429 to the
	// second call). At the cycle cadence they rotate through within minutes
	// while each still keeps its own interval.
	taken := map[string]bool{}
	for _, c := range companies {
		interval, limited := boardInterval(c)
		if limited && c.LastPolledAt != nil && now.Sub(*c.LastPolledAt) < interval {
			continue
		}
		// One board per cycle applies to metered providers, whose rate limits
		// are per key; a heavy board is only heavy for itself.
		_, metered := minPollInterval[c.Provider]
		if metered {
			if taken[c.Provider] {
				continue
			}
			taken[c.Provider] = true
		}
		due = append(due, c)
	}
	return due
}

// deliverNotifications announces the matches nobody has been told about yet,
// one push per profile, and records what went out.
//
// The queue lives in the database rather than in this cycle's results, which is
// the whole point: a push that fails, or a phone inside its quiet hours, is
// offered again next cycle instead of vanishing with the cycle that found it.
func deliverNotifications(ctx context.Context, pool *pgxpool.Pool, notifier *notify.Notifier, profiles []profile) error {
	// Past a day it is not news any more. Mark it announced rather than
	// buzzing about yesterday after an outage.
	if _, err := pool.Exec(ctx, `
		update matches set notified_at = now()
		where notified_at is null and created_at < now() - interval '24 hours'`); err != nil {
		return err
	}

	rows, err := pool.Query(ctx, `
		select m.profile_id, m.job_id, j.company, j.title
		from matches m
		join jobs j on j.id = m.job_id
		join profiles p on p.id = m.profile_id
		left join job_state s on s.owner = p.owner and s.job_id = m.job_id
		where m.notified_at is null and s.hidden_at is null
		order by m.profile_id, m.job_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	pendingJobs := map[int64][]providers.Job{}
	pendingIDs := map[int64][]int64{}
	for rows.Next() {
		var profileID, jobID int64
		var job providers.Job
		if err := rows.Scan(&profileID, &jobID, &job.Company, &job.Title); err != nil {
			return err
		}
		pendingJobs[profileID] = append(pendingJobs[profileID], job)
		pendingIDs[profileID] = append(pendingIDs[profileID], jobID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	owners := make(map[int64]profile, len(profiles))
	for _, p := range profiles {
		owners[p.id] = p
	}
	for profileID, jobs := range pendingJobs {
		p, known := owners[profileID]
		if !known {
			continue
		}
		if !notifier.Notify(ctx, p.owner, p.name, jobs) {
			continue
		}
		if _, err := pool.Exec(ctx, `
			update matches set notified_at = now()
			where profile_id = $1 and job_id = any($2::bigint[])`,
			profileID, pendingIDs[profileID]); err != nil {
			return err
		}
	}
	return nil
}

func pollCompany(ctx context.Context, pool *pgxpool.Pool, c Company, profiles []profile) (int, []newMatch, error) {
	fetch, ok := providers.All[c.Provider]
	if !ok {
		// Unreachable while companies.txt is validated on load, but a bad row
		// should not panic the cycle.
		return 0, nil, errors.New("unknown provider " + c.Provider)
	}

	jobs, err := fetch(ctx, c.Slug)
	if err != nil {
		slog.Warn("board fetch failed", "provider", c.Provider, "slug", c.Slug, "error", err)
		recordResult(ctx, pool, c, err)
		return 0, nil, err
	}
	jobs = youngEnough(jobs, time.Now(), ageLimit(c.Provider))
	if remoteFeedProviders[c.Provider] {
		open := jobs[:0]
		for _, j := range jobs {
			if reachable(j.Location) {
				open = append(open, j)
			}
		}
		jobs = open
	}

	newJobs, err := insertJobs(ctx, pool, c, jobs)
	if err != nil {
		recordResult(ctx, pool, c, err)
		return 0, nil, err
	}

	// The fetch is this board's complete current list, so anything stored from
	// it that the fetch does not contain has been closed by the company —
	// delete it, match history and all. Metered providers only show a window
	// of their results, so absence there proves nothing; their postings leave
	// through the age sweep instead.
	if _, metered := minPollInterval[c.Provider]; !metered {
		if err := deleteAbsent(ctx, pool, c, jobs); err != nil {
			recordResult(ctx, pool, c, err)
			return 0, nil, err
		}
	}

	newMatches, err := insertMatches(ctx, pool, newJobs, profiles)
	if err != nil {
		recordResult(ctx, pool, c, err)
		return 0, nil, err
	}

	recordResult(ctx, pool, c, nil)
	return len(newJobs), newMatches, nil
}

// youngEnough drops postings already past maxJobAge, so the sweep and the
// ingest agree about what belongs in the database. It applies the location
// exclusions in the same pass, for the same reason.
func youngEnough(jobs []providers.Job, now time.Time, maxAge time.Duration) []providers.Job {
	kept := make([]providers.Job, 0, len(jobs))
	for _, j := range jobs {
		if !allowed(j.Location) {
			continue
		}
		if j.PostedAt.IsZero() || now.Sub(j.PostedAt) < maxAge {
			kept = append(kept, j)
		}
	}
	return kept
}

// deleteAbsent removes this board's stored jobs that its current listing no
// longer contains. Rows from before the slug column existed carry ” and are
// first claimed by their board when re-seen (see insertJobs), so they are
// never deleted by a board that does not own them.
func deleteAbsent(ctx context.Context, pool *pgxpool.Pool, c Company, current []providers.Job) error {
	ids := make([]string, len(current))
	for i, j := range current {
		ids[i] = j.ExternalID
	}
	tag, err := pool.Exec(ctx, `
		delete from jobs
		where provider = $1 and slug = $2 and not (external_id = any($3))`,
		c.Provider, c.Slug, ids)
	if err != nil {
		return err
	}
	if n := tag.RowsAffected(); n > 0 {
		slog.Info("closed jobs removed", "provider", c.Provider, "slug", c.Slug, "count", n)
	}
	return nil
}

// storedJob is a job that was genuinely new in this cycle.
type storedJob struct {
	id  int64
	job providers.Job
}

// aggregator names the providers whose boards are a query across many employers
// rather than one employer's own board.
var aggregator = map[string]bool{
	"careerjet": true, "jobven": true, "jobspipe": true,
	"himalayas": true, "jobicy": true,
}

// insertJobs writes every posting and returns only the ones that did not exist
// before. "on conflict do nothing returning id" is the whole of new-job
// detection: a row comes back only when the insert actually happened.
func insertJobs(ctx context.Context, pool *pgxpool.Pool, c Company, jobs []providers.Job) ([]storedJob, error) {
	if len(jobs) == 0 {
		return nil, nil
	}

	// Some boards (Lever, Ashby) do not name the company, so fill it in on the
	// struct itself — the same value must reach both the insert and, through
	// storedJob, the notification that names the company.
	//
	// Aggregators are exempt: their board is a search, not an employer, so
	// lending it a posting whose employer is withheld — recruiters do that
	// routinely — invents a company that does not exist. Better a row with no
	// company, which the feed simply leaves out, than a confident wrong one.
	if !aggregator[c.Provider] {
		for i := range jobs {
			if jobs[i].Company == "" {
				jobs[i].Company = c.displayName()
			}
		}
	}

	// Rows inserted before the slug column existed carry ''; the first board to
	// re-see them claims them, which is what lets deleteAbsent trust the slug.
	claim := make([]string, len(jobs))
	for i, j := range jobs {
		claim[i] = j.ExternalID
	}
	if _, err := pool.Exec(ctx, `
		update jobs set slug = $2
		where provider = $1 and slug = '' and external_id = any($3)`,
		c.Provider, c.Slug, claim); err != nil {
		return nil, err
	}

	batch := &pgx.Batch{}
	for _, j := range jobs {
		// A missing date must be NULL, not the zero time.
		var posted any
		if !j.PostedAt.IsZero() {
			posted = j.PostedAt
		}
		// Two not-exists guards, both dedup. The first is by URL: an
		// aggregator (jobven) crawls the same ATS postings some boards serve
		// directly, and both sides end at the same canonical link. The second
		// is by identity — same title, company and place — which catches the
		// same posting under two different URLs, 138 stored rows worth. First
		// board to see a job owns it; the age sweep or its board's
		// absence-deletion retires it.
		// A stored posting is refreshed when the board's version differs — a
		// retitle, a move, a salary appearing, or a parser of ours getting
		// better. It used to be "do nothing", so a job kept whatever text it
		// had when first seen: fifty-six Workday rows sat with no location at
		// all after the extraction that caused it was fixed, because nothing
		// ever revisited them.
		//
		// The where clause means an unchanged posting is not written at all, and
		// xmax distinguishes the two outcomes that do return a row: zero for an
		// insert, the transaction id for an update. Only inserts are new jobs.
		// Without that, every corrected row would be announced again.
		batch.Queue(`
			insert into jobs (provider, external_id, slug, company, title, location, remote, url, salary, posted_at)
			select $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			where not exists (select 1 from jobs d where d.url = $8 and d.provider <> $1)
			  and not exists (
				select 1 from jobs d2
				where lower(d2.title) = lower($5)
				  and lower(d2.company) = lower($4)
				  and lower(d2.location) = lower($6)
				  and (d2.provider, d2.external_id) <> ($1, $2))
			on conflict (provider, external_id) do update
			   set slug = excluded.slug, company = excluded.company,
			       title = excluded.title, location = excluded.location,
			       remote = excluded.remote, url = excluded.url,
			       salary = excluded.salary,
			       -- never trade a known date for a board that stopped saying
			       posted_at = coalesce(excluded.posted_at, jobs.posted_at)
			 where (jobs.title, jobs.location, jobs.remote, jobs.url,
			        jobs.salary, jobs.company, jobs.slug)
			    is distinct from
			       (excluded.title, excluded.location, excluded.remote,
			        excluded.url, excluded.salary, excluded.company, excluded.slug)
			returning id, xmax::text::bigint = 0 as inserted`,
			c.Provider, j.ExternalID, c.Slug, j.Company, j.Title, j.Location, j.Remote, j.URL, j.Salary, posted)
	}

	// Every queued result must be consumed before Close, so errors surface here
	// rather than being swallowed by the deferred Close.
	results := pool.SendBatch(ctx, batch)
	defer results.Close()

	var stored []storedJob
	var refreshed int
	for i := range jobs {
		var id int64
		var inserted bool
		err := results.QueryRow().Scan(&id, &inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // knew it already, and nothing about it changed
		}
		if err != nil {
			return nil, err
		}
		if !inserted {
			refreshed++
			continue // knew it already; its text is now the board's
		}
		stored = append(stored, storedJob{id: id, job: jobs[i]})
	}
	if refreshed > 0 {
		slog.Info("postings refreshed", "provider", c.Provider, "slug", c.Slug, "count", refreshed)
	}
	return stored, nil
}

// newMatch is a job that matched a profile for the first time — one line of a
// notification.
type newMatch struct {
	profileID int64
	job       providers.Job
}

// insertMatches records which profiles each new job satisfies and returns the
// rows that were genuinely new, which is what a notification summarises.
func insertMatches(ctx context.Context, pool *pgxpool.Pool, jobs []storedJob, profiles []profile) ([]newMatch, error) {
	batch := &pgx.Batch{}
	var queued []newMatch
	for _, j := range jobs {
		for _, p := range profiles {
			if match.Matches(p.criteria, j.job) {
				batch.Queue(`
					insert into matches (profile_id, job_id) values ($1, $2)
					on conflict do nothing`, p.id, j.id)
				queued = append(queued, newMatch{profileID: p.id, job: j.job})
			}
		}
	}
	if batch.Len() == 0 {
		return nil, nil
	}

	results := pool.SendBatch(ctx, batch)
	defer results.Close()

	var inserted []newMatch
	for i := range queued {
		tag, err := results.Exec()
		if err != nil {
			return nil, err
		}
		// Zero rows means the match already existed, so it is not news.
		if tag.RowsAffected() > 0 {
			inserted = append(inserted, queued[i])
		}
	}
	return inserted, nil
}

// recordResult stores the outcome so a board that quietly stops working is
// visible in the database rather than only in the logs.
func recordResult(ctx context.Context, pool *pgxpool.Pool, c Company, cause error) {
	// Every attempt is stamped, successful or not.
	//
	// This used to spare a failed metered board its timestamp, so a transient
	// 502 would not cost it a whole interval of silence. That traded one board's
	// retry latency for every sibling's turn: dueNow gives a metered provider
	// one board per cycle and takes the first that is due, so a board whose
	// timestamp never advances is first for ever. Six Careerjet searches went
	// live behind an unauthorised IP and exactly one of them was ever tried —
	// the other five were starved by the failure of the first.
	query := `update companies set last_polled_at = now(), last_error = $3
		where provider = $1 and slug = $2`
	var lastError any
	if cause != nil {
		lastError = cause.Error()
	}
	_, err := pool.Exec(ctx, query, c.Provider, c.Slug, lastError)
	if err != nil && ctx.Err() == nil {
		slog.Error("recording poll result", "provider", c.Provider, "slug", c.Slug, "error", err)
	}
}

// profile is a search profile as the poller needs it: an id to write matches
// against, a name for the notification, and the criteria to compare.
type profile struct {
	id       int64
	name     string
	owner    string
	criteria match.Criteria
}

func loadProfiles(ctx context.Context, pool *pgxpool.Pool) ([]profile, error) {
	rows, err := pool.Query(ctx, `select id, name, owner, keywords, locations, remote_only from profiles order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []profile
	for rows.Next() {
		var p profile
		if err := rows.Scan(&p.id, &p.name, &p.owner, &p.criteria.Keywords, &p.criteria.Locations, &p.criteria.RemoteOnly); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}
