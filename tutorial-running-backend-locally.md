# Tutorial: Running `ai-backend` Locally (Neon + MiniMax)

This guide explains how to run the backend service locally with:
- Neon PostgreSQL connected
- MiniMax AI configured
- Database migrations executed explicitly via `cmd/databasemigration`

This project uses **Neon PostgreSQL only** for backend database operations.

---

## 1. Scope and Expected Result

After following this tutorial, you should be able to:
1. Run migrations against Neon from local machine.
2. Start backend server (`ai-backend/cmd/server`).
3. Verify runtime health via `GET /health`:
   - `"database":"connected"`
   - `"minimax":"configured"`
4. Validate MiniMax connectivity deeper using `cmd/verifyminimax`.

---

## 2. Prerequisites

Required software:
- Go (matching `ai-backend/go.mod` toolchain)
- `curl`
- `psql` (optional but recommended for Neon connectivity checks)

Required credentials:
- Neon connection string (`DATABASE_URL`) or Neon host/user/password/db fields
- MiniMax API key (`MINIMAX_API_KEY`)

---

## 3. Configuration Strategy (Recommended)

`ai-backend` loads config from `.env` automatically, then applies optional YAML config values.

Configuration precedence for database:
1. `DATABASE_URL` (recommended for local run)
2. `backend.database.connection_string` in `config.yaml`
3. `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`

Configuration for MiniMax:
- `MINIMAX_API_KEY`
- Optional: `AI_MODEL`, `AI_BASE_URL`, `AI_CHAT_ENDPOINT`

If you need reference values that already exist in this repository, check:
- `_docs/markdown-results/by-roles/.env`
- `config.yaml`

Copy only required values into your local `ai-backend/.env` and avoid committing secrets.

Recommended local setup:

```bash
cd ai-backend
cp .env.example .env
```

Then edit `.env` with your real values:

```env
DATABASE_URL=postgresql://<user>:<password>@<neon-host>/<db>?sslmode=require
MINIMAX_API_KEY=<your-minimax-api-key>
AI_MODEL=MiniMax-M2.5
AI_BASE_URL=https://api.minimax.io/v1
AI_CHAT_ENDPOINT=/chat/completions
PORT=8080
BACKEND_HOST=0.0.0.0
```

Do not commit real secrets to git.

---

## 4. Optional: Pre-check Neon Connection

Before running migration/server, validate DB access:

```bash
cd ai-backend
psql "$DATABASE_URL" -c "SELECT 1;"
```

If this fails, fix network/credentials first.

---

## 5. Run Database Migration (Mandatory Before Backend Start)

`cmd/server` does **not** auto-apply migrations. Run migration explicitly:

```bash
cd ai-backend
go run ./cmd/databasemigration
```

Expected successful logs include:
- `Neon database connected`
- `Running database migrations...`
- `All migrations completed successfully`
- `Database migration completed`

If migration fails, fix the SQL/config issue and rerun migration before starting backend.

---

## 6. Run Backend Server

Start backend locally:

```bash
cd ai-backend
go run ./cmd/server/main.go
```

Expected startup indicators:
- server starts on `0.0.0.0:8080` (or your configured port)
- DB status logs indicate connected when Neon credentials are valid

---

## 7. Verify Runtime Health (DB + MiniMax Readiness)

Run:

```bash
curl -s http://localhost:8080/health
```

Expected healthy output shape:

```json
{"status":"ok","database":"connected","minimax":"configured"}
```

Interpretation:
- `database=connected`: backend currently has live DB connection.
- `database=disconnected`: backend runs in limited mode; DB-backed endpoints can return `503`.
- `minimax=configured`: API key is loaded in backend process.
- `minimax=not_configured`: API key missing/empty in runtime config.

---

## 8. Verify DB-backed Endpoint

After `/health` is good, verify at least one DB-backed endpoint:

```bash
curl -s http://localhost:8080/api/skills
```

If DB is connected and schema exists, endpoint should return successful JSON response.

---

## 9. Verify MiniMax Connectivity (Active Check)

`/health` only confirms key presence. For active endpoint/model verification:

```bash
cd ai-backend/cmd/verifyminimax
go run .
```

What this does:
- tests multiple MiniMax models and base URLs
- writes detailed result to:
  - `ai-backend/cmd/verifyminimax/result.json`

Success criteria:
- at least one endpoint/model combo returns success in result summary.

---

## 10. Routine When Schema Changes

When backend schema is updated (`internal/db/migrations.go`):
1. Append new migration SQL at the end of migration list.
2. Keep migration idempotent where possible (`IF NOT EXISTS`, additive changes first).
3. Run migration locally:

```bash
cd ai-backend
go run ./cmd/databasemigration
```

4. Restart backend:

```bash
go run ./cmd/server/main.go
```

5. Re-check:
- `GET /health` returns `database=connected`
- DB-backed endpoint(s) still pass

---

## 11. Common Failure Modes and Fixes

1. `/health` shows `database=disconnected`
- Ensure `DATABASE_URL` is set and valid.
- Ensure Neon host is reachable from your machine.
- Re-run `psql "$DATABASE_URL" -c "SELECT 1;"`.

2. Migration command fails
- Read failing migration index from output.
- Fix SQL/permissions problem in Neon.
- Re-run `go run ./cmd/databasemigration`.

3. `/health` shows `minimax=not_configured`
- Set `MINIMAX_API_KEY` in runtime environment or `.env`.
- Restart backend process after updating environment.

4. MiniMax verify tool fails all combinations
- Validate `MINIMAX_API_KEY`.
- Check outbound network/firewall.
- Try alternate base URL support from verify output (`api.minimax.io` vs `api.minimax.chat`).

---

## 12. Quick Run Checklist

```bash
cd ai-backend
cp .env.example .env
# edit .env -> DATABASE_URL + MINIMAX_API_KEY

go run ./cmd/databasemigration
go run ./cmd/server/main.go

curl -s http://localhost:8080/health
curl -s http://localhost:8080/api/skills

cd cmd/verifyminimax
go run .
```
