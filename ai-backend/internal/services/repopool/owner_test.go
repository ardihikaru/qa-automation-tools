package repopool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetOwnerParsing(t *testing.T) {
	p := New(t.TempDir())
	for _, bad := range []string{"", "1000", "a:b", "-1:5", "1:2:3x"} {
		if p.SetOwner(bad) {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if !p.SetOwner("1000:1000") || !p.ownerSet {
		t.Fatal("1000:1000 must be accepted")
	}
}

// FixOwner must never walk outside the pool root, even when given such a path.
func TestFixOwnerStaysInRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(root)
	p.SetOwner("0:0")
	p.FixOwner(outside) // refused: not under root
	p.FixOwner(root)    // allowed; chown to our own uid/gid is a no-op or EPERM, both ignored
	var nilPool *Pool
	nilPool.FixOwner(root) // nil-safe
}
