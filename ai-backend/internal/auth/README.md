# auth

## Overview

Package `auth` provides cookie-session authentication primitives for the
ai-backend. It covers bcrypt password hashing, MySQL-backed session tokens, a
request-context user, and `net/http` middleware. The HTTP endpoints that use
it (`/api/auth/*`) live in [`api`](../api/README.md), and `cmd/server/main.go`
wires them up only when the database connects.

## Interface

- `context.go:User`: the principal on the request context (`ID`, `Username`, `FullName`, `Email`, `WhatsApp`, `OdooID`, `Role`, `IsLead`, `Active`, `MustChangePW`). It never carries the password hash.
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

## Side Effects

- MySQL `sessions` table: `INSERT` (`CreateSession`), `SELECT ... JOIN users` (`ValidateSession`), `DELETE` by token (`RevokeSession`), and `DELETE` of expired rows (`PurgeExpiredSessions`).
- HTTP: `Require` writes 401 or redirect responses, and expires the cookie (`MaxAge: -1`, HttpOnly, SameSite=Lax, Path `/`) on invalid sessions.
- No other network or disk access.

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/auth/...`. The package currently has no test files. Its behavior is exercised indirectly through `go test -mod=mod ./internal/api/...`.
