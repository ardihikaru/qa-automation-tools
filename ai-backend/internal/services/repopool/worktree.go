package repopool

// Automation worktree helpers (CASE_AUTOMATION.md decision 3 and section 5): a throw-away worktree
// of the automation repo on its own branch (qa-auto/<code>), a node_modules link to the main
// clone, a local commit, and the two write-class steps (push, pull request).
//
// These helpers do NOT decide whether a write is allowed: the caller (internal/api) must pass
// repoWriteAllowed(RepoOpPush / RepoOpOpenPR) before Push and OpenPR. Every path is confined to
// the pool, every command runs without a shell, and the generated file is the only thing added.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AutoBranchPrefix is the namespace of every branch the case automation creates.
const AutoBranchPrefix = "qa-auto/"

var (
	prURLRe    = regexp.MustCompile(`https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]+`)
	repoPathRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

// AutoBranch returns the branch name for a case code (lower case).
func AutoBranch(code string) string { return AutoBranchPrefix + strings.ToLower(code) }

func validAutoBranch(b string) error {
	if !strings.HasPrefix(b, AutoBranchPrefix) {
		return inputErr("not an automation branch")
	}
	return ValidateBranch(b)
}

// lockWait takes the directory lock, retrying while another operation holds it (a sync or a
// verify is short; a second case job should wait instead of failing).
func (p *Pool) lockWait(ctx context.Context, key string, max time.Duration) (func(), error) {
	deadline := time.Now().Add(max)
	for {
		unlock, err := p.lock(key)
		if err == nil {
			return unlock, nil
		}
		if !errors.Is(err, ErrBusy) || time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// FetchOrigin runs `git fetch --prune origin` in the clone at rel. It changes only remote-tracking
// refs. (SyncBranches is deliberately not used: it would remove clean worktrees it does not know.)
func (p *Pool) FetchOrigin(ctx context.Context, rel string) error {
	abs, err := p.ResolveDir(rel)
	if err != nil {
		return err
	}
	unlock, err := p.lockWait(ctx, abs, 2*time.Minute)
	if err != nil {
		return err
	}
	defer unlock()
	if !isGitRepo(abs) {
		return inputErr("the repo is not cloned yet")
	}
	_, err = run(ctx, abs, []string{abs}, 90*time.Second, "git", "fetch", "--prune", "origin", "+refs/heads/*:refs/remotes/origin/*")
	return err
}

// AddWorktree creates the worktree for branch (qa-auto/...) from origin/base next to the clone at
// rootDir and returns its absolute path. A leftover worktree and local branch of the same name (a
// failed earlier job) are removed first; both are ours by the qa-auto/ prefix.
func (p *Pool) AddWorktree(ctx context.Context, rootDir, branch, base string) (string, error) {
	if err := validAutoBranch(branch); err != nil {
		return "", err
	}
	if err := ValidateBranch(base); err != nil {
		return "", err
	}
	abs, err := p.ResolveDir(rootDir)
	if err != nil {
		return "", err
	}
	wt, err := p.ResolveWorktreeDir(rootDir, Slug(branch))
	if err != nil {
		return "", err
	}
	unlock, err := p.lockWait(ctx, abs, 2*time.Minute)
	if err != nil {
		return "", err
	}
	defer unlock()
	sd := []string{abs, wt}
	if !isGitRepo(abs) {
		return "", inputErr("the repo is not cloned yet")
	}
	if _, err := run(ctx, abs, sd, 10*time.Second, "git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+base); err != nil {
		return "", inputErr("branch " + base + " not found on origin")
	}
	if _, err := os.Lstat(wt); err == nil {
		if _, err := run(ctx, abs, sd, 30*time.Second, "git", "worktree", "remove", "--force", wt); err != nil {
			return "", err
		}
	}
	_, _ = run(ctx, abs, sd, 20*time.Second, "git", "worktree", "prune")
	if _, err := run(ctx, abs, sd, 10*time.Second, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		if _, err := run(ctx, abs, sd, 20*time.Second, "git", "branch", "-D", branch); err != nil {
			return "", err
		}
	}
	if _, err := run(ctx, abs, sd, 60*time.Second, "git", "worktree", "add", "-b", branch, wt, "origin/"+base); err != nil {
		return "", err
	}
	p.FixOwner(filepath.Dir(wt))
	return wt, nil
}

// OpenWorktree returns the path of an existing automation worktree (resume after a restart).
func (p *Pool) OpenWorktree(rootDir, branch string) (string, error) {
	if err := validAutoBranch(branch); err != nil {
		return "", err
	}
	wt, err := p.ResolveWorktreeDir(rootDir, Slug(branch))
	if err != nil {
		return "", err
	}
	if fi, err := os.Lstat(wt); err != nil || !fi.IsDir() {
		return "", inputErr("the worktree is gone")
	}
	return wt, nil
}

// LinkNodeModules points <worktree>/node_modules at the clone's node_modules with a relative
// symlink (valid on the host and in the backend container). It does nothing and returns false
// when the clone has none or the worktree already has one.
func (p *Pool) LinkNodeModules(rootDir, wt string) (bool, error) {
	abs, err := p.ResolveDir(rootDir)
	if err != nil {
		return false, err
	}
	if err := p.checkWorktree(wt); err != nil {
		return false, err
	}
	src := filepath.Join(abs, "node_modules")
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return false, nil
	}
	dst := filepath.Join(wt, "node_modules")
	if _, err := os.Lstat(dst); err == nil {
		return false, nil
	}
	rel, err := filepath.Rel(wt, src)
	if err != nil {
		return false, err
	}
	if err := os.Symlink(rel, dst); err != nil {
		return false, err
	}
	return true, nil
}

// checkWorktree makes sure wt is a real directory inside the pool, under a ".branches" area.
func (p *Pool) checkWorktree(wt string) error {
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(wt)
	if err != nil {
		return inputErr("worktree not found")
	}
	if !strings.HasPrefix(real, root+string(os.PathSeparator)) || !strings.Contains(real, BranchesSuffix+string(os.PathSeparator)) {
		return inputErr("path is not a pool worktree")
	}
	return nil
}

// HostPath is the path of a pool-relative or in-pool absolute path as seen from another process
// that mounts the pool at hostRoot. p is under the backend's pool root.
func (p *Pool) HostPath(hostRoot, abs string) (string, error) {
	if !filepath.IsAbs(hostRoot) {
		return "", inputErr("the pool host directory must be absolute")
	}
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", inputErr("path escapes pool")
	}
	return filepath.Join(hostRoot, rel), nil
}

func validRepoRel(paths []string) error {
	if len(paths) == 0 {
		return inputErr("no paths")
	}
	for _, f := range paths {
		if len(f) > 300 || !repoPathRe.MatchString(f) || strings.Contains(f, "..") || strings.Contains(f, "//") {
			return inputErr("invalid repo path")
		}
	}
	return nil
}

// DiffStat stages paths in the worktree and returns `git diff --cached --stat` (clipped).
func (p *Pool) DiffStat(ctx context.Context, wt string, paths []string) (string, error) {
	if err := p.checkWorktree(wt); err != nil {
		return "", err
	}
	if err := validRepoRel(paths); err != nil {
		return "", err
	}
	sd := []string{wt}
	if _, err := run(ctx, wt, sd, 20*time.Second, "git", append([]string{"add", "--"}, paths...)...); err != nil {
		return "", err
	}
	return run(ctx, wt, sd, 20*time.Second, "git", "diff", "--cached", "--stat")
}

// Commit stages paths and commits them locally. Hooks are off and the author is fixed.
func (p *Pool) Commit(ctx context.Context, wt, message string, paths []string) error {
	if err := p.checkWorktree(wt); err != nil {
		return err
	}
	if err := validRepoRel(paths); err != nil {
		return err
	}
	if strings.TrimSpace(message) == "" || len(message) > 500 {
		return inputErr("invalid commit message")
	}
	sd := []string{wt}
	if _, err := run(ctx, wt, sd, 20*time.Second, "git", append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	_, err := run(ctx, wt, sd, 30*time.Second, "git",
		"-c", "core.hooksPath=/dev/null", "-c", "user.name=qa-automation", "-c", "user.email=qa-automation@sismedika.com",
		"commit", "--no-verify", "-m", message)
	return err
}

// Push pushes branch (qa-auto/...) to origin. WRITE-CLASS: the caller must have passed
// repoWriteAllowed(RepoOpPush). The refspec is explicit and never forced.
func (p *Pool) Push(ctx context.Context, wt, branch string) error {
	if err := validAutoBranch(branch); err != nil {
		return err
	}
	if err := p.checkWorktree(wt); err != nil {
		return err
	}
	_, err := run(ctx, wt, []string{wt}, 120*time.Second, "git", "push", "origin", "refs/heads/"+branch+":refs/heads/"+branch)
	return err
}

// OpenPR runs `gh pr create` for head into base and returns the pull request URL.
// WRITE-CLASS: the caller must have passed repoWriteAllowed(RepoOpOpenPR).
func (p *Pool) OpenPR(ctx context.Context, wt, base, head, title, body string) (string, error) {
	if err := validAutoBranch(head); err != nil {
		return "", err
	}
	if err := ValidateBranch(base); err != nil {
		return "", err
	}
	if err := p.checkWorktree(wt); err != nil {
		return "", err
	}
	if t := strings.TrimSpace(title); t == "" || len(t) > 250 || strings.HasPrefix(t, "-") {
		return "", inputErr("invalid PR title")
	}
	out, err := run(ctx, wt, []string{wt}, 60*time.Second, "gh", "pr", "create", "--base", base, "--head", head, "--title", title, "--body", body)
	if err != nil {
		return "", err
	}
	return ParsePRURL(out)
}

// ParsePRURL finds the pull request URL in gh's output (it prints the URL last).
func ParsePRURL(out string) (string, error) {
	all := prURLRe.FindAllString(out, -1)
	if len(all) == 0 {
		return "", fmt.Errorf("gh did not print a pull request URL")
	}
	return all[len(all)-1], nil
}
