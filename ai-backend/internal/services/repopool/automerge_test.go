package repopool

import (
	"os/exec"
	"strings"
	"testing"
)

// GAOS_RELEASE_B.md §4: the approved bytes are hashed in Go exactly as git hashes a blob, the
// pull request URL must match whole, and paginated rule answers are joined.

func TestGitBlobSHAMatchesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	content := "import { test } from '@playwright/test';\ntest('TC-API-001 a', async () => {});\n"
	cmd := exec.Command("git", "hash-object", "--stdin", "--no-filters")
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := GitBlobSHA([]byte(content)), strings.TrimSpace(string(out)); got != want {
		t.Errorf("GitBlobSHA = %s, git says %s", got, want)
	}
}

func TestPRRepoNeedsAWholeURL(t *testing.T) {
	o, r, n, err := PRRepo("https://github.com/acme/route-e2e/pull/4")
	if err != nil || o != "acme" || r != "route-e2e" || n != 4 {
		t.Errorf("a pull request URL read as %s/%s#%d (%v)", o, r, n, err)
	}
	for _, bad := range []string{"x https://github.com/acme/route-e2e/pull/4", "https://github.com/acme/route-e2e/pull/4/files",
		"https://github.com/acme/route-e2e/pull/0", "https://example.com/acme/r/pull/4"} {
		if _, _, _, err := PRRepo(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestUnmarshalPagesJoinsPages(t *testing.T) {
	var types []string
	if err := unmarshalPages("[\"update\"]\n[\"deletion\",\"creation\"]\n", &types); err != nil || strings.Join(types, ",") != "update,deletion,creation" {
		t.Errorf("pages joined as %v (%v)", types, err)
	}
	var bad []bool
	if err := unmarshalPages("[true, fal", &bad); err == nil {
		t.Error("a truncated answer was accepted")
	}
}
