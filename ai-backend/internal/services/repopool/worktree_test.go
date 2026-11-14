package repopool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func wtGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// automationPool builds a pool with a clone "auto" whose origin is a local bare repo.
func automationPool(t *testing.T) (*Pool, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	origin := filepath.Join(tmp, "origin.git")
	wtGit(t, tmp, "init", "--bare", origin)
	seed := filepath.Join(tmp, "seed")
	wtGit(t, tmp, "clone", origin, seed)
	if err := os.WriteFile(filepath.Join(seed, "AGENTS.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtGit(t, seed, "add", ".")
	wtGit(t, seed, "commit", "-m", "init")
	wtGit(t, seed, "branch", "-M", "main")
	wtGit(t, seed, "push", "origin", "main")
	root := filepath.Join(tmp, "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	wtGit(t, root, "clone", origin, "auto")
	return New(root), root
}

func TestAutomationWorktreeLifecycle(t *testing.T) {
	p, root := automationPool(t)
	ctx := context.Background()
	if err := p.FetchOrigin(ctx, "auto"); err != nil {
		t.Fatal(err)
	}
	branch := AutoBranch("TC-AB-001")
	if branch != "qa-auto/tc-ab-001" {
		t.Fatal(branch)
	}
	wt, err := p.AddWorktree(ctx, "auto", branch, "main")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "auto.branches", "qa-auto--tc-ab-001"); wt != want {
		// the pool root may be a symlinked temp dir; compare resolved paths
		a, _ := filepath.EvalSymlinks(wt)
		b, _ := filepath.EvalSymlinks(want)
		if a != b {
			t.Fatalf("wt=%s want %s", wt, want)
		}
	}
	// node_modules link: none in the clone -> nothing; then with one -> relative link.
	if ok, err := p.LinkNodeModules("auto", wt); ok || err != nil {
		t.Fatalf("no node_modules: %v %v", ok, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "auto", "node_modules", ".bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if ok, err := p.LinkNodeModules("auto", wt); !ok || err != nil {
		t.Fatalf("link: %v %v", ok, err)
	}
	target, _ := os.Readlink(filepath.Join(wt, "node_modules"))
	if filepath.IsAbs(target) {
		t.Fatalf("link must be relative: %s", target)
	}
	if _, err := os.Stat(filepath.Join(wt, "node_modules", ".bin")); err != nil {
		t.Fatal("link does not resolve:", err)
	}

	spec := "tests/web/ab/TC-AB-001.spec.ts"
	if err := os.MkdirAll(filepath.Join(wt, "tests/web/ab"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, spec), []byte("test('TC-AB-001 x', async () => {});\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stat, err := p.DiffStat(ctx, wt, []string{spec})
	if err != nil || !strings.Contains(stat, "TC-AB-001.spec.ts") {
		t.Fatalf("stat %q %v", stat, err)
	}
	if err := p.Commit(ctx, wt, "test(TC-AB-001): automate x", []string{spec}); err != nil {
		t.Fatal(err)
	}
	if err := p.Push(ctx, wt, branch); err != nil {
		t.Fatal(err)
	}
	if out := wtGit(t, filepath.Join(filepath.Dir(root), "origin.git"), "branch", "--list", branch); !strings.Contains(out, "qa-auto/tc-ab-001") {
		t.Fatalf("branch not on origin: %q", out)
	}
	if got, err := p.OpenWorktree("auto", branch); err != nil || got != wt {
		t.Fatalf("open: %q %v", got, err)
	}
	// A second AddWorktree replaces the leftover one.
	if _, err := p.AddWorktree(ctx, "auto", branch, "main"); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, spec)); err == nil {
		t.Fatal("re-added worktree must start clean from origin/main")
	}
}

func TestAutomationWorktreeRefusals(t *testing.T) {
	p, root := automationPool(t)
	ctx := context.Background()
	for _, b := range []string{"main", "feature/x", "qa-auto/../x", ""} {
		if _, err := p.AddWorktree(ctx, "auto", b, "main"); err == nil {
			t.Errorf("branch %q accepted", b)
		}
	}
	if _, err := p.AddWorktree(ctx, "auto", "qa-auto/tc-a-001", "nope"); err == nil {
		t.Error("missing base accepted")
	}
	if err := p.Push(ctx, root, "qa-auto/x"); err == nil {
		t.Error("push from a non-worktree accepted")
	}
	if err := p.Commit(ctx, filepath.Join(root, "auto"), "m", []string{"a"}); err == nil {
		t.Error("commit in the main clone accepted")
	}
	wt, err := p.AddWorktree(ctx, "auto", "qa-auto/tc-a-001", "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"../x"}, {"-A"}, {"a b"}, {}, {"/etc/passwd"}} {
		if err := p.Commit(ctx, wt, "m", bad); err == nil {
			t.Errorf("paths %v accepted", bad)
		}
	}
	if _, err := p.OpenPR(ctx, wt, "main", "qa-auto/tc-a-001", "--title", "b"); err == nil {
		t.Error("option-like title accepted")
	}
}

func TestHostPathAndParsePRURL(t *testing.T) {
	p, root := automationPool(t)
	wt, err := p.AddWorktree(context.Background(), "auto", "qa-auto/tc-a-001", "main")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.HostPath("/srv/repos", wt)
	if err != nil || got != "/srv/repos/auto.branches/qa-auto--tc-a-001" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := p.HostPath("/srv/repos", filepath.Dir(root)); err == nil {
		t.Error("path outside the pool accepted")
	}
	if _, err := p.HostPath("relative", wt); err == nil {
		t.Error("relative host root accepted")
	}
	url, err := ParsePRURL("Creating pull request\nhttps://github.com/o/r/pull/12\n")
	if err != nil || url != "https://github.com/o/r/pull/12" {
		t.Fatalf("%q %v", url, err)
	}
	if _, err := ParsePRURL("nothing"); err == nil {
		t.Error("no URL must be an error")
	}
}
