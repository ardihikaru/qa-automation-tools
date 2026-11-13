package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func count(t *testing.T, d *db.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.SQL.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestIsRetryableTxError(t *testing.T) {
	for _, n := range []uint16{1213, 1205} {
		if !db.IsRetryableTxError(fmt.Errorf("w: %w", &mysql.MySQLError{Number: n})) {
			t.Errorf("%d must be retryable", n)
		}
	}
	for _, err := range []error{&mysql.MySQLError{Number: 1062}, errors.New("x"), nil} {
		if db.IsRetryableTxError(err) {
			t.Errorf("%v must not be retryable", err)
		}
	}
}

func TestWithTx(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	// Retries a deadlock twice, then succeeds.
	var calls atomic.Int32
	err := d.WithTx(ctx, func(tx *sql.Tx) error {
		if calls.Add(1) < 3 {
			return &mysql.MySQLError{Number: 1213}
		}
		_, err := tx.Exec(`INSERT INTO products (name, repo_url, staging_url, root_dir) VALUES ('a','u','s','a')`)
		return err
	})
	if err != nil || calls.Load() != 3 {
		t.Fatalf("retry: err=%v calls=%d", err, calls.Load())
	}
	// Gives up after 3 attempts.
	calls.Store(0)
	err = d.WithTx(ctx, func(tx *sql.Tx) error { calls.Add(1); return &mysql.MySQLError{Number: 1205} })
	if err == nil || calls.Load() != 3 {
		t.Fatalf("limit: err=%v calls=%d", err, calls.Load())
	}
	// A non-retryable error runs once and rolls back.
	calls.Store(0)
	err = d.WithTx(ctx, func(tx *sql.Tx) error {
		calls.Add(1)
		if _, err := tx.Exec(`INSERT INTO products (name, repo_url, staging_url, root_dir) VALUES ('b','u','s','b')`); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if err == nil || calls.Load() != 1 || count(t, d, `SELECT COUNT(*) FROM products WHERE name='b'`) != 0 {
		t.Fatalf("rollback: err=%v calls=%d", err, calls.Load())
	}
}

func TestMigrationsIdempotentAndBackfill(t *testing.T) {
	d := testdb.New(t) // migrations already ran once on the empty schema
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.SQL.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// Legacy rows as the previous release stored them.
	mustExec(`INSERT INTO products (name, repo_url, staging_url, root_dir, branch, playwright_accessible) VALUES
		('p1','https://github.com/o/p1','https://s','o/p1','release/1.0',1),
		('p2','https://github.com/o/p2','https://s','o/p2','',0),
		('p3','https://github.com/o/p3','https://s','o/p3.branches',  'main',0),
		('p4','https://github.com/o/p4','https://s','O/P4.BRANCHES/x','main',0)`)
	for i := 0; i < 2; i++ {
		if err := db.RunMigrations(d.SQL); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if n := count(t, d, `SELECT COUNT(*) FROM product_repos WHERE is_main = 1`); n != 2 {
		t.Fatalf("want 2 main repos (p3, p4 skipped), got %d", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM products p WHERE NOT EXISTS (SELECT 1 FROM product_repos r WHERE r.product_id = p.id)`); n != 2 {
		t.Fatalf("post-flight query must list the 2 skipped products, got %d", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM products WHERE root_dir REGEXP '(^|/)[^/]*\\.branches(/|$)'`); n != 2 {
		t.Fatalf("pre-flight query (as documented) must find the 2 conflicting rows, got %d", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM product_repo_branches b JOIN product_repos r ON r.id=b.repo_id JOIN products p ON p.id=r.product_id
		WHERE p.name='p1' AND b.branch='release/1.0' AND b.slug='release--1.0' AND b.is_default=1`); n != 1 {
		t.Fatal("legacy branch must become the default branch")
	}
	if n := count(t, d, `SELECT COUNT(*) FROM product_repo_branches`); n != 1 {
		t.Fatalf("empty legacy branch gets no row; got %d rows", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM product_repos WHERE playwright_accessible=1`); n != 1 {
		t.Fatal("verify state must be copied")
	}
	// User edits survive a re-run (NOT EXISTS guards).
	mustExec(`DELETE FROM product_repo_branches`)
	mustExec(`INSERT INTO product_repo_branches (repo_id, branch, slug, is_default) SELECT id, 'dev', 'dev', 1 FROM product_repos WHERE root_dir='o/p1'`)
	if err := db.RunMigrations(d.SQL); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM product_repo_branches`); n != 1 {
		t.Fatalf("re-run must not re-add branches, got %d", n)
	}
}

func TestUniqueKeyBackstopsAndCascade(t *testing.T) {
	d := testdb.New(t)
	mustExec := func(q string, args ...any) error {
		_, err := d.SQL.Exec(q, args...)
		return err
	}
	dupKey := func(err error, key string) {
		t.Helper()
		var me *mysql.MySQLError
		if !errors.As(err, &me) || me.Number != 1062 || !strings.Contains(me.Message, key) {
			t.Fatalf("want 1062 on %s, got %v", key, err)
		}
	}
	for _, n := range []string{"a", "b"} {
		if err := mustExec(`INSERT INTO products (name, repo_url, staging_url, root_dir) VALUES (?, 'u', 's', ?)`, n, n); err != nil {
			t.Fatal(err)
		}
	}
	ins := func(pid int, name, url, dir string, main, central int) error {
		return mustExec(`INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_central) VALUES (?,?,?,?,?,?)`, pid, name, url, dir, main, central)
	}
	if err := ins(1, "m", "https://github.com/o/m", "d/m", 1, 1); err != nil {
		t.Fatal(err)
	}
	dupKey(ins(1, "m2", "https://github.com/o/m2", "d/m2", 1, 0), "uq_product_repos_main")
	dupKey(ins(1, "c2", "https://github.com/o/c2", "d/c2", 0, 1), "uq_product_repos_central")
	dupKey(ins(1, "m", "https://github.com/o/x", "d/x", 0, 0), "uq_product_repos_name")
	dupKey(ins(1, "x", "https://github.com/o/m", "d/x", 0, 0), "uq_product_repos_url")
	dupKey(ins(2, "x", "https://github.com/o/x", "D/M", 0, 0), "uq_product_repos_root_dir") // case-insensitive
	if err := ins(2, "m", "https://github.com/o/m", "d/m2", 1, 1); err != nil {
		t.Fatalf("another product may have its own main/central and reuse names: %v", err)
	}
	if err := ins(1, "s", "https://github.com/o/s", "d/s", 0, 0); err != nil {
		t.Fatal(err)
	}
	rid1 := count(t, d, `SELECT id FROM product_repos WHERE root_dir='d/m'`)
	rid2 := count(t, d, `SELECT id FROM product_repos WHERE root_dir='d/m2'`)
	br := func(rid int, b, slug string, def int) error {
		return mustExec(`INSERT INTO product_repo_branches (repo_id, branch, slug, is_default) VALUES (?,?,?,?)`, rid, b, slug, def)
	}
	if err := br(rid1, "main", "main", 1); err != nil {
		t.Fatal(err)
	}
	if err := br(rid1, "Main", "Main", 0); err != nil {
		t.Fatalf("branches are case sensitive: %v", err)
	}
	dupKey(br(rid1, "dev", "dev", 1), "uq_product_repo_branch_default")
	dupKey(br(rid1, "main", "x", 0), "uq_product_repo_branches")
	if err := br(rid1, "a/b", "a--b", 0); err != nil {
		t.Fatal(err)
	}
	dupKey(br(rid1, "a--b", "a--b", 0), "uq_product_repo_branch_slug")
	if err := br(rid2, "main", "main", 1); err != nil {
		t.Fatalf("other repo may use the same branch names: %v", err)
	}
	if err := mustExec(`INSERT INTO product_settings (product_id, allow_non_automation_work) VALUES (1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := mustExec(`DELETE FROM products WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"product_repos WHERE product_id=1", "product_settings WHERE product_id=1"} {
		if n := count(t, d, `SELECT COUNT(*) FROM `+tbl); n != 0 {
			t.Fatalf("cascade left rows in %s", tbl)
		}
	}
	if n := count(t, d, `SELECT COUNT(*) FROM product_repo_branches`); n != 1 {
		t.Fatalf("cascade must remove only product 1's branches, %d left", n)
	}
}
