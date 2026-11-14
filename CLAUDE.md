# qa-automation-tools — agent instructions

This is an AI-assisted QA automation system for **HISv3**, the Sismedika hospital information system. Playwright explores and tests HISv3. A Go backend stores flows, issues and skills in MySQL and calls an LLM through **ai-manager**. A React dashboard shows the results.

- Production: **https://qa-tool.sismedika.com**, running on rnd-v3-vpn. See [deploy/his-v3-qa/README.md](deploy/his-v3-qa/README.md).
- System contract: [README.md](README.md). The per-directory README files are EGM (Elephant-Goldfish Model, see below) contracts: read the one for the directory you are about to change.
- Goals: [OBJECTIVES.md](OBJECTIVES.md). Design specs: [docs/designs/SITEMAP.md](docs/designs/SITEMAP.md).

EGM artifact directory: .dev/docs

(EGM drafts and review reports go there. It is gitignored; approved specs move to `docs/designs/`.)

## Component map

| Path | What | Build / test |
|---|---|---|
| `ai-backend/` | Go 1.23 REST API (`net/http`, switch router), MySQL, cookie auth, AI facade | `cd ai-backend && go build -mod=mod ./... && go test -mod=mod ./...` |
| `dashboard/` | React 18 + Vite + Tailwind SPA, same-origin `/api` | `cd dashboard && npm run build` |
| `playwright-executor/` | **The only Playwright runtime.** HTTP service (`case_run`) on rnd-v3 as a systemd user unit, node via mise, capped at 2.5 GiB / 60 % CPU | `cd playwright-executor && mise exec -- npm test` |
| `deploy/playwright-executor/` | systemd unit template + `install.sh` | see its README |
| `this-qa-automation-system-playwright/` | E2E tests of *this* system (API + dashboard) | `cd this-qa-automation-system-playwright && npx playwright test` |
| `lib/config.ts` | Shared TS loader for `config.yaml` and env | — |
| `deploy/his-v3-qa/` | Docker Compose stack, edge nginx mirror, `deploy.sh` | see its README |

`ai-backend`, `dashboard`, `playwright-executor` and `this-qa-automation-system-playwright` each have their own `CLAUDE.md` with local rules.

## Access prerequisites
- **WireGuard.** You must be a peer on `10.10.0.0/24`. The hub is `clino` (`103.127.134.25:51820`). Ask the infra owner (ardihikaru) to add a peer; the setup script lives in the personal-workspace repo at `apps/ai-antigravity-cli/deploy/wireguard-client/setup-client.sh`.
- **ssh aliases.** You need `clino` (user `clinoid`), plus `rnd-v2-vpn` and `rnd-v3-vpn` (user `sismedika`, `ProxyJump clino`). All three hosts have passwordless `sudo`.
- Without WG access you can still build and run unit tests, but anything that needs `10.10.0.x` (DB, ai-manager, deploy) will fail.

## Infrastructure (WireGuard `10.10.0.0/24`, hub = `clino`)

| IP | ssh alias | Role for this repo |
|---|---|---|
| 10.10.0.1 / 103.127.134.25 | `clino` | Shared nginx TLS edge: `/etc/nginx/conf.d/his-v3-qa.conf`, cert lineage `qa-tool` |
| 10.10.0.4 | `rnd-v2-vpn` | MySQL `:5306` (container `master-compose-ai-mysqldb-master-1`, DB/user `qa_automation`); **ai-manager `:8099`** |
| 10.10.0.7 | `taskmgr-vpn` | taskmgr Neo4j (optional one-shot user seed; port not reachable from rnd-v3) |
| 10.10.0.9 | `rnd-v3-vpn` | This app (Docker Compose, `10.10.0.9:8140`); **playwright-executor `:8141`** (systemd `--user`, 2.5 GiB / 60 % CPU cap); **ai-claude-cli `:8105`**, which is ai-manager's `claude-v3` upstream for every LLM call in this repo |

## Environment variable canon
- DB: `DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME` (or a MySQL `DATABASE_URL`). The local `.env` files also contain `MYSQL_*`. **The code ignores those**; they point at the HISv3 *production* schema. Never use them for this app.
- AI: `AI_PROVIDER` (`ai-manager` by default, or `minimax`), `AI_MANAGER_URL` (`http://10.10.0.4:8099`), `AI_MANAGER_LOGIN_ID`, `AI_MANAGER_SECRET_KEY`, `AI_MANAGER_PROVIDER` (**`claude-v3`**, always), `AI_MANAGER_NO_FALLBACK` (`true`), `AI_MANAGER_MODEL` (empty means claude-v3's default, `claude-opus-5`), `AI_TIMEOUT_SECONDS` (180). **claude-v3 via ai-manager is the only supported LLM path.** `MINIMAX_*`/`AI_MODEL`/`AI_BASE_URL` belong to the **dormant** `AI_PROVIDER=minimax` code path. Never enable it without the user's explicit approval. There is no automatic failover in this repo.
- Playwright: backend `PLAYWRIGHT_EXECUTOR_URL` (`http://10.10.0.9:8141`; empty means `case_run` jobs fail), `PLAYWRIGHT_EXECUTOR_TOKEN`, `PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES` (30). On the executor side: `EXECUTOR_*`, `HISV3_STAGING_URL`, `HISV3_CREDENTIALS_FILE`, `RUN_TIMEOUT_SECONDS`, `MAX_QUEUE`, in `~/.config/qa-automation/playwright-executor.env` on rnd-v3 (mode 600). Template: [playwright-executor/.env.example](playwright-executor/.env.example).
- Auth: `SESSION_COOKIE_*`, `DASHBOARD_ORIGIN`, `ADMIN_*`, `AI_SERVICE_*`, `TASKMGR_NEO4J_*`. `AI_SERVICE_USERNAME`/`AI_SERVICE_PASSWORD` seed the AI service account (role `Service`, which may impersonate any user; see `.claude/rules/qa-tool-ai-account.md`). Users are seeded at server start (`ai-backend/internal/seed/seed_users.go`): `TASKMGR_NEO4J_*` copies taskmgr users once, then `ADMIN_USERNAME`/`ADMIN_PASSWORD` upserts one admin. Production already has the 44 migrated users, so both are left empty there.
- Templates: [ai-backend/.env.example](ai-backend/.env.example), [deploy/his-v3-qa/.env.prod.example](deploy/his-v3-qa/.env.prod.example), [config.yaml.template](config.yaml.template).

## Hard rules
1. **Secrets.** Never print, commit or paste the contents of `.env*`, `config.yaml`, `_data/`, `~/.config/ai-stack/basic-auth.json`, the infra-prod `.env` on rnd-v2, or `~/.config/qa-automation/*` on rnd-v3 (executor env and HISv3 credentials). Load them into env or stdin; print only key names or counts. See `.claude/rules/secrets.md`.
2. **All LLM calls go through ai-manager, pinned to upstream `claude-v3`.** `claude-v3` is the Claude CLI REST service (`ai-claude-cli`) on rnd-v3 at `10.10.0.9:8105`. Never call `:8105` directly, never switch to another upstream (antigravity, claude, minimax...), and never disable `no_fallback` without the user's explicit approval. In code, use `ai.Service` / `ChatProvider` in Go (`ai-backend/internal/services/ai/`); there is no TS caller (the old `callAI` in playwright-docs was removed). Never add a direct provider SDK or HTTP call elsewhere.
3. **Schema changes** are appended to `ai-backend/internal/db/migrations.go` as idempotent statements and applied with `go run -mod=mod ./cmd/databasemigration`. The server never migrates on its own.
4. **Auth is enabled only when the DB connects.** If `/health` shows `database: disconnected`, the API is open. Treat that as an incident, not a warning.
5. **Shared infra.** The clino edge serves many products. Always run `sudo nginx -t` before a reload, never add `default_server`, and prefix `map` variables with `$hisv3qa_`. The rnd-v2 MySQL is shared, so only touch the `qa_automation` schema.
6. Never publish container ports on `0.0.0.0` on rnd-v3. Docker bypasses ufw. The same goes for host services: playwright-executor binds `10.10.0.9` only.
7. `ai-backend/vendor/` is stale and gitignored, so always use `-mod=mod`.
8. Git: follow [Git workflow](#git-workflow) below. Deploying is a **separate** approval: a merge to `main` never authorises a deploy.
9. **Playwright runs only in playwright-executor**, and only against HISv3 staging. Don't add Playwright to the backend image or the compose stack, and don't raise the unit's `MemoryMax`/`CPUQuota` without approval.

## Git workflow
- **Worktrees.** Always work in a git worktree, never in the main checkout: `git worktree add ../qa-automation-tools.worktrees/<branch> -b <branch> main`. Remove it with `git worktree remove` once the work is done.
- **PRs and merges.** Branch off `main`, use conventional commits, and open PRs periodically (small, reviewable steps). Pushing branches, opening PRs and merging them to `main` are pre-authorised. Deploying stays a separate approval.
- **Cleanup.** Delete every merged branch (local and remote) and prune worktrees right after the merge: `git branch -d <branch>`, `git push origin --delete <branch>`, `git worktree prune`.

## Workflow — Elephant-Goldfish Model (EGM)
*Design with an elephant, review with a goldfish.* The plugin is `egm@egm` (enabled in `.claude/settings.json`).
- **Elephant** (this context, plus the `qa-automation-elephant` skill and the `qa-elephant` agent): holds the full history. It explores, then drafts specs.
- **Goldfish** (`egm:egm-*` reviewer agents): reviewers with no conversation history. They see only the document and the code it cites, and never this file. Any assumption a review must challenge has to be written into the review criteria.
- **Human**: decides, approves, and owns the result.

When to use it:
- **Features, schema/infra changes, new integrations:** `/egm:egm-brainstorm`, then `/egm:egm-draft` (to `.dev/docs`, then `docs/designs/<FEATURE>.md` with a SITEMAP entry), then `/egm:egm-review`, then `/egm:egm-resolve`, then a human PR of the spec, then implementation. Once the review's final stage (Implementation Readiness) passes and a human has approved the spec PR, the spec's "Target Files" list is the set of files implementation may change.
- **Bug fixes and small chores:** skip the design phase. Still update the README contract of any directory whose interface or invariants change.
- **Every week, or before a release:** `/egm:egm-drift-report docs/designs/`, then decide whether to fix the doc or the code.
- **After large refactors:** `/egm:egm-bootstrap <dir>` to refresh the README contracts. It merges into existing files and must cite code.

Project skills: `qa-automation-elephant` (load before any design work), `deploy-qa-tool`, `ai-manager-gateway`, `qa-db-ops`, `qa-tool-session` (use the prod app as the `ai-service` account and impersonate any user; agent: `qa-tool-operator`), `case-automation-runbook` and `gaos-integration` (run/create case automation and the GAOS link/Inbox; agent: `qa-case-automation`). Rules in `.claude/rules/` load automatically by path.
