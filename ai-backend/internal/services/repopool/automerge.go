package repopool

// The pinned auto-merge of an approved spec pull request (GAOS_RELEASE_B.md §4). Every command
// here is fixed: the caller passes only values this file validates first, and the output of each
// `gh api` call is reduced by --jq to a short answer (run truncates output at 2000 bytes).

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// HeadSHA is the commit the worktree's HEAD names.
func (p *Pool) HeadSHA(ctx context.Context, wt string) (string, error) {
	if err := p.checkWorktree(wt); err != nil {
		return "", err
	}
	out, err := run(ctx, wt, []string{wt}, 10*time.Second, "git", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(out)
	if !shaHexRe.MatchString(sha) {
		return "", fmt.Errorf("git rev-parse: unexpected output")
	}
	return sha, nil
}

// CommittedExactly reports whether the commits on HEAD since origin/<base> change exactly path,
// and whether HEAD:path is the git blob of content (the bytes a person approved, hashed in Go so
// no clean filter of the clone can change them).
func (p *Pool) CommittedExactly(ctx context.Context, wt, base, path string, content []byte) (bool, error) {
	if err := p.checkWorktree(wt); err != nil {
		return false, err
	}
	if err := ValidateBranch(base); err != nil {
		return false, err
	}
	if err := validRepoRel([]string{path}); err != nil {
		return false, err
	}
	names, err := run(ctx, wt, []string{wt}, 20*time.Second, "git", "diff", "--name-only", "--no-renames",
		"refs/remotes/origin/"+base+"...HEAD")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(names) != path {
		return false, nil
	}
	blob, err := run(ctx, wt, []string{wt}, 10*time.Second, "git", "rev-parse", "--verify", "HEAD:"+path)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(blob) == GitBlobSHA(content), nil
}

// GitBlobSHA is git's object id of a blob holding content (sha1 of "blob <n>\x00" + content).
func GitBlobSHA(content []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// PRRepo is the owner/repo and number of a pull request URL (prURLRe).
func PRRepo(prURL string) (owner, repo string, n int, err error) {
	if !prURLRe.MatchString(prURL) || prURLRe.FindString(prURL) != prURL {
		return "", "", 0, inputErr("invalid pull request URL")
	}
	parts := strings.Split(strings.TrimPrefix(prURL, "https://github.com/"), "/")
	if _, err := fmt.Sscanf(parts[3], "%d", &n); err != nil || n <= 0 {
		return "", "", 0, inputErr("invalid pull request number")
	}
	return parts[0], parts[1], n, nil
}

// RepoAdmin reports whether this identity has admin permission on owner/repo.
func (p *Pool) RepoAdmin(ctx context.Context, wt, owner, repo string) (bool, error) {
	// A missing permissions object (an App token) is not "not admin": it answers "missing".
	out, err := p.ghAPI(ctx, wt, "repos/"+owner+"/"+repo,
		`if .permissions == null then "missing" else (.permissions.admin or .permissions.maintain) end`, false)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "false", nil // anything but an explicit false counts as admin
}

// BranchRuleTypes lists the types of the rules that apply to branch (rulesets, any source).
func (p *Pool) BranchRuleTypes(ctx context.Context, wt, owner, repo, branch string) ([]string, error) {
	if err := ValidateBranch(branch); err != nil {
		return nil, err
	}
	// An update rule that allows fetch-and-merge is reported as "update-fetch", not "update".
	out, err := p.ghAPI(ctx, wt, "repos/"+owner+"/"+repo+"/rules/branches/"+branch,
		`[.[] | if .type == "update" and (.parameters.update_allows_fetch_and_merge // false) then "update-fetch" else .type end]`, true)
	if err != nil {
		return nil, err
	}
	var types []string
	if err := unmarshalPages(out, &types); err != nil {
		return nil, err
	}
	return types, nil
}

// unmarshalPages joins the JSON arrays `gh api --paginate --jq` prints, one per page and line.
func unmarshalPages[T any](out string, into *[]T) error {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var page []T
		if err := json.Unmarshal([]byte(line), &page); err != nil {
			return fmt.Errorf("gh api: unexpected rules answer")
		}
		*into = append(*into, page...)
	}
	return nil
}

// FixedRequiredChecks reports how many required status checks apply to branch, and whether every
// one names the integration (GitHub App or Actions) that must post it.
func (p *Pool) FixedRequiredChecks(ctx context.Context, wt, owner, repo, branch string) (n int, allFixed bool, err error) {
	if err := ValidateBranch(branch); err != nil {
		return 0, false, err
	}
	out, err := p.ghAPI(ctx, wt, "repos/"+owner+"/"+repo+"/rules/branches/"+branch,
		`[.[] | select(.type=="required_status_checks") | .parameters.required_status_checks[]? | (.integration_id != null)]`, true)
	if err != nil {
		return 0, false, err
	}
	var fixed []bool
	if err := unmarshalPages(out, &fixed); err != nil {
		return 0, false, err
	}
	allFixed = len(fixed) > 0
	for _, f := range fixed {
		allFixed = allFixed && f
	}
	return len(fixed), allFixed, nil
}

// EnableAutoMerge turns on GitHub auto-merge (squash) for the pull request, pinned to sha: GitHub
// refuses it, and later never merges, if the head is not sha. WRITE-CLASS: the caller must have
// passed repoWriteAllowed(RepoOpOpenPR).
func (p *Pool) EnableAutoMerge(ctx context.Context, wt, prURL, sha string) error {
	if _, _, _, err := PRRepo(prURL); err != nil {
		return err
	}
	if !shaHexRe.MatchString(sha) {
		return inputErr("invalid commit")
	}
	if err := p.checkWorktree(wt); err != nil {
		return err
	}
	_, err := run(ctx, wt, []string{wt}, 60*time.Second, "gh", "pr", "merge", prURL, "--auto", "--squash", "--match-head-commit", sha)
	return err
}

// DisableAutoMerge turns auto-merge off on a pull request (a GAOS cancel, GAOS_RELEASE_B.md §4.2).
// It runs in the pool root: the job's worktree may already be gone.
func (p *Pool) DisableAutoMerge(ctx context.Context, prURL string) error {
	if _, _, _, err := PRRepo(prURL); err != nil {
		return err
	}
	_, err := run(ctx, p.Root, []string{p.Root}, 60*time.Second, "gh", "pr", "merge", prURL, "--disable-auto")
	return err
}

// ghAPI runs a read-only `gh api <path> --jq <filter>` (with --paginate for a list). owner and
// repo come from PRRepo. The filter keeps the answer far below run's 2000-byte output cap.
func (p *Pool) ghAPI(ctx context.Context, wt, path, jq string, paginate bool) (string, error) {
	if err := p.checkWorktree(wt); err != nil {
		return "", err
	}
	if strings.Contains(path, "..") || strings.HasPrefix(path, "-") || !repoPathRe.MatchString(path) {
		return "", inputErr("invalid API path")
	}
	args := []string{"api", "--method", "GET"}
	if paginate {
		args = append(args, "--paginate")
	}
	return run(ctx, wt, []string{wt}, 30*time.Second, "gh", append(args, path, "--jq", jq)...)
}
