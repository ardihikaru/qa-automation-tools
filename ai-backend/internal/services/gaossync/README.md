# services/gaossync

## Overview
GAOS integration (spec: [GAOS_INTEGRATION.md](../../../../docs/designs/GAOS_INTEGRATION.md)). For a product flagged `product_settings.gaos_integrated`, it pushes the product's active knowledge-base test cases (`kb_test_cases`, `kb_epics`, `kb_story_features`) to the GAOS REST API so they appear on its kanban, and reports each case's script facts (`implemented`, `spec`). It is one-way and idempotent.

## Interface
- `New(db, apiURL, token) *Syncer`; `Configured()` is true only with a valid `GAOS_API_URL` and `GAOS_API_TOKEN` (`ValidateConfig`, `config.go`). `ValidProductID` (`^p-[0-9]+$`, at most 40).
- `Start(productID) (*Run, error)`: a manual push in the background. Errors: `ErrNotConfigured`, `ErrNotIntegrated`, `ErrBusy`. `Run.Done` closes at the end; `Run.Result()` is the last pass's `Result` (`status` ok/partial/failed, `note`, counts, `rejected_sample`).
- `Trigger(productID)`: automatic start (never blocks, never errors, ignores a product that is not integrated); a trigger during a push makes the running push run once more. `Retry(productID)`: `Trigger` only when the last outcome is not `ok`.
- `StartSweeper(ctx, interval)`: the outbox sweeper (`sweeper.go`), started by `cmd/server/main.go` with a 1 minute interval; first sweep 5 s after start.
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

- **Durable delivery (outbox).** `Trigger`/`Retry` first persist `product_settings.gaos_push_requested_at = UTC_TIMESTAMP(6)` (only for a configured server and a flagged product with an id; `Retry` still only when the last outcome is not `ok`), then start or coalesce in memory. A marker-write failure is logged and swallowed; the in-memory push still runs. `Start` writes no marker. A pass reads the marker when it starts and, if it ends `ok` or `partial`, clears it `WHERE gaos_push_requested_at <= <that value>` and resets `gaos_push_attempts`, so a request that arrived during the pass survives it and is delivered by the re-run. A `failed` pass keeps the marker and adds one attempt; an abandoned pass (flag or id changed) clears nothing.
- **Marker before return, and a level-triggered net.** `Trigger`/`Retry` read the settings and write the marker **before returning** to their caller (only the push is asynchronous), so the push is owed durably once an upload has its answer. The sweeper first promotes to a marker every flagged product with an id whose `kb_case_results` or `kb_test_cases` rows are newer than `gaos_last_sync_at` (or that never pushed), which covers a kill between the data commit and the marker write; at worst one redundant push (GAOS answers `unchanged`).
- **Sweeper.** Once at start and then each interval, over every product with a marker: not flagged or no valid id: marker cleared, nothing sent; flagged: started through `begin` (a running push is never duplicated) unless the time since `gaos_last_sync_at` is below `min(attempts x 30 s, 10 min)`. One sweep at a time per process and per database (MySQL named lock `gaossync-sweeper:<database>`; a process that cannot take it skips the sweep); after 30 failed attempts it gives up on a product (marker cleared, the `failed` outcome stays), panics recovered, stops with its context, does nothing when the server is not configured. Delivery is at-least-once; GAOS's `unchanged`/`stale` answers make a re-send harmless.

## Defect push (`defects.go`, QA_DEFECT_TRIAGE.md)

`pushDefects` runs at the end of every pass: it sends the `qa_defect_state` rows whose hash differs from `pushed_hash` and
`rejected_hash` to `POST /products/{id}/qa/defects` (`{"source":"push","items":[...]}`, at most 50 items and 240 KiB per
request). It derives `automationType`, `backend`, `targetRepos` (product repos by name plus `product_service_repos` for e2e)
and `operations` (via `SetSpecReader`, set by the server). 404 or 405 backs the defect push off for 30 minutes without
failing the product; 413 halves the batch; `rejected` items are not retried until they change; `defect-limit` keeps them
pending for an hour. The sweeper's level-triggered clause also owes a push to a product with pending states.

## GAOS Inbox (`inbox.go`, CASE_AUTOMATION.md section 6)
`CreateInboxItem(ctx, gaosPID, InboxCreate)` posts `{externalId, title, question, context, options:[{label,description}], expiresInHours}` to `{base}/products/{pid}/inbox` (201 new or 200 existing; the answer must echo `externalId` and carry a `status`). `GetInboxItem(ctx, gaosPID, externalID)` reads `{base}/products/{pid}/inbox/{externalId}` (`status` `open|answered|expired`, `choice`, `optionLabel`, `text`, `answeredAt`). Same conventions as the push: QA bearer, redirects refused, 429 retried, failure notes hold no URL or body. `GetInboxItem` also returns GAOS's `source` object raw. `AnswerInboxItem(ctx, gaosPID, externalID, InboxAnswer{Choice, Text, Actor{Username, Display}})` (HISTORY_ANSWER.md) posts to `{base}/products/{pid}/inbox/{externalId}/answer` (GAOS v0.43.0; the caller's context is the budget; a 429 is retried once after `Retry-After` capped at 5 s) and returns the item on 200, on 409 `already_answered` (`Already`) and on 410 `expired` (`Expired`); every other status is a `*Failure` with its status and at most GAOS's short error code (404 also means an older GAOS without the endpoint). `ValidActorUsername` is GAOS's rule for `actor.username`. They need no database (only a configured client) and never touch `product_settings`.

## Side Effects
HTTPS calls to GAOS; writes `product_settings.gaos_last_sync_at/status/note` and the outbox columns `gaos_push_requested_at/gaos_push_attempts`. Reads `kb_*` and `kb_case_results`.

## Verification
`go test -mod=mod ./internal/services/gaossync/`: config validation, mapping and skip rules, clipping run without a database. With `QA_TEST_MYSQL_DSN` (scratch MySQL): pushes against an `httptest` fake GAOS (batching, 429, 401/403/404/503/500, redirects, response shape and size, idempotent re-send, stored runs sent as result/runAt/runRef (and omitted without one), `stale` informational, the response bodies GAOS serves today, one-at-a-time, coalescing, stale push, token never in notes or logs), and `outbox_test.go`: marker written before the push and cleared by ok/partial, restart delivery by a fresh `Syncer`, failed push keeps the marker with backoff then delivers, trigger during a pass survives it, unflagged marker cleared, marker-write failure still pushes, sweeper never duplicates a running push.

**Defect cases.** `readCatalog` also reads the case codes of `qa_defect_state`; `buildPlan` sends a `blocked` case that is in that set and has a usable spec, as an implemented `planned` case, so GAOS knows the case when its defect arrives.

**Operation names.** `operationsFor` walks the spec's imports (relative ones and the aliases of the repo's `tsconfig.json` `paths`), at most 24 files, those of the spec's own service first, and lists up to 5 GraphQL operation or REST route names for the defect push.

The operations are ranked by words shared with the spec's source (`rankByHint`) before the five-name cap.
