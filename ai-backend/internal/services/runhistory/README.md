# services/runhistory

## Overview
The run history of [RUN_HISTORY.md](../../../../docs/designs/RUN_HISTORY.md). It logs every Playwright report the backend receives in `pw_runs` / `pw_run_cases`, and folds a build's `job_events` step times into its per-step breakdown. The read API is `internal/api/history.go` (`/api/history/*`).

## Interface
- `Record(ctx, db, RunIn, *caseresults.Report) (int64, error)` (`record.go`): one transaction (`db.WithTx`, retried on deadlock). `INSERT … ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` on `UNIQUE (product_id, source, run_ref, started_at)`: `RowsAffected` 1 = a new run (its case rows are inserted, 500 per statement), 0 = already recorded (the existing id is returned, nothing else is written). `RunIn{ProductID, Source (ci|upload|case-run|case-create), RunRef, JobID, CaseCode, RunIndex *int}`. `started_at` is written as UTC with milliseconds; the report is never modified.
- `Breakdown(jobType, []StepSum) []StepMs` (`breakdown.go`): pure. For `case_create`, executor steps (`validate`, `install`, `report`, anything outside `prepare, generate, write, run, repair, approval, push, pr, done`) and a `done` that is not the job's last event fold into `run`. Steps are summed and listed in canonical order (`case_run`: `validate, install, run, report, done`), unknown steps after them alphabetically, zero steps dropped.
- `Clip(ms)`: negative → 0 (whole-second job/approval timestamps can trail millisecond events).

## Invariants
- Writes only `pw_runs` and `pw_run_cases`. Never reads or writes `kb_case_results`, so what is pushed to GAOS does not change.
- Stores codes, counts and durations only: no test titles, error text or attachments.
- Callers (`ingestReport`, `finishCaseRun`, `executorRunner.RunCase`) log a `Record` error and carry on: an upload or a job never fails because of the history.

## Side effects
Writes `pw_runs`, `pw_run_cases`.

## Testing
`go test -mod=mod ./internal/services/runhistory/` (`Breakdown`, no DB). `record_integration_test.go` needs `QA_TEST_MYSQL_DSN` (scratch MySQL 8): new run, re-recorded report (same id, no new rows), a second start, another source, `run_index`.
