# auth

## Overview

Package `auth` provides cookie-session authentication primitives for the
ai-backend. It covers bcrypt password hashing, MySQL-backed session tokens, a
request-context user, and `net/http` middleware. The HTTP endpoints that use
it (`/api/auth/*`) live in [`api`](../api/README.md), and `cmd/server/main.go`
wires them up only when the database connects.

## Interface

- `context.go:User`: the principal on the request context (`ID`, `Username`, `FullName`, `Email`, `WhatsApp`, `OdooID`, `Role`, `IsLead`, `Active`, `MustChangePW`, `Impersonator *Actor`, and the unserialised `SessionToken`/`SessionExpires`). It never carries the password hash. While impersonating it is the **target**, and `Impersonator` is the real user.
- `context.go:Actor{ID, Username, FullName, Role}`, `context.go:ImpersonatorRoles` (`Admin`, `Service`), `context.go:CanImpersonate(*User) bool`, `context.go:RequireRealUser(*User) bool`.
- `impersonate.go:StartImpersonation(ctx, db, parentToken, parentExpires, impersonatorID, targetID, ip, ua) (auditID, expires, err)`, `impersonate.go:EndImpersonation(ctx, db, parentToken, reason) error`, `impersonate.go:ImpersonationExpiry(now, parentExpires)`, `impersonate.go:MaxImpersonation` (1h).
- `context.go:SetUser(ctx, *User) context.Context` and `context.go:GetUser(ctx) (*User, bool)`.
- `middleware.go:Config{DB *sql.DB, CookieName string}`.
- `middleware.go:Require(c Config) func(http.Handler) http.Handler`: session-enforcing middleware.
- `middleware.go:IsAuthAllowList(method, path string) bool`: returns true for `OPTIONS` requests, `/health`, and `/api/auth/login`.
- `password.go:Hash(plain string) (string, error)`, `password.go:Check(hash, plain string) error`, `password.go:ErrInvalidCredentials`.
- `session.go:CreateSession(ctx, db, userID, ttl) (token string, expires time.Time, err error)`.
- `session.go:ValidateSession(ctx, db, token) (*User, error)`, `session.go:ErrSessionInvalid`.
- `session.go:RevokeSession(ctx, db, token) error` and `session.go:PurgeExpiredSessions(ctx, db) error`.

## Invariants

- Passwords use bcrypt at `password.go:bcryptCost = 12`. `Check` returns only `ErrInvalidCredentials` on mismatch and never exposes the bcrypt error.
- Session tokens are 32 bytes from `crypto/rand`, hex-encoded to 64 characters (`session.go:CreateSession`). Expiry is computed in UTC (`time.Now().UTC().Add(ttl)`).
- `session.go:ValidateSession` returns `ErrSessionInvalid` in three cases: the token is empty, it is unknown or expired (`expires_at > UTC_TIMESTAMP()` fails), or the user has `Active == false`.
- `middleware.go:Require` rejects missing or invalid cookies. `/api/*` paths get a JSON 401 `{"error":"unauthorized"}`, and other paths get a 302 redirect to `/login` (`unauthorized`). An invalid session also clears the cookie (`clearCookie`). On success, the middleware calls `SetUser` on the request context.
- `RevokeSession` with an empty or unknown token is a no-op and returns nil.
- `IsAuthAllowList` must stay consistent with the allow-list in `api/router.go:ServeHTTP`.
- **Impersonation** ([spec](../../../docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md)). An impersonation is an `active_impersonations` row keyed by the real session token, so the cookie never changes. `ValidateSession` returns the target with `Impersonator` set only when that row is unexpired, the real user's role is in `ImpersonatorRoles`, and the target exists and is active. In every other case it returns the real user with `Impersonator == nil`. A target identity never exists without `Impersonator`.
- The impersonation expiry is `min(real session expiry, now + 1h)`. Every connection is pinned to UTC (`db.NormalizeDSN`), so Go-written expiries and `UTC_TIMESTAMP()` agree on any host time zone. `TestSessionExpiryIT_HostTimeZone` covers this; run it once with `TZ=Asia/Jakarta`.
- `CanImpersonate` requires `Impersonator == nil`, so impersonations never nest. `MustChangePW` is false for an impersonated identity.
- Every endpoint that changes a user account or its credentials must refuse when `RequireRealUser(u)` is false (today: `ChangePassword`).
- `impersonation_audit` rows are never deleted. `ended_reason` is `stop`, `logout` or `superseded`; NULL with `expires_at` in the past means the impersonation expired.
- `Require` logs `impersonated-write actor=… target=… method=… path=… status=…` for every non-GET/HEAD request made while impersonating.

## Side Effects

- MySQL `sessions` table: `INSERT` (`CreateSession`), `SELECT ... JOIN users` plus `LEFT JOIN active_impersonations`/`users` (`ValidateSession`), `DELETE` by token (`RevokeSession`), and `DELETE` of expired rows (`PurgeExpiredSessions`).
- MySQL `active_impersonations` and `impersonation_audit`: `StartImpersonation` and `EndImpersonation`, each in one transaction with `SELECT ... FOR UPDATE` on the active row.
- Log lines: `auth: session query failed` on SQL errors in `ValidateSession`, and `impersonated-write` from `Require`.
- HTTP: `Require` writes 401 or redirect responses, and expires the cookie (`MaxAge: -1`, HttpOnly, SameSite=Lax, Path `/`) on invalid sessions.
- No other network or disk access.

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/auth/...` (`impersonate_test.go`, `middleware_test.go`).
- The DB-backed impersonation flow is in `api/impersonation_integration_test.go`. It needs `IMPERSONATION_IT_DSN` pointing at a **disposable** MySQL 8, for example `docker run -d --rm --name qa-it-mysql -e MYSQL_ROOT_PASSWORD=itpw -e MYSQL_DATABASE=qa_it -p 127.0.0.1:33306:3306 mysql:8.0`, then `IMPERSONATION_IT_DSN='root:itpw@tcp(127.0.0.1:33306)/qa_it?multiStatements=true'`. The test normalizes the DSN to UTC itself.
