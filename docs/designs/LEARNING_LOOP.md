# LEARNING_LOOP: findings from real automation work become merged policies in the automation repo

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** rev 3, implemented (2026-11-21). Rev 1 was reviewed by four Goldfish reviewers (design, implementation readiness, codebase details, zoom-out): 8 Blockers, 23 Majors, 21 Nits; rev 2 by a second readiness reviewer: 2 Blockers, 5 Majors, 5 Nits. Every finding is resolved (sections 12 and 13). Not deployed.
*   **Approval:** the product owner delegated every choice to the orchestrating agent ("work among your agents, never as my confirmation"). The EGM review is the gate; no human sign-off of this spec was asked for.
*   **Depends on:** [CASE_AUTOMATION.md](CASE_AUTOMATION.md) (the `case_create` machine, `job_runs`/`job_events`, `case_approvals` and its two doors), [PRODUCT_REPOS_AUTOMATION_ROLE.md](PRODUCT_REPOS_AUTOMATION_ROLE.md) (`repoWriteAllowed`), [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (`kb_assistants`: the synced `AGENTS.md` and `.claude/rules/*.md`), [GAOS_INBOUND.md](GAOS_INBOUND.md) (the inbox-answer webhook).
*   **Terms.** *Automation repo*: the product's `product_repos` row with `is_automation = 1` (HISv3 = product 3 = `developersismedika/automation-test-hisv3`). *Finding*: one thing worth learning, observed in real work. *Proposal*: a `policy_propose` job: an AI-drafted policy for the automation repo, built from findings, approved by a human, published as a PR. *Learned policy*: a proposal whose PR was merged.

## 1. Analysis

**Today's `/learning` is dead.** `LearningProgressView.tsx` charts `learning_cache` rows (intent / action / page / score) from `GET /api/learning?limit=500`; `/navigation` reads the same route plus `/api/learning/navigation` (`navigation_edges`). The writers were the exploratory-QA crawler of 2026-10 (commits `9efd478`, `3783fbb`, `afbc455`); the crawler and its run types were removed (PLAYWRIGHT_EXECUTOR.md) and no code calls `learning.Service.Add*` any more. Production returns `[]`.

**Where findings come from now.** The `case_create` machine (`caseauto/machine.go`): generate → run → ≤ 3 repair rounds → approval (Approve / Reject / Needs changes, dashboard or GAOS Inbox) → push `qa-auto/<code>` → PR. Every step is a `job_events` row (`the reply was refused: …`, `the spec did not pass: …`, `the spec passed`); every human answer is a `case_approvals` row with `answer_text`. Epic batches drive many of these.

**The feedback channel is half-blind.** `initialConvo` puts `AGENTS.md` in the prompt clipped to **12,000 bytes**, silently. HISv3's `AGENTS.md` is 30,480 bytes; the cut lands inside the §4 project map, so §4–§9 (coding standards, the REST rules of §5.3, the policies of §6, recipes, definition of done) never reach the model. `.claude/rules/*.md` are not read at all. A policy appended to `AGENTS.md` would be invisible, and a rules file would be too unless the machine reads it.

**Real findings of 2026-11-20** (the seed examples; none is invented):
(a) a generated spec doubled `/api/v1` because `HIS_VENDOR_BACKEND_URL` already ends in `/api/v1`; a reviewer answered *Needs changes*, the repair fixed it → harvested as `review_changes`;
(b) Playwright's WHATWG URL layer decodes `%2e%2e` and normalises dot segments, so path-traversal cases need a raw HTTP request (TC-VSYS-008 left planned) → manual;
(c) specs must use typed clients, not raw `request.get()` (golden rule 2) → a proposal must come back **covered**;
(d) leak-marker lists diverged; now one `SERVER_LEAK_MARKERS` in `src/api/core/assertions.ts` → manual;
(e) while GitHub Actions is off, results are promoted only after an operator's 3× staging run plus upload → manual, process-level.

## 2. Goals and non-goals

Goals: (1) findings captured automatically and by hand, with product, source, evidence and time, deduplicated; (2) proposals drafted by claude-v3 (via ai-manager, `ai.Service` only) from selected findings, approved by a human through the dashboard and, for GAOS-integrated products, the GAOS Inbox, then published as a PR with the existing machinery; (3) lifecycle finding → proposed → PR open → learned, or rejected / closed; (4) `/learning` shows it over time with a product filter and status filters; (5) merged policies are in the next `case_create` prompt, proven by a test.

Non-goals: automatic proposals (a human presses *Draft policy*: bounded spend, no noise); editing or deleting existing rules (a learned policy only **adds** one file); pushing to a default branch; any GAOS change; fuzzy merging of findings; the crawler UI.

## 3. Decisions

1.  **A policy lands as one new file, `.claude/rules/learned-<slug>.md`, in the automation repo.**
    *   Claude Code loads `.claude/rules/*.md` without `paths:` front matter as project memory, so an agent working in the automation repo sees it with no index edit. `case_create` reads these files (decision 8).
    *   One file per policy means two learning PRs never edit the same file (slug uniqueness, decision 9), history is append-only, and a closed PR leaves nothing behind. Rejected: a section in `AGENTS.md` (past the clip, and every PR would edit the same lines); one `learned-policies.md` (every PR conflicts).
    *   The file has no YAML front matter. Its exact shape (`learnloop.RenderFile`):
        ```
        # Learned policy: <title>

        <!-- qa-learning: product=<pid> job=<job id> findings=<ids> -->

        <policy>

        ## Why

        <rationale>

        ## Evidence

        - <finding kind>: cases <codes, ≤ 5>; QA tools job <job id>     (one line per finding, no excerpts)
        ```
        Evidence names cases and job ids only: reviewer texts and run output stay in the QA tools' database (golden rule 5 forbids PII in the repo; the redactor removes secrets, not patient data).
2.  **A proposal is a job.** `job_runs.type = 'policy_propose'`, run by `Router.runPolicyPropose` like `runCaseCreate`. This reuses `job_events` (the live log and `/jobs/:id`), `case_approvals` (one approval per round; `case_code = ''`), the dashboard approval route `POST /api/jobs/{id}/approval`, the GAOS inbox-answer webhook (the external id is `qa-<job id>-pr[N]`, which `inboxExternalRe` already accepts — the webhook is **not** changed), `FailOrphanedJobs`/`ResumeCaseApprovals`/`ResumeOrphanedJobs` (extended to the new type), and the jobs list. There is no proposal table and no event table.
    *   Payload: `{product_id, finding_ids, triggered_by, branch, spec_path, spec_content, draft: {decision, slug, title, policy, rationale, covered_by}, pr_url}`. `spec_path`/`spec_content` are the policy file's path and full rendered text, so `/jobs/:id` shows the exact file under review (the "Generated spec" card).
    *   Result: `{decision: new|covered, covered_by, file_path, pr_url}`, later `{pr_state: OPEN|MERGED|CLOSED, merge_commit, merged_at, pr_checked_at}` (merged into the JSON with `JSON_SET`).
3.  **The proposal machine** (`learnloop.Proposer`, side effects behind interfaces, unit-tested with fakes):
    `prepare` (load findings, the synced repo files) → `generate` (one LLM call, one corrective turn on an invalid answer, else fail) → `covered` ends the job **completed** with `result.decision = covered` → `approval` (shared wait, decision 6; *Needs changes* → `generate` again with the reviewer text, at most 2 change rounds, a third → failed `changes were requested too many times`) → `push` → `pr` → `done`. Steps are written as `job_events.step` (`prepare`, `generate`, `approval`, `push`, `pr`, `done`).
4.  **Derived proposal state** (`learnloop.ProposalState`, used by the API and the page):

    | Job | Extra fact | State |
    |---|---|---|
    | queued / running | open approval | `awaiting_approval` |
    | queued / running | an event of step `push` or `pr` | `publishing` |
    | queued / running | otherwise | `drafting` |
    | completed | `result.decision = covered` | `covered` |
    | completed | `result.pr_state = MERGED` | `learned` |
    | completed | `result.pr_state = CLOSED` | `closed` |
    | completed | `pr_url <> ''` | `pr_open` |
    | failed | error starts with `rejected`, `approval expired`, `changes were requested too many times` (`epicauto.HumanOutcome`) | `rejected` |
    | failed | otherwise | `failed` |
5.  **Findings and their status.** Status: `new`, `proposed`, `learned`, `covered`, `dismissed`. `proposal_job_id` holds the active or last proposal.

    | Event | Who | Finding change |
    |---|---|---|
    | proposal created (`POST /api/learning/proposals`) | user | every finding `new` → `proposed`, `proposal_job_id` set, one transaction; a finding not `new` → 409 `finding_busy`, nothing changes |
    | proposal state `covered` | reconcile (decision 10) | `proposed` → `covered` |
    | proposal state `learned` | PR poll | `proposed` → `learned` |
    | proposal state `rejected`, `failed`, `closed` | reconcile | `proposed` → `new` (`proposal_job_id` kept as history) |
    | dismiss | user | `new` → `dismissed` |
    | reopen | user | `dismissed` or `covered` → `new` |

    The reconcile runs every driver tick with conditional updates (`… WHERE f.status = 'proposed' AND f.proposal_job_id = j.id AND <job state>`), so every path that ends a job (the runner, an orphan failure at startup, a cancel) releases its findings without hooks.
6.  **One approval wait, shared.** The approval loop of `caseauto.Machine.approve` (open the record, optional GAOS Inbox item, poll the record every 2 s and GAOS every 30 s, 72 h expiry, first answer wins via `ApprovalStore.Answer … WHERE status = 'open'`, `DecisionFromAnswer` for GAOS answers) is extracted into the exported `caseauto.AwaitApproval(ctx, AwaitDeps, AwaitRequest)`; `Machine.approve` calls it with its own texts, the proposer with its own. The GAOS item for a proposal: title `Approve learned policy for <product>`, question naming the title, `context` = the policy and rationale (both ≤ 1,200 bytes, so GAOS shows the whole policy text), options Approve / Reject / Needs changes. There is no in-place edit: a reviewer who wants different text answers *Needs changes*, and the new round's text gets a new approval (new external id), so nobody approves text they did not see.
7.  **The draft** (`learnloop.Prompt`, `ParseDraft`). Input, all redacted and clipped: the findings' kind, title, lesson text and ≤ 3 evidence excerpts each, inside `~~~` fences marked as data; the synced `AGENTS.md` (from `kb_assistants`, kind `instructions`, path `AGENTS.md`) as its golden-rules section plus all headings; the synced `.claude/rules/*.md` paths and titles, with the bodies of learned ones (≤ 6 KB); on a change round, the reviewer text and the previous draft. Output: one JSON object, `{"decision":"new","slug","title","policy","rationale"}` or `{"decision":"covered","covered_by","rationale"}`, unknown fields refused. Validation: slug `^[a-z0-9]+(?:-[a-z0-9]+){0,7}$` ≤ 45 chars; title one line ≤ 120; policy 40–1,200 bytes; rationale ≤ 1,000; `covered_by` 1–200. The policy must not contain (**injection screen**): a level-1 heading, a line that starts with `---` or `@`, `<!--` or `-->`, a line that starts with `<` (block HTML). The repo files come from the last knowledge sync, not from git, so drafting needs no repo lock.
8.  **`case_create` reads learned policies.**
    *   `RepoOps` gains `ReadLearned(ws) ([]LearnedFile, error)`: the regular files `.claude/rules/learned-*.md` of the worktree (no symlinks, ≤ 64 KB each, ≤ 50 files), sorted by name. `poolRepo` implements it with its own confined reader, because `confined`/`safeRelRe` refuse a leading dot.
    *   `initialConvo` adds after `REPO GUIDE`: `LEARNED POLICIES (merged rules of this repository; follow them like the repo guide):`, then per file `// <path>` and its policy part only (the text between the provenance comment and `## Why`; for a file not in that shape, the whole body without front matter), each ≤ 1,500 bytes, 16,000 bytes in total. A file that does not fit is left out with a `warn` event naming it.
    *   The `AGENTS.md` clip stays at 12,000 bytes (raising it changes every generation and was not measured), but a clip now writes a `warn` event: `AGENTS.md is N bytes; only the first 12000 reach the model`. Raising it is an open item for the product owner.
    *   The `run` `warn` event `the spec did not pass: …` gets the first error line of the output tail (`Error:` / `expect(` / `TypeError` / `ReferenceError` / `SyntaxError` / `Timeout` line, ≤ 300 bytes, redacted) on a second line, so findings carry the real failure.
    *   The worktree is created from `origin/<default branch>` after a fetch (`poolRepo.Prepare`), so a merged policy is in the prompt of every `case_create` job prepared after the merge.
9.  **Publishing**, inside the proposal job after approval:
    *   Guard: `repoWriteAllowed(RepoOpPush)` and `(RepoOpOpenPR)` (through `poolRepo.Push` / `OpenPR`).
    *   `FetchOrigin`; `AddWorktree(rootDir, "qa-auto/learn-<job id lower>", default)`; `LinkNodeModules` is not needed.
    *   Slug: the drafted slug, else `-2` … `-9`, taken when the file exists in the worktree **or** another `policy_propose` job whose state is `publishing` / `pr_open` has that `file_path`; none free → fail.
    *   Write only that file (`poolRepo.WriteLearned`, which refuses any path but `learnloop.IsLearnedPath`, `O_NOFOLLOW`, confined), normalised for prettier's Markdown defaults (LF, no trailing spaces, one blank line between blocks, `-` bullets, final newline).
    *   `Commit` (`docs(rules): learned policy - <title>`, author `qa-automation`); `repopool.validRepoRel` accepts exactly `^\.claude/rules/learned-[a-z0-9-]+\.md$` in addition to its current rule.
    *   `Push`; `OpenPR` with the automation repo's Bahasa template (AGENTS.md §6.7, headings verbatim: `# Formulir Pull Request`, `## Sumber task` with `Kode`, `## Deskripsi Pull Request`, `## Apa saja yang sudah dites oleh developer?`, `## Apa saja yang perlu dites oleh QA?`, `## Penambahan kebutuhan struktur / resource data`, `## Potensi breaking?`, `## File yang Diubah`, `## Rencana Pengujian`, footer); then a best-effort `gh pr edit --add-label ai-assisted` (`repopool.(*Pool).LabelPR`, same guard; a missing label is a `warn` event).
    *   A restart during publishing is the existing orphan rule: no push event yet → resume at publish (approved) or fail; a push event → fail ("the branch may be pushed; check the automation repo, then start a new proposal"). A new proposal gets a new job id, so a new branch: a half-pushed branch is never reused.
10. **The driver** (`Router.StartLearningLoop(serverCtx, 30 s)`, after `StartEpicDriver`; ticks never overlap; each stage has its own timeout):
    (a) **harvest** when ≥ 10 min since the last (first tick at start);
    (b) **reconcile** finding statuses (decision 5);
    (c) **PR poll**: `policy_propose` jobs `completed` with a `pr_url` and no final `pr_state`, `pr_checked_at` older than 10 min (or never), at most 5 per tick: `repopool.(*Pool).PRStatus(ctx, rootDir, url)` runs `gh pr view <url> --json state,mergedAt,mergeCommit` in the product's automation clone (read-only, 30 s timeout). `MERGED` → result gets `pr_state`, `merge_commit`, `merged_at`, and the findings become `learned`; `CLOSED` → `pr_state = CLOSED`; else only `pr_checked_at`. `POST /api/learning/proposals/{id}/check` runs it at once. The knowledge sync is a cross-check, not a detector: the page shows "in the synced repo" when `kb_assistants` has a live row with that path.
    Drafting and publishing run in the job goroutine, never in the tick.
11. **Harvest** (`learnloop.Harvest`, window: the last 30 days, re-scanned every run; the tables are small, so no cursor). Product: `case_approvals.product_id`, or `CAST(JSON_EXTRACT(j.payload,'$.product_id') AS UNSIGNED)` for jobs.

    | Kind | Rows | `source_key` |
    |---|---|---|
    | `review_changes` | `case_approvals` `status = 'changes'`, `answer_text <> ''`, `answered_at` in window, joined to its `job_runs` row of type `case_create` | `approval:<external_id>` |
    | `review_rejected` | same with `status = 'rejected'` (a rejection without text is skipped) | `approval:<external_id>` |
    | `repair_fixed` | `case_create` jobs created in window with a `run`/`warn` event starting `the spec did not pass:` and a **higher-`seq`** event `the spec passed` | `job:<id>:repair` |
    | `repeated_failure` | messages of jobs created in window: `case_create` events `generate`/`repair` `warn` `the reply was refused: …` and `run` `warn` `the spec did not pass: …`; `job_runs.error` of `failed` `case_create` and `case_run` jobs that is not a human outcome (`epicauto.HumanOutcome`, plus `stopped while waiting for approval`). A signature (`learnloop.Signature`) seen in ≥ 3 distinct jobs of a product becomes one finding with one evidence row per job | `job:<id>:sig:<first 12 hex of Fingerprint(signature)>` |

    `Signature`: strip the generic prefix; if the first line is only `<code> failed|passed|was skipped`, the rest is the signature; a remainder shorter than 12 normalised characters, or one of the verdict texts `no test titled …`, `the executor returned no report`, `the report could not be read`, is not a signature.

    Lesson text and title: `review_*` → the reviewer text; `repair_fixed` → the failure lines of the failing runs (≤ 3, joined); `repeated_failure` → the signature. Title = `<Kind label>: <first line of the lesson, ≤ 120>` (`Reviewer asked for changes`, `Reviewer rejected a spec`, `A repair fixed a failing spec`, `Repeated failure`).

    **Idempotency and dedup**, per candidate: if a `learning_evidence` row with that `source_key` exists, skip. Else in one transaction: find the product's open finding (`new` or `proposed`) with the same `Fingerprint(lesson)`; if none, insert a finding; insert the evidence (a duplicate key rolls the transaction back); bump `occurrences` and `last_seen_at` when joined. Title and detail never change on a join. A re-run creates nothing (tested). `Normalize`: lower case; URLs → `<url>`; case codes → `<code>`; quoted strings → `<str>`; hex ≥ 7 and numbers → `<n>`; other punctuation → space; collapsed; ≤ 400 chars. Exact fingerprints only: fuzzy similarity merged different Playwright failures that share boilerplate.
12. **Manual findings**: `POST /api/learning/findings {product_id, title, detail, case_code?, job_id?, run_ref?}` by a user with `canVerify`; same fingerprint dedup (the answer says `merged`); `source_key = manual:<random 16 hex>`; `created_by` = user id.
13. **`/api/learning` and `/api/learning/navigation` stay as they are**, with their tables: `/navigation` (redesigned in parallel work) still reads them. Removing them belongs to that work. The new routes are new paths.

## 4. Data (appended to `migrations.go`, idempotent)

```sql
CREATE TABLE IF NOT EXISTS learning_findings (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  kind VARCHAR(24) NOT NULL,                  -- review_changes | review_rejected | repair_fixed | repeated_failure | manual
  status VARCHAR(16) NOT NULL DEFAULT 'new',  -- new | proposed | learned | covered | dismissed
  title VARCHAR(200) NOT NULL,
  detail TEXT NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  occurrences INT NOT NULL DEFAULT 1,
  proposal_job_id VARCHAR(191) NULL,
  created_by VARCHAR(64) NOT NULL DEFAULT 'system',
  status_note VARCHAR(300) NOT NULL DEFAULT '',
  first_seen_at DATETIME NOT NULL,
  last_seen_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_lf_product (product_id, status, last_seen_at),
  INDEX idx_lf_fp (product_id, fingerprint),
  INDEX idx_lf_job (proposal_job_id),
  CONSTRAINT fk_lf_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS learning_evidence (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  finding_id BIGINT NOT NULL,
  source_key VARCHAR(160) NOT NULL,
  job_id VARCHAR(191) NOT NULL DEFAULT '',
  case_code VARCHAR(64) NOT NULL DEFAULT '',
  run_ref VARCHAR(120) NOT NULL DEFAULT '',
  excerpt VARCHAR(1000) NOT NULL DEFAULT '',
  observed_at DATETIME NOT NULL,
  UNIQUE KEY uq_le_source (source_key),
  INDEX idx_le_finding (finding_id, observed_at),
  CONSTRAINT fk_le_finding FOREIGN KEY (finding_id) REFERENCES learning_findings(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

Rollback: the tables are inert for the old binary; `policy_propose` jobs left running are failed by the old binary's `FailOrphanedJobs` (it fails every type it does not know).

## 5. API (auth as every `/api/**`; errors `{"error","code"}`)

| Method, path | Who | Answer |
|---|---|---|
| `GET /api/learning/findings?product_id=&status=&kind=&q=&limit=&offset=` | signed in | `{items, total, counts:{new,proposed,learned,covered,dismissed}}`; newest `last_seen_at` first; `limit` 1–200 (default 50); `q` matches title/detail; 400 `bad_input` on a bad value |
| `GET /api/learning/findings/{id}` | signed in | the finding plus `evidence` (≤ 50, newest first; `job_id`, `case_code`, `run_ref`, `excerpt`, `observed_at`); 404 `not_found` |
| `POST /api/learning/findings` | canVerify | 201 `{finding, merged}`; 400 `bad_input` (unknown product, title 1–200, detail 1–2,000, codes by `caseauto.CaseCodeRe`); 403 `forbidden` |
| `POST /api/learning/findings/{id}/dismiss` `{reason?}` / `…/reopen` | canVerify | 200 the finding; 409 `bad_transition` |
| `GET /api/learning/proposals?product_id=&state=&limit=&offset=` | signed in | `{items, total, counts}`; an item is `{id (job id), product_id, product_name, state, created_at, completed_at, triggered_by, finding_ids, title, slug, file_path, policy, rationale, covered_by, pr_url, pr_state, merge_commit, merged_at, in_repo, error}` |
| `POST /api/learning/proposals` `{product_id, finding_ids}` | canVerify | 202 `{job_id}`; 400 `bad_input` (1–10 distinct ids of that product); 409 `finding_busy` (a finding is not `new`); 503 `unconfigured` (no AI, no repo pool, or no automation repo) |
| `POST /api/learning/proposals/{job id}/check` | canVerify | 200 the proposal after a PR poll; 409 `bad_transition` unless state `pr_open`; 503 `unconfigured` (no repo pool) |
| `GET /api/learning/timeline?product_id=&limit=` | signed in | `{items}` newest first, ≤ 200: derived from findings (`finding_new` at `first_seen_at`, `finding_dismissed`/`finding_learned` at `updated_at`) and proposals (`proposal_created` at `created_at`, `proposal_<state>` at `completed_at` or `merged_at`) |
| `POST /api/jobs/{id}/approval` (existing) | canVerify | answers a proposal's approval, unchanged |

`product_id` is optional on reads (omitted = all products). Dispatch: a prefix `/api/learning/` in `dispatchAuthenticated` for `findings`, `proposals`, `timeline`; `/api/learning` and `/api/learning/navigation` keep their `case`s.

## 6. Dashboard

`/learning` (route and nav entry kept; the nav label becomes "Learning") replaces `LearningProgressView.tsx`'s body; new files live in `dashboard/src/views/learning/`.

*   Header "Learning" with one line: what the QA tools learned from real automation work, and the policies merged into the automation repo.
*   **Filters** in the URL query: **Product** (required control: "All products" or one product from `/api/products`; default: the first product with an automation repo), finding **Status**, **Kind**, text search; policy **State**.
*   **Tiles**: new findings, awaiting approval, PRs open, learned policies.
*   **Tabs**: *Timeline* (by day; each row links to the finding, `/jobs/:id`, the case page `/products/:pid/knowledge/test-cases/:code` and the PR); *Findings* (expand for evidence with links; select 1–10 new findings of one product → **Draft policy**; **Add finding** dialog; Dismiss / Reopen); *Policies* (state chip, title, file path, the full policy text, rationale or `covered_by`, findings; *Awaiting approval*: Approve / Needs changes (text) / Reject through the existing approval route; *PR open*: Check now; *Learned*: PR link and merge commit; every row links to `/jobs/:id`).
*   Loading, empty ("Nothing learned yet for <product>"), and error-with-Retry states; root fills the content area; at phone width the filters wrap and rows stack.
*   Polls every 10 s while a visible proposal is `drafting` or `publishing`.

## 7. Safety

*   **Writes**: only after a human approved the exact file text shown on `/jobs/:id` (and in GAOS), only the one learned file, only on `qa-auto/learn-*`, only through `repoWriteAllowed`, never forced, never to a default branch.
*   **Injection**: reviewer texts, run output and repo files are fenced data in the prompt; the policy is validated by the screen of decision 7 before it is stored, shown or written.
*   **PII**: the committed file holds no excerpts (decision 1).
*   **Spend**: one LLM call per draft round (+ one corrective turn), only on a human's request, ≤ 3 rounds.
*   **Prompt growth** in `case_create`: ≤ 16 KB of learned policies, with a `warn` per file left out.
*   **Roles**: reads for every signed-in user (reviewer texts are already visible on `/jobs`); writes need `canVerify`, like case approvals.

## 8. Target files

`ai-backend/internal/db/migrations.go`; `internal/services/learnloop/` (new: `normalize.go`, `draft.go`, `harvest.go`, `proposer.go`, `state.go`, `README.md`, tests); `internal/services/caseauto/{approval.go (new: AwaitApproval), machine.go, caseauto_test.go, README.md}`; `internal/services/repopool/{worktree.go (validRepoRel exception, PRStatus, LabelPR), worktree_test.go, README.md}`; `internal/api/{learning_loop.go, learning_loop_test.go (new), case_automation.go (poolRepo.ReadLearned / WriteLearned), jobs_executor.go, jobs_resume.go (policy_propose orphans), router.go (prefix), README.md}`; `cmd/server/main.go`; `dashboard/src/views/LearningProgressView.tsx`, `dashboard/src/views/learning/*` (new), `dashboard/src/components/Layout.tsx` (label); `this-qa-automation-system-playwright/tests/api/learning-loop.spec.ts`, `tests/dashboard/learning.spec.ts`; `ai-backend/internal/services/learning/README.md`; `docs/designs/SITEMAP.md`.

## 9. Verification

*   `learnloop`: Normalize/Fingerprint on the seed texts and on negative pairs (two failures sharing `expect(received)` boilerplate stay apart); Signature; draft parse/validate (new, covered, bad slug, H1, `@`/`---`/`<!--` lines, too long); RenderFile; FreeSlug; ProposalState for every row of decision 4; Proposer with fakes: covered, approve → PR, changes → redraft, too many changes, invalid answer → corrected, invalid twice → fail.
*   `caseauto`: a learned file in the fake repo is in the first prompt with Why/Evidence stripped; the budget leaves a file out with a warn; the AGENTS clip warn; the failure excerpt in the warn event; the existing approval tests pass on `AwaitApproval`.
*   `repopool`: `PRStatus` and `LabelPR` with a fake `gh` on `PATH`; `validRepoRel` accepts the learned path and still refuses other dot paths.
*   `api` integration (`QA_TEST_MYSQL_DSN`): harvest from inserted rows (twice: nothing new; dedup joins); manual finding; reconcile; a full proposal against a temporary bare origin with a fake `gh` and a fake LLM: approve through `/api/jobs/{id}/approval` → PR → poll MERGED → findings `learned`; the GAOS webhook answers a proposal approval; `finding_busy`; roles; filters.
*   Dashboard: `tsc --noEmit`, lint, build; `tests/dashboard/learning.spec.ts` (read-only against a deployed dashboard, skipped without credentials).
*   After deploy (separate approval): harvest shows the 2026-11-20 *Needs changes* (a); enter (b), (d), (e) by hand; a draft from (c) comes back `covered`.

## 10. Accepted limitations and open items

*   The `AGENTS.md` 12 KB clip stays (now visible as a `warn`); raising it, or selecting sections by heading, needs a before/after pass-rate check — product-owner decision.
*   The automation repo's `CLAUDE.md` says "the user commits and opens the PR" and branches are `{type}/{description}`; `qa-auto/*` PRs by the tool (already done by `case_create`) are the precedent. The repo owners should confirm `.claude/rules/learned-*` under its §6.3 and may add a `rule-index.md` row.
*   `npm run check` runs `prettier --check .`; the file is normalised to prettier's Markdown defaults but not run through prettier (no Node in the backend image).
*   A finding that recurs after its policy merged is a new finding even when the job's worktree predates the merge.
*   `learn-*` worktrees are left in the pool like `case_create`'s; the next `SyncBranches` removes clean ones.

## 11. Assumptions Goldfish reviewers must challenge

1.  Claude Code loads `.claude/rules/*.md` without front matter at session start.
2.  The 30-day re-scan is cheap at today's volume (hundreds of approvals and jobs a month).
3.  The `qa-<job id>-pr[N]` external id of a proposal is safe in the existing webhook (`ByExternal` → job → product check).

## 12. Review resolution (rev 1 → rev 2)

*   **Zoom-out M1/M2/M4** (a second workflow beside `case_create`): a proposal is now a `policy_propose` job (decision 2); `learning_proposals`, `learning_proposal_findings`, `learning_events` are gone (5 tables → 2); the webhook is unchanged; the in-page Edit/Retry are gone (retry = a new proposal). **Zoom-out M3**: `repeated_failure` kept (the product owner asked for it) but bounded and reusing `epicauto.HumanOutcome`. **Nits**: fingerprint-only dedup; one status vocabulary (decision 4/5).
*   **Design B1** (approve unseen text): no in-place edit; GAOS gets the whole policy (≤ 1,200 bytes); `/jobs/:id` shows the exact file. **B2** (injection): screen in decision 7. **B3** (PII): Evidence lists codes and job ids only. **M1** (orphan PR): the orphan rule of decision 9 and a new branch per proposal. **M2** (slug collision): decision 9. **M3** (stuck findings): decision 5 table plus reconcile. **M4** (budget): 1,200-byte policies, policy part only, 16 KB, warn per file left out. **M5** (repo checks): verbatim headings, `ai-assisted` label, prettier-shaped text, open item. **M6**: evidence-first idempotency (decision 11). **M7**: exact fingerprints only. **M8**: the clip is not raised; a warn instead. **M9**: drafting/publishing run in the job goroutine; the tick only harvests, reconciles and polls. **Nits** N1–N3 in section 10; N4: the PR state is stored; N5: `Answer` is already conditional on `status = 'open'`.
*   **Readiness B1/B2** (finding and proposal transitions): decisions 4 and 5. **B3** (harvest SQL): decision 11 names the tables, columns, ordering (`seq`), product extraction and failed status; `case_run` contributes only `job_runs.error`. **M1** titles: decision 11. **M2**: evidence-first. **M3** error codes: section 5. **M4** GAOS mapping: `DecisionFromAnswer` and the existing webhook. **M5** expiry: 72 h for every product, as for cases. **M6** slugs: decision 9. **Nits**: manual key is random; joins never change title/detail; counts keys are the statuses/states; the signature hash is `Fingerprint`; the file template is in decision 1; `answered_by` is not copied.
*   **Codebase B1/B2** (leading-dot paths): `validRepoRel` gets one exact exception; `poolRepo.ReadLearned`/`WriteLearned` have their own confined helpers. **M3**: `epicauto.HumanOutcome`. **M4**: the verdict texts that are not signatures are listed. **M5**: the clip warn. **M6**: headings verbatim, label added. **Nits**: the clip lands in §4 (section 1 fixed); Normalize order matches the code; repo conventions in section 10; nav label "Learning".

## 13. Round-2 resolution (rev 2 → rev 3; these supersede earlier wording)

1.  **Orphans of the new type** (Blocker 1). `FailOrphanedJobs` never touches `policy_propose` (`AND type <> 'policy_propose'`); `Router.ResumeProposals` (called after `ResumeCaseApprovals`, needs only the DB and the AI) owns them: a payload `pr_url` → completed; a `push`/`pr` event → failed ("the branch may be pushed"); latest approval `open` → the wait resumes on **that** external id with its round; `approved` → publish; anything else (still drafting, already answered with changes/reject/expiry, unreadable draft, loop not configured) → failed, and its open approvals are set `expired` by `system` so no door offers a dead answer. `ResumeCaseApprovals`/`ResumeOrphanedJobs` are unchanged (they only select case types).
2.  **A resume never regenerates** (Blocker 2). `learnloop.Resume` carries the stored `payload.draft`, the latest external id and its round (`-prN` → round N-1); a resumed job never calls the LLM before the answer.
3.  **`AwaitApproval` signature** (Major 3): `caseauto.AwaitApproval(ctx, AwaitDeps{Approvals, Inbox, Poll, GaosPoll, TTL, Now, Sleep, Event}, AwaitRequest{JobID, ProductID, CaseCode, ExternalID, GaosProductID, Item, WaitingMsg, LogPrefix})`; `ApprovalFailure` is the step failure; `DiffStat` stays in `Machine.approve`.
4.  **GAOS context** (Major 4): product, file path, the **whole** policy (≤ 1,200), then the rationale clipped to the room left under 1,500, then the job link. Tested.
5.  **Reconcile covers `learned`** (Major 5): `Reconcile` maps every ended proposal state (covered, learned, rejected, failed, closed) and runs every tick, after every job end and after `ResumeProposals`.
6.  **Slug collisions** (Major 6): `onPushing` writes `payload.spec_path` before the push; `takenPaths` reads `payload.spec_path` and `result.file_path` of the product's other proposals that are running or have an open PR; slug choice and file write run under `learnState.publishMu`.
7.  **Open approvals on failed jobs** (Major 7): resolved by 1.
8.  Nits: decision 4 uses the three human-decision prefixes directly (an orphan "already passed the approval step" is a technical failure for a proposal); a completed job with no matching fact is `failed`; `learnloop.MaxDuration` = 73 h; `/jobs/:id` shows the Markdown file in its "Generated spec" card as plain text (no rendering, by design: the approver reads the raw file); a failed proposal whose push may have happened returns its findings to `new` with `status_note` naming the ended proposal.
9.  **After the rebase onto RUN_HISTORY / AI_CONVERSATIONS** (merged to `main` while this was built): the proposer's LLM calls carry `ai.ConvRef{Kind: job, Ref: <job id>}` and message kinds (`prompt`, `reply`, `refusal`), so a proposal's drafting appears on its job page like a `case_create` conversation. The two learning tables are appended after the conversation tables; no earlier migration changed.
