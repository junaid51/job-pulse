#!/usr/bin/env bash
# Exercises every user-facing action against a running JobPulse, as a device
# with no history, and cleans up after itself.
#
# It exists because the X on a job row answered 404 for every search result for
# an unknown length of time: the unit tests were green, the feed rendered, and
# nothing anywhere pressed the button. A test that renders is not a test that
# works.
#
#   scripts/smoke.sh                          # against localhost:8091
#   scripts/smoke.sh https://…onrender.com    # against production
set -uo pipefail

API="${1:-http://localhost:8091}"
DEV="smoke-$(date +%s)-$$"
pass=0 fail=0

ok() { printf '  \033[32m✓\033[0m %s\n' "$1"; pass=$((pass + 1)); }
no() { printf '  \033[31m✗\033[0m %s\n' "$1"; fail=$((fail + 1)); }
check() { # check <description> <expected> <actual>
  [ "$2" = "$3" ] && ok "$1" || no "$1 — wanted $2, got $3"
}

req() { # req <method> <path> [body] → status code
  local method=$1 path=$2 body=${3:-}
  if [ -n "$body" ]; then
    curl -s -o /tmp/smoke.out -w '%{http_code}' -X "$method" "$API$path" \
      -H "X-Device: $DEV" -H 'Content-Type: application/json' -d "$body"
  else
    curl -s -o /tmp/smoke.out -w '%{http_code}' -X "$method" "$API$path" -H "X-Device: $DEV"
  fi
}
field() { python3 -c "import json;print(json.load(open('/tmp/smoke.out'))$1)" 2>/dev/null; }
count() { python3 -c "import sys,json;print(len(json.load(open('/tmp/smoke.out'))['jobs']))" 2>/dev/null; }
has_job() { python3 -c "
import sys,json
print('yes' if any(j['id']==$1 for j in json.load(open('/tmp/smoke.out'))['jobs']) else 'no')" 2>/dev/null; }

echo "smoke: $API as $DEV"

echo "health and boards"
check "GET /healthz" 200 "$(req GET /healthz)"
check "  database ok" ok "$(field "['database']")"
check "GET /api/boards" 200 "$(req GET /api/boards)"
check "  boards are listed" yes "$(python3 -c "
import json;print('yes' if len(json.load(open('/tmp/smoke.out'))['boards']) > 50 else 'no')")"

echo "saved searches"
check "POST /api/profiles" 201 "$(req POST /api/profiles '{"name":"Smoke","keywords":["engineer"],"locations":[],"remote_only":false}')"
PID=$(field "['profile']['id']")
check "  it matched something immediately" yes "$([ "$(field "['matched']")" -gt 0 ] 2>/dev/null && echo yes || echo no)"
check "GET /api/profiles" 200 "$(req GET /api/profiles)"
check "PUT /api/profiles/:id" 200 "$(req PUT "/api/profiles/$PID" '{"name":"Smoke","keywords":["engineer","-senior"],"locations":[],"remote_only":false}')"

echo "feeds"
check "one search's feed" 200 "$(req GET "/api/jobs?profile_id=$PID&sort=matched&limit=5")"
MATCHED=$(python3 -c "import json;print(json.load(open('/tmp/smoke.out'))['jobs'][0]['id'])")
check "every search at once (mine=1)" 200 "$(req GET '/api/jobs?mine=1&sort=matched&limit=5')"
check "the whole corpus (q=)" 200 "$(req GET '/api/jobs?q=engineer&limit=5')"
check "a widened saved search (keyword=)" 200 "$(req GET '/api/jobs?keyword=engineer&limit=5')"
check "an @place filter" 200 "$(req GET '/api/jobs?q=engineer&location=dubai&limit=5')"
check "the Gulf + India filter" 200 "$(req GET '/api/jobs?q=engineer&market=1&limit=5')"
check "remote only" 200 "$(req GET '/api/jobs?q=engineer&remote=1&limit=5')"
check "what I applied to" 200 "$(req GET '/api/jobs?sort=applied&limit=5')"
check "a place is a whole word" 0 "$(req GET '/api/jobs?location=india&limit=200' >/dev/null; python3 -c "
import json,re
print(sum(1 for j in json.load(open('/tmp/smoke.out'))['jobs'] if re.search(r'indiana', j['location'], re.I)))")"
check "an unknown sort is refused" 400 "$(req GET '/api/jobs?profile_id='"$PID"'&sort=nonsense')"
# Someone else's profile id is answered with an empty feed rather than a 404,
# which tells the caller nothing about whether it exists. That is the intent.
check "another device's profile shows nothing" 0 "$(req GET '/api/jobs?profile_id=999999999' >/dev/null; count)"

echo "the row actions — the ones that were dead"
UNMATCHED=$(req GET '/api/jobs?q=warehouse&limit=40' >/dev/null; python3 -c "
import json
for j in json.load(open('/tmp/smoke.out'))['jobs']:
    if not j.get('matched_by'):
        print(j['id']); break")
check "hide a matched job" 204 "$(req POST "/api/jobs/$MATCHED/hide")"
check "  gone from its feed" no "$(req GET "/api/jobs?profile_id=$PID&sort=matched&limit=200" >/dev/null; has_job "$MATCHED")"
check "  gone from search too" no "$(req GET '/api/jobs?q=engineer&limit=200' >/dev/null; has_job "$MATCHED")"
check "undo the hide" 204 "$(req POST "/api/jobs/$MATCHED/unhide")"
check "  back in its feed" yes "$(req GET "/api/jobs?profile_id=$PID&sort=matched&limit=200" >/dev/null; has_job "$MATCHED")"
if [ -n "$UNMATCHED" ]; then
  check "hide a job no search caught" 204 "$(req POST "/api/jobs/$UNMATCHED/hide")"
  check "  gone from search" no "$(req GET '/api/jobs?q=warehouse&limit=200' >/dev/null; has_job "$UNMATCHED")"
  check "mark that job applied" 200 "$(req POST "/api/jobs/$UNMATCHED/applied")"
  check "  it is in Applied even though it is hidden" yes "$(req GET '/api/jobs?sort=applied&limit=50' >/dev/null; has_job "$UNMATCHED")"
  check "unmark it" 200 "$(req POST "/api/jobs/$UNMATCHED/applied")"
  check "  and Applied is empty again" 0 "$(req GET '/api/jobs?sort=applied&limit=50' >/dev/null; count)"
fi
check "hide a job that does not exist" 404 "$(req POST '/api/jobs/999999999/hide')"
check "mark applied on a matched job" 200 "$(req POST "/api/jobs/$MATCHED/applied")"
check "  editing the search keeps it" yes "$(req PUT "/api/profiles/$PID" '{"name":"Smoke","keywords":["nothingmatchesthis"],"locations":[],"remote_only":false}' >/dev/null
  req GET '/api/jobs?sort=applied&limit=50' >/dev/null; has_job "$MATCHED")"

echo "notifications and devices"
check "POST /api/notifications/seen" 200 "$(req POST /api/notifications/seen)"
check "GET device status" 200 "$(req GET /api/devices/status)"
check "  no token for a fresh device" False "$(field "['registered']")"
# 404 with a notifier configured, 503 without one (a local run has no service
# account), and both are the honest answer.
code=$(req POST /api/devices/test)
check "a test push without a token is refused" yes "$([ "$code" = 404 ] || [ "$code" = 503 ] && echo yes || echo no)"
check "quiet hours can be set" 200 "$(req PUT /api/devices/quiet-hours '{"from":0,"to":8}')"
check "a bad quiet window is refused" 400 "$(req PUT /api/devices/quiet-hours '{"from":99,"to":8}')"
check "POST /api/devices needs a token" 400 "$(req POST /api/devices '{"platform":"web"}')"

echo "polling"
# 202 locally, 401 in production, where the endpoint carries POLL_TOKEN. Both
# mean the request was answered without waiting for a cycle, which is the
# property that matters: a poll that ran inside the request once died on a
# scheduler's 30-second timeout for nineteen hours.
code=$(req POST /api/poll)
check "POST /api/poll answers at once" yes "$([ "$code" = 202 ] || [ "$code" = 401 ] && echo yes || echo no)"

echo "cleanup"
check "DELETE /api/profiles/:id" 204 "$(req DELETE "/api/profiles/$PID")"
check "  and its feed is empty" 0 "$(req GET "/api/jobs?profile_id=$PID" >/dev/null; count)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
