# services/gaossync

## Overview
GAOS integration (spec: [GAOS_INTEGRATION.md](../../../../docs/designs/GAOS_INTEGRATION.md)). For a product flagged `product_settings.gaos_integrated`, it pushes the product's active knowledge-base test cases (`kb_test_cases`, `kb_epics`, `kb_story_features`) to the GAOS REST API so they appear on its kanban, and reports each case's script facts (`implemented`, `spec`). It is one-way and idempotent.

## Interface
- `New(db, apiURL, token) *Syncer`; `Configured()` is true only with a valid `GAOS_API_URL` and `GAOS_API_TOKEN` (`ValidateConfig`, `config.go`). `ValidProductID` (`^p-[0-9]+$`, at most 40).
- `Start(productID) (*Run, error)`: a manual push in the background. Errors: `ErrNotConfigured`, `ErrNotIntegrated`, `ErrBusy`. `Run.Done` closes at the end; `Run.Result()` is the last pass's `Result` (`status` ok/partial/failed, `note`, counts, `rejected_sample`).
- `Trigger(productID)`: automatic start (never blocks, never errors, ignores a product that is not integrated); a trigger during a push makes the running push run once more. `Retry(productID)`: `Trigger` only when the last outcome is not `ok`.
- `Derive(ctx, productID)` (`derived.go`) returns the planning fields the push derives per case; the decisions view compares them with `gaos_decisions`.
- Wired by `cmd/server/main.go` to `knowledge.Syncer.OnSucceeded` and `OnUnchanged`, and (as `Trigger`) to `Router.SetOnResultsStored`, so an upload that stored results starts a push, and to the products API (`internal/api/products_gaos.go`).

## Invariants
- **Create-only mapping.** Only cases with status `planned` or `automated`, code `^TC-[A-Z]+-[0-9]{3}$` whose middle segment equals the epic code (`^[A-Z]{2,8}$`), and enum-valid polarity/level/priority are sent; every other active case is counted as `skipped`. `decision` is `planned` with an empty `reason`. Scenario at most 600 bytes, spec at most 240, features at most 12. `exists-differs` is informational.
- **Order.** `POST {GAOS_API_URL}/products/{id}/qa/cases` in batches of at most 200 (epic entries computed once for the whole push), then `.../qa/results` in batches of at most 200 for every sent case GAOS did not refuse. Each item is `{caseId, implemented, spec}` plus, when `kb_case_results` holds a run for the case, `result`, `runAt` (unix seconds) and `runRef` (dropped when empty, over 120 bytes or not printable ASCII); no run, no such fields. A `stale` refusal is counted in `results_stale` and does not make the push `partial`; any other refusal does.
- **One writer.** One push per product at a time and one push at a time overall (GAOS allows one write in flight per token). The four catalog reads are one read-only REPEATABLE READ transaction.
- **Transport.** 30 s per request, 5 min per push, response capped at 2 MiB, redirects refused, no proxy, strict response shape (unknown extra fields ignored). 429: wait `Retry-After` (1 to 10 s), at most 3 retries per batch. Any other non-200 stops the push as `failed`.
- **Secrets.** The token is only in the `Authorization` header. Notes, logs and API answers never hold the token, the URL or a response body.
- **Stale push.** The flag and id are re-read before every request; a push for a product that was switched off or re-pointed stops and records nothing, and its outcome is written only `WHERE gaos_integrated = 1 AND gaos_product_id = <the id it used>`.
- A push never touches the knowledge sync: it runs in its own goroutine and context.

## Side Effects
HTTPS calls to GAOS; writes `product_settings.gaos_last_sync_at/status/note`. Reads `kb_*` and `kb_case_results`.

## Verification
`go test -mod=mod ./internal/services/gaossync/`: config validation, mapping and skip rules, clipping run without a database. With `QA_TEST_MYSQL_DSN` (scratch MySQL): pushes against an `httptest` fake GAOS (batching, 429, 401/403/404/503/500, redirects, response shape and size, idempotent re-send, stored runs sent as result/runAt/runRef (and omitted without one), `stale` informational, the response bodies GAOS serves today, one-at-a-time, coalescing, stale push, token never in notes or logs).
