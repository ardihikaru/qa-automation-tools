#!/usr/bin/env bash
# qa-tool.sh — drive the qa-automation API (https://qa-tool.sismedika.com) as
# the AI service account, with optional impersonation of any user.
#
# Credentials: $QA_TOOL_ENV (default ~/.config/qa-automation/ai-service.env,
# mode 600) with QA_TOOL_URL, QA_TOOL_USERNAME, QA_TOOL_PASSWORD.
# The password never appears in argv or output: jq reads it from the env and
# curl reads the body from stdin.
#
# Cookie jar: $QA_TOOL_JAR (default $XDG_RUNTIME_DIR/qa-tool-session.jar).
#
# Usage:
#   qa-tool.sh login                 # POST /api/auth/login, prints {username, role}
#   qa-tool.sh me                    # who am I (and impersonator, if any)
#   qa-tool.sh users                 # GET /api/users (username, role, active)
#   qa-tool.sh as <username>         # start impersonating
#   qa-tool.sh stop                  # stop impersonating
#   qa-tool.sh logout                # log out and delete the jar
#   qa-tool.sh call <METHOD> <path> [json-body]   # any API call; prints status + body
#   qa-tool.sh smoke <qa-username>   # end-to-end check, see SKILL.md
set -euo pipefail

ENV_FILE="${QA_TOOL_ENV:-$HOME/.config/qa-automation/ai-service.env}"
JAR="${QA_TOOL_JAR:-${XDG_RUNTIME_DIR:-/tmp}/qa-tool-session.jar}"

die() { echo "qa-tool: $*" >&2; exit 1; }

[ -r "$ENV_FILE" ] || die "credential file $ENV_FILE not found (see SKILL.md: Setup)"
mode=$(stat -c %a "$ENV_FILE")
[ "$mode" = "600" ] || [ "$mode" = "400" ] || die "$ENV_FILE must be mode 600 (is $mode)"
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
: "${QA_TOOL_URL:=https://qa-tool.sismedika.com}"
[ -n "${QA_TOOL_USERNAME:-}" ] && [ -n "${QA_TOOL_PASSWORD:-}" ] || die "QA_TOOL_USERNAME/QA_TOOL_PASSWORD missing in $ENV_FILE"
QA_TOOL_URL="${QA_TOOL_URL%/}"

umask 077

# req METHOD PATH [BODY] → prints "<status>\n<body>"
req() {
  local method=$1 path=$2 body=${3:-} empty='{}'
  local args=(-sS -o - -w '\n%{http_code}' -b "$JAR" -c "$JAR" -X "$method" "$QA_TOOL_URL$path")
  if [ "$method" != GET ]; then
    args+=(-H 'Content-Type: application/json' --data-binary @-)
    printf '%s' "${body:-$empty}" | curl "${args[@]}"
  else
    curl "${args[@]}"
  fi
}

# split req output into $STATUS and $BODY
call() {
  local out
  out=$(req "$@")
  STATUS=${out##*$'\n'}
  BODY=${out%$'\n'*}
}

expect() { # expect <status> <label>
  if [ "$STATUS" = "$1" ]; then echo "  ok   $2 ($STATUS)"; else echo "  FAIL $2: got $STATUS want $1: $BODY"; FAILED=1; fi
}

who() { echo "$BODY" | jq -c '{user: .user.username, role: .user.role, impersonator: (.user.impersonator.username // null)}'; }

cmd=${1:-}; shift || true
case "$cmd" in
  login)
    out=$(jq -n '{username: env.QA_TOOL_USERNAME, password: env.QA_TOOL_PASSWORD}' |
      curl -sS -o - -w '\n%{http_code}' -c "$JAR" -X POST -H 'Content-Type: application/json' \
        --data-binary @- "$QA_TOOL_URL/api/auth/login")
    STATUS=${out##*$'\n'}; BODY=${out%$'\n'*}
    [ "$STATUS" = 200 ] || die "login failed ($STATUS): $BODY"
    who ;;
  me)     call GET /api/auth/me; [ "$STATUS" = 200 ] || die "me: $STATUS $BODY"; who ;;
  users)  call GET /api/users; [ "$STATUS" = 200 ] || die "users: $STATUS $BODY"
          echo "$BODY" | jq -r '.[] | [.username, .role, (if .active then "active" else "inactive" end)] | @tsv' ;;
  as)     [ -n "${1:-}" ] || die "usage: as <username>"
          call POST /api/auth/impersonate "$(jq -cn --arg u "$1" '{username: $u}')"
          [ "$STATUS" = 200 ] || die "impersonate: $STATUS $BODY"; who ;;
  stop)   call POST /api/auth/impersonate/stop; [ "$STATUS" = 200 ] || die "stop: $STATUS $BODY"; who ;;
  logout) call POST /api/auth/logout || true; rm -f "$JAR"; echo "logged out ($STATUS)" ;;
  call)   [ -n "${2:-}" ] || die "usage: call <METHOD> <path> [json-body]"
          call "$1" "$2" "${3:-}"; echo "$STATUS"; echo "$BODY" ;;
  smoke)
    [ -n "${1:-}" ] || die "usage: smoke <qa-username>"
    FAILED=0
    echo "smoke against $QA_TOOL_URL"
    out=$(curl -sS -o - -w '\n%{http_code}' "$QA_TOOL_URL/health"); STATUS=${out##*$'\n'}; BODY=${out%$'\n'*}
    expect 200 "health"
    [ "$(echo "$BODY" | jq -r .database)" = connected ] && echo "  ok   database connected" || { echo "  FAIL database not connected — auth is OFF, treat as incident"; FAILED=1; }
    "$0" login >/dev/null && echo "  ok   login" || { echo "  FAIL login"; exit 1; }
    call GET /api/auth/me; expect 200 "me"; [ "$(echo "$BODY" | jq -r .user.role)" = Service ] || { echo "  FAIL role is not Service"; FAILED=1; }
    call GET /api/users; expect 200 "users"
    call POST /api/auth/impersonate "$(jq -cn --arg u "$1" '{username: $u}')"; expect 200 "impersonate $1"
    [ "$(echo "$BODY" | jq -r .user.impersonator.role)" = Service ] || { echo "  FAIL impersonator not Service"; FAILED=1; }
    call GET /api/products; expect 200 "products read as $1"
    call POST /api/products '{"name":"qa-tool-smoke-must-not-exist"}'; expect 403 "products create refused for QA"
    call POST /api/auth/impersonate/stop; expect 200 "stop"
    [ "$(echo "$BODY" | jq -r .user.role)" = Service ] || { echo "  FAIL not back to Service"; FAILED=1; }
    call POST /api/auth/logout; expect 204 "logout"; rm -f "$JAR"
    [ "$FAILED" = 0 ] && echo "SMOKE OK" || { echo "SMOKE FAILED"; exit 1; } ;;
  *) sed -n '2,21p' "$0"; exit 2 ;;
esac
