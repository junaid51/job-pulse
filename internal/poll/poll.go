// Package poll fetches every configured board on a timer, stores what is new and
// records which search profiles it matched.
package poll

import (
	"context"
	"errors"
	"log/slog"
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

// minPollInterval slows selected providers down below the cycle cadence.
// Aggregator APIs meter requests, and a search index refreshes far slower than
// a company's own board; a cycle skips their boards until the interval passes.
var minPollInterval = map[string]time.Duration{
	"careerjet": 6 * time.Hour,
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

	// Matched jobs are collected across every board and notified once at the end,
	// so a profile that matches something at four companies gets one push.
	matched := map[int64][]providers.Job{}

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
			for _, m := range newMatches {
				matched[m.profileID] = append(matched[m.profileID], m.job)
			}
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

	notifyProfiles(ctx, notifier, profiles, matched)

	return stats, nil
}

// dueNow drops boards whose provider has a minimum poll interval that has not
// passed yet. Everything else is always due.
func dueNow(companies []Company, now time.Time) []Company {
	due := make([]Company, 0, len(companies))
	for _, c := range companies {
		interval, metered := minPollInterval[c.Provider]
		if metered && c.LastPolledAt != nil && now.Sub(*c.LastPolledAt) < interval {
			continue
		}
		due = append(due, c)
	}
	return due
}

// notifyProfiles sends one push per profile that gained something this cycle,
// routed to the device that owns the profile.
func notifyProfiles(ctx context.Context, notifier *notify.Notifier, profiles []profile, matched map[int64][]providers.Job) {
	for _, p := range profiles {
		if jobs := matched[p.id]; len(jobs) > 0 {
			notifier.Notify(ctx, p.owner, p.name, jobs)
		}
	}
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
	jobs = youngEnough(jobs, time.Now())

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
// ingest agree about what belongs in the database.
func youngEnough(jobs []providers.Job, now time.Time) []providers.Job {
	kept := make([]providers.Job, 0, len(jobs))
	for _, j := range jobs {
		if j.PostedAt.IsZero() || now.Sub(j.PostedAt) < maxJobAge {
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
		batch.Queue(`
			insert into jobs (provider, external_id, slug, company, title, location, remote, url, salary, posted_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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
	var lastError any
	if cause != nil {
		lastError = cause.Error()
	}
	_, err := pool.Exec(ctx, `
		update companies set last_polled_at = now(), last_error = $3
		where provider = $1 and slug = $2`, c.Provider, c.Slug, lastError)
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
