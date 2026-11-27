package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
)

// GAOS_RELEASE_B.md §4: each check refuses with its token; all passing enables the pinned merge.

type fakeAMGitHub struct {
	exact    bool
	exactErr error
	gotBase  []string
	admin    bool
	adminErr error
	types    []string
	checks   int
	fixed    bool
	mergeErr error
	merged   []string
}

func (f *fakeAMGitHub) CommittedExactly(_ context.Context, _, base, _ string, _ []byte) (bool, error) {
	return f.exact, f.exactErr
}
func (f *fakeAMGitHub) RepoAdmin(context.Context, string, string, string) (bool, error) {
	return f.admin, f.adminErr
}
func (f *fakeAMGitHub) BranchRuleTypes(context.Context, string, string, string, string) ([]string, error) {
	return f.types, nil
}
func (f *fakeAMGitHub) FixedRequiredChecks(_ context.Context, _, _, _, branch string) (int, bool, error) {
	f.gotBase = append(f.gotBase, branch)
	return f.checks, f.fixed, nil
}
func (f *fakeAMGitHub) EnableAutoMerge(_ context.Context, _, prURL, sha string) error {
	f.merged = append(f.merged, prURL+"@"+sha)
	return f.mergeErr
}

type fakeAMGAOS struct {
	allowed bool
	err     error
	asked   int
}

func (f *fakeAMGAOS) SpecAutoMergeAllowed(context.Context, string) (bool, error) {
	f.asked++
	return f.allowed, f.err
}

const amSHA = "0123456789abcdef0123456789abcdef01234567"
const amPR = "https://github.com/acme/route-e2e/pull/4"

func goodMerger() (*specAutoMerger, *fakeAMGitHub, *fakeAMGAOS) {
	gh := &fakeAMGitHub{types: []string{"update", "deletion"}, checks: 2, fixed: true, exact: true}
	ga := &fakeAMGAOS{allowed: true}
	return &specAutoMerger{
		requested: func(context.Context) bool { return true },
		on:        func() bool { return true },
		gaos:      ga,
		gh:        gh,
		writeOK:   func(context.Context) error { return nil },
	}, gh, ga
}

func amJob() caseauto.Job {
	return caseauto.Job{ID: "job-1", GaosProductID: "p-9", DefaultBranch: "main"}
}

var amWS = caseauto.Workspace{Dir: "/wt", Branch: "qa-auto/tc-api-001", Base: "release"}

func amIn(job caseauto.Job, pr, sha string) caseauto.AutoMergeIn {
	return caseauto.AutoMergeIn{Job: job, Workspace: amWS, PRURL: pr, PushedSHA: sha,
		ApprovalSource: caseauto.SourceJobPage, SpecPath: "tests/api/rest/x/TC-API-001.spec.ts", SpecContent: "x"}
}

func TestSpecAutoMergeEnablesOnlyWhenEveryCheckHolds(t *testing.T) {
	ctx := context.Background()
	m, gh, ga := goodMerger()
	if got := m.AutoMerge(ctx, amIn(amJob(), amPR, amSHA)); got != caseauto.AutoMergeEnabled {
		t.Fatalf("every check passing answered %q", got)
	}
	if len(gh.gotBase) != 1 || gh.gotBase[0] != "release" {
		t.Errorf("the required checks were read on %v, want the pull request's base", gh.gotBase)
	}
	if len(gh.merged) != 1 || gh.merged[0] != amPR+"@"+amSHA || ga.asked != 1 {
		t.Errorf("merged %v, GAOS asked %d times; want one pinned merge", gh.merged, ga.asked)
	}

	cases := map[string]func(m *specAutoMerger, gh *fakeAMGitHub, ga *fakeAMGAOS, job *caseauto.Job, sha, pr *string){
		"off": func(m *specAutoMerger, _ *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			m.requested = func(context.Context) bool { return false }
		},
		"refused:disabled": func(m *specAutoMerger, _ *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			m.on = func() bool { return false }
		},
		"refused:gaos-denied": func(_ *specAutoMerger, _ *fakeAMGitHub, ga *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			ga.allowed = false
		},
		"refused:gaos-unreachable": func(_ *specAutoMerger, _ *fakeAMGitHub, ga *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			ga.err = errors.New("x")
		},
		"refused:no-sha": func(_ *specAutoMerger, _ *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, sha, _ *string) { *sha = "" },
		"refused:bad-pr-url": func(_ *specAutoMerger, _ *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, pr *string) {
			*pr = "https://example.com/x"
		},
		"refused:admin-identity": func(_ *specAutoMerger, gh *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			gh.admin = true
		},
		"refused:github-unreadable": func(_ *specAutoMerger, gh *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			gh.adminErr = errors.New("x")
		},
		"refused:no-branch-ruleset": func(_ *specAutoMerger, gh *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			gh.types = []string{"deletion"}
		},
		"refused:no-fixed-check": func(_ *specAutoMerger, gh *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			gh.fixed = false
		},
		"refused:write-not-allowed": func(m *specAutoMerger, _ *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			m.writeOK = func(context.Context) error { return errors.New("x") }
		},
		"refused:merge-failed": func(_ *specAutoMerger, gh *fakeAMGitHub, _ *fakeAMGAOS, _ *caseauto.Job, _, _ *string) {
			gh.mergeErr = errors.New("x")
		},
	}
	for want, breakIt := range cases {
		m, gh, ga := goodMerger()
		job, sha, pr := amJob(), amSHA, amPR
		breakIt(m, gh, ga, &job, &sha, &pr)
		got := m.AutoMerge(ctx, amIn(job, pr, sha))
		if got != want {
			t.Errorf("want %s, got %s", want, got)
		}
		if want != "refused:merge-failed" && len(gh.merged) != 0 {
			t.Errorf("%s still called gh pr merge", want)
		}
	}
	// A GAOS-relayed approval, a commit that is not exactly the approved file, an update rule
	// without the deletion rule.
	for want, in := range map[string]func(*caseauto.AutoMergeIn, *fakeAMGitHub){
		"refused:not-approved-in-qa-tool": func(i *caseauto.AutoMergeIn, _ *fakeAMGitHub) { i.ApprovalSource = caseauto.SourceGAOS },
		"refused:content-mismatch":        func(_ *caseauto.AutoMergeIn, gh *fakeAMGitHub) { gh.exact = false },
		"refused:content-unverified":      func(_ *caseauto.AutoMergeIn, gh *fakeAMGitHub) { gh.exactErr = errors.New("x") },
		"refused:no-branch-ruleset":       func(_ *caseauto.AutoMergeIn, gh *fakeAMGitHub) { gh.types = []string{"update"} },
	} {
		m, gh, _ := goodMerger()
		i := amIn(amJob(), amPR, amSHA)
		in(&i, gh)
		if got := m.AutoMerge(ctx, i); got != want || len(gh.merged) != 0 {
			t.Errorf("want %s, got %s (merged %v)", want, got, gh.merged)
		}
	}
	// No required check at all is a refusal too; a product not linked to GAOS never merges.
	m, gh, _ = goodMerger()
	gh.checks = 0
	if got := m.AutoMerge(ctx, amIn(amJob(), amPR, amSHA)); got != "refused:no-fixed-check" {
		t.Errorf("no required check answered %s", got)
	}
	m, _, _ = goodMerger()
	j := amJob()
	j.GaosProductID = ""
	if got := m.AutoMerge(ctx, amIn(j, amPR, amSHA)); got != "refused:gaos-denied" {
		t.Errorf("a product without GAOS answered %s", got)
	}
}

func TestGAOSCapabilitiesFollowTheSwitch(t *testing.T) {
	r := &Router{}
	for _, tc := range []struct{ env, want string }{{"", `["kind"]`}, {"on", `["kind","auto-merge"]`}} {
		t.Setenv("QA_SPEC_AUTO_MERGE", tc.env)
		w := httptest.NewRecorder()
		r.gaosCapabilities(w, httptest.NewRequest(http.MethodGet, gaosCapabilitiesPath, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"capabilities":`+tc.want) {
			t.Errorf("QA_SPEC_AUTO_MERGE=%q answered %d %s", tc.env, w.Code, w.Body)
		}
	}
	if !IsGAOSInboundPath(gaosCapabilitiesPath) {
		t.Error("the capabilities route is not behind the GAOS bearer guard")
	}
}

func TestGAOSKindRules(t *testing.T) {
	for _, tc := range []struct {
		kind, level string
		ok          bool
	}{{"be", "API", true}, {"be", "WEB", false}, {"be", "GQL", false}, {"fullstack", "WEB", true}, {"fullstack", "E2E", true},
		{"fullstack", "API", false}, {"", "WEB", true}} {
		if got := gaosKindFitsLevel(tc.kind, tc.level); got != tc.ok {
			t.Errorf("kind %q level %q: %v, want %v", tc.kind, tc.level, got, tc.ok)
		}
	}
	if l := gaosKindLevels("be"); strings.Join(l, ",") != "API,GQL" || gaosKindLevels("fullstack") != nil || gaosKindLevels("") != nil {
		t.Errorf("run-all levels: be %v", l)
	}
}
