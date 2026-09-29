# JobPulse

Watches public company job boards and tells me when a new matching job appears.

One Go binary (REST API + poller), one PostgreSQL database, one web app (a PWA)
with three screens. The design and its deliberate omissions are in
[ARCHITECTURE.md](ARCHITECTURE.md) — read that before adding anything.

## Status

**Live.** The backend polls 220 boards across thirteen providers every five
minutes, stores what is new, matches it against search profiles, and pushes one
alert per posting to the phone; the app is an installable PWA with search,
sorting and push. It runs entirely on free tiers — see [Deployment](#deployment).

## The scout

Once a day, `cmd/scout` looks for direct boards belonging to employers whose
postings already match a saved search but only ever arrive through an
aggregator — jobs that are reaching you hours late when a direct board would
deliver them in minutes.

```bash
# against a local backend, with a model on your own machine and no keys at all
docker run -d -p 11434:11434 -v ollama-models:/root/.ollama ollama/ollama
docker exec ollama ollama pull qwen2.5:7b
go run ./cmd/scout -targets 3
```

Daily rather than weekly because the work list has a half-life: eight to
eighteen new such employers turn up a day, and aggregator postings age out after
seven days, taking their matches with them. `scout -list` answers "is there
anything to do" without needing a model at all, so a quiet day costs seconds.

It proposes; it never decides. `POST /api/boards` probes each offer itself and
refuses anything that does not answer with reachable postings, so a confident
wrong answer costs nothing. In CI it runs on GitHub's free public-repository
runners with the model on the runner — or on Groq's free tier if `GROQ_API_KEY`
is set as a secret.

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
disappears on the next poll, and nothing first published more than fourteen days
ago is kept or ingested at all — seven for the aggregators, whose postings cannot
be checked for removal. A job you marked applied is kept regardless.

The slug is whatever identifies the company on that provider, usually the last
part of its careers URL. The file is the source of truth and is re-read on every
start, so removing a line stops that board being polled. Supported providers:
`greenhouse`, `lever`, `ashby`, `smartrecruiters`, `workable`, `recruitee`,
`teamtailor`, `phenom`, `oracle`, `workday`, plus the metered aggregators
`jobven`, `jobspipe` and `careerjet`, whose "slug" is a saved search rather than
a company.

Adapters that stop earning their place are deleted rather than left in the
registry: `manatal` and `remotive` on 2026-08-27 for posting dead and irrelevant
jobs, and the remote feeds `himalayas` and `jobicy` on 2026-09-29, when neither
held a single posting open to this market. They are in the git history if any
of them ever earns its way back.

A `workday` slug is the careers host and site plus the location facet that
narrows a global board to this market — `kbr.wd5.myworkdayjobs.com/KBR_Careers?locationHierarchy1=…`.
Both the facet's name and its ids are per-tenant; read them off the board's own
response (`jq .facets`) rather than copying another board's.

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
| `GOOGLE_APPLICATION_CREDENTIALS` | *(unset)*                                              | the service account as a path, the JSON itself, or base64; unset = log instead of push |
| `CAREERJET_API_KEY` / `CAREERJET_SITE` | *(unset)*                                        | publisher key + site, locked to declared IPs; unset = careerjet lines error quietly |
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
internal/notify/    push to the phone over FCM
internal/providers/ one file per job board
cmd/scout/          the daily discovery agent
migrations/         numbered .sql files, embedded into the binary
web/src/            api.ts, query.ts, push.ts, feed.ts, App.tsx, styles.css
web/src/screens/    Jobs, Settings
e2e/                the Playwright suite CI runs against a real backend
scripts/smoke.sh    every API route against a running server
deploy/             how production is set up
```

Adding a migration means dropping `0002_thing.up.sql` and `0002_thing.down.sql`
into `migrations/`. Nothing else needs changing.

## Deployment

`go build ./cmd/jobpulse` produces a binary that needs only a `DATABASE_URL`,
and the [Dockerfile](Dockerfile) wraps it in a 33 MB image. Nothing in the code
knows about a cloud provider.

**Production** is the backend on Northflank's free sandbox, the database on
Supabase's free tier, and the web app on Firebase Hosting. How it is set up, and
what is and is not billed, is in [deploy/README.md](deploy/README.md). A push to
`main` rebuilds and redeploys the backend.

**What watches it.** `GET /healthz` reports `database`, `poller` (`ok`, `stale`,
`failing`, `never ran`), `poll_age_seconds` and `push` — the last because a
server that cannot reach the phone looks exactly like a quiet job market. The
hourly `poll` workflow fails when the boards have not been read for half an hour
or push is anything but `ok`, which makes GitHub send mail. A `pg_cron` job in
the database (`jobpulse-wake`) also pings `/healthz` every five minutes, and any
inbound request revives a poller that has fallen behind, so a stall mends itself
before anyone is emailed about it.

Lessons from earlier hosts, all paid for with outages:

- **Check what the host meters, not just what it offers.** Render billed
  *downloads* as bandwidth, and a job-board watcher does almost nothing else:
  it suspended the whole workspace at 5 GB for seventeen days. Most hosts meter
  only what they send; this app sends a phone a few kilobytes.
- **Metered compute and frequent polling do not mix.** A database billed for the
  time it is awake is awake all month once something knocks every five minutes.
  Prefer a plan metered on storage.
- **Never poll inside a request.** A cycle outlives a scheduler's 30-second
  timeout, so every run was killed halfway and each aborted call was logged as a
  success. `POST /api/poll` answers 202 and the cycle runs detached.
- **Free schedulers cannot wake a sleeping host.** cron-job.org gives up at 30
  seconds and GitHub's cron drifts to hours; a database's `pg_cron` with a
  60-second `pg_net` timeout is the only free clock that could. An always-on
  host makes the question moot.
- **Put the server near the database.** A cycle makes hundreds of round trips,
  so a cross-continent database turns seconds into minutes.

There are no backups, on purpose. The corpus is rebuilt from the boards within
one cycle, and the only irreplaceable rows — profiles and their applied history
— are a few dozen, a minute of retyping. The database is a few megabytes, two
orders of magnitude inside any free Postgres tier.
