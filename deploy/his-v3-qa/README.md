# his-v3-qa deployment

## Overview
Production deployment of qa-automation at **https://his-v3.qa.sismedika.com**.
Everything private travels over the WireGuard mesh `10.10.0.0/24` (hub: `clino`).

| Hop | Host | Address | What |
|---|---|---|---|
| Edge | `clino` | 103.127.134.25 / 10.10.0.1 | nginx, TLS (Let's Encrypt lineage `his-v3-qa`), `/etc/nginx/conf.d/his-v3-qa.conf` |
| App | `rnd-v3-vpn` | 10.10.0.9:8140 | Docker Compose: `dashboard` (nginx + SPA, proxies `/api`, `/health`) and `backend` (Go, :8080 internal) |
| DB | `rnd-v2-vpn` | 10.10.0.4:5306 | MySQL 8 `master-compose-ai-mysqldb-master-1`, database `qa_automation`, user `qa_automation` |
| AI | `rnd-v2-vpn` | 10.10.0.4:8099 | ai-manager (`POST /v1/chat`, Basic auth), always with `provider: claude-v3`, `no_fallback` |
| LLM | `rnd-v3-vpn` | 10.10.0.9:8105 | ai-claude-cli, ai-manager's `claude-v3` upstream (`claude-opus-5`). Same host as the app, but it is only ever called *through* ai-manager. **The only LLM path** (MiniMax is dormant) |
| Browser | `rnd-v3-vpn` | 10.10.0.9:8141 | playwright-executor: systemd **user** unit, not part of this compose stack. Capped at 2.5 GiB / 60 % CPU. The backend calls it for `/api/jobs/*`. See [../playwright-executor](../playwright-executor/README.md) |

## Interface
- `docker-compose.prod.yml`: the stack. The port is bound to the WG IP only (`HISV3QA_BIND_IP`).
- `nginx.dashboard.conf`: the in-container nginx (SPA + reverse proxy to `backend:8080`).
- `nginx-edge.conf`: a mirror of the clino vhost. The repo copy is the source of truth for content; the live file must stay byte-identical to it.
- `.env.prod.example`: template for `.env.prod`, which lives only on rnd-v3 with mode 600 and is gitignored.
- `deploy.sh [<sha> | --branch <b>]`: pulls the current branch, or checks out `<sha>` to roll back, or returns to branch `<b>` to roll forward. Then it runs `up -d --build` and gates on `database: connected` plus an anonymous `/api/auth/me` returning 401. Every run is appended to `deploys.log`.

## Invariants
- Never publish container ports on `0.0.0.0`: Docker bypasses ufw. Bind to `10.10.0.9` only.
- Docker must start after `wg0` exists (systemd drop-in `/etc/systemd/system/docker.service.d/after-wg.conf`), or the port bind fails at boot.
- The clino edge is shared. Use the prefix `$hisv3qa_` for `map` variables, never set `default_server`, and always run `sudo nginx -t` before `systemctl reload nginx`.
- Secrets live only in `.env.prod` on rnd-v3. Never echo them in logs, commits or chat. `PLAYWRIGHT_EXECUTOR_TOKEN` there must equal `EXECUTOR_TOKEN` in `~/.config/qa-automation/playwright-executor.env`.
- `deploy.sh` doesn't touch playwright-executor. When `playwright-executor/` changes, also run `deploy/playwright-executor/install.sh` (use `SKIP_DEPS=1` once the apt deps are installed).
- The backend's cookie auth is enabled **only if the DB connects**. If `/health` shows `"database":"disconnected"`, the API is open. Treat that as an incident.

## Side Effects
- Writes to MySQL `qa_automation` on rnd-v2-vpn.
- AI calls are billed and logged by ai-manager (`caller: qa-automation`), and are visible in ai-manager `/admin/usage/recent`.

## Verification
- Deploy: `ssh rnd-v3-vpn '~/devel/projects/qa-automation-tools/deploy/his-v3-qa/deploy.sh'`
- Health from WG: `curl -s http://10.10.0.9:8140/health` should show `database: connected`, `ai_provider: ai-manager`, `ai_upstream: claude-v3`, `ai: ok`, `executor: ok`.
- Public: `curl -sI https://his-v3.qa.sismedika.com` returns `200`. `/api/auth/me` returns `401` until you log in.
- AI round trip on rnd-v3: `docker compose -f deploy/his-v3-qa/docker-compose.prod.yml exec backend wget -qO- localhost:8080/health`
- Logs: `docker compose -f deploy/his-v3-qa/docker-compose.prod.yml logs -f backend`
- Rollback: `deploy.sh <last OK sha from deploys.log>`; roll forward later with `deploy.sh --branch main`
- Cert renewal: `ssh clino 'sudo certbot renew --dry-run --cert-name his-v3-qa'`
