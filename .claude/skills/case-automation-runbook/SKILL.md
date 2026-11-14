---
name: case-automation-runbook
description: Run an existing SUT test case's Playwright spec, or create a new automation spec for a case with claude-v3, through the qa-tool (job states, live event log, approval via dashboard or GAOS Inbox, PR on the automation repo). Use when asked to run/rerun/create automation for a case, watch or debug a case_run/case_create job, or fix why one is stuck or failed.
---

# Case automation runbook

Contract: [docs/designs/CASE_AUTOMATION.md](../../../docs/designs/CASE_AUTOMATION.md). Code: `ai-backend/internal/services/caseauto/` (create state machine), `internal/api/case_automation.go`, `job_events.go`, `playwright-executor/src/runs/caseRun.ts`, dashboard `views/knowledge/CaseAutomation.tsx` and `views/JobView.tsx`.

## Flow
- **Run** (case has `spec_paths`): `POST /api/products/{id}/knowledge/test-cases/{code}/run` → job `case_run` → executor runs `playwright test <spec> -g <CODE>` in the product's automation clone → report → `caseresults.Save` (`kb_case_results`) → GAOS push.
- **Create** (no spec): `POST .../create-automation` → job `case_create`: `prepare` (worktree `<root>.branches/qa-auto--<code>`, branch `qa-auto/<code>`) → `generate` (claude-v3 via `ai.Service` only) → `write` → `run` → `repair` (≤3) → `approval` → `push` → `pr` → `done`.
- **Approval** has two doors: dashboard card (Approve / Reject / Needs changes) and, if the product is GAOS-integrated, an Inbox item `qa-<job>-pr`. First answer wins. `changes` loops ≤2 times (`qa-<job>-pr2`). Nothing is pushed before approval.

## Watching
`GET /api/jobs/{id}/events?since=<seq>` (poll 2 s) or the dashboard `/jobs/{id}`. Executor side: `GET :8141/v1/runs/{id}?since=<n>` (bearer, never print it).

## Prerequisites on app-host (check, never print values)
- `REPO_POOL_HOST_DIR` in `deploy/qa-tool/.env.prod` (host path of the pool the backend sees as `/data/repos`).
- The product has an `automation` repo, verified, cloned in the pool; `npm ci` is done by the executor once per lockfile hash.
- `~/.config/qa-automation/automation-<productId>.env`, mode 600, holding the automation repo's staging variables (see its `env/.env.staging.example`: URLs, `SUT_AUTH_*`, `PROTECTED_HOSTS`). The executor refuses a run if it is missing, not mode 600, or has a host outside `EXECUTOR_ALLOWED_HOSTS`. Ask the user to create it: never invent or print credentials.
- Executor unit re-installed after executor changes (`deploy-qa-tool` skill). Caps unchanged (2.5 GiB / 60 % CPU, one run at a time).

## Troubleshooting
| Symptom | Likely cause |
|---|---|
| 503 on run/create | executor URL, `REPO_POOL_DIR` or `REPO_POOL_HOST_DIR` unset, or AI not configured |
| 409 | case already has/lacks a spec, no automation repo, or an active job (body has its `job_id`) |
| job `failed: no test titled <CODE>` | spec title does not start with the case code |
| executor 400 | param validation (path escapes the pool, mq spec, bad env file name `automation-*.env`) |
| approval stuck open | GAOS webhook failed; the 30 s poller or the dashboard door still answers; expires after 72 h |
| orphaned job after restart | a job with an open approval resumes; others are failed by `FailOrphanedJobs` |

## Rules
- All LLM calls: `ai.Service` / claude-v3 via ai-manager. Never from the executor.
- Writes to the automation repo only through `repoWriteAllowed(push|open_pr)`; staging only; never print env files, tokens or the executor bearer.
- Verify DB-backed code with a scratch MySQL (`QA_TEST_MYSQL_DSN`, e.g. `docker run mysql:8.0`); never the `qa_automation` schema.
