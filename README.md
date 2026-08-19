# JobPulse

Watches public company job boards and tells me when a new matching job appears.

One Go binary (REST API + poller), one PostgreSQL database, one Flutter app with
three screens. The design and its deliberate omissions are in
[ARCHITECTURE.md](ARCHITECTURE.md) — read that before adding anything.

## Status

**Milestone 1 — foundations.** The backend starts, applies its schema and serves
`/healthz`; the Flutter app builds and runs. There are no providers, no polling,
no matching, no notifications and no UI yet — those are M2–M4.

## Stack

Go · chi · pgx · golang-migrate · PostgreSQL · Flutter · Riverpod

SQL is written by hand. There is no ORM, no code generation and no repository
layer: handlers and the poller take a `*pgxpool.Pool` and run their own queries.

## Run it locally

```bash
docker compose up -d          # PostgreSQL on :5432
go run ./cmd/jobpulse         # migrates, then serves on :8080
cd app && flutter run         # Android, iOS or web
```

Migrations run automatically on startup, so there is no separate migrate step
and no `migrate` CLI to install.

If port 5432 or 8080 is already taken on your machine:

```bash
POSTGRES_PORT=5434 docker compose up -d
PORT=8091 DATABASE_URL='postgres://jobpulse:jobpulse@localhost:5434/jobpulse?sslmode=disable' go run ./cmd/jobpulse
```

## Verify it works

```bash
curl localhost:8080/healthz          # {"database":"ok","status":"ok"}
go test ./...
make psql                            # then: \dt   → five tables
cd app && flutter analyze && flutter test
```

`/healthz` pings the database, so a 200 means the process is genuinely wired to
Postgres. It returns 503 when the database is unreachable.

## Configuration

Everything has a working default, so a fresh clone needs no setup.

| Variable        | Default                                                              |
| --------------- | -------------------------------------------------------------------- |
| `DATABASE_URL`  | `postgres://jobpulse:jobpulse@localhost:5432/jobpulse?sslmode=disable` |
| `PORT`          | `8080`                                                               |
| `POSTGRES_PORT` | `5432` (host port published by Docker Compose only)                  |

## Layout

```
cmd/jobpulse/      main: config, migrate, HTTP server, graceful shutdown
internal/api/      chi router and handlers
internal/config/   environment variables
internal/db/       pgx pool and migration runner
migrations/        numbered .sql files, embedded into the binary
app/               Flutter client
```

Adding a migration means dropping `0002_thing.up.sql` and `0002_thing.down.sql`
into `migrations/`. Nothing else needs changing.

## Deployment

`go build ./cmd/jobpulse` produces a static binary that needs only a
`DATABASE_URL`. It runs on any host with any PostgreSQL; Compose exists purely
to hand you a local database. Nothing in the code knows about a cloud provider.
