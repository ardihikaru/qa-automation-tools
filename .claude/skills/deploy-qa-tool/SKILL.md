---
name: deploy-qa-tool
description: Deploy, roll back, health-check or debug the production qa-automation stack at https://qa.mfardiansyah.id (Docker Compose on app-host 10.8.0.9 behind the edge-host nginx TLS edge). Use when asked to deploy/redeploy/release, check prod status or logs, roll back, renew or inspect the certificate, or change the edge vhost.
---

# Deploy qa-tool

Topology and invariants: [deploy/qa-tool/README.md](../../../deploy/qa-tool/README.md). Deploying is outward-facing, so **only do it when the user asks**.

## Pre-flight (local)
1. `cd ai-backend && go build -mod=mod ./... && go test -mod=mod ./...`
2. `cd dashboard && npm run build`
3. If the change includes a schema change, apply the migration **first**. See the `qa-db-ops` skill.
4. Check which branch app-host is on: `ssh app-host 'git -C ~/devel/projects/qa-automation-tools status -sb | head -1'`. `deploy.sh` runs `git pull --ff-only` on that branch and **fails** if the pull fails. Pushing and merging to `main` each need explicit approval; a deploy request does not cover them. If the commit isn't pushed, ship it with `git bundle` (see "Unpushed code").

## Deploy
```bash
ssh app-host '~/devel/projects/qa-automation-tools/deploy/qa-tool/deploy.sh'                # pull current branch, deploy
ssh app-host '~/devel/projects/qa-automation-tools/deploy/qa-tool/deploy.sh <sha>'          # ROLLBACK / pin (detached HEAD)
ssh app-host '~/devel/projects/qa-automation-tools/deploy/qa-tool/deploy.sh --branch main'  # roll forward after a rollback
```
- **Gate:** the script exits non-zero (and prints the backend logs) unless `/health` reports `"database":"connected"` **and** an anonymous `/api/auth/me` returns `401`, i.e. auth is on. If `ai` is not `ok`, it prints a warning but keeps the deploy.
- **History:** every deploy is appended to `deploy/qa-tool/deploys.log` on app-host (`<time> <sha> <ref> OK|FAILED`). The rollback target is the last `OK` sha before the bad one.
- **After a rollback**, a plain `deploy.sh` refuses to run on a detached HEAD. Use `--branch <name>` to roll forward.
- **Schema:** migrations only go forward. They must stay compatible with the previous release; otherwise a rollback also needs a DB restore (`qa-db-ops`).

## Verify
```bash
curl -s https://qa.mfardiansyah.id/health      # status ok, database connected, ai_provider ai-manager, ai ok
curl -s -o /dev/null -w '%{http_code}\n' https://qa.mfardiansyah.id/api/auth/me   # 401 = auth is ON
```
`database: disconnected` means **auth is off and the API is open**. Fix it immediately: check the `DB_*` values in `.env.prod`, and check that MySQL on db-host can be reached at `10.8.0.4:5306` from app-host.

## Operate
```bash
ssh app-host 'cd ~/devel/projects/qa-automation-tools && docker compose -f deploy/qa-tool/docker-compose.prod.yml ps'
ssh app-host 'cd ~/devel/projects/qa-automation-tools && docker compose -f deploy/qa-tool/docker-compose.prod.yml logs --tail 100 backend'
```
- **Secrets:** `deploy/qa-tool/.env.prod` on app-host (mode 600). To change a value, edit it in place over ssh **without printing it**, then run `deploy.sh`. New keys must also go into `.env.prod.example`.
- **Unpushed code:** run `git bundle create /tmp/qa.bundle <branch>` locally and `scp` it to app-host `/tmp/`. On app-host, if that branch is checked out, run `git pull --ff-only /tmp/qa.bundle <branch>`; otherwise run `git fetch /tmp/qa.bundle <branch>:<branch> && git checkout <branch>`. Then run `NO_PULL=1 deploy.sh`. The branch doesn't exist on GitHub yet, so a plain pull would fail.

## Edge (edge-host, shared with many products)
- The live file is `/etc/nginx/conf.d/qa-tool.conf`. The repo mirror is `deploy/qa-tool/nginx-edge.conf`; keep them identical.
- The repo mirror is the source of truth for *content*. The live file is what runs, so after any change confirm they match: `ssh edge-host 'cat /etc/nginx/conf.d/qa-tool.conf' | diff - deploy/qa-tool/nginx-edge.conf`.
- Change procedure: (1) back up the live file with `sudo cp /etc/nginx/conf.d/qa-tool.conf /tmp/qa-tool.conf.bak`, (2) `scp` the new file to `/tmp`, then `sudo install -m 644` it into conf.d, (3) run `sudo nginx -t`. **Only when that succeeds**, run `sudo systemctl reload nginx`. If it fails, `sudo install` the `.bak` back and run `nginx -t` again.
- Cert: lineage `qa-tool`, webroot `/var/www/acme`, renewed automatically by certbot. To check it: `ssh edge-host 'sudo certbot renew --dry-run --cert-name qa-tool'`.
- Never add `default_server`. Every `map` variable must be prefixed with `$qatool_`.

## Host facts (app-host)
- Ubuntu 24.04, 4 vCPU, 3.8 GB RAM, Docker CE with the compose plugin.
- `docker.service` starts after `wg-quick@wg0` (drop-in `/etc/systemd/system/docker.service.d/after-wg.conf`), so the bind to `10.8.0.9:8140` works at boot.
- ufw is inactive. The port is protected only by binding to the WG IP, so never publish on `0.0.0.0`.
