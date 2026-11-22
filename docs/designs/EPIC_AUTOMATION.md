# EPIC_AUTOMATION: automate an epic's planned cases, two at a time, with a human approving each spec

Status: **v3, implemented** (2026-11-15) after two EGM review rounds (readiness, design, codebase details); the product owner delegated the choices ("dont ask me") and has not read it. Not deployed. Builds on [CASE_AUTOMATION.md](CASE_AUTOMATION.md) and the GAOS Inbox ([GAOS_INBOUND.md](GAOS_INBOUND.md)). It adds no new way to write code, call the LLM, run Playwright or open a PR: it only decides **which case is next**, using the same `case_create` job.

## 1. Problem

GAOS tracks SUT as 47 epic cards ("Automate: AUTH", …), Blocked by its design gate (BR-46) with its own agent runner off. Each epic is a group of cases in `kb_test_cases`; 1,206 of 1,700 cases have no spec and 646 of those are plain *planned* (`reason_category = ''`). An operator can press **Create automation** on one case at a time; there is no way to say "work through AUTH".

## 2. Goal and non-goals

Goal: **Automate epic**: pick the epic's eligible cases and drive them through the existing `case_create` machine, **two in flight at a time**, until each has a spec approved by a human and a PR, or a recorded failure.

The human gate is unchanged: `case_create` runs `… run → repair → approval → push → pr` (`caseauto/machine.go`). The Inbox item and the 72 h wait are for the **generated spec**; the branch and PR exist only after approval. So "in flight" means generating, running, awaiting approval, or publishing.

Non-goals: moving, creating or closing a GAOS card (BR-54: QA data never moves a task; the operator moves the card); writing missing test *cases* (every epic already has cases); retrying declined cases; a second concurrent batch for a product; any new LLM, executor or git path; any GAOS change.

## 3. Decisions

1. **The batch is a snapshot list; item state is derived, not stored.** `epic_batches.cases` (JSON array of case codes, in order) is fixed at start. Each job the driver enqueues carries `batch_id` and `batch_seq` in its `payload` **in the same `job_runs` write that creates it**, so there is no second write to lose and no reconcile table to drift. A case's state in a batch comes from the `case_create` job with that `batch_id` (3.4).
2. **Eligible case**: active (`removed_at IS NULL`), `status = 'planned'`, `reason_category = ''`, `JSON_LENGTH(spec_paths) = 0`, `epic_code = ?` and the epic is an active `kb_epics` row of the product; no `gaos_decisions` row with `decision <> 'planned'` (that table is advisory; `kb_test_cases` is authoritative; a missing row is eligible); and **none** of: an active `case_run`/`case_create` job (`activeCaseJob`'s expression); a `case_create` job with `result.pr_url <> ''` (a PR exists; the next knowledge sync after merge flips the case); a `case_create` job that **failed with a human outcome** (`error LIKE 'rejected%'` or `'approval expired%'`: the operator said no or did not answer; use the per-case button to try again); a `case_create` job that failed with a push/PR step event (a branch may be pushed: needs a person); two or more technical failures. One grouped query over `job_runs` per tick (the payload expressions are unindexed; never per case).
3. **Order**: `ORDER BY FIELD(priority,'P0','P1','P2','P3') = 0, FIELD(priority,'P0','P1','P2','P3'), code` (`utf8mb4_bin`); `''` priority sorts last. `max_cases` (default 10, 1..25) are taken from the front. The same order and query serve `GET`, `POST` and the enqueue recheck.
4. **Derived case state** from its batch job: no job → `pending`; `queued`/`running` → `active` (with `awaiting_approval` when a `case_approvals` row for the job is `open`); `completed` with `result.pr_url <> ''` → `pr`; `failed` with error `rejected…` or `approval expired…` → `declined`; any other `failed` → `failed`; `completed` without a PR url → `failed`; a case that now has a spec and no job → `pr` is not assumed, it is `skipped` (someone automated it by hand).
5. **Driver** (`epicauto`): one goroutine, started after `ResumeCaseApprovals` and `ResumeOrphanedJobs`, stopped by `serverCtx`; one tick every 15 s, **ticks never overlap**; one batch at a time in `created_at` order. Per `running` batch, in this fixed order: (a) load states (one query); (b) **stop** the batch (`stopped`, note "3 consecutive failures") if the **3 most recent finished jobs, ordered by `completed_at`**, are all `failed` (declined jobs do not count and do not reset the streak; a `pr` resets it); (c) **complete** it when no case is `pending` or `active`; (d) otherwise enqueue `pending` cases in order until `active == 2` (constant `MaxOpen`, not configurable), each through `enqueueCaseAction`.
6. **One enqueue path.** `handleCaseAction` (`case_automation.go:138`) is refactored: the guards and the enqueue move into `enqueueCaseAction(ctx, d, pid, code, action, triggeredBy string, extra map[string]interface{}) caseEnqueue`, returning a struct `{JobID, ActiveJobID string; Err *caseErr}` with `caseErr{Status int; Msg string; RetryAfter bool}` (typed, no HTTP writer). It still contains: draining 503, unconfigured 503, case 404, repo 409, `hasSpec` 409, and `activeCaseJob` 409 with `ActiveJobID`, all under `caseAuto.mu`, reusing `loadCase`, `loadAutomationRepo`, `activeCaseJob`, `repopool.AutoBranch` and the one payload builder. `extra` is merged into the payload before `enqueueJob`. The HTTP handler keeps the role check and writes the JSON. The driver passes action `create-automation` and `triggeredBy = batch.started_by`. **Result mapping in the driver**: success → next; 409 active → leave the case (it will show as `active` next tick); 409 has-spec → `skipped` in the next load; 503/draining/unconfigured → stop the tick, change nothing; 404/other 409 → log, leave `pending`, retry next tick, at most 3 ticks per case then it is treated as `failed` (an in-memory counter, lost on restart: then it simply retries).
7. **Cancel and the race with a tick.** `POST …/cancel` sets the batch `cancelled` **while holding `caseAuto.mu`**; the driver re-reads the batch status under the same mutex inside `enqueueCaseAction` (via the `extra` batch id) and refuses to enqueue for a batch that is not `running`. No new job starts after the operator cancelled; running jobs are left alone (reject their approvals to stop them).
8. **One running batch per product**, enforced in the schema, not only in code: `running_flag TINYINT GENERATED ALWAYS AS (IF(status = 'running', 1, NULL)) STORED` with `UNIQUE KEY uq_epic_batches_running (product_id, running_flag)` (the shape `kb_syncs` uses; a flag built from `product_id` would clash with its cascading foreign key). A second `POST` gets a duplicate-key error mapped to 409 `batch_active`. Batch row creation is a single `INSERT`.
9. **Authority**: starting needs `canWrite` (= `canVerify`, from `productRole`), like per-case actions; `cancel` needs it for the batch's product. The driver acts as `started_by` and does **not** re-check the role (accepted; cancel is always available). Single backend process assumed (as `caseAuto.mu` already assumes).
10. **Drain, restart.** The enqueue path refuses while draining (decision 6) and `serverCtx` stops the driver; no DB call after shutdown. A restart simply resumes ticking from MySQL; orphaned jobs are retried by `ResumeOrphanedJobs` and stay `active`. A deploy that exhausts a job's retries makes it `failed` (technical) and can stop the batch after three such failures; that is intended.

## 4. Data (`migrations.go`, idempotent, `utf8mb4_general_ci` like the other `kb_*` tables)

```sql
CREATE TABLE IF NOT EXISTS epic_batches (
  id VARCHAR(40) PRIMARY KEY,                       -- eb-<unix>-<6 hex>
  product_id INT NOT NULL,
  epic_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  status VARCHAR(12) NOT NULL DEFAULT 'running',    -- running | completed | cancelled | stopped
  cases JSON NOT NULL,                              -- ordered case codes, fixed at start
  started_by VARCHAR(64) NOT NULL DEFAULT '',       -- user id
  note VARCHAR(300) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  running_flag TINYINT GENERATED ALWAYS AS (IF(status = 'running', 1, NULL)) STORED,
  UNIQUE KEY uq_epic_batches_running (product_id, running_flag),
  INDEX idx_epic_batches_product (product_id, epic_code, created_at),
  CONSTRAINT fk_epic_batches_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

No item table, no change to an existing table. Rollback: the table is inert.

## 5. Routes

Auth as the rest of `/api/products/**`. Error body everywhere: `{"error": "<text>", "code": "<code>"}`; codes: `unconfigured`, `draining`, `bad_epic`, `unknown_epic`, `no_repo`, `nothing_eligible`, `batch_active`, `bad_range`, `bad_transition`, `not_found`, `forbidden`.

- `GET /api/products/{id}/knowledge/epics/{epic}/automation` (any role that can read the product) → `{"repo": bool, "eligible": n, "sample": ["TC-…" ≤ 10, in order], "batch": null | {id, status, note, started_by, created_at (RFC3339), max_open: 2, cases: [{code, seq, state: pending|active|awaiting_approval|pr|declined|failed|skipped, job_id: string|"" , detail}]}}`; `batch` is the product's **newest batch of that epic**; no repo → `{"repo": false, "eligible": 0, "sample": [], "batch": null}`.
- `POST …/epics/{epic}/automate` body `{"max_cases": 10}` (empty body ok; non-integer, null or out of 1..25 → 400 `bad_range`; unknown fields rejected) → 202 `{"batch_id", "planned": [codes]}`; 409 `batch_active` (message names the epic of the running batch), `no_repo`, `nothing_eligible`; 400 `bad_epic`/`unknown_epic` (`epic` must be an active `kb_epics` row of the product); 403; 503. The first enqueue happens on the next tick (≤ 15 s).
- `POST /api/epic-batches/{id}/cancel` → 200 the batch object as above; 409 `bad_transition` unless `running`; 404; 403. Dispatched by a new prefix in `dispatchAuthenticated` that loads the batch's `product_id` and calls `productRole`.

The epic routes are a new `case` in `knowledge.go:handleKnowledge` **before** the generic `entityPaths` branch (`epics` is an entity path), receiving `canWrite`.

## 6. Dashboard

`TestCasesTab.tsx` (already has `epic`, `canWrite`, `hasRepo`): when `epic !== ''`, `hasRepo` and `canWrite`, an **Automate epic** panel (state matrix below), `api.ts` gains `epicAutomation`, `automateEpic`, `cancelBatch` beside `createAutomation`, polling with the existing hooks every 5 s while the batch is `running`, stopping on unmount.

| State | Shown | Controls |
|---|---|---|
| loading / error | spinner / the error text with Retry | Retry |
| no batch, eligible 0 | "Nothing to automate in this epic" | none |
| no batch, eligible n | "n planned cases without automation" | number field (default 10, 1..25), **Start** |
| running | status chip, a row per case in order with its state and a job link (`/jobs/:id`) when there is one | **Cancel** (confirm) |
| completed / cancelled / stopped | the same list, the note ("3 consecutive failures"), counts | **Start** again if eligible > 0 |
| start returns 409 | the server's message inline | none |

## 7. GAOS

No change. The operator sees each generated spec as a **Needs you** item (existing, CASE_AUTOMATION decision 5), a PR after approval, and automated counts and results on the card's badge after merge and sync. The card is not moved. BR-46: the GAOS design gate guards GAOS's own agents, not the QA tools; the QA tools' per-case flow already works on these cases, and this feature only chooses the next one.

## 8. Safety and failure

Bounded spend: ≤ 25 cases a batch, 2 in flight, one batch a product, stop after 3 consecutive technical failures, a case is not retried after a human said no or after 2 technical failures, and never while a PR or active job exists. Every side effect stays in `case_create`. The driver holds no secrets and calls no LLM. Logs: one line per transition (`epic batch <id> <epic>: enqueued <code> job <id>`, `… stopped: …`, `… completed`) with the batch id, never content. Batch start and cancel are logged with the user id. Closed-unmerged PRs keep a case ineligible until the operator uses the per-case button (documented, not automated).

## 9. Target files

`ai-backend/internal/db/migrations.go` (the table), `internal/services/epicauto/` (new: `epicauto.go` pure decisions: `Classify`, `Plan`; `README.md`; `epicauto_test.go`), `internal/api/{case_automation.go (extract `enqueueCaseAction`, `caseEnqueue`, `caseErr`), epic_batches.go (new: eligibility query, batch load, handlers, driver glue), knowledge.go (route), router.go (dispatch prefix), epic_batches_test.go (new)}`, `cmd/server/main.go` (start the driver after the Resume calls), `dashboard/src/views/knowledge/{TestCasesTab.tsx, api.ts, EpicAutomation.tsx (new)}`, READMEs of `ai-backend/internal/api` and `internal/services/epicauto`, `this-qa-automation-system-playwright` API spec, `docs/designs/SITEMAP.md`, `.dev/docs` → `docs/designs/EPIC_AUTOMATION.md`.

## 10. Verification

`go test -race -mod=mod ./internal/services/epicauto/ ./internal/api/`:
- `epicauto` (pure): `Classify` for every job status/error combination; `Plan`: fills to 2 in order, stops at 3 consecutive technical failures (declined ones neither count nor reset; a `pr` resets), completes when nothing pending/active, enqueues nothing when stopped or cancelled.
- `api` (fake executor, integration DSN `QA_TEST_MYSQL_DSN` for the SQL; skipped without it): eligibility query exclusions (spec, reason, removed, PR exists, active job, declined, push/PR failure, 2 failures, non-planned decision) and the order; two concurrent `automate` POSTs (one 202, one 409 `batch_active`); a tick racing `cancel` (no enqueue after cancel); a tick racing `handleCaseAction` on the same case (one job); `enqueueCaseAction` result mapping; a crash after enqueue is harmless (the job carries `batch_id`, so the case is `active`).
- Dashboard: the panel's state matrix with mocked calls.
- E2E on staging after deploy (needs a deploy approval): batch of 2 on a small epic; both reach awaiting approval; reject one, approve one.

## 11. Round-2 decisions (these supersede earlier wording)

1. **Status writes are conditional.** Every write of a batch status is `UPDATE epic_batches SET status=?, note=? WHERE id=? AND status='running'` and the transition is acted on (logged) only when one row changed. So a tick that loaded states before an operator cancelled can never turn `cancelled` into `stopped`/`completed`. `cancel` takes `caseAuto.mu` around its update; `enqueueCaseAction` reads the batch status inside the mutex (field `BatchID string` on its input struct, SQL `SELECT status FROM epic_batches WHERE id = ?`) and refuses with `Status 409, Code batch_not_running` when it is not `running`.
2. **A case is re-checked before every enqueue.** The driver runs the single-case form of the eligibility query before each enqueue. A case that has **no batch job and is not eligible** (removed, a spec now exists, an active job started by hand, a PR exists, a human declined earlier) has the derived state **`skipped`**, so `pending` always means "eligible and waiting" and a batch can always finish. There is no in-memory retry counter (replaces decision 6's counter): an unexpected error leaves the case `pending` and is logged; the next tick retries.
3. **The 2-in-flight cap counts the batch's own jobs.** A job started by hand is not counted; its case becomes `skipped` as above.
4. **Human outcomes are `declined`:** job `error` starting `rejected`, `approval expired`, or `changes were requested too many times`, and the orphan text containing `already passed the approval step`. `stopped while waiting for approval` (the 76 h cap or a shutdown) is technical. A job that failed after a `push` or `pr` event (a branch may be pushed) is **`needs_person`**: it is never re-enqueued and it **stops the batch at once** with note "needs a person: <code>".
5. **Jobs per case.** If several jobs carry the batch id and case code, the newest by `created_at` (then `id`) counts. "Three most recent finished jobs" is scoped to the batch, ordered by `COALESCE(completed_at, created_at) DESC, id DESC`. Exclusions apply **before** `LIMIT max_cases` when the snapshot is taken.
6. **Eligibility SQL (shape).** One statement `SELECT t.code, t.priority FROM kb_test_cases t WHERE t.product_id=? AND t.epic_code=? AND t.removed_at IS NULL AND t.status='planned' AND t.reason_category='' AND JSON_LENGTH(t.spec_paths)=0 AND NOT EXISTS (SELECT 1 FROM gaos_decisions g WHERE g.product_id=t.product_id AND g.case_code=t.code AND g.decision<>'planned') AND t.code NOT IN (<codes of jobs excluded below>) ORDER BY …`. The excluded codes come from **one grouped query over `job_runs`** (`type IN ('case_run','case_create')`, `CAST(JSON_EXTRACT(payload,'$.product_id') AS UNSIGNED)=?`, `JSON_UNQUOTE(JSON_EXTRACT(payload,'$.case_code'))` as the key): any `queued`/`running`; any `completed` with `JSON_UNQUOTE(JSON_EXTRACT(result,'$.pr_url'))<>''`; any `failed` `case_create` with a human-outcome error; any `failed` `case_create` with an `EXISTS` row in `job_events` of step `push` or `pr`; `COUNT(*) >= 2` technical `failed` `case_create`. Revisit with an index if `job_runs` grows past ~100k rows.
7. **Enqueue must not lose the row.** A new `enqueueJobTx` variant returns the error of the `job_runs` insert and starts no goroutine when it failed; `enqueueCaseAction` uses it, the existing `enqueueJob` keeps its behaviour for other callers.
8. **`enqueueCaseAction`**: input struct `{PID int; Code, Action, TriggeredBy, BatchID string}`; it keeps the guards before the lock and moves them inside only the batch-status read; the `run` action stays in the handler (the driver only creates automation). `caseErr` gains `Code` (`unconfigured`, `draining`, `no_repo`, `not_found`, `has_spec`, `case_active`, `batch_not_running`, `internal`); the handler maps them to the existing responses unchanged. A new `writeCodedError(w, status, code, msg)` writes `{error, code}`; the existing `writeJSONError` is untouched.
9. **Body decoding** uses `map[string]json.RawMessage`: empty body = defaults, unknown key → 400 `bad_range`, a non-integer or `null` `max_cases` → 400 `bad_range`. `epic` is checked against `^[A-Za-z0-9_-]{1,64}$` (`bad_epic`) then against active `kb_epics` (`unknown_epic`).
10. **Wiring and seams.** `Router.StartEpicDriver(ctx, interval)` (called from `cmd/server/main.go` after `ResumeOrphanedJobs`); `Router.epicTick(ctx)` is the unit a test calls directly; ticks are serialised by a mutex-guarded `running` flag. Duplicate key on insert maps to 409 `batch_active` and a follow-up `SELECT` names the running batch's epic. Cancel's dispatch prefix is added in `dispatchAuthenticated` before `/api/products/`; `productRole(req)` is not product-scoped, so the handler checks `canWrite` and loads the batch's product itself. Integration tests use `testdb.New(t)`; tests that need an active job insert `job_runs` rows directly (no fake AI/executor needed for them). Dashboard polling copies the `window.setInterval` pattern of `CaseAutomation.tsx`; 409 messages render from `ApiError.message`.
11. **Role.** The driver does not re-check roles per tick (accepted; the operator can cancel at any time).
