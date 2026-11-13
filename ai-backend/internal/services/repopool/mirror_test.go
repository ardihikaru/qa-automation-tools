package repopool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestReadTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := t.TempDir()
	gitT(t, src, "init", "-q", "-b", "main")
	write := func(rel, body string) {
		p := filepath.Join(src, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/a.md", "# A\n")
	write("docs/sub/b.md", "bé\r\n")
	write("other/c.md", "no")
	// A smudge filter in the attributes must not run: raw objects are read.
	write(".gitattributes", "*.md filter=evil\n")
	gitT(t, src, "add", "-A")
	gitT(t, src, "commit", "-q", "-m", "one")
	sha1 := gitT(t, src, "rev-parse", "HEAD")

	old := mirrorRemote
	mirrorRemote = func(string) string { return src }
	t.Cleanup(func() { mirrorRemote = old })

	poolDir := t.TempDir()
	p := New(poolDir)
	ctx := context.Background()
	sha, files, err := p.ReadTree(ctx, 7, "https://github.com/acme/catalog", "main", "docs", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if sha != sha1 {
		t.Errorf("sha %s, want %s", sha, sha1)
	}
	if len(files) != 2 || string(files["docs/a.md"]) != "# A\n" || string(files["docs/sub/b.md"]) != "bé\r\n" {
		t.Errorf("files = %q", files)
	}
	if _, err := os.Stat(filepath.Join(poolDir, MirrorsDir, "7.git", "HEAD")); err != nil {
		t.Errorf("mirror missing: %v", err)
	}
	if ents, _ := os.ReadDir(poolDir); len(ents) != 1 {
		t.Errorf("pool must only gain %s, has %v", MirrorsDir, ents)
	}

	// Second commit: the mirror moves forward.
	write("docs/a.md", "# A2\n")
	gitT(t, src, "commit", "-q", "-am", "two")
	sha2, files, err := p.ReadTree(ctx, 7, "https://github.com/acme/catalog", "main", "docs", 1<<20)
	if err != nil || sha2 == sha1 || string(files["docs/a.md"]) != "# A2\n" {
		t.Fatalf("second read: %v %s %q", err, sha2, files["docs/a.md"])
	}

	// A broken mirror (HEAD removed) is re-initialised, never resolved upward.
	_ = os.Remove(filepath.Join(poolDir, MirrorsDir, "7.git", "HEAD"))
	if sha3, _, err := p.ReadTree(ctx, 7, "https://github.com/acme/catalog", "main", "docs", 1<<20); err != nil || sha3 != sha2 {
		t.Fatalf("re-init: %v %s", err, sha3)
	}

	// Size cap.
	if _, _, err := p.ReadTree(ctx, 7, "https://github.com/acme/catalog", "main", "docs", 3); err == nil {
		t.Error("want size cap error")
	}

	// Input validation.
	for _, tc := range []struct{ url, branch, prefix string }{
		{"https://evil.example/x/y", "main", "docs"},
		{"https://github.com/acme/catalog", "-x", "docs"},
		{"https://github.com/acme/catalog", "main", "../docs"},
		{"https://github.com/acme/catalog", "main", "/docs"},
	} {
		if _, _, err := p.ReadTree(ctx, 7, tc.url, tc.branch, tc.prefix, 1<<20); err == nil {
			t.Errorf("%+v: want error", tc)
		}
	}
	p.AllowedOwners = []string{"other"}
	if _, _, err := p.ReadTree(ctx, 7, "https://github.com/acme/catalog", "main", "docs", 1<<20); err == nil {
		t.Error("owner not allowed: want error")
	}
}

func TestSafeErrText(t *testing.T) {
	if got := safeErrText("fatal: https://x-access-token:abc@github.com/o/r"); strings.Contains(got, "abc") {
		t.Errorf("leaked: %s", got)
	}
}
