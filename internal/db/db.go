// Package db owns the Postgres connection pool and the schema migrations.
//
// There is deliberately no repository or store type here. Handlers and the
// poller take a *pgxpool.Pool and write their own SQL, which keeps every query
// visible at the place it is used.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, used by golang-migrate

	"github.com/junaidkhan/job-pulse/migrations"
)

// Open creates the pool and verifies the database is actually reachable, so a
// bad DATABASE_URL fails at startup instead of on the first request.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate applies any outstanding migrations from the embedded SQL files.
//
// It runs on every startup and is a no-op once the schema is current, so there
// is one way to get a correct database in development and in production.
// golang-migrate needs a database/sql handle, which is the only reason pgx's
// stdlib driver is imported; application queries all go through the pool.
func Migrate(url string) error {
	sqlDB, err := sql.Open("pgx/v5", url)
	if err != nil {
		return fmt.Errorf("open for migrate: %w", err)
	}
	defer sqlDB.Close()

	driver, err := migratepgx.WithInstance(sqlDB, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrate source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
