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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	repoURLRe = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(\.git)?$`)
	relDirRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
	branchRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
)

// ErrBusy is returned when another operation is already running for the same directory.
var ErrBusy = errors.New("another operation is already running for this product")

// ValidateRepoURL accepts only https://github.com/<owner>/<repo>[.git].
func ValidateRepoURL(raw string) error {
	if len(raw) > 300 || !repoURLRe.MatchString(raw) {
		return errors.New("repo_url must look like https://github.com/<owner>/<repo>")
	}
	return nil
}

// ValidateRelDir accepts a relative path made of safe segments (no "..", no absolute path).
func ValidateRelDir(rel string) error {
	if len(rel) > 200 || !relDirRe.MatchString(rel) {
		return errors.New("root_dir must be a relative path of letters, digits, '.', '_' or '-' (no '..')")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "." || seg == ".." || strings.HasSuffix(seg, ".lock") {
			return errors.New("root_dir contains an invalid segment")
		}
	}
	return nil
}

// ValidateBranch accepts conservative git ref names that cannot be mistaken for flags.
func ValidateBranch(b string) error {
	if !branchRe.MatchString(b) || strings.Contains(b, "..") || strings.Contains(b, "//") ||
		strings.HasSuffix(b, "/") || strings.HasSuffix(b, ".lock") || strings.HasSuffix(b, ".") {
		return errors.New("invalid branch name")
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

// Check is one verification result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Report is the outcome of Verify.
type Report struct {
	OK        bool    `json:"ok"`
	Checks    []Check `json:"checks"`
	Branch    string  `json:"branch,omitempty"`
	CheckedAt string  `json:"checked_at"`
}

// Pool is a directory holding product checkouts.
type Pool struct {
	Root string
	HTTP *http.Client

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// New returns a Pool rooted at root. Returns nil when root is empty (feature disabled).
func New(root string) *Pool {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	return &Pool{
		Root:  root,
		HTTP:  &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
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
			return "", errors.New("path escapes pool")
		}
		probe = next
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", err
	}
	if real != root && !strings.HasPrefix(real, root+string(os.PathSeparator)) {
		return "", errors.New("path escapes pool")
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

// run executes a command without a shell and returns trimmed, truncated output.
func run(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
		// Pool checkouts may be owned by another uid (host vs container).
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*",
		// Authenticate https git operations with GH_TOKEN via gh.
		"GIT_CONFIG_KEY_1=credential.https://github.com.helper", "GIT_CONFIG_VALUE_1=!gh auth git-credential",
	)
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
	_, err := os.Stat(filepath.Join(abs, ".git"))
	return err == nil
}

// Clone runs `gh repo clone` into rel. The target must not exist or must be empty.
func (p *Pool) Clone(ctx context.Context, repoURL, rel string) error {
	if err := ValidateRepoURL(repoURL); err != nil {
		return err
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
		return errors.New("target directory already exists and is not empty")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	_, err = run(ctx, p.Root, 5*time.Minute, "gh", "repo", "clone", repoURL, abs)
	return err
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
		return "", errors.New("root_dir is not a git repository; clone it first")
	}
	unlock, err := p.lock(abs)
	if err != nil {
		return "", err
	}
	defer unlock()

	if _, err := run(ctx, abs, 2*time.Minute, "git", "fetch", "--prune", "origin"); err != nil {
		return "", err
	}
	if _, err := run(ctx, abs, 30*time.Second, "git", "checkout", branch); err != nil {
		return "", err
	}
	if _, err := run(ctx, abs, 30*time.Second, "git", "merge", "--ff-only", "origin/"+branch); err != nil {
		return "", err
	}
	return currentBranch(ctx, abs), nil
}

func currentBranch(ctx context.Context, abs string) string {
	out, err := run(ctx, abs, 10*time.Second, "git", "rev-parse", "--abbrev-ref", "HEAD")
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
			if _, err := os.Stat(f); err == nil {
				rel, _ := filepath.Rel(abs, f)
				return true, rel
			}
		}
		if b, err := os.ReadFile(filepath.Join(d, "package.json")); err == nil {
			s := string(b)
			if strings.Contains(s, `"@playwright/test"`) || strings.Contains(s, `"playwright"`) {
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
	if err != nil {
		add("root_dir", false, err.Error())
	} else if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		add("root_dir", false, "directory does not exist in the repo pool")
	} else {
		add("root_dir", true, "exists and is readable")
		if !isGitRepo(abs) {
			add("git_repo", false, "no .git directory")
		} else {
			add("git_repo", true, "git repository")
			origin, err := run(ctx, abs, 10*time.Second, "git", "remote", "get-url", "origin")
			switch {
			case err != nil:
				add("origin_matches", false, "cannot read origin remote")
			case NormalizeRepo(origin) != NormalizeRepo(repoURL):
				add("origin_matches", false, "origin is "+NormalizeRepo(origin)+", expected "+NormalizeRepo(repoURL))
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
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false, "not a valid http(s) URL"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, "not a valid http(s) URL"
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return false, "unreachable: " + shortErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return true, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	s := err.Error()
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}
