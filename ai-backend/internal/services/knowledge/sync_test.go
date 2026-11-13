package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

type fakeAI struct {
	mu    sync.Mutex
	calls int
	err   error
	block chan struct{}
}

func (f *fakeAI) Configured() bool { return true }
func (f *fakeAI) ChatMessages(ctx context.Context, _ []models.Message, _ ai.ChatOpts) (string, error) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return "- summary", f.err
}

type fakeRepo struct {
	mu    sync.Mutex
	sha   string
	files map[string][]byte
	err   error
	gate  chan struct{}
}

func (r *fakeRepo) read(ctx context.Context, _ int, _, _ string, _ func(string) bool, _ int64) (string, map[string][]byte, error) {
	if r.gate != nil {
		<-r.gate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sha, r.files, r.err
}

func (r *fakeRepo) set(sha string, files map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sha = sha
	r.files = map[string][]byte{}
	for k, v := range files {
		r.files[k] = []byte(v)
	}
}

const (
	modA = "# Part A\n### A01 — Reg\n| <a id=\"f-a01-01\"></a>A01.01 | Pasien | Search | his | `PROD` | 2026-01-01 | | | | | | |\n| A01.02 | Pasien | Edit | his | `UNKNOWN` | — | | | | | | |\n"
	epic = "# Out\n- **US-OUT-01** (A01.01) — search\n- **US-OUT-02** (A01.02) — edit\n" +
		"| ☑ | TC-OUT-001 | US-OUT-01 | G1 | + | API | P0 | automated | `t.spec.ts` | |\n" +
		"| ☐ | TC-OUT-002 | US-OUT-02 | G2 | - | WEB | P1 | planned | | |\n"
)

func setupProduct(t *testing.T, d *db.DB, format string) int {
	t.Helper()
	ctx := context.Background()
	res, err := d.Exec(ctx, `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES ('SUT', 'https://github.com/o/r', 'https://s', 'sut', 'main')`)
	if err != nil {
		t.Fatal(err)
	}
	pid := int(res.LastInsertId())
	if _, err := d.Exec(ctx, `INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_automation) VALUES (?, 'auto', 'https://github.com/o/r', 'sut', 1, 1)`, pid); err != nil {
		t.Fatal(err)
	}
	if format != "" {
		if _, err := d.Exec(ctx, `INSERT INTO product_settings (product_id, knowledge_format) VALUES (?, ?) ON DUPLICATE KEY UPDATE knowledge_format = VALUES(knowledge_format)`, pid, format); err != nil {
			t.Fatal(err)
		}
	}
	return pid
}

type syncRow struct {
	status, aiStatus string
	diff             Diff
	counts           Counts
	errText          string
}

func waitSync(t *testing.T, d *db.DB, id int64) syncRow {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var r syncRow
		var dj, cj []byte
		var et *string
		if err := d.QueryRow(context.Background(), `SELECT status, ai_status, diff, counts, error FROM kb_syncs WHERE id = ?`, id).Scan(&r.status, &r.aiStatus, &dj, &cj, &et); err != nil {
			t.Fatal(err)
		}
		if r.status != "running" && r.aiStatus != "pending" {
			_ = json.Unmarshal(dj, &r.diff)
			_ = json.Unmarshal(cj, &r.counts)
			if et != nil {
				r.errText = *et
			}
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("sync %d did not finish", id)
	return syncRow{}
}

func TestSyncLifecycle(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{}
	repo.set("1111111111111111111111111111111111111111", map[string]string{"docs/modules/A-core.md": modA, "docs/test-catalog/svc/out.md": epic, ".claude/agents/x.md": "---\nname: x\n---\nhi\n", "CLAUDE.md": "# P\n"})
	fai := &fakeAI{}
	s := &Syncer{DB: d, AI: fai, ReadTree: repo.read, syncTimeout: time.Minute, aiTimeout: time.Minute}

	// First sync: everything added, summary written.
	id, err := s.Start(ctx, pid, TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	r := waitSync(t, d, id)
	if r.status != "succeeded" || r.aiStatus != "done" || r.diff.Total["added"] != 1+1+1+2+1+2+2+2 || r.counts.Agents != 1 || r.counts.Instructions != 1 {
		t.Fatalf("first: %+v", r)
	}
	if r.counts.TestCases != 2 || r.counts.Automated != 1 || r.counts.FeaturesWithStories != 2 {
		t.Fatalf("counts: %+v", r.counts)
	}

	// Annotation on a story that will be removed and restored.
	if _, err := d.Exec(ctx, `INSERT INTO kb_annotations (product_id, entity_type, entity_code, note) VALUES (?, 'story', 'US-OUT-02', 'keep me')`, pid); err != nil {
		t.Fatal(err)
	}

	// Same commit, manual: empty diff, unchanged, no entity writes, no AI call.
	var before string
	_ = d.QueryRow(ctx, `SELECT MAX(updated_at) FROM kb_test_cases WHERE product_id = ?`, pid).Scan(&before)
	id, _ = s.Start(ctx, pid, TriggerManual, "")
	r = waitSync(t, d, id)
	if r.status != "succeeded" || !r.diff.Unchanged || !(&r.diff).Empty() || r.aiStatus != "none" || fai.calls != 1 {
		t.Fatalf("idempotent: %+v calls=%d", r, fai.calls)
	}

	// Same commit, schedule: skipped.
	id, _ = s.Start(ctx, pid, TriggerSchedule, "")
	if r = waitSync(t, d, id); r.status != "skipped" {
		t.Fatalf("schedule: %+v", r)
	}

	// Remove US-OUT-02 and TC-OUT-002; shift lines (location-only change for the rest).
	repo.set("2222222222222222222222222222222222222222", map[string]string{
		"docs/modules/A-core.md":       "\n\n" + modA,
		"docs/test-catalog/svc/out.md": "# Out\n- **US-OUT-01** (A01.01) — search\n| ☑ | TC-OUT-001 | US-OUT-01 | G1 | + | API | P0 | automated | `t.spec.ts` | |\n",
	})
	id, _ = s.Start(ctx, pid, TriggerSchedule, "")
	r = waitSync(t, d, id)
	if r.status != "succeeded" || r.diff.Total["removed"] != 4 || len(r.diff.ByType["assistant"]["removed"]) != 2 || r.diff.Total["updated"] != 0 || r.diff.Total["added"] != 0 {
		t.Fatalf("remove: %+v", r.diff)
	}
	var line int
	_ = d.QueryRow(ctx, `SELECT source_line FROM kb_features WHERE product_id = ? AND code = 'A01.01'`, pid).Scan(&line)
	if line != 5 {
		t.Errorf("location not refreshed: line %d", line)
	}
	var links int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM kb_story_features WHERE product_id = ?`, pid).Scan(&links)
	if links != 1 {
		t.Errorf("links = %d", links)
	}

	// Restore.
	repo.set("3333333333333333333333333333333333333333", map[string]string{"docs/modules/A-core.md": modA, "docs/test-catalog/svc/out.md": epic})
	id, _ = s.Start(ctx, pid, TriggerManual, "")
	r = waitSync(t, d, id)
	if r.diff.Total["restored"] != 2 || r.counts.Stories != 2 {
		t.Fatalf("restore: %+v", r)
	}
	var note string
	_ = d.QueryRow(ctx, `SELECT note FROM kb_annotations WHERE product_id = ? AND entity_code = 'US-OUT-02'`, pid).Scan(&note)
	if note != "keep me" {
		t.Error("annotation lost")
	}

	// AI failure never fails the sync.
	fai.err = errors.New("upstream 502")
	repo.set("4444444444444444444444444444444444444444", map[string]string{"docs/modules/A-core.md": modA, "docs/test-catalog/svc/out.md": epic + "| ☐ | TC-OUT-003 | US-OUT-02 | G3 | + | API | P2 | planned | | |\n"})
	id, _ = s.Start(ctx, pid, TriggerManual, "")
	if r = waitSync(t, d, id); r.status != "succeeded" || r.aiStatus != "failed" {
		t.Fatalf("ai fail: %+v", r)
	}

	// A vanished half (test catalog renamed) fails instead of removing all test cases.
	repo.set("4545454545454545454545454545454545454545", map[string]string{"docs/modules/A-core.md": modA})
	id, _ = s.Start(ctx, pid, TriggerManual, "")
	if r = waitSync(t, d, id); r.status != "failed" || !strings.Contains(r.errText, "no test cases any more") {
		t.Fatalf("vanished half: %+v", r)
	}

	// Empty catalog fails instead of removing everything.
	repo.set("5555555555555555555555555555555555555555", map[string]string{"docs/README.md": "# x"})
	id, _ = s.Start(ctx, pid, TriggerManual, "")
	if r = waitSync(t, d, id); r.status != "failed" || r.counts.TestCases != 0 {
		t.Fatalf("empty: %+v", r)
	}
	var active int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL`, pid).Scan(&active)
	if active != 3 {
		t.Errorf("failed sync must not remove rows: %d active", active)
	}
}

func TestSyncConcurrencyAndOrphans(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{gate: make(chan struct{})}
	repo.set("1111111111111111111111111111111111111111", map[string]string{"docs/modules/A-core.md": modA})
	s := &Syncer{DB: d, ReadTree: repo.read, syncTimeout: time.Minute, aiTimeout: time.Minute}

	id, err := s.Start(ctx, pid, TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Start(ctx, pid, TriggerManual, "")
	var running ErrRunning
	if !errors.As(err, &running) || running.SyncID != id {
		t.Fatalf("want ErrRunning{%d}, got %v", id, err)
	}
	close(repo.gate)
	if r := waitSync(t, d, id); r.status != "succeeded" {
		t.Fatalf("%+v", r)
	}

	// Orphans: a running row from a dead process is failed at startup.
	if _, err := d.Exec(ctx, `INSERT INTO kb_syncs (product_id, status, trigger_kind, started_at, ai_status) VALUES (?, 'running', 'manual', UTC_TIMESTAMP(), 'none')`, pid); err != nil {
		t.Fatal(err)
	}
	s.FailOrphanedSyncs(ctx)
	var n int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM kb_syncs WHERE status = 'running'`).Scan(&n)
	if n != 0 {
		t.Error("orphan not reconciled")
	}

	// Configuration errors.
	pid2 := setupProductNamed(t, d, "Other", "")
	if _, err := s.Start(ctx, pid2, TriggerManual, ""); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("want ErrNotConfigured, got %v", err)
	}
	if _, err := d.Exec(ctx, `INSERT INTO product_settings (product_id, knowledge_format) VALUES (?, 'nope') ON DUPLICATE KEY UPDATE knowledge_format='nope'`, pid2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(ctx, pid2, TriggerManual, ""); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("want ErrUnknownFormat, got %v", err)
	}
	if _, err := s.Start(ctx, 99999, TriggerManual, ""); !errors.Is(err, ErrProductNotFound) {
		t.Errorf("want ErrProductNotFound, got %v", err)
	}
}

func setupProductNamed(t *testing.T, d *db.DB, name, format string) int {
	t.Helper()
	ctx := context.Background()
	res, err := d.Exec(ctx, `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES (?, 'https://github.com/o/x', 'https://s', ?, 'main')`, name, "dir-"+name)
	if err != nil {
		t.Fatal(err)
	}
	pid := int(res.LastInsertId())
	if _, err := d.Exec(ctx, `INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_automation) VALUES (?, 'auto', 'https://github.com/o/x', ?, 1, 1)`, pid, "dir-"+name); err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestSeedMigration(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	// testdb already ran migrations once on an empty schema: the seed found no SUT.
	pid := setupProduct(t, d, "")
	if err := db.RunMigrations(d.SQL); err != nil {
		t.Fatal(err)
	}
	var f *string
	_ = d.QueryRow(ctx, `SELECT knowledge_format FROM product_settings WHERE product_id = ?`, pid).Scan(&f)
	if f == nil || *f != TestCatalogFormatName {
		t.Fatalf("seed: %v", f)
	}
	// An admin turns it off; re-running migrations must not turn it back on.
	if _, err := d.Exec(ctx, `UPDATE product_settings SET knowledge_format = '' WHERE product_id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(d.SQL); err != nil {
		t.Fatal(err)
	}
	_ = d.QueryRow(ctx, `SELECT knowledge_format FROM product_settings WHERE product_id = ?`, pid).Scan(&f)
	if f == nil || *f != "" {
		t.Fatalf("seed overrode the admin: %v", f)
	}
}
