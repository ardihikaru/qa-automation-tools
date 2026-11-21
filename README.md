# QA Automation Tools Blueprint

A QA automation system with AI-powered features for testing and monitoring healthcare web applications (HIS v3).

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
     │  https://qa-tool.sismedika.com   (TLS at the clino nginx edge, then WireGuard)
     ▼
 ┌──────────────────────── rnd-v3-vpn 10.10.0.9 (4 vCPU / 3.8 GiB) ──────────────────────────┐
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
 │                                 │ 10.10.0.9:8141        │──┼──Chromium──▶ HISv3 STAGING  │
 │                                 │ 1 browser at a time   │  │                              │
 │                                 └───────────────────────┘  │                              │
 │  systemd --user                 ┌───────────────────────┐  │                              │
 │                                 │ ai-claude-cli :8105   │◀─┼───────────────┐              │
 │                                 │ ("claude-v3")         │  │               │              │
 │                                 └───────────────────────┘  │               │              │
 └────────────────────────────────────────────────────────────┼───────────────┼──────────────┘
                                                              │ WireGuard     │
 ┌──────────────── rnd-v2-vpn 10.10.0.4 ──────────────────────▼───────────────┼──────┐
 │  MySQL 8 :5306 (schema qa_automation)   ai-manager :8099 POST /v1/chat ─────┘      │
 │                                         provider=claude-v3, no_fallback=true       │
 └────────────────────────────────────────────────────────────────────────────────────┘

  this-qa-automation-system-playwright ──▶ ai-backend + dashboard (self-tests)
```

- **ai-backend** is the Go API. It stores flows, issues, skills, learning data, cost data and jobs in MySQL 8.
  Every LLM call goes through `internal/services/ai` (`ai.Service` → ai-manager → **claude-v3**, with `no_fallback`).
  Case automation jobs (`case_run`, `case_create`) are enqueued by the case endpoints; `case_run` is dispatched to playwright-executor (`internal/api/jobs_executor.go`).
- **playwright-executor** is the only place Playwright runs. It is a small Node/TS service on rnd-v3, run as a
  systemd user unit with Node from mise, and capped by its cgroup at 2.5 GiB RAM and 60 % of all CPUs.
  It runs a product's automation specs (`case_run`) from the repo pool against HISv3 staging.
  It makes no AI calls and no calls to the backend: the backend polls it.
- **dashboard** is the React + Vite UI. Every route except `/login` is gated by a cookie-session login.
- The old `playwright-qa`, `playwright-docs` prototypes and the dashboard-driven `qa_run` / `docs_generate` jobs were removed (legacy cleanup). Git history keeps them.
- **this-qa-automation-system-playwright** is the E2E self-verification suite for the backend API and the dashboard.
- **LLM policy:** ai-manager pinned to `claude-v3` (the Claude CLI REST on rnd-v3) is the only supported path.
  The MiniMax provider code is **dormant**: it is never selected by default and must not be switched on without the owner's explicit approval.

## Subdirectories & System Boundaries

- `ai-backend/`: [ai-backend/README.md](./ai-backend/README.md). Go API service: router, auth, config, MySQL, AI provider, skills, learning.
  - [internal/api](./ai-backend/internal/api/README.md): HTTP router, `/health`, `/api/*` handlers, `/api/auth/*`, job records and case automation.
  - [internal/auth](./ai-backend/internal/auth/README.md): bcrypt passwords, MySQL `sessions`, `Require` middleware.
  - [internal/config](./ai-backend/internal/config/README.md): `LoadConfig` (env > `.env` > `config.yaml`) and the `GetDatabaseURL` MySQL DSN.
  - [internal/db](./ai-backend/internal/db/README.md): MySQL 8 connection pool and idempotent `RunMigrations`.
  - [internal/services/ai](./ai-backend/internal/services/ai/README.md): `ChatProvider`. `aiManagerProvider` (claude-v3) is the only supported provider; `minimaxProvider` is dormant.
  - `internal/services/executor`: playwright-executor client (`Submit`, `Get`, `Wait`, `Health`).
  - [internal/services/skills](./ai-backend/internal/services/skills/README.md): skill registry (`skills`, `skill_logs`) and built-in skills.
  - [internal/services/learning](./ai-backend/internal/services/learning/README.md): legacy learning cache and navigation edges of the removed crawler (no longer read by the dashboard).
  - [internal/services/learnloop](./ai-backend/internal/services/learnloop/README.md): the learning loop: findings, policy proposals, learned policies in the automation repo.
  - [internal/services/navmap](./ai-backend/internal/services/navmap/README.md): pure builder of the route and page inventory (NAVIGATION_MAP.md).
- `dashboard/`: [dashboard/README.md](./dashboard/README.md). React SPA. Calls same-origin `/api/*` with `credentials: 'include'`.
- `playwright-executor/`: [playwright-executor/README.md](./playwright-executor/README.md). Playwright run service (`case_run`). It has an HTTP API with a bearer token, concurrency 1, and is staging only.
- `deploy/playwright-executor/`: [deploy/playwright-executor/README.md](./deploy/playwright-executor/README.md). systemd user unit template, `install.sh`, and the resource budget.
- `this-qa-automation-system-playwright/`: [this-qa-automation-system-playwright/README.md](./this-qa-automation-system-playwright/README.md). Playwright API and dashboard regression suite for this repository.
- `lib/`: [lib/README.md](./lib/README.md). Shared TypeScript `config.yaml` loader (`lib/config.ts`); currently has no importer.
- `deploy/his-v3-qa/`: [deploy/his-v3-qa/README.md](./deploy/his-v3-qa/README.md). **Production** Docker Compose stack on rnd-v3-vpn, behind the clino nginx edge.
- `product-documentations/`: functional documentation source for HIS v3 modules (reference only; the backend no longer serves it).
- Root `*.md` files: operational docs, plans, tutorials, assessments and architecture notes.

## Global Invariants

- **Auth depends on the DB.** `ai-backend/cmd/server/main.go` wires cookie-session auth (`router.SetAuth`) only when `db.NewConnection` succeeds.
  If `/health` reports `"database":"disconnected"`, the API is open.
- **Migrations are explicit.** The server never migrates. Run `go run ./cmd/databasemigration`. It is idempotent: `CREATE TABLE IF NOT EXISTS`, append-only.
- **One LLM path: ai-manager → claude-v3.** Backend LLM calls go through `ChatProvider`, which is pinned to `AI_MANAGER_PROVIDER=claude-v3` with `AI_MANAGER_NO_FALLBACK=true`.
  `AI_PROVIDER=minimax` still exists in code but is dormant and needs explicit owner approval. playwright-executor makes no LLM calls.
- **Playwright runs only in playwright-executor.** It is bound to the WireGuard IP, runs one browser at a time, and its systemd cgroup caps it at `MemoryMax=2560M` and `CPUQuota = nproc × 60 %`.
  It only runs specs from the configured repo pool against allow-listed hosts. When `PLAYWRIGHT_EXECUTOR_URL` is empty, `case_run` jobs fail with "playwright-executor is not configured".
- **Same-origin API.** The dashboard uses relative `/api/*` paths. They go through the Vite proxy in dev and the in-container nginx in production.
  Backend CORS comes from `DASHBOARD_ORIGIN` (`router.go:applyCORS`).
- **Secrets** live in `.env`, `config.yaml` (both gitignored) and, in production, in `.env.prod` on rnd-v3 only. Never commit them.

## Global Side Effects

- MySQL 8 database (`qa_automation`): see [ai-backend/internal/db](./ai-backend/internal/db/README.md) for tables.
- Outbound LLM calls to ai-manager (`POST {AI_MANAGER_URL}/v1/chat`, default `http://10.10.0.4:8099` over WireGuard), always answered by `claude-v3`. ai-manager bills and logs these calls.
- playwright-executor runs `npx playwright test` inside the product's repo-pool checkout on rnd-v3 for `case_run`.
- Read-only user import from the taskmgr Neo4j at backend startup, when `TASKMGR_NEO4J_URI` is set.
- The self-test suite mutates backend data: it creates and deletes learning records.

## System Verification

- Backend: `cd ai-backend && go test -mod=mod ./...`
- Dashboard: `cd dashboard && npm run build` (and `npm run lint`)
- Playwright executor: `cd playwright-executor && mise exec -- npm test`
- Self-tests (backend on :8080, dashboard on :3000): `cd this-qa-automation-system-playwright && npx playwright test`
- AI provider smoke test: `cd ai-backend && go run ./cmd/verifyai`
- Production health: see [deploy/his-v3-qa/README.md](./deploy/his-v3-qa/README.md#verification)

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

- A MySQL 8 database (production uses `qa_automation` on rnd-v2-vpn over WireGuard)
- ai-manager credentials (`AI_MANAGER_LOGIN_ID` / `AI_MANAGER_SECRET_KEY`) and WireGuard access to it. No other LLM credentials are needed: claude-v3 via ai-manager is the only supported path.
- For real Playwright jobs: a reachable playwright-executor (`PLAYWRIGHT_EXECUTOR_URL` and `_TOKEN`). Without it, `case_run` jobs fail.

---

## Quick Start

### Option 1: Docker Compose (local)

The root `docker-compose.yml` builds `backend` (:8080) and `dashboard` (:3000 -> 80) and mounts `./config.yaml` into the backend.
For production, use [deploy/his-v3-qa](./deploy/his-v3-qa/README.md) instead.

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
AI_MANAGER_URL=http://10.10.0.4:8099
AI_MANAGER_LOGIN_ID=...
AI_MANAGER_SECRET_KEY=...
# (AI_PROVIDER=minimax is dormant; do not use)

# PLAYWRIGHT (optional locally; empty = case_run jobs fail)
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
| `AI_MANAGER_URL` | ai-manager base URL | `http://10.10.0.4:8099` |
| `AI_MANAGER_LOGIN_ID`, `AI_MANAGER_SECRET_KEY` | ai-manager HTTP Basic credentials | — |
| `AI_MANAGER_PROVIDER`, `AI_MANAGER_NO_FALLBACK`, `AI_MANAGER_MODEL` | Upstream pinned inside ai-manager (Claude CLI REST on rnd-v3 `10.10.0.9:8105`) | `claude-v3`, `true`, empty (`claude-opus-5`) |
| `AI_TIMEOUT_SECONDS` | LLM call timeout | `180` |
| `MINIMAX_API_KEY`, `AI_MODEL`, `AI_BASE_URL`, `AI_CHAT_ENDPOINT` | **Dormant** MiniMax settings (unused) | `MiniMax-M2.5`, `https://api.minimax.io/v1`, `/chat/completions` |
| `PLAYWRIGHT_EXECUTOR_URL`, `PLAYWRIGHT_EXECUTOR_TOKEN`, `PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES` | playwright-executor endpoint, bearer token, max wait per job | empty (case_run fails), —, `30` |
| `GAOS_API_URL`, `GAOS_API_TOKEN` | GAOS REST API base URL (https, or http to loopback/`10.10.0.0/24` only) and the QA bearer (`gaos_...`, secret); both empty = the GAOS integration is off ([GAOS_INTEGRATION.md](docs/designs/GAOS_INTEGRATION.md); how the two systems work together, both ways: [GAOS_COLLABORATION.md](docs/GAOS_COLLABORATION.md)) | empty, — |
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
| Production | `https://qa-tool.sismedika.com` (`/api`, `/health` proxied by the dashboard nginx) |

### Endpoints

The full, code-verified list with methods is in [ai-backend/internal/api/README.md](./ai-backend/internal/api/README.md).
Only `/health` and `/api/auth/login` are public. When the backend has a DB, every other `/api/*` endpoint requires the session cookie.

| Endpoint | Methods | Description |
|----------|---------|-------------|
| `/health` | GET | Status, DB, AI provider and AI health |
| `/api/auth/login`, `/api/auth/logout`, `/api/auth/me`, `/api/auth/change-password` | POST / GET | Cookie-session auth (only when DB connected) |
| `/api/auth/impersonate`, `/api/auth/impersonate/stop`, `/api/users` | POST / GET | Admin/Service impersonation of another user, and the user list for its picker ([spec](docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md)) |
| `/api/skills`, `/api/skills/logs`, `/api/skills/stats` | GET (+POST logs) | Skills |
| `/api/cost/overview`, `/trend`, `/efficiency`, `/by-feature`, `/by-model`, `/recent`, `/filters`, `/gateway` | GET | Cost analytics (AI call ledger + ai-manager usage) |
| `/api/products/{id}/navigation` (+ `/routes/{key}`) | GET | Route and page map of a product with coverage and the next cases to automate ([spec](docs/designs/NAVIGATION_MAP.md)); the dashboard's `/navigation` |
| `/api/learning` (+ `/navigation`) | GET, POST, DELETE | Legacy crawler learning records (no longer read by the dashboard) |
| `/api/learning/findings`, `/proposals`, `/timeline` (+ `/{id}/dismiss\|reopen\|check`) | GET / POST | Learning loop: findings from automation work, policy proposals (`policy_propose` jobs) for the automation repo, PR tracking ([spec](docs/designs/LEARNING_LOOP.md)) |
| `/api/jobs`, `/api/jobs/{id}` (+ `/events`, `/approval`) | GET / POST | Job records (case automation); `/approval` takes `via` (`job-page`, `history`, `learning`) and relays a run-history answer to GAOS ([spec](docs/designs/HISTORY_ANSWER.md)) |
| `/api/history/builds`, `/runs`, `/syncs`, `/approvals` | GET | Run history; `/approvals` lists every approval waiting now, answered from `/history` ([spec](docs/designs/RUN_HISTORY.md), [answer](docs/designs/HISTORY_ANSWER.md)) |

### Example API Calls

```bash
# Health check
curl http://localhost:8080/health

# Log in (when auth is enabled) and reuse the session cookie
curl -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"<user>","password":"<password>"}'

# List recent jobs
curl -b cookies.txt http://localhost:8080/api/jobs
```

---

## Deployment

### Production: his-v3-qa (Docker Compose on rnd-v3-vpn)

Production runs at **https://qa-tool.sismedika.com**. A Docker Compose stack on rnd-v3-vpn runs `dashboard` (nginx + SPA, which proxies `/api` and `/health`) and `backend`.
It sits behind the shared clino nginx edge and reaches MySQL and ai-manager over WireGuard.
See [deploy/his-v3-qa/README.md](./deploy/his-v3-qa/README.md) for deploy, rollback and health checks.

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
├── deploy/his-v3-qa/         # Production Docker Compose stack
├── deploy/playwright-executor/ # systemd user unit + install.sh (2.5 GiB / 60 % CPU)
├── lib/                      # Shared TS config loader
├── playwright-executor/      # THE Playwright runtime (HTTP service, mise node 24)
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
- [Production Deployment (his-v3-qa)](./deploy/his-v3-qa/README.md)
- [REST API Usage](./rest-api-usage-documentation.md)
- [Legacy Cloud Run Deployment Tutorial](./deployment-tutorial.md)
- `MiniMax-API-Endpoint.pdf` and `Playwright-for-System-Documentation.pdf` (local reference PDFs; `*.pdf` is gitignored)

---

## License

Copyright © 2025 Sismedika. All rights reserved.
