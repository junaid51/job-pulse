#!/usr/bin/env bash
# Provisions a fresh Ubuntu box to run JobPulse: PostgreSQL, the binary under
# systemd, and Caddy terminating TLS. Idempotent — safe to re-run after a
# failure, and the way to apply a change is to edit and run it again.
#
#   sudo ./setup.sh jobpulse-junaid.duckdns.org
#
# It deliberately does not handle secrets. /etc/jobpulse/env is created with a
# generated database password and placeholders; fill in the API keys there and
# restart. The Firebase service account is copied separately, by scp.
set -euo pipefail

HOSTNAME_ARG="${1:?usage: setup.sh <hostname>}"
GO_VERSION="1.26.0"
REPO="https://github.com/junaid51/job-pulse"
APP_DIR="/opt/jobpulse"
ENV_FILE="/etc/jobpulse/env"

log() { printf '\n=== %s\n' "$*"; }

log "swap (Postgres and a Go build are unhappy in 1 GB without it)"
if [ ! -f /swapfile ]; then
  fallocate -l 2G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
fi

log "packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq postgresql postgresql-contrib git curl ca-certificates \
  debian-keyring debian-archive-keyring apt-transport-https iptables-persistent

log "opening 80 and 443 in the local firewall (Oracle images ship them shut)"
# The VCN security list is the other half of this and is set in the console.
for port in 80 443; do
  iptables -C INPUT -p tcp --dport "$port" -m state --state NEW -j ACCEPT 2>/dev/null ||
    iptables -I INPUT 6 -p tcp --dport "$port" -m state --state NEW -j ACCEPT
done
netfilter-persistent save

log "Go ${GO_VERSION}"
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${GO_VERSION}"; then
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture).tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tgz
  rm /tmp/go.tgz
fi

log "database"
systemctl enable --now postgresql
if ! sudo -u postgres psql -tAc "select 1 from pg_roles where rolname='jobpulse'" | grep -q 1; then
  DB_PASSWORD="$(openssl rand -hex 24)"
  sudo -u postgres psql -qc "create role jobpulse login password '${DB_PASSWORD}'"
  sudo -u postgres psql -qc "create database jobpulse owner jobpulse"
  mkdir -p /etc/jobpulse
  cat >"$ENV_FILE" <<ENV
# Filled in by setup.sh. Restart after editing: systemctl restart jobpulse
DATABASE_URL=postgres://jobpulse:${DB_PASSWORD}@127.0.0.1:5432/jobpulse?sslmode=disable
PORT=8080
POLL_INTERVAL=5m
COMPANIES_FILE=${APP_DIR}/companies.txt
APP_URL=https://jobpulse-junaid.web.app
GOOGLE_APPLICATION_CREDENTIALS=/etc/jobpulse/firebase-service-account.json
POLL_TOKEN=$(openssl rand -hex 16)
CAREERJET_API_KEY=
CAREERJET_SITE=https://jobpulse-junaid.web.app/
JOBVEN_API_KEY=
JOBSPIPE_API_KEY=
ENV
  chmod 600 "$ENV_FILE"
  echo "wrote ${ENV_FILE} — add the API keys before the first poll"
fi

log "application user and build"
id -u jobpulse >/dev/null 2>&1 || useradd --system --home "$APP_DIR" --shell /usr/sbin/nologin jobpulse
if [ -d "$APP_DIR/.git" ]; then
  git -C "$APP_DIR" fetch --quiet origin main && git -C "$APP_DIR" reset --quiet --hard origin/main
else
  git clone --quiet "$REPO" "$APP_DIR"
fi
cd "$APP_DIR"
/usr/local/go/bin/go build -o "$APP_DIR/jobpulse" ./cmd/jobpulse
chown -R jobpulse:jobpulse "$APP_DIR"

log "service"
install -m 644 "$APP_DIR/deploy/jobpulse.service" /etc/systemd/system/jobpulse.service
systemctl daemon-reload
systemctl enable --now jobpulse
systemctl restart jobpulse

log "TLS via Caddy for ${HOSTNAME_ARG}"
if ! command -v caddy >/dev/null; then
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' |
    gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
    >/etc/apt/sources.list.d/caddy-stable.list
  apt-get update -qq && apt-get install -y -qq caddy
fi
cat >/etc/caddy/Caddyfile <<CADDY
# Automatic Let's Encrypt certificate for the DuckDNS name, reverse-proxying
# to the service on loopback. Caddy renews on its own.
${HOSTNAME_ARG} {
	encode gzip
	reverse_proxy 127.0.0.1:8080
}
CADDY
systemctl restart caddy

log "nightly backup"
install -m 755 "$APP_DIR/deploy/backup.sh" /usr/local/bin/jobpulse-backup
cat >/etc/systemd/system/jobpulse-backup.service <<'UNIT'
[Unit]
Description=JobPulse database backup
[Service]
Type=oneshot
EnvironmentFile=/etc/jobpulse/env
ExecStart=/usr/local/bin/jobpulse-backup
UNIT
cat >/etc/systemd/system/jobpulse-backup.timer <<'UNIT'
[Unit]
Description=Nightly JobPulse backup
[Timer]
OnCalendar=*-*-* 02:30:00
Persistent=true
[Install]
WantedBy=timers.target
UNIT
systemctl daemon-reload
systemctl enable --now jobpulse-backup.timer

log "done"
systemctl --no-pager --lines=5 status jobpulse || true
echo "health: curl -s https://${HOSTNAME_ARG}/healthz"
