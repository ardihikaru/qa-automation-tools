# Module checklist export (PDF / Excel / CSV, cached in MinIO)

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Draft rev 3 (2026-11-17). Rev 2 was reviewed by seven Goldfish reviewers (5 Blockers, 17 Majors, 15 Nits). Every finding is resolved in this revision; see "Review resolution". Not approved.
*   **Date:** 2026-11-17
*   **Depends on:** [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (implemented): the `kb_*` tables and the `Syncer`.
*   **Code references** use function names. Line numbers are approximate.
*   **Terms.** *Goldfish*: an EGM reviewer with no conversation history. *app-host*: the host that runs the production Docker Compose stack (see the root `CLAUDE.md`). *Hard rule N*: a numbered rule in the root `CLAUDE.md`. *The plan*: the requester's approved implementation plan, which is not stored in the repo; this spec restates every decision it needs.

## Overview
`SUT-MODULE-CHECKLIST.pdf` (19 pages, A4, 440 features) was built by hand from the SUT module catalogue through a LaTeX file that is not in the repo. This spec lets the dashboard produce a document with the same content on demand, from data that is already in `qa_automation`:

*   the knowledge base page `/products/:id/knowledge` (Modules tab) gets a **Download checklist** button and an options dialog (format, "Add last update to Status column", "Force regenerate");
*   the **backend** renders the file as PDF, Excel (`.xlsx`) or CSV;
*   every generated file is **stored in MinIO**. The next download of the same content is served from MinIO and nothing is regenerated;
*   a new table, `kb_feature_status_history`, records when each feature's release status changed, so the "last update" can be shown.

The markdown catalogue is **not** read at export time. The source is the synced knowledge base: the part, module, sub-module and feature rows (`kb_parts`, `kb_modules`, `kb_submodules`, `kb_features`), read by dedicated queries inside one transaction (see "Snapshot").

## Problem
*   The checklist is produced by hand, so it goes stale as soon as the catalogue changes, and nobody can regenerate it without the LaTeX file.
*   The knowledge base stores only the current release status of a feature (`kb_features.release_status`). The only change marker is `changed_sync_id`, and it moves on **any** content change (`store.go: upsert`, full writes), not only on a status change. So the data cannot answer "since when is this feature PROD?".
*   Re-rendering the document on every click repeats work when nothing changed. The implementer records the measured render time and file sizes in the implementation PR; this spec does not claim a number.

## Goals
1.  A `GET` endpoint that returns the module checklist of a product as PDF, XLSX or CSV, with the same content in all three.
2.  Optional "Status sejak" (status since) information, taken from a new status-change history.
3.  Generated files are cached in MinIO under a key that changes whenever the synced data, the product name, the history data (for the last-update variant) or the template changes. A hit is therefore never older than the data it was built from.
4.  The user can bypass the cache ("Force regenerate").
5.  Concurrent identical requests generate once.
6.  The rest of the app is unaffected when MinIO is not configured or not reachable.
7.  The PDF has an **approximate** layout of `SUT-MODULE-CHECKLIST.pdf`: the same sections and columns, the colours and strings fixed in "PDF layout". Pixel parity is not a goal.

## Non-Goals
*   **Reading the markdown at export time.** The DB is the source.
*   **Browser-side generation (jsPDF or similar).** Decided against; the backend renders.
*   **Exporting epics, stories, test cases, coverage or annotations.** Only parts, modules, sub-modules and features. The Repo, Evidence and Notes columns stay out, as in the current PDF.
*   **Deleting, expiring or listing objects.** v1 never deletes anything in MinIO (see "Accepted limitations").
*   **Per-user or per-role variants of the file.** One file per `(namespace, product, sync, template, product name, history watermark, option, format)`.
*   **Exposing MinIO to browsers** (no presigned URLs). The backend always streams.
*   **A history UI.** The history table is read only by the export in v1.
*   **A new object-storage use for anything else.** The `objectstore` package is generic, but this spec authorises one caller.
*   **`status_as_of` as a date source.** It is the date the catalogue last *confirmed* a status (fixture `testdata/sample-catalog/docs/modules/C-partner-api.md`), not the date it changed, so it is not used.

## Decisions (user, 2026-11-17)
1.  Generation on the backend (PDF, Excel, CSV), not in the browser.
2.  Every generated file is stored in MinIO; later downloads come from MinIO.
3.  The dialog has "Force regenerate (ignore cached file)".
4.  First option: "Add last update to Status column": the date each feature's status last changed.
5.  MinIO is the **staging MinIO that SUT uses**; the cache key includes the latest succeeded sync id; status history is tracked on every sync.
6.  **Date order:** a `sync` history row, then a `backfill` history row, then a dash. A `backfill` date is approximate: `~` in the PDF with a footnote, and the `Sumber tanggal` column (`sync` or `backfill`) in XLSX and CSV. `status_as_of` is **not** used (it is "last confirmed", not "changed").
7.  **Bucket** = the permanent `bucketName` of the staging MinIO credentials, not `tempBucketName`.
8.  **MinIO runtime failure** does not fail the request: the file is generated and streamed without caching, `X-Export-Cache: bypass`, and the error is logged. There is no 502.
9.  **Any reader** may use `force=1` (the 30 s cooldown stays).
10. **Button** on the Modules tab only, outside the `canWrite` guard.
11. **No retention in v1.** `objectstore` has no `List` or `Delete`. Retention is a future item.
12. **Product name** (from the products master, `products.name`) in the PDF header and the filename: `<product>-MODULE-CHECKLIST.<ext>`.
13. **Scope is kept in full** (review finding M15, decided by the human): the history table, all three formats generated together, `force` with the 30 s cooldown, and four new dependencies. Simpler alternatives are recorded in "Alternatives Considered" and were not chosen.
14. **A mandatory `MINIO_KEY_NAMESPACE`** is a segment of every key, and a pre-deploy checklist guards the shared bucket (see "Pre-deploy checklist"). The spec does not wait for infra's answer on a scoped key.

## Design

### API contract
`GET /api/products/{id}/knowledge/exports/modules-checklist?format=pdf|xlsx|csv&last_update=0|1&force=0|1`

Routed in `handleKnowledge` (`ai-backend/internal/api/knowledge.go`) as a new `case seg == "exports" && len(rest) == 2 && rest[1] == "modules-checklist"`. The handler is a function in a new file `ai-backend/internal/api/knowledge_export.go`.

**Check order:** 405, then 400 (parameters), then 404 (product), then 503 (object storage not configured), then 409 (no succeeded sync), then 413 (too large). The existing top-of-function 503 for `r.kb == nil` runs first, as for every knowledge route.

| Aspect | Contract |
|---|---|
| Method | `GET` only; anything else is 405 `method not allowed` (the file's existing wording). |
| Auth | Any authenticated user, like every other knowledge `GET`. `handleKnowledge` gates only writes through `canWrite` (`products.go` passes `canVerify`); this endpoint does **not** call `deny()`. It sits behind the same cookie-session middleware as the rest of `/api`. |
| `format` | Required. Exactly `pdf`, `xlsx` or `csv`. Otherwise 400 `format must be pdf, xlsx or csv`. |
| `last_update`, `force` | Optional, default `0`. A new strict parser `strictBool(req, name) (val bool, ok bool)` accepts exactly `0`, `1`, `true`, `false` (and absent). Any other value is 400 `<name> must be 0, 1, true or false`. The existing lenient `boolParam` (which maps junk to false) is not used here. |
| 200 body | The file bytes. `Content-Type`: `application/pdf`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`, or `text/csv; charset=utf-8`. `Content-Length` is set (the file is buffered). |
| `Content-Disposition` | `attachment; filename="<name>"`, with `<name>` = `<product>-MODULE-CHECKLIST.<ext>`, for example `SUT-MODULE-CHECKLIST.pdf`. `<product>` is `products.name` sanitised for a header: every run of characters outside `[A-Za-z0-9._-]` becomes `-`, leading and trailing `-` are trimmed, and it is cut to 60 characters. An empty result falls back to `product-<id>`. It never contains a quote, `/`, `;`, a space or a control character. Non-ASCII letters are dropped, not percent-encoded, so no `filename*` is needed. |
| `X-Export-Cache` | `hit` (served from MinIO), `miss` (generated now and stored), `regenerated` (`force=1` honoured and stored), or `bypass` (MinIO failed at runtime; generated and streamed without caching). `Cache-Control: no-store`. |
| 404 | Product not found. `Syncer.productExists` returns `knowledge.ErrNotFound`, so the body is `{"error":"not found"}` (existing `writeKnowledgeError` mapping). |
| 409 | No succeeded sync yet: `{"error":"no knowledge base sync has succeeded yet"}`. New error `knowledge.ErrNoSucceededSync`. |
| 413 | More than 20 000 feature rows: `{"error":"catalogue too large to export"}`. New error `knowledge.ErrExportTooLarge`. |
| 503 | Object storage not configured: `{"error":"object storage not configured"}`. Written by the handler when `r.exports == nil`. A runtime MinIO failure is **not** a 503; see `bypass`. |

`writeKnowledgeError` (`knowledge.go`) gets two new cases: `ErrNoSucceededSync` to 409 and `ErrExportTooLarge` to 413. Without them both would fall to the default 500.

**CORS and headers.** In production the dashboard and the API are served from one origin (`/api`), so `Content-Disposition` and `X-Export-Cache` are readable. In local development the dashboard runs at `localhost:6173` against the API, which is cross-origin (`applyCORS` in `router.go`), and those two headers are not exposed. There the client falls back to a generated filename and omits the cache note. No CORS change is made.

The endpoint is a `GET` that writes to MinIO as a cache. This is not a user-data write, and it creates no `kb_*` rows.

### Snapshot
A sync commits entity rows, history rows (new) and `status='succeeded'` in **one transaction**. Verified in `sync.go`: the `s.DB.WithTx` closure runs `SELECT ... FROM products ... FOR UPDATE`, then `apply`, `countActive`, and `UPDATE kb_syncs SET status = 'succeeded', finished_at = UTC_TIMESTAMP(), commit_sha = ?`; an error rolls everything back and the sync is then marked `failed` with a fresh context. The export therefore reads inside **read-only REPEATABLE READ transactions** opened with `DB.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})`. `DB.WithTx` passes `nil` options and cannot be used. The same call is already used in `services/gaossync/plan.go` (`readCatalog`), so the driver honours these options.

The existing `Syncer.Tree`, `list`, `decorate`, `featureStats` and `productExists` all use `s.DB`, not a transaction, so they **cannot** be reused for the snapshot. `Tree` is not changed. The snapshot uses dedicated queries on the `*sql.Tx`:

*   **Latest sync** (the only form allowed; no aggregate mixed with columns):
    `SELECT id, commit_sha, started_at, finished_at FROM kb_syncs WHERE product_id = ? AND status = 'succeeded' ORDER BY id DESC LIMIT 1`. `sql.ErrNoRows` maps to `ErrNoSucceededSync`.
*   **Product name:** `SELECT name FROM products WHERE id = ?`; `sql.ErrNoRows` maps to `ErrNotFound`.
*   **Parts:** `SELECT code, name FROM kb_parts WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL ORDER BY code`.
*   **Modules:** `SELECT code, part_code, name FROM kb_modules WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL ORDER BY code`.
*   **Sub-modules:** `SELECT code, module_code, name FROM kb_submodules WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL ORDER BY code`.
*   **Features:** `SELECT code, module_code, submodule_code, name, release_status FROM kb_features WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL ORDER BY module_code, code`.
*   **History:** `SELECT feature_code, release_status, source, MAX(changed_at) FROM kb_feature_status_history WHERE product_id = ? GROUP BY feature_code, release_status, source` (only when `last_update=1`).

Every `ORDER BY` ends with `code`, so output order does not depend on collation or insertion order. Rows in the output keep this order: parts by code, modules by code within their part, features by code within their module, and the sub-module cell is a column, not a grouping.

**Orphan rule.** A feature whose module is not an active synced module, and a module whose part is not an active synced part, are **excluded** from the file. The snapshot records the count in `Skipped`, and the generator logs one line (`knowledge export: skipped N orphan rows, product=<id>`). A feature with no sub-module, or a sub-module that is not active, gets an empty sub-module cell and is kept. `origin='sync'` is required on all four tables (`kb_features.origin` allows `'local'`; local creation through the API covers stories, test cases and epics only, so this filter is a guard, and it is intentionally stricter than `Tree`). The row-count test compares against this rule (active features whose module and part are active).

### Cache key
```
qa-automation/{MINIO_KEY_NAMESPACE}/knowledge-exports/{productId}/modules-checklist/sync-{syncId}/v{TEMPLATE_VERSION}/n{nameHash}/{plain|last-update-h{historyMaxId}}.{pdf|xlsx|csv}
```

*   `qa-automation/` is the fixed prefix inside the `objectstore` package.
*   **`MINIO_KEY_NAMESPACE`** is mandatory, matches `^[a-z0-9][a-z0-9-]{0,31}$`, and separates environments that share one bucket (production `prod`, a dev backend something else). Keys are built from per-database integers, so two backends on one bucket without a namespace would overwrite and serve each other's files. If it is empty or invalid, object storage counts as **not configured** (503).
*   `productId`, `syncId` and `historyMaxId` are integers read from the DB. `nameHash` is the first 8 hex characters of the SHA-1 of the sanitised product name, so a product rename gives a new key (the name is embedded in the PDF header and the filename). `historyMaxId` is `COALESCE(MAX(id), 0)` of `kb_feature_status_history` for the product, in the `last-update` variant only, so a re-run of the backfill (which can add rows) gives a new key. The variant and the extension come from a closed whitelist.
*   `TEMPLATE_VERSION` is a Go constant in `export_checklist.go`. **Raise it on any change that alters the output** of any renderer (layout, columns, colours, wording, time zone). A golden-hash test (see Verification) fails when the output changes without a version bump, including a library upgrade.
*   A new succeeded sync, a rename, a template change or new history rows give a new key. A status change always arrives through a sync, so the sync id covers it.
*   Verified in `sync.go`: only a **scheduled** sync whose `(repo, branch, format, parser version, commit)` equals the last succeeded one is finished as `skipped` (it never becomes `succeeded`, so the key is unchanged). A **manual** sync of the same commit is not skipped: it runs `apply`, is committed as `succeeded` with `diff.unchanged = true`, and changes the key although the content is identical. A scheduled sync after a parser-version bump also proceeds. This costs one extra generation and is accepted.
*   The key does **not** include local annotations or drafts (not exported). If a later amendment exports them, the key rule must change.

### Request flow
`ExportCache` (new, `ai-backend/internal/services/knowledge/export_cache.go`, owned by the `knowledge` package, which imports `objectstore`) holds the orchestration:

```go
type ExportCache struct {
    store   objectstore.Store          // never nil
    ns      string                     // MINIO_KEY_NAMESPACE
    src     snapshotSource             // implemented by *Syncer; faked in tests
    now     func() time.Time           // injected clock (cooldown)
    render  func(*ChecklistSnapshot, string, bool) ([]byte, string, error) // injected renderer (tests count calls)
    timeout time.Duration              // MINIO_TIMEOUT_SECONDS
    flights singleflight.Group
    bg      sync.WaitGroup             // background Puts; Wait() for tests and shutdown
}
func NewExportCache(src snapshotSource, store objectstore.Store, ns string, timeout time.Duration) *ExportCache
func (c *ExportCache) Serve(ctx context.Context, pid int, q ExportQuery) (ExportResult, error)
```

`ExportQuery{Format string; LastUpdate, Force bool}`; `ExportResult{Body []byte; ContentType, Filename, Cache string}`. `snapshotSource` has two methods: `ChecklistKeyInfo(ctx, pid, withHistory bool) (KeyInfo, error)` (one small read-only transaction: latest sync, product name, `historyMaxId`) and `ChecklistSnapshot(ctx, pid, withHistory bool) (*ChecklistSnapshot, error)` (the full read). The handler tests use a fake `snapshotSource`, so they need no database.

Wiring: `knowledge_export.go` has `func (r *Router) SetExportCache(c *knowledge.ExportCache) { r.exports = c }` and `router.go` gains the field `exports *knowledge.ExportCache`. In `cmd/server/main.go`, `objectstore.New(cfg)` returns `(*MinioStore, error)`. Only when it returns no error does `main` build `ExportCache` and call `SetExportCache`; an unconfigured or invalid config leaves the field nil. The interface is never assigned a typed nil pointer.

```
Serve(pid, q):
 1. key info = ChecklistKeyInfo (light read)            -> ErrNotFound / ErrNoSucceededSync
 2. key = build(info, q)
 3. force=0:  Get(key). found -> hit. not found -> go to 5. error -> log, go to 5 with skipPuts.
    force=1:  Stat(key) only for the cooldown. found and age < 30 s -> Get, hit.
              missing or error -> go to 5.
 4. (hit) return
 5. flights.Do(flightKey) {                              flightKey = key without extension, plus ":force" when force=1
      full snapshot (one RO tx; re-reads the latest sync id in that transaction)
      if the sync id differs from the key info, rebuild the key from the snapshot (it is the newer one)
      render pdf, xlsx and csv for q.LastUpdate from the SAME snapshot
      unless skipPuts: Put(requested key) ; on error -> log, result = bypass
    }
 6. after the flight returns and the response is ready: if the requested Put succeeded,
    Put the other two formats in a goroutine (bg), under ONE deadline of 2 x timeout; errors are logged.
 7. return the requested bytes with Cache = miss | regenerated | bypass
```

*   **Light read first.** A `hit` costs one small DB read (latest sync, product name, `historyMaxId`) and one MinIO `Get`; the tree and history are read only on a miss. Because the full read happens in its own transaction, it re-reads the latest sync id and uses that id for the stored key, so the stored object always matches its key.
*   **One round trip on the normal path.** `Get` returns `(data, found, err)`; `NoSuchKey` is `found=false`, not an error. `Stat` is used only for the force cooldown, because it needs `LastModified`. If a `Get` fails after the object was found, the request is treated as a `Get` failure (log, generate, `bypass`, no `Put`s).
*   **Three formats per generation.** One snapshot is rendered as PDF, XLSX and CSV for the requested `last_update` value, and all three are stored, so the next click in another format is a hit. The other `last_update` value is not pre-generated. Each object is looked up on its own, so a partially populated directory is harmless.
*   **Put order and failure.** The requested format is stored first, inside the flight. If that `Put` fails, the error is logged, the other two are skipped, and the bytes are streamed with `bypass`. A failed `Get` or `Stat` skips every `Put` (to avoid a second timeout). The two extra `Put`s run after the response is ready, in a goroutine under one 2 x `timeout` deadline, so a slow MinIO cannot delay the user. A MinIO failure never produces an error response; only the renderer or the DB can.
*   **Latency bound.** At most two MinIO timeouts can be added to one request (one failed or slow `Get`/`Stat`, or one slow `Put` of the requested object), plus generation. The generation has its own 60 s ceiling.
*   **Singleflight** (`golang.org/x/sync/singleflight`, new direct dependency). The flight key is the object directory without the extension plus `:force` for forced requests, so concurrent clicks in different formats share one generation, and a forced request never joins an unforced flight (and so never reports the wrong `X-Export-Cache`). The function runs under `context.WithoutCancel(req.Context())` with its own timeouts, so one client closing its tab does not fail the others. The flight returns the bytes of all three formats; each waiter picks its own.
*   **`force=1`.** It does a `Stat` of the **requested** key only. If the object exists and `now - LastModified` is under 30 s, the force is ignored and the stored object is served as `hit`. A negative age (clock skew between MinIO and the backend) counts as 0, so it is also under the cooldown. In every other case (missing, older, or `Stat` failed) it regenerates and **overwrites** the objects (`Put` overwrites) and answers `regenerated`. Because the key is content-addressed and rendering is deterministic, forcing normally produces the same content; it is there for a corrupted or hand-deleted object. The 30 s constant is not configuration. This applies to every reader (decision 9).
*   **Bypass log line.** Each `bypass` logs `knowledge export: cache bypass product=<id> op=<stat|get|put> code=<minio error code>`. There is no metrics system in the backend, so the log line is the signal.
*   **No other request-path write.** The handler never lists or deletes, and never reads outside its own key.

### Status history table
Appended to `ai-backend/internal/db/migrations.go` as idempotent statements (hard rule 3). The server never migrates itself; run `go run -mod=mod ./cmd/databasemigration` before the binary is deployed, and once more right after (see "Pre-deploy checklist").

```sql
CREATE TABLE IF NOT EXISTS kb_feature_status_history (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  feature_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  release_status VARCHAR(20) NOT NULL,
  prev_status VARCHAR(20) NULL,
  sync_id INT NULL,
  commit_sha CHAR(40) NULL,
  changed_at DATETIME NOT NULL,
  source ENUM('sync','backfill') NOT NULL,
  CONSTRAINT fk_kb_fsh_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  INDEX idx_kb_fsh_feature (product_id, feature_code, changed_at),
  UNIQUE KEY uq_kb_fsh_event (product_id, feature_code, sync_id, source)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

*   `feature_code` has the same collation as `kb_features.code` (`utf8mb4_bin`) so joins by code are exact. There is deliberately **no FK** to `kb_features` (features are soft-deleted, and codes are the link, as everywhere in the knowledge base).
*   **Idempotence.** The unique key `(product_id, feature_code, sync_id, source)` makes a re-executed sync write idempotent. Writes use only `INSERT ... ON DUPLICATE KEY UPDATE id = id`; `INSERT IGNORE` is **not** used, because it also hides truncation and FK errors. MySQL treats NULLs in a unique key as distinct, so the key does not protect rows with a NULL `sync_id`; **backfill idempotence is carried by its `NOT EXISTS` guard**, not by the key (in fact backfill rows always have a `sync_id`, see below).
*   `prev_status` is NULL for a feature's first row.

**Sync-time writes** (`store.go`, `apply`). `apply` gains two parameters: `sha string` (the commit the sync read) and `firstSync bool` (true when the product has no earlier succeeded sync, `!hasPrev` in `sync.go`). The sole caller is `sync.go` (the `WithTx` closure; `sha` and `hasPrev` are in scope there). No test calls `apply` directly (checked by search), so only `sync.go` changes with the signature.

For the feature table only, the existing `SELECT code, origin, content_hash, removed_at IS NOT NULL, source_file, source_line` also reads `release_status` (other tables select `'' AS release_status`), and `existing` gets a `status string` field. The new status is `r.vals[4]` of the feature row (`vals` order is `module_code, submodule_code, name, repo_entry, release_status, ...`, see `rowsOf`). After the existing add/restore/update decision, for `et == TypeFeature`, history rows are collected and inserted **in the same transaction**, batched like `upsert` (`batchSize`):

| Case | Row |
|---|---|
| feature added (`!ok`), not a first sync | `release_status = new`, `prev_status = NULL`, `source = 'sync'` |
| feature added in a **first sync** of the product | `release_status = new`, `prev_status = NULL`, **`source = 'backfill'`** (the first observation is not a change date) |
| feature restored (`e.removed`) and the status **differs** from the stored one | `release_status = new`, `prev_status = old`, `source = 'sync'` |
| feature restored and the status is **the same** | none (nothing changed; the earlier row for that status keeps its date) |
| feature updated and `release_status` differs from the stored value | `release_status = new`, `prev_status = old`, `source = 'sync'` |
| any other update (same status, including a parser-version bump) | none |
| removed | none (a removal is not a status; the feature is not exported) |

`sync_id` = this sync, `commit_sha` = `sha`, `changed_at = UTC_TIMESTAMP()`. `sha` is passed in because `kb_syncs.commit_sha` is written only at the end of the transaction; reading it back inside `apply` would return NULL. Because the history rows, the entity rows and `status='succeeded'` commit together, the history can never disagree with the entity tables, and a failed sync leaves no history.

**Backfill** (a migration statement appended after the `CREATE TABLE`; it gives existing features a starting date):

```sql
INSERT INTO kb_feature_status_history
  (product_id, feature_code, release_status, prev_status, sync_id, commit_sha, changed_at, source)
SELECT f.product_id, f.code, f.release_status, NULL, f.changed_sync_id, s.commit_sha, s.started_at, 'backfill'
FROM kb_features f
JOIN kb_syncs s ON s.id = f.changed_sync_id AND s.product_id = f.product_id
WHERE f.origin = 'sync' AND f.removed_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM kb_feature_status_history h
                  WHERE h.product_id = f.product_id AND h.feature_code = f.code
                    AND h.release_status = f.release_status)
ON DUPLICATE KEY UPDATE id = id;
```

*   **`NOT EXISTS` is the sole guard.** The statement has no `-- skip-if:` line (that mechanism takes one integer query outside a transaction). The condition "no row **with the current status**" makes a second run a no-op and also repairs the deploy window below. `internal/db/skipif_test.go` (`TestGuardedMigrationsAreKnown`) pins the list of guarded statements; this one is unguarded, so the test needs no new entry, but it must still pass, and `migrations_test.go` is run (and extended if it enumerates statements).
*   `kb_features.changed_sync_id` is set on every synced row (`upsert` writes it on insert and on every full write), so the inner join covers every active synced feature whose sync row still exists. A feature without a matching sync row is not backfilled and shows a dash. There is no `COALESCE` and no `updated_at` fallback (`updated_at` also moves on the location-only refresh, so it is not a date of anything we care about).
*   **The date is approximate by construction.** Verified in `store.go` `upsert`: `changed_sync_id` is set on every full write (insert, restore, content-hash change, so any change of evidence, notes or name) and is **not** touched by the location-only refresh (`full=false`). The backfilled date is `kb_syncs.started_at` of the sync of the feature's last full write; it can be later than the real status change, and never earlier than the last content change (a status change changes the content hash, so it always is a full write). It is the sync's start time, not the commit's date.
*   **Deploy window.** The migration runs before the new binary. If the old binary changes a feature's status in between, `kb_features` holds the new status and the backfill row holds the old one, and no sync row is written for it (the new binary compares against the already-updated stored value). The status-aware `NOT EXISTS` lets the **second migration run right after deploy** insert a `backfill` row with the new status, so no feature is left without a date.
*   A feature tagged only `GIT:main` has no real date until a sync sees a change; it shows its approximate backfill date at first.

**What the export reads (date order).** For each exported feature, the "since" date is the first of these that yields a date, with its **source**:
1.  **`sync`:** `MAX(changed_at)` over the feature's history rows with `source = 'sync'` and `release_status` equal to the feature's **current** status. Exact to the sync that observed the change.
2.  **`backfill`:** `MAX(changed_at)` over its rows with `source = 'backfill'` and the current status. Approximate.
3.  Otherwise: a dash.

The order is one pure function, `pickStatusDate(syncAt, backfillAt *time.Time) (at *time.Time, source string)`, so it is table-testable. `backfill` is marked approximate in every format (below).

### Export service: package `ai-backend/internal/services/knowledge`
New files `export_checklist.go` (snapshot types, `TEMPLATE_VERSION`, `pickStatusDate`, `RenderChecklist`), `export_cache.go` (above), `export_pdf.go`, `export_xlsx.go`, `export_csv.go`. New errors: `ErrNoSucceededSync` and `ErrExportTooLarge`.

*   `type ChecklistSnapshot struct { ProductName string; SyncID int64; CommitSHA string; SyncedAt time.Time; Skipped int; Parts []ChecklistPart }`; a part holds modules, a module holds rows `{ID, Submodule, Feature, Status, StatusSince *time.Time, DateSource string}`.
*   `func (s *Syncer) ChecklistSnapshot(ctx, pid, withHistory bool) (*ChecklistSnapshot, error)` and `ChecklistKeyInfo`, as described in "Snapshot". More than 20 000 feature rows returns `ErrExportTooLarge`.
*   `func RenderChecklist(snap *ChecklistSnapshot, format string, withLastUpdate bool) ([]byte, string, error)`.
*   Rendering is **deterministic**: it uses `snap.SyncedAt` (the sync's `finished_at`), never `time.Now()`. The PDF sets a fixed creation date from `SyncedAt` and disables compression and random IDs where the library allows it, so regenerated output is the same.
*   **Time zone.** Every date shown (PDF header, "sejak", XLSX, CSV) is converted from UTC to **Asia/Jakarta, UTC+7**, through `time.FixedZone("WIB", 7*3600)` (no tzdata dependency). Dates are shown as dates only. Indonesian month abbreviations in the PDF: `Jan Feb Mar Apr Mei Jun Jul Agu Sep Okt Nov Des`.

**PDF layout (approximate).** `github.com/go-pdf/fpdf`, core Helvetica; Latin text and the em dash are covered by the cp1252 translator (`UnicodeTranslatorFromDescriptor("")`). Any character outside cp1252 is replaced with `?` explicitly, and a test covers it.
*   A4 portrait, 9 pt, 14 mm margins (182 mm usable). Header: `<product name> — Checklist Katalog Modul` and the sync date (`<d> <Mon> <yyyy>`). The product name is `products.name` through the translator, cut to 60 characters. Footer: `Halaman <n> dari <N>`.
*   Title in teal `#007E82`. Status legend, total summary table, table of contents with page numbers (fpdf aliases or a two-pass render), per-part summary, then one table per module.
*   Per-module table columns and widths: `ID` 18 mm, `Sub-modul` 38 mm, `Fitur` 80 mm, `Status` 28 mm, `Cek` 18 mm (an empty square). The header row is teal `#007E82` with white bold text and repeats on every page.
*   Status text is bold and coloured, no emoji. Proposed colours (not taken from the reference PDF, which was not available to the author): `PROD` `#1B7F3B`, `DEV-DONE` `#1F5FBF`, `IN-PROGRESS` `#C46A00`, `NOT-STARTED` `#6B7280`, `UNKNOWN` `#9CA3AF`. A status outside the five uses `UNKNOWN`'s colour.
*   Legend lines (Indonesian): the five statuses in the colours above, then, when `last_update=1`, `~ = tanggal perkiraan (dari backfill, sebelum pelacakan perubahan status dimulai)`.
*   With `last_update=1` the status cell has a second line, for example `PROD` and `sejak 01 Sep 2026`. A `backfill` date is prefixed with `~` (`sejak ~01 Sep 2026`) and the legend footnote explains `~`. No date shows `—`.

**XLSX** (`github.com/xuri/excelize/v2`): a `Ringkasan` sheet and one sheet per part (names clipped to 31 characters and stripped of `[]:*?/\`); columns `ID | Sub-modul | Fitur | Status | Cek`; status cell colouring as above; autofilter and a frozen header row. With the option, an extra `Status sejak` column holding a real date cell (`yyyy-mm-dd`, so Excel can sort it) and a `Sumber tanggal` column with `sync` or `backfill` (empty when there is no date).

**CSV** (`encoding/csv`): UTF-8 with a BOM so Excel opens it correctly; the same columns; one header row. With the option, `Status sejak` as `YYYY-MM-DD` (empty when unknown, because `—` is noise for filters) and `Sumber tanggal` with `sync`, `backfill` or empty.

**Formula injection.** Feature and sub-module names come from a git repo and could begin with `=`, `+`, `-` or `@`. In CSV and XLSX a text cell that starts with one of `= + - @ \t \r` is prefixed with `'` (CSV) or written as an explicit string cell (XLSX). Status and date columns are generated by the code and need no prefixing. The test covers this.

### MinIO configuration and the `objectstore` package
New package `ai-backend/internal/services/objectstore`:

```go
type Store interface {
    Stat(ctx context.Context, key string) (Info, bool, error)   // Info{Size, LastModified}; bool = exists
    Get(ctx context.Context, key string) ([]byte, bool, error)  // bool = found; NoSuchKey is (nil, false, nil)
    Put(ctx context.Context, key, contentType string, data []byte) error
}
func New(cfg config.ObjectStoreConfig) (*MinioStore, error)     // validates; nil store + error when unconfigured or invalid
```
*   MinIO implementation with `github.com/minio/minio-go/v7`; the client is an **unexported** field. A fake in-memory `Store` lives in the test files only.
*   **Prefix enforcement is in the package, not in the callers.** The constructor takes the fixed prefix `qa-automation/` and refuses to build if it is empty or does not end in `/`. Every method rejects, before any network call, a key that: does not start with the prefix, has an empty remainder, contains an empty segment (`//`), a `.` or `..` segment, a backslash, or a control character, or begins with `/`. There is **no** `List` and **no** `Delete`, so a bug cannot list or delete objects that belong to SUT in the shared bucket.
*   `Get` is capped (the export objects are small; cap 64 MiB) and returns an error beyond the cap.

**Environment variable names** (names only; values never appear in the repo, a log, an error, a response or this spec):

| Name | Meaning |
|---|---|
| `MINIO_ENDPOINT` | `host:port`, no scheme (minio-go form) |
| `MINIO_ACCESS_KEY` | secret |
| `MINIO_SECRET_KEY` | secret |
| `MINIO_BUCKET` | bucket name |
| `MINIO_KEY_NAMESPACE` | mandatory key segment, `^[a-z0-9][a-z0-9-]{0,31}$` |
| `MINIO_USE_SSL` | `true`/`false` |
| `MINIO_TIMEOUT_SECONDS` | per-call timeout, default 30 |

*   Read in `ai-backend/internal/config/config.go` (a new `ObjectStoreConfig` next to `ProductsConfig`), built in `ai-backend/cmd/server/main.go`. **Configured** means `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET` and a valid `MINIO_KEY_NAMESPACE` are all set. Otherwise nothing is built, nothing connects at startup, and the export answers 503. A MinIO outage at startup does not stop the server: the connection is lazy, on first use.
*   **Transport.** `MINIO_USE_SSL=false` would send the access key in clear text. It is accepted only when the endpoint host is an IP inside `10.8.0.0/24` (the WireGuard network); otherwise the config is invalid, the export is treated as not configured, and startup logs the reason (no values).
*   **Secrets handling.** `ObjectStoreConfig` has `String()` and `GoString()` on a **value** receiver that print `***` for both secrets, so `%v`, `%+v` and `%#v` cannot leak them. Both secret values are appended to the `Secrets` list in `cmd/server/main.go` (the existing literal-secret redactor list passed to the case-automation redactor). minio-go errors are logged as **operation and error code only** (`minio.ToErrorResponse(err).Code`, or the error's Go type), never the message, which can contain the host, bucket, key and request id. Startup logs only `object storage: configured` or `not configured`. A redaction test formats the config with `%v`, `%+v` and `%#v` and asserts no secret appears.
*   The names (no values) are added to `ai-backend/.env.example` and `deploy/qa-tool/.env.prod.example`. `docker-compose.prod.yml` uses `env_file: .env.prod`, so no compose change is expected; confirm at implementation that no explicit allow-list exists.
*   The values come from the staging MinIO credentials file of SUT (a JSON file under `~/.config/minio/`). They are loaded **into the process environment or a secret file only**, never printed (hard rule 1). `.claude/rules/secrets.md` is extended to name `~/.config/minio/*`. `MINIO_BUCKET` is the permanent `bucketName` (decision 7). The file's other keys (`tempBucketName`, `shareDuration`, `timeout`) are not used; the temporary bucket is avoided because it may carry a lifecycle rule that empties it (not verified).

**Cross-environment dependency (explicit).** The **production** qa-tool (`qa-tool.mfardiansyah.id`, app-host) will read and write the **staging** MinIO that SUT itself uses, with the same bucket and, today, the same access key.
*   The credential is shared with SUT, so it can read and delete SUT objects. In-process protection (fixed prefix, no list, no delete, key validation) is the only layer until infra provides a scoped credential. This is why "Pre-deploy checklist" lists a scoped key or a dedicated bucket as the **requested** end state. It does not block this spec.
*   A staging MinIO outage or reset takes the export cache down or wipes it. The export still works (`bypass`), and a wiped cache regenerates on demand.
*   Rotating the staging keys in SUT breaks the cache (not the export) until `.env.prod` on app-host is updated.
*   Only the `qa-automation/` prefix is ours. We never create buckets, never list the bucket, never write outside the prefix. `Put` and `force=1` overwrite silently, so the prefix must be unused by anyone else (checked before deploy).
*   Bucket versioning and lifecycle rules are unknown; they are part of the checklist.

### Failure modes
| Situation | Behaviour |
|---|---|
| `MINIO_*` unset, incomplete, or invalid (namespace, plaintext off the WireGuard net) | Export endpoint 503 `object storage not configured`. Everything else works. No startup failure. |
| `r.kb == nil` | 503 `knowledge base not available` (existing). |
| Product not found | 404. |
| Product has no succeeded sync | 409. Nothing is generated or written. |
| More than 20 000 feature rows | 413. |
| MinIO `Get`/`Stat` fails (network, auth, timeout), or a found object cannot be read | Error code logged; the file is generated and streamed, `bypass`, nothing stored. |
| `Put` of the requested format fails | Logged; the generated bytes are streamed, `bypass`; the other two formats are not written. |
| `Put` of the other two formats fails | Logged, ignored. The next request for that format regenerates it. |
| Two concurrent clicks, same variant | Singleflight: one generation; both get the file. A forced request has its own flight. |
| Two backend replicas | Singleflight is in-process, so two replicas could each generate and overwrite the same key with identical content. The stack runs one backend container today. |
| A sync commits while a generation runs | The full read is one REPEATABLE READ transaction and re-reads the sync id; the object is stored under the sync id it was built from. The next request sees the new key. |
| Client disconnects | The flight continues (detached context) and the objects are stored. |
| Renderer error (bug) | 500 `internal error`, logged by `writeKnowledgeError`'s default case; nothing is stored. |
| Orphan features (module or part missing) | Excluded, counted and logged (see "Snapshot"). |

### Security
*   **Authentication/authorisation:** any authenticated reader, the same as the other knowledge `GET`s. Auth is active only when the DB connects (hard rule 4); the export needs the DB anyway. The data exported (module and feature names and release status) is what any logged-in user already sees on the Modules tab.
*   **No new write surface on user data.** The only side effect is writing cache objects under `qa-automation/{namespace}/knowledge-exports/`.
*   **No secrets in the output.** The files contain only catalogue fields (id, sub-module, name, status, date). They exclude Repo, Evidence and Notes. The filename is built from the sanitised product name only.
*   **No secrets in logs, errors or responses.** See "Secrets handling" above: value-receiver `String`/`GoString`, redactor list, error code only, fixed client messages.
*   **Path safety.** Keys are built only from integers, a hash, a validated namespace and a closed whitelist, and `objectstore` re-validates them before any call.
*   **Force abuse.** Any reader may use `force=1` (decision 9). The 30 s cooldown plus singleflight bound the work an authenticated reader can cause. While MinIO is down the cooldown cannot be evaluated (no `LastModified`), so every request generates; singleflight still collapses concurrent ones. No per-user rate limiter is added.
*   **Spreadsheet injection** is handled in the CSV/XLSX section.
*   **Rule 6 (no new public ports):** no port is published and no new service is added; the backend makes an outbound connection to the MinIO endpoint only.
*   **Rule 2 (LLM only through ai-manager):** this feature makes no LLM call.

### Dashboard
*   `dashboard/src/views/knowledge/KnowledgeView.tsx`: add a `btn btn-secondary` button, a `Download` lucide icon plus "Download checklist", in the header next to the draft buttons. The draft buttons sit inside `{canWrite && summary.configured && (...)}`. The new button must be **outside the `canWrite` guard**, so read-only roles see it, and is shown only when `summary.configured` **and the derived `tab === 'modules'`**. The `tab` value defaults to `modules` when `?tab` is absent (`KnowledgeView.tsx`), so the condition uses the derived value, not the raw query string. It opens the dialog.
*   New `dashboard/src/views/knowledge/ChecklistDialog.tsx`, built on `components/Modal.tsx` and `components/SubmitButton.tsx`:
    *   Format: radio group, PDF (default), Excel, CSV.
    *   Checkbox "Add last update to Status column"; helper text: "Dates marked ~ are approximate: they come from the first tracking of this product, not from the exact change."
    *   Checkbox "Force regenerate (ignore cached file)".
    *   Submit "Download" with a loading state and an inline error (the 409, 413 and 503 messages are shown as the server wrote them). After success, a small note from `X-Export-Cache`: "Served from cache" (`hit`), "Generated now" (`miss`, `regenerated`), or "Generated now; the cache is unavailable" (`bypass`). If the header is unreadable (local development), no note is shown.
*   `dashboard/src/views/knowledge/api.ts`: `downloadChecklist(pid, opts)`: `fetch(url, { credentials: 'include' })`, read the blob, take the filename from `Content-Disposition` (falling back to a generated name), and trigger the save through a temporary object URL that is revoked afterwards. No new npm dependency. A non-2xx response is parsed as `{error}`.
*   `dashboard/README.md`: add the new API path to the table (`views/knowledge/api.ts`) and a sentence on the dialog.

## Alternatives Considered
| Alternative | Why not chosen |
|---|---|
| **Browser-side PDF (jsPDF) from the existing `tree` API** | Decided against by the human (Decision 1): three formats, caching and the history need the backend; layout fidelity and large catalogues are harder in the browser. |
| **No cache** (render on every click) | Rejected by the human (Decision 2). It would also be simplest; it is kept as a fallback: `bypass` is exactly this path, so the feature degrades to it when MinIO fails. |
| **Cache blobs in MySQL** (a table keyed like the object) | Avoids the cross-environment dependency, but grows the shared db-host schema with binary data; the human chose MinIO. Remains the fallback if the shared-bucket risk is judged too high. |
| **Dedicated bucket or a prefix-scoped access key** | The right end state for the shared-credential risk, but needs infra work that this spec cannot do. Listed as a pre-deploy request, not a blocker (Decision 14). |
| **LaTeX or headless-browser rendering on the server** | Needs a TeX toolchain or Chromium in the backend image (hard rule 9 keeps browsers out of it); `fpdf` is pure Go and `CGO_ENABLED=0` builds keep working. |
| **A `status_changed_at` column on `kb_features` instead of a history table** | Smaller (no new table), but only records the last change, loses `prev_status` and a per-sync trail, and still needs a backfill. The human kept the history table (Decision 13). |
| **Render only the requested format; drop `force`/cooldown; ship CSV and PDF first** | Smaller and cheaper, but contradicts Decisions 1 to 3 and 9. Kept in full by the human (Decision 13). |
| **`status_as_of` as a date source** | It means "last confirmed" in the SUT catalogue, so it would label a re-confirmed status as newly changed. Dropped (Decision 6). |

## Risks & Mitigations
| Risk | Mitigation |
|---|---|
| Production holds a credential that can delete SUT's objects | No `List`/`Delete` in `objectstore`, fixed prefix, key validation, namespace; pre-deploy request for a scoped key or dedicated bucket. |
| Our prefix is already in use, so a `Put` overwrites foreign data | Pre-deploy prefix-empty check by an operator. |
| Dev and prod backends share a bucket and collide | Mandatory `MINIO_KEY_NAMESPACE`; export refuses without it. |
| Stale or wrong file served | Key includes sync id, template version, product-name hash and (last-update variant) history watermark; golden-hash test forces a version bump on output changes. |
| Secret leakage through logs or `%v` | Value-receiver `String`/`GoString`, redactor list, error code only, redaction test. |
| New dependencies need a newer Go than 1.23.8 | Pinned to versions that build on `go 1.23` (see Target Files), verified by a build in the `golang:1.23-alpine` image with `CGO_ENABLED=0`; if a transitive module forces a bump, that is a separate approval, not a silent Dockerfile change. |
| History table drifts from entity tables | Written in the same transaction; DB tests cover add, restore, change, parser bump. |
| Wrong "since" dates | `backfill` dates are approximate and marked; `sync` dates are exact to the sync, not the commit (Accepted limitations). |
| MinIO down or slow | `bypass` path; at most two timeouts of added latency; extra `Put`s run after the response. |
| Abuse through `force=1` | 30 s cooldown, singleflight, authenticated readers only. |
| Deploy window leaves a feature without a date | Migration run again right after deploy (status-aware `NOT EXISTS`). |
| Cache growth in a shared bucket | Accepted for v1 (a few files per sync); retention is a future item (needs `List`/`Delete`). |

## Accepted limitations
1.  **`backfill` dates are approximate** (see "Backfill"). `sync` dates are exact to the sync that observed the change, not to the commit that made it (a sync every 6 h can merge several commits).
2.  **Cache growth.** Each succeeded sync creates up to 6 objects per product (3 formats x 2 option variants, if both are used) under a new key, and nothing deletes the old ones (decision 11). **Future item:** retention restricted to `qa-automation/{namespace}/knowledge-exports/`, which needs `List` and `Delete` on `objectstore` and a separate amendment.
3.  **A manual sync of an unchanged commit invalidates the cache** for the product (new succeeded sync id, identical content).
4.  **Helvetica/cp1252 only.** Text outside cp1252 is rendered as `?` in the PDF (not in XLSX/CSV). A product with non-Latin text would need a bundled TTF font (future item). The SUT catalogue is Latin (Indonesian) with em dashes.
5.  **Shared staging MinIO** (see "Cross-environment dependency"). While it is down, every export is generated on the fly (`bypass`).
6.  `force=1` is ignored within 30 s of the last write of that object.
7.  A failing MinIO is visible only in logs and in the `X-Export-Cache: bypass` header.
8.  Orphan rows (features whose module or part is not active) are not exported.

## Open Questions
None. Future items: retention of old cache objects (needs `List`/`Delete`); a bundled TTF font for non-Latin product names; a scoped MinIO credential or dedicated bucket (infra).

## Pre-deploy checklist
Deploy is a separate approval (hard rule 8). None of these items blocks approving or implementing the spec; items 1 to 3 gate the **production deploy** only.
1.  **Ask infra** for a MinIO access key whose policy allows only get/put/stat on `<bucket>/qa-automation/*`, or a dedicated bucket. Record the answer. Until then the shared key is used and the risk above is accepted.
2.  **Prefix-empty check.** An operator, with the credentials loaded into their own shell (never printed, never in this repo), counts objects under `qa-automation/` with an S3 client (`mc ls --recursive` or equivalent). The count must be 0 (or contain only this feature's earlier test objects). This is done outside the app, because `objectstore` has no `List`. Record bucket versioning and lifecycle rules at the same time.
3.  Confirm the endpoint is reachable **from inside the backend container** on app-host (Docker network, not only the host), and whether it speaks TLS or only plain HTTP over the WireGuard network (`MINIO_USE_SSL`).
4.  Choose `MINIO_KEY_NAMESPACE` (`prod` on app-host; any other value in dev).
5.  Apply the migration to prod `qa_automation`; set the `MINIO_*` values in `.env.prod` (never printed); deploy the binary; **run the migration a second time** (it backfills any feature whose status changed in the window and does nothing else).

## Assumptions Goldfish reviewers must challenge
1.  **The dedicated snapshot queries are the complete and right source** for the checklist, including the orphan rule (excluded, counted, logged) and the `origin='sync'` filter.
2.  **Sync atomicity and the snapshot.** The sync commits entity rows, history rows and `status='succeeded'` in one transaction (verified in `sync.go`), and `BeginTx` with `TxOptions{ReadOnly, Isolation}` works with this driver (precedent: `gaossync/plan.go`). The re-read of the sync id inside the full read keeps the stored key honest. A test commits a sync while a snapshot runs.
3.  **`changed_sync_id` semantics** (verified in `store.go` `upsert`): set on every full write, not on location-only refreshes, and a status change always changes the content hash. So a backfilled date is never earlier than the last content change.
4.  **History rows are written only in `apply`**, the only place `release_status` changes for synced rows. A parser-version bump rewrites hashes (an `updated` event) and must write no history row when the status is equal; a first sync writes `backfill`-source rows; a restore with equal status writes nothing.
5.  **Idempotence.** Sync writes rely on the unique key plus `ON DUPLICATE KEY UPDATE id = id`; backfill relies on its status-aware `NOT EXISTS`. A duplicate feature id cannot reach `apply` (the parser map keeps the last), so one row per feature per sync holds.
6.  **The 30 s force cooldown and per-flight singleflight are enough** abuse protection for authenticated readers, who may all use `force=1`.
7.  **Prefix-only isolation in the shared bucket is acceptable until a scoped credential exists.** Challenge whether a production service should hold a full-rights staging key at all, and whether the pre-deploy checklist is a strong enough control.
8.  **A manual sync of an unchanged commit is `succeeded`** and so changes the cache key (verified in `sync.go`: the `skipped` path requires `trigger == TriggerSchedule`). Accepted.
9.  **The PDF is an approximate layout.** The reference `SUT-MODULE-CHECKLIST.pdf` is not in the repo and was not seen by the author; colours and widths here are proposals. go-pdf/fpdf can produce a TOC with page numbers and a repeating table header.
10. **Production is same-origin; local dev is not.** In dev the two response headers are unreadable and the client degrades.
11. **The MinIO endpoint is reachable from the backend container** and its TLS posture is as the checklist finds. Asserted by the requester for app-host, not verified by the author.
12. **Streaming an uncached file on any MinIO error (`bypass`) is acceptable**, including that a failing MinIO is visible only in logs and a header.
13. **The sanitised `products.name` is safe** in `Content-Disposition` and in the PDF (cp1252 plus `?` replacement), and a rename is covered by the name hash in the key.
14. **Deterministic output.** PDF, XLSX and CSV rendering is deterministic enough for a golden-hash test (fixed creation date, no `time.Now()`); if the PDF library proves nondeterministic, the test falls back to extracted text.

## Target Files
*   **Spec PR:** `docs/designs/MODULE_CHECKLIST_EXPORT.md`, `docs/designs/SITEMAP.md` (this row), `docs/designs/PRODUCT_KNOWLEDGE_SYNC.md` (one "Amendment 3" pointer to this spec, no other change).
*   **Backend:**
    *   new `ai-backend/internal/services/objectstore/` (`objectstore.go`, `minio.go`, `objectstore_test.go`, `fake_test.go` (test helper), `README.md`)
    *   new `ai-backend/internal/services/knowledge/export_checklist.go`, `export_cache.go`, `export_pdf.go`, `export_xlsx.go`, `export_csv.go`, `export_checklist_test.go`, `export_cache_test.go`
    *   `ai-backend/internal/services/knowledge/store.go` (`apply`: new `sha` and `firstSync` parameters, read the stored `release_status` for features, insert history rows), new `ai-backend/internal/services/knowledge/store_history_test.go`
    *   `ai-backend/internal/services/knowledge/sync.go` (**mandatory**: the single `apply` call site passes `sha` and `!hasPrev`)
    *   `ai-backend/internal/services/knowledge/README.md`
    *   not changed: `ai-backend/internal/services/knowledge/query.go` (`Tree` and its helpers stay as they are)
    *   `ai-backend/internal/db/migrations.go` (new table and the backfill statement; appended only), `ai-backend/internal/db/README.md`, `ai-backend/internal/db/skipif_test.go` and `ai-backend/internal/db/migrations_test.go` (must pass; extend only if they enumerate statements)
    *   new `ai-backend/internal/api/knowledge_export.go` (handler, `strictBool`, `SetExportCache`), `ai-backend/internal/api/knowledge_export_test.go`; `ai-backend/internal/api/knowledge.go` (one `case` in `handleKnowledge`, two cases in `writeKnowledgeError`); `ai-backend/internal/api/router.go` (the `exports` field only); `ai-backend/internal/api/README.md`
    *   `ai-backend/internal/config/config.go` (`ObjectStoreConfig`, `String`/`GoString`), a config redaction test in the config package, `ai-backend/cmd/server/main.go` (build the store and cache; append both secrets to `Secrets`)
    *   `ai-backend/README.md` (only if it lists environment variables), `ai-backend/.env.example`, `deploy/qa-tool/.env.prod.example` (names only)
    *   `ai-backend/go.mod`, `ai-backend/go.sum`. New dependencies, pinned to versions whose own `go` directive is at most 1.23 (checked when drafting): `github.com/minio/minio-go/v7 v7.0.80` (go 1.22), `github.com/xuri/excelize/v2 v2.9.0` (go 1.18), `github.com/go-pdf/fpdf v0.9.0` (go 1.20), `golang.org/x/sync v0.10.0` (go 1.18). Their **transitive** requirements (for example `golang.org/x/crypto`, `x/net`) are not checked here; the implementer verifies the build in `golang:1.23-alpine` with `CGO_ENABLED=0` and reports any forced bump. `ai-backend/Dockerfile` changes only if a bump is unavoidable, and then only after the human approves.
*   **Rules:** `.claude/rules/secrets.md` (add `~/.config/minio/*`).
*   **Dashboard:** `dashboard/src/views/knowledge/KnowledgeView.tsx`, new `dashboard/src/views/knowledge/ChecklistDialog.tsx`, `dashboard/src/views/knowledge/api.ts`, `dashboard/README.md`.
*   **E2E (optional):** `this-qa-automation-system-playwright/tests/api/knowledge.spec.ts` (extend: 400 on a bad format, 200 with the right headers when object storage is configured).
*   **Not changed:** the executor, `docker-compose.prod.yml`, nginx, `lib/`, the sync parser (`testcatalog.go`, `catalog.go`), the `Tree` JSON shape.

## Phases
Each phase is its own small PR (root `CLAUDE.md` git workflow); a later phase starts after the earlier one is merged.
1.  **History:** migration (table and backfill), `apply` and `sync.go` changes, `store_history_test.go`, db README. Checkpoint: DB tests pass; migration is a no-op on the second run.
2.  **Snapshot and renderers:** `ChecklistSnapshot`, `pickStatusDate`, CSV, XLSX, PDF, golden-hash tests. Checkpoint: row counts and golden hashes pass; visual check of the PDF and XLSX.
3.  **Storage and endpoint:** `objectstore`, config, `ExportCache`, handler, errors, env examples, secrets rule. Checkpoint: unit and handler tests; staging-MinIO check.
4.  **Dashboard:** button, dialog, `api.ts`, README. Checkpoint: build, lint and a manual download in each format.
5.  **Deploy** (separate approval): the "Pre-deploy checklist".

## Verification Plan
1.  `cd ai-backend && gofmt -l <touched files>`, then `go build -mod=mod ./... && go vet -mod=mod ./... && go test -mod=mod -race ./...`. Also build in the backend image's Go: `docker build` of `ai-backend/Dockerfile` (`golang:1.23-alpine`, `CGO_ENABLED=0`) must succeed with the new dependencies.
2.  **Unit tests (no DB, no MinIO):**
    *   `objectstore`: key validation (outside prefix, empty remainder, `//`, `.` and `..` segments, leading `/`, backslash, control characters); constructor rejects an empty prefix; the interface has no list/delete.
    *   **Config redaction:** `%v`, `%+v` and `%#v` of `ObjectStoreConfig` contain no secret; invalid namespace and plaintext-off-WireGuard make the config invalid.
    *   **Date order (`pickStatusDate`), table test:** a `sync` row wins over a `backfill` row; only `backfill` gives `backfill`; neither gives a dash; rows with a status other than the current one are ignored; the marks (`~` and the `Sumber tanggal` value) are right for each source in all three renderers.
    *   **Renderers** over a small hand-built snapshot and a snapshot built from `testdata/sample-catalog`: non-empty output with the right magic bytes (`%PDF-`, `PK`, BOM); the row count in CSV and XLSX equals the number of active synced features whose module and part are active; with `last_update=1` the date strings are present (Jakarta time, Indonesian months) and empty or `—` for unknown; formula-injection cells are neutralised; non-cp1252 characters become `?` in the PDF; determinism (two renders give identical content).
    *   **Golden hashes:** a hash of the CSV, of a normalised XLSX cell dump and of the PDF bytes (fixed creation date) for the fixture snapshot. A change fails the test until `TEMPLATE_VERSION` is raised and the hashes are updated, so a library upgrade cannot change the output silently.
    *   **Cache logic** against the fake store and injected clock and renderer: miss then hit (the second call does not render), `force=1` then `regenerated`, force within the cooldown becomes `hit` (including a negative age), a new sync id, a changed `TEMPLATE_VERSION`, a renamed product, a new history watermark and a different namespace each give a different key; `Get`/`Stat`/`Put` errors give a `bypass` result with the generated file and the right `Put` skipping; concurrent calls render once and a forced call does not join an unforced flight (`-race`); the extra `Put`s run after the result is returned (`Wait()`).
    *   **Handler** with a fake `snapshotSource`: 405, 400 on bad `format`/`last_update`/`force` (`strictBool`: `0|1|true|false` only), 503 when the cache is nil, headers, the order 405/400/404/503/409. Cases that need real rows (404 for a missing product, 409 with no succeeded sync, a read-only role getting 200) are in the DB list.
3.  **DB tests** (run when `QA_TEST_MYSQL_DSN` is set; helpers `testdb.New`, `sync_test.go:setupProduct`, `crud_integration_test.go:setupCRUDIntegrationRouter`), using the `testdata/sample-catalog` fixtures:
    *   a **first** sync writes one `backfill`-source row per feature; a later new feature writes a `sync` row;
    *   a second sync of the same commit writes none;
    *   change one feature's status in a copied fixture and sync again: exactly one new `sync` row with the right `prev_status`;
    *   change only a feature's notes: no history row;
    *   a **parser-version bump** with equal statuses: no history row;
    *   remove then restore with the same status: no row; restore with a different status: one row;
    *   the backfill statement on a database that has features but no history inserts one `backfill` row per active feature, a second run inserts none, and after a status change made outside a sync (the deploy window) a second run inserts a row with the new status;
    *   the snapshot read sees one consistent sync id and feature set while a sync is committing; no succeeded sync gives `ErrNoSucceededSync`;
    *   handler: 404, 409, and a read-only role gets 200.
4.  **Migration:** run `go run -mod=mod ./cmd/databasemigration` against a dev schema, then a second time and confirm it changes nothing; `skipif_test.go` and `migrations_test.go` pass.
5.  `cd dashboard && npx tsc --noEmit && npm run lint && npm run build`.
6.  **Staging-MinIO check** (needs WireGuard; credentials loaded into env only, never printed; use a test namespace such as `dev-<name>`): with `MINIO_*` set, download PDF: header `miss`, the object exists under `qa-automation/<namespace>/` (key printed; no listing outside the prefix); again: `hit`, no regeneration in the log; tick Force: `regenerated` (or `hit` inside 30 s); run a sync: a new key; rename the product: a new key; repeat for xlsx and csv. Confirm from **inside the backend container** that the endpoint is reachable. With `MINIO_*` unset: the export answers 503 and every other page works. With a wrong secret: the export still returns the file with `bypass` and a log line with an error code and no secret.
7.  **Visual check** of the PDF against `SUT-MODULE-CHECKLIST.pdf` (sections, columns, colours, TOC, header, footer, repeating table header; page count in the same range as 19). Open the XLSX in LibreOffice (frozen header, filter, sheet names) and the CSV in a spreadsheet (BOM, accents, em dash).
8.  **Deploy** (a separate approval): follow the "Pre-deploy checklist". After deploy, through `qa-tool-session` as `ai-service` (a `GET`, no impersonated write): request the export and check `X-Export-Cache`.

## Review resolution (rev 2 to rev 3)
| Finding | Resolution |
|---|---|
| B1 snapshot SQL | Latest-sync query rewritten (`ORDER BY id DESC LIMIT 1`, `ErrNoRows` to `ErrNoSucceededSync`). |
| B2 stale name/history in key | `nameHash` and the last-update history watermark added to the key; Goal 3 reworded. |
| B3 `status_as_of` | Dropped entirely (human); date order is `sync` > `backfill` > dash; tests and column values updated. |
| B4 `Tree` in a tx | Dedicated queries on the transaction, explicit ordering, orphan rule, row-count rule; `query.go` unchanged. |
| B5 missing sections | "Alternatives Considered" and "Risks & Mitigations" added. |
| M1 errors | Two new errors, `writeKnowledgeError` edit, check order, 413 and 404 body in the contract. |
| M2 force | `Stat` of the requested key only for the cooldown, `force` in the flight key, skew tolerated. |
| M3 `sync.go` | Mandatory; `apply(sha, firstSync)`; `vals[4]`; no test calls `apply`. |
| M4 orchestration | `ExportCache` type, fields, constructor, setter, typed-nil avoidance. |
| M5, M6 shared credential, namespace | Mandatory `MINIO_KEY_NAMESPACE`; pre-deploy checklist (prefix-empty, scoped key or dedicated bucket requested, not blocking). |
| M7 false "since" dates | Restore with equal status writes nothing; first-sync adds are `backfill`-source. |
| M8 backfill | Exact SQL, `NOT EXISTS` sole guard, db tests listed, deploy-window repair by a second run. |
| M9 idempotence | `ON DUPLICATE KEY UPDATE id = id` only; backfill idempotence is `NOT EXISTS`. |
| M10 dependencies | Pinned versions, build in the Go 1.23 image. |
| M11 handler tests | DB-needing cases moved; helpers cited; fake `snapshotSource`. |
| M12 PDF parity | Goal 7 downgraded; colours, widths, strings, months, time zone fixed. |
| M13 secrets | Value-receiver `String`/`GoString`, redactor list, error code only, TLS rule, redaction test. |
| M14 latency | Light read first, extra `Put`s after the response, latency bound stated. |
| M15 scope | Kept in full (Decision 13). |
| M16 Target Files | Added the db tests, config test, `ai-backend/README.md`, E2E path, setter placement. |
| M17 parsing and gating | `strictBool`; derived `tab === 'modules'`. |
| Nits N1 to N11 | Time zone (N1); assumptions renumbered and settled ones turned into facts (N2); `managedByRepo` claim corrected (N3); guard wording (N4); single `Get` (N5); key validation (N6); `secrets.md` (N7); ordering and golden hash (N8); subjective words removed and terms defined (N9); typos and status line (N10); phases, `bypass` log line, dev-origin note, parser-bump test (N11). |

## Amendment 1: Scope and "Prod since" (2026-11-26)

**Why.** Owners want to download only the features that are in production, and only those that reached production in a period ("PROD features since May 2026 to Nov 2026"). `status_as_of` cannot answer that: it is the date a status was *confirmed* (a whole catalogue can carry one bulk date), not when the feature went live. The automation repo's catalogue gains a **`Prod since`** column for that fact; this amendment imports it and filters on it.

**Catalogue format.** A feature row has 12 columns, or 13 with `Prod since` right after `Status as of`. Both layouts parse (a repo moves from 12 to 13 in one commit; this parser must be deployed first, because an older parser rejects 13-column rows and the sync then drops every feature). Values: `YYYY-MM` (that month), `YYYY-MM..YYYY-MM` (somewhere in that period, from <= to), `<YYYY-MM` (before that month), `?` (in production, date unknown), `—` or absent (none; stored as `''`). An invalid value is a sync problem and is stored as `''`. The repo's own validator requires a value for `PROD` rows and `—` otherwise; this parser does not enforce that pairing (it reports formats only).

**Storage.** `kb_features.prod_since VARCHAR(20) NOT NULL DEFAULT ''` (guarded `ALTER`), raw text; it is part of the generic feature columns, so the feature API returns it and the dashboard shows it on the feature page and panel. Parsing happens at export time (`ParseProdSince`), so the format can grow without a migration.

**API.** `GET .../exports/modules-checklist` gains:
| Param | Values | Default | 400 when |
|---|---|---|---|
| `scope` | `all`, `prod` | `all` | other value, repeated |
| `since_from`, `since_to` | `YYYY-MM` | empty (open end) | not `YYYY-MM`, repeated, given without `scope=prod`, from > to |

**Filter (`FilterChecklist`, after the snapshot, before rendering).** `all` keeps every row (output identical to before, apart from the new column). `prod` keeps `PROD` rows only. With a period, a `PROD` row stays when its period overlaps `[since_from, since_to]` (open ends allowed): `<2026-05` is `(-inf, 2026-04]`, so it only matches a period that starts before May 2026; `2026-07..2026-08` matches any period touching July or August. `PROD` rows with `?`, `''` or an unparsable value are left out of a ranged export and **counted** (`ExcludedUnknown`); the header says how many. Modules and parts left without rows are dropped.

**Renderers.** A `Prod sejak` column after `Status` in all three formats, shown as `Jul 2026`, `Jul–Agu 2026`, `Des 2025–Jan 2026`, `< Mei 2026`, `?` or empty. A filtered export prints its scope line under the PDF title and in the XLSX summary (A3), e.g. `Hanya fitur PROD yang periode Prod sejak-nya beririsan dengan Mei 2026 s.d. Nov 2026 (16 fitur PROD tanpa tanggal tidak disertakan)` ("beririsan" names the overlap rule). The CSV has no header block; its file name carries the scope. `TemplateVersion` becomes 2.

**Cache key and file name.** The variant gets a scope suffix only for `prod` (`…/plain/prod`, `…/plain/prod-202605-202611`, `x` for an open end), so `scope=all` keeps the old key shape. The download name gets the same suffix in upper case (`<product>-MODULE-CHECKLIST-PROD-202605-202611.pdf`).

**Dashboard.** The checklist dialog gets a *Scope* radio (All features / Production only) and, for Production only, optional *from* / *to* month inputs (`<input type="month">`), validated client-side (from <= to) and again by the API.

**Edges and limitations (review 2026-11-26).** Empty `scope=`, `since_from=` or `since_to=` mean the default (all / open end). Checks run after `format`, `last_update` and `force`, in this order: repeated param, scope value, month format, range without `scope=prod`, from > to. The 413 size check runs on the whole catalogue, before filtering. A ranged export of a catalogue that has no `Prod since` column is empty apart from the "N fitur PROD tanpa tanggal" count in its header, and the dialog's help text says that undated PROD rows are left out. The 13-column layout is detected by the cell count, so a 12-column row with a stray `|` would read Evidence as Prod since (reported as a problem; the repo's own validator rejects such rows). The format version bump (1 → 2) makes every product's next scheduled sync re-import its unchanged commit once. Overlap means "possibly in production during the period": a vague `2026-01..2026-12` row matches a May–Nov query.

**Assumptions to challenge.** (1) Overlap, not containment, is what owners mean by "since between May and Nov" (a `Jul–Agu` row is in; a `<2026-05` row is out). (2) Leaving `?` rows out of ranged exports, with a count, is better than including them flagged. (3) Parsing at export time is cheap enough (one regexp per row, at most `maxExportFeatures`).

**Target files (amendment).** `ai-backend/internal/db/migrations.go`, `internal/db/skipif_test.go`, `internal/services/knowledge/{catalog,store,query,testcatalog,export_checklist,export_cache,export_csv,export_xlsx,export_pdf,export_filter}.go` and their tests, `testdata/sample-catalog/docs/modules/C-partner-api.md` (13-column layout) + `testdata/README.md`, `internal/api/knowledge_export.go` + test, `dashboard/src/views/knowledge/{api.ts,ChecklistDialog.tsx,EntityPanel.tsx,pages/FeaturePage.tsx}`.

**Fix 2026-11-26 (TemplateVersion 3): two "sejak" dates.** With `last_update=1`, the PDF printed `sejak ~14 Nov 2026` under `PROD` next to the `Prod sejak` column, and users read it as the release date. It is the date this tool *recorded* the status (backfill or sync). Changes: the PDF sub-line reads `tercatat <date>` / `tercatat ~<date>` / `tercatat —`, and the footnote says it is not a release date; a `PROD` row with a known `Prod since` period (not `?`, not empty) shows no sub-line; the CSV/XLSX column `Status sejak` is renamed `Status tercatat` (values unchanged); the dialog's option reads "Add the date each status was recorded" and is switched off when *Production only* is chosen (still selectable). The `Status sejak` wording in the base spec above is superseded by this paragraph.
