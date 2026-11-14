package repopool

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// SetOwner makes the pool hand every path it creates to uid:gid ("1000:1000"). The backend
// container runs as root, but playwright-executor (a host user) must write node_modules and
// reports into the clones and worktrees it runs tests in. Empty or invalid = leave ownership alone.
func (p *Pool) SetOwner(spec string) bool {
	if p == nil {
		return false
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return false
	}
	u, g, ok := strings.Cut(spec, ":")
	uid, e1 := strconv.Atoi(u)
	gid, e2 := strconv.Atoi(g)
	if !ok || e1 != nil || e2 != nil || uid < 0 || gid < 0 {
		return false
	}
	p.ownerUID, p.ownerGID, p.ownerSet = uid, gid, true
	return true
}

// FixOwner gives path (and everything below it) to the configured owner. Best effort: a failure
// is ignored because ownership only matters for the executor, and the next call retries. It
// never follows symlinks and never leaves the pool root.
func (p *Pool) FixOwner(path string) {
	if p == nil || !p.ownerSet || path == "" {
		return
	}
	root := filepath.Clean(p.Root)
	path = filepath.Clean(path)
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return
	}
	_ = filepath.WalkDir(path, func(f string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, e := d.Info(); e == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == p.ownerUID && int(st.Gid) == p.ownerGID {
				return nil
			}
		}
		_ = os.Lchown(f, p.ownerUID, p.ownerGID)
		return nil
	})
}
