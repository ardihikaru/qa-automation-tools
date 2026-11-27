package caseauto

import (
	"context"
	"strings"
	"testing"
)

// GAOS_RELEASE_B.md §3.3, §4: the commit pushed after the approval, and the auto-merge hook.

type headRepo struct {
	*fakeRepo
	sha string
}

func (h *headRepo) HeadSHA(context.Context, Workspace) (string, error) { return h.sha, nil }

type fakeMerger struct {
	outcome string
	gotURL  string
	gotSHA  string
	got     AutoMergeIn
	calls   int
}

func (f *fakeMerger) AutoMerge(_ context.Context, in AutoMergeIn) string {
	f.calls++
	f.gotURL, f.gotSHA, f.got = in.PRURL, in.PushedSHA, in
	return f.outcome
}

const pushedSHA = "0123456789abcdef0123456789abcdef01234567"

func TestPublishRecordsThePushedCommitAndTheAutoMergeOutcome(t *testing.T) {
	for _, outcome := range []string{AutoMergeEnabled, "refused:no-fixed-check", AutoMergeOff} {
		r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
		r.m.Repo = &headRepo{fakeRepo: r.repo, sha: pushedSHA}
		fm := &fakeMerger{outcome: outcome}
		r.m.AutoMerge = fm
		r.ap.onGet = approveAfter(1, ApprovalApproved, "dashboard", "ok")
		if _, err := r.m.Run(context.Background(), testJob()); err != nil {
			t.Fatalf("%v\n%s", err, r.ev.all())
		}
		if r.st.kv["pushed_sha"] != pushedSHA {
			t.Errorf("pushed_sha = %v, want the worktree's HEAD", r.st.kv["pushed_sha"])
		}
		if fm.calls != 1 || fm.gotSHA != pushedSHA || fm.gotURL != "https://github.com/o/r/pull/7" {
			t.Errorf("the merger was called %d times with %q %q", fm.calls, fm.gotURL, fm.gotSHA)
		}
		if fm.got.SpecPath != specPath || fm.got.SpecContent == "" {
			t.Errorf("the merger did not get the approval's source and the approved file: %+v", fm.got)
		}
		if r.st.kv["auto_merge"] != outcome {
			t.Errorf("auto_merge = %v, want %s", r.st.kv["auto_merge"], outcome)
		}
		if strings.HasPrefix(outcome, "refused:") && !strings.Contains(r.ev.all(), "auto-merge not enabled (no-fixed-check)") {
			t.Errorf("a refusal left no event: %s", r.ev.all())
		}
	}
}

func TestPublishWithoutHooksRecordsNeither(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.ap.onGet = approveAfter(1, ApprovalApproved, "dashboard", "ok")
	if _, err := r.m.Run(context.Background(), testJob()); err != nil {
		t.Fatalf("%v", err)
	}
	if v := r.st.kv["pushed_sha"]; v != "" {
		t.Error("a repo without HeadSHA recorded a pushed_sha")
	}
	if v := r.st.kv["auto_merge"]; v != "" {
		t.Error("a machine without an AutoMerger recorded auto_merge")
	}
	// A malformed HEAD is never recorded.
	r2 := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r2.m.Repo = &headRepo{fakeRepo: r2.repo, sha: "not-a-sha"}
	r2.ap.onGet = approveAfter(1, ApprovalApproved, "dashboard", "ok")
	if _, err := r2.m.Run(context.Background(), testJob()); err != nil {
		t.Fatalf("%v", err)
	}
	if v := r2.st.kv["pushed_sha"]; v != "" {
		t.Error("a malformed HEAD was recorded")
	}
}
