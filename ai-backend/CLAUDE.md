# ai-backend — agent instructions

This is a Go 1.23 REST API. The contract is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md).

## Commands
- Build: `go build -mod=mod ./...`. The `vendor/` directory is stale and gitignored, so always pass `-mod=mod`.
- Test: `go test -mod=mod ./...`. Integration tests skip themselves when their env vars are missing.
- Migrate: `go run -mod=mod ./cmd/databasemigration`. It needs `DB_*`.
- AI smoke test: `go run -mod=mod ./cmd/verifyai ["prompt"]`. It needs `AI_MANAGER_*` and a WireGuard peer.
- Run: `go run -mod=mod ./cmd/server`.

## Conventions
- **Routing:** one `switch` in `internal/api/router.go` (`dispatchAuthenticated`). Add a `case`, then a handler method on `*Router`. Auth bypass is hard-coded in `Router.ServeHTTP` (`OPTIONS`, `/api/auth/login`, `/health`). `auth.IsAuthAllowList` mirrors that list but is not called, so keep the two in sync.
- **DB access** goes through `internal/db.DB` (context-first `Query`/`QueryRow`/`Exec` over `database/sql`, MySQL syntax). Use `?` placeholders and `UTC_TIMESTAMP()`. `db.NewConnection` pins every connection to UTC (`loc=UTC`, session `time_zone='+00:00'`; see `db.NormalizeDSN`), so Go `time.Time` values, `NOW()` and `UTC_TIMESTAMP()` all agree. Open other connections through `NormalizeDSN` too.
- **Schema changes:** append to the ordered list in `internal/db/migrations.go`, using `CREATE TABLE IF NOT EXISTS` with InnoDB and utf8mb4. Never edit a migration that has already been applied.
- **AI:** callers use `*ai.Service` (`Chat`, `ChatMessages`, `ClassifyIssue`, `GenerateSuggestion`, `GenerateSkill`). To add a provider, implement `ChatProvider` (`internal/services/ai/provider.go`) and add it to `NewFromConfig`. Get usage and cost from `LastUsage()` and prefer that over `EstimateCost`.
- **Config:** env always beats `config.yaml`. The YAML loader ignores `backend.database.*` and the AI secrets, so the DB is configured through `DB_*` env only. Defaults are `10.10.0.4:5306`, `qa_automation`. Secrets come only from env. Add new keys to `internal/config/config.go`, `.env.example` and `deploy/his-v3-qa/.env.prod.example`.
- **Health:** `/health` returns `status`, `database`, `ai_provider`, `ai_upstream` and `ai`, and keeps `minimax` for older probes. `database` is a snapshot taken at startup, not a live ping. The ai-manager health result is cached for 30 seconds.
- Run `gofmt` on the files you touch. `router.go` still has formatting debt from before this rule, so don't mass-reformat it in the same commit as a functional change.
