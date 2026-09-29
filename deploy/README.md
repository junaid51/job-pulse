# Running JobPulse on Northflank

The current host. Chosen because it is the one free, always-on platform whose
meter points the right way for this app: **ingress is free**, and JobPulse is
almost entirely ingress — it downloads job boards all day and sends a phone a
few kilobytes. Render billed those downloads as bandwidth and suspended the
workspace at 5 GB; Northflank does not count them.

The free **Developer Sandbox** gives two always-on services. One is enough.

## What it costs

- Sandbox plan and the free `nf-compute-10` service (0.1 vCPU, 256 MB): **$0**.
- The card is for verification only.
- Egress is listed at $0.06/GB with no sandbox exception written down either
  way. Ours is request headers, TLS handshakes and API responses — roughly
  1–3 GB a month, so the worst case is cents. Check **Billing** a day after the
  first deploy: nothing is charged until the end of the cycle, so if it is not
  $0.00 there is time to delete the service and pay nothing.
- **Never upgrade to Pay as you go.** Never pick a compute plan other than the
  free one.

Measured before choosing: a cold cycle over 213 boards peaked at 47 MB of
memory under a hard 256 MB limit.

## Setup

1. Sign up at <https://app.northflank.com/signup> with GitHub, so repository
   access comes with the account.
2. New project `jobpulse`, region **US - Central** (Council Bluffs). The
   sandbox offers only US - Central and London; US - West needs Pay as you go.
   The database is Supabase in Oregon, about 40 ms from Iowa and 140 ms from
   London, and a request that makes several queries pays that each time.
3. Create → Service → **Combined service**:
   - repository `junaid51/job-pulse`, branch `main`
   - build with **Dockerfile**, path `/Dockerfile`, context `/`
   - port `8080`, HTTP, publicly exposed
   - compute `nf-compute-10`, one instance
4. Runtime variables:
   - `DATABASE_URL` — Supabase session pooler URI
   - `POLL_TOKEN` — must match the GitHub secret `JOBPULSE_POLL_TOKEN`
   - `POLL_INTERVAL` — `5m`
   - `APP_URL` — `https://jobpulse-junaid.web.app`
   - `GOOGLE_APPLICATION_CREDENTIALS` — the service account itself, since there
     is no disk to mount a file on. Paste it **base64-encoded**, which leaves an
     environment editor nothing to damage:
     `base64 -i <the adminsdk .json> | tr -d '\n' | pbcopy`. Raw JSON works
     too. Afterwards `/healthz` must say `"push": "ok"`; anything else names
     what went wrong.
   - `CAREERJET_API_KEY`, `CAREERJET_SITE`, `JOBSPIPE_API_KEY`,
     `JOBVEN_API_KEY` — blank is allowed; that provider is skipped.

Every push to `main` rebuilds and redeploys.

## Cut over

Once `https://<service>.code.run/healthz` answers with `database: ok` and
`push: ok`:

1. `web/.env.production` → `VITE_JOBPULSE_API=<the code.run URL>`, then from
   `web/`: `npm run build`, grep the bundle for the new URL, `firebase deploy
   --only hosting`, and load the deployed site to see real jobs render.
2. GitHub secret `JOBPULSE_URL` → the new URL (the scout and the hourly alarm
   read it).
3. The Supabase waker, in the SQL editor. The service no longer sleeps, so this
   is a second pair of hands rather than the thing that keeps it alive:

   ```sql
   select cron.unschedule('jobpulse-wake');
   select cron.schedule('jobpulse-wake', '*/5 * * * *', $$
     select net.http_get(url := '<the code.run URL>/healthz',
                         timeout_milliseconds := 60000)
   $$);
   ```
4. `POLL_TOKEN=… scripts/smoke.sh <the code.run URL>`

## Known gap: Careerjet

Careerjet's key is locked to declared IPs, and a free Northflank service has no
fixed outbound address: it sends from Google Cloud's us-central1 pool, which is
107 ranges and about five million addresses. On 2026-09-29 six requests arrived
from six addresses in six different ranges, so declaring addresses one at a
time never catches up. Declare the **range** instead — the dashboard accepts
CIDRs — using the block each refusal names:

    careerjet 403: Unauthorized access from IP <address>   (companies.last_error)

Look the address up in <https://www.gstatic.com/ipranges/cloud.json> (scope
`us-central1`) and add its prefix. Declared so far: `34.41.0.0/16`,
`136.112.0.0/14`, `34.55.0.0/16`, `35.253.0.0/16`, `35.222.0.0/15`,
`34.9.0.0/16`. The key stays secret either way; the IP lock is only a second
fence. Worth the trouble: measured on 2026-09-03, 94% of Careerjet's postings
were on none of the other boards.
