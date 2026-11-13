# Tutorial Running Integration Tests

## Scope

This guide is for running **integration tests** in `ai-backend/internal/api`:

1. MySQL-backed CRUD integration tests.
2. Search/health integration tests that use a real DB and the configured AI provider.

## Prerequisites

1. Go installed.
2. A reachable, **disposable** MySQL 8 database. The tests run migrations and write rows, so never point them at the production `qa_automation` schema.
3. ai-manager credentials for the search integration tests.

The test suite can read env values from:
- `ai-backend/.env`
- project root `.env`
- `_docs/markdown-results/by-roles/.env`

Values already set in the environment win over values from these files.

## Recommended Env Setup

```bash
export DB_HOST=127.0.0.1 DB_PORT=3306 DB_USER=qa_test DB_PASSWORD=... DB_NAME=qa_automation_test
# or a go-sql-driver/mysql DSN:
# export DATABASE_URL='qa_test:...@tcp(127.0.0.1:3306)/qa_automation_test?parseTime=true'
```

## Run Integration Tests Only

```bash
cd ai-backend
go test -mod=mod ./internal/api -count=1 -v -run 'Test(RolesCRUDIntegration|FlowsCRUDIntegration|IssuesCRUDIntegration|LearningCRUDIntegration|GoalEventsCRUDIntegration|AICacheCRUDIntegration|EmbeddingCacheCRUDIntegration|SearchIntegration|SearchWithDatabase|HealthEndpointWithDB)'
```

## What These Tests Do

1. Initialize integration env and config.
2. Connect to MySQL.
3. Run DB migrations once for the suite.
4. Execute API-level CRUD scenarios with a real DB.
5. Validate search and health behavior on a real DB.

## Expected Result

```text
PASS
ok   github.com/qa-automation/ai-backend/internal/api  <duration>
```

## Skip Conditions

Tests skip (instead of fail) when:

1. No database is configured or the database is unreachable.
2. The AI provider is not configured (search integration tests).

## Troubleshooting

1. `failed to ping database`: verify `DB_*` / `DATABASE_URL` and network reachability.
2. AI auth errors: verify `AI_MANAGER_*`.
3. Migration failures: confirm the DB user can create tables in the test schema.
