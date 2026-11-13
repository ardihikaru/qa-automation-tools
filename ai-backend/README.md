# AI Backend (Golang) Blueprint

## Overview

The Go API service for QA automation. It handles processing, AI calls, the database and the HTTP API.
It builds to a single binary from `cmd/server` (module `github.com/qa-automation/ai-backend`, Go 1.23).

`cmd/server/main.go:main` wires the pieces together in this order:

1. `config.LoadConfig()` loads configuration from env, `.env` and `config.yaml`.
2. `ai.NewFromConfig(cfg.Backend.AI)` builds the LLM client. It goes through the `ChatProvider` abstraction.
   The only supported path is ai-manager (`POST {AI_MANAGER_URL}/v1/chat`), pinned to `claude-v3`. The MiniMax code (`AI_PROVIDER=minimax`) is dormant.
   Playwright jobs (`/api/jobs/qa/run`, `/api/jobs/docs/generate`) are dispatched to playwright-executor (`internal/api/jobs_executor.go`).
3. `db.NewConnection(cfg)` connects to MySQL 8. Connection failure is non-fatal: the server keeps running without a DB.
4. When the DB connects, the DB-backed services start (`learning.NewService`, `skills.NewService` + `EnsureBuiltinSkills`).
   Users are seeded (`seed.SeedUsers`) and cookie-session auth is wired (`router.SetAuth`).
5. `api.NewRouter(...)` serves HTTP on `BACKEND_HOST:PORT` (default `0.0.0.0:8080`).

## Subdirectories & Boundaries

| Path | Responsibility | Contract |
|------|----------------|----------|
| `cmd/server/` | Main entry point (see Overview) | (this file) |
| `cmd/databasemigration/` | The only way to run schema migrations: `db.NewConnection` then `db.RunMigrations` | [internal/db](./internal/db/README.md), [database-migration.md](./database-migration.md) |
| `cmd/verifyai/` | Smoke test for the configured AI provider. Prints provider, `Configured()` and `Health()`, then sends one chat; exits 1 on failure | [internal/services/ai](./internal/services/ai/README.md) |
| `cmd/verifyminimax/` | Standalone MiniMax endpoint probe that writes `result.json` (legacy diagnostics) | n/a |
| `internal/api/` | HTTP router, all `/api/*` handlers, `/health`, auth endpoints, simulated job queue | [README](./internal/api/README.md) |
| `internal/auth/` | bcrypt passwords, MySQL-backed session tokens, `Require` middleware | [README](./internal/auth/README.md) |
| `internal/config/` | `Config` loading (env > `.env` > `config.yaml`), `GetDatabaseURL` MySQL DSN | [README](./internal/config/README.md) |
| `internal/db/` | MySQL 8 connection (`connection.go`) and idempotent migrations (`migrations.go`) | [README](./internal/db/README.md) |
| `internal/services/ai/` | `ChatProvider` interface; `aiManagerProvider` (claude-v3, the only supported one) and `minimaxProvider` (dormant); issue classification | [README](./internal/services/ai/README.md) |
| `internal/services/executor/` | playwright-executor HTTP client (`Submit`, `Get`, `Wait`, `Health`) | [playwright-executor](../playwright-executor/README.md) |
| `internal/services/skills/` | Skill registry and built-in skills | [README](./internal/services/skills/README.md) |
| `internal/services/knowledge/` | Product knowledge base: format adapters (`hisv3-test-catalog-v1`), sync engine (bare-mirror read, diff/upsert, AI change summary, scheduler), queries for `/api/products/{id}/knowledge/*` | [README](./internal/services/knowledge/README.md) |
| `internal/services/repopool/` | Product repo pool: clone, branch worktrees, verify, sync-owned bare mirrors | [README](./internal/services/repopool/README.md) |
| `internal/services/learning/` | Learning cache, navigation, sequences and context records | [README](./internal/services/learning/README.md) |
| `internal/seed/` | `SeedUsers`: imports users from the taskmgr Neo4j (read-only, `TASKMGR_NEO4J_*`), plus an `ADMIN_USERNAME`/`ADMIN_PASSWORD` admin and an `AI_SERVICE_USERNAME`/`AI_SERVICE_PASSWORD` AI service account (role `Service`, may impersonate) | n/a |
| `internal/models/` | Shared data structs (`Flow`, `Skill`, `RawIssue`, `LearningRecord`, `CostOverview`, ...) | n/a |

`internal/migrations/`, `internal/services/ast/` and `internal/services/patch/` are currently empty and hold no code.

## API Endpoints

The authoritative list, with methods, is in [internal/api/README.md](./internal/api/README.md). Summary:

- `/health` is always public. It returns `status`, `database`, `ai_provider`, `ai_upstream` (`claude-v3`), `ai`, `executor`, and also `minimax` for backward compatibility.
- These auth endpoints exist only when the DB is connected: `/api/auth/login`, `/api/auth/logout`, `/api/auth/me`, `/api/auth/change-password`.
- `/api/flows`, `/api/roles`, `/api/docs`, `/api/videos`
- `/api/issues`, `/api/issues/status`, `/api/issues/notes`
- `/api/skills`, `/api/skills/logs`, `/api/skills/stats`
- `/api/search?q=`
- `/api/cost/{overview,trend,efficiency,by-feature,by-model,recent,filters,gateway}`
- `/api/analytics`
- `/api/learning`, `/api/learning/{navigation,sequences,context,stats}`
- `/api/ai-cache`, `/api/embedding-cache`, `/api/goal-events`
- `/api/jobs`, `/api/jobs/qa/run`, `/api/jobs/docs/generate`, `/api/jobs/{id}`

## Global Invariants

- Cookie-session auth is enabled **only when the DB connects**: `cmd/server/main.go` calls `router.SetAuth` inside `if database != nil`.
  Without a DB the API is open, and the startup log says `Auth: disabled (no database — backend is OPEN)`.
- `cmd/server` never runs migrations. Schema changes go through `go run ./cmd/databasemigration`, which is idempotent (`CREATE TABLE IF NOT EXISTS`).
- Server LLM traffic goes through `internal/services/ai` (`ChatProvider`). It is the only package under `internal/` that makes outbound HTTP calls.
  The only exception is the standalone diagnostic `cmd/verifyminimax`, which calls MiniMax directly.
- CORS is set by `api/router.go:applyCORS` from `DASHBOARD_ORIGIN` (default `http://localhost:6173`).
  `config.go` also loads `CORS_ENABLED`, `CORS_ORIGINS` and `API_SECRET_KEY`, but the router does not read them.
- Seeding never overwrites an existing user's password (`seed/upsert.go:upsertUser`).
- The `Service` role is written only by `seed/seed_users.go:upsertServiceUser`. It never comes from taskmgr (`normalizeRole` maps it to `Developer`), the metadata update never changes a `Service` row, and an existing non-Service username is never promoted. The service password must be at least 32 characters, and the account has `must_change_pw = 0`.
  New users are created with `must_change_pw = 1`.

## Global Side Effects

- MySQL 8 database (`DB_*` or `DATABASE_URL`). Tables are listed in [internal/db/README.md](./internal/db/README.md).
- Outbound HTTP to ai-manager (HTTP Basic, over WireGuard in production) or to the MiniMax API.
- When `TASKMGR_NEO4J_URI` is set, the server reads from the taskmgr Neo4j at startup.
- Reads `../product-documentations` and `../playwright-docs/output/videos` from disk (`/api/docs`, `/api/videos`).

## Deployment

- Production: [deploy/his-v3-qa](../deploy/his-v3-qa/README.md). Docker Compose on rnd-v3-vpn, behind the clino nginx edge, at https://qa-tool.sismedika.com.
  The image is built from this directory's `Dockerfile`.
- `cloudbuild.yaml` (Cloud Run) is **legacy** and is no longer the deploy path.

## Tech Stack

- Golang (single binary, `net/http` with a hand-written path switch)
- MySQL 8 (`github.com/go-sql-driver/mysql`)
- LLM via ai-manager (default) or MiniMax (`AI_PROVIDER=minimax`, default model `MiniMax-M2.5`)
- Neo4j driver, used only for read-only user seeding from taskmgr

## Verification

- Unit and integration tests: `cd ai-backend && go test -mod=mod ./...`
  - DB integration tests in `internal/api` call `t.Skip` when no DB URL is configured.
- AI provider smoke test: `cd ai-backend && go run ./cmd/verifyai`
- Migrations: `cd ai-backend && go run ./cmd/databasemigration`
- Health: `curl -s localhost:8080/health`
