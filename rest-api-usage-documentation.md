# REST API Usage Documentation

Date: 2026-04-18
Scope: `ai-backend` refactor reassessment based on `ai-backend/resources/*.http`

## Runtime Connectivity Checks

### Health check contract
- Endpoint: `GET /health`
- Expected indicators:
  - `database: "connected"` means backend can reach the MySQL database (`qa_automation`).
  - `database: "disconnected"` means DB is not connected.
  - `minimax: "configured"` means MiniMax API key is loaded in runtime config.
  - `minimax: "not_configured"` means MiniMax API key is not loaded.

### Observed health response (runtime)
- Command: `curl http://127.0.0.1:8080/health`
- Result:
  - `{"status":"ok","database":"connected","minimax":"configured"}`
- Interpretation:
  - MySQL connectivity is active on the running backend process.
  - MiniMax integration is configured in runtime.

### MiniMax outbound verification
- Command: `go run ./cmd/verifyminimax`
- Summary result:
  - Successful tests: 3/16
  - Best working setup:
    - Base URL: `https://api.minimax.io/v1`
    - Model: `MiniMax-M2.5`
    - Endpoint: `/chat/completions`
    - Status: `200`
- Notes:
  - Alternate base URL (`api.minimax.chat`) returned authorization failures.
  - Invalid model test intentionally failed with `400` (expected negative case).

## API Usage Reassessment (Based on `resources/*.http`)

### Endpoints covered by unit tests (no DB mode)
- `GET /health`
- `GET /api/flows`, `POST /api/flows`
- `GET /api/roles`, `POST /api/roles`
- `GET /api/issues`, `POST /api/issues`
- `POST /api/issues/status`, `POST /api/issues/notes`
- `GET /api/skills`
- `GET/POST /api/skills/logs`
- `GET /api/skills/stats`
- `GET /api/search` (including missing `q` validation)
- `GET /api/cost/overview`
- `GET /api/cost/trend`
- `GET /api/cost/efficiency`
- `GET /api/cost/by-feature`
- `GET /api/docs`
- `GET /api/videos`
- `GET /api/analytics`
- `GET/POST/DELETE /api/learning`
- `GET/POST /api/learning/navigation`
- `GET/POST /api/learning/sequences`
- `GET/POST /api/learning/context`
- `GET /api/learning/stats`
- `GET/POST/DELETE /api/ai-cache`
- `GET/POST/DELETE /api/embedding-cache`
- `GET/POST/DELETE /api/goal-events`

### Endpoints covered by DB CRUD integration tests (MySQL-backed)
- `POST/GET/DELETE /api/roles`
- `POST/GET /api/flows` (with seeded role)
- `POST/GET /api/issues` + `POST /api/issues/status` + `POST /api/issues/notes`
- `POST/GET/DELETE /api/learning`
- `POST/GET/DELETE /api/goal-events`
- `POST/GET/DELETE /api/ai-cache`
- `POST/GET/DELETE /api/embedding-cache`

### Behavior validated in this refactor
- DB-dependent endpoints return `503 Service Unavailable` when DB is not available.
- `GET /api/search` with missing `q` returns `400 Bad Request`.
- CORS preflight (`OPTIONS`) remains functional.
- `/health` remains available regardless of DB connection state.
- `POST /api/issues` no longer attempts AI classification when AI service is nil.
- Integration tests load `DB_*`/`DATABASE_URL` from multiple `.env` paths automatically.

## Test Execution Results

- `go test ./internal/api -count=1 -v`: PASS
- `go test ./... -count=1`: PASS

## Important Runtime Note

When trying to start a new backend instance during this task, port `8080` was already in use by an existing process. The successful `/health` check therefore reflects the already-running backend service bound to that port.
