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
  `action_sequences`, `context_cache`, `users`, `sessions`, `products`,
  `active_impersonations`, `impersonation_audit` (append-only; never purge).

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
