package repopool

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// MirrorsDir holds the bare mirrors owned by the knowledge sync (PRODUCT_KNOWLEDGE_SYNC.md,
// "Reading the repo"). relDirRe forbids a leading "." in any segment, so no product root_dir
// can claim it.
const MirrorsDir = ".kb-mirrors"

var (
	shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)
	// mirrorGitCfg disables everything in git that could run code or rewrite objects.
	mirrorGitCfg = []string{"-c", "core.hooksPath=/dev/null", "-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "fetch.writeFetchHEAD=false"}
)

// mirrorRemote turns a validated repo URL into the fetch URL. Tests replace it with a local path.
var mirrorRemote = func(repoURL string) string {
	return "https://github.com/" + NormalizeRepo(repoURL) + ".git"
}

// ReadTree fetches branch of repoURL into the sync-owned bare mirror <pool>/.kb-mirrors/<repoID>.git
// and returns the commit SHA and the raw bytes of every regular file for which keep(path) is true
// (paths are relative to the repo root; keep nil keeps everything). It never touches the product
// clone, its worktrees or its remote.
func (p *Pool) ReadTree(ctx context.Context, repoID int, repoURL, branch string, keep func(path string) bool, maxBytes int64) (string, map[string][]byte, error) {
	if repoID <= 0 {
		return "", nil, inputErr("invalid repo id")
	}
	if err := ValidateRepoURL(repoURL); err != nil {
		return "", nil, err
	}
	if !p.OwnerAllowed(repoURL) {
		return "", nil, inputErr("repo owner is not allowed in this pool")
	}
	if err := ValidateBranch(branch); err != nil {
		return "", nil, err
	}
	abs, err := p.resolveValidated(MirrorsDir + "/" + strconv.Itoa(repoID) + ".git")
	if err != nil {
		return "", nil, err
	}
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return "", nil, fmt.Errorf("pool root unavailable: %w", err)
	}
	unlock, err := p.lock(abs)
	if err != nil {
		return "", nil, err
	}
	defer unlock()

	env := []string{"GIT_CEILING_DIRECTORIES=" + root}
	sd := []string{abs}
	git := func(timeout time.Duration, stdin io.Reader, limit int64, args ...string) ([]byte, error) {
		full := append([]string{"--git-dir=" + abs}, mirrorGitCfg...)
		return runBytes(ctx, abs, sd, env, stdin, timeout, limit, "git", append(full, args...)...)
	}

	// 3. (Re)initialise a missing or broken mirror.
	if fi, err := os.Stat(filepath.Join(abs, "HEAD")); err != nil || !fi.Mode().IsRegular() {
		if err := os.RemoveAll(abs); err != nil {
			return "", nil, fmt.Errorf("reset mirror: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return "", nil, fmt.Errorf("create mirrors dir: %w", err)
		}
		if _, err := runBytes(ctx, filepath.Dir(abs), sd, env, nil, 20*time.Second, 1<<16, "git", "init", "--bare", "-q", abs); err != nil {
			return "", nil, err
		}
	}
	// 4. Fetch the one branch, shallow, from the URL stored in the DB.
	ref := "refs/heads/" + branch
	if _, err := git(120*time.Second, nil, 1<<16, "fetch", "--no-tags", "--depth=1", "--quiet", mirrorRemote(repoURL), "+"+ref+":"+ref); err != nil {
		return "", nil, err
	}
	// 5. Resolve the commit.
	out, err := git(10*time.Second, nil, 1<<10, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", nil, err
	}
	sha := strings.TrimSpace(string(out))
	if !shaRe.MatchString(sha) {
		return "", nil, fmt.Errorf("unexpected commit id from git")
	}
	// 6. List the blobs, keep the wanted ones.
	out, err = git(30*time.Second, nil, 64<<20, "ls-tree", "-r", "-z", "--full-tree", sha)
	if err != nil {
		return "", nil, err
	}
	type entry struct{ oid, path string }
	var entries []entry
	for _, rec := range bytes.Split(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		meta, path, ok := bytes.Cut(rec, []byte{'\t'})
		if !ok {
			continue
		}
		f := strings.Fields(string(meta)) // <mode> <type> <oid>
		if len(f) != 3 || f[1] != "blob" || (f[0] != "100644" && f[0] != "100755") {
			continue
		}
		if keep != nil && !keep(string(path)) {
			continue
		}
		entries = append(entries, entry{oid: f[2], path: string(path)})
	}
	files := map[string][]byte{}
	if len(entries) > 0 {
		// 7. Raw object bytes via cat-file --batch: no filters, no attributes.
		var in bytes.Buffer
		for _, e := range entries {
			in.WriteString(e.oid + "\n")
		}
		raw, err := git(60*time.Second, &in, maxBytes+int64(len(entries))*128, "cat-file", "--batch")
		if err != nil {
			return "", nil, err
		}
		r := bufio.NewReader(bytes.NewReader(raw))
		var total int64
		for _, e := range entries {
			header, err := r.ReadString('\n')
			if err != nil {
				return "", nil, fmt.Errorf("read object header: %w", err)
			}
			h := strings.Fields(header) // <oid> blob <size>
			if len(h) != 3 || h[0] != e.oid || h[1] != "blob" {
				return "", nil, fmt.Errorf("unexpected object header for %s", e.path)
			}
			size, err := strconv.ParseInt(h[2], 10, 64)
			if err != nil || size < 0 {
				return "", nil, fmt.Errorf("bad object size for %s", e.path)
			}
			total += size
			if total > maxBytes {
				return "", nil, fmt.Errorf("the selected files are larger than %d bytes", maxBytes)
			}
			buf := make([]byte, size)
			if _, err := io.ReadFull(r, buf); err != nil {
				return "", nil, fmt.Errorf("read object %s: %w", e.path, err)
			}
			if _, err := r.Discard(1); err != nil {
				return "", nil, fmt.Errorf("read object %s: %w", e.path, err)
			}
			files[e.path] = buf
		}
	}
	// 8. Housekeeping: shallow fetches pile up packs while gc.auto is off.
	if packs, _ := filepath.Glob(filepath.Join(abs, "objects", "pack", "*.pack")); len(packs) > 20 {
		if _, err := git(60*time.Second, nil, 1<<16, "gc", "--prune=now", "--quiet"); err != nil {
			fmt.Fprintf(os.Stderr, "repopool: mirror gc %s: %v\n", abs, err)
		}
	}
	return sha, files, nil
}

// runBytes runs a command like run, but returns stdout as raw bytes (capped at maxBytes) and keeps
// stderr only for the error text. extraEnv is appended to childEnv.
func runBytes(ctx context.Context, dir string, safeDirs, extraEnv []string, stdin io.Reader, timeout time.Duration, maxBytes int64, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(childEnv(safeDirs...), extraEnv...)
	cmd.Stdin = stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	stdout := &capBuffer{max: maxBytes}
	var stderr capBuffer
	stderr.max = 2000
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	if stdout.over {
		return nil, fmt.Errorf("%s output exceeds %d bytes", name, maxBytes)
	}
	if err != nil {
		msg := strings.TrimSpace(safeErrText(stderr.String()))
		if msg == "" {
			msg = err.Error()
		}
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		return nil, &runError{msg: fmt.Sprintf("%s failed: %s", name, msg), Code: code}
	}
	return stdout.Bytes(), nil
}

// credURLRe matches userinfo in URLs so it never reaches an error message.
var credURLRe = regexp.MustCompile(`(https?://)[^/@\s]+@`)

func safeErrText(s string) string { return credURLRe.ReplaceAllString(s, "$1") }

// capBuffer keeps at most max bytes and remembers whether more were written.
type capBuffer struct {
	bytes.Buffer
	max  int64
	over bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	room := c.max - int64(c.Len())
	if int64(len(p)) > room {
		c.over = true
		if room > 0 {
			c.Buffer.Write(p[:room])
		}
		return len(p), nil
	}
	return c.Buffer.Write(p)
}
