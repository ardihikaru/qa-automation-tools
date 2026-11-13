// Package testdb gives DB-backed tests a throw-away MySQL schema.
//
// Tests call New(t). When QA_TEST_MYSQL_DSN is empty the test is skipped, so CI without a
// scratch server still passes. The DSN must point at a scratch MySQL server; its schema name
// is never used (a uniquely named schema is created and dropped), and a DSN whose schema is
// qa_automation is refused outright so the shared schema can never be touched.
package testdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/db"
)

// EnvDSN is the environment variable holding the scratch-server DSN.
const EnvDSN = "QA_TEST_MYSQL_DSN"

// protectedSchema is the production schema name that must never be used by tests.
const protectedSchema = "qa_automation"

// CheckDSN parses dsn and rejects the protected schema.
func CheckDSN(dsn string) (*mysql.Config, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", EnvDSN, err)
	}
	if strings.EqualFold(cfg.DBName, protectedSchema) {
		return nil, fmt.Errorf("%s points at the protected schema %q; refusing", EnvDSN, protectedSchema)
	}
	return cfg, nil
}

// New returns a *db.DB on a fresh scratch schema with all migrations applied, or skips the test.
func New(t testing.TB) *db.DB {
	t.Helper()
	dsn := os.Getenv(EnvDSN)
	if dsn == "" {
		t.Skipf("set %s to a scratch MySQL DSN to run DB-backed tests", EnvDSN)
	}
	cfg, err := CheckDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ParseTime = true
	cfg.MultiStatements = false

	admin := cfg.Clone()
	admin.DBName = ""
	adminDB, err := sql.Open("mysql", admin.FormatDSN())
	if err != nil {
		t.Fatalf("open scratch server: %v", err)
	}
	var suffix [6]byte
	_, _ = rand.Read(suffix[:])
	schema := "qa_test_" + hex.EncodeToString(suffix[:])
	if _, err := adminDB.Exec("CREATE DATABASE `" + schema + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create scratch schema: %v", err)
	}

	scratch := cfg.Clone()
	scratch.DBName = schema
	sqlDB, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatalf("open scratch schema: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_, _ = adminDB.Exec("DROP DATABASE IF EXISTS `" + schema + "`")
		_ = adminDB.Close()
	})
	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return &db.DB{SQL: sqlDB}
}
