# Tutorial Running Integration Tests

## Scope

This guide is for running **integration tests** in `ai-backend/internal/api`:

1. Neon-backed CRUD integration tests.
2. Search/health integration tests that use real DB and configured MiniMax settings.

## Prerequisites

1. Go installed.
2. Accessible Neon connection string.
3. MiniMax key for search integration tests.

The test suite can read env values from:
- `ai-backend/.env`
- project root `.env`
- `_docs/markdown-results/by-roles/.env` (`NEON_DB_CONN`, `MINIMAX_API_KEY`)

If `DATABASE_URL` is empty but `NEON_DB_CONN` exists, tests map it automatically.

## Recommended Env Setup

From repo root:

```bash
cd ~/devel/projects/qa-automation-tools
export DATABASE_URL='postgresql://...'
export MINIMAX_API_KEY='sk-api-...'
```

## Run Integration Tests Only

```bash
cd ai-backend
go test ./internal/api -count=1 -v -run 'Test(RolesCRUDIntegration|FlowsCRUDIntegration|IssuesCRUDIntegration|LearningCRUDIntegration|GoalEventsCRUDIntegration|AICacheCRUDIntegration|EmbeddingCacheCRUDIntegration|SearchIntegration|SearchWithDatabase|HealthEndpointWithDB)'
```

## What These Tests Do

1. Initialize integration env and config.
2. Connect to Neon.
3. Run DB migrations once for the suite.
4. Execute API-level CRUD scenarios with real DB.
5. Validate search and health behavior on real DB.

## Expected Result

Passing run ends with:

```text
PASS
ok   github.com/qa-automation/ai-backend/internal/api  <duration>
```

## Skip Conditions

Tests can skip (instead of fail) when:

1. `DATABASE_URL`/Neon connection is unavailable.
2. MiniMax key is unavailable (search integration tests).

This behavior is intentional for safer local/CI execution.

## Troubleshooting

1. `failed to ping database`:
- Verify `DATABASE_URL` (or `NEON_DB_CONN`) is valid and reachable.

2. MiniMax auth errors:
- Verify `MINIMAX_API_KEY` and endpoint config.

3. Migration-related failures:
- Re-run test after confirming DB user privileges on Neon schema.

4. Need full backend run after integration:

```bash
cd ai-backend
go test ./... -count=1
```
