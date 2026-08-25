# JobPulse — Architecture

A small tool that watches public company job boards and tells me when a new
matching job appears.

One Go binary. One Postgres database. One web app. Three screens.

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
        │ Web app (PWA)        │
        │ Jobs · Notifs · Set. │
        └──────────────────────┘
```

Two goroutines in one process, not two services. `go run ./cmd/jobpulse`
starts both. There is no queue, no worker, no scheduler daemon — a
`time.Ticker` is enough for something that runs every 15 minutes.

### Request/poll flow

**Poll cycle** (the internal ticker, `POST /api/poll`, or any request arriving
more than five minutes after the last cycle — always detached from the request
that triggered it, and capped at eight minutes so a dead board cannot hold the
lock forever):

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

The jobs table holds only what is worth applying to, and cleanup is the
poller's job. Each fetch is a board's complete current listing, so a stored job
absent from it was closed by the company — deleted on the spot, match history
and all. Postings older than 45 days never enter and are swept out if stored:
past that, an unfilled listing is advertising, not an opening. The ingest
filter and the sweep must agree, or an old posting still on its board would be
deleted and re-announced every cycle. Aggregator (metered) providers only show
a window of results, so absence proves nothing there; their rows leave through
the age sweep alone.

Notes:

- `keywords`/`locations` as `text[]` instead of child tables. They are lists of
  short strings that are always read and written together with the profile.
  Two extra tables would buy nothing.
- `matches` is the whole feed, at every scope: one saved search, every saved
  search (`mine=1`, which also reports which searches caught a job), or the jobs
  you applied to. `created_at` orders arrivals, `seen_at` is the unread dot,
  `applied_at` is the applied view. One table, one query shape. This started as
  two screens with two queries and they drifted apart — the feed said a job was
  21 hours old while the notifications screen called it new.
- `q=` is one condition per word, ANDed, matched on word boundaries against the
  title (through the role dictionary), the location (through the place atlas)
  and the company name. It began as a single `ilike '%query%'`, which meant word
  order decided the result — "frontend engineer" found eleven jobs and "engineer
  frontend" none — "ontend engine" matched the middle of words, and any query
  spanning two fields ("dubai react") could never match anything. The boundaries
  are also what make short words safe: "qa" matches "QA Engineer" and not
  "Qatar", so no length rule is needed and "D4 Insight" still works.
- No `descriptions`. The Apply button goes to the real posting; storing
  megabytes of HTML to render a worse version of it is not worth it. The price is
  paid at the search bar: a skill named only in the body text — "typescript" in a
  posting titled "Frontend Engineer" — is not findable here, and the empty state
  says so rather than implying the market is empty. (Greenhouse
  drops from 4.4 MB to 360 KB per board once you stop asking for `content=true`.)
- `companies` has a composite natural key. No surrogate id needed since jobs
  denormalize the company display name.

Migrations: `golang-migrate` over plain `.sql` files in `migrations/`, embedded
into the binary and applied at startup — so there is no migrate CLI to install
and no separate step to forget.
Queries: hand-written SQL on a `pgx` pool, at the call site that needs it. There
is a small number of queries in this project, so keeping the SQL explicit beats
introducing code generation.

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
| Teamtailor | `{slug}.teamtailor.com/jobs.json` | `propertyfinder`, `fresha` |
| Careerjet | `search.api.careerjet.net/v4/query` (aggregator; slug is a saved search) | `software+engineer\|dubai\|en_AE` |
| Manatal | `api.manatal.com/open/v3/career-page/{slug}/jobs/` | `nathanhr` |
| Phenom | `POST https://{slug}/widgets` (slug is the careers host) | `careers.majidalfuttaim.com` |
| Oracle | `{host}/hcmRestApi/.../recruitingCEJobRequisitions` (slug is `host\|siteNumber`) | `esbe.fa.em8.oraclecloud.com\|CX_1001` |

| Provider | id | title | location | remote | url | posted |
|---|---|---|---|---|---|---|
| Greenhouse | `id` | `title` | `location.name` | location text contains "remote" | `absolute_url` | `first_published` |
| Lever | `id` | `text` | `categories.location` | `workplaceType == "remote"` | `hostedUrl` | `createdAt` (epoch **ms**) |
| Ashby | `id` | `title` | `location` | `isRemote` (nullable) | `jobUrl` | `publishedAt` |
| SmartRecruiters | `id` | `name` | `location.fullLocation` | `location.remote` | *constructed* (below) | `releasedDate` |
| Workable | `shortcode` | `title` | `city, country` | `telecommuting` | `url` | `published_on` (date only) |
| Recruitee | `id` | `title` | `location` | `remote` | `careers_url` | `published_at` (`"… UTC"` string) |
| Teamtailor | `id` | `title` | `_jobposting.jobLocation[].address` | location text only | `url` | `date_published` |

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
- **Teamtailor**: a JSON Feed whose items carry a schema.org JobPosting under
  `_jobposting`. `jobLocation` is a list (multi-city postings) and sometimes
  absent; countries are ISO codes, expanded to names so profiles can match them.
  Large boards paginate via `next_url`. No remote flag at all.
- **Manatal**: the Gulf recruitment agencies' ATS, so its boards are the
  closest thing to agency inventory a polite poller can reach. No posting date
  (jobs age from first sight), `organization_name` is a department rather than
  the company, and job pages live on Manatal's careers-page.com domain.
- **Phenom**: powers enterprise career sites, so the slug is the careers host
  itself and the "API" is the public POST /widgets search the site's own page
  makes. Pages with from/size against totalHits; no job URL in the payload (it
  is built from jobSeqNo); the company field sometimes holds a bare number and
  is dropped when it does.
- **Oracle Recruiting Cloud**: enterprise career sites (hotel groups, telcos,
  holdings) on a public REST API. Host is per-tenant and unguessable, so the
  slug is `host|siteNumber`. The finder's `;` and `,` are Oracle syntax and must
  not be percent-encoded; requisitions need `expand=requisitionList`; a browser
  User-Agent is required.
- **Careerjet**: the one aggregator, and different in kind. The slug is a saved
  search, not a company. It needs credentials (`CAREERJET_API_KEY`,
  `CAREERJET_SITE`), calls only work from IPs declared in its publisher
  dashboard, postings have no stable id (identity is a hash of title, company,
  location and date — the URLs are per-request tracking tokens), `page_size` is
  silently capped at twenty, and the poller visits it every six hours rather
  than every cycle.

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

Firebase Cloud Messaging over its HTTP v1 API — one authenticated JSON POST per
device, with `golang.org/x/oauth2` handling the token exchange. The Admin SDK
would pull in gRPC, OpenTelemetry and Firestore (fifty-odd modules) to make that
same POST. The app registers its token with `POST /api/devices`; the poller
sends to every stored token.

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

Chi, `encoding/json`, plain `http.HandlerFunc`. No DTO layer — the struct a
query scans into is the JSON shape.

```
GET    /api/profiles
POST   /api/profiles              {name, keywords[], locations[], remote_only}
PUT    /api/profiles/{id}
DELETE /api/profiles/{id}

GET    /api/jobs?profile_id=&before=&limit=50     matched jobs, newest first
GET    /api/jobs?mine=1                           every saved search's matches
GET    /api/jobs?q=                               the whole corpus
POST   /api/notifications/seen                    mark the arrivals seen

POST   /api/devices               {token, platform}
POST   /api/poll                  start a cycle; 202 at once, runs in the background
GET    /healthz                   database, plus poller state and the age of the last cycle
GET    /healthz
```

Cursor pagination on `before` (a timestamp), because offset pagination on a feed
that grows at the head shows duplicates. Ten extra lines, one real bug avoided.

No auth, per the brief. That means: bind to `127.0.0.1` at home, or reach it
through a private tunnel. Do not put this on a public IP.

---

## 8. Web app

React + Vite + TypeScript, hand-written CSS, no router (three tabs and a URL
hash), no state library (a sixty-line fetch cache with named keys), Firebase's
JS SDK for push. It ships as a PWA: Add to Home Screen gives an icon, a
full-screen app and push notifications on iOS — without an Apple Developer
account, which native push cannot do. The production bundle is ~80 KB gzipped.

This replaced a working Flutter implementation. Flutter's reason to exist here
was one codebase targeting Android, iOS and web; once web push proved out on a
real iPhone and the native builds were dropped, it was paying a 4–8 MB
CanvasKit tax and rendering to a canvas for what three list screens express
naturally in HTML.

```
web/src/
  main.tsx               entry: mount + service worker registration
  App.tsx                tabs, unread badge, inline SVG icons
  api.ts                 typed fetch functions, one per endpoint
  hooks.ts               the fetch cache: useQuery + invalidate
  push.ts                FCM init, gesture-driven enable, token registration
  format.ts              shortAgo, provider labels
  screens/               Jobs, Settings
  components/            JobRow, the loading/empty/error states
  styles.css             the whole design: tokens, dark-first, light variant
```

**Jobs** — the whole app. One row of chips names what you are looking at (all
searches, one search, applied), a search field covers every board, and one
Where button holds region, place and remote-only. Rows are two lines (title,
company · location · source) grouped under when they arrived. Looking at the
arrivals marks them seen; unread dots stay for that viewing.

**Settings** — searches CRUD in a bottom sheet, push status and quiet hours,
with a gesture-driven Enable button (iOS requires the permission request to
come from a tap). Board health, the device id and the backend URL sit behind
Advanced.

There is no Notifications screen. It showed the same matched jobs the feed
shows, and a reader had to guess which of the two lists to trust.

The one service worker does both jobs: shows pushes that arrive while no window
is focused, and keeps the app shell cached so it opens instantly and offline.

FCM push carries no payload beyond a nudge — the app invalidates the jobs
provider and refetches. Never trust a notification body as data you then store.

---

## 9. Layout & running it

```
job-pulse/
  cmd/jobpulse/main.go          flags, wiring, ticker + server
  internal/api/                 router.go, handlers.go
  internal/db/                  pgx pool + migration runner
  internal/providers/           providers.go + one file per provider
  internal/match/match.go
  internal/notify/fcm.go
  internal/poll/poll.go
  migrations/                   0001_init.up.sql / .down.sql
  companies.txt                 seed list: "greenhouse stripe"
  docker-compose.yml            postgres only
  Makefile                      up, run, test, seed
  web/                          the React PWA
```

```bash
git clone … && cd job-pulse
docker compose up -d          # postgres
make seed                     # load companies.txt
go run ./cmd/jobpulse         # migrates, serves :8080, polls every 15m
cd web && npm run dev
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
| 1 | Migrations, Greenhouse + Lever, poll loop, `/api/jobs`. Verify new-job detection against a real board. |
| 2 | Remaining four providers, matching, profiles CRUD, `companies.txt` seeding. |
| 3 | Web app: three screens, API client, theme. |
| 4 | FCM both ends, `seen_at` plumbing, README. |

Roughly 1,200 lines of Go and 900 of Dart. If it grows much past that, something
has been over-designed.

---

## 11. What I deliberately did NOT build

**Global job search / crawling.** No provider offers it, and getting it means
scraping aggregators or driving a headless browser. A curated `companies.txt` is
more accurate, never breaks, and matches how I actually job-hunt.

**Identity without accounts (added after real use).** Every device mints an
anonymous UUID on first launch and sends it as X-Device; profiles, matches,
notifications and push routing all belong to that id. Two of the author's own
devices sharing one profile list turned out to be wrong in practice, and this
is the smallest fix: no passwords, no sessions, no users table — a column and
WHERE clauses. Anyone with the URL gets their own empty space. What it is not:
security. A device id is not a secret worth stealing (it unlocks that device's
own search profiles), and anyone who wants "real" multi-user still runs their
own copy.

**Authentication and user accounts.** Single user. Auth would mean a users
table, token issuance, refresh, password reset, and a foreign key on every
table — for one person. Bind to localhost instead. If I ever share it with a
friend, they run their own copy; that is one `git clone`, not a multi-tenancy
migration.

**A provider plugin system.** Registry interfaces, dynamic loading, per-provider
config schemas — all to avoid editing a 20-line map. Six providers, one
`map[string]Provider`. A seventh is one file plus one line.

**Repository pattern / service layer / hexagonal boundaries.** Handlers and the
poller take the `pgx` pool and run their own SQL, so every query is visible where
it is used. Wrapping that in interfaces so I can swap Postgres for a database I
will never use is pure ceremony.

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

**Scrapers for bot-walled boards.** Workday's public JSON API was investigated:
its reachable tenants are global-corporate boards with no Gulf inventory, while
the Gulf-specific tenants (talabat, regional arms of the big firms) sit behind
edge bot-shields that only a headless browser defeats. Driving a browser to
defeat an anti-automation shield is the browser automation this project refuses
on principle, and the reachable-but-irrelevant tenants were left out as noise.

**Docker image for the backend, CI/CD, Kubernetes manifests.** `go run` locally,
`go build` on the box. Compose exists only to hand me a Postgres.

**Tests for everything.** I will write table-driven tests for `match.Matches`
and one golden-file test per provider parser (a saved JSON fixture → expected
`[]Job`), because those are where bugs are silent and expensive. Not handler
tests, not an integration harness, not a mock HTTP layer.

**A frontend framework's runtime.** Flutter web shipped megabytes of CanvasKit
to draw three screens; the React replacement is ~80 KB gzipped and renders real
HTML. No Next.js either: nothing here needs a server or SSR — Firebase Hosting
serves static files.

**Native iOS and Android builds.** The PWA delivers the whole product — icon,
full screen, push — and the native route demanded an Apple Developer
subscription for push, weekly re-signing on a free account, and a Gradle build,
all to end at the same three screens. Dropped after web push worked on a real
iPhone.

**AI anything.** No embeddings for "semantic" matching, no LLM job scoring, no
resume tailoring, no cover letters. It would add an API key, latency, cost, and
non-determinism to a problem that `strings.Contains` solves correctly.
