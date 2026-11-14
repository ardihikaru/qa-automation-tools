# CASE_AUTOMATION: run and create automation for a test case

Status: approved by the product owner in chat on 2026-11-14 ("I approve it all"); implementation follows this contract. It **supersedes** the rejection of "running the automation repo's suite from playwright-executor" in [CASE_RESULTS.md](CASE_RESULTS.md) (see Decision 1).

## 1. Goal

On `/products/{id}/knowledge?tab=test-cases` (and the case page):

* a case **with** an automation spec gets **Run**: the spec runs on rnd-v3 against staging, and the verdict lands in `kb_case_results` and is pushed to GAOS (existing path);
* a case **without** one gets **Create automation**: claude-v3 (via ai-manager, only) writes a spec, the spec is run and repaired up to 3 times, then a human approves (dashboard, or the GAOS Inbox) and a PR is opened on the automation repo;
* every step of both is visible live in the dashboard.

## 2. Decisions

1. **Reverse the CASE_RESULTS rejection**, with these answers to its objections:
   * *repo dependencies*: `npm ci` runs once per lockfile hash in the product's automation clone in the repo pool (cached `node_modules`);
   * *secrets and targets (API, MQ)*: the automation repo's env file lives on rnd-v3 at `~/.config/qa-automation/automation-<productId>.env` (mode 600, never printed) and is loaded only into the child process. Only the Playwright projects the case needs are run (`--project` from the spec path: `tests/web/**` → `web-*`, `tests/api/rest/**` → `api-rest`; `mq` is refused);
   * *2.5 GiB budget*: unchanged. One run at a time (existing queue), the same unit caps; `--workers=1`;
   * *staging only*: the executor refuses a run unless the env file's base URLs are on the staging allow-list (`EXECUTOR_ALLOWED_HOSTS`).
2. **All LLM calls stay in the backend** (`ai.Service`, claude-v3). The executor never talks to an LLM and never pushes. Orchestration of *create* is a backend state machine (`internal/services/caseauto`) that uses: repopool (git), `ai.Service` (generation), the executor (`case_run`, test execution only) and the approval gate.
3. **Writes need the existing guard.** Committing, pushing and opening a PR go through `repoWriteAllowed(RepoOpPush|RepoOpOpenPR)` for the product's `automation` repo (PRODUCT_REPOS_AUTOMATION_ROLE.md). No push happens before approval.
4. **Progress is an event log**, not a stream: rows in `job_events`, read by polling (2 s). No SSE (nginx buffering is not changed).
5. **Approval has two doors, one record**: the dashboard (Approve / Reject / Needs changes) and, when the product is GAOS-integrated, a GAOS Inbox item. Whichever answers first wins; the other is closed.

## 3. Data (appended to `migrations.go`, idempotent)

```sql
CREATE TABLE IF NOT EXISTS job_events (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  job_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  at DATETIME(3) NOT NULL,
  level VARCHAR(8) NOT NULL,        -- info | warn | error
  step VARCHAR(32) NOT NULL,        -- prepare | install | generate | write | run | repair | approval | push | pr | done
  message VARCHAR(1000) NOT NULL,
  UNIQUE KEY uq_job_seq (job_id, seq),
  KEY idx_job (job_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS case_approvals (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  job_id VARCHAR(64) NOT NULL,
  product_id INT NOT NULL,
  case_code VARCHAR(64) NOT NULL,
  external_id VARCHAR(80) NOT NULL,   -- "qa-<job_id>-pr", the GAOS Inbox externalId
  status VARCHAR(16) NOT NULL DEFAULT 'open', -- open | approved | rejected | changes | expired
  answered_by VARCHAR(16) NULL,       -- dashboard | gaos
  answer_text VARCHAR(1000) NULL,
  gaos_created TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  answered_at DATETIME NULL,
  UNIQUE KEY uq_ext (external_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

`job_runs.payload` carries `{product_id, case_code, spec_paths, branch, pr_url}`; no column change. Log text is clipped to 1000 chars and passed through the secret screen below.

**Secret screen.** Anything written to `job_events`, sent to GAOS or to the LLM is passed through `redact()`: values of the env file's keys, `gh*_` / `gaos_` / `Bearer ` tokens, and URL userinfo are replaced by `***`.

## 4. Executor contract (`playwright-executor`)

New run types `case_run` (the only new one; *create* needs just this).

`POST /v1/runs` params (validated in `validate.ts`; any other key is rejected):

| Param | Rule |
|---|---|
| `repo_dir` | absolute; `realpath` must be inside `EXECUTOR_REPO_POOL` (default `/home/sismedika/devel/projects/qa-automation-tools/repos`), contain `playwright.config.ts` |
| `case_code` | `^TC-[A-Z0-9]+-[0-9]{3,4}$` |
| `spec_paths` | 1..5 repo-relative paths, no `..`, must exist, must end `.spec.ts`, under `tests/web/` or `tests/api/rest/` |
| `env_file` | one of the names found in `EXECUTOR_ENV_DIR`; never a path |
| `repair` | optional integer, attempt number (log only) |

Behaviour: `npm ci` if `package-lock.json` hash differs from `node_modules/.qa-lock` → run `npx playwright test <spec_paths> -g "<case_code>" --workers=1 --retries=0 --reporter=json,list` with `VENDOR_E2E_RUN=<run id>`, `TEST_ENV=staging` and the env file loaded; timeout `RUN_TIMEOUT_SECONDS`.

Result: `{exit_code, passed, failed, skipped, report: <results.json content, ≤2 MiB>, tail: <last 4 KiB of stdout>}`.

Events: `RunRecord.events: {n, at, level, step, message}[]` (ring of 500). `GET /v1/runs/{id}?since=<n>` returns the record with only events whose `n > since`. stdout is split by line, redacted, clipped to 300 chars.

## 5. Backend contract (`ai-backend`)

* `POST /api/products/{id}/knowledge/test-cases/{code}/run` → 202 `{job_id}`; 409 if the case has no `spec_paths` or a job for the case is active; 403 unless the user may verify (same as `canVerify`).
* `POST /api/products/{id}/knowledge/test-cases/{code}/create-automation` → 202 `{job_id}`; 409 if the case already has a spec, the product has no `automation` repo, or a job is active.
* `GET /api/jobs/{id}/events?since=<seq>` → `{events:[…], status, phase, approval?, pr_url?}`.
* `GET /api/products/{id}/knowledge/test-cases/{code}/jobs` → last 10 jobs for the case.
* `POST /api/jobs/{id}/approval` body `{decision: approve|reject|changes, text?}` → answers the open approval (dashboard door).
* `POST /api/v1/gaos/inbox-answer` (bearer `GAOS_INBOUND_TOKEN`, fail-closed like GAOS_INBOUND.md) body `{externalId, productId, status, choice, optionLabel, text, answeredAt}` → answers it (GAOS door); idempotent.
* New job types `case_run`, `case_create` in `executorRunTypes` / `enqueueJob`. Legacy `qa_run`/`docs_generate` are removed (Phase 0b).

**case_run job**: payload → `executor.Submit(case_run)` → `Wait` (poll 2 s, copying new events into `job_events`) → `caseresults.Parse(report)` → `caseresults.Save(productID, rep, runRef=job id, source="case-run")` → `gaos.Trigger(productID)`. A spec that did not match the case code gives `failed` with the message "no test titled <code> in <spec>".

**case_create job** (`caseauto`), states `prepare → generate → write → run → (repair ≤3) → approval → push → pr → done`; any failure → `failed` with the last event as reason; `reject`/`changes` end or loop:

1. *prepare*: `repopool.SyncBranches` for the automation repo; worktree `qa-auto--<code>` on branch `qa-auto/<code-lower>` from the default branch; symlink `node_modules` from the main clone.
2. *generate*: prompt = case (scenario, preconditions, steps, expected), its story/feature, the repo's `AGENTS.md`, and 2 existing specs from the same module (read from the worktree) as style examples. `ai.Service.ChatMessages` returns one file, `tests/<web|api/rest>/<module>/<code>.spec.ts`; the title **must** start with the case code. The reply is validated (single file, path pattern, size ≤ 20 KiB, contains the code, no `process.env` dumps, no outbound hosts other than the env file's).
3. *write*: file written inside the worktree only (path re-checked).
4. *run*: `case_run` on the worktree. On `failed`, *repair*: the failing tail (redacted) goes back to the LLM, ≤ 3 rounds total.
5. *approval*: creates `case_approvals`(open), shows the diff stat, and (if integrated) `gaossync.CreateInboxItem` with options *Approve / Reject / Needs changes*, `externalId=qa-<job>-pr`. A 30 s poller fallback asks GAOS for open items. Expires after 72 h → job `failed: approval expired`.
6. *push*: after `approve` and `repoWriteAllowed(push)`: commit (`test(<code>): automate <title>`), push the branch. *pr*: `gh pr create` (`repoWriteAllowed(open_pr)`), PR body lists the case, the run result and the job link. `pr_url` stored in the payload; `kb_test_cases` is **not** edited (the next knowledge sync, after merge, flips it to automated).

Orphans: `FailOrphanedJobs` at start also fails jobs in `approval`'s *waiting* only if the approval is not `open` (an open approval survives a restart and resumes via the poller).

## 6. GAOS Inbox contract (implemented in the GAOS repo)

`POST /v1/products/{pid}/inbox` and `GET /v1/products/{pid}/inbox/{externalId}` (QA-scope bearer), plus a webhook from GAOS to `GAOS_QA_URL + /api/v1/gaos/inbox-answer` (bearer `GAOS_QA_TOKEN`). Shapes as implemented in the GAOS repo's design doc; this tool treats any non-2xx and any unknown `status` as "still open" and keeps polling.

## 7. Dashboard

* `TestCasesTab` and `TestCasePage`: **Run** (case has `spec_paths`), **Create automation** (case has none and the product has an automation repo); status chip of the last job.
* New route `/jobs/:id` (also opened in a drawer from the case page): header (status, phase, case, branch), a timeline of `job_events` polled every 2 s until `completed|failed` (plus 30 s after), the generated spec (read from the `write` event's file), the report summary, an **Approval** card (Approve / Reject / Needs changes + text; shows "also asked in GAOS Inbox"), and the PR link.

## 8. Security and failure

* LLM output is never executed outside the executor and never gets a shell: only the validated file is written.
* Executor refuses non-staging hosts; no new executor egress.
* A GAOS outage never blocks: the dashboard door always works.
* Rollback: migrations are additive; new routes are inert if the executor lacks the run type (job `failed` with a clear message).

## 9. Target files

`ai-backend/internal/db/migrations.go`, `internal/api/{router.go,jobs_executor.go,knowledge_cases.go (new),job_events.go (new),gaos_inbox_inbound.go (new)}`, `internal/services/caseauto/` (new), `internal/services/repopool/` (worktree helpers, commit/push/PR), `internal/services/gaossync/inbox.go` (new), `internal/services/executor/client.go` (events), `playwright-executor/src/{types.ts,validate.ts,queue.ts,server.ts,runs/caseRun.ts}`, `deploy/playwright-executor/` + env docs, `dashboard/src/views/knowledge/*`, `dashboard/src/views/JobView.tsx` (new), README contracts of each touched directory, this-qa-automation-system-playwright specs.

## 10. Verification

Unit: executor param validation and redaction; `caseauto` state machine with fake LLM/executor/repo; GAOS client against `httptest`. E2E on staging after deploy: Run a spec-backed case; Create automation for one easy case and watch it to the PR.
