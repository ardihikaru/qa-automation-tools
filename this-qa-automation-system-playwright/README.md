# QA Automation System - Self-Verification Tests

## Overview

This directory contains Playwright-based tests for verifying the QA Automation System itself. The tests ensure that both the REST API backend ([../ai-backend/README.md](../ai-backend/README.md)) and the Dashboard UI are functioning correctly. The detailed test plan is in [TEST-PLAN.md](./TEST-PLAN.md).

There are 83 tests in 13 spec files: 48 API tests under `tests/api/` (Playwright `request` fixture) and 35 dashboard tests under `tests/dashboard/` (Playwright `page` fixture). All of them target **local** services at fixed URLs from `config.ts`. They do not target the deployed QA stack ([../deploy/qa-tool/README.md](../deploy/qa-tool/README.md)).

> **Current drift (read before running):** these tests were written before two
> backend changes and have not been updated:
>
> 1. **AI provider.** The backend now defaults to `AI_PROVIDER=ai-manager`. `/health`
>    (`ai-backend/internal/api/router.go` `serveHealth`) returns
>    `{status, database, ai_provider, ai, minimax}`, and `minimax` is
>    `"configured"` only when the provider is `minimax`. So
>    `tests/api/health.spec.ts` "should show minimax configured" fails on a
>    default (ai-manager) backend.
> 2. **Authentication.** When the backend has a database, `cmd/server/main.go`
>    calls `router.SetAuth(...)`, and every path except `/health` and
>    `/api/auth/login` requires a session cookie (401 otherwise). The dashboard
>    wraps all routes in `ProtectedRoute` and redirects to `/login`. None of the
>    specs log in, so most API and dashboard tests fail against an auth-enabled backend.
>    They only pass against a backend running without auth (no DB, standalone mode).
>    The tests that need DB data (for example `database: 'connected'`) fail in that mode.

## System Under Test

### Backend (REST API)
- **Base URL**: http://localhost:8080 (`config.backendUrl`; backend default `PORT` 8080)
- **Database**: MySQL 8, set via `DB_*` or a MySQL `DATABASE_URL` DSN (`ai-backend/internal/config/config.go` `GetDatabaseURL`). Use a disposable database because the tests write data.
- **AI**: production ai-manager by default (`AI_PROVIDER=ai-manager`). MiniMax direct is the fallback (`AI_PROVIDER=minimax`).

### Dashboard (Web UI)
- **Base URL**: http://localhost:3000 (`config.dashboardUrl`; Vite `VITE_DEV_SERVER_PORT` default 3000)
- **Framework**: React + Vite + Tailwind CSS

## Prerequisites

1. **Backend Running**: Start the AI Backend server. Pass secrets via the environment and never commit them.
   ```bash
   cd ai-backend
   DB_HOST="..." DB_PORT="..." DB_USER="..." DB_PASSWORD="..." DB_NAME="..." \
   AI_MANAGER_LOGIN_ID="..." AI_MANAGER_SECRET_KEY="..." \
   ./server
   # MiniMax fallback instead: AI_PROVIDER=minimax MINIMAX_API_KEY="..." ./server
   ```

2. **Dashboard Running**: Start the dashboard
   ```bash
   cd dashboard
   npm run dev
   ```

3. **Verify Services**:
   ```bash
   # Check backend health
   curl http://localhost:8080/health
   # Returns: {"status":"ok","database":"connected|disconnected","ai_provider":"...","ai":"...","minimax":"configured|not_configured"}

   # Check dashboard
   curl http://localhost:3000
   ```

4. **Browser**: `playwright.config.ts` sets `channel: 'chrome'`, so Google Chrome must be installed. You can also run `npx playwright install chrome`.

## Installation

```bash
cd this-qa-automation-system-playwright
npm install
```

## Running Tests

```bash
# Run all tests
npm test

# Run API tests only
npm run test:api

# Run dashboard tests only
npm run test:dashboard

# Run with headed browser
npm run test:headed

# View HTML report
npm run report
```

Note: `playwright.config.ts` only configures the `list` reporter. `npm run report` (`playwright show-report`) has nothing to show unless you run with `--reporter=html`.

## Interface

- `config.ts` exports `config`:
  - `backendUrl`: `QA_BACKEND_URL` env, default `http://localhost:8080`
  - `impersonator`, `impersonatorPassword`, `impersonationTarget`: `E2E_IMPERSONATOR_USERNAME`, `E2E_IMPERSONATOR_PASSWORD`, `E2E_TARGET_USERNAME`. Used only by `tests/api/impersonation.spec.ts`, which is skipped when they are unset. The impersonator must have role Admin or Service, and the target should be a QA/QC user. That spec logs in itself. It keeps one `impersonation_audit` row per run, by design; this is an accepted exception to the clean-up rule.
  - `dashboardUrl`: `QA_DASHBOARD_URL` env, default `http://localhost:3000`
  - `apiTimeout`: 10000
  - `pageTimeout`: 30000
  - `expectedSkills`: `['max_length', 'required_field', 'data_persistence', 'multi_tab_save']`
- `playwright.config.ts` (`defineConfig`):
  - `testDir: './tests'`, `fullyParallel: false`, `workers: 1`
  - `retries` is 2 on `CI` and 0 otherwise; `forbidOnly` is set on `CI`
  - `timeout: 30000`, `expect.timeout: 5000`
  - `use`: `baseURL: process.env.QA_BACKEND_URL || 'http://localhost:8080'`, `channel: 'chrome'`, `trace: 'on-first-retry'`, `screenshot: 'only-on-failure'`, `video: 'retain-on-failure'`
  - One project, `chromium` (`devices['Desktop Chrome']`)
- `package.json` scripts: `test`, `test:api`, `test:dashboard`, `test:headed`, `report`
- Spec files (`test.describe` suites): see the Test Files Structure below.

## Invariants

- Specs build URLs from `config.backendUrl` / `config.dashboardUrl`, not from the Playwright `baseURL`. To change the target set `QA_BACKEND_URL` / `QA_DASHBOARD_URL` (e.g. both `https://qa-tool.mfardiansyah.id`); note the suite does not log in, so auth-gated routes return 401 against a DB-backed backend.
- Tests run serially (`workers: 1`, `fullyParallel: false`). Mutating API tests (issues, learning) assume they run in file order against shared data.
- Some API tests accept more than one status code because of the environment: `POST /api/flows` with invalid data accepts `[400, 500]`, `POST /api/learning` accepts `[201, 500]`, `DELETE /api/learning?id=1` accepts `[200, 404]`, and invalid `POST /api/issues` accepts `[201, 400]`.
- Method-not-allowed checks expect `405` for `POST /api/cost/overview`, `PUT /api/flows` and `POST /api/skills`.
- `tests/api/skills.spec.ts` requires every built-in skill in `config.expectedSkills` to be present and every returned skill to have `is_active === true`.
- In dashboard specs, a "critical" console error is any `console.error` whose text does not contain `favicon`, `404` or `net::ERR`. "fetch from API" tests wait up to 10s for a 200 response from the matching `/api/...` URL.
- No spec logs in (no `/api/auth/login` call, no `storageState`). See the drift note in the Overview.

## Side Effects

- **Writes to the backend database** (the tests are not read-only and do not clean up):
  - `POST /api/flows` creates a flow named `Test Flow` (`tests/api/flows.spec.ts`).
  - `POST /api/issues` creates an issue with `page: 'test-page'`, and the invalid-data case may also create one (`tests/api/issues.spec.ts`).
  - `POST /api/issues/status` sets `is_fixed: true` on issue `id: 1`, and `POST /api/issues/notes` overwrites the notes of issue `id: 1`.
  - `POST /api/learning` inserts a `test_intent` record, and `DELETE /api/learning?id=1` deletes learning record 1 (`tests/api/learning.spec.ts`).
- `GET /api/search` is read-only: it runs a `LIKE` query on `learning_cache.intent` (`router.go:handleSearch`) and makes no AI call.
- **Local disk**: Playwright writes traces, screenshots and videos for failed tests to `test-results/` (ignored).
- **Browser**: launches the system Chrome channel for dashboard specs.

Never point these tests at a shared or production database.

## Test Coverage Summary

### API Tests (48 test cases)

| Spec file | Endpoints exercised | Tests |
|-----------|---------------------|-------|
| `health.spec.ts` | `GET /health`, `GET /api/analytics` | 5 |
| `flows.spec.ts` | `GET/POST/PUT /api/flows` | 5 |
| `issues.spec.ts` | `GET /api/issues` (+ `is_fixed`, `category`, `search` filters), `POST /api/issues`, `POST /api/issues/status`, `POST /api/issues/notes` | 9 |
| `skills.spec.ts` | `GET/POST /api/skills`, `GET /api/skills/stats`, `GET /api/skills/logs` (+ `passed`, `skill_id`) | 9 |
| `cost.spec.ts` | `GET/POST /api/cost/overview`, `GET /api/cost/trend`, `GET /api/cost/efficiency`, `GET /api/cost/by-feature` | 9 |
| `learning.spec.ts` | `GET/POST/DELETE /api/learning`, `GET /api/learning/navigation`, `GET /api/learning/stats`, `GET /api/search` | 11 |

Not covered by any spec: `/api/roles`, `/api/ai-cache`, `/api/embedding-cache`, `/api/docs`, `/api/videos`, `/api/goal-events`, `/api/auth/*`.

### Dashboard Tests (35 test cases)

| Route | Spec file | Tests |
|-------|-----------|-------|
| `/` (redirect to /docs), `/qa`, `/issues`, `/cost`, `/fixes`, `/skills` | `navigation.spec.ts` | 8 |
| `/docs` | `docs.spec.ts` | 3 |
| `/qa` | `qa-insights.spec.ts` | 4 |
| `/issues` | `issues.spec.ts` | 4 |
| `/cost` | `cost.spec.ts` | 6 |
| `/fixes` | `fix-tracking.spec.ts` | 4 |
| `/skills` | `skills.spec.ts` | 6 |

Not covered: `/login`, `/navigation`, `/learning` (all present in `dashboard/src/App.tsx`).

## Test Files Structure

```
this-qa-automation-system-playwright/
├── config.ts                    # Test configuration (URLs, timeouts, expected skills)
├── playwright.config.ts         # Playwright config
├── package.json                 # Dependencies and scripts
├── tsconfig.json                # TypeScript config
├── TEST-PLAN.md                 # Detailed test plan
├── tests/
│   ├── api/
│   │   ├── health.spec.ts       # Health & analytics
│   │   ├── flows.spec.ts        # Flow management
│   │   ├── issues.spec.ts       # Issue tracking
│   │   ├── skills.spec.ts       # Skills & logs
│   │   ├── cost.spec.ts         # Cost analytics
│   │   └── learning.spec.ts     # Learning system + search
│   └── dashboard/
│       ├── navigation.spec.ts   # Navigation tests
│       ├── docs.spec.ts         # /docs view
│       ├── qa-insights.spec.ts  # /qa view
│       ├── issues.spec.ts       # /issues view
│       ├── cost.spec.ts         # /cost view
│       ├── fix-tracking.spec.ts # /fixes view
│       └── skills.spec.ts       # /skills view
└── test-results/                # Failure artifacts (generated, ignored)
```

## Known Limitations

1. **Missing API Tests**: Some endpoints lack test coverage (see the lists above).
2. **No Authentication Support**: Specs do not log in, but the backend and dashboard now enforce cookie-session auth when a DB is configured (see the drift note).
3. **Stale health assertion**: `health.spec.ts` still expects `minimax: "configured"`. With the default ai-manager provider, `ai_provider` / `ai` would be the right fields to assert.
4. **Static File Tests**: /api/docs and /api/videos depend on static files and are not tested.
5. **Hardcoded IDs**: issue `id: 1` and learning `id: 1` must exist for the status/notes/delete tests to be meaningful.

## Maintenance

To add new tests:
1. Create spec file in `tests/api/` or `tests/dashboard/`
2. Follow naming convention: `*.spec.ts`
3. Use existing test patterns (`import { config } from '../../config'`)
4. Update this README and [TEST-PLAN.md](./TEST-PLAN.md) with coverage changes

## Reporting

- Default reporter: `list` (console).
- HTML report: run `npx playwright test --reporter=html`, then `npm run report`.
- Failure artifacts (trace on first retry, screenshot, video): `test-results/`.

## Exit Criteria

- All critical API endpoints return 200
- All dashboard routes load without errors
- No console errors in dashboard
- Response times within acceptable limits

## Verification

- Run: `cd this-qa-automation-system-playwright && npx playwright test` (same as `npm test`)
- API only: `npm run test:api`. Dashboard only: `npm run test:dashboard`.
- Requires a running backend on :8080 and dashboard on :3000 (see Prerequisites and the drift note).

---

**Last Updated**: 2026-11-13
**Total Tests**: 83 (48 API + 35 dashboard)
**Pass Rate**: not re-measured after the ai-manager and auth changes. The previous 100% (83/83) result is from 2026-04-18.
