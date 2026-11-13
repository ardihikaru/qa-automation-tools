# Tutorial: Running `ai-backend` Locally (MySQL + ai-manager)

This guide explains how to run the backend service locally with:
- MySQL 8 connected (schema `qa_automation` on the shared rnd-v2 MySQL, `10.10.0.4:5306`, over WireGuard)
- LLM calls going through ai-manager, pinned to upstream `claude-v3`
- Database migrations executed explicitly via `cmd/databasemigration`

MySQL is the only supported database.

---

## 1. Scope and Expected Result

After following this tutorial, you should be able to:
1. Run migrations against the MySQL database from your machine.
2. Start the backend server (`ai-backend/cmd/server`).
3. Verify runtime health via `GET /health` (`"database":"connected"`, `ai_provider`, `ai_upstream`).
4. Smoke-test the AI path with `cmd/verifyai`.

---

## 2. Prerequisites

Required software:
- Go (matching `ai-backend/go.mod` toolchain)
- `curl`
- `mysql` client (optional, for connectivity checks)

Required access:
- A WireGuard peer on `10.10.0.0/24` (for the DB and ai-manager)
- MySQL credentials for the `qa_automation` user
- ai-manager credentials (`AI_MANAGER_LOGIN_ID`, `AI_MANAGER_SECRET_KEY`)

---

## 3. Configuration

`ai-backend` loads `.env` automatically. Environment variables always beat `config.yaml`, and the YAML `backend.database.*` block is ignored, so configure the DB through env only.

Database settings (one of):
1. `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` (recommended; defaults are `10.10.0.4`, `5306`, `qa_automation`)
2. `DATABASE_URL` as a go-sql-driver/mysql DSN, e.g. `user:pass@tcp(10.10.0.4:5306)/qa_automation?parseTime=true`

Legacy `MYSQL_*` variables in some local `.env` files are ignored by the code and point at the HISv3 production schema. Never use them for this app.

Setup:

```bash
cd ai-backend
cp .env.example .env
# edit .env: DB_PASSWORD, AI_MANAGER_LOGIN_ID, AI_MANAGER_SECRET_KEY
```

Do not commit real secrets to git.

---

## 4. Optional: Pre-check the MySQL Connection

```bash
MYSQL_PWD="$DB_PASSWORD" mysql -h 10.10.0.4 -P 5306 -u qa_automation qa_automation -e 'SELECT 1;'
```

If this fails, fix WireGuard access or credentials first.

---

## 5. Run Database Migration (Mandatory Before Backend Start)

`cmd/server` does **not** auto-apply migrations. Run migration explicitly:

```bash
cd ai-backend
go run -mod=mod ./cmd/databasemigration
```

Expected successful logs include:
- `MySQL database connected`
- `Running MySQL migrations...`
- `All MySQL migrations completed successfully`
- `Database migration completed`

---

## 6. Run Backend Server

```bash
cd ai-backend
go run -mod=mod ./cmd/server
```

The server starts on `0.0.0.0:8080` (or your configured `PORT`).

---

## 7. Verify Runtime Health

```bash
curl -s http://localhost:8080/health
```

Interpretation:
- `database=connected`: the backend connected to MySQL at startup (a snapshot, not a live ping).
- `database=disconnected`: the backend runs in limited mode **and auth is disabled**. Treat it as an incident.
- `ai_provider` / `ai_upstream`: should be `ai-manager` / `claude-v3`.

---

## 8. Verify a DB-backed Endpoint

```bash
curl -s http://localhost:8080/api/skills
```

With auth enabled you must log in first (`POST /api/auth/login`) and send the session cookie.

---

## 9. Verify AI Connectivity

```bash
cd ai-backend
go run -mod=mod ./cmd/verifyai "say hello"
```

---

## 10. Routine When Schema Changes

1. Append new idempotent migration SQL at the end of the list in `internal/db/migrations.go`.
2. Run `go run -mod=mod ./cmd/databasemigration`.
3. Restart the backend and re-check `/health` and a DB-backed endpoint.

---

## 11. Common Failure Modes

1. `/health` shows `database=disconnected`
   - Check `DB_*` (or `DATABASE_URL`) and that `10.10.0.4:5306` is reachable over WireGuard.
2. Migration command fails
   - Read the failing migration index from the output, fix the SQL or permissions, and re-run.
3. AI errors (401/502/503)
   - Check `AI_MANAGER_*` and that `AI_MANAGER_PROVIDER=claude-v3`.
