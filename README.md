# JobPulse

Watches public company job boards and tells me when a new matching job appears.

One Go binary (REST API + poller), one PostgreSQL database, one Flutter app with
three screens. The design and its deliberate omissions are in
[ARCHITECTURE.md](ARCHITECTURE.md) — read that before adding anything.

## Status

**Milestone 3 — usable end to end.** The backend polls six providers, stores what
is new, matches it against search profiles and serves them over REST; the app has
its three screens and an Apply button on every job. A live cycle over the boards
in `companies.txt` takes about three seconds.

The backend half of push notifications (M4) is in: the poller sends one FCM
summary per profile per cycle, or logs what it would have sent when
`GOOGLE_APPLICATION_CREDENTIALS` is unset — so a clone still needs no Google
account. Still to come: the Firebase Messaging wiring inside the app, which
needs a Firebase project. Until then the app shows new matches when you open it
or pull to refresh.

The app is a PWA: one Flutter codebase compiled to the web, installed via Add to
Home Screen, with push notifications working on iPhone that way — no Apple
Developer account needed. The native iOS and Android shells were deliberately
dropped; `flutter create .` regenerates them if ever wanted.

## Stack

Go · chi · pgx · golang-migrate · PostgreSQL · Flutter · Riverpod

SQL is written by hand. There is no ORM, no code generation and no repository
layer: handlers and the poller take a `*pgxpool.Pool` and run their own queries.

## Run it locally

```bash
docker compose up -d          # PostgreSQL on :5432
go run ./cmd/jobpulse         # migrates, polls, serves on :8080
cd app && flutter run -d chrome
```

Migrations run automatically on startup, so there is no separate migrate step
and no `migrate` CLI to install.

On a real phone, `localhost` is the phone, not your computer, so point the app at
your machine's address on the network:

```bash
cd app && flutter run --dart-define=JOBPULSE_API=http://192.168.1.20:8080
```

The current URL is shown under Settings → Backend.

If port 5432 or 8080 is already taken on your machine:

```bash
POSTGRES_PORT=5434 docker compose up -d
PORT=8091 DATABASE_URL='postgres://jobpulse:jobpulse@localhost:5434/jobpulse?sslmode=disable' go run ./cmd/jobpulse
```

## Boards to watch

None of these providers offer search across companies — every public API is
scoped to one company's board — so JobPulse polls the list in
[companies.txt](companies.txt):

```
# provider         slug         display name
greenhouse         stripe       Stripe
lever              spotify      Spotify
```

The slug is whatever identifies the company on that provider, usually the last
part of its careers URL. The file is the source of truth and is re-read on every
start, so removing a line stops that board being polled. Supported providers:
`greenhouse`, `lever`, `ashby`, `smartrecruiters`, `workable`, `recruitee`,
`teamtailor`.

## API

```
GET    /healthz                                    pings the database

GET    /api/profiles
POST   /api/profiles          {name, keywords[], locations[], remote_only}
PUT    /api/profiles/{id}
DELETE /api/profiles/{id}

GET    /api/jobs?profile_id=1&limit=50&cursor=…    matched jobs, newest first

GET    /api/notifications?limit=50&cursor=…        match feed, all profiles
POST   /api/notifications/seen                     mark the feed read

POST   /api/devices                                {token, platform} — FCM registration
POST   /api/poll                                   run a cycle now; returns its stats
```

Creating or editing a profile backfills it against every job already stored, so
it is never mysteriously empty. `/api/jobs` returns `next_cursor` when another
page exists; pass it back as `cursor` and treat it as opaque.

There is no authentication, by design. Bind it to `127.0.0.1` or reach it over a
private tunnel — do not put this on a public IP.

## Verify it works

```bash
curl localhost:8080/healthz                  # {"database":"ok","status":"ok"}
go test ./...
cd app && flutter analyze && flutter test
```

End to end, against real boards:

```bash
curl -X POST localhost:8080/api/profiles -H 'Content-Type: application/json' \
  -d '{"name":"Backend Go","keywords":["go","backend"],"locations":[],"remote_only":true}'

curl 'localhost:8080/api/jobs?profile_id=1&limit=5'
make psql   # then: select provider, count(*) from jobs group by provider;
```

The poll log line is the quickest check that a cycle worked:

```
msg="poll cycle" companies=6 failed=0 new_jobs=1452 new_matches=0 duration=2.7s
```

`new_jobs=0` on the second cycle is the point: it means new-job detection is
working rather than re-alerting on everything.

## Configuration

Everything has a working default, so a fresh clone needs no setup.

| Variable         | Default                                                                | Purpose                        |
| ---------------- | ---------------------------------------------------------------------- | ------------------------------ |
| `DATABASE_URL`   | `postgres://jobpulse:jobpulse@localhost:5432/jobpulse?sslmode=disable` |                                |
| `PORT`           | `8080`                                                                 |                                |
| `POLL_INTERVAL`  | `15m`                                                                  | Go duration; `0` disables the internal ticker |
| `COMPANIES_FILE` | `companies.txt`                                                        |                                |
| `GOOGLE_APPLICATION_CREDENTIALS` | *(unset)*                                              | service account JSON; unset = log instead of push |
| `POSTGRES_PORT`  | `5432`                                                                 | host port published by Compose |

## Layout

```
cmd/jobpulse/       main: config, migrate, poller, HTTP server, graceful shutdown
internal/api/       chi router and handlers
internal/config/    environment variables
internal/db/        pgx pool and migration runner
internal/match/     does a job satisfy a profile
internal/poll/      the poll cycle and companies.txt
internal/providers/ one file per job board
migrations/         numbered .sql files, embedded into the binary
app/lib/            api.dart, models.dart, providers.dart, router.dart, theme.dart
app/lib/screens/    jobs, notifications, settings
```

Adding a migration means dropping `0002_thing.up.sql` and `0002_thing.down.sql`
into `migrations/`. Nothing else needs changing.

## Deployment

`go build ./cmd/jobpulse` produces a binary that needs only a `DATABASE_URL`. It
runs on any host with any PostgreSQL; Compose exists purely to hand you a local
database. Nothing in the code knows about a cloud provider.

Two shapes cover every host:

**Always-on machine** (a VM, a Raspberry Pi): run the binary, done. The internal
poller ticks every `POLL_INTERVAL`.

**Scale-to-zero container** (the usual free tier): these give no persistent disk
and no always-on process, so point `DATABASE_URL` at a free hosted Postgres, set
`POLL_INTERVAL=0`, and have any free cron service call `POST /api/poll` every
15 minutes. The request wakes the container, runs a full cycle, sends the
notifications and returns its stats — the app was built around that endpoint
being synchronous.

The database is about a megabyte for thousands of jobs, so any free Postgres
tier is two orders of magnitude more than enough.
