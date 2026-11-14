# services/caseresults

## Overview
Per-case automated results (spec: [CASE_RESULTS.md](../../../../docs/designs/CASE_RESULTS.md)). It reads a Playwright JSON report, maps each spec to the `TC-XXX-NNN` codes in its title, and stores the latest result of every case of a product in `kb_case_results`. GAOS reads them through `GET /api/v1/gaos/results`.

## Interface
- `Parse(r io.Reader, now time.Time) (*Report, error)` (`parse.go`): `Report{StartedAt, Results map[code]pass|fail|skipped, Tests}`. `ErrNotReport` when the JSON has no suites. Playwright statuses: `expected` and `flaky` are pass, `unexpected` is fail, `skipped` is skipped; several tests for one code merge as fail over pass over skipped. A start time in the future is replaced by `now`.
- `Save(ctx, db, productID, report, runRef, source) (*Outcome, error)` (`store.go`): upserts in one transaction. A report older than the stored `run_at` leaves the row unchanged (`Stale`). Codes that are not active `kb_test_cases` of the product are only reported (`Unmatched`, first 20, plus the count).
- `DefaultRunRef(t)`: `pw-<unix>`.

## Invariants
- Latest run wins per `(product_id, case_code)`; `run_at` is UTC.
- Writes only `kb_case_results`; never `kb_test_cases`.

## Side effects
Reads `kb_test_cases`, writes `kb_case_results`.

## Testing
`go test -mod=mod ./internal/services/caseresults/` (parser, no DB). The store is covered by `internal/api/case_results_test.go` with `QA_TEST_MYSQL_DSN`.
