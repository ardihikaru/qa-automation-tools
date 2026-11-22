package knowledge

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/navmap"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// NAVIGATION_MAP.md section 5: the inventory is built after a succeeded sync, rebuilt on a skipped
// sync when missing or outdated, and a navigation failure never changes the sync's status.

const (
	navModB = "# Part B\n### B06 — Tech\n" +
		"| <a id=\"f-b06-01\"></a>B06.01 | Liveness | Probe `GET /service-status` | sut-vendor-backend `/api/v1/service-status` | `PROD` | 2026-01-01 | | | | | | |\n"
	navEpic = "# Sys\n- **US-SYS-01** (B06.01) — probe\n" +
		"| ☑ | TC-SYS-001 | US-SYS-01 | when `GET /api/v1/service-status` is sent then 200 | + | API | P1 | automated | `tests/api/rest/sut-vendor-backend/TC-SYS-001.spec.ts` | |\n" +
		"| ☐ | TC-SYS-002 | US-SYS-01 | GET /kiosk-sut-v2/health — 200 | + | API | P1 | planned | | |\n"
	navPage = "export class LoginPage extends BasePage {\n  protected readonly path = '/auth/login';\n}\n"
)

func navFiles() map[string]string {
	return map[string]string{
		"docs/modules/B-vendor.md":                             navModB,
		"docs/test-catalog/vendor-backend/system.md":           navEpic,
		"src/web/sut-frontend/pages/auth/login.page.ts":        navPage,
		"tests/api/rest/sut-vendor-backend/TC-SYS-001.spec.ts": "test('TC-SYS-001 probe', async () => {});\n",
	}
}

type navBuildRow struct {
	status  string
	syncID  int64
	routes  int
	version int
	errText string
}

func readNavBuild(t *testing.T, d *db.DB, pid int) (navBuildRow, bool) {
	t.Helper()
	var r navBuildRow
	err := d.QueryRow(context.Background(), `SELECT status, COALESCE(sync_id, 0), routes, nav_version, error FROM nav_builds WHERE product_id = ?`, pid).
		Scan(&r.status, &r.syncID, &r.routes, &r.version, &r.errText)
	if err != nil {
		return r, false
	}
	return r, true
}

// waitNav polls nav_builds until cond holds (the build runs after the sync's status is written).
func waitNav(t *testing.T, d *db.DB, pid int, cond func(navBuildRow) bool) navBuildRow {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, ok := readNavBuild(t, d, pid); ok && cond(b) {
			return b
		}
		time.Sleep(20 * time.Millisecond)
	}
	b, _ := readNavBuild(t, d, pid)
	t.Fatalf("navigation build did not reach the expected state: %+v", b)
	return b
}

func navRouteCount(t *testing.T, d *db.DB, pid int) int {
	t.Helper()
	var n int
	if err := d.QueryRow(context.Background(), `SELECT COUNT(*) FROM nav_routes WHERE product_id = ?`, pid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNavBuiltBySync(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{}
	repo.set("1111111111111111111111111111111111111111", navFiles())
	var succeeded, unchanged atomic.Int32
	s := &Syncer{DB: d, ReadTree: repo.read, syncTimeout: time.Minute, aiTimeout: time.Minute,
		OnSucceeded: func(int) { succeeded.Add(1) }, OnUnchanged: func(int) { unchanged.Add(1) }}

	id, err := s.Start(ctx, pid, TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitSync(t, d, id); r.status != "succeeded" {
		t.Fatalf("sync: %+v", r)
	}
	b := waitNav(t, d, pid, func(b navBuildRow) bool { return b.syncID == id })
	if b.status != "ok" || b.version != navmap.Version || b.routes != navRouteCount(t, d, pid) {
		t.Fatalf("nav build after sync: %+v", b)
	}
	var paths []string
	rows, err := d.Query(ctx, `SELECT CONCAT(surface, ' ', app, ' ', path) FROM nav_routes WHERE product_id = ? ORDER BY 1`, pid)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p string
		_ = rows.Scan(&p)
		paths = append(paths, p)
	}
	rows.Close()
	want := []string{
		"api sut-vendor-backend /api/v1/kiosk-sut-v2/health",
		"api sut-vendor-backend /api/v1/service-status",
		"web sut-frontend /auth/login",
	}
	if len(paths) != len(want) {
		t.Fatalf("routes %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("routes %v, want %v", paths, want)
		}
	}
	var specLinks int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM nav_links WHERE product_id = ? AND ref = 'TC-SYS-001' AND source = 'spec'`, pid).Scan(&specLinks)
	if specLinks != 0 {
		// The spec calls no client method: it uses no route.
		t.Errorf("spec links = %d", specLinks)
	}

	// A skipped scheduled sync rebuilds a missing inventory, writes no catalog row and does not
	// call OnSucceeded.
	if _, err := d.Exec(ctx, `DELETE FROM nav_builds WHERE product_id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, `DELETE FROM nav_routes WHERE product_id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	before := succeeded.Load()
	id2, err := s.Start(ctx, pid, TriggerSchedule, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitSync(t, d, id2); r.status != "skipped" {
		t.Fatalf("schedule: %+v", r)
	}
	if succeeded.Load() != before || unchanged.Load() != 1 {
		t.Errorf("hooks: succeeded %d (before %d), unchanged %d", succeeded.Load(), before, unchanged.Load())
	}
	b, ok := readNavBuild(t, d, pid)
	if !ok || b.status != "ok" || b.syncID != id || navRouteCount(t, d, pid) != 3 {
		t.Fatalf("rebuild on skip: %+v ok=%v routes=%d", b, ok, navRouteCount(t, d, pid))
	}

	// Up to date: a skipped sync does not rebuild (built_at unchanged).
	var at1, at2 time.Time
	_ = d.QueryRow(ctx, `SELECT built_at FROM nav_builds WHERE product_id = ?`, pid).Scan(&at1)
	time.Sleep(1100 * time.Millisecond)
	id3, _ := s.Start(ctx, pid, TriggerSchedule, "")
	waitSync(t, d, id3)
	_ = d.QueryRow(ctx, `SELECT built_at FROM nav_builds WHERE product_id = ?`, pid).Scan(&at2)
	if !at1.Equal(at2) {
		t.Errorf("up-to-date inventory was rebuilt: %v -> %v", at1, at2)
	}

	// An outdated version is rebuilt on a skipped sync.
	if _, err := d.Exec(ctx, `UPDATE nav_builds SET nav_version = 0 WHERE product_id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	id4, _ := s.Start(ctx, pid, TriggerSchedule, "")
	waitSync(t, d, id4)
	if b, _ = readNavBuild(t, d, pid); b.version != navmap.Version {
		t.Errorf("outdated version not rebuilt: %+v", b)
	}

	// A navigation failure keeps the previous inventory and never fails the sync.
	if _, err := d.Exec(ctx, `ALTER TABLE nav_links RENAME TO nav_links_gone`); err != nil {
		t.Fatal(err)
	}
	repo.set("2222222222222222222222222222222222222222", navFiles())
	id5, _ := s.Start(ctx, pid, TriggerManual, "")
	if r := waitSync(t, d, id5); r.status != "succeeded" {
		t.Fatalf("sync with a broken nav store: %+v", r)
	}
	b = waitNav(t, d, pid, func(b navBuildRow) bool { return b.syncID == id5 })
	if b.status != "failed" || b.errText == "" || navRouteCount(t, d, pid) != 3 {
		t.Errorf("failed build: %+v routes=%d", b, navRouteCount(t, d, pid))
	}
}
