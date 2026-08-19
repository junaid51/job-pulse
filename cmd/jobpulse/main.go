// Command jobpulse is the whole backend: migrations, HTTP API and (from M2) the
// provider poller, in one process started with `go run ./cmd/jobpulse`.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/junaid51/job-pulse/internal/api"
	"github.com/junaid51/job-pulse/internal/config"
	"github.com/junaid51/job-pulse/internal/db"
	"github.com/junaid51/job-pulse/internal/notify"
	"github.com/junaid51/job-pulse/internal/poll"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// run exists so that every failure path returns an error and the deferred
// cleanup still happens; main only decides the exit code.
func run() error {
	cfg := config.Load()

	// Signals cancel this context, which shuts the server down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	slog.Info("migrations applied")

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// companies.txt is the source of truth for which boards get polled, so it is
	// reloaded on every start.
	count, err := poll.SyncCompanies(ctx, pool, cfg.CompaniesFile)
	if err != nil {
		return err
	}
	slog.Info("companies loaded", "count", count, "file", cfg.CompaniesFile)

	notifier := notify.New(ctx, pool, cfg.FirebaseCredentials)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewRouter(pool, notifier),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	var poller sync.WaitGroup
	if cfg.PollInterval > 0 {
		poller.Add(1)
		go func() {
			defer poller.Done()
			poll.Run(ctx, pool, notifier, cfg.PollInterval)
		}()
		slog.Info("poller started", "interval", cfg.PollInterval.String())
	} else {
		// Scale-to-zero mode: an external scheduler calls POST /api/poll.
		slog.Info("internal poller disabled; poll via POST /api/poll")
	}

	errs := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)

	// The poller shares ctx, so it is already winding down; wait so an in-flight
	// cycle finishes its writes before the pool closes.
	poller.Wait()
	return err
}
