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
	"github.com/junaid51/job-pulse/internal/providers"
)

// concurrency is how many boards are fetched at once. Low on purpose: this is
// someone else's public API and a poll has fifteen minutes to finish.
const concurrency = 4

// ErrBusy is returned when a cycle is already running.
var ErrBusy = errors.New("poll already in progress")

// running makes cycles mutually exclusive, so a manual /api/poll during a
// scheduled cycle does not double-fetch every board.
var running sync.Mutex

// Stats is what one cycle did, for the log line and the /api/poll response.
type Stats struct {
	Companies  int `json:"companies"`
	Failed     int `json:"failed"`
	NewJobs    int `json:"new_jobs"`
	NewMatches int `json:"new_matches"`
}

// Run polls immediately, then every interval until ctx is cancelled.
func Run(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) {
	// Poll at startup so a fresh clone has jobs without waiting a quarter hour.
	logCycle(ctx, pool)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logCycle(ctx, pool)
		}
	}
}

func logCycle(ctx context.Context, pool *pgxpool.Pool) {
	start := time.Now()
	stats, err := Cycle(ctx, pool)
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
		"duration", time.Since(start).Round(time.Millisecond).String(),
	)
}

// Cycle polls every company once. One board failing never stops the others: the
// error is recorded on the row and the next cycle tries again fifteen minutes
// later, which is why polling needs no retry queue.
func Cycle(ctx context.Context, pool *pgxpool.Pool) (Stats, error) {
	if !running.TryLock() {
		return Stats{}, ErrBusy
	}
	defer running.Unlock()

	companies, err := loadCompanies(ctx, pool)
	if err != nil {
		return Stats{}, err
	}
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
			stats.NewMatches += newMatches
		}()
	}
	wg.Wait()

	return stats, ctx.Err()
}

func pollCompany(ctx context.Context, pool *pgxpool.Pool, c Company, profiles []profile) (int, int, error) {
	fetch, ok := providers.All[c.Provider]
	if !ok {
		// Unreachable while companies.txt is validated on load, but a bad row
		// should not panic the cycle.
		return 0, 0, errors.New("unknown provider " + c.Provider)
	}

	jobs, err := fetch(ctx, c.Slug)
	if err != nil {
		slog.Warn("board fetch failed", "provider", c.Provider, "slug", c.Slug, "error", err)
		recordResult(ctx, pool, c, err)
		return 0, 0, err
	}

	newJobs, err := insertJobs(ctx, pool, c, jobs)
	if err != nil {
		recordResult(ctx, pool, c, err)
		return 0, 0, err
	}

	newMatches, err := insertMatches(ctx, pool, newJobs, profiles)
	if err != nil {
		recordResult(ctx, pool, c, err)
		return 0, 0, err
	}

	recordResult(ctx, pool, c, nil)
	return len(newJobs), newMatches, nil
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

	batch := &pgx.Batch{}
	for _, j := range jobs {
		company := j.Company
		if company == "" {
			company = c.displayName()
		}
		// A missing date must be NULL, not the zero time.
		var posted any
		if !j.PostedAt.IsZero() {
			posted = j.PostedAt
		}
		batch.Queue(`
			insert into jobs (provider, external_id, company, title, location, remote, url, posted_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8)
			on conflict (provider, external_id) do nothing
			returning id`,
			c.Provider, j.ExternalID, company, j.Title, j.Location, j.Remote, j.URL, posted)
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

// insertMatches records which profiles each new job satisfies.
//
// TODO(M4): the matches created here are what a push notification summarises.
func insertMatches(ctx context.Context, pool *pgxpool.Pool, jobs []storedJob, profiles []profile) (int, error) {
	batch := &pgx.Batch{}
	for _, j := range jobs {
		for _, p := range profiles {
			if match.Matches(p.criteria, j.job) {
				batch.Queue(`
					insert into matches (profile_id, job_id) values ($1, $2)
					on conflict do nothing`, p.id, j.id)
			}
		}
	}
	if batch.Len() == 0 {
		return 0, nil
	}

	results := pool.SendBatch(ctx, batch)
	defer results.Close()

	matched := 0
	for range batch.Len() {
		tag, err := results.Exec()
		if err != nil {
			return 0, err
		}
		matched += int(tag.RowsAffected())
	}
	return matched, nil
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
// against and the criteria to compare.
type profile struct {
	id       int64
	criteria match.Criteria
}

func loadProfiles(ctx context.Context, pool *pgxpool.Pool) ([]profile, error) {
	rows, err := pool.Query(ctx, `select id, keywords, locations, remote_only from profiles order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []profile
	for rows.Next() {
		var p profile
		if err := rows.Scan(&p.id, &p.criteria.Keywords, &p.criteria.Locations, &p.criteria.RemoteOnly); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}
