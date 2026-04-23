# Tutorial: Login to Neon Database (for User or AI Agent)

This guide shows how to access the Neon PostgreSQL database used by `ai-backend`.

## Scope

Use this when you need to:
- verify Neon credentials
- connect via SQL client (`psql`)
- check that backend database access is healthy

## Prerequisites

- `psql` installed (PostgreSQL client)
- repository cloned locally
- valid Neon credentials already available in local env file

## Where Credentials Come From in This Project

Primary source:
- `ai-backend/.env` via `DATABASE_URL`

Fallback supported by backend config:
- `backend.database.connection_string` in `config.yaml`
- split DB env vars (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`)

Do not commit or share plaintext secrets.

## Method 1: Login via Neon Console (Web)

1. Open Neon Console: `https://console.neon.tech`.
2. Sign in to the Neon project that matches this environment.
3. Open the target branch/database.
4. Use Neon SQL Editor for read-only checks first.

Recommended quick query:

```sql
SELECT current_database(), current_user, now();
```

## Method 2: Login via `psql` (CLI)

From repository root:

```bash
cd ai-backend
export DATABASE_URL="$(grep -E '^DATABASE_URL=' .env | cut -d= -f2-)"
psql "$DATABASE_URL"
```

Inside `psql`, run:

```sql
SELECT current_database(), current_user, version();
```

List tables in `public` schema:

```sql
SELECT table_name
FROM information_schema.tables
WHERE table_schema='public' AND table_type='BASE TABLE'
ORDER BY table_name;
```

Exit `psql`:

```sql
\\q
```

## Non-Interactive Connectivity Check

```bash
cd ai-backend
DATABASE_URL="$(grep -E '^DATABASE_URL=' .env | cut -d= -f2-)"
psql "$DATABASE_URL" -P pager=off -c "SELECT 1 AS neon_ok;"
```

Expected: one row with `neon_ok = 1`.

## Backend-Level Neon Verification

Run migration to verify backend can authenticate and apply schema:

```bash
cd ai-backend
GOCACHE=/tmp/go-build-cache go run ./cmd/databasemigration
```

Expected logs include:
- `Neon database connected`
- `All migrations completed successfully`

## Runtime Health Verification

Start backend and check health endpoint:

```bash
cd ai-backend
PORT=18080 GOCACHE=/tmp/go-build-cache go run ./cmd/server
```

In another terminal:

```bash
curl -sS http://localhost:18080/health
```

Healthy DB result must include:

```json
{"status":"ok","database":"connected","minimax":"configured"}
```

## Troubleshooting

1. `failed to ping database`:
- verify `DATABASE_URL` format and SSL params
- verify Neon host/network reachability
- retry with direct `psql "$DATABASE_URL"`

2. `"database":"disconnected"` on `/health`:
- backend started without valid DB config
- check `ai-backend/.env` and restart backend

3. DNS/network blocked in restricted environment:
- run commands with proper network permission/escalation in your execution environment

## Security Notes

- never paste real `DATABASE_URL` into docs, issues, or chat
- rotate credentials immediately if leaked
- prefer env-based loading over hardcoded credentials
