// Package api is the HTTP layer: a chi router and the handlers it calls.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/junaid51/job-pulse/internal/notify"
)

// NewRouter wires the routes. The pool is passed in rather than reached for
// from a global, which is the whole of the dependency injection in this project.
func NewRouter(pool *pgxpool.Pool, notifier *notify.Notifier) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, cors, requestLogger)

	r.Get("/healthz", healthz(pool))

	r.Route("/api", func(r chi.Router) {
		r.Get("/jobs", listJobs(pool))

		r.Get("/profiles", listProfiles(pool))
		r.Post("/profiles", createProfile(pool))
		r.Put("/profiles/{id}", updateProfile(pool))
		r.Delete("/profiles/{id}", deleteProfile(pool))

		r.Post("/jobs/{id}/hide", hideJob(pool))
		r.Post("/jobs/{id}/unhide", unhideJob(pool))
		r.Post("/jobs/{id}/applied", toggleApplied(pool))

		r.Post("/notifications/seen", markSeen(pool))

		r.Get("/boards", listBoards(pool))

		r.Post("/devices", registerDevice(pool))
		r.Get("/devices/status", deviceStatus(pool))
		r.Post("/devices/test", testDevice(notifier))
		r.Put("/devices/quiet-hours", setQuietHours(pool))

		r.Post("/poll", triggerPoll(pool, notifier))
	})

	return r
}
