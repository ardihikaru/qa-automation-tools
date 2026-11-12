package repopool

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// Real clone + branch switch. Needs gh, git, network and GH_TOKEN; set
// REPOPOOL_IT_REPO (https://github.com/<owner>/<repo>) to enable.
func TestIntegration_CloneCheckoutVerify(t *testing.T) {
	repo := os.Getenv("REPOPOOL_IT_REPO")
	if repo == "" || os.Getenv("GH_TOKEN") == "" {
		t.Skip("set REPOPOOL_IT_REPO and GH_TOKEN to run")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}
	p := New(t.TempDir())
	ctx := context.Background()
	if err := p.Clone(ctx, repo, "it/repo"); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if err := p.Clone(ctx, repo, "it/repo"); err == nil {
		t.Fatal("second clone into a non-empty dir must fail")
	}
	br, err := p.Checkout(ctx, "it/repo", "main")
	if err != nil || br != "main" {
		t.Fatalf("checkout main: %q %v", br, err)
	}
	rep := p.Verify(ctx, repo, "it/repo", "https://example.com")
	for _, c := range rep.Checks {
		if c.Name == "git_repo" || c.Name == "origin_matches" || c.Name == "root_dir" {
			if !c.OK {
				t.Fatalf("%s failed: %s", c.Name, c.Detail)
			}
		}
	}
}
