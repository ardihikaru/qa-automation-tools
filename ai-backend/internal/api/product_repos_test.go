package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/models"
)

func TestRootDirOverlap(t *testing.T) {
	others := []string{"org/repo", "solo"}
	for _, rel := range []string{"org/repo", "org/repo/sub", "org", "solo",
		// Footprints: the worktree area "<root_dir>.branches" is claimed too.
		"org/repo.branches", "org/repo.branches/x", "solo.branches"} {
		if !rootDirOverlap(rel, others) {
			t.Errorf("%q should overlap", rel)
		}
	}
	for _, rel := range []string{"org/repo2", "other", "solo2", "orgx", "org/repository"} {
		if rootDirOverlap(rel, others) {
			t.Errorf("%q should not overlap", rel)
		}
	}
	if rootDirOverlap("a/b", nil) {
		t.Error("no others means no overlap")
	}
}

func TestWriteAllowedTruthTable(t *testing.T) {
	ops := []RepoOp{RepoOpCreateBranch, RepoOpPush, RepoOpOpenPR}
	for _, op := range ops {
		for _, c := range []struct {
			isMain, flag, allowed bool
		}{{true, false, true}, {true, true, true}, {false, true, true}, {false, false, false}} {
			err := writeAllowed(c.isMain, c.flag, op)
			if c.allowed && err != nil || !c.allowed && err != ErrWriteNotAllowed {
				t.Errorf("op=%s main=%v flag=%v: got %v", op, c.isMain, c.flag, err)
			}
		}
	}
	for _, op := range []RepoOp{"", "delete", "PUSH", "clone"} {
		if err := writeAllowed(true, true, op); err != ErrWriteNotAllowed {
			t.Errorf("unknown op %q must be denied, got %v", op, err)
		}
	}
}

func TestConflictMessage(t *testing.T) {
	want := map[string]string{
		"uq_products_name":               "a product with this name already exists",
		"uq_products_root_dir":           "this root_dir is used by another product",
		"uq_product_repos_root_dir":      "root_dir is already used by another repo",
		"uq_product_repos_name":          "a repo with this name already exists in the product",
		"uq_product_repos_url":           "this repository is already listed for the product",
		"uq_product_repos_main":          "the product already has a main repo",
		"uq_product_repos_central":       "the product already has a central repo",
		"uq_product_repo_branches":       "duplicate or colliding branch name",
		"uq_product_repo_branch_slug":    "duplicate or colliding branch name",
		"uq_product_repo_branch_default": "only one default branch is allowed",
	}
	for key, msg := range want {
		for _, form := range []string{"product_repos." + key, key} { // MySQL 8 prefixes the table
			err := fmt.Errorf("wrapped: %w", &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'x' for key '" + form + "'"})
			got, ok := conflictMessage(err)
			if !ok || got != msg {
				t.Errorf("%s: got %q %v", form, got, ok)
			}
		}
	}
	for _, err := range []error{
		&mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'x' for key 't.some_other_key'"},
		&mysql.MySQLError{Number: 1213, Message: "Deadlock"},
		fmt.Errorf("plain"),
	} {
		if _, ok := conflictMessage(err); ok {
			t.Errorf("%v must not map to 409", err)
		}
	}
	rec := httptest.NewRecorder()
	if !writeDBConflict(rec, &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'x' for key 'products.uq_products_name'"}) || rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	writeOpError(rec, "test", fmt.Errorf("secret detail"))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("500 must be generic: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRoleMatrix(t *testing.T) {
	type row struct{ manage, verify, toggle bool }
	want := map[string]row{
		"Admin": {true, true, true}, "DevOps": {true, true, true}, "Developer": {true, true, false},
		"QA": {false, true, false}, "QC": {false, true, false}, "UI/UX Designer": {false, false, false},
	}
	for role, w := range want {
		req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
		req = req.WithContext(auth.SetUser(req.Context(), &auth.User{Role: role}))
		m, v := productRole(req)
		if m != w.manage || v != w.verify || canToggleWork(req) != w.toggle {
			t.Errorf("%s: manage=%v verify=%v toggle=%v", role, m, v, canToggleWork(req))
		}
	}
	if canToggleWork(httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Error("no user must not toggle")
	}
}

func TestOrdering(t *testing.T) {
	repos := []models.ProductRepo{
		{ID: 1, Name: "zeta"}, {ID: 2, Name: "alpha"}, {ID: 3, Name: "central", IsCentral: true}, {ID: 4, Name: "main", IsMain: true},
		{ID: 5, Name: "mid"},
	}
	sortRepos(repos)
	var names []string
	for _, r := range repos {
		names = append(names, r.Name)
	}
	if got := strings.Join(names, ","); got != "main,central,alpha,mid,zeta" {
		t.Fatalf("repo order: %s", got)
	}
	// Main that is also central stays first and is not duplicated by the central rank.
	both := []models.ProductRepo{{Name: "b"}, {Name: "a", IsMain: true, IsCentral: true}}
	sortRepos(both)
	if both[0].Name != "a" {
		t.Fatal("main+central first")
	}
	got := orderBranches([]string{"release/1.0", "dev", "main", "alpha"}, "main")
	if strings.Join(got, ",") != "main,alpha,dev,release/1.0" {
		t.Fatalf("branch order: %v", got)
	}
}

func TestProductJSONShape(t *testing.T) {
	b, err := json.Marshal(models.Product{Repos: []models.ProductRepo{{Branches: []string{"main"}, DefaultBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"allow_non_automation_work", "repos", "repo_url", "root_dir", "branch", "playwright_accessible", "last_verified_at", "verify_report"} {
		if _, ok := m[k]; !ok {
			t.Errorf("product JSON lacks %q", k)
		}
	}
	repo := m["repos"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "product_id", "name", "repo_url", "root_dir", "is_main", "is_central", "default_branch", "branches",
		"playwright_accessible", "last_verified_at", "verify_report", "sync_report", "created_at", "updated_at"} {
		if _, ok := repo[k]; !ok {
			t.Errorf("repo JSON lacks %q", k)
		}
	}
}

func TestNormalizeBranches(t *testing.T) {
	list, def, msg := normalizeBranches([]string{" main ", "release/1.0"}, "main", true)
	if msg != "" || def != "main" || len(list) != 2 || list[0] != "main" {
		t.Fatalf("valid: %v %q %q", list, def, msg)
	}
	if l, d, m := normalizeBranches(nil, "", false); m != "" || len(l) != 0 || d != "" {
		t.Fatalf("legacy empty allowed when not strict: %v %q %q", l, d, m)
	}
	if l, d, m := normalizeBranches(nil, "dev", false); m != "" || len(l) != 1 || d != "dev" {
		t.Fatalf("default alone becomes the list: %v %q %q", l, d, m)
	}
	eleven := []string{}
	for i := 0; i < 11; i++ {
		eleven = append(eleven, fmt.Sprintf("b%d", i))
	}
	bad := []struct {
		list []string
		def  string
	}{
		{nil, ""}, {[]string{"main"}, ""}, {[]string{"main"}, "dev"}, {eleven, "b0"},
		{[]string{"main", "main"}, "main"}, {[]string{"a/b", "a--b"}, "a/b"}, {[]string{"-x"}, "-x"},
		{[]string{"a/../b"}, "a/../b"}, {[]string{strings.Repeat("a", 101)}, strings.Repeat("a", 101)},
	}
	for i, c := range bad {
		if _, _, m := normalizeBranches(c.list, c.def, true); m == "" {
			t.Errorf("case %d must be rejected: %v %q", i, c.list, c.def)
		}
	}
	// Case-sensitive: Main and main coexist.
	if _, _, m := normalizeBranches([]string{"Main", "main"}, "main", true); m != "" {
		t.Errorf("Main and main are different refs: %s", m)
	}
}

func TestValidateRepoAndSet(t *testing.T) {
	good := func() repoBody {
		return repoBody{Name: "main", RepoURL: "https://github.com/o/r", RootDir: "/o/r/", IsMain: true, Branches: []string{"main"}, DefaultBranch: "main"}
	}
	in := good()
	if msg := validateRepo(&in); msg != "" || in.RootDir != "o/r" {
		t.Fatalf("valid repo: %q %+v", msg, in)
	}
	for i, mut := range []func(*repoBody){
		func(r *repoBody) { r.Name = "" },
		func(r *repoBody) { r.RepoURL = "https://evil.com/o/r" },
		func(r *repoBody) { r.RootDir = "../x" },
		func(r *repoBody) { r.RootDir = "a.branches/x" },
		func(r *repoBody) { r.Branches, r.DefaultBranch = []string{"--x"}, "--x" },
	} {
		r := good()
		mut(&r)
		if msg := validateRepo(&r); msg == "" {
			t.Errorf("case %d must be rejected", i)
		}
	}

	m := good()
	s1 := repoBody{Name: "s1", RepoURL: "https://github.com/o/s1", RootDir: "o/s1", IsCentral: true}
	if msg := validateRepoSet([]repoBody{m, s1}); msg != "" {
		t.Fatalf("valid set: %s", msg)
	}
	bads := map[string][]repoBody{
		"none":        {},
		"no main":     {s1},
		"two mains":   {m, {Name: "m2", RepoURL: "https://github.com/o/m2", RootDir: "o/m2", IsMain: true}},
		"two central": {m, s1, {Name: "s2", RepoURL: "https://github.com/o/s2", RootDir: "o/s2", IsCentral: true}},
		"dup name":    {m, {Name: "main", RepoURL: "https://github.com/o/x", RootDir: "o/x"}},
		"dup url":     {m, {Name: "x", RepoURL: "https://github.com/O/R.git", RootDir: "o/x"}},
		"overlap":     {m, {Name: "x", RepoURL: "https://github.com/o/x", RootDir: "o/r/sub"}},
	}
	for name, set := range bads {
		cp := append([]repoBody{}, set...)
		if msg := validateRepoSet(cp); msg == "" {
			t.Errorf("%s must be rejected", name)
		}
	}
}
