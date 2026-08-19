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
// TODO(M4): /api/notifications and /api/devices.
func NewRouter(pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, requestLogger)

	r.Get("/healthz", healthz(pool))

	r.Route("/api", func(r chi.Router) {
		r.Get("/jobs", listJobs(pool))

		r.Get("/profiles", listProfiles(pool))
		r.Post("/profiles", createProfile(pool))
		r.Put("/profiles/{id}", updateProfile(pool))
		r.Delete("/profiles/{id}", deleteProfile(pool))

		r.Post("/poll", triggerPoll(pool))
	})

	return r
}
