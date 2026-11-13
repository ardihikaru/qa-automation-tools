package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// DB-backed tests: skipped unless QA_TEST_MYSQL_DSN points at a scratch MySQL server.

func newDBRouter(t *testing.T) *Router {
	t.Helper()
	return &Router{db: testdb.New(t)}
}

func call(t *testing.T, r *Router, role, method, path string, body any) (int, []byte) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{Role: role}))
	rec := httptest.NewRecorder()
	r.handleProducts(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func decodeProduct(t *testing.T, b []byte) models.Product {
	t.Helper()
	var p models.Product
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return p
}

func mustCall(t *testing.T, r *Router, role, method, path string, body any, want int) models.Product {
	t.Helper()
	code, b := call(t, r, role, method, path, body)
	if code != want {
		t.Fatalf("%s %s: want %d, got %d: %s", method, path, want, code, b)
	}
	if code == http.StatusNoContent {
		return models.Product{}
	}
	return decodeProduct(t, b)
}

func find(t *testing.T, p models.Product, name string) models.ProductRepo {
	t.Helper()
	for _, r := range p.Repos {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("repo %q not in %+v", name, p.Repos)
	return models.ProductRepo{}
}

func createHISv3(t *testing.T, r *Router) models.Product {
	t.Helper()
	return mustCall(t, r, "Admin", http.MethodPost, "/api/products", map[string]any{
		"name": "HISv3", "staging_url": "https://staging.example.com",
		"repos": []map[string]any{
			{"name": "automation", "repo_url": "https://github.com/o/auto", "root_dir": "o/auto", "is_main": true,
				"branches": []string{"main", "release/1.0"}, "default_branch": "main"},
			{"name": "his-v3-ai", "repo_url": "https://github.com/o/his-v3-ai", "root_dir": "o/his-v3-ai", "is_central": true,
				"branches": []string{"dev"}, "default_branch": "dev"},
		},
	}, http.StatusCreated)
}

// createSplit makes a product whose AI workspace (main) differs from its automation repo.
func createSplit(t *testing.T, r *Router, staging string) models.Product {
	t.Helper()
	return mustCall(t, r, "Admin", http.MethodPost, "/api/products", map[string]any{
		"name": "Split", "staging_url": staging,
		"repos": []map[string]any{
			{"name": "his-v3-ai", "repo_url": "https://github.com/o/his-v3-ai", "root_dir": "o/ai-main", "is_main": true,
				"branches": []string{"dev"}, "default_branch": "dev"},
			{"name": "automation-test-hisv3", "repo_url": "https://github.com/o/automation-test-hisv3", "root_dir": "o/auto-test", "is_automation": true,
				"branches": []string{"release", "main"}, "default_branch": "main"},
		},
	}, http.StatusCreated)
}

// assertMirror checks the legacy columns equal the automation repo's values (the drift invariant)
// and that the product has exactly one main and one automation repo. The legacy branch is the
// automation repo's default, else its first branch alphabetically, else ”.
func assertMirror(t *testing.T, r *Router, pid int, ctx string) {
	t.Helper()
	p, err := fetchProduct(context.Background(), r.db, pid)
	if err != nil {
		t.Fatal(err)
	}
	var auto *models.ProductRepo
	mains, autos := 0, 0
	for i := range p.Repos {
		if p.Repos[i].IsMain {
			mains++
		}
		if p.Repos[i].IsAutomation {
			auto, autos = &p.Repos[i], autos+1
		}
	}
	if mains != 1 || autos != 1 {
		t.Fatalf("%s: want exactly one main and one automation, got %d and %d", ctx, mains, autos)
	}
	wantBranch := auto.DefaultBranch
	if wantBranch == "" {
		for _, b := range auto.Branches {
			if wantBranch == "" || b < wantBranch {
				wantBranch = b
			}
		}
	}
	if p.RepoURL != auto.RepoURL || p.RootDir != auto.RootDir || p.Branch != wantBranch ||
		p.PlaywrightAccessible != auto.PlaywrightAccessible || string(p.VerifyReport) != string(auto.VerifyReport) ||
		(p.LastVerifiedAt == nil) != (auto.LastVerifiedAt == nil) {
		t.Fatalf("%s: legacy columns drifted from automation repo: product=%+v automation=%+v", ctx, p, *auto)
	}
}

func TestProductsAPI_CreateReadAndLegacyMirror(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	if len(p.Repos) != 2 || !p.Repos[0].IsMain || !p.Repos[0].IsAutomation || !p.Repos[1].IsCentral || p.Repos[1].IsAutomation {
		t.Fatalf("order main, central; main defaults to automation: %+v", p.Repos)
	}
	main := p.Repos[0]
	if main.DefaultBranch != "main" || strings.Join(main.Branches, ",") != "main,release/1.0" {
		t.Fatalf("branches: %+v", main)
	}
	if p.RepoURL != main.RepoURL || p.RootDir != "o/auto" || p.Branch != "main" || p.AllowNonAutomationWork {
		t.Fatalf("mirror/flag: %+v", p)
	}
	assertMirror(t, r, p.ID, "create")

	// Legacy flat create makes one main repo named "main".
	q := mustCall(t, r, "Developer", http.MethodPost, "/api/products", map[string]any{
		"name": "Legacy", "staging_url": "https://s.example.com", "repo_url": "https://github.com/o/legacy", "root_dir": "o/legacy", "branch": "develop"}, http.StatusCreated)
	if len(q.Repos) != 1 || q.Repos[0].Name != "main" || !q.Repos[0].IsMain || !q.Repos[0].IsAutomation || q.Repos[0].DefaultBranch != "develop" || q.Branch != "develop" {
		t.Fatalf("legacy create: %+v", q)
	}
	// Both forms together, no main, two mains: 400. Duplicate name: 409. QA: 403.
	for name, body := range map[string]map[string]any{
		"both": {"name": "X", "staging_url": "https://s.example.com", "repo_url": "https://github.com/o/x", "root_dir": "o/x",
			"repos": []map[string]any{{"name": "m", "repo_url": "https://github.com/o/x", "root_dir": "o/x", "is_main": true}}},
		"no main": {"name": "X", "staging_url": "https://s.example.com", "repos": []map[string]any{{"name": "m", "repo_url": "https://github.com/o/x", "root_dir": "o/x"}}},
		"two automation": {"name": "X", "staging_url": "https://s.example.com", "repos": []map[string]any{
			{"name": "m", "repo_url": "https://github.com/o/x", "root_dir": "o/x", "is_main": true, "is_automation": true},
			{"name": "n", "repo_url": "https://github.com/o/y", "root_dir": "o/y", "is_automation": true}}},
	} {
		if code, b := call(t, r, "Admin", http.MethodPost, "/api/products", body); code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d %s", name, code, b)
		}
	}
	if code, _ := call(t, r, "Admin", http.MethodPost, "/api/products", map[string]any{"name": "HISv3", "staging_url": "https://s.example.com",
		"repo_url": "https://github.com/o/dupname", "root_dir": "o/dupname"}); code != http.StatusConflict {
		t.Errorf("duplicate name: want 409, got %d", code)
	}
	if code, _ := call(t, r, "Admin", http.MethodPost, "/api/products", map[string]any{"name": "Other", "staging_url": "https://s.example.com",
		"repo_url": "https://github.com/o/other", "root_dir": "o/auto/nested"}); code != http.StatusConflict {
		t.Errorf("overlapping root_dir: want 409, got %d", code)
	}
	if code, _ := call(t, r, "QA", http.MethodPost, "/api/products", map[string]any{"name": "Z"}); code != http.StatusForbidden {
		t.Errorf("QA create: want 403, got %d", code)
	}
	// List.
	code, b := call(t, r, "QA", http.MethodGet, "/api/products", nil)
	var list struct {
		Products []models.Product `json:"products"`
	}
	if code != 200 || json.Unmarshal(b, &list) != nil || len(list.Products) != 2 {
		t.Fatalf("list: %d %s", code, b)
	}
}

func TestProductsAPI_RepoLifecycleAndInvariants(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	base := fmt.Sprintf("/api/products/%d", p.ID)
	central := find(t, p, "his-v3-ai")
	main := find(t, p, "automation")

	// Add a sibling; same repo spelled differently, root_dir overlap and duplicate name are 409.
	p = mustCall(t, r, "Developer", http.MethodPost, base+"/repos", map[string]any{"name": "sib", "repo_url": "https://github.com/o/sib", "root_dir": "o/sib",
		"branches": []string{"main", "dev"}, "default_branch": "dev"}, http.StatusCreated)
	sib := find(t, p, "sib")
	if sib.IsMain || sib.DefaultBranch != "dev" || sib.Branches[0] != "dev" {
		t.Fatalf("sibling: %+v", sib)
	}
	for name, body := range map[string]map[string]any{
		"url":     {"name": "x", "repo_url": "https://github.com/O/Sib.git", "root_dir": "o/x"},
		"overlap": {"name": "x", "repo_url": "https://github.com/o/x", "root_dir": "o/sib/inner"},
		"name":    {"name": "sib", "repo_url": "https://github.com/o/x", "root_dir": "o/x"},
	} {
		if code, b := call(t, r, "Developer", http.MethodPost, base+"/repos", body); code != http.StatusConflict {
			t.Errorf("%s: want 409, got %d %s", name, code, b)
		}
	}
	// Roles.
	if code, _ := call(t, r, "QA", http.MethodPost, base+"/repos", map[string]any{"name": "q"}); code != http.StatusForbidden {
		t.Errorf("QA add repo: %d", code)
	}
	// Repo of another product is 404 (not leaked).
	other := mustCall(t, r, "Admin", http.MethodPost, "/api/products", map[string]any{"name": "Other", "staging_url": "https://s.example.com",
		"repo_url": "https://github.com/o/other", "root_dir": "o/other", "branch": "main"}, http.StatusCreated)
	if code, _ := call(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d/repos/%d", other.ID, sib.ID), map[string]any{"name": "hijack"}); code != http.StatusNotFound {
		t.Errorf("foreign repo: want 404, got %d", code)
	}
	// Central moves; central flag clears.
	p = mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, sib.ID), map[string]any{"is_central": true}, http.StatusOK)
	if !find(t, p, "sib").IsCentral || find(t, p, "his-v3-ai").IsCentral {
		t.Fatalf("central must move: %+v", p.Repos)
	}
	p = mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, sib.ID), map[string]any{"is_central": false}, http.StatusOK)
	if find(t, p, "sib").IsCentral {
		t.Fatal("central must clear")
	}
	// Main: cannot be unset or deleted; promotion swaps atomically and mirrors.
	if code, _ := call(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, main.ID), map[string]any{"is_main": false}); code != http.StatusConflict {
		t.Errorf("unset main: %d", code)
	}
	if code, _ := call(t, r, "Admin", http.MethodDelete, fmt.Sprintf("%s/repos/%d", base, main.ID), nil); code != http.StatusConflict {
		t.Errorf("delete main: %d", code)
	}
	p = mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, central.ID), map[string]any{"is_main": true}, http.StatusOK)
	if !find(t, p, "his-v3-ai").IsMain || find(t, p, "automation").IsMain {
		t.Fatalf("promotion: %+v", p)
	}
	// Promoting main does not move the mirror: it follows the automation repo.
	if p.RootDir != "o/auto" || p.Branch != "main" || !find(t, p, "automation").IsAutomation || find(t, p, "his-v3-ai").IsAutomation {
		t.Fatalf("main promotion must not move the mirror: %+v", p)
	}
	assertMirror(t, r, p.ID, "main promotion")
	// Automation: idempotent on the current automation repo, 409 to unset or delete it.
	mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, main.ID), map[string]any{"is_automation": true}, http.StatusOK)
	for name, c := range map[string]struct {
		method string
		body   any
	}{
		"unset":  {http.MethodPut, map[string]any{"is_automation": false}},
		"delete": {http.MethodDelete, nil},
	} {
		code, b := call(t, r, "Admin", c.method, fmt.Sprintf("%s/repos/%d", base, main.ID), c.body)
		if code != http.StatusConflict || !strings.Contains(string(b), "automation repo") {
			t.Errorf("%s automation: want 409, got %d %s", name, code, b)
		}
	}
	// Promote automation: moves atomically, clears verify of both repos, mirror moves.
	for _, rp := range []models.ProductRepo{main, central} {
		if err := r.storeVerify(context.Background(), p.ID, find(t, p, rp.Name), p.StagingURL, repoReport(true)); err != nil {
			t.Fatal(err)
		}
	}
	p = mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, central.ID), map[string]any{"is_automation": true}, http.StatusOK)
	if !find(t, p, "his-v3-ai").IsAutomation || find(t, p, "automation").IsAutomation || p.RootDir != "o/his-v3-ai" || p.Branch != "dev" {
		t.Fatalf("automation promotion: %+v", p)
	}
	for _, rp := range p.Repos {
		if (rp.ID == main.ID || rp.ID == central.ID) && (rp.LastVerifiedAt != nil || rp.PlaywrightAccessible) {
			t.Fatalf("promotion must clear verify of the demoted and promoted repo: %+v", rp)
		}
	}
	assertMirror(t, r, p.ID, "automation promotion")
	// Branches PUT: validation, replace, default move, mirror.
	for name, body := range map[string]map[string]any{
		"empty":      {"branches": []string{}, "default_branch": ""},
		"no default": {"branches": []string{"a"}},
		"unlisted":   {"branches": []string{"a"}, "default_branch": "b"},
		"too many":   {"branches": []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}, "default_branch": "a"},
		"slug clash": {"branches": []string{"a/b", "a--b"}, "default_branch": "a/b"},
		"bad name":   {"branches": []string{"-x"}, "default_branch": "-x"},
		"duplicate":  {"branches": []string{"a", "a"}, "default_branch": "a"},
	} {
		if code, b := call(t, r, "Developer", http.MethodPut, fmt.Sprintf("%s/repos/%d/branches", base, central.ID), body); code != http.StatusBadRequest {
			t.Errorf("branches %s: want 400, got %d %s", name, code, b)
		}
	}
	p = mustCall(t, r, "Developer", http.MethodPut, fmt.Sprintf("%s/repos/%d/branches", base, central.ID),
		map[string]any{"branches": []string{"dev", "main", "release/2"}, "default_branch": "main"}, http.StatusOK)
	if got := find(t, p, "his-v3-ai"); got.DefaultBranch != "main" || strings.Join(got.Branches, ",") != "main,dev,release/2" || p.Branch != "main" {
		t.Fatalf("branches: %+v branch=%s", got, p.Branch)
	}
	assertMirror(t, r, p.ID, "branches")
	// Edit url/dir: clears verify state, keeps mirror; delete a sibling is 200.
	p = mustCall(t, r, "Developer", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, central.ID), map[string]any{"root_dir": "o/renamed"}, http.StatusOK)
	if p.RootDir != "o/renamed" {
		t.Fatalf("mirror root_dir: %s", p.RootDir)
	}
	p = mustCall(t, r, "Developer", http.MethodDelete, fmt.Sprintf("%s/repos/%d", base, sib.ID), nil, http.StatusOK)
	if len(p.Repos) != 2 {
		t.Fatalf("sibling not deleted: %+v", p.Repos)
	}
	// Pool routes without a pool: 503. Unknown paths: 404. Wrong method: 405.
	if code, _ := call(t, r, "Admin", http.MethodPost, fmt.Sprintf("%s/repos/%d/checkout", base, central.ID), nil); code != http.StatusServiceUnavailable {
		t.Errorf("no pool: %d", code)
	}
	if code, _ := call(t, r, "Admin", http.MethodGet, base+"/repos/abc", nil); code != http.StatusNotFound {
		t.Errorf("bad rid: %d", code)
	}
	if code, _ := call(t, r, "Admin", http.MethodGet, base+"/repos", nil); code != http.StatusMethodNotAllowed {
		t.Errorf("GET repos: %d", code)
	}
	if code, _ := call(t, r, "Admin", http.MethodPost, "/api/products/999999/repos", map[string]any{"name": "n", "repo_url": "https://github.com/o/n", "root_dir": "o/n", "is_main": true}); code != http.StatusNotFound {
		t.Errorf("missing product: %d", code)
	}
}

func TestProductsAPI_FlagRoleAndLegacyPut(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	base := fmt.Sprintf("/api/products/%d", p.ID)
	// Developer changing the flag: 403 and nothing in the request is saved.
	if code, _ := call(t, r, "Developer", http.MethodPut, base, map[string]any{"name": "Renamed", "allow_non_automation_work": true}); code != http.StatusForbidden {
		t.Fatalf("developer toggle: %d", code)
	}
	if got := mustCall(t, r, "Developer", http.MethodGet, base, nil, 200); got.Name != "HISv3" || got.AllowNonAutomationWork {
		t.Fatalf("nothing may be saved on 403: %+v", got)
	}
	// Same value from a Developer is fine (no change); legacy repo fields are ignored with 200.
	got := mustCall(t, r, "Developer", http.MethodPut, base, map[string]any{"name": "Renamed", "allow_non_automation_work": false,
		"repo_url": "https://github.com/evil/x", "root_dir": "evil", "branch": "x"}, 200)
	if got.Name != "Renamed" || got.RootDir != "o/auto" || got.RepoURL != "https://github.com/o/auto" {
		t.Fatalf("legacy fields must be ignored: %+v", got)
	}
	if got := mustCall(t, r, "Admin", http.MethodPut, base, map[string]any{"allow_non_automation_work": true}, 200); !got.AllowNonAutomationWork {
		t.Fatal("admin toggle")
	}
	if got := mustCall(t, r, "DevOps", http.MethodPut, base, map[string]any{"allow_non_automation_work": false}, 200); got.AllowNonAutomationWork {
		t.Fatal("devops toggle")
	}
	// Omitted flag leaves it unchanged.
	mustCall(t, r, "Admin", http.MethodPut, base, map[string]any{"allow_non_automation_work": true}, 200)
	if got := mustCall(t, r, "Developer", http.MethodPut, base, map[string]any{"staging_url": "https://new.example.com"}, 200); !got.AllowNonAutomationWork || got.StagingURL != "https://new.example.com" {
		t.Fatalf("flag must be kept: %+v", got)
	}
	if code, _ := call(t, r, "Developer", http.MethodPut, base, map[string]any{"staging_url": "ftp://x"}); code != http.StatusBadRequest {
		t.Fatalf("bad staging: %d", code)
	}
	// Delete cascades.
	mustCall(t, r, "Admin", http.MethodDelete, base, nil, http.StatusNoContent)
	if n := countRows(t, r, `SELECT COUNT(*) FROM product_repos`) + countRows(t, r, `SELECT COUNT(*) FROM product_settings`); n != 0 {
		t.Fatalf("cascade left %d rows", n)
	}
}

func countRows(t *testing.T, r *Router, q string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQL.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestProductsAPI_RepoLessProduct(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	base := fmt.Sprintf("/api/products/%d", p.ID)
	sib := find(t, p, "his-v3-ai")
	// Simulate a backfill conflict / rollback window: the product has no repo rows.
	if _, err := r.db.SQL.Exec(`DELETE FROM product_repos WHERE product_id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	got := mustCall(t, r, "Admin", http.MethodGet, base, nil, 200)
	if got.Repos == nil || len(got.Repos) != 0 {
		t.Fatalf("repos must be an empty list: %+v", got.Repos)
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodPut, fmt.Sprintf("%s/repos/%d", base, sib.ID)},
		{http.MethodDelete, fmt.Sprintf("%s/repos/%d", base, sib.ID)},
		{http.MethodPut, fmt.Sprintf("%s/repos/%d/branches", base, sib.ID)},
		{http.MethodPost, base + "/repos"}, // not main
	} {
		body := map[string]any{"name": "n", "repo_url": "https://github.com/o/n", "root_dir": "o/n"}
		if strings.HasSuffix(c.path, "/branches") {
			body = map[string]any{"branches": []string{"main"}, "default_branch": "main"}
		}
		code, b := call(t, r, "Admin", c.method, c.path, body)
		if code != http.StatusConflict || !strings.Contains(string(b), "no main repo") {
			t.Errorf("%s %s: want 409 no main repo, got %d %s", c.method, c.path, code, b)
		}
	}
	got = mustCall(t, r, "Admin", http.MethodPost, base+"/repos", map[string]any{"name": "new-main", "repo_url": "https://github.com/o/nm", "root_dir": "o/nm", "is_main": true}, http.StatusCreated)
	if len(got.Repos) != 1 || !got.Repos[0].IsMain || !got.Repos[0].IsAutomation {
		t.Fatalf("adding a main repo must repair the product and make it automation: %+v", got.Repos)
	}
	assertMirror(t, r, p.ID, "repair")
}

func TestRepoWriteAllowedDB(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	ctx := context.Background()
	main, sib := find(t, p, "automation"), find(t, p, "his-v3-ai")
	if err := repoWriteAllowed(ctx, r.db, main.ID, RepoOpPush); err != nil {
		t.Fatalf("main+automation: %v", err)
	}
	if err := repoWriteAllowed(ctx, r.db, sib.ID, RepoOpPush); err != ErrWriteNotAllowed {
		t.Fatalf("sibling, flag off: %v", err)
	}
	mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"allow_non_automation_work": true}, 200)
	if err := repoWriteAllowed(ctx, r.db, sib.ID, RepoOpOpenPR); err != nil {
		t.Fatalf("sibling, flag on: %v", err)
	}
	if err := repoWriteAllowed(ctx, r.db, 987654, RepoOpPush); err != ErrRepoNotFound {
		t.Fatalf("unknown repo: %v", err)
	}
	// Split roles: a main repo that is not automation is read-only while the flag is off.
	sp := createSplit(t, r, "https://s.example.com")
	aiMain, auto := find(t, sp, "his-v3-ai"), find(t, sp, "automation-test-hisv3")
	if !aiMain.IsMain || aiMain.IsAutomation || auto.IsMain || !auto.IsAutomation {
		t.Fatalf("split roles: %+v", sp.Repos)
	}
	if err := repoWriteAllowed(ctx, r.db, auto.ID, RepoOpPush); err != nil {
		t.Fatalf("automation: %v", err)
	}
	if err := repoWriteAllowed(ctx, r.db, aiMain.ID, RepoOpCreateBranch); err != ErrWriteNotAllowed {
		t.Fatalf("main-only repo, flag off: %v", err)
	}
	mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", sp.ID), map[string]any{"allow_non_automation_work": true}, 200)
	if err := repoWriteAllowed(ctx, r.db, aiMain.ID, RepoOpCreateBranch); err != nil {
		t.Fatalf("main-only repo, flag on: %v", err)
	}
}

// Randomised operations; after every one the legacy columns must equal the automation repo's values.
func TestMirrorDrift(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	base := fmt.Sprintf("/api/products/%d", p.ID)
	rng := rand.New(rand.NewSource(7))
	n := 0
	for i := 0; i < 60; i++ {
		cur, err := fetchProduct(context.Background(), r.db, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		pick := cur.Repos[rng.Intn(len(cur.Repos))]
		var desc string
		switch rng.Intn(7) {
		case 0:
			n++
			desc = "add"
			call(t, r, "Admin", http.MethodPost, base+"/repos", map[string]any{"name": fmt.Sprintf("r%d", n), "repo_url": fmt.Sprintf("https://github.com/o/r%d", n),
				"root_dir": fmt.Sprintf("o/r%d", n), "is_main": rng.Intn(3) == 0, "is_automation": rng.Intn(3) == 0, "branches": []string{"main", "dev"}, "default_branch": "main"})
		case 1:
			desc = "promote"
			call(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, pick.ID), map[string]any{"is_main": true})
		case 2:
			desc = "branches"
			def := []string{"main", "dev", "x/y"}[rng.Intn(3)]
			call(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d/branches", base, pick.ID), map[string]any{"branches": []string{"main", "dev", "x/y"}, "default_branch": def})
		case 3:
			desc = "delete"
			call(t, r, "Admin", http.MethodDelete, fmt.Sprintf("%s/repos/%d", base, pick.ID), nil)
		case 4:
			n++
			desc = "edit dir"
			call(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, pick.ID), map[string]any{"root_dir": fmt.Sprintf("o/moved%d", n)})
		case 6:
			desc = "automate"
			call(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, pick.ID), map[string]any{"is_automation": true})
		case 5:
			desc = "verify store"
			rp := pick
			_ = r.storeVerify(context.Background(), p.ID, rp, cur.StagingURL, repoReport(rng.Intn(2) == 0))
		}
		assertMirror(t, r, p.ID, fmt.Sprintf("step %d (%s)", i, desc))
	}
}

// Parallel promotions leave exactly one main and one automation repo and a consistent mirror.
func TestParallelPromotions(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	base := fmt.Sprintf("/api/products/%d", p.ID)
	p = mustCall(t, r, "Admin", http.MethodPost, base+"/repos", map[string]any{"name": "third", "repo_url": "https://github.com/o/third", "root_dir": "o/third"}, 201)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		rp := p.Repos[i%len(p.Repos)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, b := call(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, rp.ID), map[string]any{"is_main": true})
			if code != 200 {
				t.Errorf("promotion: %d %s", code, b)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.storeVerify(context.Background(), p.ID, rp, p.StagingURL, repoReport(true))
		}()
	}
	wg.Wait()
	assertMirror(t, r, p.ID, "parallel")
}

func repoReport(ok bool) repopool.Report {
	return repopool.Report{OK: ok, Checks: []repopool.Check{{Name: "root_dir", OK: ok}}, CheckedAt: "2026-11-13T00:00:00Z"}
}

// Parallel automation promotions racing storeVerify: exactly one automation repo, consistent mirror.
func TestParallelAutomationPromotions(t *testing.T) {
	r := newDBRouter(t)
	p := createHISv3(t, r)
	base := fmt.Sprintf("/api/products/%d", p.ID)
	p = mustCall(t, r, "Admin", http.MethodPost, base+"/repos", map[string]any{"name": "third", "repo_url": "https://github.com/o/third", "root_dir": "o/third"}, 201)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		rp := p.Repos[i%len(p.Repos)]
		wg.Add(2)
		go func() {
			defer wg.Done()
			code, b := call(t, r, "Admin", http.MethodPut, fmt.Sprintf("%s/repos/%d", base, rp.ID), map[string]any{"is_automation": true})
			if code != 200 {
				t.Errorf("automation promotion: %d %s", code, b)
			}
		}()
		go func() {
			defer wg.Done()
			_ = r.storeVerify(context.Background(), p.ID, rp, p.StagingURL, repoReport(true))
		}()
	}
	wg.Wait()
	assertMirror(t, r, p.ID, "parallel automation")
	if n := countRows(t, r, `SELECT COUNT(*) FROM product_repos WHERE product_id=? AND is_automation=1`, p.ID); n != 1 {
		t.Fatalf("want one automation repo, got %d", n)
	}
}

// degrade puts a product into a degraded state by SQL (a rollback window or a backfill conflict).
func degrade(t *testing.T, r *Router, pid int, state string) {
	t.Helper()
	q := map[string]string{
		"S1":       `UPDATE product_repos SET is_main=0 WHERE product_id=?`,
		"S2":       `UPDATE product_repos SET is_automation=0 WHERE product_id=?`,
		"both":     `UPDATE product_repos SET is_main=0, is_automation=0 WHERE product_id=?`,
		"repoless": `DELETE FROM product_repos WHERE product_id=?`,
	}[state]
	if _, err := r.db.SQL.Exec(q, pid); err != nil {
		t.Fatal(err)
	}
}

// The precedence table: main is checked first (errNoMain), then automation (errNoAutomation).
func TestPrecedenceTable(t *testing.T) {
	newRepoBody := func(extra map[string]any) map[string]any {
		b := map[string]any{"name": "added", "repo_url": "https://github.com/o/added", "root_dir": "o/added", "branches": []string{"main"}, "default_branch": "main"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	const noMain, noAuto = "no main repo", "no automation repo"
	type step struct {
		name, method, path string
		body               map[string]any
		want               int
		text               string
	}
	for _, state := range []string{"S1", "S2", "both", "repoless"} {
		t.Run(state, func(t *testing.T) {
			r := newDBRouter(t)
			r.repoPool = repopool.New(t.TempDir()) // so the legacy routes get past the pool check
			p := createSplit(t, r, "http://127.0.0.1:1")
			main, auto := find(t, p, "his-v3-ai"), find(t, p, "automation-test-hisv3")
			degrade(t, r, p.ID, state)
			base := fmt.Sprintf("/api/products/%d", p.ID)
			rid := auto.ID
			if state == "S1" || state == "both" {
				rid = main.ID
			}
			firstText := noMain
			if state == "S2" {
				firstText = noAuto
			}
			steps := []step{
				{"GET", "GET", base, nil, 200, ""},
				{"PUT product", "PUT", base, map[string]any{"name": "Renamed"}, 200, ""},
				{"PUT repo", "PUT", fmt.Sprintf("%s/repos/%d", base, rid), map[string]any{"name": "renamed"}, 409, firstText},
				{"DELETE repo", "DELETE", fmt.Sprintf("%s/repos/%d", base, rid), nil, 409, firstText},
				{"PUT branches", "PUT", fmt.Sprintf("%s/repos/%d/branches", base, rid), map[string]any{"branches": []string{"a"}, "default_branch": "a"}, 409, firstText},
				{"legacy verify", "POST", base + "/verify", nil, 409, firstText},
				{"legacy clone", "POST", base + "/clone", nil, 409, firstText},
				{"legacy checkout", "POST", base + "/checkout", map[string]any{"branch": "x"}, 409, firstText},
				{"POST plain repo", "POST", base + "/repos", newRepoBody(nil), 409, firstText},
			}
			for _, s := range steps {
				code, b := call(t, r, "Admin", s.method, s.path, s.body)
				if code != s.want || !strings.Contains(string(b), s.text) {
					t.Errorf("%s: want %d %q, got %d %s", s.name, s.want, s.text, code, b)
				}
			}
			if state != "repoless" {
				// Per-repo verify/clone/checkout go through loadRepoForRoute (pool routes would reach git).
				if _, _, err := loadRepoForRoute(context.Background(), r.db, p.ID, rid); err == nil || !strings.Contains(err.Error(), firstText) {
					t.Errorf("loadRepoForRoute: %v", err)
				}
				if code, b := call(t, r, "Admin", "POST", fmt.Sprintf("%s/repos/%d/verify", base, rid), nil); code != 409 || !strings.Contains(string(b), firstText) {
					t.Errorf("per-repo verify: %d %s", code, b)
				}
			}
			// The scheduler skips the product (no report stored for any repo).
			r.verifyAllProducts(context.Background())
			if n := countRows(t, r, `SELECT COUNT(*) FROM product_repos WHERE product_id=? AND last_verified_at IS NOT NULL`, p.ID); n != 0 {
				t.Errorf("verifyAllProducts must skip a degraded product, %d repos were verified", n)
			}

			switch state {
			case "S1": // automation exists, main is missing: is_main:true repairs; the new repo is not automation.
				got := mustCall(t, r, "Admin", "POST", base+"/repos", newRepoBody(map[string]any{"is_main": true}), 201)
				if !find(t, got, "added").IsMain || find(t, got, "added").IsAutomation || !find(t, got, "automation-test-hisv3").IsAutomation {
					t.Errorf("S1 repair: %+v", got.Repos)
				}
				assertMirror(t, r, p.ID, "S1 repair")
			case "S2": // main exists, automation missing: only is_automation:true is accepted.
				if code, b := call(t, r, "Admin", "POST", base+"/repos", newRepoBody(map[string]any{"is_main": true})); code != 409 || !strings.Contains(string(b), noAuto) {
					t.Errorf("S2 POST is_main only: %d %s", code, b)
				}
				got := mustCall(t, r, "Admin", "PUT", fmt.Sprintf("%s/repos/%d", base, main.ID), map[string]any{"is_automation": true, "name": "ai-renamed"}, 200)
				if rp := find(t, got, "ai-renamed"); !rp.IsAutomation || !rp.IsMain {
					t.Errorf("S2 PUT with other fields: %+v", got.Repos)
				}
				assertMirror(t, r, p.ID, "S2 repair")
			case "both": // is_main alone is not enough: no automation repo exists.
				if code, b := call(t, r, "Admin", "POST", base+"/repos", newRepoBody(map[string]any{"is_main": true})); code != 409 || !strings.Contains(string(b), noAuto) {
					t.Errorf("both POST is_main only: %d %s", code, b)
				}
				got := mustCall(t, r, "Admin", "POST", base+"/repos", newRepoBody(map[string]any{"is_main": true, "is_automation": true}), 201)
				if rp := find(t, got, "added"); !rp.IsMain || !rp.IsAutomation {
					t.Errorf("both repair: %+v", got.Repos)
				}
				assertMirror(t, r, p.ID, "both repair")
			case "repoless": // a repo-less product: the new main repo is automation by implication.
				got := mustCall(t, r, "Admin", "POST", base+"/repos", newRepoBody(map[string]any{"is_main": true}), 201)
				if rp := find(t, got, "added"); !rp.IsMain || !rp.IsAutomation {
					t.Errorf("repoless repair: %+v", got.Repos)
				}
				assertMirror(t, r, p.ID, "repoless repair")
			}
		})
	}
}

// Verify: only the automation repo gets the Playwright and staging checks; the product-level
// playwright_accessible equals the automation repo's; a staging change clears only its report.
func TestVerifySplitRoles(t *testing.T) {
	staging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer staging.Close()
	r := newDBRouter(t)
	pool := repopool.New(t.TempDir())
	pool.HTTP = staging.Client()
	r.repoPool = pool
	p := createSplit(t, r, staging.URL)
	mainRepo, auto := find(t, p, "his-v3-ai"), find(t, p, "automation-test-hisv3")

	for _, c := range []struct {
		rp         models.ProductRepo
		playwright bool
	}{{mainRepo, false}, {auto, true}} {
		dir := filepath.Join(pool.Root, filepath.FromSlash(c.rp.RootDir))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", c.rp.RepoURL}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Skipf("git unavailable: %v %s", err, out)
			}
		}
		if c.playwright {
			if err := os.WriteFile(filepath.Join(dir, "playwright.config.ts"), []byte("export default {}"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx := context.Background()
	for _, rp := range p.Repos {
		if err := r.verifyAndStore(ctx, p, rp); err != nil {
			t.Fatal(err)
		}
	}
	p, err := fetchProduct(ctx, r.db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	checks := func(rp models.ProductRepo) map[string]bool {
		var rep repopool.Report
		if err := json.Unmarshal(rp.VerifyReport, &rep); err != nil {
			t.Fatalf("report of %s: %v", rp.Name, err)
		}
		out := map[string]bool{}
		for _, c := range rep.Checks {
			out[c.Name] = c.OK
		}
		return out
	}
	mainRepo, auto = find(t, p, "his-v3-ai"), find(t, p, "automation-test-hisv3")
	if got := checks(auto); !got["playwright"] || !got["staging_url"] || !auto.PlaywrightAccessible {
		t.Fatalf("automation report must have playwright and staging_url: %v", got)
	}
	got := checks(mainRepo)
	if _, ok := got["playwright"]; ok {
		t.Fatalf("main-only repo must not get the playwright check: %v", got)
	}
	if _, ok := got["staging_url"]; ok {
		t.Fatalf("main-only repo must not get the staging probe: %v", got)
	}
	if !got["git_repo"] || !got["origin_matches"] || !mainRepo.PlaywrightAccessible {
		t.Fatalf("main-only repo gets the git checks: %v", got)
	}
	if p.PlaywrightAccessible != auto.PlaywrightAccessible {
		t.Fatal("product playwright_accessible must equal the automation repo's")
	}
	assertMirror(t, r, p.ID, "verify")

	// A staging change clears only the automation repo's state and re-mirrors.
	p = mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"staging_url": "https://other.example.com"}, 200)
	if a := find(t, p, "automation-test-hisv3"); a.LastVerifiedAt != nil || a.PlaywrightAccessible {
		t.Fatalf("automation state must be cleared: %+v", a)
	}
	if m := find(t, p, "his-v3-ai"); m.LastVerifiedAt == nil {
		t.Fatal("the main-only repo's state must survive a staging change")
	}
	if p.PlaywrightAccessible || p.LastVerifiedAt != nil {
		t.Fatalf("product-level state must follow the automation repo: %+v", p)
	}
	assertMirror(t, r, p.ID, "staging change")

	// A result computed for a repo that is no longer automation is dropped.
	p = mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d/repos/%d", p.ID, mainRepo.ID), map[string]any{"is_automation": true}, 200)
	if err := r.storeVerify(ctx, p.ID, auto, "https://other.example.com", repoReport(true)); err != nil {
		t.Fatal(err)
	}
	p, _ = fetchProduct(ctx, r.db, p.ID)
	if a := find(t, p, "automation-test-hisv3"); a.LastVerifiedAt != nil {
		t.Fatalf("a result computed for a demoted automation repo must be dropped: %+v", a)
	}
	assertMirror(t, r, p.ID, "stale")
}

// A create without is_automation defaults it to main; adding a repo with is_automation promotes it.
func TestCreateDefaultsAutomationToMain(t *testing.T) {
	r := newDBRouter(t)
	p := mustCall(t, r, "Admin", http.MethodPost, "/api/products", map[string]any{
		"name": "D", "staging_url": "https://s.example.com",
		"repos": []map[string]any{
			{"name": "m", "repo_url": "https://github.com/o/m", "root_dir": "o/m", "is_main": true, "branches": []string{"main"}, "default_branch": "main"},
			{"name": "s", "repo_url": "https://github.com/o/s", "root_dir": "o/s"},
		}}, 201)
	if !find(t, p, "m").IsAutomation || find(t, p, "s").IsAutomation {
		t.Fatalf("default: %+v", p.Repos)
	}
	assertMirror(t, r, p.ID, "default")
	if err := r.storeVerify(context.Background(), p.ID, find(t, p, "m"), p.StagingURL, repoReport(true)); err != nil {
		t.Fatal(err)
	}
	p = mustCall(t, r, "Admin", http.MethodPost, fmt.Sprintf("/api/products/%d/repos", p.ID), map[string]any{
		"name": "auto", "repo_url": "https://github.com/o/auto", "root_dir": "o/auto", "is_automation": true, "branches": []string{"main"}, "default_branch": "main"}, 201)
	if find(t, p, "m").IsAutomation || !find(t, p, "auto").IsAutomation || find(t, p, "m").LastVerifiedAt != nil || p.RootDir != "o/auto" {
		t.Fatalf("add with is_automation: %+v", p.Repos)
	}
	assertMirror(t, r, p.ID, "add automation")
	// A second automation repo at the DB level is 1062, mapped to 409 by key name.
	_, err := r.db.SQL.Exec(`UPDATE product_repos SET is_automation=1 WHERE product_id=? AND name='s'`, p.ID)
	if msg, ok := conflictMessage(err); !ok || msg != "the product already has an automation repo" {
		t.Fatalf("unique key must map to 409: %v", err)
	}
}
