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
)

var (
	repoURLRe = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(\.git)?$`)
	relDirRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
	branchRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
)

// InputError marks a problem with caller-supplied input (maps to HTTP 400).
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func inputErr(msg string) error { return &InputError{Msg: msg} }

// ErrBusy is returned when another operation is already running for the same directory.
var ErrBusy = errors.New("another operation is already running for this product")

// ValidateRepoURL accepts only https://github.com/<owner>/<repo>[.git].
func ValidateRepoURL(raw string) error {
	m := repoURLRe.FindStringSubmatch(raw)
	if len(raw) > 300 || m == nil || m[1] == "." || m[1] == ".." || m[2] == "." || m[2] == ".." {
		return inputErr("repo_url must look like https://github.com/<owner>/<repo>")
	}
	return nil
}

// ValidateRelDir accepts a relative path made of safe segments (no "..", no absolute path).
func ValidateRelDir(rel string) error {
	if len(rel) > 200 || !relDirRe.MatchString(rel) {
		return inputErr("root_dir must be a relative path of letters, digits, '.', '_' or '-' (no '..')")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "." || seg == ".." || strings.HasSuffix(seg, ".lock") {
			return inputErr("root_dir contains an invalid segment")
		}
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
// into them. safeDir scopes git's safe.directory to the one checkout being operated on
// (pool checkouts may be owned by another uid, e.g. root in the container vs. the host user).
func childEnv(safeDir string) []string {
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
	if safeDir != "" {
		cfg = append(cfg, fmt.Sprintf("GIT_CONFIG_KEY_%d=safe.directory", n), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", n, safeDir))
		n++
	}
	env = append(env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", n))
	return append(env, cfg...)
}

// run executes a command without a shell and returns trimmed, truncated output.
// Callers must treat the output as internal detail (it may contain git's wording).
func run(ctx context.Context, dir, safeDir string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = childEnv(safeDir)
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
		return s, fmt.Errorf("%s failed: %s", name, s)
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

	if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
		return inputErr("target directory already exists and is not empty")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if _, err = run(ctx, p.Root, abs, 3*time.Minute, "gh", "repo", "clone", repoURL, abs); err != nil {
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

	if _, err := run(ctx, abs, abs, 90*time.Second, "git", "fetch", "--prune", "origin"); err != nil {
		return "", err
	}
	if _, err := run(ctx, abs, abs, 20*time.Second, "git", "checkout", branch, "--"); err != nil {
		return "", err
	}
	if _, err := run(ctx, abs, abs, 20*time.Second, "git", "merge", "--ff-only", "origin/"+branch); err != nil {
		return "", err
	}
	return currentBranch(ctx, abs), nil
}

func currentBranch(ctx context.Context, abs string) string {
	out, err := run(ctx, abs, abs, 10*time.Second, "git", "rev-parse", "--abbrev-ref", "HEAD")
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

// Verify runs every check and never returns an error: failures are part of the report.
func (p *Pool) Verify(ctx context.Context, repoURL, rel, stagingURL string) Report {
	rep := Report{OK: true, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	add := func(name string, ok bool, detail string) {
		rep.Checks = append(rep.Checks, Check{Name: name, OK: ok, Detail: detail})
		if !ok {
			rep.OK = false
		}
	}

	abs, err := p.ResolveDir(rel)
	if err == nil {
		unlock, lerr := p.lock(abs)
		if lerr != nil {
			rep.Busy = true
			return rep
		}
		defer unlock()
	}
	if err != nil {
		var ie *InputError
		if errors.As(err, &ie) {
			add("root_dir", false, ie.Msg)
		} else {
			add("root_dir", false, "pool directory unavailable")
		}
	} else if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		add("root_dir", false, "directory does not exist in the repo pool")
	} else {
		add("root_dir", true, "exists and is readable")
		if !isGitRepo(abs) {
			add("git_repo", false, "no .git directory")
		} else {
			add("git_repo", true, "git repository")
			origin, err := run(ctx, abs, abs, 10*time.Second, "git", "remote", "get-url", "origin")
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
		if ok, where := HasPlaywright(abs); ok {
			add("playwright", true, "found "+where)
		} else {
			add("playwright", false, "no playwright.config.* or playwright dependency in package.json")
		}
	}

	ok, detail := p.checkURL(ctx, stagingURL)
	add("staging_url", ok, detail)
	return rep
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
