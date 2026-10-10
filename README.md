# JobPulse

Watches public company job boards and pushes a new matching job to your phone
within minutes of it appearing.

Live at [jobpulse-junaid.web.app](https://jobpulse-junaid.web.app). One Go
binary (poller plus REST API), one PostgreSQL database, one React PWA. The
backend polls about 220 boards across thirteen hiring providers every five
minutes, stores what is new, matches it against saved searches, and sends one
push per search per cycle. It runs entirely on free tiers.

The design, and every decision not to build something, is in
[ARCHITECTURE.md](ARCHITECTURE.md). Read that before adding anything.

## Why it exists

Greenhouse, Lever, Ashby, Workday and the rest expose public APIs scoped to a
single company's board. None of them offer search across companies; that
product is built on crawlers and commercial feeds. So JobPulse polls a curated
list of boards instead, which is more accurate than scraping, never breaks, and
matches how a job hunt actually works: there are about a hundred employers you
care about, not the whole internet.

The whole product is the gap between a posting appearing and the phone buzzing.
Aggregators deliver the same posting about a day late (one of them measured a
median lag of twenty-one hours); a direct board delivers it inside a cycle.

## What is in it

**The poller.** A `time.Ticker` and a goroutine in the same process as the
API, at most four boards in flight, one retry on failure, a failing board
recorded and skipped so one dead company never stalls a cycle. New-job
detection is one SQL clause: `INSERT ... ON CONFLICT DO NOTHING RETURNING id`,
and the returned rows are the new jobs. Production polls 202 boards in about
26 seconds; a cold cycle peaked at 47 MB of memory on a 0.1 vCPU, 256 MB host.

**Thirteen providers, one interface.** Each adapter is a file of roughly sixty
lines mapping one board API onto a `Job`. Adding a provider is one file and one
map entry. The gotchas per provider (Lever's epoch milliseconds, Ashby's 12 MB
payloads, Workday's per-tenant location facets) are documented in the
architecture file because every one of them was found by probing the API
rather than reading about it.

**Matching that agrees with itself.** A saved search and a typed query expand
through the same small role dictionary, so "frontend" finds React and Angular
titles in both places and never bare "Software Engineer". Matches are
re-derived every cycle, so editing the dictionary fixes what is already stored.
Two of the rows in the end-to-end fixture exist because they were bugs: a
posting in Indianapolis, Indiana must never answer a search for India.

**Push that degrades.** Firebase Cloud Messaging over its HTTP v1 API, one
authenticated POST per device, batched per search so fifteen matches are one
buzz. With no Firebase credentials it logs to stdout instead, so a fresh clone
runs with `docker compose up` and nothing else.

**A PWA instead of an app.** The React build is about 80 KB gzipped and
replaced a working Flutter implementation: once web push proved out on a real
iPhone, Flutter was paying a multi-megabyte CanvasKit tax to draw three list
screens. Add to Home Screen gives an icon, full screen and push on iOS without
an Apple Developer account.

**A scout that proposes and never decides.** See below.

**Tests that press the button.** Unit tests cover the matcher, the poller, the
query building and every provider parser against saved fixtures (97 Go test
functions). The browser suite runs the real backend against a real Postgres in
CI, seeded with a known corpus, and drives the whole app through a phone
browser. Two further checks are plain SQL and `curl` against that backend: that
a housekeeping sweep never deletes a job you applied to, and that board
probation retires a board that never delivered without retiring one that did.
The browser half exists because the unit tests were green on a day when the
dismiss button on every job row answered 404 and the frontend swallowed it.

**Production that complains.** `GET /healthz` reports the database, the
poller's state and age, and whether push can reach the phone, because a server
that cannot reach the phone looks exactly like a quiet job market. An hourly
GitHub workflow fails, and therefore emails, when the boards have not been read
for half an hour or push is anything but `ok`. A `pg_cron` job in the database
pings the same endpoint every five minutes, and any inbound request revives a
poller that has fallen behind.

## The scout

Once a day, `cmd/scout` looks for direct boards belonging to employers whose
postings already match a saved search but only ever arrive through an
aggregator. Those are the jobs reaching you late: aggregators deliver a posting
a median of eleven hours after it goes up, a direct board about six minutes.

**Code finds, a model judges, the server decides.** The scout sweeps every
eligible employer's spellings across the hiring systems addressed by a company's
name, and reads careers pages for the ones addressed by a tenant instead. Each
candidate board then goes to a model as one question — *does this board belong
to this employer?* — with what the board posts beside what the employer is known
to post. Only a yes is proposed, and `POST /api/boards` probes it again with the
poller's own code and refuses anything without postings this hunt can reach.

It was an agent first, and measuring it is why it is not one now. Over 89 hunts
every board it added had come from the name sweep; the model's own exploration
found none, and 79% of the careers pages it opened were addresses it had made
up. It took two and a half minutes an employer, so a day reached eight of 170.
The sweep alone covered a hundred in three minutes. What the sweep cannot do is
tell a company from a namesake — "Future Data" swept to a fitness app hiring
health coaches — and that judgment is the one job the model is good at.

The judge is `gemma4:12b` with reasoning off, chosen on a 23-case exam of real
candidates under the runner's limits (four CPUs, 16 GB):

| Model | Right | Wrong yes | Per case |
|---|---|---|---|
| **gemma4:12b** | **22 / 23** | 0 | 58 s |
| qwen3.5:9b | 21 / 23 | 0 | 39 s |
| gpt-oss:20b | 21 / 23 | 0 | 70 s, at the memory limit |
| qwen3:8b | 20 / 23 | 0 | 14 s |
| qwen3.5:4b | 16 / 23 | 0 | 27 s |
| qwen2.5:7b (before) | 10 / 23 | 0 | 21 s, answered no to everything |

Reasoning left on cost over five minutes a question on four CPUs. The exam is
`cmd/scout/testdata/judge_cases.json`; rerun it for any model or prompt change
with `SCOUT_MODEL=<model> go test -tags judgeeval -run TestJudgeExam ./cmd/scout/`.

Its work is reviewed by outcome, not by another model. A discovered board that
has held nothing in the market and matched no search within 21 days is retired,
and every run prints a scorecard of what each discovered board has delivered on
the workflow run's page.

It runs in GitHub Actions on a free public-repository runner and touches
nothing the poller depends on. If it is broken or wrong, the app behaves exactly
as it does without it.

```bash
# against a local backend, with the model on your own machine and no keys at all
docker run -d -p 11434:11434 -v ollama-models:/root/.ollama ollama/ollama
docker exec ollama ollama pull gemma4:12b
SCOUT_MODEL_REASONING=none go run ./cmd/scout -targets 20
```

`scout -list` answers "is there anything to do" without a model, so a quiet
day costs seconds.

## Decisions worth reading

[ARCHITECTURE.md](ARCHITECTURE.md) section 11 lists everything deliberately
not built and why. A few of them:

- **No model anywhere near the notification path.** The product is a latency
  gap; the model lives on the far side of a gate, on someone else's compute.
- **No authentication.** Every device mints an anonymous id and sees only its
  own searches. A user table, sessions and password reset for one person is
  ceremony; anyone who wants multi-user runs their own copy.
- **No provider plugin system, no repository layer, no queue.** Thirteen
  adapters are a map. Handlers run their own SQL on a `pgx` pool so every
  query is visible where it is used. A few thousand requests an hour is a
  ticker, not a scheduler.
- **No stored job descriptions.** Thirty times the storage to render a worse
  version of the page the Apply button already opens. The price is paid at the
  search bar, and the empty state says so.
- **Workday was refused, then measured, then built.** The first version of
  that section said Workday tenants held no Gulf inventory. That was a claim
  about a measurement that had never been made properly: applying each board's
  own location facet turned global career sites into small local boards and
  added 1,709 in-market postings to a corpus of 5,600.

## Lessons from production

All paid for with outages.

- **Check what the host meters, not what it offers.** A job-board watcher does
  almost nothing but download. One host billed downloads as bandwidth and
  suspended the workspace at 5 GB for seventeen days.
- **Never poll inside a request.** A cycle outlives a scheduler's 30-second
  timeout, so every run was killed halfway and each aborted call logged as a
  success. `POST /api/poll` answers 202 and the cycle runs detached.
- **`shell: bash` is load-bearing in GitHub Actions.** The default shell has no
  `pipefail`, so a pipe into `tee` took `tee`'s exit status. For seventeen days
  the scout failed to reach a down server, reported success, and a missing
  output compared equal to zero, so every day was quietly "nothing to do".
- **Put the server near the database.** A cycle makes hundreds of round trips;
  a cross-continent database turns seconds into minutes.
- **There are no backups, on purpose.** The corpus rebuilds from the boards in
  one cycle; the only irreplaceable rows are a few dozen saved searches.

## Stack

Go, chi, pgx, golang-migrate, PostgreSQL, React, Vite, TypeScript, Playwright.
SQL is written by hand. There is no ORM, no code generation and no repository
layer.

## Run it locally

```bash
docker compose up -d          # PostgreSQL on :5432
go run ./cmd/jobpulse         # migrates, polls, serves on :8080
cd web && npm install && VITE_JOBPULSE_API=http://localhost:8080 npm run dev
```

Migrations run automatically on startup. To use the dev build from a phone,
`localhost` is the phone, not your computer: set `VITE_JOBPULSE_API` to your
machine's address on the network. The URL in use is shown under Settings,
Backend.

If port 5432 or 8080 is taken:

```bash
POSTGRES_PORT=5434 docker compose up -d
PORT=8091 DATABASE_URL='postgres://jobpulse:jobpulse@localhost:5434/jobpulse?sslmode=disable' go run ./cmd/jobpulse
```

## Tests

```bash
go test ./...                     # the matcher, the poller, the API's query building, every parser
cd web && npx vitest run          # the feed's pure logic
scripts/smoke.sh                  # every route, against a running backend
cd e2e && npx playwright test     # the whole app, driven through a phone browser
```

The browser suite runs the real backend against a real Postgres with
`COMPANIES_FILE=e2e/fixtures/companies.txt`, which is deliberately empty, so a
poll cycle reaches nothing and the corpus is exactly the rows in
`e2e/fixtures/seed.sql`. Seed after the backend starts: its first cycle removes
every job whose board is not in the file.

One trap when running the browser suite by hand: the backend URL is baked into
the bundle, so `dist` has to be built against whichever backend the tests will
talk to. A `dist` left over from a production build sends the suite at
production, where the seeded corpus does not exist.

Both suites run on every push (`.github/workflows/ci.yml`).

## Boards to watch

None of the providers offer search across companies, so JobPulse polls the
list in [companies.txt](companies.txt):

```
# provider         slug         display name
greenhouse         stripe       Stripe
lever              spotify      Spotify
```

The slug is whatever identifies the company on that provider, usually the last
part of its careers URL. The file is the source of truth for its own rows and
is re-read on every start; boards the scout discovered carry a different origin
and are managed through the API. Supported providers: `greenhouse`, `lever`,
`ashby`, `smartrecruiters`, `workable`, `recruitee`, `teamtailor`, `phenom`,
`oracle`, `workday`, plus the metered aggregators `jobven`, `jobspipe` and
`careerjet`, whose "slug" is a saved search rather than a company.

A `workday` slug is the careers host and site plus the location facet that
narrows a global board to this market. Both the facet's name and its ids are
per-tenant; read them off the board's own response (`jq .facets`) rather than
copying another board's.

The database stores only live, applyable listings: a job removed from its
board disappears on the next poll, and nothing first published more than
fourteen days ago is kept or ingested at all (seven for the aggregators, whose
postings cannot be checked for removal). A job you marked applied is kept
regardless.

Adapters that stop earning their place are deleted rather than left in the
registry. They are in the git history if any of them ever earns its way back.

## API

```
GET    /healthz                                    database, poller state and age, push state

GET    /api/profiles
POST   /api/profiles          {name, keywords[], locations[], remote_only}
PUT    /api/profiles/{id}
DELETE /api/profiles/{id}

GET    /api/jobs?profile_id=1&limit=50&cursor=...  sort=posted|matched|applied; q= and location= search/filter
GET    /api/jobs?mine=1                            every saved search's matches, newest arrival first
GET    /api/boards                                 every board's health
POST   /api/jobs/{id}/hide                         hide from this device's feeds
POST   /api/jobs/{id}/unhide                       the undo
POST   /api/jobs/{id}/applied                      toggle applied; answers the new state

POST   /api/notifications/seen                     mark the arrivals seen

POST   /api/devices                                {token, platform, timezone}, FCM registration
GET    /api/devices/status                         does the server hold a token, and when did a push last land
POST   /api/devices/test                           prove the push chain in one request
PUT    /api/devices/quiet-hours                    {from, to} in the device's timezone; equal hours = off
POST   /api/poll                                   start a cycle; answers 202 at once, polls in the background

GET    /api/discovery                              employers worth looking for, boards that stopped answering
POST   /api/boards                                 offer a board; the server probes it and decides
```

Profile keywords match case-insensitive substrings, expanded through the role
dictionary in [aliases.go](internal/match/aliases.go); a `-` prefix excludes
(`designer, -senior`). `q=` matches every word independently, on word
boundaries. Creating or editing a profile backfills it against every job
already stored. `/api/jobs` returns `next_cursor` when another page exists;
pass it back as `cursor` and treat it as opaque.

Every device mints an anonymous id (`X-Device` header) and sees only its own
profiles, matches and notifications; the job corpus is shared. Push follows the
profile's owner. There is no authentication, by design: bind it to
`127.0.0.1` or reach it over a private tunnel.

## Configuration

Everything has a working default, so a fresh clone needs no setup.

| Variable         | Default                                                                | Purpose                        |
| ---------------- | ---------------------------------------------------------------------- | ------------------------------ |
| `DATABASE_URL`   | `postgres://jobpulse:jobpulse@localhost:5432/jobpulse?sslmode=disable` |                                |
| `PORT`           | `8080`                                                                 |                                |
| `POLL_INTERVAL`  | `5m`                                                                   | Go duration; cannot be disabled, and is raised to 1m if shorter |
| `COMPANIES_FILE` | `companies.txt`                                                        |                                |
| `GOOGLE_APPLICATION_CREDENTIALS` | *(unset)*                                              | the service account as a path, the JSON itself, or base64; unset = log instead of push |
| `CAREERJET_API_KEY` / `CAREERJET_SITE` | *(unset)*                                        | publisher key and site, locked to declared IPs; unset = careerjet lines error quietly |
| `JOBVEN_API_KEY` | *(unset)*                                                              | metered aggregator key; unset = jobven lines error quietly |
| `JOBSPIPE_API_KEY` | *(unset)*                                                            | metered aggregator key; unset = jobspipe lines error quietly |
| `APP_URL`        | `https://jobpulse-junaid.web.app`                                      | where a tapped notification opens |
| `POSTGRES_PORT`  | `5432`                                                                 | host port published by Compose |

## Layout

```
cmd/jobpulse/       main: config, migrate, poller, HTTP server, graceful shutdown
cmd/scout/          the daily discovery agent
internal/api/       chi router and handlers
internal/config/    environment variables
internal/db/        pgx pool and migration runner
internal/match/     does a job satisfy a profile; the role dictionary
internal/poll/      the poll cycle and companies.txt
internal/notify/    push to the phone over FCM
internal/providers/ one file per job board
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

Production is the backend on Northflank's free sandbox, the database on
Supabase's free tier, and the web app on Firebase Hosting. How it is set up,
and what is and is not billed, is in [deploy/README.md](deploy/README.md). A
push to `main` rebuilds and redeploys the backend.

## Licence

MIT. See [LICENSE](LICENSE).
