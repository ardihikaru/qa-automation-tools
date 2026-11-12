# api

## Overview

Package `api` is the HTTP surface of the ai-backend. One `Router`
(`router.go:Router`) implements `http.Handler` and dispatches on exact
`req.URL.Path` matches (a `switch` statement, not a third-party mux) to
handlers backed by MySQL 8 (through the [`db`](../db/README.md) wrapper), the
[`services/ai`](../services/ai/README.md) service, and the
[`services/learning`](../services/learning/README.md) service. Cookie-session
login endpoints live in `auth_handlers.go`, and the session and middleware
primitives come from [`auth`](../auth/README.md).

## Interface

- `router.go:NewRouter(db *db.DB, aiService *ai.Service, learningService *learning.Service, skillsService *skills.Service) *Router`: every dependency may be nil. `skillsService` ([`services/skills`](../services/skills/README.md)) is stored on the struct, but no handler in `router.go` calls it.
- `router.go:(*Router).ServeHTTP`: the entry point. It runs in this order: `applyCORS`, then `OPTIONS` returns 200, then the allow-list (`/api/auth/login` when auth is wired, and `/health`), then `requireAuth` (if set) wrapping `dispatchAuthenticated`.
- `router.go:(*Router).SetAuth(h *AuthHandlers, mw func(http.Handler) http.Handler, dashboardOrigin string)`: wires the auth handlers, the middleware, and the CORS origin.
- `auth_handlers.go:AuthHandlers{DB *sql.DB, CookieName string, SessionTTL time.Duration, Secure bool}` with methods `Login`, `Logout`, `Me`, `ChangePassword`.

### Routes (as registered in `router.go:ServeHTTP` / `dispatchAuthenticated` / `handleJobs`)

Methods come from the `req.Method` checks or `switch req.Method` inside each handler.

| Path | Methods | Handler |
| --- | --- | --- |
| `/health` | any | `serveHealth` (no auth) |
| `/api/auth/login` | POST | `AuthHandlers.Login` (no auth; registered only when auth is wired) |
| `/api/auth/logout` | POST | `AuthHandlers.Logout` (auth wired only) |
| `/api/auth/me` | GET | `AuthHandlers.Me` (auth wired only) |
| `/api/auth/change-password` | POST | `AuthHandlers.ChangePassword` (auth wired only) |
| `/api/flows` | GET, POST | `handleFlows` |
| `/api/roles` | GET, POST, DELETE (`?id=`) | `handleRoles` |
| `/api/docs` | GET | `handleDocs` |
| `/api/videos` | GET | `handleVideos` |
| `/api/issues` | GET, POST | `handleIssues` |
| `/api/issues/status` | POST | `handleIssueStatus` |
| `/api/issues/notes` | POST | `handleIssueNotes` |
| `/api/skills` | GET | `handleSkills` |
| `/api/skills/logs` | GET, POST | `handleSkillLogs` |
| `/api/skills/stats` | GET | `handleSkillStats` |
| `/api/search` | GET (`?q=` required) | `handleSearch` |
| `/api/cost/overview` | GET | `handleCostOverview` |
| `/api/cost/trend` | GET | `handleCostTrend` |
| `/api/cost/efficiency` | GET | `handleCostEfficiency` |
| `/api/cost/by-feature` | GET | `handleCostByFeature` |
| `/api/analytics` | GET | `handleAnalytics` |
| `/api/learning` | GET, POST, DELETE (`?id=`) | `handleLearning` |
| `/api/learning/navigation` | GET, POST | `handleLearningNavigation` |
| `/api/learning/sequences` | GET, POST | `handleLearningSequences` |
| `/api/learning/context` | GET, POST | `handleLearningContext` |
| `/api/learning/stats` | GET | `handleLearningStats` |
| `/api/ai-cache` | GET, POST, DELETE (`?hash=`) | `handleAICache` |
| `/api/embedding-cache` | GET, POST, DELETE (`?hash=`) | `handleEmbeddingCache` |
| `/api/goal-events` | GET, POST, DELETE (`?id=`) | `handleGoalEvents` |
| `/api/jobs` | GET | `handleListJobs` |
| `/api/jobs/qa/run` | POST (202) | `handleRunQAJob` |
| `/api/jobs/docs/generate` | POST (202) | `handleGenerateDocsJob` |
| `/api/jobs/{id}` | GET | `handleJobs` -> `handleGetJob` |

Any other path returns 404 `Not Found` (`dispatchAuthenticated` default).

## Invariants

- Cookie-session auth is enabled only when the database connects. `cmd/server/main.go` calls `router.SetAuth(...)` inside `if database != nil`. Otherwise `authHandlers` and `requireAuth` stay nil (`router.go:Router` comment), all `/api/*` routes are open, and the `/api/auth/*` routes fall through to 404.
- The allow-list in `router.go:ServeHTTP` (`OPTIONS`, `/health`, `/api/auth/login`) must stay in sync with `auth/middleware.go:IsAuthAllowList`.
- CORS (`router.go:applyCORS`): when auth is wired, the router reflects the request `Origin` instead of `*` and sets `Access-Control-Allow-Credentials: true` and `Vary: Origin`. The configured origin comes from `SetAuth`'s `dashboardOrigin`.
- `/health` (`router.go:serveHealth`) always returns 200 with JSON keys `status`, `database`, `ai_provider` (`aiService.ProviderName()`), `ai_upstream` (`aiService.Upstream()`, `claude-v3` in prod), `ai` (`aiService.Health(ctx)`), `minimax`, and `executor` (`ok|degraded|unreachable|not_configured`; the executor's `/health`, cached for 30 s). `minimax` is `"configured"` only when the provider is `ai.ProviderMiniMax` and `Configured()` is true. It is kept for backward compatibility.
- DB-backed handlers return 503 `Database not available` when `router.go:getDB` returns nil.
- Pagination (`router.go:parsePagination`): page and limit must be greater than 0, and limit is capped at 100.
- `auth_handlers.go:Login` lowercases and trims the username, and returns the same 401 `Invalid username or password` for every credential failure. `auth_helpers.go:lookupForLogin` matches only `active = 1` users.
- `auth_helpers.go:validateNewPassword` requires a new password of at least 8 characters that differs from both the username (case-insensitive) and the current password.
- Jobs (`router.go:enqueueJob`) get IDs `job-<unixnano>`. The in-memory `jobManager` map is guarded by `sync.RWMutex`. Every state change is upserted into `job_runs`.
  - **Executor configured** (`SetExecutor`, from `PLAYWRIGHT_EXECUTOR_URL`): `jobs_executor.go:runExecutorJob` submits `qa_run` / `docs_generate` to playwright-executor. It forwards only `roles`, `max_steps`, `interactive_skills` and `flows` (top-level or under `params`). It polls every 3 s, for up to `PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES`.
  - States: `queued` -> `running` (once the executor starts) -> `completed` | `failed`. The result is the executor's result plus `executor_run_id`.
  - For `qa_run`, issues go into `raw_issues` (`recordExecutorIssues`, `error_type = executor:<type>`, skipping exact duplicates of open issues), and their count is stored in `issues_recorded`.
  - For `docs_generate`, `summarizeDocsRun` adds `documentation_markdown` (ai-manager / claude-v3), or `ai_error` if that call fails.
  - **Not configured:** the job completes after a simulated 200 ms with `result.simulated = true`.
  - At startup, `FailOrphanedJobs` marks `queued`/`running` rows left by a previous process as `failed`.
- `handleSearch` does a `LIKE` substring match on `learning_cache.intent`, returns at most 10 rows, and never calls the vector or AI services.

## Side Effects

- MySQL reads and writes through `db.DB`. Tables touched in `router.go`: `flows`, `roles`, `raw_issues`, `issue_suggestions`, `skills`, `skill_logs`, `learning_cache`, `request_logs` (cost endpoints), `ai_cache`, `embedding_cache`, `goal_events`, `navigation_edges`, `action_sequences`, `job_runs` (`upsertJobRecord` runs `INSERT ... ON DUPLICATE KEY UPDATE`).
- `auth_handlers.go`: `Login` inserts into `sessions` (via `auth.CreateSession`) and sets the HttpOnly, SameSite=Lax cookie (`auth_helpers.go:setSessionCookie`). `Logout` deletes the session row and expires the cookie. `ChangePassword` runs `UPDATE users SET password_hash = ?, must_change_pw = 0`.
- LLM call: `POST /api/issues` calls `aiService.ClassifyIssue` when the category is empty and the service is non-nil, then updates `raw_issues.category`. `/health` calls `aiService.Health`. Both go through the `services/ai` ChatProvider.
- Learning endpoints delegate to `learningService` (`AddNavigationEdge`, `GetNavigationEdges`, `AddActionSequence`, `GetActionSequences`, `AddContextRecord`, `GetContextRecord`, `GetLearningStats`).
- Filesystem reads relative to the process CWD: `handleDocs` reads `../product-documentations`, and `handleVideos` reads `../playwright-docs/output/videos` (`os.ReadDir`).
- Each enqueued job starts a background goroutine (`enqueueJob`).

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/api/...`
- The tests use no build tags. `router_test.go` runs without a DB. The integration tests (`crud_integration_test.go`, `search_integration_test.go`) call `t.Skip` when no database URL is configured or the DB is unreachable. `testenv_integration_test.go:loadIntegrationEnvFiles` loads `DATABASE_URL` (or `NEON_DB_CONN`) from nearby `.env` files.
- When a DB is reachable, `setupCRUDIntegrationRouter` runs `db.RunMigrations` and the CRUD tests write rows to that database. Point the tests at a disposable MySQL instance.
- Some search tests also skip when the MiniMax API key is not configured.
