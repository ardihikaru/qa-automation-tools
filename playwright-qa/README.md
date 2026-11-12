# Playwright QA System

> **Deprecated (2026-11-13).** Superseded by [playwright-executor](../playwright-executor/README.md).
> Its login, random exploration and built-in skills were ported there (`src/runs/qaRun.ts`, `src/skills/builtin.ts`).
> Nothing runs this package any more, and it still doesn't compile (see Invariants). It stays only for reference until a cleanup PR deletes it.
> Don't extend it, and don't run it against production.

## Overview

Autonomous exploration and bug detection. A TypeScript/Playwright package that logs into
the SUT staging frontend as several roles, clicks through the UI (random, learning-guided,
or goal-planning modes), runs QA "skills" (DOM checks) on each page, and reports flows,
issues, and learning records to the QA backend over HTTP
(see [ai-backend API](../ai-backend/internal/api/README.md)).

Key feature: exploration, learning, skills, goal detection. Several pieces are still
stubs (see Invariants), so treat this package as a prototype rather than a production
runner.

### Exploration Modes

| Mode | Description | Where |
|------|-------------|-------|
| `random` | Random element click | `src/explorer/random.ts` `randomStep`; `QARunner.randomStep` |
| `guided` | Learning-based action selection (80% exploit cache, 20% explore) | `src/explorer/guided.ts` `guidedStep`, `shouldExplore`, `scoreAction`, `SCORING_WEIGHTS` |
| `planning` | Multi-step planning agent (30% planning, else random click) | `src/explorer/planner.ts` `generateActionTree`, `executePlan`, `usePlanningMode`, `KNOWN_GOALS` |

### Built-in QA Skills (`BUILTIN_SKILLS` in `src/skills/builtin/skills.ts`)

1. **Max Length** (`max_length`, selector, high) - flags `textarea`/text `input` without `maxlength`
2. **Required Field** (`required_field`, validation, high) - submits a form with empty required fields
3. **Data Persistence** (`data_persistence`, data, medium) - fills a `form[data-persist]` input, reloads, checks the value
4. **Multi-tab Save** (`multi_tab_save`, selector, medium) - looks for tab containers (`.tab-content`, `[data-tab]`, `[role="tablist"]`)

## Interface

### Structure

All code lives under `src/` (`tsconfig.json` `rootDir: ./src`). The top-level
`config/`, `explorer/`, `logger/`, `runner/`, and `skills/{builtin,learned}/`
directories are empty placeholders with no files.

| Module | Responsibility | Public symbols |
|--------|----------------|----------------|
| `src/explorer/random.ts` | Random exploration per configured role | `main`, `loginAs(page, baseUrl, credentials)`, `randomStep`, `getClickableElements` |
| `src/explorer/guided.ts` | Guided exploration with the backend learning cache | `main`, `guidedStep`, `loadLearningCache`, `scoreAction`, `shouldExplore`, `SCORING_WEIGHTS` |
| `src/explorer/planner.ts` | Planning, goal detection | `main`, `loadConfig`, `generateActionTree`, `executePlan`, `checkGoalAchievement`, `determineGoal`, `usePlanningMode`, `KNOWN_GOALS`, types `Goal`, `Plan`, `PlanStep`, `Action` |
| `src/runner/run.ts` | Main execution loop tying modes, skills, logger | `QARunner`, `main`, types `ExplorationMode` (`'random' \| 'guided' \| 'planning'`), `RunnerConfig` |
| `src/logger/api.ts` | Batched log shipping to the backend | `APILogger` (`log`, `logFlow`, `logSkill`, `logIssue`, `logGoal`, `logExploration`, `flush`, `close`), `createLogger`, `logPageState`, types `LogEntry`, `LogLevel`, `LogTarget`, `FlowLog`, `SkillLog`, `IssueLog`, `GoalLog` |
| `src/skills/builtin/skills.ts` | Hardcoded skills | `BUILTIN_SKILLS`, `SkillRunner` (`runSkill`, `runAllBuiltin`, `runByName`), `runSkills`, `getBuiltinSkills`, `getSkillByName`, types `Skill`, `SkillResult` |
| `src/skills/learned/learnedSkills.ts` | Learned skills | `LearnedSkillRunner` (`loadFromBackend`, `runSkill`, `runAll`), `generateSkillFromIssues`, `analyzePageForPatterns`, types `LearnedSkill`, `IssueMatch` |

### Configuration (environment variables)

- `src/runner/run.ts`: `MODE` (default `random`), `RUN_SKILLS` (`false` disables), `MAX_ITERATIONS` (default `10`).
- `src/explorer/guided.ts` / `planner.ts` `loadConfig`: `STAGING_URL`, `ADMIN_USER`/`ADMIN_PASS`,
  `DOCTOR_USER`/`DOCTOR_PASS`, `CASHIER_USER`/`CASHIER_PASS`, `API_URL` (default
  `http://localhost:8080`), `MAX_STEPS`, `TIMEOUT`, `PLAN_DEPTH` (planner only); then
  overridden by `../config.yaml` (`staging.url`, `credentials.admin|doctor_gp|cashier`)
  relative to `process.cwd()` if present.
- `src/explorer/random.ts`: is meant to read the shared config through `getStaging`, `getPlaywrightQA`,
  `getAllCredentials`, `getCredentials`, `getBackendURL` ([lib/config](../lib/README.md)). Its import path is
  currently broken (see Invariants).
- `src/logger/api.ts` `createConfig`: `API_URL`, `LOG_BATCH_SIZE` (10), `LOG_FLUSH_INTERVAL` (5000 ms),
  `LOG_RETRY_COUNT` (3), `LOG_RETRY_DELAY` (1000 ms).
- `src/skills/learned/learnedSkills.ts`: `API_URL`.

### Backend endpoints used

| Endpoint | Method | Caller |
|----------|--------|--------|
| `/api/flows` | POST | `random.ts` `logAction`; `APILogger` target `flow` (and default) |
| `/api/skills` | POST | `APILogger` target `skill` |
| `/api/skills?builtin=false` | GET | `LearnedSkillRunner.loadFromBackend` |
| `/api/issues` | POST | `APILogger` target `issue` |
| `/api/analytics` | POST | `APILogger` target `goal` |
| `/api/learning` | GET / POST | `guided.ts` `loadLearningCache` / `recordToLearningCache`; `APILogger` target `exploration` |
| `/api/search?q=` | GET | `guided.ts` `queryLearningCache` |

## Invariants

- **Known build breakage** (as of this revision): `src/explorer/random.ts` imports
  `../../lib/config`, which resolves to `playwright-qa/lib/config` (does not exist; the
  shared module is the repo-root `lib/config.ts`). `src/runner/run.ts` imports
  `loadConfig` and `Config` from `../explorer/random`, which exports neither, and calls
  `loginAs(page, role, config)` while `random.ts` defines `loginAs(page, baseUrl, credentials)`.
  `npm run build` (`tsc`) is therefore expected to fail until these are fixed.
- Every module with a `main()` (`random.ts`, `guided.ts`, `planner.ts`, `run.ts`) calls
  `main().catch(console.error)` at import time. Importing `../explorer/random` from
  `run.ts` therefore also starts the random explorer.
- `QARunner` does not use the explorer modules: its `guidedStep` falls back to `randomStep`
  and `planningStep` falls back to `guidedStep` (inline stubs in `run.ts`). It always runs
  roles `admin`, `doctor`, `cashier`, headless with `--no-sandbox`, viewport 1280x720.
- `generateSkillFromIssues` is heuristic (`detectPatterns` over error strings), not
  AI-generated; it makes no backend/LLM call.
- `APILogger.flush` sends the whole queued batch to the endpoint of the **first** entry's
  `target` (`getEndpoint`), so mixed-target batches land on one endpoint. Failed batches
  are re-queued after `retryCount` attempts.
- All backend calls are unauthenticated (no cookie/credentials). The current backend
  gates every path except `/health` and `/api/auth/login` behind a session when auth is
  enabled (`ai-backend/internal/auth/middleware.go` `IsAuthAllowList`), so reporting to
  a production backend returns 401 unless auth is disabled.
- When env vars and `config.yaml` are absent, `guided.ts`/`planner.ts` fall back to
  hard-coded staging URL and role credentials in `loadConfig`. Do not add more; prefer
  `config.yaml`/env.

## Side Effects

- Launches Chromium via `chromium.launch` and drives the staging SUT frontend
  (`<stagingUrl>/auth/login`, then real clicks). `planner.ts` `simulateAction` performs
  real clicks, not a dry run.
- Skills mutate page state: `required_field` clicks `button[type="submit"]`;
  `data_persistence` fills an input and calls `page.reload()`.
- HTTP POSTs to the backend endpoints above (flows, skills, issues, analytics, learning).
- `APILogger` starts a `setInterval` flush timer on construction; call `close()` to stop it.
- Reads `../config.yaml` from disk (`guided.ts`, `planner.ts`); `random.ts` reads config via `lib/config`.
- `random.ts`/`guided.ts`/`planner.ts` call `process.exit(1)` on unhandled errors in `main`.

## API Testing

### Production API

The current production backend is reached through the dashboard at
https://qa.mfardiansyah.id (Docker Compose stack on `app-host`, see
[deploy/qa-tool](../deploy/qa-tool/README.md)); nginx proxies `/api` and `/health`
to the backend. The old Cloud Run `ai-backend-...run.app` URL is legacy.

### Testing Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check (public) |
| `/api/flows` | GET/POST | CRUD flows |
| `/api/issues` | GET/POST | Issue tracking |
| `/api/skills` | GET | QA skills |
| `/api/search` | GET | Search learning cache |
| `/api/cost/*` | GET | Cost analytics |

### Manual Testing

```bash
BASE_URL="https://qa.mfardiansyah.id"

# Health (no session needed)
curl $BASE_URL/health

# Everything else needs a session cookie from /api/auth/login
curl -c /tmp/qa.cookies -H 'Content-Type: application/json' \
  -d '{"username":"<user>","password":"<password>"}' $BASE_URL/api/auth/login
curl -b /tmp/qa.cookies $BASE_URL/api/flows
curl -b /tmp/qa.cookies "$BASE_URL/api/search?q=login"
```

### Test Results

See the root `TEST-RESULTS.md` (gitignored, local-only; template: [`../TEST-SCENARIO.md.example`](../TEST-SCENARIO.md.example)) for detailed endpoint test results including:
- Status codes for all endpoints
- Known issues (NULL values, status code mismatches)
- Error handling validation

## Verification

`package.json` scripts (run `cd playwright-qa && npm install` first; there is no lockfile or `node_modules`):

- Type-check/compile: `cd playwright-qa && npm run build` (`tsc`; currently fails, see Invariants)
- Random explorer: `cd playwright-qa && npm run explore` (`ts-node src/explorer/random.ts`)
- Runner: `cd playwright-qa && npx ts-node src/runner/run.ts`
- `npm run dev` / `npm start` target `src/index.ts` / `dist/index.js`, which do not exist.
- `npm test` runs `playwright test`, but `@playwright/test` is not a dependency and there
  are no spec files, so it has nothing to run.
