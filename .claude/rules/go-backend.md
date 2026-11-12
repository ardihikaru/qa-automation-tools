---
paths:
  - "ai-backend/**/*.go"
---

# Go backend rules

- Build/test with `-mod=mod` (stale gitignored `vendor/`).
- LLM access only through `*ai.Service`. A new provider means a new `ChatProvider` registered in `ai.NewFromConfig`, with unit tests that use `httptest.Server`.
- Schema changes: append idempotent statements to `internal/db/migrations.go`. Never edit an applied migration, and never migrate from `cmd/server`.
- New routes: add a `case` in `dispatchAuthenticated` (`internal/api/router.go`). Never add paths to `IsAuthAllowList` without an explicit decision in a spec.
- New config: env first (`getEnvString`/`getEnvInt` in `internal/config/config.go`), then document it in `.env.example` **and** `deploy/qa-tool/.env.prod.example`.
- Keep `/health` keys stable (`status`, `database`, `ai_provider`, `ai`, `minimax`). Probes and E2E tests read them.
- If the change alters a package's interface or invariants, update that package's README contract in the same commit.
