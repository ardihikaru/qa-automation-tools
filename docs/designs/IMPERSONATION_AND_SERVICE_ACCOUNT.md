# Impersonation and the AI service account

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Implemented on branch `feat/impersonation-service-account` (owner approved the spec 2026-11-13). Goldfish review 2026-11-13: design, codebase-details and implementation-readiness; re-review "Ready with fixes", and those fixes are applied.
*   **Date:** 2026-11-13

## Overview
This spec adds two things to the qa-automation app at https://qa-tool.mfardiansyah.id:
1.  **A seeded `Service` account for AI agents.** Claude Code sessions use it to call the production API. Before this, the only option was a person's password.
2.  **Impersonation ("log in as").** A user whose real role is `Admin` or `Service` can act as any other active user. The app then behaves as it would for that user, and a banner shows who is really acting. The user leaves with "Return".

## Problem
*   **No account for AI agents.** Seeding (`ai-backend/internal/seed/seed_users.go:35`) creates the taskmgr users plus one optional `ADMIN_USERNAME` admin. In production `ADMIN_*` is empty.
*   **No way to see what a role sees.** Role rules exist only in `productRole` (`ai-backend/internal/api/products.go:49`) and its dashboard copy (`dashboard/src/views/ProductsView.tsx:70-72`). Today, checking them means knowing a real QA user's password.
*   **No impersonation or user-list endpoint.** No code in the repo matches `impersonat`, `login_as`, `act_as` or `original_user`.

## Goals
*   A `Service` role, and one service user seeded from `AI_SERVICE_USERNAME` and `AI_SERVICE_PASSWORD`. The account is never forced to change its password.
*   Real `Admin` or `Service` users can impersonate any other active user, **with the target's full rights** (owner's decision, see Decisions).
*   An audit trail of each impersonation (who, whom, when, how it ended, from where) and one log line per write made while impersonating.
*   In the dashboard, a "Log in as…" picker and an impersonation banner with a Return button.

## Non-Goals
*   User management: no endpoints to create, edit, deactivate or reset users.
*   A permissions or grants table. The role name stays the only authority.
*   Recording the real actor on each data row. The app records no actor on any write today; correlation uses the audit table and the log lines.
*   API tokens. Agents use the cookie session through a curl cookie jar.
*   Login rate limiting. This is a follow-up. A password of 32 or more random characters is the mitigation for now.

## Decisions (owner, 2026-11-13)
1.  The `Service` role and every `Admin` may impersonate.
2.  The AI account gets a new `Service` role. On its own it has no product rights.
3.  A Service impersonation has the target's **full** rights, writes included. The reviewer proposed a read-only default, and the owner declined it. Every write is logged (§4).
4.  The full EGM process is used.

## Current state (cited)
*   **`sessions`** (`ai-backend/internal/db/migrations.go:277`) has columns `token CHAR(64) PK, user_id, expires_at, created_at` and indexes on user and expiry.
*   **`users`** (`migrations.go:259`): `role VARCHAR(32)`. `normalizeRole` (`seed_users.go:172`) maps taskmgr labels onto `Admin, Developer, DevOps, QA, QC, UI/UX Designer`; anything else becomes `Developer`.
*   **Request auth.** `auth.Require` (`ai-backend/internal/auth/middleware.go:18`) calls `ValidateSession` (`auth/session.go:38`). Any error clears the cookie and returns 401 (`middleware.go:27-31`).
*   **Routing.** `ServeHTTP` lets `OPTIONS`, `/api/auth/login` and `/health` through without auth (`ai-backend/internal/api/router.go:557-567`). The auth routes sit in the `if r.authHandlers != nil` switch in `dispatchAuthenticated` (`router.go:638-650`).
*   **CORS.** `applyCORS` reflects the request `Origin` only when `DASHBOARD_ORIGIN` is `*` or empty (`router.go:583-590`). Production pins it to `https://qa-tool.mfardiansyah.id`.
*   **Cookie.** `setSessionCookie` (`ai-backend/internal/api/auth_helpers.go:41`) sets an HttpOnly cookie with SameSite=Lax, `Path=/` and `Secure` taken from env.
*   **Seeding.**
    *   Options come from `os.Getenv` in `ai-backend/cmd/server/main.go:99-106`.
    *   For an existing user, `upsertUser` (`seed/upsert.go:27`) unconditionally updates `role, is_lead, whatsapp, odoo_id`. A new user always gets `must_change_pw=1`.
    *   The Neo4j seed runs before the admin upsert, on every start.
*   **Migrations.** The server never migrates; `cmd/databasemigration` does.

## Technical Plan

### 1. Schema (append to `migrations.go`)
Two new tables. `sessions` is not altered.
```sql
CREATE TABLE IF NOT EXISTS active_impersonations (
  parent_token    CHAR(64) NOT NULL PRIMARY KEY,  -- the real user's session token (the cookie)
  impersonator_id CHAR(36) NOT NULL,
  target_user_id  CHAR(36) NOT NULL,
  audit_id        BIGINT   NOT NULL,
  expires_at      DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS impersonation_audit (
  id              BIGINT AUTO_INCREMENT PRIMARY KEY,
  impersonator_id CHAR(36)    NOT NULL,
  target_user_id  CHAR(36)    NOT NULL,
  started_at      DATETIME    NOT NULL,
  expires_at      DATETIME    NOT NULL,
  ended_at        DATETIME    NULL,
  ended_reason    VARCHAR(16) NULL,   -- stop | logout | superseded ; NULL + expires_at in the past = expired
  ip              VARCHAR(64) NULL,
  user_agent      VARCHAR(255) NULL,
  KEY idx_ia_impersonator (impersonator_id),
  KEY idx_ia_target (target_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```
*   **One token.** The cookie always holds the real user's own session token. An impersonation is a row keyed by that token, so the cookie never changes value.
    *   Ending an impersonation only deletes the row.
    *   There is no parent token to restore and no orphan session.
    *   A target-scoped token can never be traded for the impersonator's session.
*   **One impersonation per session.** The primary key on `parent_token` enforces it.
*   **Timestamps.** All of them are computed in Go with `time.Now().UTC()` and passed as parameters, as in `CreateSession` (`session.go:24`). No column relies on `CURRENT_TIMESTAMP`.
*   **Retention.** `impersonation_audit` rows are never deleted, and no purge job may touch them. At 44 users this is fine.
*   **Stale rows.** Expired `active_impersonations` rows are ignored by `ValidateSession`. The next start, stop or logout on that session removes them.

### 2. Roles
*   `auth.ImpersonatorRoles = {"Admin", "Service"}` is one shared constant, used by both `CanImpersonate` and `ValidateSession`.
*   `auth.CanImpersonate(u *User) bool` returns true when `u != nil`, `u.Impersonator == nil` and `u.Role ∈ ImpersonatorRoles`.
    *   An impersonated identity always has `Impersonator` set, so impersonations cannot nest. That holds even when the target is an Admin or the Service user.
*   **`normalizeRole` does not accept `Service`.** A taskmgr user labelled "Service" still becomes `Developer`. Only `upsertServiceUser` ever writes `Service`.
*   **`upsertUser` existing-row update** becomes `UPDATE users SET … WHERE id = ? AND role <> 'Service'`, so the Neo4j seed can never demote the service row.
*   **`productRole`** already returns `false, false` for `Service`, as does `ProductsView`. Add `"Service": {false, false}` to `TestProductRole` (`ai-backend/internal/api/products_test.go:37`).
*   **`auth.RequireRealUser(u) bool`** returns false while impersonating.
    *   Every endpoint that changes a user account or its credentials must call it.
    *   Today that is only `ChangePassword`. The rule is written into `auth/README.md`.

### 3. auth package
`User` gains two fields:
*   `Impersonator *Actor` with the JSON tag `impersonator,omitempty`. `Actor` is `{ID, Username, FullName, Role}`.
*   `SessionToken string` with the tag `json:"-"`. It is the cookie token, which the handlers need.
*   `SessionExpires time.Time` with the tag `json:"-"`, filled from `s.expires_at` for both the real and the target identity.

`ValidateSession(token)` runs one query:
```sql
SELECT <real user cols>, s.expires_at,
       ai.target_user_id, ai.expires_at, <target user cols>
FROM sessions s
JOIN users u            ON u.id = s.user_id
LEFT JOIN active_impersonations ai ON ai.parent_token = s.token AND ai.expires_at > UTC_TIMESTAMP()
LEFT JOIN users t       ON t.id = ai.target_user_id
WHERE s.token = ? AND s.expires_at > UTC_TIMESTAMP()
```

| Case | Result |
|---|---|
| no session row, or session expired | `ErrSessionInvalid`, as today |
| real user inactive | `ErrSessionInvalid`, as today |
| no active, unexpired impersonation row | the real user, `Impersonator=nil` |
| active row, real user still Admin/Service, target exists and is active | the **target** user, with `Impersonator` set to the real user and `MustChangePW=false` |
| active row, but the real user is no longer Admin/Service, or the target is missing or inactive | the **real** user, `Impersonator=nil` (the impersonation is ignored) |
| SQL error | wrapped error, logged with `log.Printf` so that, for example, a missing table shows up in journalctl; the middleware still returns 401 |

*   Target columns are scanned into `sql.NullString`, `sql.NullBool` and `sql.NullTime`.
*   "Target missing" means `t.id IS NULL`.
*   The two expiry columns are aliased `s_expires_at` and `ai_expires_at`.
*   Keep `LIMIT 1`.

Every failure falls back to the real, authenticated identity. No state ever yields a target identity without the impersonator marker. Because the row is keyed by the real session, revoking or expiring that session ends the impersonation as well.

New file `ai-backend/internal/auth/impersonate.go`. Token generation stays in `CreateSession`; nothing new is needed.
*   **`StartImpersonation(ctx context.Context, db *sql.DB, parentToken string, parentExpires time.Time, impersonatorID, targetID, ip, ua string) (auditID int64, exp time.Time, err error)`** runs in one transaction:
    1.  `SELECT … FOR UPDATE` any existing `active_impersonations` row for `parentToken`.
        *   If its `expires_at > now`, set its audit row's `ended_at=now` and `ended_reason='superseded'`.
        *   If it has expired, leave its audit row untouched, so it reads as expired.
        *   In both cases, delete the active row.
        *   Over HTTP this only happens for an expired row or when two starts race, because a session that is impersonating gets 403.
    2.  Compute `exp = min(parentExpires, now+1h)` in UTC.
    3.  Insert the audit row, then the active row.
    4.  Return `auditID` and `exp`.
*   **`EndImpersonation(ctx context.Context, db *sql.DB, parentToken, reason string) error`** runs in one transaction.
    *   It deletes the active row.
    *   It sets the audit row's `ended_at` and `ended_reason` only when the row's `expires_at > now`. An expired row is deleted without touching its audit row.
    *   It is a no-op if there is no row.

### 4. API
All handlers are methods on `*AuthHandlers` in a new `ai-backend/internal/api/impersonation.go`. The routes are added to the `if r.authHandlers != nil` switch in `dispatchAuthenticated`. With no DB they do not exist and fail closed. The allow-list does not change.

**Shared response shape.** `Me`, `impersonate` (200) and `stop` (200) all return `{"user": User, "mustChangePassword": bool}`.
*   `user.impersonator` is present only while impersonating.
*   Both `mustChangePassword` flags are `false` while impersonating.

**CSRF.** The new POST handlers return 415 unless `Content-Type` starts with `application/json`.
*   Cross-origin JSON requests then need a preflight, and `applyCORS` only allows the pinned origin.
*   This also blocks the `text/plain` no-preflight POST that a sibling `*.mfardiansyah.id` page could send under SameSite=Lax.
*   The dashboard's `authFetch` already sends that header (`dashboard/src/auth/api.ts:38`).
*   Extending the check to all existing POST endpoints is a follow-up.

| Route | Check order | Behaviour |
|---|---|---|
| `GET /api/users` | 405 if not GET → 403 `forbidden` unless `CanImpersonate` | Returns `[{id, username, fullName, role, active}]` ordered by username, including inactive users and the caller. Never includes `password_hash`. |
| `POST /api/auth/impersonate` `{"username":"…"}` | 405 → 415 → 403 `forbidden` unless `CanImpersonate` → 400 `invalid request body` / empty → lowercase+trim → 404 `user not found` (lookup without `active` filter) → 400 `cannot impersonate yourself` → 409 `user is inactive` | Calls `StartImpersonation` with the client IP (`X-Real-IP`, falling back to `RemoteAddr`) and the user agent. The cookie is unchanged. Returns 200 with the shared shape for the target. Logs `impersonate start actor=<u> target=<u> audit=<id>`. |
| `POST /api/auth/impersonate/stop` | 405 → 415 → 400 `not impersonating` if `Impersonator == nil` | `EndImpersonation(token, "stop")`, then 200 with the shared shape for the real user, re-read with `ValidateSession`. Logs `impersonate stop … reason=stop`. |

*   The impersonate 200 body is built by re-reading `auth.ValidateSession(ctx, db, token)`, the same as stop, so `user.impersonator` comes from the same code path as `/me`.
*   Error statuses:
    *   A `StartImpersonation` error returns 500 `could not start impersonation`.
    *   An `EndImpersonation` error in stop returns 500 `could not stop impersonation`.
    *   A second impersonate while already impersonating returns 403 `forbidden`.

Existing handlers:
*   **`Me`:** the same shape; it now just carries `impersonator`.
*   **`ChangePassword`:** returns 403 `not allowed while impersonating` when `!RequireRealUser`.
*   **`Logout`:** calls `EndImpersonation(token, "logout")` best-effort (an error is logged and ignored), then revokes the session and returns 204 as today. The real user is signed out completely.

**Write log.** `auth.Require` logs one line per non-GET request while `Impersonator != nil`, after the handler runs: `impersonated-write actor=<u> target=<u> method=<m> path=<p> status=<code>`. It uses a small status-capturing `ResponseWriter` wrapper.
*   The wrapper records 200 if the handler never calls `WriteHeader`.
*   Stop and logout requests made while impersonating are logged too, which is expected.

### 5. Seeding
*   `seed.Options` gains `ServiceUsername` and `ServicePassword`. `cmd/server/main.go` fills them from `AI_SERVICE_USERNAME` and `AI_SERVICE_PASSWORD`, the same way as `ADMIN_*`.
*   `userRow` gains `MustChangePW bool`, and the `INSERT` uses it. Existing callers pass `true`.
*   New `upsertServiceUser`, based on `upsertAdmin` (`seed_users.go:124`). It runs after the Neo4j seed and the admin upsert, and before `countUsers`, whether or not `ADMIN_*` is set. It:
    1.  Lowercases and trims the username.
    2.  Warns and skips if the password is shorter than 32 characters.
    3.  Reads the existing row. If the row exists with a role other than `Service`, it logs an error and skips; it never promotes a human account. If the row exists as `Service`, it does nothing (the password is never overwritten).
    4.  Otherwise it inserts the user with role `Service`, `is_lead=0` and `must_change_pw=0`.
*   **Password rotation.** The service account calls the existing `POST /api/auth/change-password` while not impersonating, and the new value is then written to `.env.prod` and the workstation credential file.
*   **Username.** Use one that cannot collide with taskmgr, for example `ai-service`.
*   Document both variables (key names only) in `ai-backend/.env.example`, `deploy/qa-tool/.env.prod.example` and the env table of the root `README.md`.

### 6. Dashboard
*   **`auth/api.ts`:**
    *   `AuthUser.impersonator?: {id, username, fullName?, role}`.
    *   New functions `listUsers()`, `impersonate(username)` and `stopImpersonation()`.
    *   `ROLE_BADGE.Service` uses a distinct colour (`bg-cyan-100 text-cyan-700 ring-cyan-200`); the fallback would otherwise show it in the Admin colours.
    *   Correct the header comment that claims a global 401 handler.
*   **`AuthContext`:** `impersonate()` and `stopImpersonation()` call the API, then `window.location.assign('/')`. The full reload ensures no page state from the previous identity survives. A 401 sets the user to null and goes to `/login`.
*   **`components/ImpersonationBanner.tsx`:**
    *   A full-width amber bar (`role="status"`) above `<header>` in `Layout.tsx`.
    *   Text: "Viewing as **X** (role badge). Signed in as **Y**. [Return to Y]". Return is disabled while its request is in flight.
    *   It calls `refresh()` on window focus, so an expired impersonation, which falls back to the real user, clears the banner.
*   **`UserChip.tsx`:**
    *   A new icon button (lucide `UserCog`, `aria-label="Log in as another user"`) to the left of the logout button. It is shown when `user.role ∈ {Admin, Service}` and `!user.impersonator`.
    *   While impersonating, the logout confirmation reads "This signs out \<real name\> completely."
*   **`components/ImpersonateDialog.tsx`:**
    *   Reuses a `Modal` extracted from `ProductsView.tsx:403-417` into `components/Modal.tsx` (Escape key, aria, click outside to close).
    *   It filters by username or full name. Seeded users mostly have no full name, so the username is the main key.
    *   It shows role badges, hides the caller, disables inactive users, and shows the `{error}` text inline.
*   **`ProtectedRoute`:** redirects `/change-password` to `/` while `user.impersonator` is set.

### 7. AI tooling (this repo)
*   **Skill `.claude/skills/qa-tool-session/SKILL.md`:**
    *   Credentials live in `~/.config/qa-automation/ai-service.env` (mode 600, keys `QA_TOOL_URL`, `QA_TOOL_USERNAME`, `QA_TOOL_PASSWORD`).
    *   Load them with `set -a; . file; set +a`. Never `cat` or echo the file, and pass the password to curl through stdin (`--data @-`).
    *   Use a cookie jar in the session scratchpad and send `Content-Type: application/json` on every request.
    *   Smoke sequence and expected statuses:
        1.  `/health` returns 200 with the DB connected.
        2.  login returns 200 with role `Service`.
        3.  `/api/users` returns 200.
        4.  impersonating a QA user returns 200, and `user.impersonator.role` is `Service`.
        5.  `GET /api/products` returns 200; a product POST returns 403 (QA cannot manage).
        6.  stop returns 200 with role `Service`.
        7.  logout returns 204.
    *   Always stop and log out at the end, and delete the jar.
*   **Agent `.claude/agents/qa-tool-operator.md`:** tools are Bash, Read, Grep and Glob, with `skills: [qa-tool-session]`. It checks production behaviour as a named user or role and reports back. It never changes passwords or users, and it makes writes only when the requesting prompt explicitly asks for them.
*   **Rule `.claude/rules/qa-tool-ai-account.md`** (no `paths`, so it always loads):
    *   AI agents use only the Service account and never a human's password; they impersonate instead.
    *   Writes while impersonating need an explicit user request.
    *   Always stop and log out.
    *   The credential file is a secret.
*   **Secrets:** add the credential file to `.claude/rules/secrets.md` and to the `.claude/settings.json` Read deny list.

## Security analysis
*   **The Service account is equivalent to an Admin,** with write access through impersonation, by the owner's decision.
    *   Its password must be at least 32 random characters, stored only in `.env.prod` on app-host and the owner's workstation file.
    *   The agent-side guardrail is the rule file plus the operator agent's instructions.
    *   The server-side trace is the audit row plus one `impersonated-write` log line per write.
*   **No new privilege beyond an Admin's.** The new power is acting under another user's name, and that is always marked by the banner and audited.
*   **CSRF.** The new endpoints require a JSON `Content-Type`, so cross-origin calls need a preflight, which `applyCORS` rejects for any origin other than `DASHBOARD_ORIGIN`. The `DASHBOARD_ORIGIN=*` reflect mode is a dev-only misconfiguration; the production template pins it.
*   **Containment.** An impersonation lasts at most one hour, bounded by the parent session. It dies with the parent session (same key), cannot nest, and cannot change the target's password. Every invalid state falls back to the real user.

## Assumptions Goldfish reviewers must challenge
1.  Role names (Admin, Service) are enough as the impersonation permission. Owner decision: no read-only mode.
2.  The single-token design (row keyed by the real session) has no state in which a request acts as the target without `Impersonator` set.
3.  Falling back to the real user, rather than returning 401, when an impersonation is invalid is safe, because the real user was authenticated independently.
4.  A JSON-only `Content-Type` check on the new POST endpoints is enough CSRF protection, given the pinned `DASHBOARD_ORIGIN`.
5.  `RequireRealUser` on `ChangePassword` covers every account-changing endpoint that exists today.
6.  `upsertUser`'s `AND role <> 'Service'` guard plus `normalizeRole` refusing `Service` stop taskmgr from creating or demoting Service accounts.

## Rollout (each step needs its own approval)
1.  Merge the implementation PR to `main`.
2.  **Migrate first.** On app-host, run `go run -mod=mod ./cmd/databasemigration` against prod *before* deploying the new backend.
    *   The new code queries `active_impersonations` on every request. Without the table, every request returns 401 and every user is logged out.
    *   The new tables are unused by the old code, so migrating while it runs is safe.
3.  Generate the service password with `openssl rand -base64 36`, never echoed. Write it to the app-host `.env.prod` (`AI_SERVICE_USERNAME=ai-service` and `AI_SERVICE_PASSWORD`) and to `~/.config/qa-automation/ai-service.env` on the workstation, both mode 600.
4.  Deploy with the `deploy-qa-tool` skill. Seeding runs at startup, so the service user appears on this restart.
5.  Run the skill's smoke sequence.

**Rollback:** redeploy the old image. The old code ignores both tables, so any live impersonation simply stops applying. Leave the tables and the Service user in place.

## Target Files
*   **Create:**
    *   Backend:
        *   `ai-backend/internal/auth/impersonate.go` + `impersonate_test.go`
        *   `ai-backend/internal/api/impersonation.go`
        *   `ai-backend/internal/api/impersonation_test.go`: handler 405/415/403/400/404/409 and `ChangePassword` 403, using a stubbed context user
        *   `ai-backend/internal/api/impersonation_integration_test.go`
        *   `ai-backend/internal/auth/middleware_test.go`: the status wrapper defaults to 200, and exactly one `impersonated-write` line is logged per non-GET request while impersonating, none otherwise
        *   `ai-backend/internal/seed/seed_users_test.go`
    *   Dashboard: `dashboard/src/components/{ImpersonationBanner,ImpersonateDialog,Modal}.tsx`
    *   E2E: `this-qa-automation-system-playwright/tests/api/impersonation.spec.ts`
    *   AI tooling: `.claude/skills/qa-tool-session/SKILL.md` + `qa-tool.sh` (helper script), `.claude/agents/qa-tool-operator.md`, `.claude/rules/qa-tool-ai-account.md`
*   **Modify:**
    *   Backend:
        *   `ai-backend/cmd/server/main.go`
        *   `ai-backend/internal/db/migrations.go`
        *   `ai-backend/internal/auth/{context,session,middleware}.go`
        *   `ai-backend/internal/api/{auth_handlers,router,products_test}.go`
        *   `ai-backend/internal/seed/{seed_users,upsert}.go`
    *   Dashboard: `dashboard/src/auth/{api.ts,AuthContext.tsx,ProtectedRoute.tsx}`, `dashboard/src/components/{UserChip,Layout}.tsx`, `dashboard/src/views/ProductsView.tsx` (Modal extraction)
    *   E2E: `this-qa-automation-system-playwright/{config.ts,README.md}`
    *   Env templates: `ai-backend/.env.example`, `deploy/qa-tool/.env.prod.example`
    *   READMEs: `README.md`, `ai-backend/README.md`, `ai-backend/internal/{auth,api,db}/README.md`, `dashboard/README.md`, `deploy/qa-tool/README.md`
    *   Claude config: `.claude/rules/secrets.md`, `.claude/settings.json`, `CLAUDE.md`, `dashboard/CLAUDE.md` (role list note)
    *   Design docs: `docs/designs/SITEMAP.md`

## Verification Plan
*   From `ai-backend/`:
    *   `gofmt -l` on the touched files
    *   `go build -mod=mod ./... && go test -mod=mod ./...`
*   **Unit tests:**
    *   `CanImpersonate`: each role, nil, and nested.
    *   `normalizeRole("Service") == "Developer"`.
    *   `productRole` for `Service`.
    *   The expiry `min()`.
    *   Handler status codes and messages: 405, 415, 403, 400, 404, 409, plus `ChangePassword` 403 while impersonating.
*   **Integration test** (`impersonation_integration_test.go`):
    *   It is gated like `crud_integration_test.go:30-77`, wires `AuthHandlers` and `auth.Require` itself, and creates and cleans up its own random users.
    *   Asserts:
        *   start makes `/me` the target with the impersonator set
        *   stop makes it the real user again
        *   a second `auth.StartImpersonation` call on the same parent token, made directly rather than over HTTP, supersedes the first, and its audit row reads `superseded`
        *   a start over an expired row leaves the old audit row's `ended_reason` NULL
        *   `POST /api/auth/impersonate` while impersonating returns 403 `forbidden`
        *   logout while impersonating invalidates the token
        *   an expired row falls back to the real user
        *   a target deactivated mid-session falls back to the real user
        *   an impersonator demoted to QA falls back
        *   `/api/users` JSON has no password field
        *   the Neo4j-style `upsertUser` does not change a `Service` row's role
*   **Dashboard:** `npx tsc --noEmit && npm run build`.
*   **E2E test** (`tests/api/impersonation.spec.ts`):
    *   It skips unless `E2E_IMPERSONATOR_USERNAME`, `E2E_IMPERSONATOR_PASSWORD` and `E2E_TARGET_USERNAME` are set.
    *   Audit rows are kept by design, which is an accepted exception to the suite's clean-up rule.
*   **After deploy:** run the skill's smoke sequence against production.

## Review outcome (2026-11-13)
Changes made in response to the review:
*   **Single-token design.** It replaced the cookie swap, which had in-flight requests deleting the restored cookie, lockout at the 1-hour cap, orphan sessions, and a target token that could be traded for the parent session.
*   **Fail-safe fallback to the real user.** It replaced a JOIN filter that failed open.
*   **The impersonation dies with its parent session.**
*   **Audit table** with the end reason, IP and user agent, plus a log line per write.
*   **`normalizeRole` refuses `Service`,** and the Neo4j seed cannot demote the Service row.
*   **Corrected premises:** the CORS reflection claim, plus a JSON `Content-Type` check.
*   **Migrate-before-deploy rollout,** and the `cmd/server/main.go` wiring.
*   **Exact response shapes, check order and status codes.**
*   **Dashboard reload on identity change,** and a concrete UI.
*   **Corrected test paths and gating.**

Declined: a read-only default for Service impersonations (owner decision 3).
Follow-ups:
*   Login rate limiting.
*   The `Content-Type` check on all POST endpoints.
*   Removing `applyCORS` reflect mode.

## Implementation notes (2026-11-13)
*   **Time zones.** The DSN used `loc=Local`, so on a non-UTC host Go timestamps were stored in local time while SQL compared them with `UTC_TIMESTAMP()`. On a UTC+7 host, sessions outlived their TTL by 7 h. The fix (2026-11-13, `fix/session-timezone`) is `db.NormalizeDSN`, which pins every connection to UTC (`loc=UTC`, session `time_zone='+00:00'`). Production (container and MySQL both UTC) was unaffected.
*   **Verification.**
    *   Unit tests: `internal/auth`, `internal/seed`, `internal/api`.
    *   `impersonation_integration_test.go` against a disposable `mysql:8.0` container.
    *   `tests/api/impersonation.spec.ts` (3/3 pass) and `qa-tool.sh smoke`, against a local backend on that container.
    *   `npm run build`.
*   **Not yet verified:** browser checks of the dashboard UI, and production. Production needs the migration, the env and a deploy, each a separate approval.
