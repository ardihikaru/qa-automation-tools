package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/config"
)

// DB wraps the MySQL *sql.DB and exposes context-first Query/QueryRow/Exec
// helpers. Connection lifecycle (Ping, Close) lives here; query execution
// lives on this struct.
type DB struct {
	SQL *sql.DB
}

// ExecResult wraps sql.Result to expose RowsAffected/LastInsertId without
// requiring callers to handle a second error return.
type ExecResult struct {
	inner sql.Result
}

// RowsAffected returns the number of affected rows (0 on error).
func (e ExecResult) RowsAffected() int64 {
	if e.inner == nil {
		return 0
	}
	n, _ := e.inner.RowsAffected()
	return n
}

// LastInsertId returns the AUTO_INCREMENT id assigned by MySQL (0 on error).
func (e ExecResult) LastInsertId() int64 {
	if e.inner == nil {
		return 0
	}
	n, _ := e.inner.LastInsertId()
	return n
}

// Query proxies to QueryContext.
func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.SQL.QueryContext(ctx, query, args...)
}

// QueryRow proxies to QueryRowContext.
func (db *DB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.SQL.QueryRowContext(ctx, query, args...)
}

// Exec proxies to ExecContext and wraps the result in ExecResult.
func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (ExecResult, error) {
	res, err := db.SQL.ExecContext(ctx, query, args...)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{inner: res}, nil
}

// NewConnection opens the MySQL pool described by cfg.GetDatabaseURL()
// (by default the shared db-host MySQL at 10.8.0.4:5306, schema qa_automation).
// NormalizeDSN pins every connection to UTC, whatever the host time zone or
// the DSN supplied (DATABASE_URL / connection_string are used verbatim):
//   - loc=UTC: Go time.Time values are written and read as UTC;
//   - time_zone='+00:00': NOW()/CURRENT_TIMESTAMP in this session are UTC,
//     matching UTC_TIMESTAMP();
//   - parseTime=true: DATETIME scans into time.Time.
//
// Without this, a non-UTC host (loc=Local) stored Go timestamps in local
// time while queries compared them against UTC_TIMESTAMP(), so e.g. sessions
// on a UTC+7 host lived 7 hours longer than their TTL.
func NormalizeDSN(dsn string) (string, error) {
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid MySQL DSN: %w", err)
	}
	c.Loc = time.UTC
	c.ParseTime = true
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	c.Params["time_zone"] = "'+00:00'"
	return c.FormatDSN(), nil
}

func NewConnection(cfg *config.Config) (*DB, error) {
	dsn := cfg.GetDatabaseURL()
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is empty: check DB_HOST/DB_USER or DATABASE_URL")
	}
	dsn, err := NormalizeDSN(dsn)
	if err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping mysql: %w", err)
	}

	return &DB{SQL: sqlDB}, nil
}

// Ping verifies the connection is alive. Used by integration tests.
func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.SQL == nil {
		return fmt.Errorf("db connection not initialized")
	}
	return db.SQL.PingContext(ctx)
}

// Close closes the underlying *sql.DB.
func (db *DB) Close() {
	if db == nil || db.SQL == nil {
		return
	}
	_ = db.SQL.Close()
}
