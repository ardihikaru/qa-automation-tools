# GAOS_AUTOMATION: GAOS starts a case automation and watches the job

Status: implemented by PR #83. Builds on [CASE_AUTOMATION.md](CASE_AUTOMATION.md) and [GAOS_INBOUND.md](GAOS_INBOUND.md); the consumer-facing text is in [GAOS_COLLABORATION.md](../GAOS_COLLABORATION.md).

## 1. Goal

GAOS holds the `GAOS_INBOUND_TOKEN` bearer credential and no session, so the session-authenticated `create-automation` and `run` routes are out of its reach. Two bearer routes give it the same two actions and a way to watch the job. They are a thin shim: no automation logic is added.

## 2. Contract

Guard (both routes): bearer `GAOS_INBOUND_TOKEN`, constant-time compare, `503` when the token is unset (closed), `401` for a missing or wrong token, `405` for a wrong method, `503` when the database is unavailable. Neither the token nor a case body is logged.

### `POST /api/v1/gaos/automation`

Body `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"create"|"run"}`.

| Code | When |
|---|---|
| 202 `{job_id}` | a job was started |
| 200 `{job_id}` | the same work is already active: the same case, or for `caseCode` `*` the same product; the active job's id is returned |
| 404 | unknown or unflagged product (`gaos_integrated` + `gaos_product_id`); unknown or removed case |
| 409 | `create` and the case has a spec; `run` and it has none; no automation repo; `*` and no automated case |
| 422 | bad JSON, missing `caseCode`, `mode` not `create` or `run`, or `*` with `create` |
| 503 | executor, repo pool or `REPO_POOL_HOST_DIR` missing, AI missing for `create`, draining |

`create` and `run` of one case go through `enqueueCaseAction` and the `caseauto` machine. `run` with `*` starts one job of type `gaos_run_all` that runs every live case with a spec, one `case_run` after the other, through the same path, against staging.

### `GET /api/v1/gaos/jobs/{id}`

`200 {job_id, status, phase, job_type, error, report:{passed,failed,skipped}}`, `404` for an unknown id.

* `status`: `queued`, `running`, `succeeded`, `failed`. The job page's `completed` is `succeeded` here. A run whose tests fail is still `succeeded`; the failures are in `report`.
* `phase`: the last `job_events` step; for a run-all, `run <case> (n/total)`.
* `job_type`: `create` or `run` (`case_create` and `case_run` and `gaos_run_all` are internal names).
* `error`: empty until the job fails; short, redacted by the job's own path.

## 3. Decisions

1. **The two paths are not added to `auth.IsAuthAllowList`.** That list is exact-match documentation and is never called, and a job id cannot be listed. The real gate is `IsGAOSInboundPath`, which sends the path to `serveGAOSInbound` before `requireAuth`.
2. **Reuse `enqueueCaseAction`.** The shim translates the body and returns; the guards (spec, repo, active job, executor, AI) are the dashboard's, so the two doors cannot diverge.
3. **A duplicate is a 200, not a 409.** GAOS polls and retries; answering with the active job's id makes a retry harmless.
4. **Run-all is a sequential job built on single runs.** Its state is a `job_runs` row of type `gaos_run_all` (no new table). A case that cannot be started counts as `skipped`; a failed case job counts as `failed`; the job itself fails only when nothing can be started (`503`) or after 3 hours. A restart marks a run-all `failed` (`FailOrphanedJobs`); it is not resumed.
5. **GAOS gets no approval gate.** A `create` job still waits for a human to approve the spec (dashboard or GAOS Inbox) before the pull request. GAOS polls every 15 s for up to 40 minutes, then fails its card; the QA job keeps waiting and can still finish. Raising the GAOS limit or approving sooner is the remedy, not a change here.

## 4. Target files

`ai-backend/internal/api/gaos_automation.go` (+ test), `gaos_inbound.go` (routing), `job_events.go` (shared `jobPhase`, `jobReportCounts`), `ai-backend/internal/api/README.md`, `docs/GAOS_COLLABORATION.md`.
