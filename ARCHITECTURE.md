# JobPulse — Architecture

A small tool that watches public company job boards and tells me when a new
matching job appears.

One Go binary. One Postgres database. One Flutter app. Three screens.

---

## 1. The one constraint that shapes everything

Greenhouse, Lever, Ashby, SmartRecruiters, Workable and Recruitee do **not**
offer global job search. Every public API they expose is scoped to a single
company's board:

```
https://boards-api.greenhouse.io/v1/boards/stripe/jobs
https://api.lever.co/v0/postings/spotify?mode=json
```

There is no `?q=golang&location=remote` across all customers. That product
(LinkedIn, Indeed) is built on crawlers and commercial feeds, and we are
explicitly not building it.

So JobPulse polls **a list of company boards that I maintain**, and matches my
search profiles against what those boards return.

```
companies.txt  →  provider + board slug, one per line
```

This is the honest shape of the problem given the sources. It also means the
tool is genuinely useful: I care about ~100 specific companies, not the whole
internet. Adding a company is one line in a text file.

Everything below follows from this.

---

## 2. System shape

```
┌──────────────────────────────────────────────────────┐
│  jobpulse (single Go process)                        │
│                                                      │
│   ┌────────────────┐          ┌──────────────────┐   │
│   │ poller         │          │ HTTP API (chi)   │   │
│   │ ticker, 15 min │          │ :8080            │   │
│   └───────┬────────┘          └────────┬─────────┘   │
│           │                            │             │
│           └──────────┬─────────────────┘             │
│                      ▼                               │
│              ┌───────────────┐                       │
│              │ Postgres      │                       │
│              └───────────────┘                       │
└──────────────────┬───────────────────────────────────┘
                   │ FCM push
                   ▼
        ┌──────────────────────┐
        │ Flutter app          │
        │ Jobs · Notifs · Set. │
        └──────────────────────┘
```

Two goroutines in one process, not two services. `go run ./cmd/jobpulse`
starts both. There is no queue, no worker, no scheduler daemon — a
`time.Ticker` is enough for something that runs every 15 minutes.

### Request/poll flow

**Poll cycle** (every 15 min, or `POST /api/poll`):

1. Load all companies from DB.
2. For each (max 4 concurrent): fetch board JSON → normalize to `Job`.
3. `INSERT ... ON CONFLICT DO NOTHING RETURNING id` — the returned rows *are*
   the new jobs. That single clause is the entire "detect new jobs" feature.
4. For each new job × each profile: `match.Matches(profile, job)` → insert row
   into `matches`.
5. One FCM push per profile that got new matches ("3 new jobs · Backend Go").

**App flow**: pull `GET /api/jobs?profile_id=…`, render list, Apply button opens
`job.url` in the browser. Push notification just tells the app to refresh.

---

## 3. Database

Five tables. No soft deletes, no audit columns, no polymorphic anything.

```sql
create table profiles (
  id          bigserial primary key,
  name        text        not null,
  keywords    text[]      not null default '{}',   -- OR-ed, matched on title
  locations   text[]      not null default '{}',   -- OR-ed, empty = anywhere
  remote_only boolean     not null default false,
  created_at  timestamptz not null default now()
);

create table companies (
  provider   text not null,          -- greenhouse | lever | ashby | ...
  slug       text not null,          -- board identifier on that provider
  name       text not null default '',
  last_polled_at timestamptz,
  last_error     text,
  primary key (provider, slug)
);

create table jobs (
  id          bigserial primary key,
  provider    text not null,
  external_id text not null,         -- provider's own job id
  company     text not null,         -- display name from the payload
  title       text not null,
  location    text not null default '',
  remote      boolean not null default false,
  url         text not null,         -- official application page
  posted_at   timestamptz,
  first_seen_at timestamptz not null default now(),
  unique (provider, external_id)
);

create table matches (
  profile_id bigint not null references profiles(id) on delete cascade,
  job_id     bigint not null references jobs(id)     on delete cascade,
  created_at timestamptz not null default now(),
  seen_at    timestamptz,
  primary key (profile_id, job_id)
);

create table devices (
  token      text primary key,       -- FCM registration token
  platform   text not null,
  created_at timestamptz not null default now()
);

create index on jobs (first_seen_at desc);
create index on matches (profile_id, created_at desc);
```

Notes:

- `keywords`/`locations` as `text[]` instead of child tables. They are lists of
  short strings that are always read and written together with the profile.
  Two extra tables would buy nothing.
- `matches` is both the Jobs feed and the Notifications feed. Jobs screen =
  matches joined to jobs, ordered by job recency. Notifications screen = same
  rows ordered by `created_at` with `seen_at` for the unread dot. One table,
  two queries.
- No `descriptions`. The Apply button goes to the real posting; storing
  megabytes of HTML to render a worse version of it is not worth it. (Greenhouse
  drops from 4.4 MB to 360 KB per board once you stop asking for `content=true`.)
- `companies` has a composite natural key. No surrogate id needed since jobs
  denormalize the company display name.

Migrations: `golang-migrate`, plain `.sql` files in `migrations/`.
Queries: `sqlc` from `internal/db/queries.sql` → typed Go. `pgx` pool.

---

## 4. Providers

All six are unauthenticated GETs returning JSON. I probed each one live; the
mappings below are verified, not guessed.

```go
// internal/providers/providers.go
type Job struct {
    ExternalID string
    Company    string
    Title      string
    Location   string
    Remote     bool
    URL        string
    PostedAt   time.Time
}

type Provider interface {
    Name() string
    Fetch(ctx context.Context, slug string) ([]Job, error)
}

var All = map[string]Provider{
    "greenhouse":     greenhouse{}, "lever":    lever{},
    "ashby":          ashby{},      "workable": workable{},
    "smartrecruiters": smartrecruiters{}, "recruitee": recruitee{},
}
```

One 12-line interface, one map, six files of ~60 lines each. Adding a provider =
new file + one map entry. That is the whole "plugin system".

### Endpoints and mappings

| Provider | Endpoint (`{slug}` = board) | Verified with |
|---|---|---|
| Greenhouse | `boards-api.greenhouse.io/v1/boards/{slug}/jobs` | `stripe` (577 jobs) |
| Lever | `api.lever.co/v0/postings/{slug}?mode=json` | `spotify`, `leverdemo` |
| Ashby | `api.ashbyhq.com/posting-api/job-board/{slug}` | `openai` |
| SmartRecruiters | `api.smartrecruiters.com/v1/companies/{slug}/postings?limit=100&offset=N` | `Visa` |
| Workable | `apply.workable.com/api/v1/widget/accounts/{slug}?details=true` | `blueground`, `spotawheel` |
| Recruitee | `{slug}.recruitee.com/api/offers/` | `channable` |

| Provider | id | title | location | remote | url | posted |
|---|---|---|---|---|---|---|
| Greenhouse | `id` | `title` | `location.name` | location text contains "remote" | `absolute_url` | `first_published` |
| Lever | `id` | `text` | `categories.location` | `workplaceType == "remote"` | `hostedUrl` | `createdAt` (epoch **ms**) |
| Ashby | `id` | `title` | `location` | `isRemote` (nullable) | `jobUrl` | `publishedAt` |
| SmartRecruiters | `id` | `name` | `location.fullLocation` | `location.remote` | *constructed* (below) | `releasedDate` |
| Workable | `shortcode` | `title` | `city, country` | `telecommuting` | `url` | `published_on` (date only) |
| Recruitee | `id` | `title` | `location` | `remote` | `careers_url` | `published_at` (`"… UTC"` string) |

Gotchas worth knowing before you write the code:

- **Greenhouse**: never pass `content=true`. 12× the bytes for a description we
  don't store.
- **Lever**: `createdAt` is milliseconds since epoch, not a timestamp string.
  An unknown slug returns `404 {"ok":false}`; a valid company with nothing open
  returns `200 []`.
- **Ashby**: `isRemote` is often `null` — treat as false. Payloads are large
  (OpenAI's board is 12 MB) because descriptions are always inlined; decode with
  a struct that ignores them so they never hit the heap twice.
- **SmartRecruiters**: the list endpoint has no `applyUrl`. Build it:
  `https://jobs.smartrecruiters.com/{slug}/{id}` (verified 200). Paginate with
  `offset`/`limit` until `len(content) == 0`. It is also the only provider with a
  server-side `releasedAfter=` filter — ignore it, uniform code is worth more.
- **Workable**: `published_on` is `YYYY-MM-DD`, so ordering within a day is
  arbitrary. Some accounts return `{"jobs": []}` rather than 404.
- **Recruitee**: response contains offers in several `status` values — keep only
  `"published"`. Timestamps are `"2026-08-03 15:40:43 UTC"`, needing a custom
  layout.

### Politeness

`http.Client{Timeout: 30s}`, a real `User-Agent`, at most 4 boards in flight,
one retry on 5xx/timeout. A failing board writes `last_error` and the cycle
continues — one dead company never stalls the poll.

---

## 5. Matching

```go
// internal/match/match.go — the whole file is ~40 lines
func Matches(p db.Profile, j providers.Job) bool {
    if p.RemoteOnly && !j.Remote {
        return false
    }
    if len(p.Locations) > 0 && !containsAnyFold(j.Location, p.Locations) {
        return false
    }
    return len(p.Keywords) == 0 || containsAnyFold(j.Title, p.Keywords)
}
```

Case-insensitive substring on the title. No stemming, no fuzzy matching, no
Postgres full-text, no relevance score.

Two call sites, same function:
- new job arrives → test against every profile,
- new profile created → test against every stored job (a backfill so a fresh
  profile isn't empty).

If matching turns out to be too loose in practice, the fix is editing this
function, not introducing a query language.

---

## 6. Notifications

Firebase Cloud Messaging via `firebase.google.com/go/v4/messaging`. The app
registers its token with `POST /api/devices`; the poller sends to every stored
token.

**One push per profile per cycle**, not per job:

> **3 new jobs · Backend Go** — Stripe, Channable, Orfium

Fifteen matches in one cycle should not be fifteen buzzes. A stale token
returns `UNREGISTERED`; delete that row and move on.

If `GOOGLE_APPLICATION_CREDENTIALS` is unset, the notifier logs to stdout
instead. The whole project must be runnable with `docker compose up` and no
Firebase account — push is the one piece that needs external setup, so it is the
one piece that degrades gracefully.

---

## 7. HTTP API

Chi, `encoding/json`, plain `http.HandlerFunc`. No DTO layer — sqlc's structs
are the JSON shapes.

```
GET    /api/profiles
POST   /api/profiles              {name, keywords[], locations[], remote_only}
PUT    /api/profiles/{id}
DELETE /api/profiles/{id}

GET    /api/jobs?profile_id=&before=&limit=50     matched jobs, newest first
GET    /api/notifications?limit=50                match events, unread flag
POST   /api/notifications/seen                    {job_ids: [...]} → mark read

POST   /api/devices               {token, platform}
POST   /api/poll                  trigger a cycle now (dev + pull-to-refresh)
GET    /healthz
```

Cursor pagination on `before` (a timestamp), because offset pagination on a feed
that grows at the head shows duplicates. Ten extra lines, one real bug avoided.

No auth, per the brief. That means: bind to `127.0.0.1` at home, or reach it
through a private tunnel. Do not put this on a public IP.

---

## 8. Flutter app

`Riverpod` for state, `GoRouter` for the three routes, `Dio` for HTTP,
`firebase_messaging` for push.

```
app/lib/
  main.dart              ProviderScope + router + theme
  api.dart               Dio client, one method per endpoint
  models.dart            Job, Profile, Notification (fromJson only)
  providers.dart         AsyncNotifier per screen (~3 of them)
  screens/jobs.dart
  screens/notifications.dart
  screens/settings.dart
  widgets/job_tile.dart
```

Eight files. No feature folders, no per-screen barrel files, no clean layers.

**Jobs** — profile chips along the top, dense list below. Each row: title,
company · location, relative time, `Apply` on the right (`url_launcher`,
external browser). Pull to refresh calls `POST /api/poll` then refetches.

**Notifications** — reverse-chronological match events, unread ones with a small
accent dot. Tapping opens the job. Opening the screen marks visible items seen.

**Settings** — create/edit/delete profiles (name, keyword chips, location chips,
remote-only switch), backend URL, and the FCM token with a copy button for
debugging.

**Look** — dark-first with a light variant, one accent colour, no elevation, no
gradients, hairline dividers, generous vertical rhythm, `Inter` for text and a
mono face for timestamps and locations. Tight information density like Linear;
no avatars, badges, or engagement furniture.

FCM push carries no payload beyond a nudge — the app invalidates the jobs
provider and refetches. Never trust a notification body as data you then store.

---

## 9. Layout & running it

```
job-pulse/
  cmd/jobpulse/main.go          flags, wiring, ticker + server
  internal/api/                 router.go, handlers.go
  internal/db/                  queries.sql, sqlc generated code
  internal/providers/           providers.go + one file per provider
  internal/match/match.go
  internal/notify/fcm.go
  internal/poll/poll.go
  migrations/                   0001_init.up.sql / .down.sql
  companies.txt                 seed list: "greenhouse stripe"
  docker-compose.yml            postgres only
  Makefile                      migrate, sqlc, run, seed
  app/                          Flutter
```

```bash
git clone … && cd job-pulse
docker compose up -d          # postgres
make migrate && make seed     # schema + companies.txt
go run ./cmd/jobpulse         # API on :8080, polls immediately then every 15m
cd app && flutter run
```

Configuration is five environment variables with working defaults:
`DATABASE_URL`, `PORT`, `POLL_INTERVAL`, `GOOGLE_APPLICATION_CREDENTIALS`,
`COMPANIES_FILE`.

Deployment is "run the binary next to a Postgres". It works on a free VM, a
Fly-style container, a Raspberry Pi, or a laptop with cron. Nothing in the code
knows or cares.

---

## 10. Realistic plan

| Weekend | Work |
|---|---|
| 1 | Migrations, sqlc, Greenhouse + Lever, poll loop, `/api/jobs`. Verify new-job detection against a real board. |
| 2 | Remaining four providers, matching, profiles CRUD, `companies.txt` seeding. |
| 3 | Flutter: three screens, API client, theme. |
| 4 | FCM both ends, `seen_at` plumbing, README. |

Roughly 1,200 lines of Go and 900 of Dart. If it grows much past that, something
has been over-designed.

---

## 11. What I deliberately did NOT build

**Global job search / crawling.** No provider offers it, and getting it means
scraping aggregators or driving a headless browser. A curated `companies.txt` is
more accurate, never breaks, and matches how I actually job-hunt.

**Authentication and user accounts.** Single user. Auth would mean a users
table, token issuance, refresh, password reset, and a foreign key on every
table — for one person. Bind to localhost instead. If I ever share it with a
friend, they run their own copy; that is one `git clone`, not a multi-tenancy
migration.

**A provider plugin system.** Registry interfaces, dynamic loading, per-provider
config schemas — all to avoid editing a 20-line map. Six providers, one
`map[string]Provider`. A seventh is one file plus one line.

**Repository pattern / service layer / hexagonal boundaries.** sqlc already
generates a typed data access layer. Wrapping it in interfaces so I can swap
Postgres for a database I will never use is pure ceremony. Handlers call
queries directly.

**A separate worker process, queue, or scheduler.** Six providers × ~100
companies every 15 minutes is a few hundred HTTP requests an hour. A goroutine
with a ticker does this. Redis, Kafka, RabbitMQ, Temporal, cron containers — all
solving a load problem I do not have.

**Elasticsearch / full-text search / relevance ranking.** Matching is
substring-on-title over a corpus in the tens of thousands. Postgres `tsvector`
would be overkill; Elasticsearch would be a second datastore to operate for a
feature that is 40 lines of Go.

**Storing job descriptions.** Multiplies storage by ~30×, forces HTML
sanitization, and produces a worse rendering than the real posting the Apply
button already opens.

**Job state: saved / applied / archived / hidden.** Every one of these is a
column plus an endpoint plus a UI affordance plus a sync question. I want to be
told about new jobs and then leave the app. If I miss saving jobs after a month
of real use, `matches.saved_at` is a five-minute migration — added because I
needed it, not because I predicted it.

**Company management UI.** A text file edited in the editor I already have open,
version-controlled with the rest of the repo, beats a CRUD screen and an admin
panel.

**Email / Telegram / Slack / webhook notifications.** FCM covers phone and web,
which is where I read notifications. Each extra channel is another credential,
another failure mode, another template.

**Per-job push notifications.** Deliberately batched per profile per cycle.
Fifteen matches should not be fifteen buzzes.

**Analytics, metrics, tracing, structured event logging.** `log/slog` to stdout
tells me what happened. Prometheus and OpenTelemetry are for systems with users.

**Retry queues, dead-letter tables, circuit breakers.** A board that fails gets
one retry, records `last_error`, and is tried again in 15 minutes. Polling is
naturally self-healing — that is the main reason to prefer it over webhooks here.

**Docker image for the backend, CI/CD, Kubernetes manifests.** `go run` locally,
`go build` on the box. Compose exists only to hand me a Postgres.

**Tests for everything.** I will write table-driven tests for `match.Matches`
and one golden-file test per provider parser (a saved JSON fixture → expected
`[]Job`), because those are where bugs are silent and expensive. Not handler
tests, not an integration harness, not a mock HTTP layer.

**AI anything.** No embeddings for "semantic" matching, no LLM job scoring, no
resume tailoring, no cover letters. It would add an API key, latency, cost, and
non-determinism to a problem that `strings.Contains` solves correctly.
