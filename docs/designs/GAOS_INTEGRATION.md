# GAOS integration: push a product's test cases to the GAOS kanban

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Rev 2 (2026-11-14): rev 1 was read by two zero-context reviewers (design, implementation readiness); see "Review triage". Owner pre-authorised implementation and merge; deploy is not authorised.
*   **Date:** 2026-11-14
*   **Depends on:** [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (implemented): the `kb_*` tables this feature reads.
*   **Code references** are `file:line` at `main` = `55c905e`. Paths are relative to the repo root.

## Overview
GAOS is a separate planning system (its own repo, running at `https://gaos.sismedika.com`). It keeps a kanban of cards per product. The product owner asks for this: in `/products`, each product gets a flag "integrated with GAOS". For a flagged product, every test case the QA tools hold is created in GAOS (so it appears on the HISv3 kanban) and kept up to date when the catalog changes.

The QA tools already hold the test cases: the product knowledge sync copies the automation repo's `docs/` catalog into `kb_test_cases` and `kb_epics` (`ai-backend/internal/services/knowledge/sync.go:263-290`). This feature adds a **one-way push** of those rows to the GAOS REST API:

1.  a per-product setting (flag + GAOS product id + last push outcome) in `product_settings`;
2.  a new Go package `ai-backend/internal/services/gaossync` that reads the active rows and calls two GAOS endpoints;
3.  three triggers: after every successful knowledge sync, when the flag is turned on, and a manual button;
4.  a badge in the products list and a "GAOS integration" card in the product detail.

## GAOS contract (as given by the GAOS side; this repo cannot verify it offline)
Base URL `GAOS_API_URL` (e.g. `https://gaos.sismedika.com/api/v1`), header `Authorization: Bearer <GAOS_API_TOKEN>`, `Content-Type: application/json`. The token is scoped to specific products on the GAOS side; HISv3 is GAOS product `p-6`. That id is per-product data in `product_settings`, never in code.

*   `POST /products/{pid}/qa/cases` with `{"source":"qa-automation-tools","epics":[{code,title,service,source,module}],"cases":[{caseId,epic,module,features[],userStory,scenario,polarity,level,pri,decision:"planned",reason:"",implemented,spec}]}`. Answer 200 `{created,unchanged,rejected:[{caseId,reason}],epicsAdded,cardsAdded,cardsDeferred,catHead}`.
*   `POST /products/{pid}/qa/results` with `{"source":"qa-automation-tools","items":[{caseId,implemented,spec}]}`. Answer 200 `{applied,unchanged,rejected:[{caseId,reason}],catHead}`.
*   Planning fields are **create-only**: an existing `caseId` whose planning fields differ is refused with `exists-differs` and left unchanged. GAOS owns the planning half once a case exists; this feature only creates new cases and reports script facts (`implemented`, `spec`).
*   `decision` must be `planned` with an empty `reason`. Case id middle segment must equal the epic code (`TC-AUTH-001` belongs to `AUTH`, else `bad-case-epic`). Field patterns: epic `^[A-Z]{2,8}$`; module `^[A-D][0-9]{2}$` or empty; user story `^US-[A-Z]+-[0-9]{2}$` or empty; scenario at most 600 bytes; spec at most 240 bytes; features each `^[A-D][0-9]{2}\.[0-9]{2,3}(\.[0-9]{2})?$`, at most 12; level `API|GQL|WEB|E2E`; pri `P0..P3`; polarity `+` or `-`; at most 200 cases per batch.
*   HTTP 429 means "one write in flight per token", with a `Retry-After` header in seconds. 401, 403, 404, 503 are terminal for the push.
*   A new epic on the GAOS side becomes a blocked kanban card by itself. The QA tools do nothing for that.

## Problem
*   The test cases live only in the QA tools (`kb_test_cases`). The HISv3 kanban in GAOS is filled by hand, so it drifts from the catalog.
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
*   **Updating planning fields in GAOS.** GAOS refuses it by design; a changed scenario in the catalog is reported as informational (`exists-differs`), not as a failure. A case that later becomes `blocked`, `manual-only` or `wont`, or disappears from the catalog, stays on the GAOS kanban: there is no withdraw call (Open question 4).
*   **Pushing local edits.** Local drafts (`L-` codes) are never sent and a local edit does not trigger a push; only a knowledge sync, the flag, or the button does.
*   **Cases that are not plannable.** Cases with status `blocked`, `manual-only` or `wont` are not sent.
*   **playwright-executor, deployment, and the knowledge sync's own behaviour.**

## Decisions
1.  **One-way, idempotent push; GAOS keeps the planning half.** It matches the contract above and needs no state of ours beyond the last outcome.
2.  **Only `planned` and `automated` cases are sent.** `decision` must be `planned`, so a case in `blocked`, `manual-only` or `wont` cannot be expressed. Skipping them is counted as `skipped`, not as a failure.
3.  **Whole-catalog push, not a diff.** The knowledge sync's diff (`kb_syncs.diff`, written at `sync.go:286`) would save requests, but a diff is lost when a push fails, and a full re-send costs at most `ceil(1659/200)` = 9 batches for HISv3 (counts in PRODUCT_KNOWLEDGE_SYNC.md). Idempotency makes the full push the simplest correct design.
4.  **In-process single flight, per product and per token.** GAOS allows one write in flight per token and this instance has one token, so pushes of two products would collide. The service therefore also holds one global push slot: a push waits for it (inside its own budget) before its first request. A mutex-guarded map of running products stays on top for the per-product rules. The backend runs as one container (`deploy/his-v3-qa/docker-compose.prod.yml`), so a DB lock is not needed. A trigger that arrives while a push runs sets a `dirty` flag, and the running push runs once more when it ends, so a catalog change is never lost. The critical section is exactly `lock; if dirty {dirty=false; unlock; run again}; else delete(running); unlock`, so a trigger cannot land on a finished entry. `Run.Result()` is the result of the last pass, which is also what was stored. The deployment is one backend container; a rolling deploy can overlap two for a while, which is harmless because the push is idempotent (accepted limit, no DB lock).
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
`internal/config/config.go`: `ProductsConfig` (`config.go:70-76`) is for the repo pool; a new `GAOSConfig{APIURL, APIToken}` is added to `BackendConfig` (`config.go:50-57`, `yaml:"-"`, env only) and read with `getEnvString("GAOS_API_URL", "")`, `getEnvString("GAOS_API_TOKEN", "")` next to the executor block (`config.go:327-331`). Documented, placeholders only, in `ai-backend/.env.example` and `deploy/his-v3-qa/.env.prod.example`; `README.md:350` env table gets a row. `deploy/his-v3-qa/docker-compose.prod.yml:18` already loads the whole `.env.prod`, so no compose change.

`gaossync.ValidateConfig(url, token)` rules: URL parses; scheme `https`, or `http` only when the host is an IP literal that is loopback or inside `10.10.0.0/24`; no userinfo, no query, no fragment; non-empty host; trailing `/` trimmed. Token: starts with `gaos_`, length at least 37, only printable ASCII without spaces (a header-injection guard). Both empty = "not configured" with no warning; one set and invalid = a logged warning naming the variable, never the value. Signature: `ValidateConfig(url, token string) (baseURL string, err error)`. `localhost` as a name is refused for `http` (IP literals only). The length rule counts the whole token including `gaos_`. The prefix and length rules follow what the GAOS side gave us; if GAOS changes its token format the rule is a one-line change.

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

**Selection (from the DB).** The four reads run in one read-only `REPEATABLE READ` transaction, so a knowledge sync committing meanwhile cannot tear them; it ends before the first HTTP call.
```sql
SELECT code, epic_code, COALESCE(story_code,''), COALESCE(scenario,''), polarity, level, priority, status, spec_paths
  FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL ORDER BY code
SELECT sf.story_code, sf.feature_code, f.module_code FROM kb_story_features sf
  JOIN kb_features f ON f.product_id = sf.product_id AND f.code = sf.feature_code AND f.removed_at IS NULL
  WHERE sf.product_id = ? ORDER BY sf.feature_code
SELECT code, title, service FROM kb_epics WHERE product_id = ? AND removed_at IS NULL
```
(columns at `migrations.go:570-600` and `:602-610`; `spec_paths` is a JSON array scanned into `[]string`; links of any `origin` count; a NULL story has no features.) A case is **sent** only when all of these hold; otherwise it is `skipped` (counted, never sent):
*   `status` is `planned` or `automated`;
*   `code` matches `^TC-[A-Z]+-[0-9]{3}$` (so local `L-` drafts are skipped) and its middle segment equals `epic_code`, which matches `^[A-Z]{2,8}$`;
*   `polarity`, `level`, `priority` are in the GAOS enums.

**Mapping.** `caseId` = code; `epic` = epic code; `features` = the story's feature codes that match the GAOS pattern, in plain string order, truncated to 12 (filter first, then truncate); `module` = the `module_code` of the first of those features when it matches `^[A-D][0-9]{2}$`, else empty; `userStory` = the story code if it matches the pattern, else empty; an empty scenario is sent as is (the contract does not forbid it); `scenario` clipped to 600 bytes on a rune boundary with `...` as the last three bytes of the 600; `polarity`, `level`, `pri` = `priority`; `decision` `planned`, `reason` `""`; `implemented` = status `automated`; `spec` = first of `spec_paths` only when automated and valid (relative, no `..`, at most 240 bytes), else `""`. Automated with no usable spec is sent as `implemented:false`, `spec:""` and counted in the note.

**Epics** are computed once over the whole sent set, never per batch: one entry per distinct epic code of the sent cases, `title` = `kb_epics.title` clipped to 200 runes (GAOS's limit is unknown; the clip is conservative), `service` = `kb_epics.service`, `source` = `qa-automation-tools`, `module` per Decision 8. A sent case whose epic has no active `kb_epics` row is still sent, with `title` = code and `service` = `""`.

**Requests.**
1.  Cases in batches of at most 200 (401 cases make batches of 200, 200, 1), each batch carrying the epic entries its cases use, identical in every batch. Sequential. If this step fails at HTTP level the results step does not run.
2.  Then results in batches of at most 200 for every sent case that GAOS did not reject for a reason other than `exists-differs` (a case rejected as invalid does not exist in GAOS, so a result for it would only be rejected too).
3.  Per-request timeout 30 s; the whole push has a 5 minute budget (its own context, detached from the HTTP request that started it, counted from when it gets the global slot). HISv3 needs 9 + 9 requests, normally seconds; if the budget ends first the push is `failed` with the position reached and the next push starts over (idempotent). Before every request the push re-reads `gaos_integrated` and `gaos_product_id` and stops quietly, recording nothing, when the flag is off or the id is no longer the one it started with, so a correction of the id can never be followed by more writes to the old product. The client uses no proxy (`Transport.Proxy = nil`) so the bearer is never sent through an environment proxy; `GAOS_API_URL` is trusted operator input. Response read capped at 2 MiB; redirects are refused (`CheckRedirect` returns `http.ErrUseLastResponse`, a 3xx then fails the push). Response shape is strict: the required counters must be present and non-negative integers and `rejected` an array of `{caseId, reason}` strings; anything else fails the push. Unknown extra fields are ignored so GAOS can add fields.
4.  HTTP 429: wait `Retry-After` delta-seconds (missing, not an integer or below 1 means 1 s; cap 10 s; the wait also ends with the budget), at most 3 retries per batch (the counter restarts for the next batch); then fail. 401, 403, 404, 503 and any other non-200: stop, status `failed`, note `GAOS answered HTTP <code> on <step>` plus a fixed hint (for example 404 "check the GAOS product id and the token's product scope"). A transport error or timeout fails the push with a fixed text. **The response body, the token and the URL's query are never put in a note or a log.**

**Outcome.** `ok`: no HTTP-level failure and every rejection is `exists-differs` (this includes a run with nothing to send: note `nothing to send`). `partial`: no HTTP-level failure and some other rejection. `failed`: an HTTP-level failure (cases already created stay created; the counts so far are in the note). Rejections of both steps feed `rejected` (every reason except `exists-differs`), `exists_differs` is counted apart, the first 20 in request order form `rejected_sample` with each reason clipped to 80 characters (the dashboard renders it as text). Counters are summed over batches, `cat_head` is the last answer's, clipped to 80 characters. The outcome is stored with a fresh 10 s context and only `WHERE product_id = ? AND gaos_integrated = 1 AND gaos_product_id = <the id the push used>`, so a stale push cannot overwrite a newer state. Nothing is persisted while a push runs, so a restart mid-push leaves the previous outcome. Notes (at most 300 runes, clipped with `...`):

| Case | Note |
|---|---|
| success | `sent N cases (created C, unchanged U, exists-differs E), skipped S, results applied A; rejected R` plus ` ; N automated cases without a usable spec` when non-zero |
| HTTP failure | `<step>: GAOS answered HTTP <code>` plus ` (<hint>)` for 401 `the token was rejected`, 403 `the token is not allowed to write here`, 404 `the GAOS product id is wrong or not enabled for this token`, 503 `GAOS is unavailable`; `<step>` is `cases batch i/n` or `results batch i/n` |
| 429 exhausted | `<step>: GAOS answered HTTP 429 after 3 retries (one write per token at a time)` |
| transport | `<step>: could not reach GAOS` or `no answer from GAOS in time` |
| shape | `<step>: GAOS answered 200 with an unreadable or incomplete body` |
| budget | `time budget reached at <step>` |
| internal | `internal error` (DB read failure, panic; the cause is logged, never stored) |

**Single flight and coalescing.** `Start` returns `ErrBusy` when the product is running (even if only `Trigger` started it). `Trigger` sets `dirty` instead. A panic is recovered and recorded as `failed` (`internal error`).

**Retry of a failed push.** A scheduled knowledge sync that is skipped because the commit is unchanged calls `OnUnchanged`, which pushes only when the product's last outcome is not `ok` (empty, `failed` or `partial`). A flagged product whose push failed is therefore retried at the next schedule tick (6 h) without pushing every product every tick.

### Triggers
*   **After a knowledge sync.** `knowledge.Syncer` gets two exported callbacks, `OnSucceeded` and `OnUnchanged`, both `func(productID int)`, set in `cmd/server/main.go` before `StartScheduler` (`main.go:100`) so no sync can run before they are set. `run` calls it right after the success transaction commits (`sync.go:295`), inside a `recover`, before the AI summary. The callback is `gaosSyncer.Trigger`, which spawns its own goroutine. A failed sync calls nothing. A scheduled sync skipped because the commit is unchanged (`sync.go:230-234`) calls `OnUnchanged` instead (see Retry of a failed push).
*   **Flag turned on, or the GAOS product id changed while on.** `PUT /api/products/{id}` calls `Trigger` after its transaction commits.
*   **Manual.** `POST /api/products/{id}/gaos/sync`.

### Products API (`internal/api/products.go`)
*   `productUpdateBody` (`products.go:620-630`) gains `GaosIntegrated *bool json:"gaos_integrated"` and `GaosProductID *string json:"gaos_product_id"`. The body is decoded strictly (`decodeStrictJSON`, `products.go:528-540`, called at `:635`), so the field names must exist or a client gets 400.
*   Role rule: identical to `knowledge_format` (`products.go:672-682`): a change by anyone but Admin or DevOps (`canToggleWork`, `products.go:260`) is 403 and nothing is saved; sending the current value is not a change.
*   Order of checks, inside the product transaction: (1) a change by a non-Admin/non-DevOps is 403 `only Admin and DevOps can change gaos_integrated` (or `gaos_product_id`); (2) a non-empty `gaos_product_id` must match `^p-[0-9]+$` (at most 40 chars) whether or not the flag is on, else 400 `gaos_product_id must look like p-6`; (3) the resulting state "on" requires a non-empty id (400 `gaos_product_id is required to turn the integration on`) and a configured server (400 `GAOS integration is not configured on this server (GAOS_API_URL and GAOS_API_TOKEN)`). Both fields are written in one `INSERT ... ON DUPLICATE KEY UPDATE` like `knowledge_format` (`products.go:685-686`). Changing the id clears `gaos_last_sync_at`, `gaos_last_status` and `gaos_last_note`; turning off keeps them. With the flag already on, sending only a new id is allowed. After commit, `Trigger` runs when `(!wasOn && nowOn) || (nowOn && idChanged)`. The response is the product, as for every PUT. The request field names are flat (`gaos_integrated`, `gaos_product_id`) while the response nests them (`gaos.integrated`, `gaos.product_id`) on purpose: it matches how `knowledge_format` and the rest of the settings are posted versus how a card reads them.
*   JSON: `models.Product` gains `GAOS GAOSSettings json:"gaos"` = `{configured, integrated, product_id, last_sync_at, last_status, last_note}`; `loadProducts` selects the new columns in the settings query (`products.go:202`); the router fills `configured` just before responding. The list response also carries a top-level `gaos_configured`.
*   `POST /api/products/{id}/gaos/sync` (POST only, else 405; no body): routed beside `knowledge` at `products.go:507` for `parts[1] == "gaos" && parts[2] == "sync" && len(parts) == 3`; checks in this order: 403 unless `canToggleWork`, 400 not configured, 409 `product is not integrated with GAOS`, 409 `a push to GAOS is already running for this product`. It waits for the push up to 280 s (below the 300 s timeout of the dashboard nginx, `nginx.dashboard.conf:38-39`, and the 330 s of the edge, `nginx-edge.conf:66-67`) and answers 200 with the `Result` (a failed push is a 200 with `status:"failed"`); if the push is still running it answers 202 `{"status":"running"}` and the push goes on. A client disconnect does not stop the push. The dashboard re-reads the product after a 202.
*   Wiring: `Router` gets `gaos *gaossync.Syncer` and `SetGAOS`; `main.go` builds `gaossync.New(database, cfg.Backend.GAOS.APIURL, cfg.Backend.GAOS.APIToken)` and sets `syncer.OnSucceeded = g.Trigger`, `syncer.OnUnchanged = g.Retry` after `knowledge.NewSyncer` (`main.go:97`). `configured` is filled by `respondProduct` (`products.go:783`) and by the list handler (`products.go:446-451`), which also adds `gaos_configured` next to `pool_enabled`.

### Dashboard
`dashboard/src/views/products/api.ts` gets the `gaos` fields; `ProductsView.tsx` shows a small `GAOS` badge on integrated products (title = last status and time); `ProductDetail.tsx` gets a "GAOS integration" card (toggle, id input, last sync, "Sync to GAOS now"). The toggle and input are disabled with visible text when the server is not configured or the user is not Admin/DevOps. Patterns follow `dashboard/src/views/products/KnowledgeCell.tsx` and `.claude/rules/dashboard.md`.

## Alternatives considered
| Alternative | Why not |
|---|---|
| Push only the sync diff | A failed push loses the diff; full re-send is idempotent and 9 requests for HISv3 (Decision 3). |
| `knowledge` imports `gaossync` | Couples the parser to an external system; a callback keeps `knowledge` testable offline. |
| A DB lock row for single flight | The deployment is one backend container; a mutex is enough and has no stale-lock failure. |
| Per-product API URL or token | More secrets in the DB; the token is one per QA tools instance and scoped by GAOS. |
| Store push state in its own table or take a DB advisory lock | One backend container; the push is idempotent, so the overlap of a rolling deploy is harmless. `product_settings` holds all that the UI needs. |
| Skip the results step when nothing changed (hash of the last payload) | An optimisation; a full re-send is the safe default. Revisit if traffic matters. |
| Run it from a cron | The owner wants it "automatically" after a sync; the sync already has a schedule. |

## Change list
*   `ai-backend/internal/db/migrations.go`, `skipif_test.go`, `migrations_test.go` (columns, idempotency)
*   `ai-backend/internal/config/config.go`, `.env.example`, `deploy/his-v3-qa/.env.prod.example`, `README.md`, `deploy/his-v3-qa/README.md`
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
4.  Ask the GAOS side for an update or withdraw call, so that demoted, removed or edited cases can follow the catalog.
5.  Should a case whose planning fields later change in the catalog be surfaced somewhere (count of `exists-differs`)? The count is in the result and the note only.

## Review triage (rev 2)
Two zero-context reviewers read rev 1 (design/architecture and implementation readiness). Every finding is resolved above unless listed here.

| Finding | Resolution |
|---|---|
| Wrong anchors (`sync.go:205`, `:269`, `:223-298`, `products.go:432`, `store.go:288`) | Fixed. |
| Push can keep writing to a corrected or switched-off product | Accepted: flag and id re-read before every request; outcome written conditionally. |
| Coalescing race, which result is stored | Accepted: critical section and last-pass rule written down. |
| 429 is per token, single flight per product | Accepted: one global push slot. `Retry-After` stays capped at 10 s (owner's rule). |
| Torn snapshot of four reads | Accepted: one read-only REPEATABLE READ transaction. |
| Manual endpoint and proxy timeouts | Checked: dashboard nginx 300 s, edge 330 s; the 280 s wait stays and a 202 covers the rest. A pure-202 design was rejected: the owner asked for the result summary. |
| A failed push is never retried | Accepted: `OnUnchanged` retry when the last outcome is not `ok`. |
| Proxy env, plain http | Accepted: no proxy. `http` to loopback and 10.10.0.0/24 stays: the owner specified it. Hostname dial checks rejected: the URL is trusted operator input. |
| Budget versus worst case | Documented; position reached is recorded. Hash short-circuit rejected for now (alternatives). |
| Epic fields per batch, missing epic row | Accepted: computed once; fallback title = code. |
| Silent drift (`exists-differs`, demoted cases) | Documented in non-goals and Open question 4; the count is in the note. A history table was rejected (alternatives). |
| Token format guessed | Kept as the owner specified; noted. |
| Dashboard wording, test names | Dashboard text is in the implementation; tests are listed by behaviour above and named in the test files. |
