package caseauto

import (
	"context"
	"strings"
	"testing"
)

// LEARNING_LOOP.md decision 8: the loop closes only if a merged learned policy reaches the
// first generation prompt of the next case_create job.

const learnedFile = `# Learned policy: Build vendor URLs from the registry base URL

<!-- qa-learning: product=3 job=job-9 findings=4,7 -->

Build REST URLs from restServices['sut-vendor-backend'].baseURL. It already ends in /api/v1: never append /api/v1 again.

## Why

A reviewer asked for changes on a doubled /api/v1 prefix.

## Evidence

- review_changes: cases TC-VSYS-003; QA tools job job-1
`

func TestLearnedPoliciesReachTheFirstPrompt(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.repo.files[".claude/rules/learned-vendor-url-base.md"] = learnedFile
	r.repo.files[".claude/rules/credentials-security.md"] = "hand-written rule, not learned"
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	if _, err := r.m.Run(context.Background(), testJob()); err != nil {
		t.Fatalf("%v\n%s", err, r.ev.all())
	}
	first := r.llm.calls[0][0].Content
	for _, want := range []string{"LEARNED POLICIES", "// .claude/rules/learned-vendor-url-base.md", "Build vendor URLs from the registry base URL", "never append /api/v1 again"} {
		if !strings.Contains(first, want) {
			t.Errorf("prompt lacks %q:\n%s", want, first)
		}
	}
	// Why, Evidence and the provenance comment are for humans; hand-written rules are not learned.
	for _, bad := range []string{"## Why", "A reviewer asked", "## Evidence", "qa-learning:", "hand-written rule"} {
		if strings.Contains(first, bad) {
			t.Errorf("prompt carries %q", bad)
		}
	}
	if i, j := strings.Index(first, "REPO GUIDE"), strings.Index(first, "LEARNED POLICIES"); i < 0 || j < i {
		t.Errorf("learned block must follow the repo guide (%d, %d)", i, j)
	}
	if !strings.Contains(r.ev.all(), "1 learned policy in the prompt") {
		t.Errorf("no info event:\n%s", r.ev.all())
	}
}

func TestLearnedBudgetLeavesFilesOutWithAWarn(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	big := strings.Repeat("Use the typed client. ", 70) // ~1.5 KB each after the clip
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		r.repo.files[".claude/rules/learned-"+n+".md"] = big
	}
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	if _, err := r.m.Run(context.Background(), testJob()); err != nil {
		t.Fatal(err)
	}
	first := r.llm.calls[0][0].Content
	if !strings.Contains(first, "learned-a.md") || strings.Contains(first, "learned-l.md") {
		t.Errorf("budget order wrong")
	}
	if !strings.Contains(r.ev.all(), "over the 16000-byte budget were left out: .claude/rules/learned-k.md, .claude/rules/learned-l.md") {
		t.Errorf("no budget warn:\n%s", r.ev.all())
	}
}

func TestGuideClipIsVisible(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.repo.files["AGENTS.md"] = strings.Repeat("x", 30480)
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	if _, err := r.m.Run(context.Background(), testJob()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.ev.all(), "warn/generate: AGENTS.md is 30480 bytes; only the first 12000 reach the model") {
		t.Errorf("no clip warn:\n%s", r.ev.all())
	}
}

func TestFailureExcerptInTheRunWarn(t *testing.T) {
	failing := RunResult{Message: code + " failed", Failed: 1, Tail: "Running 1 test\n  1) tests/web/x.spec.ts\n    Error: expect(received).toBe(expected)\n    at foo"}
	r := newRig([]string{reply(specPath, goodSpec("")), reply(specPath, goodSpec(""))}, []RunResult{failing, pass})
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	if _, err := r.m.Run(context.Background(), testJob()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.ev.all(), "warn/run: the spec did not pass: "+code+" failed\nError: expect(received).toBe(expected)") {
		t.Errorf("no excerpt:\n%s", r.ev.all())
	}
}

func TestLearnedPolicyPart(t *testing.T) {
	got := LearnedPolicyPart(learnedFile)
	want := "Learned policy: Build vendor URLs from the registry base URL\nBuild REST URLs from restServices['sut-vendor-backend'].baseURL. It already ends in /api/v1: never append /api/v1 again."
	if got != want {
		t.Errorf("got %q", got)
	}
	if got := LearnedPolicyPart("---\npaths: x\n---\n# Hand rule\nbody\n"); got != "# Hand rule\nbody" {
		t.Errorf("front matter: %q", got)
	}
	if FailureExcerpt("all good") != "" || FailureExcerpt("x\nTimeout 30000ms exceeded.\n") != "Timeout 30000ms exceeded." {
		t.Error("excerpt")
	}
}
