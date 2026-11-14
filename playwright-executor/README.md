# playwright-executor

## Overview

A small Node/TypeScript HTTP service that runs **every** Playwright job of qa-automation against SUT **staging**. ai-backend submits runs and polls for results. The executor holds only browser-driving logic: it makes no LLM calls and no calls back to the backend.

It runs on app-host (`10.8.0.9:8141`) as a **systemd user unit**, with Node provided by **mise**. A kernel cgroup caps it at **2.5 GiB RAM** and **60 % of all CPUs**. See [deploy/playwright-executor](../deploy/playwright-executor/README.md).

It replaces the run paths of `playwright-qa/` (exploration, skills) and `playwright-docs/` (flow recording), which are now deprecated.

```
dashboard ──POST /api/jobs/qa/run──▶ ai-backend ──POST /v1/runs──▶ playwright-executor ──Chromium──▶ SUT staging
                                         ▲    └──GET /v1/runs/:id (poll 3 s)──┘
                                         └─ issues → raw_issues; docs → Markdown via ai-manager (claude-v3)
```

## Interface

### HTTP API

Every route except `/health` requires `Authorization: Bearer $EXECUTOR_TOKEN`, compared in constant time.

| Route | Purpose |
|---|---|
| `GET /health` | `{status: ok\|degraded, service, busy, queued, active?, chromium: installed\|missing, roles, memory:{current_mb,max_mb,peak_mb}, rss_mb}`. Returns 503 when Chromium is missing. |
| `POST /v1/runs` | Body `{job_id, type, params}`. Returns 202 `{id, status:"queued", job_id}`, 400 on validation errors, 401 without a token, 429 when the queue is full. |
| `GET /v1/runs/:id[?since=n]` | The full `RunRecord`: `{id, job_id, type, params, status: queued\|running\|completed\|failed, events[], result?, error?, created_at, started_at?, completed_at?, duration_ms?}`. With `since=n` (non-negative integer, else 400) `events` holds only entries with `n > since`. Returns 404 for unknown ids or ids that have expired. |

`params` (`src/validate.ts`):

| Param | Meaning |
|---|---|
| `roles` | Credential role names. Default `EXECUTOR_DEFAULT_ROLES`; maximum 5. |
| `max_steps` | Random clicks per role, 1..50. Default 10. |
| `interactive_skills` | When true, also runs `required_field`, which clicks submit. Default false. |
| `flows` | `docs_generate` only: flow ids to restrict the run to. |

The target URL is **never** taken from the request.

`events` is a ring of the last 500 `{n, at, level: info|warn|error, step, message}`; `n` is monotonic per run from 1. Only `case_run` emits events today.

`case_run` params (`validate.ts`, `caseFs.ts`; any other key is a 400):

| Param | Rule |
|---|---|
| `repo_dir` | Absolute. Its `realpath` must be strictly inside `EXECUTOR_REPO_POOL` and contain `playwright.config.ts`. |
| `case_code` | `^TC-[A-Z0-9]+-[0-9]{3,4}$` |
| `spec_paths` | 1..5 repo-relative paths ending `.spec.ts`, under `tests/web/` or `tests/api/rest/`, no `..`, no `mq` segment, must exist. Each `realpath` must stay inside the repo. |
| `env_file` | A file name matching `automation-*.env` that exists in `EXECUTOR_ENV_DIR` as a regular file (not a symlink). Never a path. |
| `repair` | Optional integer 0..99, attempt number (log only). |

### Run types

| Type | Code | Result |
|---|---|---|
| `qa_run` | `src/runs/qaRun.ts` | Per role: UI login, then `max_steps` random same-origin clicks. Destructive, logout and submit controls are skipped. Built-in skills run on each newly visited page, and page JS errors and HTTP 5xx responses are captured. Result: `{target, roles[], issues[], summary}`. Each issue has `{role, url, type: skill\|page_error\|http_5xx\|login, skill?, severity, title, message, elements?}`. |
| `docs_generate` | `src/runs/docsGenerate.ts` | Per role: UI login, then replays `src/flows/flows.ts`, taking a screenshot plus the page title and heading after each step. Result: `{target, artifacts_dir, flows[], role_errors, summary}`. Screenshots go under `EXECUTOR_OUTPUT_DIR/<run_id>/<role>/<flow>/step-NN.png`. |


`case_run` (`src/runs/caseRun.ts`): runs one test case of an automation repo clone. `npm ci` runs only when `sha256(package-lock.json)` differs from `node_modules/.qa-lock`. Then it runs `npx playwright test <specs> --project=<web-*|api-rest> -g <case_code> --workers=1 --retries=0 --reporter=json,list` with `VENDOR_E2E_RUN=<run id>`, `TEST_ENV=staging` and the env file loaded. No executor browser is launched. Result: `{exit_code, passed, failed, skipped, report, tail, report_error?}`. `report` is the parsed `<repo>/.runs/<run id>/results.json` (at most 2 MiB, else `null` plus `report_error`). Counts come from `report.stats` (`passed` = expected + flaky). `tail` is the last 4 KiB of redacted stdout. A non-zero exit is still a `completed` run; a refused host, bad env file or failed `npm ci` makes the run `failed`. Event steps: `validate`, `install`, `run`, `report`, `done`.

Built-in skills (`src/skills/builtin.ts`): `max_length`, `label_association`, `input_type` and `multi_tab_save` are read-only. `required_field` is interactive.

### Configuration (env only, `src/config.ts`)

`.env.example` lists every key:

| Key | Purpose |
|---|---|
| `EXECUTOR_BIND`, `EXECUTOR_PORT` | Listen address. Production uses `10.8.0.9:8141`. |
| `EXECUTOR_TOKEN` | Bearer token, at least 32 chars. Also set in the backend's `PLAYWRIGHT_EXECUTOR_TOKEN`. |
| `TARGET_STAGING_URL` | Staging only. |
| `TARGET_CREDENTIALS_FILE` | JSON `{role: {username, password, role_code}}`, mode 600, generated from `config.yaml` `credentials`. |
| `EXECUTOR_DEFAULT_ROLES` | Roles used when a run names none. |
| `EXECUTOR_OUTPUT_DIR`, `EXECUTOR_RETENTION_DAYS` | Where artifacts go and how long they are kept (default 7 days). |
| `RUN_TIMEOUT_SECONDS` | Per-run limit (900). |
| `MAX_QUEUE` | Maximum waiting runs (10). |
| `EXECUTOR_HEADLESS` | Run Chromium headless. |
| `EXECUTOR_REPO_POOL` | `case_run`: directory of automation repo clones (default `/home/qa/devel/projects/qa-automation-tools/repos`). |
| `EXECUTOR_ENV_DIR` | `case_run`: directory of `automation-<productId>.env` files, mode 600 (default `~/.config/qa-automation`). |
| `EXECUTOR_ALLOWED_HOSTS` | `case_run`: comma-separated hostnames (`*.suffix` allowed) that the env file's http(s) URL values may point at. Default: the `TARGET_STAGING_URL` host. |

## Invariants

- **One browser at a time.** `RunQueue` (`src/queue.ts`) has concurrency 1, and after an abort it waits for the old browser to close before starting the next run. Together with the low-memory Chromium flags (`src/browser.ts`), this keeps the service inside its 2.5 GiB cgroup.
- **Timeouts kill the browser.** When a run exceeds `RUN_TIMEOUT_SECONDS`, its `AbortSignal` fires, the browser is closed, and the run is marked `failed`.
- **Staging only.** The target comes from `TARGET_STAGING_URL`, random clicks stay on that origin, and the explorer never clicks submit buttons or controls whose text looks destructive.
- **No secrets in source or logs.** Credentials come only from `TARGET_CREDENTIALS_FILE`. Logs print role names and the staging host, never credentials or the token.
- **No AI, no backend calls.** The pull model means the executor needs no backend credentials (hard rule 2).
- **State is in memory.** Run records last 24 h and are lost on restart. The backend marks its own orphaned jobs as failed at startup (`FailOrphanedJobs`).
- **`case_run` child processes.** Spawned with argument arrays (no shell), in their own process group (abort or timeout sends SIGTERM, then SIGKILL, to the group). The child env is `PATH HOME USER LANG LC_ALL TZ TMPDIR PLAYWRIGHT_BROWSERS_PATH XDG_RUNTIME_DIR CI NO_COLOR` plus only the chosen env file's keys; the executor's own token and credentials never reach it. `npm ci` gets no env-file keys.
- **`case_run` refusals.** The run fails before spawning unless every http(s) URL value in the env file is on `EXECUTOR_ALLOWED_HOSTS` (and at least one exists) and the env file is mode 600. Paths are re-confined at run time, after POST. `mq` specs are refused.
- **`case_run` output is redacted.** Env-file values (4+ chars), `gh*_`/`github_pat_`/`gaos_` tokens, `Bearer` tokens and URL userinfo are masked, then each line is clipped to 300 chars.
- **Never bind `0.0.0.0`.** `install.sh` refuses to.

## Side Effects

- Drives Chromium against SUT staging. Logins and clicks are real. `required_field` (opt-in) submits empty forms.
- Writes screenshots to `EXECUTOR_OUTPUT_DIR` and prunes run directories older than `EXECUTOR_RETENTION_DAYS` at the start of each run.
- Writes JSON lines to stdout, which journald collects (`journalctl --user -u playwright-executor`).

## Verification

```bash
cd playwright-executor
mise exec -- npm ci
mise exec -- npm run typecheck
mise exec -- npm test          # unit: auth, validation, queue ordering/timeout, HTTP API
mise exec -- npm run build
```

For production verification (limits, a real run, and backend integration), see [deploy/playwright-executor/README.md](../deploy/playwright-executor/README.md#verification).
