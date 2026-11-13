# db

## Overview

Package `db` owns the MySQL connection for ai-backend and the full schema DDL.
It wraps `database/sql` (driver `github.com/go-sql-driver/mysql`) in a small
`DB` struct whose method signatures mimic pgx, so call sites written for the
earlier PostgreSQL (Neon) backend keep working after the move to MySQL.

Schema migrations are an in-code ordered list of SQL statements. The only
production caller of `RunMigrations` is the dedicated command
`ai-backend/cmd/databasemigration/main.go`; `cmd/server` opens a connection via
`NewConnection` but does not run migrations. The `api` package's integration
test (`internal/api/crud_integration_test.go`) also calls `RunMigrations`. The
directory `ai-backend/internal/migrations` exists but is empty and is not
referenced by any Go code.

## Interface

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
  the unexported `migrations` slice in order.
- Tables created by `migrations.go:migrations`: `roles`, `flows`, `steps`,
  `ai_cache`, `embedding_cache`, `learning_cache`, `skills`, `skill_logs`,
  `learned_skills`, `raw_issues`, `issue_suggestions`, `request_logs`,
  `flow_outcomes`, `job_runs`, `goal_events`, `navigation_edges`,
  `action_sequences`, `context_cache`, `users`, `sessions`, `products`, `product_settings`,
  `product_repos`, `product_repo_branches`.
- `connection.go:DB.WithTx(ctx, fn func(*sql.Tx) error)` - commits on nil, rolls back otherwise, and retries the whole
  function up to 3 times on MySQL error 1213 (deadlock) or 1205 (lock wait timeout); `IsRetryableTxError` is the predicate.

## Invariants

- `connection.go:NewConnection` returns an error (and opens nothing) when
  `GetDatabaseURL()` is empty, and closes the pool if the 10-second startup
  `PingContext` fails.
- Pool limits are fixed in `connection.go:NewConnection`: `SetMaxOpenConns(25)`,
  `SetMaxIdleConns(5)`, `SetConnMaxLifetime(30m)`, `SetConnMaxIdleTime(5m)`.
- Every statement in `migrations.go:migrations` must stay idempotent: all DDL
  uses `CREATE TABLE IF NOT EXISTS`, and the three `UPDATE raw_issues ...`
  back-fill statements only touch rows whose `page`/`action`/`severity` is
  blank. Re-running `RunMigrations` against an existing database is therefore
  safe. New migrations must be appended to the slice and must also be safe to
  re-run, because no applied-version table is kept.
- `migrations.go:RunMigrations` stops at the first failing statement and
  returns `migration <index> failed` with the first non-empty SQL line
  (`migrations.go:summarize`). Earlier statements are not rolled back.
- All tables use `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci`.
- The header comment in `migrations.go` mentions `ensureCheckConstraint`, but no
  such function exists in the codebase. Do not rely on it.

## Product repos tables (PRODUCT_REPOS.md)

`product_settings` (flag `allow_non_automation_work`, missing row = false), `product_repos` (one row per repo; `main_flag`/`central_flag` are stored generated columns so `UNIQUE(product_id, flag)` allows at most one main and one central repo per product) and `product_repo_branches` (binary-collation `branch` and `slug`; at most one default per repo via `default_flag`). Migration entries 4 and 5 back-fill one main repo and its branch per legacy product; they skip products whose `root_dir` has a `.branches` segment or repeats a repo's `root_dir`, and every re-run is a no-op. The legacy `products` columns are kept (mirrored from the main repo) for rollback.

**Pre-flight** (run before migrating; fix rows first):
```sql
SELECT id, name, root_dir FROM products WHERE root_dir REGEXP '(^|/)[^/]*\\.branches(/|$)';
SELECT a.id, b.id FROM products a JOIN products b ON a.id < b.id
 AND (b.root_dir LIKE CONCAT(a.root_dir,'/%') OR a.root_dir LIKE CONCAT(b.root_dir,'/%'));
```
**Post-flight** (expected: zero rows):
```sql
SELECT p.id, p.name FROM products p WHERE NOT EXISTS (SELECT 1 FROM product_repos r WHERE r.product_id = p.id);
```
**Deploy order:** run `go run -mod=mod ./cmd/databasemigration` before starting the new binary (the old binary ignores the new tables), then once more after the new binary is live to pick up products the old binary created in the window.

**Tests:** `internal/testdb` gives DB-backed tests a scratch schema from `QA_TEST_MYSQL_DSN` (tests skip when empty; a DSN naming `qa_automation` is refused).

## Side Effects

- `connection.go:NewConnection` opens a TCP MySQL connection pool to the host in
  the DSN and performs a network ping.
- `migrations.go:RunMigrations` executes DDL and `UPDATE` statements against the
  connected database (schema creation and `raw_issues` back-fill) and writes
  progress lines through the standard `log` package.

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/db/...` (the package has
  no test files. Migrations are exercised by `internal/api/crud_integration_test.go`)
- Apply migrations: `cd ai-backend && go run ./cmd/databasemigration`
