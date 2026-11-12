# ai-manager integration and self-hosted deployment (qa.mfardiansyah.id)

*   **Author:** ardihikaru
*   **Status:** Approved (retrospective: written alongside the implementation on branch `feature/ai-manager-egm-sut-deploy`)
*   **Date:** 2026-11-13

--------------------------------------------------------------------------------

## Overview

qa-automation-tools now runs on our own infrastructure instead of Google Cloud Run and Neon, and every LLM call now goes through the team's production LLM gateway, **ai-manager**, instead of calling MiniMax directly. The app is served at `https://qa.mfardiansyah.id`. All traffic between the private hosts travels over the WireGuard mesh `10.8.0.0/24`.

## The Problem

- The LLM calls were hand-written MiniMax HTTP calls, in two places (`ai-backend/internal/services/ai/aiService.go` and `playwright-docs/src/summarizer/summarizeFlow.ts`). That gave us no provider choice, no fallback, no central usage tracking, and pricing that was hard-coded.
- The deployment targets had drifted: Cloud Run build files, `.env.example` and docs describing Neon, while the code had already moved to MySQL.
- The data sat on `203.0.113.21:31006`, a server that also hosts the SUT production schemas.

The solution: put a provider interface in front of ai-manager (reached directly over WireGuard), move the `qa_automation` schema to the shared master-compose MySQL on db-host, and run the app with Docker Compose on app-host behind the existing edge-host TLS edge.

## Goals

*   Every LLM call from Go and TS goes through ai-manager `POST /v1/chat`, with no direct provider call, and is **pinned to upstream `claude-v3`**: the Claude CLI REST (`ai-claude-cli`) on app-host at `10.8.0.9:8105`. The request sends `no_fallback: true`, and the client rejects any reply from another provider. (Amended 2026-11-13 at the user's request.) MiniMax-direct remains a **manual** alternative (`AI_PROVIDER=minimax`); there is no automatic failover in this repo.
*   Usage is attributed in ai-manager with the caller names `qa-automation` and `qa-automation-playwright-docs`.
*   Production is reachable at `https://qa.mfardiansyah.id` with a valid certificate and cookie auth enabled.
*   Data is migrated with per-table row counts identical between the old and new databases.
*   `/health` reports `database`, `ai_provider` and `ai`, so operators can see if auth is open.

## Non-Goals

*   Real embeddings. `GenerateEmbedding` stays a placeholder.
*   Async or polling mode for ai-manager (the `X-Request-ID` header).
*   CI/CD automation. Deploys run by hand with `deploy.sh`.
*   Fixing the existing debt in `playwright-qa` and `playwright-docs` builds, the E2E suite login, or `router.go` formatting. This is tracked separately.

## Open Questions

*   Should the E2E suite get a login fixture so it can run against production? The routes require a session.
*   ai-manager may move to db2-host (`10.8.0.5`). If it does, only `AI_MANAGER_URL` changes.

## Critical User Journeys (CUJs)

1.  **Issue classification.** *Given* a raw issue with no category, *when* the playwright-qa runner posts it to `/api/issues`, *then* the backend calls `ClassifyIssue`, which goes through ai-manager, and stores `frontend`, `backend` or `unknown`.
2.  **Operator health check.** *Given* the stack is running, *when* an operator `curl`s `https://qa.mfardiansyah.id/health`, *then* the response shows `database: connected`, `ai_provider: ai-manager`, `ai: ok`.
3.  **Login.** *Given* a seeded user, *when* they sign in on the dashboard, *then* a `Secure`, `HttpOnly` `qa_session` cookie is set over HTTPS, and `/api/*` calls succeed.
4.  **Rollback.** *Given* a bad release, *when* the operator takes the last `OK` sha from `deploy/qa-tool/deploys.log` and runs `deploy.sh <sha>`, *then* that commit is rebuilt and the gate passes. The gate requires `database: connected` and an anonymous `/api/auth/me` returning `401`. To roll forward later, run `deploy.sh --branch main`. Schema migrations only go forward, so they must stay compatible with the previous release.

## Technical Plan (Architecture)

```
browser ─TLS─> edge-host nginx (203.0.113.10 / 10.8.0.1) ─WG HTTP─> 10.8.0.9:8140
  app-host: dashboard (nginx: SPA, /api + /health → backend:8080) ── backend (Go)
                                    backend ─WG─> 10.8.0.4:5306  MySQL qa_automation (db-host)
                                    backend ─WG─> 10.8.0.4:8099  ai-manager (db-host)
                                                    └─ provider claude-v3 ─WG─> 10.8.0.9:8105 ai-claude-cli (app-host)
```

*   **AI seam.** `ChatProvider` (`internal/services/ai/provider.go`) has two implementations, `aiManagerProvider` (Basic auth, 180 s timeout; it caches ai-manager's `GET /v1/health` for 30 s, and `/health.ai` is `ok`, `unreachable`, `status_<code>` or `not_configured`) and `minimaxProvider`. `ai.NewFromConfig` picks one based on `AI_PROVIDER`. The public methods of `*ai.Service` keep their signatures, so callers did not change. `LastUsage()` exposes real tokens and cost.
*   **Config.** New env keys: `AI_PROVIDER`, `AI_MANAGER_URL`, `AI_MANAGER_LOGIN_ID`, `AI_MANAGER_SECRET_KEY`, `AI_MANAGER_PROVIDER` (default `claude-v3`), `AI_MANAGER_NO_FALLBACK` (default `true`), `AI_MANAGER_MODEL`, `AI_TIMEOUT_SECONDS`. `/health` adds `ai_upstream`; `ai` can also be `upstream_disabled:claude-v3`. Env takes precedence over `config.yaml`.
*   **DB.** A dedicated user `qa_automation@%` has privileges on schema `qa_automation` only. The data was moved with `mysqldump` and then loaded; migrations were re-run afterwards (they are idempotent).
*   **Deploy.** `deploy/qa-tool/` holds the Compose file. The port is bound to the WG IP only. `docker.service` is ordered after `wg-quick@wg0`. The edge vhost copies the pattern of the existing `edge-host:/etc/nginx/conf.d/ai-minimax-cli.conf`: ACME on :80 served from `/var/www/acme`, everything else redirected to 443, TLS terminated there and proxied over WG with namespaced `map` variables. Certificate lineage: `qa-tool`.
*   **Timeouts along the AI path** are strictly ordered: ai-manager single-provider worst case (~150 s) < client 180 s < dashboard nginx 300 s < edge 330 s. If ai-manager's internal fallback chain runs past 180 s, the call fails, and callers degrade (for example, `ClassifyIssue` falls back to a heuristic).

## Alternatives Considered

### Call ai-manager through its public URL
*   **Pros:** works from any host.
*   **Cons:** an extra TLS hop through the public edge, and the gateway's traffic leaves the private network for no benefit.
*   **Reason for Rejection:** app-host is a WireGuard peer, so the direct path is shorter and keeps traffic private.

### systemd units instead of Docker (the ai-manager style)
*   **Pros:** no Docker daemon, which matches how ai-manager runs.
*   **Cons:** the Dockerfiles already exist, and the dashboard would need host nginx plus a Node build on the VM.
*   **Reason for Rejection:** Compose reuses the existing images and makes rollback a single command.

### Keep the data on 203.0.113.21
*   **Reason for Rejection:** that server holds SUT production schemas. Moving QA data to the shared master-compose MySQL isolates it, and the dedicated user cannot see anything else.

## Non-Functional Requirements

*   **Security:** production app secrets live only in `deploy/qa-tool/.env.prod` (mode 600) and `~/.config/ai-stack/basic-auth.json` on app-host. No port is open beyond `wg0`. Auth is enforced whenever the DB is up.
*   **Reliability:** `restart: unless-stopped`, container health checks, and a deploy gate in `deploy.sh`: DB connected, auth on, and a `deploys.log` history.
*   **Observability:** `/health` shows provider state. Logs are kept as json-file, 10 MB × 5. AI usage can be viewed in ai-manager admin.

## Detailed Implementation Plan

### Target Files to Create/Modify

*   `ai-backend/internal/services/ai/{provider.go, aimanager.go, minimax.go, aimanager_test.go}` (Create); `aiService.go` (Modify): provider seam.
*   `ai-backend/internal/config/config.go`, `cmd/server/main.go`, `internal/api/router.go` (Modify): config, wiring, `/health`.
*   `ai-backend/cmd/verifyai/main.go` (Create): smoke test.
*   `playwright-docs/src/summarizer/summarizeFlow.ts`, `src/index.ts`, `src/recorder/recordFlow.ts`, `<repo>/lib/config.ts` (Modify): `callAI`.
*   `deploy/qa-tool/*` (Create); `dashboard/Dockerfile`, `ai-backend/.dockerignore`, `.gitignore` (Modify).
*   `ai-backend/.env.example`, `config.yaml.template`, `ai-backend/database-migration.md` (Modify): MySQL and ai-manager canon.
*   `this-qa-automation-system-playwright/{config.ts, playwright.config.ts}` (Modify): `QA_BACKEND_URL` / `QA_DASHBOARD_URL` overrides.
*   EGM adoption (Create/Modify): `CLAUDE.md` files, `.claude/{settings.json, skills, rules, agents}`, `docs/designs/`, `.egmignore`, and the README contracts.

### Verification Plan

*   **Build:** `go build -mod=mod ./...`; `docker build` for both images.
*   **Unit Tests:** `go test -mod=mod ./...`, which includes the ai-manager client tests (success, 401/502/503, empty text, timeout, health cache, usage).
*   **E2E:** `go run -mod=mod ./cmd/verifyai` returns a reply through ai-manager. `https://qa.mfardiansyah.id/health` is green, and `/api/auth/me` returns 401 when logged out. Row counts match between the old and new DB.

*Drafted with EGM*
