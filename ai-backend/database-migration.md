# Database Migration Guide

This document explains how database migration works in `ai-backend`, including local development and production deployment on Google Cloud Run.

## Overview

- Migration command: `cmd/databasemigration/main.go`
- Migration source: `internal/db/migrations.go`
- Database target: Neon PostgreSQL (only)
- Migration style: ordered, idempotent SQL statements (using `IF NOT EXISTS` and `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`)

## Prerequisites

- Go installed (matching `go.mod` toolchain)
- Neon connection configured via one of:
  - `DATABASE_URL`
  - `backend.database.connection_string` in `config.yaml`
  - `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`
- From repository root, `.env` is loaded automatically by config loader.

## How To Run Migrations

From repository root:

```bash
cd ai-backend
go run ./cmd/databasemigration
```

Expected success logs:

- `Neon database connected`
- `Running database migrations...`
- `All migrations completed successfully`

If any statement fails, command exits non-zero and prints the failing migration index.

## Current Runtime Behavior

- `cmd/server` no longer applies migrations automatically.
- You must run `cmd/databasemigration` before starting a new backend release when schema changes exist.

## How To Verify Connectivity On Running Server

## 1) Neon Database Connectivity

Call:

```bash
curl -s http://localhost:8080/health
```

Healthy DB response includes:

- `"database":"connected"`

If DB is not connected:

- `"database":"disconnected"`
- DB-backed endpoints will return `503`.

## 2) MiniMax Integration Readiness

The same `/health` response includes:

- `"minimax":"configured"` if API key is loaded into backend service
- `"minimax":"not_configured"` if API key is missing

Readiness means backend is configured to call MiniMax. To actively verify remote MiniMax API reachability and model compatibility, run:

```bash
cd ai-backend/cmd/verifyminimax
go run .
```

This writes `result.json` with successful/failed endpoint-model combinations.

## Migration Authoring Workflow

When you need to add table/column/index or alter schema:

1. Open `internal/db/migrations.go`.
2. Append new SQL at the end of `migrations` slice.
3. Keep statements idempotent.
4. Use additive-first strategy to avoid production breakage.
5. Run migration command locally.
6. Verify application behavior against changed schema.

## Patterns

### Add New Table

```sql
CREATE TABLE IF NOT EXISTS your_table (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);
```

### Add New Column

```sql
ALTER TABLE your_table
ADD COLUMN IF NOT EXISTS new_column TEXT;
```

### Add New Index

```sql
CREATE INDEX IF NOT EXISTS idx_your_table_new_column
ON your_table(new_column);
```

### Alter Column Type (safe rollout)

Preferred phased approach:

1. Add new column (nullable), e.g. `new_value`.
2. Backfill data with update script.
3. Deploy app reading both old and new columns.
4. Switch writes to new column.
5. Drop old column in a later migration.

Avoid destructive in-place type conversions during critical production rollouts.

## Local Development Flow

1. Update migration SQL in `internal/db/migrations.go`.
2. Run `go run ./cmd/databasemigration`.
3. Start backend `go run ./cmd/server`.
4. Check `GET /health`:
   - `database=connected`
   - `minimax=configured` (if key exists)
5. Run relevant tests.

## Production Deployment (Google Cloud Run)

Recommended deployment sequence:

1. Build and push backend image.
2. Run one-off migration job with same image and env as backend service:
   - Command: `go run ./cmd/databasemigration`
   - Ensure `DATABASE_URL` points to Neon production DB.
3. Confirm migration job success from logs.
4. Deploy/update Cloud Run backend service (`cmd/server`).
5. Post-deploy smoke test:
   - `GET /health` returns `status=ok`, `database=connected`
   - execute one DB-backed API request (for example `/api/skills`)
   - verify MiniMax readiness via `minimax=configured`

If migration fails, do not roll forward backend service until schema is fixed.

## Rollback Notes

- Because migrations are SQL statements applied in order, rollback is manual.
- For risky changes, prefer forward-fix migrations over destructive rollback.
- Always backup critical data before major schema change.

## CI/CD Recommendation

- Add a pipeline stage that runs `go run ./cmd/databasemigration` against target environment before service rollout.
- Block deployment if migration stage fails.

