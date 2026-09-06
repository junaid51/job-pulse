package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// board is one watched source and how it is doing — the answer to "why do I
// see nothing" that this tool's owner kept having to ask a human.
type board struct {
	Provider     string     `json:"provider"`
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Jobs         int        `json:"jobs"`
	LastPolledAt *time.Time `json:"last_polled_at"`
	LastError    *string    `json:"last_error"`
	// Discovered says the scout found this board rather than a person listing
	// it, and Retired that it was dropped for producing nothing. Both are shown
	// because a reader is owed an answer to "where did this come from".
	Discovered bool `json:"discovered"`
	Retired    bool `json:"retired"`
}

// listBoards reports every board's health. The corpus is shared, so this is
// deliberately not device-scoped: it describes the machine, not a hunt.
func listBoards(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `
			select c.provider, c.slug, c.name, c.last_polled_at, c.last_error,
			       count(j.id), c.origin = 'agent', not c.active
			from companies c
			left join jobs j on j.provider = c.provider and j.slug = c.slug
			group by c.provider, c.slug, c.name, c.last_polled_at, c.last_error,
			         c.origin, c.active
			order by c.name, c.slug`)
		if err != nil {
			serverError(w, "listing boards", err)
			return
		}
		defer rows.Close()

		boards := []board{}
		for rows.Next() {
			var b board
			if err := rows.Scan(&b.Provider, &b.Slug, &b.Name, &b.LastPolledAt,
				&b.LastError, &b.Jobs, &b.Discovered, &b.Retired); err != nil {
				serverError(w, "reading boards", err)
				return
			}
			boards = append(boards, b)
		}
		if err := rows.Err(); err != nil {
			serverError(w, "reading boards", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"boards": boards})
	}
}
