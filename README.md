# QA Automation Tools Blueprint

A QA automation system with AI-powered features for testing and monitoring healthcare web applications (SUT).

## Table of Contents

- [System Overview](#system-overview)
- [Subdirectories & System Boundaries](#subdirectories--system-boundaries)
- [Global Invariants](#global-invariants)
- [Global Side Effects](#global-side-effects)
- [System Verification](#system-verification)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Git Hooks](#git-hooks)
- [Running Backend Locally](#running-backend-locally)
- [Running Dashboard Locally](#running-dashboard-locally)
- [Running Both Services](#running-both-services)
- [Environment Variables](#environment-variables)
- [API Documentation](#api-documentation)
- [Deployment](#deployment)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

---

## System Overview

```
  Browser
     │  https://qa-tool.mfardiansyah.id   (TLS at the edge-host nginx edge, then WireGuard)
     ▼
 ┌──────────────────────── app-host 10.8.0.9 (4 vCPU / 3.8 GiB) ──────────────────────────┐
 │                                                                                           │
 │  Docker Compose (qa-automation)                                                           │
 │  ┌─────────────────────┐ /api,/health ┌────────────────────────┐                          │
 │  │ dashboard  :8140    │─────────────▶│ ai-backend (Go) :8080  │                          │
 │  │ React SPA + nginx   │              │ cookie auth (DB up)    │                          │
 │  └─────────────────────┘              │ /api/jobs/* dispatcher │                          │
 │                                       └──┬──────────┬──────┬───┘                          │
 │       POST /v1/runs, GET /v1/runs/:id    │          │      │                              │
 │       (Bearer, poll every 3 s)           ▼          │      │                              │
 │  systemd --user (mise node 24)  ┌───────────────────────┐  │                              │
 │  MemoryMax 2.5G · CPUQuota 60%  │ playwright-executor   │  │                              │
 │                                 │ 10.8.0.9:8141        │──┼──Chromium──▶ SUT STAGING  │
 │                                 │ 1 browser at a time   │  │                              │
 │                                 └───────────────────────┘  │                              │
 │  systemd --user                 ┌───────────────────────┐  │                              │
 │                                 │ ai-claude-cli :8105   │◀─┼───────────────┐              │
 │                                 │ ("claude-v3")         │  │               │              │
 │                                 └───────────────────────┘  │               │              │
 └────────────────────────────────────────────────────────────┼───────────────┼──────────────┘
                                                              │ WireGuard     │
 ┌──────────────── db-host 10.8.0.4 ──────────────────────▼───────────────┼──────┐
 │  MySQL 8 :5306 (schema qa_automation)   ai-manager :8099 POST /v1/chat ─────┘      │
 │                                         provider=claude-v3, no_fallback=true       │
 └────────────────────────────────────────────────────────────────────────────────────┘

  this-qa-automation-system-playwright ──▶ ai-backend + dashboard (self-tests)
```

- **ai-backend** is the Go API. It stores flows, issues, skills, learning data, cost data and jobs in MySQL 8.
  Every LLM call goes through `internal/services/ai` (`ai.Service` → ai-manager → **claude-v3**, with `no_fallback`).
  `/api/jobs/qa/run` and `/api/jobs/docs/generate` dispatch to playwright-executor (`internal/api/jobs_executor.go`).
- **playwright-executor** is the only place Playwright runs. It is a small Node/TS service on app-host, run as a
  systemd user unit with Node from mise, and capped by its cgroup at 2.5 GiB RAM and 60 % of all CPUs.
  It logs into SUT **staging**, explores and runs QA skills, or records documentation flows.
  It makes no AI calls and no calls to the backend: the backend polls it.
- **dashboard** is the React + Vite UI. Every route except `/login` is gated by a cookie-session login.
- **playwright-qa** and **playwright-docs** are **deprecated**. Their working logic was ported into playwright-executor;
  they remain only for reference until a later cleanup PR.
- **this-qa-automation-system-playwright** is the E2E self-verification suite for the backend API and the dashboard.
- **LLM policy:** ai-manager pinned to `claude-v3` (the Claude CLI REST on app-host) is the only supported path.
  The MiniMax provider code is **dormant**: it is never selected by default and must not be switched on without the owner's explicit approval.

## Subdirectories & System Boundaries

- `ai-backend/`: [ai-backend/README.md](./ai-backend/README.md). Go API service: router, auth, config, MySQL, AI provider, skills, learning.
  - [internal/api](./ai-backend/internal/api/README.md): HTTP router, `/health`, `/api/*` handlers, `/api/auth/*`, simulated job queue.
  - [internal/auth](./ai-backend/internal/auth/README.md): bcrypt passwords, MySQL `sessions`, `Require` middleware.
  - [internal/config](./ai-backend/internal/config/README.md): `LoadConfig` (env > `.env` > `config.yaml`) and the `GetDatabaseURL` MySQL DSN.
  - [internal/db](./ai-backend/internal/db/README.md): MySQL 8 connection pool and idempotent `RunMigrations`.
  - [internal/services/ai](./ai-backend/internal/services/ai/README.md): `ChatProvider`. `aiManagerProvider` (claude-v3) is the only supported provider; `minimaxProvider` is dormant.
  - `internal/services/executor`: playwright-executor client (`Submit`, `Get`, `Wait`, `Health`).
  - [internal/services/skills](./ai-backend/internal/services/skills/README.md): skill registry (`skills`, `skill_logs`) and built-in skills.
  - [internal/services/learning](./ai-backend/internal/services/learning/README.md): learning cache, navigation edges, action sequences, context records.
- `dashboard/`: [dashboard/README.md](./dashboard/README.md). React SPA. Calls same-origin `/api/*` with `credentials: 'include'`.
- `playwright-executor/`: [playwright-executor/README.md](./playwright-executor/README.md). Playwright run service (`qa_run`, `docs_generate`). It has an HTTP API with a bearer token, concurrency 1, and is staging only.
- `deploy/playwright-executor/`: [deploy/playwright-executor/README.md](./deploy/playwright-executor/README.md). systemd user unit template, `install.sh`, and the resource budget.
- `playwright-qa/` (**deprecated**): [playwright-qa/README.md](./playwright-qa/README.md). The old exploratory QA prototype, superseded by playwright-executor.
- `playwright-docs/` (**deprecated**): [playwright-docs/README.md](./playwright-docs/README.md). The old flow recorder and summarizer, superseded by playwright-executor plus backend `summarizeDocsRun`.
- `this-qa-automation-system-playwright/`: [this-qa-automation-system-playwright/README.md](./this-qa-automation-system-playwright/README.md). Playwright API and dashboard regression suite for this repository.
- `lib/`: [lib/README.md](./lib/README.md). Shared TypeScript `config.yaml` loader (`lib/config.ts`), imported by playwright-docs.
- `deploy/qa-tool/`: [deploy/qa-tool/README.md](./deploy/qa-tool/README.md). **Production** Docker Compose stack on app-host, behind the edge-host nginx edge.
- `product-documentations/`: functional documentation source for SUT modules, served by `/api/docs`.
- `_docs/` and root `*.md` files: operational docs, plans, tutorials, assessments and architecture notes.

## Global Invariants

- **Auth depends on the DB.** `ai-backend/cmd/server/main.go` wires cookie-session auth (`router.SetAuth`) only when `db.NewConnection` succeeds.
  If `/health` reports `"database":"disconnected"`, the API is open.
- **Migrations are explicit.** The server never migrates. Run `go run ./cmd/databasemigration`. It is idempotent: `CREATE TABLE IF NOT EXISTS`, append-only.
- **One LLM path: ai-manager → claude-v3.** Backend LLM calls go through `ChatProvider`, which is pinned to `AI_MANAGER_PROVIDER=claude-v3` with `AI_MANAGER_NO_FALLBACK=true`.
  `AI_PROVIDER=minimax` still exists in code but is dormant and needs explicit owner approval. playwright-executor makes no LLM calls.
- **Playwright runs only in playwright-executor.** It is bound to the WireGuard IP, runs one browser at a time, and its systemd cgroup caps it at `MemoryMax=2560M` and `CPUQuota = nproc × 60 %`.
  It only targets `TARGET_STAGING_URL`. The backend falls back to simulated jobs when `PLAYWRIGHT_EXECUTOR_URL` is empty.
- **Same-origin API.** The dashboard uses relative `/api/*` paths. They go through the Vite proxy in dev and the in-container nginx in production.
  Backend CORS comes from `DASHBOARD_ORIGIN` (`router.go:applyCORS`).
- **Secrets** live in `.env`, `config.yaml` (both gitignored) and, in production, in `.env.prod` on app-host only. Never commit them.

## Global Side Effects

- MySQL 8 database (`qa_automation`): see [ai-backend/internal/db](./ai-backend/internal/db/README.md) for tables.
- Outbound LLM calls to ai-manager (`POST {AI_MANAGER_URL}/v1/chat`, default `http://10.8.0.4:8099` over WireGuard), always answered by `claude-v3`. ai-manager bills and logs these calls.
- playwright-executor drives Chromium against SUT staging and writes screenshots to `~/qa-executor-data/runs` on app-host (kept 7 days). Issues found by `qa_run` are inserted into `raw_issues`.
- Read-only user import from the taskmgr Neo4j at backend startup, when `TASKMGR_NEO4J_URI` is set.
- The Playwright tools log into, and click through, the SUT staging frontend. playwright-docs writes screenshots and videos under `playwright-docs/output/`.
- The self-test suite mutates backend data: it creates flows and issues and updates and deletes learning records.

## System Verification

- Backend: `cd ai-backend && go test -mod=mod ./...`
- Dashboard: `cd dashboard && npm run build` (and `npm run lint`)
- Playwright executor: `cd playwright-executor && mise exec -- npm test`
- Self-tests (backend on :8080, dashboard on :3000): `cd this-qa-automation-system-playwright && npx playwright test`
- AI provider smoke test: `cd ai-backend && go run ./cmd/verifyai`
- Production health: see [deploy/qa-tool/README.md](./deploy/qa-tool/README.md#verification)

---

## Prerequisites

### Required Software

| Software | Version | Description |
|----------|---------|-------------|
| [Go](https://go.dev/dl/) | 1.23+ | Backend runtime |
| [Node.js](https://nodejs.org/) | 18+ | Dashboard and Playwright runtime |
| [Docker](https://docs.docker.com/get-docker/) | Latest | Container runtime |
| MySQL client | 8.x | Database client (optional) |

### Required Access

- A MySQL 8 database (production uses `qa_automation` on db-host over WireGuard)
- ai-manager credentials (`AI_MANAGER_LOGIN_ID` / `AI_MANAGER_SECRET_KEY`) and WireGuard access to it. No other LLM credentials are needed: claude-v3 via ai-manager is the only supported path.
- For real Playwright jobs: a reachable playwright-executor (`PLAYWRIGHT_EXECUTOR_URL` and `_TOKEN`). Without it, `/api/jobs/*` run in simulated mode.

---

## Quick Start

### Option 1: Docker Compose (local)

The root `docker-compose.yml` builds `backend` (:8080) and `dashboard` (:3000 -> 80) and mounts `./config.yaml` into the backend.
For production, use [deploy/qa-tool](./deploy/qa-tool/README.md) instead.

```bash
# Start all services
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f backend
docker-compose logs -f dashboard
```

### Option 2: Local Development

```bash
# Terminal 1 - Start backend
cd ai-backend
cp .env.example .env
# Edit .env with your credentials
go run ./cmd/databasemigration   # first time / after schema changes
go run ./cmd/server

# Terminal 2 - Start dashboard
cd dashboard
npm install
npm run dev
```

---

## Git Hooks

Enable repo hooks once to block filenames that break Git on Windows:

```bash
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit scripts/check-windows-paths.sh
```

The pre-commit hook will reject staged paths with Windows-invalid names
(`\`, `:`, `*`, reserved names like `CON`, trailing dot/space, etc.).

---

## Running Backend Locally

See also [tutorial-running-backend-locally.md](./tutorial-running-backend-locally.md).

### Step 1: Configure Environment

```bash
cd ai-backend

# Copy example environment file
cp .env.example .env
```

### Step 2: Edit .env File

Fill in `.env` using the variable names from `.env.example`:

```bash
# DATABASE (MySQL 8): either DATABASE_URL (MySQL DSN) or the DB_* parts
DB_HOST=...
DB_PORT=...
DB_USER=...
DB_PASSWORD=...
DB_NAME=qa_automation

# AI: ai-manager is the default provider
AI_PROVIDER=ai-manager
AI_MANAGER_URL=http://10.8.0.4:8099
AI_MANAGER_LOGIN_ID=...
AI_MANAGER_SECRET_KEY=...
# (AI_PROVIDER=minimax is dormant; do not use)

# PLAYWRIGHT (optional locally; empty = simulated /api/jobs)
PLAYWRIGHT_EXECUTOR_URL=
PLAYWRIGHT_EXECUTOR_TOKEN=

# AUTH (active only when the DB connects)
DASHBOARD_ORIGIN=http://localhost:3000
ADMIN_USERNAME=...
ADMIN_PASSWORD=...

# SERVER
PORT=8080
```

### Step 3: Install Dependencies

```bash
cd ai-backend
go mod download
```

### Step 4: Run Migrations and the Backend

```bash
# Migrations (idempotent; the server never migrates on its own)
go run ./cmd/databasemigration

# Option 1: Using Go run
go run ./cmd/server

# Option 2: Build and run
go build -o server ./cmd/server
./server
```

### Step 5: Verify Backend is Running

```bash
# Health check
curl http://localhost:8080/health

# Expected response shape:
# {"ai":"ok","ai_provider":"ai-manager","ai_upstream":"claude-v3","database":"connected","executor":"ok","minimax":"not_configured","status":"ok"}
```

---

## Running Dashboard Locally

### Step 1: Install Dependencies

```bash
cd dashboard
npm install
```

### Step 2: Configure Environment

`dashboard/vite.config.ts` reads `VITE_API_URL` (proxy target for `/api`, default `http://localhost:8080`), `VITE_DEV_SERVER_PORT` (default `3000`), `VITE_ALLOWED_HOSTS`, and `VITE_APP_DEBUG` (source maps).

### Step 3: Run the Dashboard

```bash
# Development mode (with hot reload)
npm run dev

# Or build and preview
npm run build
npm run preview
```

### Step 4: Access the Dashboard

Open your browser to:
- **Dashboard UI**: http://localhost:3000 (log in when the backend has a DB)
- **Backend API**: http://localhost:8080

---

## Running Both Services

### Option 1: Two Terminals

```bash
# Terminal 1 - Backend
cd ai-backend
go run ./cmd/server

# Terminal 2 - Dashboard
cd dashboard
npm run dev
```

### Option 2: Using Docker Compose

Use the root `docker-compose.yml` (see [Quick Start](#option-1-docker-compose-local)):

```bash
docker-compose up -d
```

---

## Environment Variables

### Backend (`ai-backend/.env`)

Loaded by `ai-backend/internal/config/config.go:LoadConfig` and `cmd/server/main.go`. See [internal/config](./ai-backend/internal/config/README.md).

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | MySQL 8 connection parts | `DB_NAME=qa_automation` |
| `DATABASE_URL` | Full MySQL DSN (overrides `DB_*`) | — |
| `AI_PROVIDER` | `ai-manager` (only supported value; `minimax` is dormant) | `ai-manager` |
| `AI_MANAGER_URL` | ai-manager base URL | `http://10.8.0.4:8099` |
| `AI_MANAGER_LOGIN_ID`, `AI_MANAGER_SECRET_KEY` | ai-manager HTTP Basic credentials | — |
| `AI_MANAGER_PROVIDER`, `AI_MANAGER_NO_FALLBACK`, `AI_MANAGER_MODEL` | Upstream pinned inside ai-manager (Claude CLI REST on app-host `10.8.0.9:8105`) | `claude-v3`, `true`, empty (`claude-opus-5`) |
| `AI_TIMEOUT_SECONDS` | LLM call timeout | `180` |
| `MINIMAX_API_KEY`, `AI_MODEL`, `AI_BASE_URL`, `AI_CHAT_ENDPOINT` | **Dormant** MiniMax settings (unused) | `MiniMax-M2.5`, `https://api.minimax.io/v1`, `/chat/completions` |
| `PLAYWRIGHT_EXECUTOR_URL`, `PLAYWRIGHT_EXECUTOR_TOKEN`, `PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES` | playwright-executor endpoint, bearer token, max wait per job | empty (simulated), —, `30` |
| `GAOS_API_URL`, `GAOS_API_TOKEN` | GAOS REST API base URL (https, or http to loopback/`10.8.0.0/24` only) and the QA bearer (`gaos_...`, secret); both empty = the GAOS integration is off ([GAOS_INTEGRATION.md](docs/designs/GAOS_INTEGRATION.md); how the two systems work together, both ways: [GAOS_COLLABORATION.md](docs/GAOS_COLLABORATION.md)) | empty, — |
| `PORT`, `BACKEND_HOST` | Listen address | `8080`, `0.0.0.0` |
| `DASHBOARD_ORIGIN` | CORS origin when auth is on | `http://localhost:6173` |
| `SESSION_COOKIE_NAME`, `SESSION_TTL_HOURS`, `SESSION_COOKIE_SECURE` | Session cookie | `qa_session`, `12`, `false` |
| `ADMIN_USERNAME`, `ADMIN_PASSWORD` | Seeded admin user | — |
| `AI_SERVICE_USERNAME`, `AI_SERVICE_PASSWORD` | Seeded AI service account (role `Service`, may impersonate; password ≥ 32 chars) | — |
| `TASKMGR_NEO4J_URI`, `TASKMGR_NEO4J_USERNAME`, `TASKMGR_NEO4J_PASSWORD`, `TASKMGR_NEO4J_DATABASE` | Optional read-only user import | database `neo4j` |

`API_SECRET_KEY`, `CORS_ENABLED` and `CORS_ORIGINS` are loaded into config but are not read by the HTTP router.

### Dashboard (`dashboard/.env`)

| Variable | Description | Default |
|----------|-------------|---------|
| `VITE_API_URL` | Dev proxy target for `/api` | `http://localhost:8080` |
| `VITE_DEV_SERVER_PORT` | Dev server port | `3000` |
| `VITE_ALLOWED_HOSTS` | Extra allowed hostnames (comma-separated) | — |
| `VITE_APP_DEBUG` | Emit source maps on build | `false` |

---

## API Documentation

### Base URL

| Environment | URL |
|-------------|-----|
| Local | `http://localhost:8080` |
| Production | `https://qa-tool.mfardiansyah.id` (`/api`, `/health` proxied by the dashboard nginx) |

### Endpoints

The full, code-verified list with methods is in [ai-backend/internal/api/README.md](./ai-backend/internal/api/README.md).
Only `/health` and `/api/auth/login` are public. When the backend has a DB, every other `/api/*` endpoint requires the session cookie.

| Endpoint | Methods | Description |
|----------|---------|-------------|
| `/health` | GET | Status, DB, AI provider and AI health |
| `/api/auth/login`, `/api/auth/logout`, `/api/auth/me`, `/api/auth/change-password` | POST / GET | Cookie-session auth (only when DB connected) |
| `/api/auth/impersonate`, `/api/auth/impersonate/stop`, `/api/users` | POST / GET | Admin/Service impersonation of another user, and the user list for its picker ([spec](docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md)) |
| `/api/flows` | GET, POST | Flows |
| `/api/roles` | GET, POST, DELETE | Roles |
| `/api/docs`, `/api/videos` | GET | Product docs and recorded videos from disk |
| `/api/issues` | GET, POST | Issues (POST classifies through the AI provider) |
| `/api/issues/status`, `/api/issues/notes` | POST | Update fix status and notes |
| `/api/skills`, `/api/skills/logs`, `/api/skills/stats` | GET (+POST logs) | Skills |
| `/api/search?q=` | GET | Search learning cache |
| `/api/cost/overview`, `/trend`, `/efficiency`, `/by-feature`, `/by-model`, `/recent`, `/filters`, `/gateway` | GET | Cost analytics (AI call ledger + ai-manager usage) |
| `/api/analytics` | GET | Usage analytics |
| `/api/learning` (+ `/navigation`, `/sequences`, `/context`, `/stats`) | GET, POST, DELETE | Learning records |
| `/api/ai-cache`, `/api/embedding-cache`, `/api/goal-events` | GET, POST, DELETE | Caches and goal events |
| `/api/jobs`, `/api/jobs/qa/run`, `/api/jobs/docs/generate`, `/api/jobs/{id}` | GET / POST | Job queue |

### Example API Calls

```bash
# Health check
curl http://localhost:8080/health

# Log in (when auth is enabled) and reuse the session cookie
curl -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"<user>","password":"<password>"}'

# Get all flows
curl -b cookies.txt http://localhost:8080/api/flows

# Get issues
curl -b cookies.txt http://localhost:8080/api/issues

# Search (requires q parameter)
curl -b cookies.txt "http://localhost:8080/api/search?q=login"
```

---

## Deployment

### Production: qa-tool (Docker Compose on app-host)

Production runs at **https://qa-tool.mfardiansyah.id**. A Docker Compose stack on app-host runs `dashboard` (nginx + SPA, which proxies `/api` and `/health`) and `backend`.
It sits behind the shared edge-host nginx edge and reaches MySQL and ai-manager over WireGuard.
See [deploy/qa-tool/README.md](./deploy/qa-tool/README.md) for deploy, rollback and health checks.

### Legacy: Google Cloud Run

`ai-backend/cloudbuild.yaml`, `dashboard/cloudbuild.yaml` and [deployment-tutorial.md](./deployment-tutorial.md) describe the earlier Cloud Run deployment.
They are **legacy** and no longer the deploy path.

---

## Testing

### Backend Tests

```bash
cd ai-backend
go test -mod=mod ./...
```

See [tutorial-running-unit-tests.md](./tutorial-running-unit-tests.md) and [tutorial-running-integration-tests.md](./tutorial-running-integration-tests.md).

### Playwright QA Engine

```bash
cd playwright-qa
npm install
npm run explore   # random explorer (see playwright-qa/README.md for current build caveats)
```

### Self-Verification Testing (System Under Test)

```bash
# Run end-to-end/API checks for this repository itself
cd this-qa-automation-system-playwright
npm install

# Run all self-verification tests
npm test

# Run only API coverage
npm run test:api

# Run only dashboard coverage
npm run test:dashboard
```

Note: the specs do not log in, so run them against a backend without a DB or with auth disabled. See the [suite README](./this-qa-automation-system-playwright/README.md).

---

## Troubleshooting

### Backend Issues

#### Database Connection Failed

The startup log prints `Database connection failed`, and the server then runs without a DB, which means without auth.

```bash
# Check the DB_* / DATABASE_URL values in ai-backend/.env, then test with the MySQL client
mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p "$DB_NAME" -e "SELECT 1"
```

#### AI Provider Not Working

```bash
cd ai-backend
go run ./cmd/verifyai   # prints provider, configured, health, then sends one chat
```

#### Port Already in Use

```bash
# Find process using port 8080
lsof -i :8080

# Kill the process
kill -9 <PID>
```

#### Missing Dependencies

```bash
cd ai-backend
go mod tidy
go mod download
```

### Dashboard Issues

#### CORS / Cookie Errors

In development the dashboard uses the Vite `/api` proxy, so requests are same-origin.
If you call the backend cross-origin, set `DASHBOARD_ORIGIN` in `ai-backend/.env` to the dashboard's origin.
`CORS_ENABLED` and `CORS_ORIGINS` have no effect on the router.

#### Cannot Connect to Backend

```bash
# Check backend is running
curl http://localhost:8080/health

# Check VITE_API_URL in dashboard/.env
cat dashboard/.env | grep VITE_API_URL
```

### Docker Issues

#### Container Won't Start

```bash
# View logs
docker logs <container-name>

# Check port mapping
docker ps

# Rebuild without cache
docker build --no-cache -t myimage .
```

---

## File Structure

```
qa-automation-tools/
├── ai-backend/               # Go backend
│   ├── cmd/                  # server, databasemigration, verifyai, verifyminimax
│   ├── internal/
│   │   ├── api/              # HTTP router + handlers
│   │   ├── auth/             # Cookie sessions
│   │   ├── config/           # Configuration
│   │   ├── db/               # MySQL connection + migrations
│   │   ├── models/           # Data models
│   │   ├── seed/             # User seeding
│   │   └── services/         # ai, skills, learning
│   ├── .env                  # Environment variables (gitignored)
│   └── Dockerfile
│
├── dashboard/                # React frontend
│   ├── src/                  # Source code
│   ├── nginx.conf.example    # Nginx config template
│   └── Dockerfile
│
├── deploy/qa-tool/         # Production Docker Compose stack
├── deploy/playwright-executor/ # systemd user unit + install.sh (2.5 GiB / 60 % CPU)
├── lib/                      # Shared TS config loader
├── playwright-executor/      # THE Playwright runtime (HTTP service, mise node 24)
├── playwright-docs/          # DEPRECATED: old documentation generator
├── playwright-qa/            # DEPRECATED: old QA prototype
├── this-qa-automation-system-playwright/ # Self-verification tests (API + dashboard)
├── product-documentations/   # Source docs for product modules
├── config.yaml               # Main configuration (gitignored; see config.yaml.template)
├── docker-compose.yml        # Local Docker compose
└── README.md                 # This file
```

---

## Additional Resources

- [Backend Blueprint](./ai-backend/README.md)
- [Database Migration Guide](./ai-backend/database-migration.md)
- [Dashboard Documentation](./dashboard/README.md)
- [Production Deployment (qa-tool)](./deploy/qa-tool/README.md)
- [REST API Usage](./rest-api-usage-documentation.md)
- [Legacy Cloud Run Deployment Tutorial](./deployment-tutorial.md)
- `MiniMax-API-Endpoint.pdf` and `Playwright-for-System-Documentation.pdf` (local reference PDFs; `*.pdf` is gitignored)

---

## License

Copyright © 2025 ardihikaru. Licensed under the MIT License.
