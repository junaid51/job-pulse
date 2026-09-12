# Running JobPulse on Oracle Cloud Always Free

Render suspended the backend for using 5 GB of bandwidth in a month. JobPulse
*downloads* about 29 GB a month — Render bills that; almost nobody else does.
Oracle bills outbound only, and gives 10 TB of it free. Inbound is free and
uncounted, so the thing that killed us on Render is the thing Oracle does not
charge for at all.

---

## The money rules

The card is required for identity verification and is not billed.

> "Your credit card will not be charged unless you upgrade your account."
> — [Oracle Cloud Infrastructure Free Tier](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier.htm)

At sign-up Oracle places a **$1 authorization** on the card and reverses it
immediately. That is a hold, not a charge.

Four rules keep it that way. They are the whole safety story:

1. **Never click "Upgrade to Pay As You Go"**, "Upgrade Now", or any upgrade
   banner. This is the only action that can ever produce a bill. The console
   will offer it repeatedly; ignore it every time.
2. **Only create resources marked "Always Free-eligible"** — the console puts a
   green badge on the shape, the image and the boot volume size. If the badge is
   missing, the resource is a paid one running on trial credits, and it gets
   reclaimed at day 30 anyway.
3. On a Free Tier account that has not been upgraded, **Oracle refuses to
   provision beyond the Always Free limits.** You cannot accidentally overspend;
   you get an error instead. The $300 trial credit is a ceiling you never reach,
   not a balance you can run down into debt.
4. **Set a $1 budget alert** anyway (step 8). If it ever fires, something is
   wrong and you have email about it the same day.

What we will use, against what Always Free allows:

| | Used | Allowed |
|---|---|---|
| Compute | 1 OCPU, 6 GB Ampere A1 (744 OCPU-hours/month) | 1,500 OCPU-hours, 9,000 GB-hours |
| Storage | 50 GB boot volume | 200 GB total |
| Outbound transfer | a few GB (API responses to one phone) | 10 TB/month |
| Inbound transfer | ~29 GB/month of job boards | free, unmetered |

After the 30-day trial expires without upgrading, Always Free resources
**continue running indefinitely** — only paid resources are reclaimed, and we
create none.

---

## 1. Sign up

<https://cloud.oracle.com/free>

**The home region is permanent and cannot be changed**, and Always Free
resources only exist in it. Pick **US West (San Jose)** or **US West (Phoenix)** —
the database is Supabase in Oregon, and a request that makes several queries
pays the round trip each time, so the server wants to sit next to the database
rather than next to you.

Finish sign-up, sign in, and stop. Do not accept any offer to upgrade.

## 2. Make an SSH key

On the Mac:

```sh
ssh-keygen -t ed25519 -f ~/.ssh/jobpulse-oracle -C jobpulse
pbcopy < ~/.ssh/jobpulse-oracle.pub
```

## 3. Create the instance

Console → **Compute → Instances → Create instance**.

- **Image**: Canonical Ubuntu 24.04 — pick the **aarch64** build.
- **Shape**: Change shape → **Ampere** → `VM.Standard.A1.Flex` → **1 OCPU,
  6 GB memory**. Confirm the *Always Free-eligible* badge is showing.
- **Networking**: create a new VCN, and leave **Assign a public IPv4 address**
  on.
- **SSH keys**: paste the public key you just copied.
- **Boot volume**: 50 GB, default (balanced) performance.

Create.

> **"Out of host capacity"** is the common failure here — Ampere A1 is in
> demand and free accounts are served last. Try each availability domain in the
> region, then retry in a few hours; capacity frees up constantly. If it will
> not come, fall back to `VM.Standard.E2.1.Micro` (x86, 1/8 OCPU, 1 GB), which
> is always available. The Dockerfile builds inside the image, so it works on
> x86 unchanged — but add swap first or the Go build will run the 1 GB box out
> of memory:
> `sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile && sudo mkswap /swapfile && sudo swapon /swapfile && echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab`

## 4. Pin the public address

Instance page → **Attached VNICs** → the primary VNIC → **IPv4 Addresses** →
the three dots on the public IP → **Edit** → **Reserved IP address** → assign a
new reserved IP.

Do this before anything else points at the machine. Careerjet allow-lists the
server's address and DuckDNS resolves to it; an ephemeral IP is released the
moment the instance is terminated, and re-registering with Careerjet is not a
thing you want to discover later. Reserved IPs attached to an Always Free
instance are free.

Note the address down — it is `SERVER_IP` from here on.

## 5. Open 80 and 443 — in both firewalls

Oracle has two, and forgetting the second one is the classic half-day of
debugging: the port is open at the cloud edge and the machine itself still
drops the packet.

**a. The security list.** Instance → the VCN link → **Security Lists** →
the default list → **Add Ingress Rules**. Add two, both:

- Source CIDR `0.0.0.0/0`, IP Protocol `TCP`, Destination Port `80`
- the same for port `443`

**b. The machine's own iptables.** Oracle's Ubuntu image ships a rule set that
drops everything except SSH.

```sh
ssh -i ~/.ssh/jobpulse-oracle ubuntu@SERVER_IP

sudo iptables -I INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 443 -j ACCEPT
sudo netfilter-persistent save      # apt-get install -y iptables-persistent if missing
```

## 6. A hostname, so there can be a certificate

The web app is served over HTTPS from Firebase, and a browser will not let an
HTTPS page call a plain-HTTP API. Caddy issues and renews a Let's Encrypt
certificate on its own, but Let's Encrypt will only certify a name.

Free name: <https://www.duckdns.org> — sign in with GitHub, create
`jobpulse-junaid`, set its IP to `SERVER_IP`. The host is then
`jobpulse-junaid.duckdns.org`.

Check it resolves before going further: `dig +short jobpulse-junaid.duckdns.org`

## 7. Install Docker and bring it up

On the server:

```sh
sudo apt-get update && sudo apt-get install -y ca-certificates curl git
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo tee /etc/apt/keyrings/docker.asc >/dev/null
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
  https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker $USER
exit                      # log back in so the group membership takes effect
```

Then:

```sh
ssh -i ~/.ssh/jobpulse-oracle ubuntu@SERVER_IP
git clone https://github.com/junaid51/job-pulse.git
cd job-pulse/deploy
cp .env.example .env
nano .env                 # fill in every value — see below
```

`.env` wants the values the old host had:

- `DATABASE_URL` — Supabase → Project Settings → Database → **Session pooler**
  URI (IPv4; the direct connection is IPv6-only)
- `POLL_TOKEN` — the same token the waker uses
- `JOBPULSE_HOST` — `jobpulse-junaid.duckdns.org`
- `APP_URL` — `https://jobpulse-junaid.web.app`
- `CAREERJET_API_KEY`, `CAREERJET_SITE`, `JOBSPIPE_API_KEY`, `JOBVEN_API_KEY`
  — copy from the Render dashboard before deleting the service. A provider with
  a blank key is skipped, not an error.

The Firebase service account is the one real secret and is not in git. From the
Mac:

```sh
scp -i ~/.ssh/jobpulse-oracle firebase-service-account.json \
    ubuntu@SERVER_IP:~/job-pulse/deploy/firebase-service-account.json
```

and on the server `chmod 600 deploy/firebase-service-account.json`.

Start it:

```sh
docker compose up -d --build
docker compose logs -f
```

The first boot runs the migrations, loads `companies.txt`, and Caddy fetches a
certificate over port 80. If the certificate never arrives, port 80 is blocked —
go back to step 5b.

## 8. The $1 tripwire

Console → **Billing & Cost Management → Budgets → Create Budget**, scope the
tenancy, monthly amount `1`, alert rule at 100% of actual spend, to your email.
It should never fire. If it does, something got provisioned that is not Always
Free — find it and delete it.

## 9. Cut over

In this order, so nothing points at a host that is not answering yet.

**a. Prove the new one works.**

```sh
curl -s https://jobpulse-junaid.duckdns.org/healthz
```

Expect `database: ok` and a `poller` that is not `never ran`.

**b. Careerjet.** Register `SERVER_IP` in the Careerjet dashboard's IP
allow-list, and drop the Render address. Until this is done Careerjet returns
nothing.

**c. The web app.** On the Mac, in `web/.env.production`, set
`VITE_JOBPULSE_API=https://jobpulse-junaid.duckdns.org`, then from `web/`:

```sh
npm run build
grep -o "https://jobpulse-junaid.duckdns[a-z.-]*" dist/assets/index-*.js   # verify before deploying
firebase deploy --only hosting
```

Load the deployed URL and confirm real jobs render. The service worker serves
the previous shell for a moment — a hard reload settles it.

**d. The waker.** In the Supabase SQL editor:

```sql
select cron.unschedule('jobpulse-wake');
select cron.schedule('jobpulse-wake', '*/5 * * * *', $$
  select net.http_get(
    url := 'https://jobpulse-junaid.duckdns.org/healthz',
    timeout_milliseconds := 60000)
$$);
```

**e. GitHub.** Repository → Settings → Secrets → update `JOBPULSE_URL` to the
new host, so the scout and the CI alarm talk to the right server.

**f. Smoke test.**

```sh
POLL_TOKEN=… ./scripts/smoke.sh https://jobpulse-junaid.duckdns.org
```

**g. Only now, delete the Render service.** It is suspended, so nothing is
double-polling in the meantime — but keep it until the smoke test is green, in
case a value needs reading back out of its dashboard.

## 10. Keeping it

- **Idle reclamation.** Oracle may reclaim an Always Free instance whose CPU,
  network *and* memory all stay under 20% across a 7-day window. Polling 230
  boards every five minutes keeps the network busy, so this should not bite —
  but if the poller ever stops for a week, the machine can disappear with it.
- **Account activity.** Always Free resources stay as long as the account is
  used within 60 days. Signing in counts.
- **Updates.** `git pull && docker compose up -d --build` on the server. There
  is no auto-deploy on push any more; that was a Render feature.
- **Certificates** live in the `caddy-data` volume. Do not
  `docker compose down -v` — that deletes them and Let's Encrypt rate-limits
  re-issues.

## 11. What this unlocks

The bandwidth work — conditional requests, dropping Workable's `details=true`,
the fifteen-minute cadence for boards that cannot be checked cheaply — was done
to survive a 5 GB cap that no longer exists. The conditional requests are worth
keeping regardless, since a 304 is free for everyone. But the throttles are not:
on a host where downloads cost nothing, Workable and Recruitee can go back to
being read every five minutes, which is the difference between seeing a job at
once and seeing it a quarter of an hour late.

Do that after the move is settled, not during it.
