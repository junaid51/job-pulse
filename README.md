# JobPulse

Watches public company job boards and tells me when a new matching job appears.

One Go binary (REST API + poller), one PostgreSQL database, one web app (a PWA)
with three screens. The design and its deliberate omissions are in
[ARCHITECTURE.md](ARCHITECTURE.md) — read that before adding anything.

## Status

**Live.** The backend polls 220 boards across fourteen providers, stores what is new,
matches it against search profiles, and pushes one summary per profile to the
phone; the app is an installable PWA with search, sorting and push. A full cycle
takes a few seconds; the deployment runs entirely on free tiers.

## Tests

```bash
go test ./...                     # the matcher, the poller, the API's query building
cd web && npx vitest run          # the feed's pure logic
scripts/smoke.sh                  # every route, against a running backend
cd e2e && npx playwright test     # the whole app, driven through a phone browser
```

The browser suite runs the real backend against a real Postgres with
`COMPANIES_FILE=e2e/fixtures/companies.txt` — deliberately empty, so a poll
cycle reaches nothing and the corpus is exactly the thirteen rows in
`e2e/fixtures/seed.sql`. Two of those rows are there because they were bugs:
a posting in "Indianapolis, Indiana" must never answer a search for India, and
one in Romania must never answer a search for the Gulf. Seed *after* the
backend starts: its first cycle removes every job whose board is not in the
file.

One trap when running the browser suite by hand: the backend URL is baked into
the bundle, so `dist` has to be built against whichever backend the tests will
talk to. A `dist` left over from a production build sends the suite at
production, where the seeded corpus does not exist — sixty-eight failures that
say nothing about the app.

Both suites run on every push (`.github/workflows/ci.yml`). They exist because
the unit tests were green on a day when the X on a job row answered 404 for
every search result, and the frontend swallowed it, so the button just looked
dead.

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
`teamtailor`, `phenom`, `oracle`, `workday`, plus the metered aggregators
`jobven` and `jobspipe`, whose "slug" is a saved search rather than a company.

Three adapters were deleted on 2026-08-27 rather than left in the registry:
`manatal` and `remotive` had already been dropped from `companies.txt` for
posting dead and irrelevant jobs, and `careerjet` had been parked for eight days
waiting for an IP allowlist that free hosting cannot satisfy anyway. They are in
the git history if any of them ever earns its way back.

A `workday` slug is the careers host and site plus the location facet that
narrows a global board to this market — `kbr.wd5.myworkdayjobs.com/KBR_Careers?locationHierarchy1=…`.
Both the facet's name and its ids are per-tenant; read them off the board's own
response (`jq .facets`) rather than copying another board's.

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
                                                   q= matches every word independently, on word boundaries
GET    /api/jobs?mine=1                            every saved search's matches, newest arrival first
GET    /api/boards                                 every board's health
POST   /api/jobs/{id}/hide                         hide from this device's feeds
POST   /api/jobs/{id}/unhide                       the undo
POST   /api/jobs/{id}/applied                      toggle applied; answers the new state

POST   /api/notifications/seen                     mark the arrivals seen

POST   /api/devices                                {token, platform, timezone} — FCM registration
GET    /api/devices/status                         does the server hold a token, and when did a push last land
POST   /api/devices/test                           prove the push chain in one request
PUT    /api/devices/quiet-hours                    {from, to} in the device's timezone; equal hours = off
POST   /api/poll                                   start a cycle; answers 202 at once, polls in the background
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
| `POLL_INTERVAL`  | `5m`                                                                   | Go duration; cannot be disabled, and is raised to 1m if shorter |
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
web/src/screens/    jobs, settings
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
`POLL_INTERVAL=0`, and have an external scheduler hit the service every five
minutes. Any URL will do — `GET /healthz` is enough — because **any inbound
request revives a poller that has not run in five minutes**, and the cycle runs
detached from the request that started it. Use a real scheduler, not GitHub
Actions: a `*/5` cron there is best effort and measured out at a 25-minute
median, which defeats the point. The ping also keeps the host awake, so the cold
start disappears — watch its free-hours allowance if it has one.

**Two things keep the boards being read.** The process polls on its own timer,
and a `pg_cron` job in the database keeps the process alive — it never sleeps,
and `pg_net` gives it a 60-second timeout, which is the one combination that can
start a spun-down instance:

```sql
create extension if not exists pg_net with schema extensions;
create extension if not exists pg_cron;
select cron.schedule('jobpulse-wake', '*/5 * * * *', $$
  select net.http_get(url := 'https://<backend>/healthz', timeout_milliseconds := 60000)
$$);
-- select * from cron.job_run_details order by start_time desc limit 5;
```

The GitHub workflow is the alarm, not a mechanism: it runs hourly and *fails* if
the boards have not been read for half an hour, which makes GitHub send mail. A
poller that stops silently is the failure this app exists to prevent.

That is the whole arrangement, and it is smaller than what it replaced. Polling
used to be a side effect of inbound traffic, with a self-ping to generate that
traffic and two external schedulers to wake the host; between them they produced
three outages. Free schedulers cannot reliably wake a spun-down instance —
cron-job.org abandons a request after 30 seconds and the instance then never
boots at all, and GitHub's cron fired a `*/10` schedule every one to five hours
— but a database cron can, and while the process lives it needs no help to poll.
Watch the host's free-hours allowance: always-on is about 730 hours a month
against a 750-hour free tier.

That decoupling is not decoration. Polling used to run *inside* the request, on
the request's context, and it cost nineteen hours of silence: a cycle over two
hundred boards takes longer than a scheduler's 30-second timeout, so every run
was killed halfway through — the boards it had reached looked fresh, the rest
went stale, and the scheduler counted each aborted call as a success until the
host started answering 503 and it disabled itself. `GET /healthz` now reports
`poller` and `poll_age_seconds`, and the app says so on the feed, because a
poller that has stopped otherwise looks exactly like a quiet job market.

Two things that bite when picking the database, both learned the hard way:

- **Metered compute and frequent polling do not mix.** A host that bills for
  the time the database is awake, and suspends it after five idle minutes,
  charges for the whole month once something knocks every five minutes. Prefer
  a plan metered on storage and transfer.
- **Put the database in the host's own region.** Every cycle makes hundreds of
  round trips across a hundred boards, so a cross-continent database turns a
  two-second cycle into a minute of waiting.

There are no backups, on purpose. The corpus is rebuilt from the boards within
one cycle, and the only irreplaceable rows — profiles and their applied history
— are a few dozen. Losing them costs a minute of retyping, which is cheaper
than any backup worth maintaining. The request wakes the container, runs a full cycle, sends the
notifications and returns its stats — the app was built around that endpoint
being synchronous.

The database is about a megabyte for thousands of jobs, so any free Postgres
tier is two orders of magnitude more than enough.
