# db

## Overview

Package `db` owns the MySQL connection for ai-backend and the full schema DDL.
It wraps `database/sql` (driver `github.com/go-sql-driver/mysql`) in a small
`DB` struct with context-first `Query`/`QueryRow`/`Exec` helpers. All SQL is
MySQL 8 syntax (`?` placeholders, no `RETURNING`; use `ExecResult.LastInsertId`).

Schema migrations are an in-code ordered list of SQL statements. The only
production caller of `RunMigrations` is the dedicated command
`ai-backend/cmd/databasemigration/main.go`; `cmd/server` opens a connection via
`NewConnection` but does not run migrations. The `api` package's integration
test (`internal/api/crud_integration_test.go`) also calls `RunMigrations`. The
directory `ai-backend/internal/migrations` exists but is empty and is not
referenced by any Go code.

## Interface

- `connection.go:NormalizeDSN(dsn string) (string, error)`: pins a MySQL DSN to UTC (see Invariants). `NewConnection` applies it.

- `connection.go:DB` - struct wrapping `SQL *sql.DB`.
- `connection.go:NewConnection(cfg *config.Config) (*DB, error)` - builds the
  DSN with `config.Config.GetDatabaseURL` (see [config](../config/README.md)),
  opens the pool, and pings it.
- `connection.go:DB.Query` / `DB.QueryRow` / `DB.Exec` - context-aware proxies
  to `QueryContext` / `QueryRowContext` / `ExecContext`.
- `connection.go:ExecResult` - wraps `sql.Result`; `RowsAffected()` and
  `LastInsertId()` return `int64` and yield `0` on error or nil result.
- `connection.go:DB.Ping(ctx)` - returns an error if the receiver or `SQL` is nil.
- `connection.go:DB.Close()` - nil-safe close of the underlying pool.
- `migrations.go:RunMigrations(sqlDB *sql.DB) error` - executes every entry of
  the unexported `migrations` slice in order (`RunMigrationList` runs an explicit list; used by tests).
  An entry whose first line is `-- skip-if: <query>` is guarded (see Invariants).
- Tables created by `migrations.go:migrations`: `roles`, `flows`, `steps`,
  `ai_cache`, `embedding_cache`, `learning_cache`, `skills`, `skill_logs`,
  `learned_skills`, `raw_issues`, `issue_suggestions`, `request_logs`,
  `flow_outcomes`, `job_runs`, `goal_events`, `navigation_edges`,
  `action_sequences`, `context_cache`, `users`, `sessions`, `products`, `product_settings`,
  `product_repos`, `product_repo_branches`, `job_events` and `case_approvals` (CASE_AUTOMATION.md: the live log and the approval gate of case jobs; HISTORY_ANSWER.md adds `answer_source`, `answer_actor`, `answer_source_detail`: where each answer came from), `active_impersonations`, `impersonation_audit` (append-only; never purge). KNOWLEDGE_TRANSLATION.md adds `users.language`, `kb_translations`, `kb_translation_runs`, `kb_translation_failures` and widens `ai_conversations.kind` to `VARCHAR(16)` (each guarded by `skip-if` or `IF NOT EXISTS`).
- `connection.go:DB.WithTx(ctx, fn func(*sql.Tx) error)` - commits on nil, rolls back otherwise, and retries the whole
  function up to 3 times on MySQL error 1213 (deadlock) or 1205 (lock wait timeout); `IsRetryableTxError` is the predicate.

## Invariants

- Every connection is pinned to UTC: `connection.go:NormalizeDSN` forces `loc=UTC`, `parseTime=true` and session `time_zone='+00:00'` on any DSN, including verbatim `DATABASE_URL` / `connection_string` values. Go `time.Time` values, `NOW()`/`CURRENT_TIMESTAMP` and `UTC_TIMESTAMP()` therefore agree on any host time zone. `NormalizeDSN` rejects non-MySQL DSNs.

- `connection.go:NewConnection` returns an error (and opens nothing) when
  `GetDatabaseURL()` is empty, and closes the pool if the 10-second startup
  `PingContext` fails.
- Pool limits are fixed in `connection.go:NewConnection`: `SetMaxOpenConns(25)`,
  `SetMaxIdleConns(5)`, `SetConnMaxLifetime(30m)`, `SetConnMaxIdleTime(5m)`.
- `qa_defect_state`, `qa_defect_push` and `product_service_repos` belong to the defect push (QA_DEFECT_TRIAGE.md).
- Every statement in `migrations.go:migrations` must stay idempotent, either by
  construction (`CREATE TABLE IF NOT EXISTS`, `INSERT ... ON DUPLICATE KEY`,
  DML guarded by `NOT EXISTS`/`IS NULL`/blank checks such as the three
  `UPDATE raw_issues ...` back-fills and the automation back-fill) or by a
  leading `-- skip-if: <query>` guard. Re-running `RunMigrations` against an
  existing database is therefore safe. New migrations must be appended to the
  slice and must also be safe to re-run, because no applied-version table is kept.
- **`-- skip-if:` grammar** (`migrations.go:parseSkipIf`): after leading whitespace,
  newlines and a UTF-8 BOM, the first line must start with `-- skip-if:`
  (case-sensitive, one space after `--`); the rest of the line is the query.
  The runner runs it outside any transaction; it must return exactly one row and
  one non-NULL integer. `> 0` skips the entry, `0` runs it; NULL, no row, several
  rows, an empty query or a query error abort with
  `migration <i> skip-if guard failed`. The whole entry text (comment included)
  is still sent to MySQL. A guarded entry runs on one pinned connection after
  `SET SESSION lock_wait_timeout = 30`. The guard check and the statement are not
  atomic across two concurrent runners (the runner is a manual command). No
  other entry starts with `-- skip-if:`.
- `migrations.go:RunMigrations` stops at the first failing statement and
  returns `migration <index> failed` with the first non-empty SQL line that is not
  a `-- skip-if:` directive (`migrations.go:summarize`). Earlier statements are not rolled back.
- All tables use `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci`.

## Product repos tables (PRODUCT_REPOS.md)

`product_settings` (flag `allow_non_automation_work`, missing row = false), `product_repos` (one row per repo; `main_flag`/`central_flag`/`automation_flag` are stored generated columns so `UNIQUE(product_id, flag)` allows at most one main, one automation and one central repo per product) and `product_repo_branches` (binary-collation `branch` and `slug`; at most one default per repo via `default_flag`). Migration entries 4 and 5 back-fill one main repo and its branch per legacy product; they skip products whose `root_dir` has a `.branches` segment or repeats a repo's `root_dir`, and every re-run is a no-op. The legacy `products` columns are kept (mirrored from the **automation** repo) for rollback.

**Automation role (PRODUCT_REPOS_AUTOMATION_ROLE.md).** Two entries are appended after `impersonation_audit`: `product_repos_add_automation_role` (guarded by `-- skip-if:` on `automation_flag` in `information_schema.columns`; one `ALTER TABLE product_repos ALGORITHM=COPY, LOCK=SHARED` adds `is_automation TINYINT(1) NOT NULL DEFAULT 0`, the stored generated `automation_flag` and `UNIQUE KEY uq_product_repos_automation (product_id, automation_flag)`; the column default keeps the previous binary's inserts working) and `product_repos_backfill_automation_role` (a self-join `UPDATE` that makes the main repo automation only for products with no automation repo yet; a no-op on re-run and where automation differs from main). Re-running the migration heals a product left without an automation repo (main becomes automation).

**Pre-flight** (run before migrating; fix rows first):
```sql
SELECT id, name, root_dir FROM products WHERE root_dir REGEXP '(^|/)[^/]*\\.branches(/|$)';
SELECT a.id, b.id FROM products a JOIN products b ON a.id < b.id
 AND (b.root_dir LIKE CONCAT(a.root_dir,'/%') OR a.root_dir LIKE CONCAT(b.root_dir,'/%'));
```
**Post-flight** (expected: zero rows):
```sql
SELECT p.id, p.name FROM products p WHERE NOT EXISTS (SELECT 1 FROM product_repos r WHERE r.product_id = p.id);
-- roles: every product with repos has exactly one main and exactly one automation repo
SELECT p.id, SUM(r.is_main) AS mains, SUM(r.is_automation) AS automations
FROM products p JOIN product_repos r ON r.product_id = p.id
GROUP BY p.id HAVING mains <> 1 OR automations <> 1;
-- legacy drift: all six legacy columns equal the automation repo's
SELECT p.id FROM products p
JOIN product_repos r ON r.product_id = p.id AND r.is_automation = 1
LEFT JOIN product_repo_branches b ON b.repo_id = r.id AND b.is_default = 1
WHERE NOT (p.repo_url <=> r.repo_url AND p.root_dir <=> r.root_dir
  AND p.branch <=> COALESCE(b.branch, (SELECT MIN(x.branch) FROM product_repo_branches x WHERE x.repo_id = r.id), '')
  AND p.playwright_accessible <=> r.playwright_accessible AND p.last_verified_at <=> r.last_verified_at
  AND p.verify_report <=> r.verify_report);
```
**Deploy order:** run `go run -mod=mod ./cmd/databasemigration` before starting the new binary (the new binary selects `is_automation`; without the column every repo read is a 500; the old binary ignores the new tables and column), then once more after the new binary is live (in a quiet window, no dashboard edits) to pick up products the old binary created in the window.

**Rollback to the previous release** ignores the column and key, but its main promotion does not touch `is_automation`, and it can delete the repo that is automation (a sibling when main differs), after which the heal silently makes main the automation repo. Before rolling back save `SELECT product_id, id, name FROM product_repos WHERE is_automation = 1`; after rolling forward run the migration, compare that query, then re-point the legacy columns (quiet window, no product edits) and run the post-flight queries:
```sql
UPDATE products p
JOIN product_repos r ON r.product_id = p.id AND r.is_automation = 1
LEFT JOIN product_repo_branches b ON b.repo_id = r.id AND b.is_default = 1
SET p.repo_url = r.repo_url, p.root_dir = r.root_dir,
    p.branch = COALESCE(b.branch, (SELECT MIN(x.branch) FROM product_repo_branches x WHERE x.repo_id = r.id), ''),
    p.playwright_accessible = r.playwright_accessible, p.last_verified_at = r.last_verified_at,
    p.verify_report = r.verify_report;
```
Concurrent racing is covered by the unique key: the loser of a promotion race gets 1062 on `uq_product_repos_automation`, mapped to 409.

## Knowledge base tables (PRODUCT_KNOWLEDGE_SYNC.md)
- `kb_syncs`: one row per sync attempt; `running_flag` (generated from `status`) + `UNIQUE(product_id, running_flag)` allows one running sync per product. `counts`/`diff`/`problems` JSON are written in the same transaction as the entity rows; `ai_status` tracks the post-commit summary.
- Entity tables `kb_parts`, `kb_modules`, `kb_submodules`, `kb_features`, `kb_epics`, `kb_stories`, `kb_test_cases`: `UNIQUE(product_id, code)`, every code column `utf8mb4_bin`, links by code; `origin` sync|local, `content_hash` (without location), `removed_at`/`removed_sync_id` soft delete.
- `kb_story_features` (link index, `origin`), `kb_annotations` (keyed by `(product_id, entity_type, entity_code)`, survives re-syncs).
- `product_settings.knowledge_format`: `NULL` never set, `''` off. Added by a guarded ALTER; a one-time guarded seed sets SUT to `test-catalog-v1` while no product has a value.

- `product_settings.gaos_integrated`, `gaos_product_id`, `gaos_last_sync_at`, `gaos_last_status` (`ok`/`failed`/`partial`), `gaos_last_note` (GAOS_INTEGRATION.md): one guarded ALTER, so a re-run is a no-op and user values survive; a missing settings row reads the defaults (off, empty).
- `product_settings.gaos_push_requested_at` (`DATETIME(6) NULL`, the durable "a push is owed" marker; `NULL` = nothing owed) and `gaos_push_attempts` (`INT NOT NULL DEFAULT 0`, failed passes since the marker was set) (GAOS_INTEGRATION.md, "Durable delivery"): one guarded ALTER (guard on the first column).
- `kb_feature_status_history` (MODULE_CHECKLIST_EXPORT.md): one row per release-status change of a synced feature (`source` `sync`) or per starting status (`backfill`, approximate date). `feature_code` is `utf8mb4_bin` like `kb_features.code`; no FK to `kb_features`. `UNIQUE(product_id, feature_code, sync_id, source)` with `ON DUPLICATE KEY UPDATE id = id` makes sync writes idempotent. The table is followed by an **unguarded** backfill `INSERT ... SELECT` whose status-aware `NOT EXISTS` is its only guard: a second run is a no-op, and a run after the deploy window inserts a row for any status changed in between. Run the migration once before and once after the deploy that introduces the table.
- `kb_assistants` (agents, skills, rules, commands, instructions; same entity semantics); `kb_annotations.entity_type` includes `assistant`; `skills.product_id` (NULL = global QA check). All three later changes are guarded (`-- skip-if:`).

**Tests:** `internal/testdb` gives DB-backed tests a scratch schema from `QA_TEST_MYSQL_DSN` (tests skip when empty; a DSN naming `qa_automation` is refused).

## Side Effects

- `connection.go:NewConnection` opens a TCP MySQL connection pool to the host in
  the DSN and performs a network ping.
- `migrations.go:RunMigrations` executes DDL and `UPDATE` statements against the
  connected database (schema creation and `raw_issues` back-fill) and writes
  progress lines through the standard `log` package.

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/db/...` (pure tests: `parseSkipIf`, `summarize`;
  with `QA_TEST_MYSQL_DSN` set, the full slice twice, the automation back-fill/heal, the runner guard and the unique keys;
  migrations are also exercised by `internal/api/crud_integration_test.go`)
- Apply migrations: `cd ai-backend && go run ./cmd/databasemigration`
