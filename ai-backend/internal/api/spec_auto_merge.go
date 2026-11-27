package api

// GAOS_RELEASE_B.md §4: the pinned auto-merge of an approved spec pull request. The Machine calls
// AutoMerge once, after the pull request is open; the checks run in the order of the spec's table
// and the first that fails answers its refusal token. A refusal never fails the job: the pull
// request stays open for a person, as before.

import (
	"context"
	"slices"
	"strings"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// autoMergeGitHub is the repo pool's GitHub side (fakes in tests).
type autoMergeGitHub interface {
	CommittedExactly(ctx context.Context, wt, base, path string, content []byte) (bool, error)
	RepoAdmin(ctx context.Context, wt, owner, repo string) (bool, error)
	BranchRuleTypes(ctx context.Context, wt, owner, repo, branch string) ([]string, error)
	FixedRequiredChecks(ctx context.Context, wt, owner, repo, branch string) (int, bool, error)
	EnableAutoMerge(ctx context.Context, wt, prURL, sha string) error
}

// autoMergeGAOS asks GAOS whether the product still allows it (gaossync.Syncer).
type autoMergeGAOS interface {
	SpecAutoMergeAllowed(ctx context.Context, gaosPID string) (bool, error)
}

type specAutoMerger struct {
	requested func(ctx context.Context) bool // the job's payload auto_merge_requested
	on        func() bool                    // QA_SPEC_AUTO_MERGE
	gaos      autoMergeGAOS
	gh        autoMergeGitHub
	writeOK   func(ctx context.Context) error // repoWriteAllowed(RepoOpAutoMerge)
	// mark records auto_merge "pending" before gh runs, so a job a cancel or a restart froze
	// still shows that a merge may be armed (nil in tests).
	mark func(ctx context.Context)
}

// autoMergePending is recorded while gh pr merge --auto runs.
const autoMergePending = "pending"

func refusedAM(token string) string { return "refused:" + token }

// AutoMerge implements caseauto.AutoMerger.
func (a *specAutoMerger) AutoMerge(ctx context.Context, in caseauto.AutoMergeIn) string {
	job, ws, prURL, sha := in.Job, in.Workspace, in.PRURL, in.PushedSHA
	if a.requested == nil || !a.requested(ctx) {
		return caseauto.AutoMergeOff
	}
	if a.on == nil || !a.on() {
		return refusedAM("disabled")
	}
	if job.GaosProductID == "" || a.gaos == nil {
		return refusedAM("gaos-denied")
	}
	allowed, err := a.gaos.SpecAutoMergeAllowed(ctx, job.GaosProductID)
	switch {
	case err != nil:
		return refusedAM("gaos-unreachable")
	case !allowed:
		return refusedAM("gaos-denied")
	}
	if sha == "" {
		return refusedAM("no-sha")
	}
	// Only a person in the QA tool approves an auto-merge: an answer relayed by GAOS (or by
	// anything holding GAOS's token) leaves the pull request for a person.
	if !strings.HasPrefix(in.ApprovalSource, "qa-tool:") {
		return refusedAM("not-approved-in-qa-tool")
	}
	// The commit holds the approved file with the approved bytes, and nothing else.
	if ok, err := a.gh.CommittedExactly(ctx, ws.Dir, ws.Base, in.SpecPath, []byte(in.SpecContent)); err != nil {
		return refusedAM("content-unverified")
	} else if !ok {
		return refusedAM("content-mismatch")
	}
	owner, repo, _, err := repopool.PRRepo(prURL)
	if err != nil {
		return refusedAM("bad-pr-url")
	}
	if admin, err := a.gh.RepoAdmin(ctx, ws.Dir, owner, repo); err != nil {
		return refusedAM("github-unreadable")
	} else if admin {
		return refusedAM("admin-identity")
	}
	types, err := a.gh.BranchRuleTypes(ctx, ws.Dir, owner, repo, ws.Branch)
	if err != nil {
		return refusedAM("github-unreadable")
	}
	// Restrict updates (without fetch-and-merge) and restrict deletions must both apply to the
	// automation branch: nobody without bypass adds a commit after the approval, or recreates it.
	if !slices.Contains(types, "update") || !slices.Contains(types, "deletion") {
		return refusedAM("no-branch-ruleset")
	}
	if n, fixed, err := a.gh.FixedRequiredChecks(ctx, ws.Dir, owner, repo, ws.Base); err != nil {
		return refusedAM("github-unreadable")
	} else if n == 0 || !fixed {
		return refusedAM("no-fixed-check")
	}
	if a.writeOK == nil || a.writeOK(ctx) != nil {
		return refusedAM("write-not-allowed")
	}
	if a.mark != nil {
		a.mark(ctx)
	}
	if err := a.gh.EnableAutoMerge(ctx, ws.Dir, prURL, sha); err != nil {
		return refusedAM("merge-failed")
	}
	return caseauto.AutoMergeEnabled
}

// newSpecAutoMerger wires the adapter for one job and one automation repository.
func (r *Router) newSpecAutoMerger(d *db.DB, jobID string, repoID int) *specAutoMerger {
	a := &specAutoMerger{
		requested: func(ctx context.Context) bool {
			j, err := r.lookupJob(ctx, jobID)
			if err != nil || j == nil {
				return false
			}
			v, _ := j.Payload["auto_merge_requested"].(bool)
			return v
		},
		on:      specAutoMergeOn,
		writeOK: func(ctx context.Context) error { return repoWriteAllowed(ctx, d, repoID, RepoOpAutoMerge) },
		mark: func(ctx context.Context) {
			jobState{r}.SetPayload(context.WithoutCancel(ctx), jobID, "auto_merge", autoMergePending)
		},
	}
	if r.gaos.Configured() {
		a.gaos = r.gaos
	}
	if r.repoPool != nil {
		a.gh = r.repoPool
	}
	return a
}
