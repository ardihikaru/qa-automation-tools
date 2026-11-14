# Per-case automated results

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Rev 1 (2026-11-14). Owner asked to resolve the open item "real per-case results" from [GAOS_INBOUND.md](GAOS_INBOUND.md) and to migrate and deploy.
*   **Depends on:** [GAOS_INBOUND.md](GAOS_INBOUND.md), [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md).
*   **Code:** `ai-backend/internal/services/caseresults/`, `internal/api/case_results.go`, `internal/api/gaos_inbound.go:gaosResults`.

## Problem
GAOS wants `result` (`pass|fail|skipped`), `runAt` and `runRef` per test case. The QA tools hold none: playwright-executor does exploratory runs and produces *issues*, not pass/fail of the automation repo's specs.

## Finding
The automation repo (`automation-test-sut`) runs its own Playwright suite. In CI it writes a JSON report (`playwright.config.ts`: `['json', {outputFile: <runDir>/results.json}]`). Its test titles begin with the case code, for example `'TC-PATREG-020 the check-in writes the registration number...'`. So the mapping run to case is in the title; nothing has to be guessed. Running that suite from playwright-executor was rejected: it needs the repo's dependencies and secrets, targets APIs and message queues as well as the browser, and would not fit the executor's 2.5 GiB budget and its "staging UI only" rule.

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

## Non-goals
*   Running specs from the QA tools; result history; showing results on the test-case pages of the dashboard.
*   Wiring the automation repo's CI to call the endpoint (the other team adds one `curl` after the test step, with `RESULTS_INGEST_TOKEN`).

## CI snippet (for the automation repo)
```
curl -fsS -X POST -H "Authorization: Bearer $RESULTS_INGEST_TOKEN" --data-binary @results.json \
  "https://qa-tool.mfardiansyah.id/api/v1/results/playwright?product=<id>&run_ref=$CI_RUN_ID"
```
