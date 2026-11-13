// Package repopool manages product repositories checked out under one pool
// directory: validate inputs, clone with `gh`, switch branches with `git`,
// and verify that a checkout is usable by Playwright.
//
// Safety rules (see README.md): every external value is validated against a
// strict pattern, commands run without a shell (argument slices), and every
// path is confined to the pool root (no "..", no symlink escape).
package repopool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

var (
	repoURLRe = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(\.git)?$`)
	relDirRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
	branchRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
	slugRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

const (
	// BranchesSuffix names the directory next to a clone that holds its extra worktrees.
	BranchesSuffix = ".branches"
	// MaxSlugLen caps the worktree directory name derived from a branch.
	MaxSlugLen = 100
	// maxWorktreeRelLen caps root_dir + ".branches/" + slug.
	maxWorktreeRelLen = 310
)

// InputError marks a problem with caller-supplied input (maps to HTTP 400).
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func inputErr(msg string) error { return &InputError{Msg: msg} }

// ErrBusy is returned when another operation is already running for the same directory.
var ErrBusy = errors.New("another operation is already running for this repo")

// ValidateRepoURL accepts only https://github.com/<owner>/<repo>[.git].
func ValidateRepoURL(raw string) error {
	m := repoURLRe.FindStringSubmatch(raw)
	if len(raw) > 300 || m == nil || m[1] == "." || m[1] == ".." || m[2] == "." || m[2] == ".." {
		return inputErr("repo_url must look like https://github.com/<owner>/<repo>")
	}
	return nil
}

// ValidateRelDir accepts a user-supplied relative path made of safe segments (no "..", no
// absolute path). Segments ending in ".branches" (any case) are reserved for worktree areas
// and rejected, so a root_dir can never claim another repo's worktrees.
func ValidateRelDir(rel string) error {
	return validateRelDir(rel, true)
}

func validateRelDir(rel string, banBranches bool) error {
	if len(rel) > 200 || !relDirRe.MatchString(rel) {
		return inputErr("root_dir must be a relative path of letters, digits, '.', '_' or '-' (no '..')")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "." || seg == ".." || strings.HasSuffix(seg, ".lock") {
			return inputErr("root_dir contains an invalid segment")
		}
		if banBranches && strings.HasSuffix(strings.ToLower(seg), BranchesSuffix) {
			return inputErr("root_dir segments must not end in " + BranchesSuffix + " (reserved for branch worktrees)")
		}
	}
	return nil
}

// Slug turns a branch name into one directory segment ("/" becomes "--").
func Slug(branch string) string { return strings.ReplaceAll(branch, "/", "--") }

// ValidateSlug checks a worktree directory name.
func ValidateSlug(slug string) error {
	if len(slug) == 0 || len(slug) > MaxSlugLen || !slugRe.MatchString(slug) || slug == "." || slug == ".." {
		return inputErr("branch name too long for a worktree")
	}
	return nil
}

// ValidateBranch accepts conservative git ref names that cannot be mistaken for flags.
func ValidateBranch(b string) error {
	if !branchRe.MatchString(b) || strings.Contains(b, "..") || strings.Contains(b, "//") ||
		strings.HasSuffix(b, "/") || strings.Contains(b, ".lock") || strings.HasSuffix(b, ".") || b == "HEAD" {
		return inputErr("invalid branch name")
	}
	return nil
}

// NormalizeRepo reduces any GitHub remote form to "owner/repo" (lower case) for comparison.
func NormalizeRepo(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	switch {
	case strings.HasPrefix(s, "git@github.com:"):
		s = strings.TrimPrefix(s, "git@github.com:")
	case strings.HasPrefix(s, "ssh://git@github.com/"):
		s = strings.TrimPrefix(s, "ssh://git@github.com/")
	default:
		if u, err := url.Parse(s); err == nil && strings.EqualFold(u.Host, "github.com") {
			s = strings.TrimPrefix(u.Path, "/")
		}
	}
	return strings.ToLower(s)
}

// safeRemote renders a git remote for display: host and path only, never userinfo or query.
func safeRemote(raw string) string {
	n := NormalizeRepo(raw)
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Host != "" && !strings.EqualFold(u.Host, "github.com") {
		return u.Host + u.Path
	}
	return n
}

// Check is one verification result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Report is the outcome of Verify.
type Report struct {
	// Busy: another clone/checkout holds the directory; nothing was checked and the
	// report must not be stored.
	Busy      bool    `json:"busy,omitempty"`
	OK        bool    `json:"ok"`
	Checks    []Check `json:"checks"`
	Branch    string  `json:"branch,omitempty"`
	CheckedAt string  `json:"checked_at"`
}

// Pool is a directory holding product checkouts.
type Pool struct {
	Root string
	HTTP *http.Client
	// AllowedOwners limits clonable GitHub owners (lower case). Empty = any owner.
	AllowedOwners []string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// newProbeClient returns the HTTP client used for staging probes: no redirects and
// no connections to loopback, link-local (incl. cloud metadata), unspecified or
// multicast addresses, checked on the resolved IP at dial time (defeats DNS rebinding).
func newProbeClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 8 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
				ip.IsUnspecified() || ip.IsMulticast() {
				return errors.New("address not allowed")
			}
			return nil
		},
	}
	return &http.Client{
		Timeout:       10 * time.Second,
		Transport:     &http.Transport{DialContext: dialer.DialContext, Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// OwnerAllowed reports whether repoURL's owner is permitted by AllowedOwners.
func (p *Pool) OwnerAllowed(repoURL string) bool {
	if len(p.AllowedOwners) == 0 {
		return true
	}
	owner := strings.SplitN(NormalizeRepo(repoURL), "/", 2)[0]
	for _, o := range p.AllowedOwners {
		if owner == o {
			return true
		}
	}
	return false
}

// New returns a Pool rooted at root. Returns nil when root is empty (feature disabled).
func New(root string) *Pool {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	return &Pool{
		Root:  root,
		HTTP:  newProbeClient(),
		locks: map[string]*sync.Mutex{},
	}
}

// ResolveDir returns the absolute checkout path for rel and guarantees it stays inside Root,
// even through symlinks in any existing ancestor.
func (p *Pool) ResolveDir(rel string) (string, error) {
	if err := ValidateRelDir(rel); err != nil {
		return "", err
	}
	return p.resolveValidated(rel)
}

// resolveValidated joins an already validated rel under Root and applies the symlink/escape guard.
func (p *Pool) resolveValidated(rel string) (string, error) {
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return "", fmt.Errorf("pool root unavailable: %w", err)
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	// Resolve the deepest existing ancestor and make sure it is inside root.
	probe := abs
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		next := filepath.Dir(probe)
		if next == probe {
			return "", inputErr("path escapes pool")
		}
		probe = next
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", err
	}
	if real != root && !strings.HasPrefix(real, root+string(os.PathSeparator)) {
		return "", inputErr("path escapes pool")
	}
	return abs, nil
}

// ResolveWorktreeDir returns the absolute path of the worktree for slug next to the clone at
// rootDir: <pool>/<rootDir>.branches/<slug>. rootDir was vetted when it was stored, so the
// ".branches" ban is not applied here; the two parts are validated separately, and any existing
// "<rootDir>.branches" or slug directory must be a real directory (not a symlink).
func (p *Pool) ResolveWorktreeDir(rootDir, slug string) (string, error) {
	if err := validateRelDir(rootDir, false); err != nil {
		return "", err
	}
	if err := ValidateSlug(slug); err != nil {
		return "", err
	}
	rel := rootDir + BranchesSuffix + "/" + slug
	if len(rel) > maxWorktreeRelLen {
		return "", inputErr("worktree path too long")
	}
	abs, err := p.resolveValidated(rel)
	if err != nil {
		return "", err
	}
	for _, d := range []string{filepath.Dir(abs), abs} {
		if fi, err := os.Lstat(d); err == nil && !fi.IsDir() {
			// Lstat reports a symlink as non-directory.
			return "", inputErr("worktree path is not a real directory")
		}
	}
	return abs, nil
}

func (p *Pool) lock(key string) (unlock func(), err error) {
	p.mu.Lock()
	l, ok := p.locks[key]
	if !ok {
		l = &sync.Mutex{}
		p.locks[key] = l
	}
	p.mu.Unlock()
	if !l.TryLock() {
		return nil, ErrBusy
	}
	return l.Unlock, nil
}

// childEnv is the minimal environment for gh/git children: no DB or AI secrets leak
// into them. safeDirs scope git's safe.directory to the checkouts being operated on
// (pool checkouts may be owned by another uid, e.g. root in the container vs. the host user).
func childEnv(safeDirs ...string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"GIT_TERMINAL_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		env = append(env, "GH_TOKEN="+t)
	}
	n := 1
	cfg := []string{
		// Authenticate https git operations with GH_TOKEN via gh.
		"GIT_CONFIG_KEY_0=credential.https://github.com.helper", "GIT_CONFIG_VALUE_0=!gh auth git-credential",
	}
	for _, d := range safeDirs {
		if d == "" {
			continue
		}
		cfg = append(cfg, fmt.Sprintf("GIT_CONFIG_KEY_%d=safe.directory", n), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", n, d))
		n++
	}
	env = append(env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", n))
	return append(env, cfg...)
}

// runError is a failed child process; Code is its exit code (-1 when unknown).
type runError struct {
	msg  string
	Code int
}

func (e *runError) Error() string { return e.msg }

// run executes a command without a shell and returns trimmed, truncated output.
// Callers must treat the output as internal detail (it may contain git's wording).
func run(ctx context.Context, dir string, safeDirs []string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = childEnv(safeDirs...)
	// gh spawns git: kill the whole process group on timeout, and never block on a
	// pipe held open by an orphaned child.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if len(s) > 2000 {
		s = s[:2000] + "…"
	}
	if ctx.Err() == context.DeadlineExceeded {
		return s, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	if err != nil {
		if s == "" {
			s = err.Error()
		}
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		return s, &runError{msg: fmt.Sprintf("%s failed: %s", name, s), Code: code}
	}
	return s, nil
}

func isGitRepo(abs string) bool {
	fi, err := os.Lstat(filepath.Join(abs, ".git"))
	return err == nil && fi.Mode()&os.ModeSymlink == 0
}

// readRegular reads a file only when it is a regular file (never follows a symlink out of the checkout).
func readRegular(path string) ([]byte, bool) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return nil, false
	}
	b, err := os.ReadFile(path)
	return b, err == nil
}

func hasPlaywrightDep(pkg []byte) bool {
	var p struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if json.Unmarshal(pkg, &p) != nil {
		return false
	}
	for _, m := range []map[string]json.RawMessage{p.Dependencies, p.DevDependencies} {
		for _, k := range []string{"@playwright/test", "playwright", "playwright-core"} {
			if _, ok := m[k]; ok {
				return true
			}
		}
	}
	return false
}

// Clone runs `gh repo clone` into rel. The target must not exist or must be empty.
func (p *Pool) Clone(ctx context.Context, repoURL, rel string) error {
	if err := ValidateRepoURL(repoURL); err != nil {
		return err
	}
	if !p.OwnerAllowed(repoURL) {
		return inputErr("repo owner is not allowed for this pool")
	}
	abs, err := p.ResolveDir(rel)
	if err != nil {
		return err
	}
	unlock, err := p.lock(abs)
	if err != nil {
		return err
	}
	defer unlock()
	return p.cloneLocked(ctx, repoURL, abs)
}

// cloneLocked is Clone without validation or locking: the caller holds the lock for abs.
func (p *Pool) cloneLocked(ctx context.Context, repoURL, abs string) error {
	if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
		return inputErr("target directory already exists and is not empty")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if _, err := run(ctx, p.Root, []string{abs}, 3*time.Minute, "gh", "repo", "clone", repoURL, abs); err != nil {
		// Do not leave a partial checkout behind: the target was absent or empty before.
		_ = os.RemoveAll(abs)
		return err
	}
	return nil
}

// Checkout fetches and switches the checkout at rel to branch, fast-forwarding it.
func (p *Pool) Checkout(ctx context.Context, rel, branch string) (string, error) {
	if err := ValidateBranch(branch); err != nil {
		return "", err
	}
	abs, err := p.ResolveDir(rel)
	if err != nil {
		return "", err
	}
	if !isGitRepo(abs) {
		return "", inputErr("root_dir is not a git repository; clone it first")
	}
	unlock, err := p.lock(abs)
	if err != nil {
		return "", err
	}
	defer unlock()
	return p.checkoutLocked(ctx, abs, branch)
}

func (p *Pool) checkoutLocked(ctx context.Context, abs, branch string) (string, error) {
	sd := []string{abs}
	if _, err := run(ctx, abs, sd, 90*time.Second, "git", "fetch", "--prune", "origin"); err != nil {
		return "", err
	}
	if _, err := run(ctx, abs, sd, 20*time.Second, "git", "checkout", branch, "--"); err != nil {
		return "", err
	}
	if _, err := run(ctx, abs, sd, 20*time.Second, "git", "merge", "--ff-only", "origin/"+branch); err != nil {
		return "", err
	}
	return currentBranch(ctx, abs), nil
}

func currentBranch(ctx context.Context, abs string) string {
	out, err := run(ctx, abs, []string{abs}, 10*time.Second, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// HasPlaywright reports whether abs (or an immediate subdirectory) has a Playwright
// config file or a package.json that depends on playwright.
func HasPlaywright(abs string) (bool, string) {
	dirs := []string{abs}
	if entries, err := os.ReadDir(abs); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != "node_modules" && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, filepath.Join(abs, e.Name()))
			}
		}
	}
	for _, d := range dirs {
		for _, ext := range []string{"ts", "js", "mjs", "cjs"} {
			f := filepath.Join(d, "playwright.config."+ext)
			if fi, err := os.Lstat(f); err == nil && fi.Mode().IsRegular() {
				rel, _ := filepath.Rel(abs, f)
				return true, rel
			}
		}
		if b, ok := readRegular(filepath.Join(d, "package.json")); ok {
			if hasPlaywrightDep(b) {
				rel, _ := filepath.Rel(abs, filepath.Join(d, "package.json"))
				return true, rel
			}
		}
	}
	return false, ""
}

// Append adds a check to the report and clears OK when it failed.
func (r *Report) Append(c Check) {
	r.Checks = append(r.Checks, c)
	if !c.OK {
		r.OK = false
	}
}

// Verify runs every check, including the staging probe, and never returns an error: failures
// are part of the report. It is VerifyRepo(requirePlaywright=true) plus ProbeStaging.
func (p *Pool) Verify(ctx context.Context, repoURL, rel, stagingURL string) Report {
	rep := p.VerifyRepo(ctx, repoURL, rel, true)
	if rep.Busy {
		return rep
	}
	rep.Append(p.ProbeStaging(ctx, stagingURL))
	return rep
}

// VerifyRepo runs the checkout checks (root_dir, git_repo, origin_matches and, when
// requirePlaywright, playwright). It never returns an error: failures are part of the report.
func (p *Pool) VerifyRepo(ctx context.Context, repoURL, rel string, requirePlaywright bool) Report {
	rep := Report{OK: true, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	abs, err := p.ResolveDir(rel)
	if err != nil {
		var ie *InputError
		if errors.As(err, &ie) {
			rep.Append(Check{Name: "root_dir", Detail: ie.Msg})
		} else {
			rep.Append(Check{Name: "root_dir", Detail: "pool directory unavailable"})
		}
		return rep
	}
	unlock, lerr := p.lock(abs)
	if lerr != nil {
		rep.Busy = true
		return rep
	}
	defer unlock()
	p.verifyRepoLocked(ctx, &rep, repoURL, abs, requirePlaywright)
	return rep
}

// verifyRepoLocked fills rep for the checkout at abs; the caller holds the lock for abs.
func (p *Pool) verifyRepoLocked(ctx context.Context, rep *Report, repoURL, abs string, requirePlaywright bool) {
	add := func(name string, ok bool, detail string) { rep.Append(Check{Name: name, OK: ok, Detail: detail}) }
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		add("root_dir", false, "directory does not exist in the repo pool")
		return
	}
	add("root_dir", true, "exists and is readable")
	if !isGitRepo(abs) {
		add("git_repo", false, "no .git directory")
	} else {
		add("git_repo", true, "git repository")
		origin, err := run(ctx, abs, []string{abs}, 10*time.Second, "git", "remote", "get-url", "origin")
		switch {
		case err != nil:
			add("origin_matches", false, "cannot read origin remote")
		case NormalizeRepo(origin) != NormalizeRepo(repoURL):
			add("origin_matches", false, "origin is "+safeRemote(origin)+", expected "+NormalizeRepo(repoURL))
		default:
			add("origin_matches", true, "origin matches repo link")
		}
		rep.Branch = currentBranch(ctx, abs)
	}
	if requirePlaywright {
		if ok, where := HasPlaywright(abs); ok {
			add("playwright", true, "found "+where)
		} else {
			add("playwright", false, "no playwright.config.* or playwright dependency in package.json")
		}
	}
}

// ProbeStaging checks that the staging URL answers (one GET, status below 500).
func (p *Pool) ProbeStaging(ctx context.Context, stagingURL string) Check {
	ok, detail := p.checkURL(ctx, stagingURL)
	return Check{Name: "staging_url", OK: ok, Detail: detail}
}

// checkURL does one GET and treats any status below 500 as reachable
// (a login redirect or 401/403 still proves the server answers).
func (p *Pool) checkURL(ctx context.Context, raw string) (bool, string) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return false, "not a valid http(s) URL (credentials in the URL are not allowed)"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, "not a valid http(s) URL"
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return false, coarseErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return true, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// coarseErr hides resolver/socket detail so verify cannot be used as a network scanner.
func coarseErr(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return "unreachable: timeout"
	case strings.Contains(err.Error(), "address not allowed"):
		return "unreachable: address not allowed"
	default:
		return "unreachable"
	}
}

// pushDisabledURL is a relative path that is not a repository, so `git push origin` fails.
const pushDisabledURL = "DISABLED"

// SetPushPolicy enables or disables pushing through the clone's origin remote (best effort
// guard, see README). A directory that is not a clone yet is a no-op: the next sync applies it.
func (p *Pool) SetPushPolicy(ctx context.Context, rel string, allowPush bool) error {
	abs, err := p.ResolveDir(rel)
	if err != nil {
		return err
	}
	if !isGitRepo(abs) {
		return nil
	}
	unlock, err := p.lock(abs)
	if err != nil {
		return err
	}
	defer unlock()
	return p.setPushPolicyLocked(ctx, abs, allowPush)
}

func (p *Pool) setPushPolicyLocked(ctx context.Context, abs string, allowPush bool) error {
	sd := []string{abs}
	if !allowPush {
		_, err := run(ctx, abs, sd, 10*time.Second, "git", "config", "remote.origin.pushurl", pushDisabledURL)
		return err
	}
	_, err := run(ctx, abs, sd, 10*time.Second, "git", "config", "--unset", "remote.origin.pushurl")
	var re *runError
	if errors.As(err, &re) && re.Code == 5 { // 5: the key was not set
		return nil
	}
	return err
}

// BranchSpec is one branch to keep checked out.
type BranchSpec struct {
	Name    string
	Default bool
}

func (p *Pool) resultMsg(err error) string {
	m := err.Error()
	if i := strings.IndexByte(m, '\n'); i >= 0 {
		m = m[:i]
	}
	// Never expose absolute pool paths.
	m = strings.ReplaceAll(m, p.Root, "<pool>")
	if real, err := filepath.EvalSymlinks(p.Root); err == nil {
		m = strings.ReplaceAll(m, real, "<pool>")
	}
	if len(m) > 200 {
		m = m[:200] + "…"
	}
	return m
}

func validateBranchSpecs(branches []BranchSpec) (def string, err error) {
	seenName := map[string]bool{}
	seenSlug := map[string]string{}
	for _, b := range branches {
		if err := ValidateBranch(b.Name); err != nil {
			return "", err
		}
		slug := Slug(b.Name)
		if err := ValidateSlug(slug); err != nil {
			return "", err
		}
		if seenName[b.Name] {
			return "", inputErr("duplicate branch " + b.Name)
		}
		if other, ok := seenSlug[slug]; ok {
			return "", inputErr("branches " + other + " and " + b.Name + " map to the same worktree directory")
		}
		seenName[b.Name] = true
		seenSlug[slug] = b.Name
		if b.Default {
			if def != "" {
				return "", inputErr("only one default branch is allowed")
			}
			def = b.Name
		}
	}
	if len(branches) > 0 && def == "" {
		return "", inputErr("a default branch is required")
	}
	return def, nil
}

// porcelainWorktrees parses `git worktree list --porcelain` into path -> branch ("" when detached).
func porcelainWorktrees(out string) map[string]string {
	m := map[string]string{}
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur = strings.TrimPrefix(line, "worktree ")
			m[cur] = ""
		case strings.HasPrefix(line, "branch ") && cur != "":
			m[cur] = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	return m
}

func realPath(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// SyncBranches brings the checkout at rel in line with branches: the default branch in the
// clone, every other listed branch in <rel>.branches/<slug>, and removes worktrees of
// unlisted branches. It never resets, cleans or forces: dirty trees are kept
// ("kept_dirty") and diverged branches are reported ("error"). The returned error is only for
// failures that stop the whole sync (invalid input, ErrBusy, clone or fetch failure); per-branch
// problems are results. allowPush controls the origin push URL (see SetPushPolicy).
// An empty branches list (legacy) stops after the fetch.
func (p *Pool) SyncBranches(ctx context.Context, repoURL, rel string, branches []BranchSpec, allowPush bool) ([]models.BranchResult, error) {
	if err := ValidateRepoURL(repoURL); err != nil {
		return nil, err
	}
	if !p.OwnerAllowed(repoURL) {
		return nil, inputErr("repo owner is not allowed for this pool")
	}
	def, err := validateBranchSpecs(branches)
	if err != nil {
		return nil, err
	}
	abs, err := p.ResolveDir(rel)
	if err != nil {
		return nil, err
	}
	unlock, err := p.lock(abs)
	if err != nil {
		return nil, err
	}
	defer unlock()

	// 1-2. Clone when absent, then the push policy.
	if !isGitRepo(abs) {
		if err := p.cloneLocked(ctx, repoURL, abs); err != nil {
			return nil, err
		}
	}
	if err := p.setPushPolicyLocked(ctx, abs, allowPush); err != nil {
		return nil, err
	}
	sd := []string{abs}
	// 3-4. Prune stale worktree records, then fetch with an explicit refspec.
	_, _ = run(ctx, abs, sd, 20*time.Second, "git", "worktree", "prune")
	if _, err := run(ctx, abs, sd, 90*time.Second, "git", "fetch", "--prune", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return nil, err
	}
	// 5. Legacy empty list: nothing to check out.
	if len(branches) == 0 {
		return []models.BranchResult{}, nil
	}

	results := []models.BranchResult{}
	add := func(name, status, msg string) {
		results = append(results, models.BranchResult{Name: name, Status: status, Message: msg})
	}
	wanted := map[string]bool{}
	for _, b := range branches {
		wanted[b.Name] = true
	}
	remoteHas := func(b string) bool {
		_, err := run(ctx, abs, sd, 10*time.Second, "git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+b)
		return err == nil
	}
	localHas := func(b string) bool {
		_, err := run(ctx, abs, sd, 10*time.Second, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+b)
		return err == nil
	}
	isDirty := func(dir string) (bool, error) {
		out, err := run(ctx, dir, []string{abs, dir}, 20*time.Second, "git", "status", "--porcelain")
		return out != "", err
	}

	// 6. Existing worktrees.
	listOut, err := run(ctx, abs, sd, 10*time.Second, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	worktrees := porcelainWorktrees(listOut)
	realWT := map[string]string{} // real path -> branch
	for path, br := range worktrees {
		realWT[realPath(path)] = br
	}
	base := realPath(filepath.Dir(abs)) + string(os.PathSeparator) + filepath.Base(abs) + BranchesSuffix + string(os.PathSeparator)

	// 7. Removals: unlisted branches and the (new) default branch.
	heldDirty := map[string]bool{}
	for path, br := range realWT {
		if !strings.HasPrefix(path, base) {
			continue
		}
		if br != "" && wanted[br] && br != def {
			continue
		}
		name := br
		if name == "" {
			name = filepath.Base(path)
		}
		dirty, derr := isDirty(path)
		switch {
		case derr != nil:
			add(name, "error", p.resultMsg(derr))
			heldDirty[br] = true
		case dirty:
			add(name, "kept_dirty", "worktree has local changes; kept")
			heldDirty[br] = true
		default:
			if _, rerr := run(ctx, abs, []string{abs, path}, 30*time.Second, "git", "worktree", "remove", path); rerr != nil {
				add(name, "error", p.resultMsg(rerr))
				heldDirty[br] = true
			} else {
				add(name, "ok", "worktree removed")
			}
		}
	}

	// 8. Default branch in the clone.
	switch {
	case !remoteHas(def):
		add(def, "missing_remote", "branch not found on origin")
	case heldDirty[def]:
		add(def, "kept_dirty", "a worktree with local changes still holds this branch; default not switched")
	default:
		status, msg := "ok", "up to date"
		if cur := currentBranch(ctx, abs); cur != def {
			dirty, derr := isDirty(abs)
			switch {
			case derr != nil:
				status, msg = "error", p.resultMsg(derr)
			case dirty:
				status, msg = "kept_dirty", "clone has local changes; default branch not switched"
			default:
				var cerr error
				if localHas(def) {
					_, cerr = run(ctx, abs, sd, 20*time.Second, "git", "checkout", def, "--")
				} else {
					_, cerr = run(ctx, abs, sd, 20*time.Second, "git", "checkout", "--track", "-b", def, "origin/"+def)
				}
				if cerr != nil {
					status, msg = "error", p.resultMsg(cerr)
				}
			}
		}
		if status == "ok" {
			if _, merr := run(ctx, abs, sd, 20*time.Second, "git", "merge", "--ff-only", "origin/"+def); merr != nil {
				status, msg = "error", p.resultMsg(merr)
			}
		}
		add(def, status, msg)
	}

	// 9. Other listed branches, one worktree each.
	commonDir := realPath(filepath.Join(abs, ".git"))
	for _, b := range branches {
		if b.Default {
			continue
		}
		if !remoteHas(b.Name) {
			add(b.Name, "missing_remote", "branch not found on origin")
			continue
		}
		wt, werr := p.ResolveWorktreeDir(rel, Slug(b.Name))
		if werr != nil {
			add(b.Name, "error", p.resultMsg(werr))
			continue
		}
		wsd := []string{abs, wt}
		if _, serr := os.Lstat(wt); serr == nil {
			// Verify before reuse: same object store and registered on exactly this branch.
			out, cerr := run(ctx, wt, wsd, 10*time.Second, "git", "rev-parse", "--git-common-dir")
			if cerr == nil && !filepath.IsAbs(out) {
				out = filepath.Join(wt, out)
			}
			if cerr != nil || realPath(out) != commonDir || realWT[realPath(wt)] != b.Name {
				add(b.Name, "error", "path exists but is not the expected worktree")
				continue
			}
		} else {
			var aerr error
			if localHas(b.Name) {
				_, aerr = run(ctx, abs, wsd, 60*time.Second, "git", "worktree", "add", wt, b.Name)
			} else {
				_, aerr = run(ctx, abs, wsd, 60*time.Second, "git", "worktree", "add", "--track", "-b", b.Name, wt, "origin/"+b.Name)
			}
			if aerr != nil {
				add(b.Name, "error", p.resultMsg(aerr))
				continue
			}
		}
		if _, merr := run(ctx, wt, wsd, 20*time.Second, "git", "merge", "--ff-only", "origin/"+b.Name); merr != nil {
			add(b.Name, "error", p.resultMsg(merr))
			continue
		}
		add(b.Name, "ok", "up to date")
	}
	return results, nil
}
