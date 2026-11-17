# services/knowledge

## Overview
Product knowledge base (spec: [PRODUCT_KNOWLEDGE_SYNC.md](../../../../docs/designs/PRODUCT_KNOWLEDGE_SYNC.md)). It imports a product's catalog (parts, modules, sub-modules, features, epics, user stories, test cases) from its **automation repo** `docs/` into `kb_*` tables, keeps a sync history with a diff, problems and an AI change summary, and serves the data to `/api/products/{id}/knowledge/*`.

## Interface
- `Format` (`catalog.go`): `Name()`, `Version()`, `Parse(files) (*Snapshot, []Problem, error)`; `Lookup`, `Names`, `Enums`. Registered: `hisv3-test-catalog-v1` (`hisv3.go`), a port of `automation-test-hisv3/scripts/test-catalog.mjs` at `521618a` (duplicates last-wins, unknown-status rows skipped, out-of-heading features kept).
- `Syncer` (`sync.go`): `NewSyncer(db, pool, ai)`, `Start(ctx, productID, trigger, userID)`, `FailOrphanedSyncs`, `StartScheduler(ctx, interval)`. Errors: `ErrProductNotFound`, `ErrNotConfigured`, `ErrUnknownFormat`, `ErrNoAutomation`, `ErrUnavailable`, `ErrRunning{SyncID}`.
- Hooks (`sync.go`): `Syncer.OnSucceeded(productID)` runs after a sync committed, `Syncer.OnUnchanged(productID)` after a scheduled sync skipped for an unchanged commit. Set at startup before `StartScheduler`; they must return at once. `knowledge` does not import the GAOS pusher (`cmd/server/main.go` wires them to `gaossync`).
- Queries (`query.go`): `Summary`, `Tree`, `Syncs`, `Sync`, `Epics`, `Stories`, `TestCases`, `Entity`, `PutAnnotation`, `DeleteAnnotation`, `SaveDraft`, `DeleteDraft`, `ValidFormat`. Errors: `ErrNotFound`, `*InputError` (400), `*ConflictError` (409).
- Coverage (`coverage.go`): `CoverageBy`, `Features`, `Modules`. Assistant files (`assistants.go`): `ParseAssistants`, `KeepForSync`; `Assistants` (query.go) lists them per product or across products.
- Storage (`store.go`): `apply(ctx, tx, productID, syncID, sha, firstSync, snap, prob)` (diff + batched upsert + soft delete + link-set diff + feature status history), `countActive`, `summarizeProblems`.

## Invariants
- The sync reads only through `repopool.ReadTree` (a sync-owned bare mirror); it never writes to a product clone or remote.
- One running sync per product (`kb_syncs.running_flag` unique key). Terminal status writes use a fresh context; `FailOrphanedSyncs` at startup fails `running` syncs and `pending` summaries.
- Entity rows, links, counts, diff, problems and `status='succeeded'` are written in one transaction. A catalog with no features and no test cases fails the sync (nothing is soft-deleted).
- Unchanged rows are not written; location-only changes update `source_file`/`source_line` without a diff entry. Same commit + parser version gives an empty diff (`diff.unchanged`); a scheduled sync of the same `(repo, branch, format, parser_version, commit)` is `skipped`.
- Status history ([MODULE_CHECKLIST_EXPORT.md](../../../../docs/designs/MODULE_CHECKLIST_EXPORT.md)): `apply` writes `kb_feature_status_history` rows in the sync's transaction: a feature added (`source='sync'`, or `'backfill'` in the product's first sync), restored or updated with a different `release_status` (with `prev_status`). A restore or update with the same status, a parser-version hash rewrite and a removal write nothing. Writes use `ON DUPLICATE KEY UPDATE id = id`.
- Synced rows are read-only through the API; local drafts use `L-` codes and are hard-deleted. Annotations are keyed by code and survive re-syncs.
- A hook never fails or delays a sync: a panic in it is recovered and logged.
- The AI summary is the only LLM call, made through `*ai.Service` (ai-manager → claude-v3) after the commit with its own 3 min timeout; its failure never fails a sync.

## Side Effects
Writes `kb_*` tables and `product_settings.knowledge_format` (via the products API); runs git through `repopool`; one LLM call per non-empty sync.

## Verification
`go test -mod=mod ./internal/services/knowledge/` (golden parse of `testdata/hisv3-521618a`, rules). With `QA_TEST_MYSQL_DSN`: sync lifecycle (idempotency, remove/restore, annotations, AI failure, empty catalog), concurrency/orphans, seed migration, status history (`store_history_test.go`: first sync, new feature, status change, notes-only, hash rewrite, restore, and the backfill statement re-run).
