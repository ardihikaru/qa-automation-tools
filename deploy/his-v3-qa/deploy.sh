#!/usr/bin/env bash
# Deploy / update qa-automation on rnd-v3-vpn. Run ON rnd-v3-vpn:
#   deploy.sh              # pull current branch (ff-only) and deploy; fails if pull fails
#   deploy.sh <sha|tag>    # ROLLBACK / pin: deploy exactly that commit (detached HEAD)
#   deploy.sh --branch b   # roll forward: return to branch b, pull, deploy
#   NO_PULL=1 deploy.sh    # deploy the checked-out branch as-is (code shipped via git bundle)
# Every deploy is appended to deploy/his-v3-qa/deploys.log (find the previous sha there).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
COMPOSE=(docker compose -f "$HERE/docker-compose.prod.yml" --env-file "$HERE/.env.prod")
LOG="$HERE/deploys.log"

[[ -f "$HERE/.env.prod" ]] || { echo "missing $HERE/.env.prod (copy .env.prod.example)"; exit 1; }
[[ "$(stat -c %a "$HERE/.env.prod")" == "600" ]] || chmod 600 "$HERE/.env.prod"

cd "$REPO"
case "${1:-}" in
  --branch)
    git fetch --quiet origin
    git checkout --quiet "${2:?usage: deploy.sh --branch <name>}"
    git pull --ff-only --quiet
    ;;
  "")
    if ! git symbolic-ref -q HEAD >/dev/null; then
      echo "ERROR: HEAD is detached at $(git rev-parse --short HEAD) (previous rollback)."
      echo "       Roll forward with: deploy.sh --branch main   (or deploy.sh <sha> to stay pinned)"
      exit 1
    fi
    if [[ "${NO_PULL:-}" == "1" ]]; then
      echo "NO_PULL=1: deploying the checked-out branch without pulling"
    else
      git pull --ff-only --quiet || { echo "ERROR: git pull --ff-only failed; resolve, deploy an explicit <sha>, or NO_PULL=1 for bundle-shipped code"; exit 1; }
    fi
    ;;
  *)
    git fetch --quiet origin || true
    git checkout --quiet "$1"
    ;;
esac
SHA="$(git rev-parse --short HEAD)"
REF="$(git symbolic-ref -q --short HEAD || echo detached)"
echo "deploying $SHA ($REF)"

"${COMPOSE[@]}" up -d --build --remove-orphans
docker image prune -f >/dev/null

# shellcheck disable=SC1090
source <(grep -E '^HISV3QA_(BIND_IP|PORT)=' "$HERE/.env.prod")
BASE="http://${HISV3QA_BIND_IP:-10.10.0.9}:${HISV3QA_PORT:-8140}"

# Health gate: liveness is not enough. The DB must be connected (otherwise cookie auth
# is DISABLED and the API is open) and an anonymous /api call must be rejected.
ok=""
for _ in $(seq 1 30); do
  if out="$(curl -fsS -m 5 "$BASE/health" 2>/dev/null)"; then
    if [[ "$out" == *'"database":"connected"'* ]]; then ok=1; break; fi
  fi
  sleep 2
done
auth_code="$(curl -s -o /dev/null -m 5 -w '%{http_code}' "$BASE/api/auth/me" || true)"
echo "health: ${out:-<none>}"
echo "anonymous /api/auth/me: $auth_code"

if [[ -n "$ok" && "$auth_code" == "401" ]]; then
  [[ "$out" == *'"ai":"ok"'* ]] || echo "warn: ai-manager not ok (AI features degraded, deploy kept)"
  [[ "$out" == *'"executor":"ok"'* ]] || echo "warn: playwright-executor not ok (QA/docs jobs will fail or simulate, deploy kept; see deploy/playwright-executor)"
  echo "$(date -u +%FT%TZ) $SHA $REF OK" >> "$LOG"
  exit 0
fi

echo "DEPLOY GATE FAILED (database must be connected and /api/auth/me must be 401)"
echo "$(date -u +%FT%TZ) $SHA $REF FAILED" >> "$LOG"
"${COMPOSE[@]}" ps
"${COMPOSE[@]}" logs --tail 50 backend
echo "Roll back with: deploy.sh <last OK sha from $LOG>"
exit 1
