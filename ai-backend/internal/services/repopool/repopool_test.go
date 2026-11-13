package repopool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRepoURL(t *testing.T) {
	good := []string{"https://github.com/org/repo", "https://github.com/org/repo.git", "https://github.com/a-b/c_d.e"}
	bad := []string{"", "http://github.com/o/r", "https://github.com/o", "https://evil.com/o/r", "git@github.com:o/r.git",
		"https://github.com/o/r; rm -rf /", "https://github.com/o/r extra", "--upload-pack=x", "https://github.com/o/r/tree/main"}
	for _, s := range good {
		if err := ValidateRepoURL(s); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateRepoURL(s); err == nil {
			t.Errorf("%q should be rejected", s)
		}
	}
}

func TestValidateRelDir(t *testing.T) {
	for _, s := range []string{"hisv3", "org/repo", "a.b/c-d_e"} {
		if err := ValidateRelDir(s); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	for _, s := range []string{"", "/abs", "../x", "a/../b", "a//b", ".hidden", "a/.", "x.lock", "a b", "-rf"} {
		if err := ValidateRelDir(s); err == nil {
			t.Errorf("%q should be rejected", s)
		}
	}
}

func TestValidateBranch(t *testing.T) {
	for _, s := range []string{"main", "feature/x-1", "release_1.2"} {
		if err := ValidateBranch(s); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	for _, s := range []string{"", "-f", "--force", "a..b", "a//b", "x/", "x.lock", "a b", "a;b", "$(x)", "a~1"} {
		if err := ValidateBranch(s); err == nil {
			t.Errorf("%q should be rejected", s)
		}
	}
}

func TestNormalizeRepo(t *testing.T) {
	want := "org/repo"
	for _, s := range []string{"https://github.com/Org/Repo.git", "https://github.com/org/repo/", "git@github.com:org/repo.git", "ssh://git@github.com/org/repo"} {
		if got := NormalizeRepo(s); got != want {
			t.Errorf("%q -> %q, want %q", s, got, want)
		}
	}
}

func TestResolveDir_BlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlink unsupported")
	}
	p := New(root)
	if _, err := p.ResolveDir("link/repo"); err == nil {
		t.Fatal("symlink escape must be rejected")
	}
	if _, err := p.ResolveDir("ok/repo"); err != nil {
		t.Fatalf("plain path should resolve: %v", err)
	}
}

func TestHasPlaywright(t *testing.T) {
	d := t.TempDir()
	if ok, _ := HasPlaywright(d); ok {
		t.Fatal("empty dir has no playwright")
	}
	_ = os.MkdirAll(filepath.Join(d, "e2e"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "e2e", "package.json"), []byte(`{"devDependencies":{"@playwright/test":"1"}}`), 0o644)
	if ok, _ := HasPlaywright(d); !ok {
		t.Fatal("should detect playwright in a subdirectory package.json")
	}
}

func TestVerify(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/down" {
			w.WriteHeader(502)
			return
		}
		w.WriteHeader(302) // login redirect still counts as reachable
	}))
	defer srv.Close()

	root := t.TempDir()
	p := New(root)
	// The test server is on loopback, which the production probe client refuses.
	p.HTTP = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	repo := filepath.Join(root, "org", "repo")
	_ = os.MkdirAll(repo, 0o755)
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/Org/Repo.git"}} {
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	_ = os.WriteFile(filepath.Join(repo, "playwright.config.ts"), []byte("export default {}"), 0o644)

	rep := p.Verify(context.Background(), "https://github.com/org/repo", "org/repo", srv.URL)
	if !rep.OK {
		t.Fatalf("expected all checks to pass: %+v", rep.Checks)
	}

	rep = p.Verify(context.Background(), "https://github.com/other/repo", "org/repo", srv.URL+"/down")
	failed := map[string]bool{}
	for _, c := range rep.Checks {
		if !c.OK {
			failed[c.Name] = true
		}
	}
	if rep.OK || !failed["origin_matches"] || !failed["staging_url"] {
		t.Fatalf("expected origin and staging failures: %+v", rep.Checks)
	}

	rep = p.Verify(context.Background(), "https://github.com/org/repo", "missing/dir", srv.URL)
	if rep.OK || rep.Checks[0].Name != "root_dir" || rep.Checks[0].OK {
		t.Fatalf("missing dir must fail root_dir first: %+v", rep.Checks)
	}
}

func TestLockBusy(t *testing.T) {
	p := New(t.TempDir())
	un, err := p.lock("k")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.lock("k"); err != ErrBusy {
		t.Fatalf("want ErrBusy, got %v", err)
	}
	un()
	if un2, err := p.lock("k"); err != nil {
		t.Fatal(err)
	} else {
		un2()
	}
}

func TestProbeClientBlocksLoopbackAndUserinfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	p := New(t.TempDir()) // default probe client
	ok, detail := p.checkURL(context.Background(), srv.URL)
	if ok || !strings.Contains(detail, "not allowed") {
		t.Fatalf("loopback must be refused, got ok=%v detail=%q", ok, detail)
	}
	for _, u := range []string{"http://127.0.0.1:1/", "http://169.254.169.254/latest/meta-data/", "http://0.0.0.0:80/"} {
		if ok, _ := p.checkURL(context.Background(), u); ok {
			t.Fatalf("%s must be refused", u)
		}
	}
	if ok, detail := p.checkURL(context.Background(), "http://user:pw@example.com/"); ok || strings.Contains(detail, "pw") {
		t.Fatalf("userinfo must be rejected without echoing it: %q", detail)
	}
}

func TestOwnerAllowed(t *testing.T) {
	p := New(t.TempDir())
	if !p.OwnerAllowed("https://github.com/anyone/repo") {
		t.Fatal("empty allowlist allows any owner")
	}
	p.AllowedOwners = []string{"developersismedika"}
	if !p.OwnerAllowed("https://github.com/DevelopersISMedika/repo") || p.OwnerAllowed("https://github.com/other/repo") {
		t.Fatal("allowlist must be case-insensitive and exclusive")
	}
	if err := p.Clone(context.Background(), "https://github.com/other/repo", "x/y"); err == nil {
		t.Fatal("clone of a disallowed owner must fail")
	} else if _, ok := err.(*InputError); !ok {
		t.Fatalf("want InputError, got %T", err)
	}
}

func TestChildEnvHasNoSecretsAndScopesSafeDirectory(t *testing.T) {
	t.Setenv("DB_PASSWORD", "supersecret")
	t.Setenv("AI_MANAGER_SECRET_KEY", "alsosecret")
	t.Setenv("GH_TOKEN", "tok")
	env := strings.Join(childEnv("/data/repos/a"), "\n")
	if strings.Contains(env, "supersecret") || strings.Contains(env, "alsosecret") {
		t.Fatal("unrelated secrets must not reach gh/git")
	}
	if !strings.Contains(env, "GH_TOKEN=tok") || !strings.Contains(env, "safe.directory") || strings.Contains(env, "GIT_CONFIG_VALUE_1=*") {
		t.Fatalf("env wrong: %s", env)
	}
	two := strings.Join(childEnv("/data/repos/a", "/data/repos/a.branches/dev", ""), "\n")
	if !strings.Contains(two, "GIT_CONFIG_COUNT=3") ||
		!strings.Contains(two, "GIT_CONFIG_VALUE_1=/data/repos/a\n") && !strings.Contains(two, "GIT_CONFIG_VALUE_1=/data/repos/a") ||
		!strings.Contains(two, "GIT_CONFIG_VALUE_2=/data/repos/a.branches/dev") {
		t.Fatalf("two safe dirs must yield two keys (empty skipped): %s", two)
	}
	if none := strings.Join(childEnv(), "\n"); !strings.Contains(none, "GIT_CONFIG_COUNT=1") {
		t.Fatalf("no safe dir: %s", none)
	}
}

func TestValidateRelDir_BansBranchesSuffix(t *testing.T) {
	for _, s := range []string{"a.branches", "a.BRANCHES/x", "x/y.Branches", "x/y.branches/z"} {
		if err := ValidateRelDir(s); err == nil {
			t.Errorf("%q must be rejected", s)
		}
	}
	for _, s := range []string{"a.branchesx", "branches", "a/branches"} {
		if err := ValidateRelDir(s); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
}

func TestSlugAndValidateSlug(t *testing.T) {
	if Slug("release/1.0") != "release--1.0" || Slug("main") != "main" {
		t.Fatal("slug mapping wrong")
	}
	for _, s := range []string{"main", "a--b", "x.y_z"} {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	for _, s := range []string{"", ".", "..", "-x", "a/b", strings.Repeat("a", 101)} {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("%q should be rejected", s)
		}
	}
}

func TestResolveWorktreeDir(t *testing.T) {
	root := t.TempDir()
	p := New(root)
	got, err := p.ResolveWorktreeDir("o/r", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "/o/r.branches/dev") {
		t.Fatalf("path: %s", got)
	}
	for _, c := range [][2]string{{"o/r", ".."}, {"o/r", "a/b"}, {"../x", "dev"}, {"o/r.lock", "dev"}, {"o/r", strings.Repeat("a", 101)}, {"o/r", ""}} {
		if _, err := p.ResolveWorktreeDir(c[0], c[1]); err == nil {
			t.Errorf("%v must be rejected", c)
		}
	}
	// The stored root_dir is not re-banned for ".branches" (it was vetted at write time).
	if _, err := p.ResolveWorktreeDir("a.branches", "dev"); err != nil {
		t.Errorf("stored root_dir is not re-validated against the ban: %v", err)
	}
	// Longest legal pair: 200 + len(".branches/") + 100 = 310, exactly the cap.
	if _, err := p.ResolveWorktreeDir(strings.Repeat("a", 200), strings.Repeat("s", 100)); err != nil {
		t.Errorf("maximum legal worktree path must resolve: %v", err)
	}
}

func TestResolveWorktreeDir_Symlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "o"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "o", "r.branches")); err != nil {
		t.Skip("symlink unsupported")
	}
	p := New(root)
	if _, err := p.ResolveWorktreeDir("o/r", "dev"); err == nil {
		t.Fatal(".branches symlinked outside the pool must be rejected")
	}
	// A symlink at the slug directory that points at another in-pool checkout.
	root2 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root2, "o", "r.branches"), 0o755)
	_ = os.MkdirAll(filepath.Join(root2, "other"), 0o755)
	if err := os.Symlink(filepath.Join(root2, "other"), filepath.Join(root2, "o", "r.branches", "dev")); err != nil {
		t.Skip("symlink unsupported")
	}
	if _, err := New(root2).ResolveWorktreeDir("o/r", "dev"); err == nil {
		t.Fatal("symlink at the slug directory must be rejected")
	}
}

func TestValidateBranchSpecs(t *testing.T) {
	ok := []BranchSpec{{"main", true}, {"release/1.0", false}}
	if def, err := validateBranchSpecs(ok); err != nil || def != "main" {
		t.Fatalf("valid specs: %q %v", def, err)
	}
	if def, err := validateBranchSpecs(nil); err != nil || def != "" {
		t.Fatalf("empty (legacy) list is valid: %q %v", def, err)
	}
	bad := [][]BranchSpec{
		{{"-x", true}}, {{"a/../b", true}}, {{"a/b", true}, {"a--b", false}}, {{"main", false}},
		{{"main", true}, {"dev", true}}, {{"main", true}, {"main", false}},
		{{strings.Repeat("a", 101), true}},
	}
	for i, b := range bad {
		if _, err := validateBranchSpecs(b); err == nil {
			t.Errorf("case %d must be rejected: %v", i, b)
		}
	}
}

func TestPorcelainWorktrees(t *testing.T) {
	out := "worktree /p/r\nHEAD abc\nbranch refs/heads/main\n\nworktree /p/r.branches/dev\nHEAD def\nbranch refs/heads/dev\n\nworktree /p/r.branches/det\nHEAD 123\ndetached\n"
	m := porcelainWorktrees(out)
	if m["/p/r"] != "main" || m["/p/r.branches/dev"] != "dev" || m["/p/r.branches/det"] != "" || len(m) != 3 {
		t.Fatalf("parsed: %v", m)
	}
}

func TestVerifyRepoSiblingSkipsPlaywright(t *testing.T) {
	root := t.TempDir()
	p := New(root)
	_ = os.MkdirAll(filepath.Join(root, "o", "r"), 0o755)
	sib := p.VerifyRepo(context.Background(), "https://github.com/o/r", "o/r", false)
	for _, c := range sib.Checks {
		if c.Name == "playwright" || c.Name == "staging_url" {
			t.Fatalf("sibling report must not include %s", c.Name)
		}
	}
	main := p.VerifyRepo(context.Background(), "https://github.com/o/r", "o/r", true)
	found := false
	for _, c := range main.Checks {
		found = found || c.Name == "playwright"
	}
	if !found || main.OK {
		t.Fatalf("main report needs a (failing) playwright check: %+v", main)
	}
}

func TestErrBusyMessage(t *testing.T) {
	if ErrBusy.Error() != "another operation is already running for this repo" {
		t.Fatal(ErrBusy.Error())
	}
}

func TestValidateRepoURL_DotSegments(t *testing.T) {
	for _, s := range []string{"https://github.com/../..", "https://github.com/./x", "https://github.com/o/..", "https://github.com/o/."} {
		if err := ValidateRepoURL(s); err == nil {
			t.Errorf("%q must be rejected", s)
		}
	}
	for _, b := range []string{"HEAD", "a.lock/b", "x.lock"} {
		if err := ValidateBranch(b); err == nil {
			t.Errorf("branch %q must be rejected", b)
		}
	}
}

func TestSafeRemoteHidesUserinfo(t *testing.T) {
	got := safeRemote("https://user:TOKEN123@gitlab.example.com/x/y.git")
	if strings.Contains(got, "TOKEN123") || strings.Contains(got, "user") {
		t.Fatalf("userinfo leaked: %q", got)
	}
}

func TestHasPlaywright_IgnoresLooseMatchesAndSymlinks(t *testing.T) {
	d := t.TempDir()
	// "playwright" only in the package name, not a dependency: must not count.
	_ = os.WriteFile(filepath.Join(d, "package.json"), []byte(`{"name":"playwright-notes","dependencies":{"left-pad":"1"}}`), 0o644)
	if ok, _ := HasPlaywright(d); ok {
		t.Fatal("name containing 'playwright' is not a dependency")
	}
	// A symlinked package.json must not be followed.
	d2 := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	_ = os.WriteFile(target, []byte(`{"devDependencies":{"@playwright/test":"1"}}`), 0o644)
	if err := os.Symlink(target, filepath.Join(d2, "package.json")); err == nil {
		if ok, _ := HasPlaywright(d2); ok {
			t.Fatal("symlinked package.json must not be read")
		}
	}
}

func TestRunKillsProcessGroupOnTimeout(t *testing.T) {
	start := time.Now()
	// sh spawns a child that would outlive a plain kill and hold the pipe open.
	_, err := run(context.Background(), t.TempDir(), nil, 300*time.Millisecond, "sh", "-c", "sleep 30 & sleep 30")
	if err == nil || time.Since(start) > 6*time.Second {
		t.Fatalf("timeout must return promptly with an error (err=%v, took %s)", err, time.Since(start))
	}
}

func TestVerifyBusyWhenLocked(t *testing.T) {
	root := t.TempDir()
	p := New(root)
	abs, err := p.ResolveDir("a/b")
	if err != nil {
		t.Fatal(err)
	}
	un, err := p.lock(abs)
	if err != nil {
		t.Fatal(err)
	}
	defer un()
	if rep := p.Verify(context.Background(), "https://github.com/o/r", "a/b", "https://example.com"); !rep.Busy {
		t.Fatal("verify during a clone/checkout must report Busy and not check")
	}
}
