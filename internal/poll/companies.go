package poll

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/providers"
)

// Company is one board to poll.
type Company struct {
	Provider string
	Slug     string
	Name     string
	// LastPolledAt feeds the per-provider throttle; nil means never polled.
	LastPolledAt *time.Time
	// LastFailed is whether the most recent attempt errored. A metered board
	// waits hours between polls, and a transient failure used to cost the whole
	// interval — twelve hours of blindness for one 502.
	LastFailed bool
}

// displayName is what jobs from this board are labelled with when the provider
// does not report a company name itself.
func (c Company) displayName() string {
	if c.Name != "" {
		return c.Name
	}
	return c.Slug
}

// ParseCompanies reads the "provider slug [display name]" lines of
// companies.txt, ignoring blank lines and # comments.
func ParseCompanies(r io.Reader) ([]Company, error) {
	var companies []Company
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		// A comment after an entry is still a comment. Only a whole-line "#"
		// was being honoured, so every measurement I noted beside a board —
		// "Groww  # 5 of 5 in Bengaluru" — became part of its display name,
		// and for an aggregator, whose board name stands in when a posting does
		// not name its employer, it became the company: the feed listed jobs
		// from an outfit called "Careerjet # 8 of 31".
		if head, _, found := strings.Cut(text, " #"); found {
			text = strings.TrimSpace(head)
		}
		fields := strings.Fields(text)
		if len(fields) < 2 {
			return nil, fmt.Errorf("line %d: want \"provider slug [name]\", got %q", line, text)
		}
		if _, ok := providers.All[fields[0]]; !ok {
			return nil, fmt.Errorf("line %d: unknown provider %q", line, fields[0])
		}
		companies = append(companies, Company{
			Provider: fields[0],
			Slug:     fields[1],
			Name:     strings.Join(fields[2:], " "),
		})
	}
	return companies, scanner.Err()
}

// SyncCompanies makes the companies table match the file: the file is the source
// of truth, so a board removed from it stops being polled.
func SyncCompanies(ctx context.Context, pool *pgxpool.Pool, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	companies, err := ParseCompanies(f)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	provs := make([]string, len(companies))
	slugs := make([]string, len(companies))
	for i, c := range companies {
		provs[i], slugs[i] = c.Provider, c.Slug
		_, err := tx.Exec(ctx, `
			insert into companies (provider, slug, name)
			values ($1, $2, $3)
			on conflict (provider, slug) do update set name = excluded.name`,
			c.Provider, c.Slug, c.Name)
		if err != nil {
			return 0, err
		}
	}

	_, err = tx.Exec(ctx, `
		delete from companies c
		where not exists (
			select 1 from unnest($1::text[], $2::text[]) as f(provider, slug)
			where f.provider = c.provider and f.slug = c.slug
		)`, provs, slugs)
	if err != nil {
		return 0, err
	}

	return len(companies), tx.Commit(ctx)
}

func loadCompanies(ctx context.Context, pool *pgxpool.Pool) ([]Company, error) {
	rows, err := pool.Query(ctx,
		`select provider, slug, name, last_polled_at, coalesce(last_error, '') <> ''
		 from companies order by provider, slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []Company
	for rows.Next() {
		var c Company
		if err := rows.Scan(&c.Provider, &c.Slug, &c.Name, &c.LastPolledAt, &c.LastFailed); err != nil {
			return nil, err
		}
		companies = append(companies, c)
	}
	return companies, rows.Err()
}
