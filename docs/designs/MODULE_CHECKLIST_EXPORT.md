# Module checklist export (PDF / Excel / CSV, cached in MinIO)

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Draft rev 2 (2026-11-17). Human decisions on the open questions folded in; claims checked against `sync.go`. Not yet reviewed by Goldfish. Not approved.
*   **Date:** 2026-11-17
*   **Depends on:** [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (implemented): the `kb_*` tables, the `Syncer`, the `Tree` query.
*   **Code references** use function names. Line numbers are approximate.

## Overview
`HISV3-MODULE-CHECKLIST.pdf` (19 pages, A4, 440 features) was built by hand from the HISv3 module catalogue through a LaTeX file that is not in the repo. This spec lets the dashboard produce the same document on demand, from data that is already in `qa_automation`:

*   the knowledge base page `/products/:id/knowledge?tab=modules` gets a **Download checklist** button and an options dialog (format, "Add last update to Status column", "Force regenerate");
*   the **backend** renders the file as PDF, Excel (`.xlsx`) or CSV;
*   every generated file is **stored in MinIO**. The next download of the same content is served from MinIO and nothing is regenerated;
*   a new table, `kb_feature_status_history`, records when each feature's release status changed, so the "last update" can be shown.

The markdown catalogue is **not** read at export time. The source is the synced knowledge base (`kb_features.release_status`, the part, module and sub-module rows), through the same grouping the `Tree` query already does (`knowledge/query.go`, `Syncer.Tree`).

## Problem
*   The checklist is produced by hand, so it goes stale as soon as the catalogue changes, and nobody can regenerate it without the LaTeX file.
*   The knowledge base stores only the current release status of a feature (`kb_features.release_status`, `status_as_of`), as parsed from the repo. `status_as_of` is the raw text of the catalogue cell. The only change marker is `changed_sync_id`, and it moves on **any** content change (`store.go: apply`, the `e.hash != r.hash()` case), not only on a status change. So the data cannot answer "since when is this feature PROD?".
*   Re-rendering a 19-page PDF on every click is wasteful when nothing changed.

## Goals
1.  `GET` endpoint that returns the module checklist of a product as PDF, XLSX or CSV, with the same content in all three.
2.  Optional "Status sejak" (status since) information, taken from a new status-change history.
3.  Generated files are cached in MinIO under a key that changes whenever the underlying data or the template changes, so a hit is always current and a stale file can never be served.
4.  The user can bypass the cache ("Force regenerate").
5.  Concurrent identical requests generate once.
6.  The rest of the app is unaffected when MinIO is not configured or not reachable.
7.  The PDF reproduces the layout of `HISV3-MODULE-CHECKLIST.pdf` (see "PDF layout").

## Non-Goals
*   **Reading the markdown at export time.** The DB is the source.
*   **Browser-side generation (jsPDF or similar).** Decided against; the backend renders.
*   **Exporting epics, stories, test cases, coverage or annotations.** Only parts, modules, sub-modules and features. The Repo, Evidence and Notes columns stay out, as in the current PDF.
*   **Deleting, expiring or listing objects.** v1 never deletes anything in MinIO (see "Accepted limitations").
*   **Per-user or per-role variants of the file.** One file per `(product, sync, template, option, format)`.
*   **Exposing MinIO to browsers** (no presigned URLs). The backend always streams.
*   **A history UI.** The history table is read only by the export in v1.
*   **A new object-storage use for anything else.** The `objectstore` package is generic, but this spec authorises one caller.

## Decisions (user, 2026-11-17)
1.  Generation on the backend (PDF, Excel, CSV), not in the browser.
2.  Every generated file is stored in MinIO; later downloads come from MinIO.
3.  The dialog has "Force regenerate (ignore cached file)".
4.  First option: "Add last update to Status column": the date each feature's status last changed.
5.  MinIO is the **staging MinIO that HISv3 uses**; the cache key includes the latest succeeded sync id; status history is tracked on every sync.

6.  **Date precedence.** `sync` history row, then `kb_features.status_as_of` (only when it parses as an ISO date; any other text is skipped), then `backfill` history row, then a dash. Dates from `status_as_of` or `backfill` are approximate and marked (`~` in the PDF with a footnote; `Sumber tanggal` column in XLSX and CSV).
7.  **Bucket** = the permanent `bucketName` of the staging MinIO credentials, not `tempBucketName`.
8.  **MinIO runtime failure** does not fail the request: the file is generated and streamed without caching, `X-Export-Cache: bypass`, and the error is logged. There is no 502.
9.  **Any reader** may use `force=1` (the 30 s cooldown stays).
10. **Button** on the Modules tab only, outside the `canWrite` guard.
11. **No retention in v1.** `objectstore` has no `List` or `Delete`. Retention is a future item.
12. **Product name** (from the products master, `products.name`) in the PDF header and the filename: `<product>-MODULE-CHECKLIST.<ext>`.

## Design

### API contract
`GET /api/products/{id}/knowledge/exports/modules-checklist?format=pdf|xlsx|csv&last_update=0|1&force=0|1`

Routed in `handleKnowledge` (`ai-backend/internal/api/knowledge.go`) as a new `case seg == "exports" && len(rest) == 2 && rest[1] == "modules-checklist"`. The handler is a thin function in a new file `ai-backend/internal/api/knowledge_export.go`.

| Aspect | Contract |
|---|---|
| Method | `GET` only; anything else is 405 `method not allowed` (the file's existing wording). |
| Auth | Any authenticated user, like every other knowledge `GET`. `handleKnowledge` gates only writes through `canWrite` (`products.go`: `canVerify`); this endpoint does **not** call `deny()`. It sits behind the same cookie-session middleware as the rest of `/api`. |
| `format` | Required. Exactly `pdf`, `xlsx` or `csv`. Otherwise 400 `format must be pdf, xlsx or csv`. |
| `last_update` | Optional, default `0`. `0`/`1` (the existing `boolParam` also accepts `true`). Any other non-empty value is 400. |
| `force` | Optional, default `0`. Same parsing. |
| 200 body | The file bytes. `Content-Type`: `application/pdf`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`, or `text/csv; charset=utf-8`. `Content-Length` is set (the file is buffered; it is under a few MB). |
| `Content-Disposition` | `attachment; filename="<name>"`, with `<name>` = `<product>-MODULE-CHECKLIST.<ext>`, for example `HISv3-MODULE-CHECKLIST.pdf`. `<product>` is `products.name` sanitised for a header: every run of characters outside `[A-Za-z0-9._-]` becomes `-`, leading and trailing `-` are trimmed, and it is cut to 60 characters. An empty result falls back to `product-<id>`. It never contains a quote, `/`, `;`, a space or a control character. Non-ASCII letters are dropped, not percent-encoded, so no `filename*` is needed. |
| `X-Export-Cache` | `hit` (served from MinIO), `miss` (generated now and stored), `regenerated` (`force=1` honoured and stored), or `bypass` (MinIO failed at runtime; generated and streamed without caching). `Cache-Control: no-store`. |
| 404 | Product not found (`knowledge.ErrProductNotFound` through `writeKnowledgeError`). |
| 409 | The product has no succeeded sync yet (nothing to export): `{"error":"no knowledge base sync has succeeded yet"}`. |
| 503 | Object storage **not configured** (`object storage not configured`; a runtime MinIO failure is not a 503, see `bypass`), or `s.kb == nil` (the existing `knowledge base not available`). |

The dashboard is same-origin (`/api`), so no CORS header change is needed to read `Content-Disposition` or `X-Export-Cache`. Verify this at implementation; if the dashboard is ever served cross-origin, `Access-Control-Expose-Headers` would be needed.

It is a `GET` that writes to MinIO as a cache. This is not a user-data write, and it creates no `kb_*` rows.

### Snapshot consistency and the cache key
The "latest succeeded sync id" and the feature rows must come from the same database state. Verified in `sync.go`: a sync commits entity rows, history rows (new) and `status='succeeded'` in **one transaction**. The `s.DB.WithTx` closure runs `SELECT ... FROM products ... FOR UPDATE`, then `apply`, `countActive`, and `UPDATE kb_syncs SET status = 'succeeded', finished_at = UTC_TIMESTAMP(), commit_sha = ?`. An error rolls everything back and the sync is then marked `failed` with a fresh context. The export therefore reads both inside **one read-only transaction** opened with `DB.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})` (the existing `DB.WithTx` calls `BeginTx(ctx, nil)` and cannot pass options, `db/connection.go`; the isolation is set explicitly instead of relying on the server default): first `SELECT MAX(id), commit_sha, started_at, finished_at FROM kb_syncs WHERE product_id = ? AND status = 'succeeded'`, then the tree, then the history. Without the shared transaction a sync landing between the two reads would put new data under the old key.

**Cache key** (the only way a key is built; no part of it comes from the request except the whitelisted values below):

```
qa-automation/knowledge-exports/{productId}/modules-checklist/sync-{latestSucceededSyncId}/v{TEMPLATE_VERSION}/{plain|last-update}.{pdf|xlsx|csv}
```

*   `productId` and `latestSucceededSyncId` are integers read from the DB. `plain|last-update` and the extension come from a closed whitelist.
*   `TEMPLATE_VERSION` is a Go constant in `export_checklist.go`. **Raise it on any change that alters the output** of any renderer (layout, columns, colours, wording), as `Format.Version()` is raised for the parser.
*   A new succeeded sync, or a template change, gives a new key automatically. A status change always arrives through a sync, so it is covered by the sync id.
*   `force` and `format` are not part of the "snapshot" part of the key: `force` bypasses the lookup, and the three formats are three objects under the same directory.
*   The key **does not** include local annotations or drafts (not exported). If a later amendment exports them, the key rule must change.
*   Verified in `sync.go`: only a **scheduled** sync whose `(repo, branch, format, parser version, commit)` equals the last succeeded one is finished as `skipped` (it never becomes `succeeded`, so the key is unchanged). A **manual** sync of the same commit is not skipped: it runs `apply`, is committed as `succeeded` with `diff.unchanged = true`, and does change the key even though the content is identical. A scheduled sync after a parser-version bump also proceeds. This costs one extra generation and is accepted (see "Accepted limitations").

### Request flow
```
validate params -> objectstore configured? (else 503)
-> read snapshot (one RO tx): latest sync, tree, history, product name   (409 if no sync)
-> key = f(snapshot.syncId, version, last_update, format)
-> force=0: Stat(key). found -> Get -> stream, X-Export-Cache: hit
   (Stat or Get error -> log, fall through to generation, result is `bypass`)
-> otherwise (miss, or force=1) -> singleflight.Do(key-set) {
       generate the 3 formats for this last_update value from the SAME snapshot
       Put the requested format first, then the other two (best effort)
   } -> stream the requested bytes, X-Export-Cache: miss | regenerated | bypass
```

*   **Three formats per generation** (as the plan decided): one snapshot is rendered as PDF, XLSX and CSV for the requested `last_update` value, and all three are stored, so the next click in another format is a hit. The other `last_update` value is not pre-generated.
*   **Each object is looked up on its own** (`Stat` per key). A partially populated directory is harmless: a missing object is simply generated on demand.
*   **Put order and failure.** The requested format is stored first. If that `Put` fails, the error is logged (without credentials), the remaining `Put`s are skipped, and the generated bytes are still streamed with `X-Export-Cache: bypass`. The other two are best effort: a failure is logged and ignored. If `Stat` or `Get` fails, generation proceeds the same way and the result is `bypass` (a failed `Stat` also skips the `Put`s for that request, to avoid a second timeout). A MinIO failure therefore never produces an error response; only the renderer or the DB can.
*   **Singleflight.** `golang.org/x/sync/singleflight` (new direct dependency) keyed by the directory part `qa-automation/.../sync-{id}/v{n}/{variant}` (without the extension), so concurrent clicks in different formats also generate once. The function runs under `context.WithoutCancel(req.Context())` with its own timeout (`MINIO_TIMEOUT_SECONDS`, default 30, per MinIO call, plus a 60 s ceiling for generation; a dead MinIO adds at most one timeout, because a failed `Stat` skips the `Put`s), so one client closing its tab does not fail the others sharing the flight. The flight shares the result bytes of all three formats; each waiter picks its own format.
*   **`force=1`** skips the `Stat`, regenerates, **overwrites** the three objects (`Put` overwrites) and answers `regenerated`. Since the key is content-addressed, forcing regenerates identical content unless the template renders time-dependent text (see "PDF layout": it does not). To keep one reader from hammering the backend with `force=1`, a force is **ignored when the object's `LastModified` is younger than 30 s**: the request is served as a `hit`. The window is a constant, not configuration. This applies to every reader (decision 9).
*   **No other request-path write.** The handler never lists, deletes or reads outside its own key.

### Status history table
Appended to `ai-backend/internal/db/migrations.go` as idempotent statements (hard rule 3). The server never migrates itself; run `go run -mod=mod ./cmd/databasemigration` before the binary is deployed.

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

*   `feature_code` has the same collation as `kb_features.code` (`utf8mb4_bin`, see `migrations.go` `kb_features`) so joins by code are exact. There is deliberately **no FK** to `kb_features` (features are soft-deleted, and codes are the link, as everywhere in the knowledge base).
*   The unique key on `(product_id, feature_code, sync_id, source)` makes the writes **idempotent**: backfill and the sync insert use `INSERT ... ON DUPLICATE KEY UPDATE id = id` (or `INSERT IGNORE`). A re-run of the migration or of a sync transaction retry adds nothing. (A feature can change status at most once per sync, so `sync_id` is a sufficient discriminator.)
*   `prev_status` is NULL for a feature's first row.

**Sync-time writes** (`store.go`, `apply`). Today the `existing` struct and the `SELECT code, origin, content_hash, removed_at IS NOT NULL, source_file, source_line` do not carry the stored `release_status`. For the feature table only, the select also reads `release_status`. After the existing add/restore/update decision, for `et == TypeFeature`, one history row is inserted **in the same transaction** when:

| Case | Row |
|---|---|
| feature added (`!ok`) | `release_status = new`, `prev_status = NULL` |
| feature restored (`e.removed`) | `release_status = new`, `prev_status = the stored status` (also when equal: a restore is a visible event) |
| feature updated and `release_status` differs from the stored value | `release_status = new`, `prev_status = old` |
| any other update (same status) | none |
| removed | none (a removal is not a status; the feature is not exported) |

`sync_id` = this sync, `commit_sha` = `sha`, the commit the sync read, which is in scope in the `WithTx` closure (`sync.go`) and is written to `kb_syncs.commit_sha` by that same transaction. It must be passed into `apply` as a new parameter (today `apply` receives `productID`, `syncID`, `snap`, `prob` only). Reading it back from `kb_syncs` inside `apply` would return NULL, because the column is set at the end of the transaction, `changed_at = UTC_TIMESTAMP()`, `source = 'sync'`. Rows are inserted in batches like `upsert` (`batchSize`).

Because the history row, the entity rows and `status='succeeded'` commit in one transaction, the history can never disagree with the entity tables, and a failed sync leaves no history.

**Backfill** (a migration statement, run once; idempotent through the unique key and a guard):
*   One row per **active, synced** feature (`origin='sync' AND removed_at IS NULL`) that has **no history row yet**.
*   `release_status` = its current `kb_features.release_status`, `prev_status` = NULL, `source = 'backfill'`.
*   `sync_id` = `COALESCE(changed_sync_id, first_sync_id)`; `changed_at` = `kb_syncs.started_at` of that sync (the sync that last changed the feature's content). If the feature has neither id, or the sync row is gone, `changed_at` falls back to `kb_features.updated_at`, and if that is NULL the row is not inserted (the export shows `—`).
*   **The date is approximate by construction.** Verified in `store.go` `upsert`: `changed_sync_id` is set on every full write (insert, restore, content-hash change, so any change of evidence, notes or name) and is **not** touched by the location-only refresh (`full=false`). So it is the sync of the feature's last full write, and the backfilled date can be later than the real status change. `first_sync_id` is set only on insert. `kb_syncs.started_at` is the sync's start time, not the commit's date. The dashboard dialog helper text says "Dates before tracking began are approximate", and backfill rows are the only ones that can be approximate.
*   A feature tagged only `GIT:main` has no real date until a sync sees a change; it shows the approximate backfill date at first.
*   The statement uses an `INSERT ... SELECT ... WHERE NOT EXISTS (SELECT 1 FROM kb_feature_status_history h WHERE h.product_id = f.product_id AND h.feature_code = f.code)` guard, so a second run, and a run after real sync rows exist, inserts nothing for those features. It is guarded the way existing set-based migration statements are (see the `-- skip-if:` convention in `migrations.go`, and the `product_repos` statement above `kb_syncs` for a set-based example).

**What the export reads.** For each exported feature, the status date is the first of these that yields a date, and its **source** is recorded:
1.  **`sync`:** `MAX(changed_at)` over the history rows with `source = 'sync'` whose `release_status` equals the feature's **current** status. Exact to the observing sync.
2.  **`status_as_of`:** `kb_features.status_as_of`, only if it parses as an ISO date (`YYYY-MM-DD`). It is the raw catalogue cell, stored with at most 20 characters, with `—` stored as empty (`hisv3.go`, `fit(k, "status_as_of", ...)`). Any other text (empty, free text, a different date format) is **skipped**, not shown, and the lookup falls through to step 3.
3.  **`backfill`:** `MAX(changed_at)` over the history rows with `source = 'backfill'` and the current status.
4.  Otherwise: a dash.

Sources `status_as_of` and `backfill` are **approximate** and are marked in every format (below); only `sync` is exact to the observing sync. The precedence is one pure function (`pickStatusDate(syncRow, statusAsOf, backfillRow) (date, source)`) so it is table-testable.

### Export service: package `ai-backend/internal/services/knowledge`
New file `export_checklist.go`, no new exported errors beyond `ErrNoSucceededSync`.

*   `type ChecklistSnapshot struct { ProductName string; SyncID int64; CommitSHA string; SyncedAt time.Time; Parts []ChecklistPart }`, where a part holds modules, a module holds rows `{ID, Submodule, Feature, Status, StatusSince *time.Time}`. Features with no sub-module have an empty sub-module cell. Removed rows (`removed_at IS NOT NULL`) and local rows are excluded (only `origin='sync'`, active; features are never local drafts, see `managedByRepo`).
*   `func (s *Syncer) ChecklistSnapshot(ctx, pid) (*ChecklistSnapshot, error)`: the single read-only transaction described above. It reuses the grouping logic of `Tree` (extract the shared grouping, or run a dedicated query; do **not** change `Tree`'s JSON output, the dashboard depends on it).
*   `func RenderChecklist(snap *ChecklistSnapshot, format string, withLastUpdate bool) ([]byte, string /*content type*/, error)`.
*   Rendering is **deterministic**: it uses `snap.SyncedAt` (the sync's `finished_at`), never `time.Now()`, so a regenerated file is byte-identical in content and a force regenerate never changes what a user sees.

**PDF** (`github.com/go-pdf/fpdf`, core Helvetica; Latin text and the em dash are covered by fpdf's cp1252 translator, so text is passed through `UnicodeTranslatorFromDescriptor("")`). Characters outside cp1252 (for example from a feature name) would render as `?`; the renderer replaces them with `?` explicitly and the test asserts the HISv3 fixture has none that matter. If a future product has non-Latin text, a bundled TTF font becomes necessary (Accepted limitations).
*   A4 portrait, 9 pt, 14 mm margins. Header: `<product name> — Checklist Katalog Modul` and the sync date. The product name is `products.name` through the cp1252 translator; any character outside cp1252 is replaced with `?`, and the name is cut to 60 characters, so an unusual name cannot break the layout. Footer: page number.
*   Title in teal `#007E82`; status legend; total summary table; table of contents with page numbers (two-pass: render, record page numbers, render again, or use fpdf aliases); per-part summary; per-module table with columns `ID | Sub-modul | Fitur | Status | Cek` (empty square), a repeating teal header row on every page, and bold coloured status text (no emoji).
*   With `last_update=1` the status cell has a second line, for example `PROD` and `sejak 01 Sep 2026`. An approximate date (source `backfill` or `status_as_of`) is prefixed with `~`, for example `sejak ~01 Sep 2026`; A footnote on the legend page explains `~`. No date from any source shows `—`.

**XLSX** (`github.com/xuri/excelize/v2`): a `Ringkasan` sheet and one sheet per part (sheet names are clipped to 31 characters and stripped of `[]:*?/\`); the same columns; status cell colouring; autofilter and a frozen header row. With the option, an extra `Status sejak` column holding a real date cell (`yyyy-mm-dd`) so Excel can sort it , and a `Sumber tanggal` column with `sync`, `backfill`, `status_as_of` or empty.

**CSV** (`encoding/csv`): UTF-8 with a BOM so Excel opens it correctly; the same columns; one header row. With the option, `Status sejak` as `YYYY-MM-DD` (empty when unknown, because `—` in a CSV is noise for filters) and `Sumber tanggal` with `sync`, `backfill`, `status_as_of` or empty.

**CSV and spreadsheet formula injection.** Feature names and sub-module names come from a git repo and could begin with `=`, `+`, `-` or `@`. In CSV and XLSX, a text cell that starts with one of `= + - @ \t \r` is prefixed with `'` (CSV) or written as an explicit string cell (XLSX). The status and date columns are generated by the code and need no prefixing. The test covers this.

### MinIO configuration and the `objectstore` package
New package `ai-backend/internal/services/objectstore`:

```go
type Store interface {
    Stat(ctx context.Context, key string) (Info, bool, error) // Info{Size, LastModified}; bool = exists
    Get(ctx context.Context, key string) ([]byte, error)
    Put(ctx context.Context, key, contentType string, data []byte) error
}
```
*   MinIO implementation with `github.com/minio/minio-go/v7`. A fake in-memory implementation lives in the test files only.
*   **Prefix enforcement is in the package, not in the callers.** The constructor takes the fixed prefix `qa-automation/`, and every method rejects, before any network call, a key that does not start with it, contains `..`, a leading `/`, a backslash or a control character. There is **no** `List` and **no** `Delete` method on the interface, so a bug cannot list or delete objects that belong to HISv3 in the shared bucket.
*   `Get` is capped (the export objects are small; cap at 64 MiB) and returns an error beyond the cap.

**Environment variable names** (names only; values never appear in the repo, a log, an error, a response or this spec):

| Name | Meaning |
|---|---|
| `MINIO_ENDPOINT` | `host:port`, no scheme (minio-go form) |
| `MINIO_ACCESS_KEY` | secret |
| `MINIO_SECRET_KEY` | secret |
| `MINIO_BUCKET` | bucket name |
| `MINIO_USE_SSL` | `true`/`false` |
| `MINIO_TIMEOUT_SECONDS` | per-call timeout, default 30 |

*   Read in `ai-backend/internal/config/config.go` (a new `ObjectStoreConfig` next to `ProductsConfig`), wired in `ai-backend/cmd/server/main.go`. **Configured** means `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` and `MINIO_BUCKET` are all non-empty. Otherwise `objectstore` is nil, nothing connects at startup, and the export endpoint answers 503. A MinIO outage at startup does not stop the server: connection is lazy, on first use.
*   The config's `String()`/`%v` output must redact the two secrets. Startup logs only `object storage: configured` or `not configured`, never the endpoint credentials.
*   The names (no values) are added to `ai-backend/.env.example` and `deploy/his-v3-qa/.env.prod.example`. `docker-compose.prod.yml` already uses `env_file: .env.prod`, so no compose change is expected; confirm at implementation that no explicit allow-list is needed.
*   The values come from the staging MinIO credentials file of HISv3 (a file under `~/.config/minio/`, JSON). They are loaded **into the process environment or a secret file only**, never printed (hard rule 1, `.claude/rules/secrets.md`). `MINIO_BUCKET` is the permanent `bucketName` (decision 7). The file's other keys (`tempBucketName`, `shareDuration`, `timeout`) are not used by this feature; the temporary bucket is avoided because it may carry a lifecycle rule that empties it (not verified).

**Cross-environment dependency (explicit).** The **production** qa-tool (`qa-tool.sismedika.com`, rnd-v3) will read and write the **staging** MinIO that HISv3 itself uses, with the same bucket and the same access key. Consequences the reviewers must weigh:
*   The credentials are shared with HISv3, so they can read and delete HISv3 objects. We rely on the `objectstore` package (fixed prefix, no list, no delete) rather than on the credential's scope.
*   A staging MinIO outage or reset takes the export cache down (or wipes it). Both are survivable: the export still works (`bypass`, generated on the fly), and a wiped cache regenerates on demand.
*   Rotating the staging keys in HISv3 breaks the export until `.env.prod` on rnd-v3 is updated.
*   The requester verified that the endpoint is reachable from rnd-v3. Reachability **from inside the backend container** (Docker network, not only the host) is re-checked as a pre-deploy step. Not repeated here.
*   Only the `qa-automation/` prefix is ours. We never create buckets, never list the bucket, and never write outside the prefix.

### Failure modes
| Situation | Behaviour |
|---|---|
| `MINIO_*` unset or incomplete | Export endpoint 503 `object storage not configured`. Everything else works. No startup failure. |
| `s.kb == nil` | 503 `knowledge base not available` (existing). |
| Product has no succeeded sync | 409. Nothing is generated or written. |
| MinIO `Stat`/`Get` fails (network, auth, timeout) | Error logged without secrets; the file is generated and streamed, `X-Export-Cache: bypass`, nothing stored. |
| `Put` of the requested format fails | Logged; the generated bytes are streamed, `bypass`. |
| `Put` of the other two formats fails | Logged, ignored. The next request for that format regenerates it. |
| Object exists but `Get` returns fewer bytes than `Stat` size, or an error | Treated as a `Get` failure: logged, regenerated, `bypass`. The body is read fully into memory before any byte is written to the client. |
| Two concurrent clicks, same variant | Singleflight: one generation; both get the file. The second reports the same `X-Export-Cache` value as the first. |
| Two backend replicas | Singleflight is in-process, so two replicas could generate once each and overwrite the same key with identical bytes. Harmless. The stack runs one backend container today. |
| Sync commits while a generation runs | The snapshot is one REPEATABLE READ transaction, so the file matches the key's sync id. The next request sees a new key. |
| Client disconnects | The flight continues (detached context), the objects are stored, and the work is not wasted. |
| Renderer error (bug) | 500 `internal error`, logged by `writeKnowledgeError`'s default case; nothing is stored. |
| Very large product | Memory is bounded by the catalogue (about 440 rows today, a few hundred KB per file). A cap of 20 000 feature rows per export returns 413 `catalogue too large to export`, as a guard. |

### Security
*   **Authentication/authorisation:** any authenticated reader, the same as the other knowledge `GET`s. Auth is active only when the DB connects (hard rule 4); the export needs the DB anyway. The data exported (module and feature names and release status) is what any logged-in user already sees on the Modules tab.
*   **No new write surface on user data.** The only side effect is writing cache objects under `qa-automation/knowledge-exports/`.
*   **No secrets in the output.** The files contain only catalogue fields (id, sub-module, name, status, date). They exclude Repo, Evidence and Notes. The filename is built from the sanitised product name only.
*   **No secrets in logs, errors or responses.** The `MINIO_ACCESS_KEY` and `MINIO_SECRET_KEY` never leave `config`/`objectstore`; errors from minio-go are logged after stripping the request headers, and the client gets a fixed message.
*   **Path safety.** Keys are built only from integers and a closed whitelist, and `objectstore` re-validates them (prefix, `..`, control characters) before any call.
*   **Force abuse.** Any reader may use `force=1` (decision 9). The 30 s cooldown plus singleflight bound the work an authenticated reader can cause. Generation is cheap, so no per-user rate limiter is added. While MinIO is down the cooldown cannot be evaluated (no `LastModified`), so every request generates; singleflight still collapses concurrent ones.
*   **Injection into spreadsheets** is handled above.
*   **Rule 6 (no new public ports):** no port is published and no new service is added; the backend makes an outbound connection to the MinIO endpoint only.
*   **Rule 2 (LLM only through ai-manager):** this feature makes no LLM call.

### Dashboard
*   `dashboard/src/views/knowledge/KnowledgeView.tsx`: add a `btn btn-secondary` button, a `Download` lucide icon plus "Download checklist", in the header next to the draft buttons. The current draft buttons sit inside `{canWrite && summary.configured && (...)}`. The new button must be **outside the `canWrite` guard**, so read-only roles see it, and is shown only when `summary.configured` **and the Modules tab (`?tab=modules`) is active** (decision 10). It opens the dialog.
*   New `dashboard/src/views/knowledge/ChecklistDialog.tsx`, built on `components/Modal.tsx` and `components/SubmitButton.tsx`:
    *   Format: radio group, PDF (default), Excel, CSV.
    *   Checkbox "Add last update to Status column"; helper text: "Dates before tracking began, or taken from the catalogue, are approximate and marked with ~."
    *   Checkbox "Force regenerate (ignore cached file)".
    *   Submit "Download" with a loading state and an inline error (the 409 and 503 messages are shown as the server wrote them). After success, a small note from `X-Export-Cache`: "Served from cache" (`hit`), "Generated now" (`miss`, `regenerated`), or "Generated now; the cache is unavailable" (`bypass`).
*   `dashboard/src/views/knowledge/api.ts`: `downloadChecklist(pid, opts)`: `fetch(url, { credentials: 'include' })`, read the blob, take the filename from `Content-Disposition` (falling back to a generated name), and trigger the save through a temporary object URL that is revoked afterwards. No new npm dependency. A non-2xx response is parsed as `{error}`.
*   `dashboard/README.md`: add the new API path to the table (`views/knowledge/api.ts`) and a sentence on the dialog.

### PDF layout (reference)
The visual acceptance reference is `HISV3-MODULE-CHECKLIST.pdf`: about 19 pages, 440 feature rows for the catalogue it was built from. The synced knowledge base held 444 features at `521618a` (`PRODUCT_KNOWLEDGE_SYNC.md`), so the exact counts differ from the PDF's by design; the test asserts "row count equals the number of active synced features of the snapshot", not 440.

## Accepted limitations
1.  **Backfilled dates are approximate** (see "Backfill"). Only dates produced by real syncs after this ships are exact to the sync that observed the change, not to the commit that made it (a sync every 6 h can merge several commits).
2.  **Cache growth.** Each succeeded sync creates up to 6 objects per product (3 formats x 2 option variants, if both are used) under a new key, and nothing deletes the old ones (decision 11). Sizes are small (a few hundred KB). **Future item:** retention restricted to the prefix `qa-automation/knowledge-exports/`, which needs `List` and `Delete` on `objectstore` and a separate amendment.
3.  **A manual sync of an unchanged commit invalidates the cache** for the product (new succeeded sync id, identical content).
4.  **Helvetica/cp1252 only.** Text outside cp1252 is rendered as `?` in the PDF (not in XLSX/CSV). The HISv3 catalogue is Latin (Indonesian) with em dashes.
5.  **Shared staging MinIO** (see "Cross-environment dependency"). While it is down, every export is generated on the fly (`bypass`).
6.  `force=1` is ignored within 30 s of the last write of that object.

## Open Questions
None. The seven questions of rev 1 were answered by the human on 2026-11-17 (Decisions 6 to 12). Future items: retention of old cache objects (needs `List`/`Delete`), and a bundled TTF font if a product with non-Latin text is exported.

## Assumptions Goldfish reviewers must challenge
1.  **The grouping in `Syncer.Tree` is the right and complete source** for the checklist: parts, modules, sub-modules, and features (including features with no sub-module, which `Tree` returns as `features_without_submodule`). Verify that excluding removed rows and only exporting `origin='sync'` is correct, and that no local feature rows exist.
2.  **The sync commits entity rows and `status='succeeded'` in one transaction** (verified in `sync.go`), so reading the latest succeeded sync id and the features in one explicit REPEATABLE READ read-only transaction yields a consistent snapshot. Challenge: does the MySQL driver and connection (`db.NewConnection`, `NormalizeDSN`) honour `TxOptions{ReadOnly, Isolation}`, and does the snapshot begin at the first read?
3.  **`changed_sync_id` is set on every full write and not on location-only refreshes** (verified in `store.go` `upsert`), so a backfilled date can be later than the real status change, never earlier than the last content change. Challenge the "never earlier" part.
4.  **History rows are written only in `apply`'s add/restore/status-differs cases**, which covers every way `release_status` can change. Challenge: can `release_status` change through any other path (the API for local rows, a manual SQL, a parser-version bump that rewrites rows)? A parser-version bump rewrites hashes (`e.hash != r.hash()`), which is an `updated` event, and the status comparison must not create spurious history rows when the status is the same.
5.  **The unique key `(product_id, feature_code, sync_id, source)` is enough for idempotence.** Challenge: two rows for one feature in one sync (a duplicate feature id in the catalogue, last wins) must not break the sync transaction.
6.  **The 30 s force cooldown and per-key singleflight are enough** abuse protection for authenticated readers, who may all use `force=1`.
7.  **Prefix-only isolation in the shared bucket** is acceptable because `objectstore` exposes no `List`/`Delete`. Challenge whether a shared credential with full rights on a shared bucket is acceptable for a production service at all, versus a dedicated bucket or a scoped access key (an infra decision this spec cannot take).
8.  **A manual sync of an unchanged commit is `succeeded`** and so changes the cache key (verified in `sync.go`: the `skipped` path requires `trigger == TriggerSchedule`). Accepted.
9.  **The plan's PDF claims**: core Helvetica plus the cp1252 translator is enough for the catalogue text, and go-pdf/fpdf can produce a TOC with page numbers and a repeating table header. A Goldfish with the actual `HISV3-MODULE-CHECKLIST.pdf` should compare. (That PDF is not in the repo; I have not seen it.)
10. **No CORS header change is needed** because the dashboard is served same-origin.
12. **Streaming an uncached file on any MinIO error (`bypass`) is acceptable**, including that a failing MinIO is visible only in logs and the `X-Export-Cache` header.
13. **`status_as_of` outranks `backfill` but not `sync`, and is used only when it parses as an ISO date** (decision 6). Challenge: the catalogue cell may hold a date that means something other than "status last changed" (for example a review date), which would then be shown as a status date, marked `~`. The cell format in production data has not been inspected.
14. **The sanitised `products.name` is safe** in `Content-Disposition` and in the PDF.
11. **The endpoint is reachable from rnd-v3** and, by extension, from the backend container: asserted by the requester, not by me.

## Target Files
*   **Spec PR:** `docs/designs/MODULE_CHECKLIST_EXPORT.md`, `docs/designs/SITEMAP.md` (this row), `docs/designs/PRODUCT_KNOWLEDGE_SYNC.md` (one "Amendment 3" pointer to this spec, no other change).
*   **Backend:**
    *   new `ai-backend/internal/services/objectstore/` (`objectstore.go`, `minio.go`, `fake_test.go` (test helper), `objectstore_test.go`, `README.md`)
    *   new `ai-backend/internal/services/knowledge/export_checklist.go`, `export_pdf.go`, `export_xlsx.go`, `export_csv.go`, `export_checklist_test.go`
    *   `ai-backend/internal/services/knowledge/store.go` (`apply`: read the stored `release_status` for features and insert history rows), `ai-backend/internal/services/knowledge/store_history_test.go` (new)
    *   `ai-backend/internal/services/knowledge/sync.go` (pass the commit SHA into `apply` only if it is not already reachable there)
    *   `ai-backend/internal/services/knowledge/query.go` (only if the grouping is extracted from `Tree`; `Tree`'s output must not change)
    *   `ai-backend/internal/services/knowledge/README.md`
    *   `ai-backend/internal/db/migrations.go` (new table, backfill; appended only), `ai-backend/internal/db/README.md`
    *   new `ai-backend/internal/api/knowledge_export.go`, `knowledge_export_test.go`; `ai-backend/internal/api/knowledge.go` (one `case` in `handleKnowledge`); `ai-backend/internal/api/router.go` (a field and setter for the store only); `ai-backend/internal/api/README.md`
    *   `ai-backend/internal/config/config.go`, `ai-backend/cmd/server/main.go`
    *   `ai-backend/go.mod`, `ai-backend/go.sum` (new deps: `github.com/minio/minio-go/v7`, `github.com/go-pdf/fpdf`, `github.com/xuri/excelize/v2`, `golang.org/x/sync`)
    *   `ai-backend/.env.example`, `deploy/his-v3-qa/.env.prod.example` (names only)
*   **Dashboard:** `dashboard/src/views/knowledge/KnowledgeView.tsx`, new `dashboard/src/views/knowledge/ChecklistDialog.tsx`, `dashboard/src/views/knowledge/api.ts`, `dashboard/README.md`.
*   **E2E (optional, see Verification 6):** `this-qa-automation-system-playwright/tests/knowledge.spec.ts` (extend) or a new `knowledge-export.spec.ts`, following that directory's conventions.
*   **Not changed:** the executor, Dockerfile, `docker-compose.prod.yml`, nginx, `lib/`, the sync parser (`hisv3.go`, `catalog.go`), the `Tree` JSON shape.

## Verification Plan
1.  `cd ai-backend && gofmt -l <touched files>`, then `go build -mod=mod ./... && go vet -mod=mod ./... && go test -mod=mod ./...`.
2.  **Unit tests (no DB, no MinIO):**
    *   `objectstore`: key validation (reject outside prefix, `..`, leading `/`, backslash, control chars); the interface has no list/delete.
    *   Renderers over a small hand-built snapshot **and** over a snapshot built from `testdata/hisv3-521618a`: non-empty output with the right magic bytes (`%PDF-`, `PK`, BOM); the row count in CSV and XLSX equals the number of active features; with `last_update=1` the date strings are present, and `—`/empty for unknown; formula-injection cells are neutralised; determinism (rendering twice yields identical CSV and XLSX cell content).
    *   Date precedence (`pickStatusDate`), a table test: `sync` row present wins over a valid `status_as_of` and a `backfill` row; no `sync` row and a valid ISO `status_as_of` wins over `backfill`; no `sync` row and a **non-date** `status_as_of` (`""`, `"TBD"`, `"01/09/2026"`, `"2026-13-45"`) falls through to `backfill`; only `backfill` gives `backfill`; nothing gives a dash; `sync` and `backfill` rows with a status other than the current one are ignored; the marks (`~` and the `Sumber tanggal` value) are right for each source in all three renderers.
    *   Cache logic against the fake store: miss then hit (second call does not render, counted through an injected renderer), `force=1` then `regenerated`, force within the cooldown becomes `hit`, a new sync id gives a different key, a changed `TEMPLATE_VERSION` gives a different key, MinIO `Stat`/`Get`/`Put` errors give a `bypass` 200 with the generated file, nil store gives 503, concurrent calls render once (race detector: `go test -race`).
    *   Handler: 405, 400 on bad `format`/`last_update`/`force`, 404, 409, 503, headers (`Content-Type`, `Content-Disposition`, `X-Export-Cache`), read-only role gets 200.
3.  **DB tests** (run when `QA_TEST_MYSQL_DSN` is set, like the existing sync tests), using the `testdata/hisv3-521618a` fixtures:
    *   a first sync writes one history row per feature (`source='sync'`, `prev_status` NULL);
    *   a second sync of the same commit writes none;
    *   change one feature's status in a copied fixture and sync again: exactly one new row with the right `prev_status`;
    *   change only a feature's notes: no history row;
    *   remove then restore a feature: a restore row;
    *   the backfill statement on a database that has features but no history inserts one row per active feature with `source='backfill'`, and a second run inserts none;
    *   the snapshot read sees one consistent sync id and feature set while a sync is committing.
4.  **Migration:** run `go run -mod=mod ./cmd/databasemigration` against a dev schema, then a second time and confirm it changes nothing.
5.  `cd dashboard && npx tsc --noEmit && npm run lint && npm run build`.
6.  **Staging-MinIO check** (needs WireGuard; credentials loaded into env only, never printed): with `MINIO_*` set, download PDF: header `miss`, the object exists under `qa-automation/` (key printed, no listing outside the prefix); again: `hit`, and a render counter or the log shows no regeneration; tick Force: `regenerated`; run a sync: a new key; repeat for xlsx and csv. Confirm from **inside the backend container** that the endpoint is reachable. With `MINIO_*` unset: the export answers 503 and every other page works.
7.  **Visual check** against `HISV3-MODULE-CHECKLIST.pdf` (colours, TOC, header, footer, repeating table header, page count in the same range). Open the XLSX in LibreOffice (frozen header, filter, sheet names) and the CSV in a spreadsheet (BOM, accents, em dash).
8.  **Deploy order** (a separate approval, hard rule 8): apply the migration against prod `qa_automation` first; put the `MINIO_*` values in `.env.prod` on rnd-v3 (never printed); then deploy the binary. After deploy, through `qa-tool-session` as `ai-service` (reads only, no impersonated writes needed; `GET` of the export writes only cache objects): request the export and check `X-Export-Cache`.
