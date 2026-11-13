package repopool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/models"
)

// Real clone + branch switch. Needs gh, git, network and GH_TOKEN; set
// REPOPOOL_IT_REPO (https://github.com/<owner>/<repo>) to enable.
func TestIntegration_CloneCheckoutVerify(t *testing.T) {
	repo := os.Getenv("REPOPOOL_IT_REPO")
	if repo == "" || os.Getenv("GH_TOKEN") == "" {
		t.Skip("set REPOPOOL_IT_REPO and GH_TOKEN to run")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}
	p := New(t.TempDir())
	ctx := context.Background()
	if err := p.Clone(ctx, repo, "it/repo"); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if err := p.Clone(ctx, repo, "it/repo"); err == nil {
		t.Fatal("second clone into a non-empty dir must fail")
	}
	br, err := p.Checkout(ctx, "it/repo", "main")
	if err != nil || br != "main" {
		t.Fatalf("checkout main: %q %v", br, err)
	}
	rep := p.Verify(ctx, repo, "it/repo", "https://example.com")
	for _, c := range rep.Checks {
		if c.Name == "git_repo" || c.Name == "origin_matches" || c.Name == "root_dir" {
			if !c.OK {
				t.Fatalf("%s failed: %s", c.Name, c.Detail)
			}
		}
	}
}

// ---- Worktree tests against temporary bare origins (need only git, no network) ----

const itRepoURL = "https://github.com/o/r"

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "protocol.file.allow=always"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// itEnv builds a bare origin with branches main, dev, release/1.0 and a--b, and a pool whose
// o/r directory is a plain clone of it (what `gh repo clone` would have produced).
type itEnv struct {
	pool   *Pool
	origin string
	work   string
	abs    string
}

func newItEnv(t *testing.T) *itEnv {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	work := filepath.Join(base, "work")
	git(t, base, "init", "--bare", "-b", "main", origin)
	git(t, base, "clone", origin, work)
	git(t, work, "checkout", "-b", "main")
	write(t, filepath.Join(work, "README"), "main\n")
	git(t, work, "add", ".")
	git(t, work, "commit", "-m", "init")
	for _, b := range []string{"dev", "release/1.0", "a--b"} {
		git(t, work, "checkout", "-b", b, "main")
		write(t, filepath.Join(work, "f-"+Slug(b)), b+"\n")
		git(t, work, "add", ".")
		git(t, work, "commit", "-m", b)
	}
	git(t, work, "checkout", "main")
	git(t, work, "push", "origin", "--all")

	poolRoot := filepath.Join(base, "pool")
	if err := os.MkdirAll(filepath.Join(poolRoot, "o"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := New(poolRoot)
	abs, err := p.ResolveDir("o/r")
	if err != nil {
		t.Fatal(err)
	}
	git(t, base, "clone", origin, abs)
	return &itEnv{pool: p, origin: origin, work: work, abs: abs}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e *itEnv) sync(t *testing.T, allowPush bool, specs ...BranchSpec) map[string]models.BranchResult {
	t.Helper()
	res, err := e.pool.SyncBranches(context.Background(), itRepoURL, "o/r", specs, allowPush)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	m := map[string]models.BranchResult{}
	for _, r := range res {
		m[r.Name] = r
	}
	return m
}

func (e *itEnv) wt(slug string) string { return e.abs + ".branches/" + slug }

func (e *itEnv) advance(t *testing.T, branch, file string) {
	t.Helper()
	git(t, e.work, "checkout", branch)
	write(t, filepath.Join(e.work, file), "x\n")
	git(t, e.work, "add", ".")
	git(t, e.work, "commit", "-m", "adv "+branch)
	git(t, e.work, "push", "origin", branch)
	git(t, e.work, "checkout", "main")
}

func want(t *testing.T, m map[string]models.BranchResult, name, status string) {
	t.Helper()
	if m[name].Status != status {
		t.Fatalf("%s: want %s, got %+v (all: %+v)", name, status, m[name], m)
	}
}

func TestSync_AddFastForwardRemoveKeepDirty(t *testing.T) {
	e := newItEnv(t)
	m := e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false}, BranchSpec{"release/1.0", false})
	want(t, m, "main", "ok")
	want(t, m, "dev", "ok")
	want(t, m, "release/1.0", "ok")
	for _, f := range []string{e.wt("dev") + "/f-dev", e.wt("release--1.0") + "/f-release--1.0"} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("worktree file missing: %v", err)
		}
	}
	// Fast-forward.
	e.advance(t, "dev", "f-dev2")
	m = e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false}, BranchSpec{"release/1.0", false})
	want(t, m, "dev", "ok")
	if _, err := os.Stat(e.wt("dev") + "/f-dev2"); err != nil {
		t.Fatalf("dev not fast-forwarded: %v", err)
	}
	// Dirty worktree for an unlisted branch is kept; a clean one is removed.
	write(t, e.wt("dev")+"/dirty.txt", "local\n")
	m = e.sync(t, true, BranchSpec{"main", true})
	want(t, m, "dev", "kept_dirty")
	want(t, m, "release/1.0", "ok")
	if _, err := os.Stat(e.wt("dev")); err != nil {
		t.Fatal("dirty worktree must stay")
	}
	if _, err := os.Stat(e.wt("release--1.0")); err == nil {
		t.Fatal("clean worktree must be removed")
	}
}

func TestSync_DefaultSwap(t *testing.T) {
	e := newItEnv(t)
	e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	m := e.sync(t, true, BranchSpec{"main", false}, BranchSpec{"dev", true})
	want(t, m, "dev", "ok")
	want(t, m, "main", "ok")
	if cur := git(t, e.abs, "rev-parse", "--abbrev-ref", "HEAD"); cur != "dev" {
		t.Fatalf("clone must be on dev, is on %s", cur)
	}
	if got := git(t, e.wt("main"), "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Fatalf("main worktree on %s", got)
	}
}

func TestSync_DirtyCloneKeepsDefaultSwitch(t *testing.T) {
	e := newItEnv(t)
	e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	write(t, filepath.Join(e.abs, "README"), "local edit\n")
	m := e.sync(t, true, BranchSpec{"dev", true}, BranchSpec{"main", false})
	want(t, m, "dev", "kept_dirty")
	if cur := git(t, e.abs, "rev-parse", "--abbrev-ref", "HEAD"); cur != "main" {
		t.Fatalf("dirty clone must not be switched, is on %s", cur)
	}
}

func TestSync_SlugCollisionAndBadNames(t *testing.T) {
	e := newItEnv(t)
	for _, specs := range [][]BranchSpec{
		{{"a/b", true}, {"a--b", false}},
		{{"main", true}, {"-x", false}},
		{{"main", true}, {"a/../b", false}},
	} {
		if _, err := e.pool.SyncBranches(context.Background(), itRepoURL, "o/r", specs, true); err == nil {
			t.Errorf("%v must be rejected", specs)
		} else if _, ok := err.(*InputError); !ok {
			t.Errorf("%v: want InputError, got %T", specs, err)
		}
	}
}

func TestSync_ExistingPathNotExpectedWorktree(t *testing.T) {
	e := newItEnv(t)
	if err := os.MkdirAll(e.wt("dev"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, e.wt("dev")+"/keep.txt", "mine\n")
	m := e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	want(t, m, "dev", "error")
	if !strings.Contains(m["dev"].Message, "not the expected worktree") {
		t.Fatalf("message: %q", m["dev"].Message)
	}
	if _, err := os.Stat(e.wt("dev") + "/keep.txt"); err != nil {
		t.Fatal("foreign directory must be left untouched")
	}
}

func TestSync_MissingRemote(t *testing.T) {
	e := newItEnv(t)
	m := e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"nope", false})
	want(t, m, "main", "ok")
	want(t, m, "nope", "missing_remote")
	m = e.sync(t, true, BranchSpec{"ghost", true})
	want(t, m, "ghost", "missing_remote")
}

func TestSync_DivergedBranchIsReportedNotReset(t *testing.T) {
	e := newItEnv(t)
	e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	write(t, e.wt("dev")+"/local.txt", "l\n")
	git(t, e.wt("dev"), "add", ".")
	git(t, e.wt("dev"), "commit", "-m", "local")
	local := git(t, e.wt("dev"), "rev-parse", "HEAD")
	e.advance(t, "dev", "f-remote")
	m := e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	want(t, m, "dev", "error")
	if git(t, e.wt("dev"), "rev-parse", "HEAD") != local {
		t.Fatal("diverged branch must be left alone")
	}
}

func TestSync_SymlinkedBranchesDirIsRefused(t *testing.T) {
	e := newItEnv(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, e.abs+".branches"); err != nil {
		t.Skip("symlink unsupported")
	}
	m := e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	want(t, m, "dev", "error")
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("nothing may be written outside the pool")
	}
}

func TestSync_SymlinkAtSlugDirIsRefused(t *testing.T) {
	e := newItEnv(t)
	other := t.TempDir()
	if err := os.MkdirAll(e.abs+".branches", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, e.wt("dev")); err != nil {
		t.Skip("symlink unsupported")
	}
	m := e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	want(t, m, "dev", "error")
	if entries, _ := os.ReadDir(other); len(entries) != 0 {
		t.Fatal("nothing may be written through the symlink")
	}
}

func TestSync_PushPolicy(t *testing.T) {
	e := newItEnv(t)
	e.sync(t, false, BranchSpec{"main", true}, BranchSpec{"dev", false})
	push := exec.Command("git", "push", "origin", "HEAD")
	push.Dir = e.wt("dev")
	if out, err := push.CombinedOutput(); err == nil {
		t.Fatalf("push must fail while disabled (shared config covers worktrees): %s", out)
	}
	e.sync(t, true, BranchSpec{"main", true}, BranchSpec{"dev", false})
	push = exec.Command("git", "push", "origin", "HEAD")
	push.Dir = e.wt("dev")
	if out, err := push.CombinedOutput(); err != nil {
		t.Fatalf("push must work once enabled: %v\n%s", err, out)
	}
	// Unsetting an unset key (exit 5) is not an error, and SetPushPolicy round-trips.
	if err := e.pool.SetPushPolicy(context.Background(), "o/r", true); err != nil {
		t.Fatalf("unset when already unset: %v", err)
	}
	if err := e.pool.SetPushPolicy(context.Background(), "o/r", false); err != nil {
		t.Fatal(err)
	}
	if got := git(t, e.abs, "config", "--get", "remote.origin.pushurl"); got != "DISABLED" {
		t.Fatalf("pushurl = %q", got)
	}
	// Not a clone yet: no-op.
	if err := e.pool.SetPushPolicy(context.Background(), "x/y", false); err != nil {
		t.Fatalf("missing clone must be a no-op: %v", err)
	}
}

func TestSync_EmptyListStopsAfterFetch(t *testing.T) {
	e := newItEnv(t)
	res, err := e.pool.SyncBranches(context.Background(), itRepoURL, "o/r", nil, true)
	if err != nil || len(res) != 0 {
		t.Fatalf("legacy empty list: %v %v", res, err)
	}
}

func TestSync_BusyWhenLocked(t *testing.T) {
	e := newItEnv(t)
	un, err := e.pool.lock(e.abs)
	if err != nil {
		t.Fatal(err)
	}
	defer un()
	if _, err := e.pool.SyncBranches(context.Background(), itRepoURL, "o/r", []BranchSpec{{"main", true}}, true); err != ErrBusy {
		t.Fatalf("want ErrBusy, got %v", err)
	}
}
