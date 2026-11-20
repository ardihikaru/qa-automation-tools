package repopool

// Pull request status and labels for the learning loop (LEARNING_LOOP.md decisions 9 and 10).

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// PRState is what `gh pr view` says about a pull request.
type PRState struct {
	State       string // OPEN | CLOSED | MERGED
	MergedAt    string // RFC 3339, "" unless merged
	MergeCommit string // 40-hex sha, "" unless merged
}

var (
	prURLExactRe = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]+$`)
	shaHexRe     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	labelRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,49}$`)
)

// PRStatus runs `gh pr view <url> --json state,mergedAt,mergeCommit` in the clone at rootDir.
// Read-only: no write guard is needed. The URL must be a plain GitHub pull request URL.
func (p *Pool) PRStatus(ctx context.Context, rootDir, prURL string) (PRState, error) {
	var st PRState
	if !prURLExactRe.MatchString(prURL) {
		return st, inputErr("not a pull request URL")
	}
	abs, err := p.ResolveDir(rootDir)
	if err != nil {
		return st, err
	}
	out, err := run(ctx, abs, []string{abs}, 30*time.Second, "gh", "pr", "view", prURL, "--json", "state,mergedAt,mergeCommit")
	if err != nil {
		return st, err
	}
	var raw struct {
		State       string `json:"state"`
		MergedAt    string `json:"mergedAt"`
		MergeCommit *struct {
			Oid string `json:"oid"`
		} `json:"mergeCommit"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return st, fmt.Errorf("gh pr view: unexpected output")
	}
	switch raw.State {
	case "OPEN", "CLOSED", "MERGED":
	default:
		return st, fmt.Errorf("gh pr view: unknown state %q", raw.State)
	}
	st.State = raw.State
	if raw.State == "MERGED" {
		st.MergedAt = raw.MergedAt
		if raw.MergeCommit != nil && shaHexRe.MatchString(raw.MergeCommit.Oid) {
			st.MergeCommit = raw.MergeCommit.Oid
		}
	}
	return st, nil
}

// LabelPR adds a label to a pull request (`gh pr edit <url> --add-label <label>`) from the
// worktree wt. WRITE-CLASS: the caller must have passed repoWriteAllowed(RepoOpOpenPR).
func (p *Pool) LabelPR(ctx context.Context, wt, prURL, label string) error {
	if !prURLExactRe.MatchString(prURL) {
		return inputErr("not a pull request URL")
	}
	if !labelRe.MatchString(label) {
		return inputErr("invalid label")
	}
	if err := p.checkWorktree(wt); err != nil {
		return err
	}
	_, err := run(ctx, wt, []string{wt}, 30*time.Second, "gh", "pr", "edit", prURL, "--add-label", label)
	return err
}
