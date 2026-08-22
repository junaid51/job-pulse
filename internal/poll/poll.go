// Package poll fetches every configured board on a timer, stores what is new and
// records which search profiles it matched.
package poll

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/match"
	"github.com/junaid51/job-pulse/internal/notify"
	"github.com/junaid51/job-pulse/internal/providers"
)

// concurrency is how many boards are fetched at once. Low on purpose: this is
// someone else's public API and a poll has fifteen minutes to finish.
const concurrency = 4

// maxJobAge is how old a posting can be and still be worth applying to. It
// rules in two places that must agree: postings older than this never enter
// the database, and stored jobs are deleted once they cross it — if only the
// sweep existed, a still-listed old posting would be re-ingested next cycle
// and re-announced forever.
const maxJobAge = 45 * 24 * time.Hour

// maxAggregatorJobAge is the same rule, shortened, for providers that serve a
// window rather than a board. Absence proves nothing there — a posting closed
// last week looks exactly like one still open — so the only defence against
// dead listings is to trust them for less time.
const maxAggregatorJobAge = 21 * 24 * time.Hour

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

// minPollInterval slows selected providers down below the cycle cadence.
// Aggregator APIs meter requests, and a search index refreshes far slower than
// a company's own board; a cycle skips their boards until the interval passes.
var minPollInterval = map[string]time.Duration{
	"careerjet": 6 * time.Hour,
	// The remote-feed aggregators show only their newest window, so absence
	// proves nothing (this map also gates deleteAbsent) — and a window that
	// deep does not need five-minute polling.
	"remotive":  time.Hour,
	"himalayas": time.Hour,
	"jobicy":    time.Hour,
	// Metered: ~700 calls/month free; four searches at this cadence use ~480.
	"jobven": 6 * time.Hour,
	// Metered in jobs returned, a thousand a month on the free tier, and the
	// arithmetic decides the cadence: three searches hold roughly 220 fresh
	// postings a month between them, and a two-day window fetches each one
	// once per poll, so daily polling costs about 440 credits and twice-daily
	// would cost 880 of the 1,000. A day's delay on the subset of postings
	// only this provider carries is the cheaper mistake — running the tier dry
	// mid-month would cost all of them.
	"jobspipe": 24 * time.Hour,
}

// ErrBusy is returned when a cycle is already running.
var ErrBusy = errors.New("poll already in progress")

// running makes cycles mutually exclusive, so a manual /api/poll during a
// scheduled cycle does not double-fetch every board.
var running sync.Mutex

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

// Cycle polls every company once. One board failing never stops the others: the
// error is recorded on the row and the next cycle tries again fifteen minutes
// later, which is why polling needs no retry queue.
func Cycle(ctx context.Context, pool *pgxpool.Pool, notifier *notify.Notifier) (Stats, error) {
	if !running.TryLock() {
		return Stats{}, ErrBusy
	}
	defer running.Unlock()

	companies, err := loadCompanies(ctx, pool)
	if err != nil {
		return Stats{}, err
	}
	companies = dueNow(companies, time.Now())
	profiles, err := loadProfiles(ctx, pool)
	if err != nil {
		return Stats{}, err
	}

	stats := Stats{Companies: len(companies)}

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
	pruned, err := reconcileMatches(ctx, pool, profiles)
	if err != nil {
		return stats, err
	}
	stats.Pruned = pruned

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
			delete from matches
			where profile_id = $1
			  and applied_at is null
			  and not (job_id = any($2::bigint[]))`, p.id, ids)
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
		interval, metered := minPollInterval[c.Provider]
		if metered && c.LastPolledAt != nil && now.Sub(*c.LastPolledAt) < interval {
			continue
		}
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
		where m.notified_at is null and m.hidden_at is null
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
	for i := range jobs {
		if jobs[i].Company == "" {
			jobs[i].Company = c.displayName()
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
		// The not-exists guard is cross-provider dedup: an aggregator (jobven)
		// crawls the same ATS postings some boards serve directly, and both
		// sides end at the same canonical URL. First provider to see a URL
		// owns it; the age sweep or its board's absence-deletion retires it.
		batch.Queue(`
			insert into jobs (provider, external_id, slug, company, title, location, remote, url, salary, posted_at)
			select $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			where not exists (select 1 from jobs d where d.url = $8 and d.provider <> $1)
			on conflict (provider, external_id) do nothing
			returning id`,
			c.Provider, j.ExternalID, c.Slug, j.Company, j.Title, j.Location, j.Remote, j.URL, j.Salary, posted)
	}

	// Every queued result must be consumed before Close, so errors surface here
	// rather than being swallowed by the deferred Close.
	results := pool.SendBatch(ctx, batch)
	defer results.Close()

	var stored []storedJob
	for i := range jobs {
		var id int64
		err := results.QueryRow().Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // already knew about this one
		}
		if err != nil {
			return nil, err
		}
		stored = append(stored, storedJob{id: id, job: jobs[i]})
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
	// A failed metered board keeps its last_polled_at: stamping it would turn
	// one transient 502 into a full interval of silence (hours, not minutes).
	// dueNow's one-per-provider rotation keeps the retries from bursting.
	query := `update companies set last_polled_at = now(), last_error = $3
		where provider = $1 and slug = $2`
	if _, metered := minPollInterval[c.Provider]; metered && cause != nil {
		query = `update companies set last_error = $3
		where provider = $1 and slug = $2`
	}
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
