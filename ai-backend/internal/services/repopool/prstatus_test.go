package repopool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGh puts a `gh` on PATH that prints out and records its arguments in <dir>/args.
func fakeGh(t *testing.T, out string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" >> \"" + filepath.Join(dir, "args") + "\"\ncat <<'EOF'\n" + out + "\nEOF\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestPRStatus(t *testing.T) {
	p, root := automationPool(t)
	_ = root
	ctx := context.Background()
	url := "https://github.com/o/r/pull/12"

	dir := fakeGh(t, `{"state":"MERGED","mergedAt":"2026-11-21T10:00:00Z","mergeCommit":{"oid":"0123456789abcdef0123456789abcdef01234567"}}`)
	st, err := p.PRStatus(ctx, "auto", url)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "MERGED" || st.MergeCommit != "0123456789abcdef0123456789abcdef01234567" || st.MergedAt != "2026-11-21T10:00:00Z" {
		t.Fatalf("%+v", st)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.Contains(string(args), "pr view "+url+" --json state,mergedAt,mergeCommit") {
		t.Fatalf("args %q", args)
	}

	fakeGh(t, `{"state":"OPEN","mergedAt":null,"mergeCommit":null}`)
	if st, err := p.PRStatus(ctx, "auto", url); err != nil || st.State != "OPEN" || st.MergeCommit != "" {
		t.Fatalf("%+v %v", st, err)
	}
	fakeGh(t, `{"state":"WEIRD"}`)
	if _, err := p.PRStatus(ctx, "auto", url); err == nil {
		t.Fatal("unknown state accepted")
	}
	for _, bad := range []string{"https://github.com/o/r/pull/12; rm -rf /", "--repo=x", "https://evil.example/o/r/pull/1"} {
		if _, err := p.PRStatus(ctx, "auto", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLearnedPathIsTheOnlyDotPath(t *testing.T) {
	if err := validRepoRel([]string{".claude/rules/learned-vendor-url.md"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{".claude/rules/other.md", ".git/config", ".claude/rules/learned-../x.md", ".claude/rules/learned-A.md"} {
		if err := validRepoRel([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLabelPRRefusals(t *testing.T) {
	p, _ := automationPool(t)
	ctx := context.Background()
	if err := p.LabelPR(ctx, "/tmp", "https://github.com/o/r/pull/1", "ai-assisted"); err == nil {
		t.Error("non-worktree accepted")
	}
	if err := p.LabelPR(ctx, "/tmp", "https://github.com/o/r/pull/1", "--x"); err == nil {
		t.Error("bad label accepted")
	}
}
