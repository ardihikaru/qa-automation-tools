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
- `auth_handlers.go:AuthHandlers{DB *sql.DB, CookieName string, SessionTTL time.Duration, Secure bool}` with methods `Login`, `Logout`, `Me`, `ChangePassword`, and in `impersonation.go` `ListUsers`, `Impersonate`, `StopImpersonation`.

### Routes (as registered in `router.go:ServeHTTP` / `dispatchAuthenticated` / `handleJobs`)

Methods come from the `req.Method` checks or `switch req.Method` inside each handler.

| Path | Methods | Handler |
| --- | --- | --- |
| `/health` | any | `serveHealth` (no auth) |
| `/api/auth/login` | POST | `AuthHandlers.Login` (no auth; registered only when auth is wired) |
| `/api/auth/logout` | POST | `AuthHandlers.Logout` (auth wired only) |
| `/api/auth/me` | GET | `AuthHandlers.Me` (auth wired only) |
| `/api/auth/change-password` | POST | `AuthHandlers.ChangePassword` (auth wired only; 403 while impersonating) |
| `/api/auth/impersonate` | POST | `AuthHandlers.Impersonate` (auth wired only; real role Admin/Service) |
| `/api/auth/impersonate/stop` | POST | `AuthHandlers.StopImpersonation` (auth wired only) |
| `/api/users` | GET | `AuthHandlers.ListUsers` (auth wired only; real role Admin/Service) |
| `/api/flows` | GET, POST | `handleFlows` |
| `/api/products` | GET, POST | `handleProducts` (`products.go`): list products with their `repos` (main first, then central, then by name), `allow_non_automation_work`, and the legacy fields mirrored from the main repo. POST takes `{name, staging_url, repos:[...]}` (exactly one `is_main`) or the legacy flat `{repo_url, root_dir, branch}` (one repo named `main`; both forms is 400). Writes: Admin/DevOps/Developer |
| `/api/products/{id}` | GET, PUT, DELETE | PUT applies `name`, `staging_url` and `allow_non_automation_work` (Admin/DevOps only; another role changing it gets 403 and nothing is saved); legacy `repo_url`/`root_dir`/`branch` in the body are ignored with 200. DELETE removes registry rows (repos, branches, settings cascade), never checkouts |
| `/api/products/{id}/repos` | POST | Add a repo (`product_repos.go`); `is_main:true` promotes atomically (old main becomes a sibling), `is_central:true` moves the central flag. 201 Product |
| `/api/products/{id}/repos/{rid}` | PUT, DELETE | PUT is partial (`name`, `repo_url`, `root_dir`, `is_main`, `is_central`); `is_main:false` on the main and DELETE of the main are 409. DELETE of the central repo is allowed. 200 Product |
| `/api/products/{id}/repos/{rid}/branches` | PUT | Replaces the branch list and the single default in one tx (1 to 10 branches; legacy empty list may stay empty). Does not touch disk |
| `/api/products/{id}/repos/{rid}/verify` | POST | `repopool.VerifyRepo` (main repo: with Playwright check and staging probe) and stores that repo's report. Also run per repo by `StartProductVerifier` (staging probed once per product). Admin/DevOps/Developer/QA/QC |
| `/api/products/{id}/repos/{rid}/clone`, `/checkout` | POST | Clone (clone only) then `SyncBranches` / `SyncBranches` for every listed branch; 200 `{product, results:[BranchResult]}`; the report is stored in `sync_report`. Detached 10 min context. 409 when the repo is busy |
| `/api/products/{id}/verify`, `/clone`, `/checkout` | POST | Legacy routes, all on the main repo; `checkout` takes optional `{branch}` (made the main repo's default, added to the list if absent). 409 `product has no main repo` for a repo-less product |
| `/api/settings` | GET | `handleSettings` (`settings.go`): non-secret config snapshot injected by `SetSettings`; returns gateway host only (no URL path, userinfo or keys) and `pinned_to_claude_v3` |
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
- CORS (`router.go:applyCORS`): the configured origin is sent as-is. Only when it is `*` or empty, and auth is wired, does the router reflect the request `Origin`. With auth wired it also sets `Access-Control-Allow-Credentials: true` and `Vary: Origin`. Production pins `DASHBOARD_ORIGIN`, so it never reflects. The configured origin comes from `SetAuth`'s `dashboardOrigin`.
- `/health` (`router.go:serveHealth`) always returns 200 with JSON keys `status`, `database`, `ai_provider` (`aiService.ProviderName()`), `ai_upstream` (`aiService.Upstream()`, `claude-v3` in prod), `ai` (`aiService.Health(ctx)`), `minimax`, and `executor` (`ok|degraded|unreachable|not_configured`; the executor's `/health`, cached for 30 s). `minimax` is `"configured"` only when the provider is `ai.ProviderMiniMax` and `Configured()` is true. It is kept for backward compatibility.
- DB-backed handlers return 503 `Database not available` when `router.go:getDB` returns nil.
- Pagination (`router.go:parsePagination`): page and limit must be greater than 0, and limit is capped at 100.
- `auth_handlers.go:Login` lowercases and trims the username, and returns the same 401 `Invalid username or password` for every credential failure. `auth_helpers.go:lookupForLogin` matches only `active = 1` users.
- **Impersonation** ([spec](../../../docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md)):
  - `Me`, `Impersonate` (200) and `StopImpersonation` (200) all return `{"user": User, "mustChangePassword": bool}`, built from `auth.ValidateSession`. `user.impersonator` is present only while impersonating.
  - `Impersonate` checks run in this order: 405, then 415 (the `Content-Type` must be `application/json`), then 403 `forbidden` (not `auth.CanImpersonate`, which includes nesting), then 400 `invalid request body`, then 404 `user not found`, then 400 `cannot impersonate yourself`, then 409 `user is inactive`, then 500 `could not start impersonation`. The cookie is never changed.
  - `StopImpersonation`: 405, 415, 400 `not impersonating`, 500 `could not stop impersonation`.
  - `ListUsers`: 405, 403. It returns `id, username, fullName, role, active` and never the password hash.
  - `Logout` while impersonating ends the impersonation (audit reason `logout`) and revokes the real session.
- `auth_helpers.go:validateNewPassword` requires a new password of at least 8 characters that differs from both the username (case-insensitive) and the current password.
- Jobs (`router.go:enqueueJob`) get IDs `job-<unixnano>`. The in-memory `jobManager` map is guarded by `sync.RWMutex`. Every state change is upserted into `job_runs`.
  - **Executor configured** (`SetExecutor`, from `PLAYWRIGHT_EXECUTOR_URL`): `jobs_executor.go:runExecutorJob` submits `qa_run` / `docs_generate` to playwright-executor. It forwards only `roles`, `max_steps`, `interactive_skills` and `flows` (top-level or under `params`). It polls every 3 s, for up to `PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES`.
  - States: `queued` -> `running` (once the executor starts) -> `completed` | `failed`. The result is the executor's result plus `executor_run_id`.
  - For `qa_run`, issues go into `raw_issues` (`recordExecutorIssues`, `error_type = executor:<type>`, skipping exact duplicates of open issues), and their count is stored in `issues_recorded`.
  - For `docs_generate`, `summarizeDocsRun` adds `documentation_markdown` (ai-manager / claude-v3), or `ai_error` if that call fails.
  - **Not configured:** the job completes after a simulated 200 ms with `result.simulated = true`.
  - At startup, `FailOrphanedJobs` marks `queued`/`running` rows left by a previous process as `failed`.
- `handleSearch` does a `LIKE` substring match on `learning_cache.intent`, returns at most 10 rows, and never calls the vector or AI services.

## Product repo pool invariants

- `services/repopool` is the only code that runs `gh`/`git`. Inputs are validated (`repo_url` must be `https://github.com/<owner>/<repo>`; `root_dir` relative with no `..`; branch conservative) and commands use argument slices, never a shell.
- Every path is resolved inside `REPO_POOL_DIR` (symlink escapes rejected). Every pool operation takes a per-repo-directory lock.
- `staging_url` is fetched by the backend during verify (GET, 10 s, no redirects). Only http(s) without credentials is accepted; the dialer refuses loopback, link-local (cloud metadata), unspecified and multicast addresses on the resolved IP. Failure detail is coarse (`unreachable`, `timeout`, `address not allowed`, `HTTP nnn`).
- Product access **fails closed**: no authenticated user means no access (`productRole`). Repo `root_dir` footprints (`root_dir` and `root_dir.branches`) may not overlap across all repos of all products. Verification results are written only if repo URL/dir (and, for the main repo, staging URL and main role) are unchanged since the check started.
- `playwright_accessible` means: all checks passed, including staging reachable **from the backend container** and a heuristic Playwright dependency/config in the checkout. It does not prove Playwright runs on the host.
- `GH_TOKEN` is read from the process env by `gh`/`git`, never logged or returned.
- **Multi-repo invariants (PRODUCT_REPOS.md).** One main repo (DB-enforced at most one; handlers never delete or unset it) and at most one central repo per product; at most one default branch per repo; names unique per product; `repo_url` compared with `repopool.NormalizeRepo` in handlers (the DB index is exact-string). Every mutating handler runs in `db.WithTx` whose first statement is `SELECT ... FROM products WHERE id=? FOR UPDATE`; git work happens outside any transaction. A 1062 is mapped to 409 by key name (`writeDBConflict`); other DB errors are 500 with a generic text.
- `mirrorMainToLegacy` is the only writer of the legacy `products.repo_url/root_dir/branch/playwright_accessible/last_verified_at/verify_report` columns (kept for rollback); it runs in the same tx as every change to the main repo.
- **Write guard.** Any endpoint that creates a remote branch, pushes or opens a PR MUST call `repoWriteAllowed(ctx, db, repoID, op)` first (main repo or `allow_non_automation_work`). No route does yet; clone, sync and verify are read-class. `SyncBranches` also disables the origin push URL for non-main repos while the flag is off.
- Repo-less products (backfill conflict or rollback window) list with `repos: []`; repo routes return 409 `product has no main repo` except `POST /repos` with `is_main:true`; the scheduled verifier skips them.

## Side Effects

- MySQL reads and writes through `db.DB`. Tables touched in `router.go`: `flows`, `roles`, `raw_issues`, `issue_suggestions`, `skills`, `skill_logs`, `learning_cache`, `request_logs` (cost endpoints), `ai_cache`, `embedding_cache`, `goal_events`, `navigation_edges`, `action_sequences`, `job_runs` (`upsertJobRecord` runs `INSERT ... ON DUPLICATE KEY UPDATE`).
- `auth_handlers.go`: `Login` inserts into `sessions` (via `auth.CreateSession`) and sets the HttpOnly, SameSite=Lax cookie (`auth_helpers.go:setSessionCookie`). `Logout` deletes the session row and expires the cookie. `ChangePassword` runs `UPDATE users SET password_hash = ?, must_change_pw = 0`.
- Products: clone/sync write under `REPO_POOL_DIR` (worktrees in `<root_dir>.branches/<slug>`); tables `products`, `product_settings`, `product_repos`, `product_repo_branches`; verify makes one outbound GET to `staging_url`.
- LLM call: `POST /api/issues` calls `aiService.ClassifyIssue` when the category is empty and the service is non-nil, then updates `raw_issues.category`. `/health` calls `aiService.Health`. Both go through the `services/ai` ChatProvider.
- Learning endpoints delegate to `learningService` (`AddNavigationEdge`, `GetNavigationEdges`, `AddActionSequence`, `GetActionSequences`, `AddContextRecord`, `GetContextRecord`, `GetLearningStats`).
- Filesystem reads relative to the process CWD: `handleDocs` reads `../product-documentations`, and `handleVideos` reads `../playwright-docs/output/videos` (`os.ReadDir`).
- Each enqueued job starts a background goroutine (`enqueueJob`).

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/api/...`
- The tests use no build tags. `router_test.go` runs without a DB. The integration tests (`crud_integration_test.go`, `search_integration_test.go`) call `t.Skip` when no database URL is configured or the DB is unreachable. `testenv_integration_test.go:loadIntegrationEnvFiles` loads `DB_*`/`DATABASE_URL` from nearby `.env` files.
- When a DB is reachable, `setupCRUDIntegrationRouter` runs `db.RunMigrations` and the CRUD tests write rows to that database. Point the tests at a disposable MySQL instance.
- Some search tests also skip when the MiniMax API key is not configured.
