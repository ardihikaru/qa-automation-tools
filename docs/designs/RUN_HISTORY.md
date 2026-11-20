# RUN_HISTORY: how long automation building and Playwright runs took, every run logged

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Rev 2 (2026-11-21), after one EGM review round (implementation readiness, design, codebase details; findings resolved in section 11, which supersedes earlier wording). The product owner asked for it and delegated the choices; implementation follows in the same PR. Not deployed.
*   **Depends on:** [CASE_AUTOMATION.md](CASE_AUTOMATION.md) (`case_create`, `case_run`, `job_events`, `case_approvals`), [CASE_RESULTS.md](CASE_RESULTS.md) (the report parser and `kb_case_results`), [EPIC_AUTOMATION.md](EPIC_AUTOMATION.md) (`batch_id` in a job's payload), [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (`kb_syncs`).

## 1. Problem

The product owner's request: "we need history of how long the automation building worked, and how long the e2e Playwright test was when it was running. Every run should be logged. Currently that UI was hidden; re-implement those hidden pages with this new workflow."

What exists today:

*   **The hidden pages.** Commit `32a80fb` removed four dashboard views: `QAInsights.tsx` (`/qa`: tiles and a 12-row jobs audit with durations), `DocsView.tsx` (`/docs`), `IssueDebugger.tsx` (`/issues`), `FixTracking.tsx` (`/fixes`). `QAInsights` ran on **mock data** (`USE_MOCK_QA_INSIGHTS = true`, `utils/mockDataGenerator.ts`). Its jobs were `qa_run` / `docs_generate`, which no longer exist.
*   **Automation building.** A `case_create` job (`caseauto/machine.go`: `prepare → generate → write → run → repair → approval → push → pr → done`) is one `job_runs` row (`created_at`, `started_at`, `completed_at`, `duration_ms`) plus its `job_events` (`at DATETIME(3)`, `step`), plus one `case_approvals` row per approval round (`created_at`, `answered_at`). Nothing prunes these tables, so every build since the feature shipped is already on record. No page shows them as a list, and `duration_ms` mixes machine time with up to 72 h of waiting for a human.
*   **Playwright runs.** A report reaches the backend three ways: CI / dashboard upload (`ingestReport`, sources `ci` and `upload`), a `case_run` job (`finishCaseRun`, source `case-run`), and each executor run inside a `case_create` job (`executorRunner.RunCase`). The first two keep only the **latest result per case** in `kb_case_results` (no duration, no run row); the third keeps nothing but counts in the job payload. `finishCaseRun` deletes the raw report from the job result. So "every run is logged" is false for Playwright runs today.

## 2. Goal and non-goals

Goal: one dashboard page, **Run history** (`/history`), on real data only, with three tabs:

1.  **Builds**: every `case_create` and `case_run` job with queue time, active time, **time waiting for a human approval shown apart from machine time**, and a per-step breakdown; link to the job page (`/jobs/:id`) and the case page.
2.  **Playwright runs**: every Playwright report the backend receives, from any of the three doors, with its source, run ref, start, duration, pass/fail/flaky/skipped counts, and on demand the per-case durations.
3.  **Knowledge syncs**: every `kb_syncs` row of every product with its duration (data exists; today only a per-product tab shows it).

Filters: product, case, epic, type/source, status/outcome, date range; paging.

Non-goals: changing what is sent to GAOS (`kb_case_results` and its upsert semantics stay exactly as they are; the D-077 contract with GAOS is untouched); storing test titles, error messages or attachments of a run; retention or pruning; charts; backfilling Playwright runs that happened before the deploy (their raw reports were not kept, see 3.9); re-implementing `/docs`, `/issues`, `/fixes` (3.10); any change to the executor or to GAOS.

## 3. Decisions

1.  **Builds are derived, not stored again.** `job_runs`, `job_events` and `case_approvals` already hold every build and every step time. A new table would be a second copy that can drift. The builds API computes durations at read time for the requested page only (≤ 100 jobs).
2.  **Playwright runs get two new append-only tables**, `pw_runs` (one row per report) and `pw_run_cases` (one row per case code in that report), written at the three doors (3.5). `kb_case_results` is not read, changed or replaced by this feature.
3.  **The parser grows, its results do not change.** `caseresults.Parse` additionally returns the report's `stats` (`duration`, `expected`, `unexpected`, `flaky`, `skipped`) and per case code: `DurationMs` = sum of `results[].duration` over every test and retry of that code, `Tests` = number of tests, `Attempts` = number of `results[]` entries. `Report.Results`, `StartedAt`, `Tests` and the skip reasons keep their exact current values; `parse_test.go` keeps passing unchanged.
4.  **Timing semantics of a build** (all milliseconds, all from UTC timestamps):
    *   `queue_ms` = `started_at − created_at` (null while queued).
    *   `active_ms` = `(completed_at, else now) − started_at`.
    *   `waiting_ms` = Σ over the job's `case_approvals` rows of `(answered_at, else completed_at, else now) − created_at`, each clipped at 0. This is the human's time, read from the approval record (the authority on when a question was asked and answered), not from events.
    *   `machine_ms` = `max(0, active_ms − waiting_ms)`.
    *   `steps` = ordered `[{step, ms}]` from `job_events`: each event's time runs until the next event of the job, and the last event's until `completed_at` (else now). Steps are folded (3.4a). The `approval` step's time is the waiting time plus the few seconds spent opening the approval; the UI shows `waiting_ms` as its own figure and colours the `approval` bar as waiting.
    *   A job resumed after a restart keeps its first `started_at`; the downtime is inside the step that was open. This is accepted and documented in the UI tooltip ("includes any backend restart").

    4a. **Step folding.** For a `case_create` job the machine steps are `prepare, generate, write, run, repair, approval, push, pr, done`; the executor's own steps that the machine forwards (`validate`, `install`, `report`, any other non-machine step, and a `done` event that is not the job's last event) count as `run`. For a `case_run` job the executor's steps are kept as they are, in the order `validate, install, run, report, done`; unknown steps go last in first-seen order. The fold is a pure function (`runhistory.Breakdown`) with table tests.
5.  **Where a Playwright run is recorded.** One function, `runhistory.Record(ctx, d, In, *caseresults.Report)`:
    *   `ingestReport` (sources `ci`, `upload`): after `caseresults.Save`, with the same `run_ref`.
    *   `finishCaseRun` (source `case-run`): with `job_id` and `case_code` of the job, `run_ref` = job id, before the raw report is dropped from the result. Skipped cases are recorded (it is a log); `kb_case_results` still ignores them as today.
    *   `executorRunner.RunCase` (source `case-create`): every executor run of a build, first run and repairs, with `job_id`, `case_code`, `run_index` = the run's `Repair` number (0 = first run), `run_ref` = job id. Nothing else of the build changes.
    *   An executor run that produced no parseable report is **not** a Playwright run row (there is nothing to time); the build's `run` step still shows its time and the job page its events.
6.  **Recording never breaks the caller.** A `Record` error is logged (`run history: <source> product <id>: <err>`) and the caller carries on: an upload still answers 200 with its `stored`/`stale` counts, a job still finishes. The CI answer gains one field, `run_id` (0 when not recorded); existing fields are unchanged.
7.  **Idempotent per report.** `UNIQUE (product_id, source, run_ref, started_at)`. Re-sending the same CI report (same `run_ref`, same `stats.startTime`) records nothing new and `Record` returns the existing id. Two executor runs of one build differ by `started_at`. A report without a usable `stats.startTime` gets "now" from `Parse` and is therefore always a new row.
8.  **Reads are open to every signed-in user**, like `GET /api/jobs` and `GET /api/products` today: the routes are read-only and show nothing a signed-in user cannot already open on the job and case pages. They sit behind the cookie middleware (`dispatchAuthenticated`); no allow-list change.
9.  **No backfill of Playwright runs.** Earlier raw reports were not kept (`finishCaseRun` drops them; `kb_case_results` is latest-only), so a backfill could only invent rows. The Playwright tab says "Recorded since <first row's date>". Builds and syncs need no backfill: their history is complete.
10. **The other hidden pages stay removed.** `/docs`, `/issues` and `/fixes` read `/api/docs*`, `/api/issues*` and `/api/fixes*`, which were deleted from the backend in `2fa3629` ("remove legacy docs/issues/qa-run API surface"). `raw_issues` and `issue_suggestions` still exist but nothing has written to them since the exploratory `qa_run` was removed; there is no real data to show, and a page over an empty, writer-less table would be a mock in disguise. QAInsights' tiles (flows, open issues, skills, cost) are dropped for the same reason; cost has its own page (`/cost`). What QAInsights really was, a jobs audit with durations, is what this page rebuilds on real data. `/qa` redirects to `/history` so old bookmarks land on its successor.

## 4. Data (`migrations.go`, appended, idempotent, unguarded `CREATE TABLE IF NOT EXISTS`)

```sql
CREATE TABLE IF NOT EXISTS pw_runs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  source VARCHAR(16) NOT NULL,                 -- ci | upload | case-run | case-create
  run_ref VARCHAR(120) NOT NULL DEFAULT '',
  job_id VARCHAR(64) NOT NULL DEFAULT '',      -- case-run / case-create
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '', -- the job's case
  run_index INT NULL,                          -- case-create: the executor-run counter (0 = first run)
  started_at DATETIME(3) NOT NULL,             -- stats.startTime (UTC)
  duration_ms BIGINT NULL,                     -- stats.duration; NULL when the report has none
  tests INT NOT NULL DEFAULT 0,
  expected INT NOT NULL DEFAULT 0,             -- passed
  unexpected INT NOT NULL DEFAULT 0,           -- failed
  flaky INT NOT NULL DEFAULT 0,                -- passed on retry
  skipped INT NOT NULL DEFAULT 0,
  cases INT NOT NULL DEFAULT 0,                -- distinct TC codes
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_pw_runs_report (product_id, source, run_ref, started_at),
  INDEX idx_pw_runs_started (started_at),
  INDEX idx_pw_runs_product (product_id, started_at),
  INDEX idx_pw_runs_job (job_id),
  CONSTRAINT fk_pw_runs_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS pw_run_cases (
  run_id BIGINT NOT NULL,
  product_id INT NOT NULL,
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  result ENUM('pass','fail','skipped') NOT NULL,
  duration_ms BIGINT NOT NULL DEFAULT 0,
  tests INT NOT NULL DEFAULT 0,
  attempts INT NOT NULL DEFAULT 0,
  PRIMARY KEY (run_id, case_code),
  INDEX idx_pw_run_cases_case (product_id, case_code, run_id),
  CONSTRAINT fk_pw_run_cases_run FOREIGN KEY (run_id) REFERENCES pw_runs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

The counts come from `stats` when present, else from the per-test statuses. `Record` writes the run row and its case rows in one transaction (`INSERT … ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`; when the run already existed, the case rows are not written again). All codes of the report are stored, also ones that are not active cases (the log records what ran); the detail marks `known: false` for them. Size: a full CI run of ~1,700 cases is ~1,700 short rows; no retention is needed at today's rate. Rollback: the two tables are inert to the old binary.

## 5. Routes

All `GET`, cookie auth (3.8), JSON, errors `{"error": "<text>"}`. Shared query parameters: `page` (≥ 1, default 1), `page_size` (default 25, clamped to 1..100 by `parsePagination`), `product` (product id), `from` / `to` (`YYYY-MM-DD`, UTC, inclusive). A malformed `product`, `from`, `to`, `case`, `epic` or enum value → 400. `case` and `epic` match `^[A-Za-z0-9._~-]{1,64}$`. Lists are newest first (`created_at DESC, id DESC` for builds, `started_at DESC, id DESC` otherwise).

*   `GET /api/history/builds?type=case_create|case_run&status=queued|running|completed|failed&case=&epic=&batch=` → `{"items": [Build], "page", "page_size", "total", "summary": {"count", "completed", "failed", "running", "avg_active_ms", "avg_waiting_ms", "avg_machine_ms"}}`. Averages are over finished jobs of the filter (null when none). `type` defaults to both. `epic` matches the case's `kb_test_cases.epic_code`; `batch` the payload's `batch_id`.
    *   `Build` = `{id, type, status, error, product_id, product_name, case_code, epic_code, batch_id, pr_url, outcome, created_at, started_at, completed_at, queue_ms, active_ms, waiting_ms, machine_ms, approvals, runs, steps: [{step, ms}]}`. `outcome` is the `case_run` verdict (`pass|fail|skipped|none`) or `""`; `approvals` = number of approval rounds; `runs` = number of `pw_runs` rows of the job. Times are RFC3339 UTC; absent ones are `null`.
*   `GET /api/history/runs?source=ci|upload|case-run|case-create&outcome=pass|fail|skipped&case=&epic=&job=` → `{"items": [Run], "page", "page_size", "total", "since": "<RFC3339 of the oldest row>"|null, "summary": {"count", "pass", "fail", "skipped", "avg_duration_ms"}}`. `outcome` of a run: `fail` when `unexpected > 0`, else `pass` when `expected + flaky > 0`, else `skipped`. `case` and `epic` keep runs that hold at least one matching `pw_run_cases` row (`epic` through `kb_test_cases.epic_code`).
    *   `Run` = `{id, product_id, product_name, source, run_ref, job_id, case_code, run_index, started_at, duration_ms, tests, passed, failed, flaky, skipped, cases, outcome}` and, when `case` is given, `case_result` and `case_duration_ms` of that case in the run.
*   `GET /api/history/runs/{id}` → `Run` plus `"case_rows": [{case_code, epic_code, known, result, duration_ms, tests, attempts}]` ordered by `duration_ms DESC, case_code`; 404 when absent.
*   `GET /api/history/syncs?status=running|succeeded|failed|skipped` → `{"items": [{id, product_id, product_name, status, trigger, repo_name, branch, commit_sha, started_at, finished_at, duration_ms, error}], "page", "page_size", "total"}`.

Dispatch: a `/api/history/` prefix in `dispatchAuthenticated`, handled by `internal/api/history.go`. Method other than GET → 405.

## 6. Dashboard

Nav entry **Run history** (`/history`, lucide `History` icon) after Products; `/qa` redirects there. One view, `views/history/HistoryView.tsx`, with three tabs; tab and filters live in the URL query (`?tab=builds&product=3&epic=AUTH&page=2`), so a link is shareable and Back works. Data through `products/api.ts`'s `api()` (same origin, credentials). Page root fills the content area (no `max-w-*`). No mock data anywhere.

*   **Filters bar** (all tabs): product select (from `GET /api/products`), date from/to, and per tab: Builds: type, status, case, epic; Runs: source, outcome, case, epic; Syncs: status.
*   **Builds tab**: summary line (count, completed, failed, avg machine time, avg waiting time). Table: job (link `/jobs/:id`), case (link to the case page, `entityHref(pid, 'test_case', code)`), epic, type, status chip, created, queue, **machine**, **waiting for approval**, total, and a stacked step bar (machine steps in blues/greens, `approval` amber) with a per-step list on hover/focus (`title` + an expandable row for keyboard and touch users). A running job shows live totals ("so far").
*   **Playwright runs tab**: summary (runs, pass, fail, skipped, avg duration), "Recorded since …". Table: started, product, source, run ref / job link, case (for job runs), duration, passed/failed/flaky/skipped, cases, outcome chip. A row expands to the per-case table (`/api/history/runs/{id}`): case (link), result, duration, tests, attempts; sortable by duration.
*   **Knowledge syncs tab**: started, product, status, trigger, commit (short), duration, error.
*   States: loading skeleton, error with Retry, empty ("No builds match these filters"), paging (Prev/Next with "page x of y").

## 7. Security and privacy

Read-only routes, no new writer outside the three existing doors, no new secret, no new allow-list path. The run tables hold codes, counts and durations only: no test titles, error text, stack traces or attachments, so nothing patient-related or secret from a report reaches them. Build rows expose `job_runs.error`, which is already redacted when written and already shown on the job page. Query parameters are validated and bound (`?`), never interpolated.

## 8. Target files

`ai-backend/internal/db/migrations.go` (two tables); `internal/services/caseresults/{parse.go, parse_test.go, README.md}` (stats and per-case durations); `internal/services/runhistory/` (new: `record.go` `Record`, `breakdown.go` `Breakdown`, `README.md`, `breakdown_test.go`, `record_integration_test.go`); `internal/api/{history.go (new), history_test.go (new), case_results.go, case_automation.go, router.go, README.md}`; `dashboard/src/{App.tsx, components/Layout.tsx, views/history/ (new: HistoryView.tsx, BuildsTab.tsx, RunsTab.tsx, SyncsTab.tsx, api.ts, format.ts)}`, `dashboard/README.md`; `this-qa-automation-system-playwright/tests/{api/history.spec.ts (new), dashboard/run-history.spec.ts (new), dashboard/navigation.spec.ts, dashboard/layout-width.spec.ts}`; `docs/designs/{RUN_HISTORY.md, SITEMAP.md}`; root `README.md` (feature list).

## 9. Verification

*   `go test -mod=mod ./internal/services/caseresults/ ./internal/services/runhistory/`: parser stats and per-case durations (retries summed, several tests per code, no stats → counts from tests, `duration` absent → NULL), existing parser tests unchanged; `Breakdown` table tests (case_create fold, executor `done` mid-job, last event to end, running job to now, case_run order, no events).
*   With `QA_TEST_MYSQL_DSN` (scratch mysql:8.0): `Record` writes run + cases, re-recording the same report returns the same id and adds no rows, a different `started_at` adds a run; `ingestReport` answers `run_id` and still leaves `kb_case_results` exactly as before (existing `TestCaseResultsDB` unchanged and passing); builds API: queue/active/waiting/machine for a job with two approval rounds, an open approval on a running job, filters (product, case, epic, batch, type, status, dates), paging and `total`, 400 on bad parameters, 405 on POST; runs API: filters, `outcome`, `case_result`, detail ordering and 404; syncs API: duration and filters.
*   Dashboard: `npx tsc --noEmit`, `npm run lint`, `npm run build`.
*   E2E (`this-qa-automation-system-playwright`, against a deployment, needs credentials): API shape of the three lists; the page loads, tabs switch, the URL keeps the tab; `/qa` redirects to `/history`; layout width.
*   After deploy (separate approval): run the migration, upload a report from the Products page, see it in Playwright runs; open a recent build and check that waiting time matches the approval's ask/answer times.

## 10. Alternatives weighed

*   **Store `stats.duration` in `job_runs.result` and keep a table only for CI/upload reports.** Rejected: two shapes for one question ("how long did this Playwright run take?"), and the case-create door runs up to five reports per job, which one `result` field cannot hold. One table, one writer function (`Record`) and three call sites is smaller to read and to query.
*   **A `build_runs` table written when a job ends.** Rejected (3.1): it copies `job_runs`/`job_events`/`case_approvals`, misses every build before the deploy, and adds a second write that can fail.
*   **Ship `pw_runs` without per-case rows and no syncs tab** (the design review's "smallest change"). Rejected: the owner's brief asks for per-case durations of a run and names the knowledge sync as part of today's workflow. The syncs tab is a read over an existing table (no write, no migration); `pw_run_cases` is the only place a case's run time can come from.

## 11. Review round 1 resolutions (supersede earlier wording)

1.  **Collations.** `job_runs` is `utf8mb4_general_ci`; `job_events` and `case_approvals` take the server default (`utf8mb4_0900_ai_ci`). The builds code never compares them on `job_id` without `COLLATE utf8mb4_general_ci` on both sides (the `epic_batches.go` pattern); the page's events and approvals are read with `WHERE job_id IN (?, …)` (bound strings, no join). The new tables are `utf8mb4_general_ci`, so `pw_runs.job_id = job_runs.id` needs no `COLLATE`. The integration tests run every query on the migrated schema, which is the collation test.
2.  **Filters and summaries are SQL over the whole filter, not the page.** Builds: `j.type IN ('case_create','case_run')` (`idx_job_runs_type`), `CAST(JSON_EXTRACT(j.payload,'$.product_id') AS UNSIGNED) = ?`, `JSON_UNQUOTE(JSON_EXTRACT(j.payload,'$.case_code')) = ?`, `JSON_UNQUOTE(JSON_EXTRACT(j.payload,'$.batch_id')) = ?`, epic via `EXISTS (SELECT 1 FROM kb_test_cases t WHERE t.product_id = <payload product> AND t.code = <payload case> AND t.epic_code = ?)`, `from`/`to` on `j.created_at` (`>= from`, `< to + 1 day`). The summary is one aggregate over the same `WHERE`, the waiting time a correlated subquery on `case_approvals` (`COLLATE` on both sides). This scans the case jobs (hundreds today); as in EPIC_AUTOMATION 11.6, add generated indexed columns if `job_runs` passes ~100k rows. Runs and syncs filter on indexed columns; their `from`/`to` use `started_at`. `from > to` → 400. Unknown parameters are ignored.
3.  **Step times are summed in SQL, for the page's jobs only.** `SELECT job_id, step, last, SUM(ms), MAX(at) FROM (SELECT job_id, step, at, LEAD(at) OVER w IS NULL AS last, COALESCE(TIMESTAMPDIFF(MICROSECOND, at, LEAD(at) OVER w), 0) DIV 1000 AS ms FROM job_events WHERE job_id IN (…) WINDOW w AS (PARTITION BY job_id ORDER BY seq)) x GROUP BY job_id, step, last`: at most jobs × steps × 2 rows. Go adds, for the `last` row, `end − at` (end = `completed_at`, else now), clipped at 0. `runhistory.Breakdown(jobType, []StepSum{Step, Last, Ms}) []StepMs` is the pure fold: per step **summed** (repeated `run`/`repair` periods add up), canonical order (`case_create`: `prepare, generate, write, run, repair, approval, push, pr, done`; `case_run`: `validate, install, run, report, done`), unknown steps after them alphabetically, zero steps omitted. For `case_create`, `validate`, `install`, `report` and any step outside the machine list fold into `run`, and a `done` with `last = false` folds into `run`. Example: `case_create` sums `{prepare 2s, generate 40s, write 0.1s, run 30s, validate 1s, done(not last) 0.2s, repair 35s, run 28s, approval 3h, push 4s, pr 2s, done(last) 0}` → `[prepare 2s, generate 40s, write 0.1s, run 59.2s, repair 35s, approval 3h, push 4s, pr 2s]`. For `case_run`, the backend's own `run` events ("submitted to playwright-executor") precede the executor's `validate`, so its `run` includes the executor queue. The page does not poll; a **Refresh** button reloads.
4.  **Precision and windows.** `job_runs` and `case_approvals` times are whole seconds, events milliseconds: every difference is clipped at 0 and figures are accurate to about a second. Each approval interval is clipped to the job's window `[started_at, completed_at or now]`. A restart while an approval is open counts as waiting (the approval stayed open); a restart in another step counts in that step. `started_at` survives a resume because `markJobRunning` sets it only when nil and the resumed record is loaded from the row. A `case_run` job's `started_at` is set when the executor reports `running` (`runExecutorJob`), so its executor queue is in `queue_ms`. A job still `running` shows its figures "so far".
5.  **`Record`.** `Record(ctx, d *db.DB, in RunIn, rep *caseresults.Report) (int64, error)`, `RunIn{ProductID int; Source, RunRef, JobID, CaseCode string; RunIndex *int}`; it never mutates `rep`. In one `d.WithTx`: `INSERT INTO pw_runs (…) VALUES (…) ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`; with this DSN (no `clientFoundRows`) `RowsAffected` is 1 for a new row and 0 for an existing one, the id is `LastInsertId()` either way, and case rows are inserted only for a new run. A deadlock (MySQL 1213) is retried once. `started_at` is written as UTC `2006-01-02 15:04:05.000`; `stats.duration` (float ms) is rounded with `math.Round`; missing or negative → NULL. Counts come from `stats.expected/unexpected/flaky/skipped` when present, else from the tests' statuses.
6.  **Call sites, exactly.** `ingestReport`: after `caseresults.Save` succeeds, `Record(…, RunIn{pid, source, runRef, "", "", nil}, rep)`; both `ci` and `upload` answers gain `run_id` (the existing id for a re-sent report, 0 when recording failed). `finishCaseRun`: right after the `v.Report == nil` return and **before** the "code not in report" branch (so a report without the case's test is still logged), `Record(…, RunIn{pid, "case-run", jobID, jobID, code, nil}, v.Report)` with `v.Report` (skipped codes intact), under the existing `pid > 0` and DB guards. `executorRunner.RunCase`: after `VerdictFromResult`, when `v.Report != nil`, `Record(…, RunIn{e.pid, "case-create", e.jobID, e.jobID, req.CaseCode, &req.Repair}, v.Report)`. The column is `run_index` (replaces `attempt`): the machine's executor-run counter (`r.runs`), which keeps counting through change-request rounds and restarts at 0 after a resume.
7.  **Known gaps, stated in the page's help text.** An executor run that failed (`StatusFailed`), timed out or returned no parseable report produces no Playwright run row; its time is in the build's `run` step and on the job page. The same report delivered through both `ci` and `upload` is two rows (two deliveries; not deduped). A report whose start time is missing or in the future gets "now" and is always a new row; executor reports always carry `stats.startTime`. `caseresults` finds codes with `TC-[A-Z]+-[0-9]{3}`, so per-case rows have that shape.
8.  **Response details.** Lists are `{items, total, page, page_size}` (the knowledge `Page<T>` shape). `since` (runs) = `MIN(created_at)` over the rows matching the `product` filter (null when none). `known` (run detail) = the code is an active case of the run's product (`kb_test_cases.removed_at IS NULL`). History reads the database only, not the in-memory job map. Validation reuses `knowledge.CodeRe` for `case`/`epic`, `strconv.Atoi` for `product` (400 unless a positive integer) and `parsePagination(page_size, page, 25)`.
9.  **Dashboard reuse.** Chips: `JOB_CHIP_CLASS`/`jobChipStatus` (`knowledge/CaseAutomation.tsx`) and `SyncStatusChip`; states and paging from `knowledge/ui.tsx` (`Pagination`, `ErrorNote`, `Empty`, `SkeletonRows`, `RelTime`, `ShaLink`); `shortSha`, `absoluteTime`, `errorText`, `isAbort` (`knowledge/api.ts`). The millisecond formatter is extracted from `knowledge/api.ts:duration()` and shared. Nav: `History as HistoryIcon`, between Products and Configuration; `/qa` → `<Navigate to="/history" replace />`. `CaseResultsSummary` gains `run_id`.
10. **Verification additions.** Integration: `finishCaseRun` with a skipped-only report records the run with its skipped case and leaves `kb_case_results` unchanged; a report without the job's case is recorded and the job still fails; `executorRunner.RunCase` against a fake executor (`httptest`, the `jobs_resume_test.go` pattern) records one `case-create` row with `run_index`, and a `StatusFailed` run records none; the upload answer keeps every existing key and adds `run_id`; the builds query runs on the migrated schema. The E2E API spec asserts `items, total, page, page_size` and, per build, `queue_ms, active_ms, waiting_ms, machine_ms, steps`. DB tests need a scratch server: `docker run -d --name qa-scratch -e MYSQL_ROOT_PASSWORD=pw -p 127.0.0.1:33099:3306 mysql:8.0`, then `QA_TEST_MYSQL_DSN='root:pw@tcp(127.0.0.1:33099)/scratch?parseTime=true' go test -mod=mod ./internal/...`; a skipped test is not a pass. After deploy, compare one build: `SELECT SUM(TIMESTAMPDIFF(SECOND, created_at, COALESCE(answered_at, UTC_TIMESTAMP()))) FROM case_approvals WHERE job_id = ?` with its `waiting_ms / 1000`.
