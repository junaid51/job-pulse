#!/usr/bin/env bash
# Nightly backup. Two destinations, on purpose:
#
#   1. A local dump, kept a week, for the ordinary "undo the last hour" case.
#   2. A restore into the old Neon database, which is still free and still
#      wired to the old Render deployment. That makes the backup off-box, in a
#      managed service with its own backups, and doubles as a warm standby:
#      rolling back is rebuilding the web app against the old API, nothing more.
#
# NEON_URL is optional; without it only the local dump runs.
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
DEST="/var/backups/jobpulse"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"

mkdir -p "$DEST"
pg_dump --clean --if-exists "$DATABASE_URL" | gzip >"${DEST}/jobpulse-${STAMP}.sql.gz"
find "$DEST" -name 'jobpulse-*.sql.gz' -mtime +7 -delete

if [ -n "${NEON_URL:-}" ]; then
  # --clean --if-exists so the mirror is a replacement, not an accumulation.
  gunzip -c "${DEST}/jobpulse-${STAMP}.sql.gz" | psql --quiet "$NEON_URL" >/dev/null
  echo "mirrored to neon"
fi

echo "backup ${STAMP} ok ($(du -h "${DEST}/jobpulse-${STAMP}.sql.gz" | cut -f1))"
