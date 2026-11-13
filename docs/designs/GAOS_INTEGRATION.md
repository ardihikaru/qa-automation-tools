# GAOS integration: push a product's test cases to the GAOS kanban

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Draft rev 1 for review (2026-11-14).
*   **Date:** 2026-11-14
*   **Depends on:** [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (implemented): the `kb_*` tables this feature reads.
*   **Code references** are `file:line` at `main` = `55c905e`. Paths are relative to the repo root.

## Overview
GAOS is a separate planning system (its own repo, running at `https://gaos.mfardiansyah.id`). It keeps a kanban of cards per product. The product owner asks for this: in `/products`, each product gets a flag "integrated with GAOS". For a flagged product, every test case the QA tools hold is created in GAOS (so it appears on the SUT kanban) and kept up to date when the catalog changes.

The QA tools already hold the test cases: the product knowledge sync copies the automation repo's `docs/` catalog into `kb_test_cases` and `kb_epics` (`ai-backend/internal/services/knowledge/sync.go:223-298`). This feature adds a **one-way push** of those rows to the GAOS REST API:

1.  a per-product setting (flag + GAOS product id + last push outcome) in `product_settings`;
2.  a new Go package `ai-backend/internal/services/gaossync` that reads the active rows and calls two GAOS endpoints;
3.  three triggers: after every successful knowledge sync, when the flag is turned on, and a manual button;
4.  a badge in the products list and a "GAOS integration" card in the product detail.

## GAOS contract (as given by the GAOS side; this repo cannot verify it offline)
Base URL `GAOS_API_URL` (e.g. `https://gaos.mfardiansyah.id/api/v1`), header `Authorization: Bearer <GAOS_API_TOKEN>`, `Content-Type: application/json`. The token is scoped to specific products on the GAOS side; SUT is GAOS product `p-6`. That id is per-product data in `product_settings`, never in code.

*   `POST /products/{pid}/qa/cases` with `{"source":"qa-automation-tools","epics":[{code,title,service,source,module}],"cases":[{caseId,epic,module,features[],userStory,scenario,polarity,level,pri,decision:"planned",reason:"",implemented,spec}]}`. Answer 200 `{created,unchanged,rejected:[{caseId,reason}],epicsAdded,cardsAdded,cardsDeferred,catHead}`.
*   `POST /products/{pid}/qa/results` with `{"source":"qa-automation-tools","items":[{caseId,implemented,spec}]}`. Answer 200 `{applied,unchanged,rejected:[{caseId,reason}],catHead}`.
*   Planning fields are **create-only**: an existing `caseId` whose planning fields differ is refused with `exists-differs` and left unchanged. GAOS owns the planning half once a case exists; this feature only creates new cases and reports script facts (`implemented`, `spec`).
*   `decision` must be `planned` with an empty `reason`. Case id middle segment must equal the epic code (`TC-AUTH-001` belongs to `AUTH`, else `bad-case-epic`). Field patterns: epic `^[A-Z]{2,8}$`; module `^[A-D][0-9]{2}$` or empty; user story `^US-[A-Z]+-[0-9]{2}$` or empty; scenario at most 600 bytes; spec at most 240 bytes; features each `^[A-D][0-9]{2}\.[0-9]{2,3}(\.[0-9]{2})?$`, at most 12; level `API|GQL|WEB|E2E`; pri `P0..P3`; polarity `+` or `-`; at most 200 cases per batch.
*   HTTP 429 means "one write in flight per token", with a `Retry-After` header in seconds. 401, 403, 404, 503 are terminal for the push.
*   A new epic on the GAOS side becomes a blocked kanban card by itself. The QA tools do nothing for that.

## Problem
*   The test cases live only in the QA tools (`kb_test_cases`). The SUT kanban in GAOS is filled by hand, so it drifts from the catalog.
*   There is no per-product switch to say "this product is tracked in GAOS".

## Goals
1.  A per-product flag `gaos_integrated` and the GAOS product id, editable by Admin and DevOps only, in the same way as `knowledge_format`.
2.  After every successful knowledge sync of a flagged product, and when the flag is turned on, and on demand, push the product's eligible test cases. **Idempotent**: re-sending everything is safe (GAOS answers `unchanged`).
3.  Never block, slow or fail a knowledge sync because of GAOS. Never run two pushes for one product at once.
4.  Record the outcome (time, `ok|partial|failed`, a short note) and show it in the dashboard.
5.  Unconfigured server (`GAOS_API_URL` or `GAOS_API_TOKEN` unset or invalid): the flag cannot be turned on and nothing is ever sent.
6.  The token never appears in a log line, a note, an API answer or a test fixture's output.

## Non-Goals
*   **The reverse direction.** GAOS reading results from, or sending a catalog to, the QA tools (`GET /api/v1/gaos/results`, `POST /api/v1/gaos/catalog` on the QA tools) is **not built**. See Open questions.
*   **Run records.** The QA tools have no run records for catalog cases, so `runAt`, `result` and `runRef` are never sent.
*   **Updating planning fields in GAOS.** GAOS refuses it by design; a changed scenario in the catalog is reported as informational (`exists-differs`), not as a failure.
*   **Cases that are not plannable.** Cases with status `blocked`, `manual-only` or `wont` are not sent.
*   **playwright-executor, deployment, and the knowledge sync's own behaviour.**

## Decisions
1.  **One-way, idempotent push; GAOS keeps the planning half.** It matches the contract above and needs no state of ours beyond the last outcome.
2.  **Only `planned` and `automated` cases are sent.** `decision` must be `planned`, so a case in `blocked`, `manual-only` or `wont` cannot be expressed. Skipping them is counted as `skipped`, not as a failure.
3.  **Whole-catalog push, not a diff.** The knowledge sync's diff (`kb_syncs.diff`, `sync.go:269`) would save requests, but a diff is lost when a push fails, and a full re-send costs at most `ceil(1659/200)` = 9 batches for SUT (counts in PRODUCT_KNOWLEDGE_SYNC.md). Idempotency makes the full push the simplest correct design.
4.  **In-process single flight.** A mutex-guarded map of running products. The backend runs as one container (`deploy/qa-tool/docker-compose.prod.yml`), so a DB lock is not needed. A trigger that arrives while a push runs sets a `dirty` flag, and the running push runs once more when it ends, so a catalog change is never lost.
5.  **The hook is a callback.** `knowledge.Syncer.OnSucceeded func(productID int)` is set in `cmd/server/main.go`; `knowledge` never imports `gaossync`.
6.  **Config is validated once, at startup, and reported as a boolean.** An invalid value is logged (without the token) and treated as "not configured".
7.  **Epic `source`** is the constant `qa-automation-tools` (GAOS's contract names the field but not its meaning; the constant cannot be rejected as a path). Owner may change it.
8.  **Epic `module`** is the module that occurs most among the epic's sent cases' modules (ties: the smallest code), or empty.
9.  **Case `module`** is the `module_code` of the case's first feature (lowest feature code), where features come from the case's story (`kb_story_features`); empty when there is none or it does not match the pattern.

## Design

### Schema
One guarded `ALTER TABLE product_settings` appended to `ai-backend/internal/db/migrations.go` after the last entry (`migrations.go:693`), in the style of the `knowledge_format` column (`migrations.go:624-625`). One statement, so it is atomic in MySQL; the `skip-if` guard tests the first new column:

| Column | Type |
|---|---|
| `gaos_integrated` | `TINYINT(1) NOT NULL DEFAULT 0` |
| `gaos_product_id` | `VARCHAR(40) NOT NULL DEFAULT ''` |
| `gaos_last_sync_at` | `DATETIME NULL` |
| `gaos_last_status` | `VARCHAR(20) NOT NULL DEFAULT ''` (`ok`, `failed`, `partial`) |
| `gaos_last_note` | `VARCHAR(300) NOT NULL DEFAULT ''` |

`TestGuardedMigrationsAreKnown` (`internal/db/skipif_test.go:34-60`) pins the list of guarded entries; the new entry is added to its `want` list.

### Config
`internal/config/config.go`: `ProductsConfig` (`config.go:70-76`) is for the repo pool; a new `GAOSConfig{APIURL, APIToken}` is added to `BackendConfig` (`config.go:50-57`, `yaml:"-"`, env only) and read with `getEnvString("GAOS_API_URL", "")`, `getEnvString("GAOS_API_TOKEN", "")` next to the executor block (`config.go:327-331`). Documented, placeholders only, in `ai-backend/.env.example` and `deploy/qa-tool/.env.prod.example`; `README.md:350` env table gets a row. `deploy/qa-tool/docker-compose.prod.yml:18` already loads the whole `.env.prod`, so no compose change.

`gaossync.ValidateConfig(url, token)` rules: URL parses; scheme `https`, or `http` only when the host is an IP literal that is loopback or inside `10.8.0.0/24`; no userinfo, no query, no fragment; non-empty host; trailing `/` trimmed. Token: starts with `gaos_`, length at least 37, only printable ASCII without spaces (a header-injection guard). Both empty = "not configured" with no warning; one set and invalid = a logged warning naming the variable, never the value.

### The sync service `internal/services/gaossync`
```go
type Syncer struct { /* db, base URL, token, *http.Client, sleep func, mutex + running map */ }
func New(d *db.DB, apiURL, token string) *Syncer      // never nil with a DB; Configured() reports validity
func (s *Syncer) Configured() bool
func (s *Syncer) Start(productID int) (*Run, error)     // manual: ErrBusy | ErrNotIntegrated | ErrNotConfigured
func (s *Syncer) Trigger(productID int)                 // automatic: never blocks, never errors, coalesces
type Run struct{ Done <-chan struct{} /* ... */ }; func (r *Run) Result() Result
```
`Result` (JSON): `status`, `note`, `sent`, `skipped`, `created`, `unchanged`, `exists_differs`, `rejected`, `rejected_sample` (first 20 `{caseId, reason}`), `results_applied`, `results_unchanged`, `epics_added`, `cards_added`, `cards_deferred`, `cat_head`, `batches`, `started_at`, `finished_at`.

**Selection (from the DB).** Active cases: `SELECT ... FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL ORDER BY code` (columns at `migrations.go:570-600`). Story features: `kb_story_features` joined to non-removed `kb_features` (`migrations.go:602-610`, `store.go:288`). Epics: `kb_epics` with `removed_at IS NULL`. A case is **sent** only when all of these hold; otherwise it is `skipped` (counted, never sent):
*   `status` is `planned` or `automated`;
*   `code` matches `^TC-[A-Z]+-[0-9]{3}$` (so local `L-` drafts are skipped) and its middle segment equals `epic_code`, which matches `^[A-Z]{2,8}$`;
*   `polarity`, `level`, `priority` are in the GAOS enums.

**Mapping.** `caseId` = code; `epic` = epic code; `module`, `features` (valid, sorted, at most 12), `userStory` (the story code if it matches the pattern, else empty) as above; `scenario` clipped to 600 bytes on a rune boundary with `...` as the last three bytes of the 600; `polarity`, `level`, `pri` = `priority`; `decision` `planned`, `reason` `""`; `implemented` = status `automated`; `spec` = first of `spec_paths` only when automated and valid (relative, no `..`, at most 240 bytes), else `""`. Automated with no usable spec is sent as `implemented:false`, `spec:""` and counted in the note.

**Requests.**
1.  Cases in batches of at most 200, each batch carrying only the epics its cases use. Sequential.
2.  Then results in batches of at most 200 for every sent case that GAOS did not reject for a reason other than `exists-differs` (a case rejected as invalid does not exist in GAOS, so a result for it would only be rejected too).
3.  Per-request timeout 30 s; the whole push has a 5 minute budget (its own context, detached from the HTTP request that started it). Response read capped at 2 MiB; redirects are refused (`CheckRedirect` returns `http.ErrUseLastResponse`, a 3xx then fails the push). Response shape is strict: the required counters must be present and non-negative integers and `rejected` an array of `{caseId, reason}` strings; anything else fails the push. Unknown extra fields are ignored so GAOS can add fields.
4.  HTTP 429: wait `Retry-After` seconds (default 1, cap 10), at most 3 retries of the same batch; then fail. 401, 403, 404, 503 and any other non-200: stop, status `failed`, note `GAOS answered HTTP <code> on <step>` plus a fixed hint (for example 404 "check the GAOS product id and the token's product scope"). A transport error or timeout fails the push with a fixed text. **The response body, the token and the URL's query are never put in a note or a log.**

**Outcome.** `ok`: no HTTP-level failure and every rejection is `exists-differs`. `partial`: no HTTP-level failure and some other rejection. `failed`: an HTTP-level failure. The outcome is stored in `gaos_last_sync_at/status/note` with a fresh 10 s context, so an expired push context cannot lose it. The note is at most 300 characters (clipped by rune).

**Single flight and coalescing.** `Start` returns `ErrBusy` when the product is running. `Trigger` sets `dirty` instead; the runner loops while `dirty`. A panic is recovered and recorded as `failed`.

**What the flag read.** A push re-reads `gaos_integrated` and `gaos_product_id` when it starts. If the flag was turned off meanwhile, the push does nothing.

### Triggers
*   **After a knowledge sync.** `knowledge.Syncer` gets an exported `OnSucceeded func(productID int)`. `run` calls it right after the success transaction commits (`sync.go:295`), inside a `recover`, before the AI summary. The callback is `gaosSyncer.Trigger`, which spawns its own goroutine. Skipped scheduled syncs (`sync.go:205`) and failed syncs do not call it.
*   **Flag turned on, or the GAOS product id changed while on.** `PUT /api/products/{id}` calls `Trigger` after its transaction commits.
*   **Manual.** `POST /api/products/{id}/gaos/sync`.

### Products API (`internal/api/products.go`)
*   `productUpdateBody` (`products.go:620-630`) gains `GaosIntegrated *bool json:"gaos_integrated"` and `GaosProductID *string json:"gaos_product_id"`. The body is decoded strictly (`products.go:432`), so the field names must exist or a client gets 400.
*   Role rule: identical to `knowledge_format` (`products.go:672-682`): a change by anyone but Admin or DevOps (`canToggleWork`, `products.go:260`) is 403 and nothing is saved; sending the current value is not a change.
*   Validation, inside the product transaction: `gaos_product_id` must match `^p-[0-9]+$` (at most 40 chars) when non-empty. Turning on requires a valid id and `GAOS configured`, else 400 with a clear message. Changing the id resets the last outcome. Turning off keeps the id.
*   JSON: `models.Product` gains `GAOS GAOSSettings json:"gaos"` = `{configured, integrated, product_id, last_sync_at, last_status, last_note}`; `loadProducts` selects the new columns in the settings query (`products.go:202`); the router fills `configured` just before responding. The list response also carries a top-level `gaos_configured`.
*   `POST /api/products/{id}/gaos/sync`: routed beside `knowledge` (`products.go:507`); Admin/DevOps else 403; 400 not configured; 409 not integrated; 409 while a push runs. It waits for the push up to 280 s (below the 300 s proxy timeout in `deploy/qa-tool/nginx.dashboard.conf:38-39`) and answers 200 with the `Result`; if the push is still running it answers 202 `{"status":"running"}` and the push goes on.

### Dashboard
`dashboard/src/views/products/api.ts` gets the `gaos` fields; `ProductsView.tsx` shows a small `GAOS` badge on integrated products (title = last status and time); `ProductDetail.tsx` gets a "GAOS integration" card (toggle, id input, last sync, "Sync to GAOS now"). The toggle and input are disabled with visible text when the server is not configured or the user is not Admin/DevOps. Patterns follow `dashboard/src/views/products/KnowledgeCell.tsx` and `.claude/rules/dashboard.md`.

## Alternatives considered
| Alternative | Why not |
|---|---|
| Push only the sync diff | A failed push loses the diff; full re-send is idempotent and 9 requests for SUT (Decision 3). |
| `knowledge` imports `gaossync` | Couples the parser to an external system; a callback keeps `knowledge` testable offline. |
| A DB lock row for single flight | The deployment is one backend container; a mutex is enough and has no stale-lock failure. |
| Per-product API URL or token | More secrets in the DB; the token is one per QA tools instance and scoped by GAOS. |
| Run it from a cron | The owner wants it "automatically" after a sync; the sync already has a schedule. |

## Change list
*   `ai-backend/internal/db/migrations.go`, `skipif_test.go`, `migrations_test.go` (columns, idempotency)
*   `ai-backend/internal/config/config.go`, `.env.example`, `deploy/qa-tool/.env.prod.example`, `README.md`, `deploy/qa-tool/README.md`
*   `ai-backend/internal/services/gaossync/` (new: `config.go`, `sync.go`, `client.go`, `README.md`, tests)
*   `ai-backend/internal/services/knowledge/sync.go`, `README.md` (`OnSucceeded`)
*   `ai-backend/internal/models/models.go`, `internal/api/products.go`, `router.go`, `README.md`, tests
*   `ai-backend/cmd/server/main.go` (wiring)
*   `dashboard/src/views/products/api.ts`, `ProductDetail.tsx`, `ProductsView.tsx`
*   `docs/designs/SITEMAP.md`

## Tests
*   `gaossync` with an `httptest` fake GAOS: batching at 200 (cases and results), the create-only mapping and every skip rule, scenario/spec clipping, `exists-differs` is informational, other rejections give `partial`, 429 retry with the sleep stubbed, 401/403/404/503 stop, a redirect is refused, the token never appears in a note or log, the response size cap, strict shape, idempotent re-send, single flight and coalescing, `ValidateConfig` cases.
*   Products API: role rule, configured-or-400, id pattern, JSON shape, 409 busy, 409 not integrated.
*   Migration: second run is a no-op and the columns exist.
*   DB-backed tests need `QA_TEST_MYSQL_DSN` (`internal/testdb/testdb.go`); they skip without it. This work runs them against a scratch MySQL 8.0.33 container.

## Open questions
1.  **Reverse endpoints.** `GET /api/v1/gaos/results` and `POST /api/v1/gaos/catalog` on the QA tools, which GAOS would call, are not built. Who calls what, and with which auth, is undecided.
2.  **Epic `source` value** (Decision 7): confirm with the GAOS side what it expects.
3.  **A scheduled sync that is skipped** (same commit) does not push, so a push that failed earlier is retried only by the next real change, a flag toggle or the button. Acceptable now; revisit if failures are common.
4.  Should a case whose planning fields later change in the catalog be surfaced somewhere (count of `exists-differs`)? The count is in the result and the note only.
