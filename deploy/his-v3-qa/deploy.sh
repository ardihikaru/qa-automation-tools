#!/usr/bin/env bash
# Deploy / update qa-automation on rnd-v3-vpn. Run ON rnd-v3-vpn from anywhere:
#   ~/devel/projects/qa-automation-tools/deploy/his-v3-qa/deploy.sh [git-ref]
# Rollback: deploy.sh <previous-commit-sha>
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
COMPOSE=(docker compose -f "$HERE/docker-compose.prod.yml" --env-file "$HERE/.env.prod")
REF="${1:-}"

[[ -f "$HERE/.env.prod" ]] || { echo "missing $HERE/.env.prod (copy .env.prod.example)"; exit 1; }
[[ "$(stat -c %a "$HERE/.env.prod")" == "600" ]] || chmod 600 "$HERE/.env.prod"

cd "$REPO"
if [[ -n "$REF" ]]; then
  git fetch --quiet origin
  git checkout --quiet "$REF"
elif [[ -d .git ]] && git remote get-url origin >/dev/null 2>&1; then
  git pull --ff-only --quiet || echo "warn: git pull failed, deploying working tree as-is"
fi
echo "deploying $(git rev-parse --short HEAD 2>/dev/null || echo working-tree)"

"${COMPOSE[@]}" up -d --build --remove-orphans
docker image prune -f >/dev/null

# shellcheck disable=SC1091
source <(grep -E '^HISV3QA_(BIND_IP|PORT)=' "$HERE/.env.prod")
URL="http://${HISV3QA_BIND_IP:-10.10.0.9}:${HISV3QA_PORT:-8140}/health"
for i in $(seq 1 30); do
  if out="$(curl -fsS -m 5 "$URL" 2>/dev/null)"; then
    echo "health: $out"
    exit 0
  fi
  sleep 2
done
echo "health check FAILED: $URL"
"${COMPOSE[@]}" ps
"${COMPOSE[@]}" logs --tail 50 backend
exit 1
