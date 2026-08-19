// Package api is the HTTP layer: a chi router and the handlers it calls.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter wires the routes. The pool is passed in rather than reached for
// from a global, which is the whole of the dependency injection in this project.
//
// TODO(M2): /api/jobs and /api/profiles.
// TODO(M4): /api/notifications and /api/devices.
func NewRouter(pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, requestLogger)

	r.Get("/healthz", healthz(pool))

	return r
}
