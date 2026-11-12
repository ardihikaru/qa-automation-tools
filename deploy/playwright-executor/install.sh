#!/usr/bin/env bash
# Install or upgrade playwright-executor as a systemd USER service on app-host.
# Idempotent. Run as the service user (qa) from anywhere inside the repo checkout:
#
#   deploy/playwright-executor/install.sh            # install/upgrade + restart + health gate
#   SKIP_DEPS=1 deploy/playwright-executor/install.sh  # skip apt deps (already installed)
#
# Secrets are NOT handled here: the env file and credentials file are created
# out-of-band (mode 600). This script only creates a placeholder env file.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP_DIR="$REPO_DIR/playwright-executor"
TMPL="$REPO_DIR/deploy/playwright-executor/playwright-executor.service.tmpl"
CONF_DIR="$HOME/.config/qa-automation"
ENV_FILE="$CONF_DIR/playwright-executor.env"
UNIT_DIR="$HOME/.config/systemd/user"
UNIT="playwright-executor.service"
MISE="${MISE:-$(command -v mise || echo "$HOME/.local/bin/mise")}"
CPU_PERCENT="${CPU_PERCENT:-60}"

log() { printf '\033[1;34m[executor-install]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[executor-install]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -ne 0 ] || die "run as the service user, not root"
[ -x "$MISE" ] || die "mise not found (set MISE=/path/to/mise)"
[ -f "$APP_DIR/package.json" ] || die "missing $APP_DIR"

# 1. Toolchain + build
log "mise install (node from $APP_DIR/mise.toml)"
cd "$APP_DIR"
"$MISE" trust -q "$APP_DIR/mise.toml"
"$MISE" install -q
log "npm ci + build"
"$MISE" exec -- npm ci --no-audit --no-fund --loglevel=error
"$MISE" exec -- npm run build --silent
"$MISE" exec -- npm prune --omit=dev --no-audit --no-fund --loglevel=error

# 2. Browser + system libraries
log "playwright install chromium"
"$MISE" exec -- npx playwright install chromium
if [ "${SKIP_DEPS:-0}" != 1 ]; then
  log "playwright install-deps chromium (sudo apt)"
  NODE_BIN="$("$MISE" which node)"
  sudo env PATH="$(dirname "$NODE_BIN"):$PATH" "$NODE_BIN" "$APP_DIR/node_modules/playwright/cli.js" install-deps chromium
fi

# 3. Env file (placeholder only; real values are written out-of-band)
mkdir -p "$CONF_DIR"
chmod 700 "$CONF_DIR"
if [ ! -f "$ENV_FILE" ]; then
  log "creating placeholder $ENV_FILE from .env.example (fill EXECUTOR_TOKEN, TARGET_STAGING_URL, credentials)"
  (umask 077; cp "$APP_DIR/.env.example" "$ENV_FILE")
fi
chmod 600 "$ENV_FILE"
# shellcheck disable=SC1090
PORT="$(set -a; . "$ENV_FILE"; echo "${EXECUTOR_PORT:-8141}")"
BIND="$(set -a; . "$ENV_FILE"; echo "${EXECUTOR_BIND:-127.0.0.1}")"
[ "$BIND" != "0.0.0.0" ] || die "EXECUTOR_BIND=0.0.0.0 is forbidden; bind the WireGuard IP"

# 4. Unit
CPU_QUOTA="$(( $(nproc) * CPU_PERCENT ))%"
mkdir -p "$UNIT_DIR"
sed -e "s|@APP_DIR@|$APP_DIR|g" -e "s|@ENV_FILE@|$ENV_FILE|g" -e "s|@MISE@|$MISE|g" \
    -e "s|@CPU_QUOTA@|$CPU_QUOTA|g" -e "s|@PORT@|$PORT|g" "$TMPL" > "$UNIT_DIR/$UNIT"
log "unit installed: CPUQuota=$CPU_QUOTA MemoryMax=2560M"
loginctl show-user "$(id -un)" -p Linger | grep -q 'Linger=yes' || log "WARNING: linger is off; run: sudo loginctl enable-linger $(id -un)"
systemctl --user daemon-reload
systemctl --user enable "$UNIT" >/dev/null
systemctl --user restart "$UNIT"

# 5. Health gate
log "waiting for http://$BIND:$PORT/health"
for _ in $(seq 1 30); do
  if body="$(curl -fsS --max-time 3 "http://$BIND:$PORT/health" 2>/dev/null)"; then
    log "healthy: $body"
    exit 0
  fi
  sleep 1
done
systemctl --user status "$UNIT" --no-pager | head -20 || true
journalctl --user -u "$UNIT" -n 30 --no-pager || true
die "playwright-executor did not become healthy"
