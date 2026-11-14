# Per-case automated results

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Rev 1 (2026-11-14). Owner asked to resolve the open item "real per-case results" from [GAOS_INBOUND.md](GAOS_INBOUND.md) and to migrate and deploy.
*   **Depends on:** [GAOS_INBOUND.md](GAOS_INBOUND.md), [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md).
*   **Code:** `ai-backend/internal/services/caseresults/`, `internal/api/case_results.go`, `internal/api/gaos_inbound.go:gaosResults`.

## Problem
GAOS wants `result` (`pass|fail|skipped`), `runAt` and `runRef` per test case. The QA tools hold none: playwright-executor does exploratory runs and produces *issues*, not pass/fail of the automation repo's specs.

## Finding
The automation repo (`automation-test-hisv3`) runs its own Playwright suite. In CI it writes a JSON report (`playwright.config.ts`: `['json', {outputFile: <runDir>/results.json}]`). Its test titles begin with the case code, for example `'TC-PATREG-020 the check-in writes the registration number...'`. So the mapping run to case is in the title; nothing has to be guessed. Running that suite from playwright-executor was rejected: it needs the repo's dependencies and secrets, targets APIs and message queues as well as the browser, and would not fit the executor's 2.5 GiB budget and its "staging UI only" rule.

## Decisions
1.  **Ingest the report, do not run the suite.** The Playwright JSON report is the input.
2.  **Mapping.** Every `TC-[A-Z]+-[0-9]{3}` in a spec title gets that spec's result. Per test: `expected` and `flaky` (passed on retry) is `pass`, `unexpected` is `fail`, `skipped` is `skipped`. Several tests for one code (projects, browsers) merge: any fail wins, then any pass, else skipped.
3.  **Storage.** `kb_case_results(product_id, case_code, result, run_at, run_ref, source, updated_at)`, PK `(product_id, case_code)`: **latest only**, GAOS keeps its own history and refuses stale. An upsert replaces a row only when the new `run_at` is not older. Codes that are not active cases of the product are not stored; they are returned as `unmatched`.
4.  **`runAt`** is the report's `stats.startTime` (UTC), replaced by "now" when absent or in the future (GAOS refuses more than 5 minutes ahead). **`runRef`** is `?run_ref=` (printable ASCII, at most 120) or `pw-<unix start>`.
5.  **Two doors, same parser.**
    *   `POST /api/v1/results/playwright?product=<local product id>&run_ref=`: for the automation repo's CI. Bearer `RESULTS_INGEST_TOKEN` (32+ chars, must differ from the GAOS tokens, else disabled at start), constant-time compare, 503 when unset, 401 when wrong. The path bypasses the cookie like the GAOS routes (explicit allow-list decision, limited to this exact path).
    *   `POST /api/products/{id}/case-results`: cookie auth, Admin, DevOps, Developer, QA, QC. Used by the dashboard's "Test results" card (file upload). 24 MiB body limit (nginx allows 25m).
6.  **To GAOS.** `GET /api/v1/gaos/results` adds `result`, `runAt`, `runRef` to an item that has a stored result (never for a removed case). A new result moves the item past the cursor: the cursor key is the later of the case's and the result's `updated_at`. `runRef` is omitted when not printable ASCII or longer than 120.
7.  **Migration.** One `CREATE TABLE IF NOT EXISTS kb_case_results`, unguarded.

## Addendum: results reach GAOS by push (2026-11-14)
Live check: the push to GAOS sent only `{caseId, implemented, spec}`, and an upload started no push, so a run reached GAOS only when an operator pressed GAOS's "Pull". Now:
1.  **Content.** In the push's second step (`POST .../qa/results`), every sent case with a `kb_case_results` row carries `result`, `runAt` (unix seconds, `TIMESTAMPDIFF` from the UTC `run_at`, so independent of the session time zone) and `runRef`. A case without a row carries none of the three. A `result` outside pass/fail/skipped or a non-positive `runAt` sends no run fields at all (GAOS wants result and runAt together). `runRef` is dropped alone when empty, over 120 bytes or not printable ASCII (GAOS: only valid with `runAt`). Cases the push skips get no item. The stored run is read in the same read-only snapshot as the catalog.
2.  **Answers.** `stale` (GAOS holds a newer run) is counted in `results_stale` and named in the note, never in `rejected`, so it does not make the push `partial`. Every other reason is a real rejection like before. `future` is possible only if a report's start time is ahead of GAOS's clock by more than 300 s; the parser already replaces a start time in the future with the server's now.
3.  **Trigger.** After `caseresults.Save` stored at least one row (`stored > 0`), `ingestReport` calls an optional hook, `Router.SetOnResultsStored`, wired in `cmd/server/main.go` to `gaossync.Trigger` (the same coalescing as the knowledge-sync trigger: one push per product, a trigger during a push re-runs it once). `caseresults` does not import `gaossync`. A nil hook does nothing and a panic in it is contained; the upload's answer never depends on the push. A product that is not flagged is ignored by `Trigger`.
4.  **Why a full push and not a results-only one.** The results step needs the list of cases GAOS accepted (a refused case gets no item), and a full push is idempotent and already coalesced. The cost is a cases step that answers `unchanged` in the common case.

5.  **Contract (GAOS, as served 2026-11-14).** Request item: `{"caseId","implemented","spec"}` plus optional `"result"` (`pass|fail|skipped`), `"runAt"` (integer unix seconds) and `"runRef"` (printable ASCII, at most 120 bytes; GAOS accepts it only with `runAt`). Answer `{"applied":n,"unchanged":n,"rejected":[{"caseId","reason"}],"catHead":<int>}`; the exact bodies are pinned in `TestGaosResultsBodiesAsServedToday`. Reasons are matched as literal strings: `stale` (informational) and `exists-differs` (informational, cases step) are not failures; everything else (`future`, `bad-result`, `bad-run-ref`, `run-incomplete`, `unknown-case`...) counts in `rejected`. `results_stale` counts a `stale` from either step. The note reads `... results applied N; rejected M` and, when any, `; K results older than GAOS's`. A case GAOS refused in the cases step (other than `exists-differs`) gets no results item; items are sent after all cases batches, 200 per request.
6.  **Known limits, deliberately not built now.** (a) A permanent rejection (for example `future` while this server's clock is more than 300 s ahead of GAOS's) keeps the push `partial` on every pass until the cause is gone; the rejected sample names it. (b) A `stale` is green by design: if GAOS holds a newer run from elsewhere, ours stays invisible there. (c) A CI that posts several shards starts a push per upload (coalesced to one running plus one re-run per product); a trailing debounce is the follow-up if that shows up in GAOS's write rate. (d) The trigger is in memory: a restart between an upload and its push loses it, and the next upload, knowledge sync or the manual button delivers it. (e) The `run_ref` column is `VARCHAR(120)`, so the over-long guard in the push only protects against a future schema change.

## Non-goals
*   Running specs from the QA tools; result history; showing results on the test-case pages of the dashboard.
*   Wiring the automation repo's CI to call the endpoint (the other team adds one `curl` after the test step, with `RESULTS_INGEST_TOKEN`).

## CI snippet (for the automation repo)
```
curl -fsS -X POST -H "Authorization: Bearer $RESULTS_INGEST_TOKEN" --data-binary @results.json \
  "https://qa-tool.sismedika.com/api/v1/results/playwright?product=<id>&run_ref=$CI_RUN_ID"
```
