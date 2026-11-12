package repopool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
