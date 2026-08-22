# JobPulse

Watches public company job boards and tells me when a new matching job appears.

One Go binary (REST API + poller), one PostgreSQL database, one web app (a PWA)
with three screens. The design and its deliberate omissions are in
[ARCHITECTURE.md](ARCHITECTURE.md) — read that before adding anything.

## Status

**Live.** The backend polls 54 boards across fourteen providers, stores what is new,
matches it against search profiles, and pushes one summary per profile to the
phone; the app is an installable PWA with search, sorting and push. A full cycle
takes a few seconds; the deployment runs entirely on free tiers.

## Stack

Go · chi · pgx · golang-migrate · PostgreSQL · React · Vite · TypeScript

SQL is written by hand. There is no ORM, no code generation and no repository
layer: handlers and the poller take a `*pgxpool.Pool` and run their own queries.

## Run it locally

```bash
docker compose up -d          # PostgreSQL on :5432
go run ./cmd/jobpulse         # migrates, polls, serves on :8080
cd web && npm install && VITE_JOBPULSE_API=http://localhost:8080 npm run dev
```

Migrations run automatically on startup, so there is no separate migrate step
and no `migrate` CLI to install.

To use the dev build from a phone, `localhost` is the phone, not your computer —
set `VITE_JOBPULSE_API` to your machine's address on the network. The URL in use
is shown under Settings → Backend.

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

The database stores only live, applyable listings: a job removed from its board
disappears on the next poll, and nothing older than 45 days is kept or ingested
at all.

The slug is whatever identifies the company on that provider, usually the last
part of its careers URL. The file is the source of truth and is re-read on every
start, so removing a line stops that board being polled. Supported providers:
`greenhouse`, `lever`, `ashby`, `smartrecruiters`, `workable`, `recruitee`, `himalayas`, `jobicy`,
`teamtailor`, `manatal`, `phenom`, `oracle`, plus the metered aggregators
`jobven` and `jobspipe`, whose "slug" is a saved search rather than a company.
`careerjet` is implemented but parked: its API is IP-allowlisted and free
hosting has no fixed egress address.

Remote feeds (`himalayas`, `jobicy`) are filtered by reachability — a posting
restricted to a country this hunt cannot work in is dropped at ingest. The list
lives in `reachableRegions` in [internal/poll](internal/poll/poll.go); widen it
there to accept, say, US-remote roles.

## API

```
GET    /healthz                                    pings the database

GET    /api/profiles
POST   /api/profiles          {name, keywords[], locations[], remote_only}
PUT    /api/profiles/{id}
DELETE /api/profiles/{id}

GET    /api/jobs?profile_id=1&limit=50&cursor=…    sort=posted|matched|applied; q= and location= search/filter
GET    /api/boards                                 every board's health
POST   /api/jobs/{id}/hide                         hide from this device's feeds
POST   /api/jobs/{id}/unhide                       the undo
POST   /api/jobs/{id}/applied                      toggle applied; answers the new state

GET    /api/notifications?limit=50&cursor=…        match feed, all profiles
POST   /api/notifications/seen                     mark the feed read

POST   /api/devices                                {token, platform, timezone} — FCM registration
POST   /api/poll                                   run a cycle now; returns its stats
```

Profile keywords match case-insensitive substrings, expanded through a small
role dictionary ("frontend" also finds React and Angular titles, but not bare
"Software Engineer" — an alias has to name the same job, not a wider one); a
`-` prefix excludes (`designer, -senior`). The search bar expands the same way,
so a profile and a typed query agree about what a role means. Matches are
re-derived every cycle, so editing [aliases.go](internal/match/aliases.go) or a
profile fixes what is already stored, not just what arrives next. Salaries are shown when a
board publishes them. Creating or editing a profile backfills it against every
job already stored, so it is never mysteriously empty. `/api/jobs` returns `next_cursor` when another
page exists; pass it back as `cursor` and treat it as opaque.

Every device mints an anonymous id (X-Device header) and sees only its own
profiles, matches and notifications; the job corpus is shared. Push follows the
profile's owner, so a match only buzzes the device that created the search.

There is no authentication, by design. Bind it to `127.0.0.1` or reach it over a
private tunnel — do not put this on a public IP.

## Verify it works

```bash
curl localhost:8080/healthz                  # {"database":"ok","status":"ok"}
go test ./...
cd web && npm test
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
msg="poll cycle" companies=30 failed=0 new_jobs=77 new_matches=6 removed=0 duration=17.2s
```

`new_jobs=0` on the second cycle is the point: it means new-job detection is
working rather than re-alerting on everything.

## Configuration

Everything has a working default, so a fresh clone needs no setup.

| Variable         | Default                                                                | Purpose                        |
| ---------------- | ---------------------------------------------------------------------- | ------------------------------ |
| `DATABASE_URL`   | `postgres://jobpulse:jobpulse@localhost:5432/jobpulse?sslmode=disable` |                                |
| `PORT`           | `8080`                                                                 |                                |
| `POLL_INTERVAL`  | `5m`                                                                   | Go duration; `0` disables the internal ticker |
| `COMPANIES_FILE` | `companies.txt`                                                        |                                |
| `GOOGLE_APPLICATION_CREDENTIALS` | *(unset)*                                              | service account JSON; unset = log instead of push |
| `CAREERJET_API_KEY` / `CAREERJET_SITE` | *(unset)*                                        | publisher key + site; unset = careerjet lines error quietly |
| `JOBVEN_API_KEY` | *(unset)*                                                              | metered aggregator key; unset = jobven lines error quietly |
| `JOBSPIPE_API_KEY` | *(unset)*                                                            | metered aggregator key; unset = jobspipe lines error quietly |
| `APP_URL`        | `https://jobpulse-junaid.web.app`                                      | where a tapped notification opens |
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
web/src/            api.ts, query.ts, push.ts, toast.tsx, App.tsx, styles.css
web/src/screens/    jobs, notifications, settings
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
few minutes. The request wakes the container, runs a full cycle, sends the
notifications and returns its stats — the app was built around that endpoint
being synchronous.

The database is about a megabyte for thousands of jobs, so any free Postgres
tier is two orders of magnitude more than enough.
