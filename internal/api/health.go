package api

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// healthz reports whether the process can reach its database. It is what a
// free-tier host's health check and `make health` both call, so it does real
// work instead of returning a constant.
func healthz(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{"status": "ok", "database": "ok"}
		status := http.StatusOK

		if err := pool.Ping(r.Context()); err != nil {
			body["status"] = "degraded"
			body["database"] = "unreachable"
			status = http.StatusServiceUnavailable
			slog.Error("health check failed", "error", err)
		}

		writeJSON(w, status, body)
	}
}
