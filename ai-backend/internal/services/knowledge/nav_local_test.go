package knowledge

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/navmap"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// TestNavDevSeed fills a local development database (NAV_DEV_DSN, never the qa_automation schema)
// with a product synced from a local checkout of the automation repo (NAV_LOCAL_REPO), so the
// dashboard's /navigation can be looked at without a deploy. NAV_DEV_FAKE_RESULTS=1 also writes
// made-up results (every automated case passes except every 25th, which fails): local only.
func TestNavDevSeed(t *testing.T) {
	dsn, root := os.Getenv("NAV_DEV_DSN"), os.Getenv("NAV_LOCAL_REPO")
	if dsn == "" || root == "" {
		t.Skip("NAV_DEV_DSN and NAV_LOCAL_REPO not set")
	}
	cfg, err := testdb.CheckDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ParseTime = true
	sqlDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatal(err)
	}
	d := &db.DB{SQL: sqlDB}
	ctx := context.Background()
	var pid int
	if err := d.QueryRow(ctx, `SELECT id FROM products WHERE name = 'HISv3'`).Scan(&pid); err != nil {
		pid = setupProduct(t, d, HISv3FormatName)
	}
	files := readLocalTree(t, root)
	s := &Syncer{DB: d, ReadTree: func(context.Context, int, string, string, func(string) bool, int64) (string, map[string][]byte, error) {
		return strings.Repeat("d", 40), files, nil
	}, syncTimeout: time.Minute, aiTimeout: time.Minute}
	id, err := s.Start(ctx, pid, TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitSync(t, d, id); r.status != "succeeded" {
		t.Fatalf("sync: %+v", r)
	}
	waitNav(t, d, pid, func(b navBuildRow) bool { return b.syncID == id })
	if os.Getenv("NAV_DEV_FAKE_RESULTS") == "1" {
		if _, err := d.Exec(ctx, `INSERT INTO kb_case_results (product_id, case_code, result, run_at, run_ref, source)
SELECT product_id, code, IF(MOD(id, 25) = 0, 'fail', 'pass'), UTC_TIMESTAMP(), 'local-fake', 'upload' FROM kb_test_cases
WHERE product_id = ? AND automated = 1 AND removed_at IS NULL
ON DUPLICATE KEY UPDATE result = VALUES(result)`, pid); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("product %d synced (sync %d)", pid, id)
}

// readLocalTree reads a working tree the way the sync's mirror read does (KeepForSync).
func readLocalTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "node_modules" || rel == ".git" || strings.HasPrefix(rel, ".runs") {
				return filepath.SkipDir
			}
			return nil
		}
		if KeepForSync(rel) {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files[rel] = b
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestNavLocalRepo builds the map from a local checkout of the HISv3 automation repo
// (NAV_LOCAL_REPO=/path/to/automation-test-hisv3). It checks the routes NAVIGATION_MAP.md
// section 10 names; skipped without the variable.
func TestNavLocalRepo(t *testing.T) {
	root := os.Getenv("NAV_LOCAL_REPO")
	if root == "" {
		t.Skip("NAV_LOCAL_REPO not set")
	}
	files := readLocalTree(t, root)
	f, _ := Lookup("hisv3-test-catalog-v1")
	snap, _, err := f.Parse(files)
	if err != nil {
		t.Fatal(err)
	}
	inv := navmap.Build(navInput(snap, files))
	byPath := map[string]navmap.Route{}
	apps := map[string]int{}
	for _, r := range inv.Routes {
		byPath[r.Surface+" "+r.App+" "+r.Path] = r
		apps[r.Surface+" "+r.App]++
	}
	t.Logf("%d routes, %d links, %d problems; apps %v", len(inv.Routes), len(inv.Links), len(inv.Problems), apps)
	if os.Getenv("NAV_LOCAL_DUMP") != "" {
		for _, r := range inv.Routes {
			t.Logf("%s %s %s %v %v %v", r.Surface, r.App, r.Path, r.Methods, r.Sources, r.PageObjects)
		}
		for _, p := range inv.Problems {
			t.Logf("problem: %s", p)
		}
	}
	direct := map[string]bool{}
	bySource := map[string]int{}
	keyPath := map[string]string{}
	for _, r := range inv.Routes {
		keyPath[r.Key] = r.Path
	}
	for _, l := range inv.Links {
		bySource[l.RefType+"/"+l.Source]++
		if l.RefType == navmap.RefCase && navmap.Direct(l.Source) {
			direct[l.Ref] = true
		}
		if l.RefType == navmap.RefCase && (l.Ref == "TC-VKAUTH-007" || l.Ref == "TC-VSYS-001" || l.Ref == "TC-RB-060" || strings.HasPrefix(l.Ref, "TC-AUTH-00")) {
			t.Logf("link %s %s %s %s", l.Ref, l.Source, l.Method, keyPath[l.RouteKey])
		}
	}
	t.Logf("cases with a direct route: %d of %d; links by source %v", len(direct), len(snap.TestCases), bySource)
	for _, want := range []string{
		"api his-vendor-backend /api/v1/service-status",
		"api his-vendor-backend /api/v1/swagger/*",
		"api his-vendor-backend /api/v1/kiosk-his-v2/health",
		"web his-frontend /auth/login",
		"web his-frontend /report-builder/list",
	} {
		if _, ok := byPath[want]; !ok {
			t.Errorf("missing route %q", want)
		}
	}
}
