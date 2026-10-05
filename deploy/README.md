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

Careerjet's key works only from declared addresses, the dashboard holds eight,
and it takes single addresses, not ranges. A free Northflank service has no
fixed outbound IP — but it does not rotate per request either. Measured on
2026-10-05: sixty fresh connections, to two different echo services, all left
from one address; and every Careerjet refusal on record lines up with a deploy.
**The address is fixed for the life of a deployment and changes when the
service is redeployed**, which by default is every push to `main`. One
deployment ran from `136.111.77.112` for five days; ten pushes in an hour on
2026-09-29 produced ten addresses. Northflank does sometimes place a new
deployment on a machine it used before.

So:

- **Redeploy only for backend changes.** Service → Build options → Advanced
  build settings → path rules, as an allow-list:

      cmd/
      internal/
      migrations/
      go.mod
      go.sum
      Dockerfile
      companies.txt

  Web, docs and workflow commits then leave the running deployment — and its
  address — alone.
- **After a backend deploy, ask the server where it now sends from:**

      curl -H "Authorization: Bearer $POLL_TOKEN" https://<service>.code.run/api/egress

  and if that address is not declared, replace the oldest of the eight with it.
  A refusal names the address too — `careerjet 403: Unauthorized access from
  IP <address>` in `companies.last_error` — but only after a search has failed.

Worth the trouble: on 2026-09-03, 94% of Careerjet's postings were on none of
the other boards, and its jobs do reach the phone.
