package caseauto

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

const code = "TC-PATREG-020"
const specPath = "tests/web/patreg/TC-PATREG-020.spec.ts"

func goodSpec(extra string) string {
	return "import { test, expect } from '@playwright/test';\n" +
		"import { login } from '../../helpers/login';\n" +
		"test('" + code + " check-in writes the registration number', async ({ page }) => {\n" +
		"  await login(page);\n  await page.goto(process.env.BASE_URL + '/x');\n" + extra + "});\n"
}

func reply(path, content string) string {
	return "Here you go.\nFILE: " + path + "\n```ts\n" + strings.TrimRight(content, "\n") + "\n```\n"
}

func TestRedact(t *testing.T) {
	r := NewRedactor("s3cr3tvalue", "abc")
	for in, want := range map[string]string{
		"Authorization: Bearer abcdefgh12345678":         "Authorization: Bearer ***",
		"token ghp_abcdefghijklmnopqrstuvwxyz0123456789": "token ***",
		"github_pat_11ABCDEFGH0123456789abcdefghij":      "***",
		"key gaos_0123456789abcdef":                      "key ***",
		"clone https://user:pw@github.com/o/r.git":       "clone https://***@github.com/o/r.git",
		"value s3cr3tvalue here, abc stays":              "value *** here, abc stays",
		"nothing secret":                                 "nothing secret",
		"https://github.com/o/r (no userinfo)":           "https://github.com/o/r (no userinfo)",
	} {
		if got := r.Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
	var nilR *Redactor
	if got := nilR.Redact("Bearer abcdefgh12345678"); strings.Contains(got, "abcdefgh") {
		t.Errorf("nil redactor leaked: %q", got)
	}
	if got := Clip("héllo wörld", 8); len(got) > 8 || !strings.HasSuffix(got, "...") {
		t.Errorf("Clip = %q", got)
	}
}

func TestParseReply(t *testing.T) {
	ok := reply(specPath, "x")
	f, err := ParseReply(ok)
	if err != nil || f.Path != specPath || f.Content != "x\n" {
		t.Fatalf("%+v %v", f, err)
	}
	for name, in := range map[string]string{
		"no file line":    "```ts\nx\n```",
		"two file lines":  "FILE: a\nFILE: b\n```ts\nx\n```",
		"no block":        "FILE: " + specPath + "\nnothing",
		"two blocks":      "FILE: " + specPath + "\n```ts\nx\n```\n```ts\ny\n```",
		"unclosed":        "FILE: " + specPath + "\n```ts\nx\n",
		"inline file ref": "see FILE: x in prose\n```ts\nx\n```",
	} {
		if _, err := ParseReply(in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestValidate(t *testing.T) {
	opts := ValidateOptions{Code: code, AllowedHosts: []string{"staging.example.com"}, AllowedImports: map[string]bool{"dotenv": true}}
	good := goodSpec("")
	cases := []struct {
		name    string
		path    string
		content string
		wantErr string
	}{
		{"good", specPath, good, ""},
		{"good api path", "tests/api/rest/patreg/TC-PATREG-020.spec.ts", good, ""},
		{"staging host ok", specPath, goodSpec("  await page.goto('https://staging.example.com/a');\n"), ""},
		{"bad dir", "tests/mq/patreg/TC-PATREG-020.spec.ts", good, "path must be"},
		{"other code in path", "tests/web/patreg/TC-PATREG-021.spec.ts", good, "path must be"},
		{"traversal", "tests/web/../x/TC-PATREG-020.spec.ts", good, "path must be"},
		{"outside tests", "src/TC-PATREG-020.spec.ts", good, "path must be"},
		{"empty", specPath, "  \n", "empty"},
		{"too big", specPath, good + strings.Repeat("// pad\n", 4000), "larger than"},
		{"no code", specPath, "import { test } from '@playwright/test';\ntest('login', async () => {});\n", "case code"},
		{"title without code", specPath, good + "test('something else', async () => {});\n", "must start with"},
		{"test.skip", specPath, good + "test.skip('" + code + " later', async () => {});\n", "not allowed"},
		{"test.only", specPath, strings.Replace(good, "test('", "test.only('", 1), "not allowed"},
		{"dump env", specPath, goodSpec("  console.log(process.env);\n"), "process.env"},
		{"stringify env", specPath, goodSpec("  const s = JSON.stringify(process.env);\n"), "process.env"},
		{"entries env", specPath, goodSpec("  Object.entries(process.env);\n"), "process.env"},
		{"spread env", specPath, goodSpec("  const e = {...process.env};\n"), "process.env"},
		{"dynamic env key", specPath, goodSpec("  const v = process.env[k];\n"), "process.env"},
		{"child process", specPath, goodSpec("  require('child_process');\n"), "runs processes"},
		{"eval", specPath, goodSpec("  eval('1');\n"), "evaluates"},
		{"dynamic import", specPath, goodSpec("  await import('x');\n"), "dynamic import"},
		{"fs import", specPath, "import * as fs from 'fs';\n" + good, "not allowed"},
		{"allowed bare import", specPath, "import 'dotenv';\n" + good, ""},
		{"relative escape", specPath, "import { x } from '../../../../outside';\n" + good, "leaves the repo"},
		{"other host", specPath, goodSpec("  await page.goto('https://evil.example.org/a');\n"), "staging host"},
		{"ws host", specPath, goodSpec("  new WebSocket('wss://evil.example.org/s');\n"), "staging host"},
		{"nul byte", specPath, good + "\x00", "UTF-8"},
	}
	for _, c := range cases {
		err := Validate(GeneratedFile{Path: c.path, Content: c.content}, opts)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: err=%v, want %q", c.name, err, c.wantErr)
		}
	}
	if err := Validate(GeneratedFile{Path: specPath, Content: good}, ValidateOptions{Code: "bad"}); err == nil {
		t.Error("bad case code accepted")
	}
}

func TestVerdict(t *testing.T) {
	report := func(status string) map[string]interface{} {
		return map[string]interface{}{"passed": 1.0, "failed": 0.0, "report": map[string]interface{}{
			"stats": map[string]interface{}{"startTime": "2026-11-14T10:00:00Z"},
			"suites": []interface{}{map[string]interface{}{"specs": []interface{}{map[string]interface{}{
				"title": code + " x", "tests": []interface{}{map[string]interface{}{"status": status}}}}}}}}
	}
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	if v := VerdictFromResult(report("expected"), code, specPath, now); !v.Pass || v.Passed != 1 {
		t.Errorf("pass: %+v", v)
	}
	if v := VerdictFromResult(report("unexpected"), code, specPath, now); v.Pass || !strings.Contains(v.Message, "failed") {
		t.Errorf("fail: %+v", v)
	}
	if v := VerdictFromResult(report("skipped"), code, specPath, now); v.Pass {
		t.Errorf("skipped passes: %+v", v)
	}
	if v := VerdictFromResult(report("expected"), "TC-PATREG-999", specPath, now); v.Pass || v.Message != "no test titled TC-PATREG-999 in "+specPath {
		t.Errorf("no title: %+v", v)
	}
	if v := VerdictFromResult(map[string]interface{}{"tail": "boom"}, code, specPath, now); v.Pass || !strings.Contains(v.Message, "boom") {
		t.Errorf("no report: %+v", v)
	}
}

func TestDecisionFromAnswer(t *testing.T) {
	for _, c := range []struct{ label, choice, want string }{
		{"Approve", "", ApprovalApproved}, {"approve", "0", ApprovalApproved}, {"Reject", "", ApprovalRejected},
		{"Needs changes", "", ApprovalChanges}, {"", `"Approve"`, ApprovalApproved}, {"Other", "2", ""}, {"", "", ""},
	} {
		got, ok := DecisionFromAnswer(c.label, c.choice)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%q/%q = %q,%v", c.label, c.choice, got, ok)
		}
	}
}

// ---- fakes ----

type fakeRepo struct {
	mu        sync.Mutex
	files     map[string]string
	committed []string
	pushed    bool
	prs       int
	pushErr   error
	prErr     error
}

func newRepo() *fakeRepo {
	return &fakeRepo{files: map[string]string{
		"AGENTS.md":                              "guide",
		"tests/web/patreg/TC-PATREG-001.spec.ts": "import { test } from '@playwright/test';\nimport { login } from '../../helpers/login';\ntest('TC-PATREG-001 a', async () => {});",
		"tests/web/other/TC-OTHER-001.spec.ts":   "import { test } from '@playwright/test';\ntest('TC-OTHER-001 a', async () => {});",
	}}
}
func (f *fakeRepo) Prepare(context.Context, Job) (Workspace, error) {
	return Workspace{Dir: "/wt", Branch: "qa-auto/x", Base: "main"}, nil
}
func (f *fakeRepo) Open(context.Context, Job) (Workspace, error) {
	return Workspace{Dir: "/wt", Branch: "qa-auto/x", Base: "main"}, nil
}
func (f *fakeRepo) ReadFile(_ Workspace, rel string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.files[rel]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("missing")
}
func (f *fakeRepo) ListSpecs(Workspace) ([]string, error) {
	var out []string
	for k := range f.files {
		if strings.HasSuffix(k, ".spec.ts") {
			out = append(out, k)
		}
	}
	return out, nil
}
func (f *fakeRepo) WriteFile(_ Workspace, rel string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[rel] = string(data)
	return nil
}
func (f *fakeRepo) DiffStat(context.Context, Workspace, string) (string, error) {
	return " 1 file changed, 5 insertions(+)", nil
}
func (f *fakeRepo) Commit(_ context.Context, _ Workspace, msg, rel string) error {
	f.committed = append(f.committed, msg+"|"+rel)
	return nil
}
func (f *fakeRepo) Push(context.Context, Workspace) error {
	if f.pushErr != nil {
		return f.pushErr
	}
	f.pushed = true
	return nil
}
func (f *fakeRepo) OpenPR(_ context.Context, _ Workspace, title, body string) (string, error) {
	if f.prErr != nil {
		return "", f.prErr
	}
	f.prs++
	if strings.Contains(body, "ghp_") {
		return "", errors.New("PR body leaked a token")
	}
	return "https://github.com/o/r/pull/7", nil
}

type fakeLLM struct {
	mu      sync.Mutex
	replies []string
	calls   [][]models.Message
	opts    []ai.ChatOpts
	err     error
}

func (l *fakeLLM) ChatMessages(_ context.Context, msgs []models.Message, o ai.ChatOpts) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, append([]models.Message(nil), msgs...))
	l.opts = append(l.opts, o)
	if l.err != nil {
		return "", l.err
	}
	if len(l.replies) == 0 {
		return "", errors.New("no more replies")
	}
	r := l.replies[0]
	l.replies = l.replies[1:]
	return r, nil
}

type fakeExec struct {
	results []RunResult
	err     error
	reqs    []RunRequest
}

func (e *fakeExec) RunCase(_ context.Context, req RunRequest, emit func(level, step, msg string)) (RunResult, error) {
	e.reqs = append(e.reqs, req)
	emit("info", "", "executor says hi")
	if e.err != nil {
		return RunResult{}, e.err
	}
	r := e.results[0]
	if len(e.results) > 1 {
		e.results = e.results[1:]
	}
	return r, nil
}

type fakeApprovals struct {
	mu    sync.Mutex
	rows  map[string]*Approval
	order []string
	// onGet is called with the call count of Get for an external id; it may answer.
	onGet func(a *fakeApprovals, ext string, n int)
	gets  map[string]int
}

func newApprovals() *fakeApprovals {
	return &fakeApprovals{rows: map[string]*Approval{}, gets: map[string]int{}}
}
func (a *fakeApprovals) Open(_ context.Context, _ string, _ int, _, ext string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.rows[ext]; !ok {
		a.rows[ext] = &Approval{ExternalID: ext, Status: ApprovalOpen}
		a.order = append(a.order, ext)
	}
	return nil
}
func (a *fakeApprovals) Get(_ context.Context, ext string) (*Approval, error) {
	a.mu.Lock()
	a.gets[ext]++
	n := a.gets[ext]
	cb := a.onGet
	a.mu.Unlock()
	if cb != nil {
		cb(a, ext, n)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if r, ok := a.rows[ext]; ok {
		c := *r
		return &c, nil
	}
	return nil, nil
}
func (a *fakeApprovals) MarkGaosCreated(_ context.Context, ext string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rows[ext].GaosCreated = true
	return nil
}
func (a *fakeApprovals) Answer(_ context.Context, ext, status, by, text string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.rows[ext]
	if r == nil || r.Status != ApprovalOpen {
		return false, nil
	}
	r.Status, r.AnsweredBy, r.Text = status, by, text
	return true, nil
}

type fakeEvents struct {
	mu   sync.Mutex
	list []string
}

func (e *fakeEvents) Add(_ context.Context, _, level, step, msg string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, level+"/"+step+": "+msg)
	return nil
}
func (e *fakeEvents) all() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.list, "\n")
}

type fakeState struct{ kv map[string]interface{} }

func (s *fakeState) SetPayload(_ context.Context, _ string, k string, v interface{}) {
	if s.kv == nil {
		s.kv = map[string]interface{}{}
	}
	s.kv[k] = v
}

type fakeInbox struct {
	created []gaossync.InboxCreate
	item    *gaossync.InboxItem
	cErr    error
}

func (i *fakeInbox) CreateInboxItem(_ context.Context, _ string, req gaossync.InboxCreate) (*gaossync.InboxCreated, error) {
	if i.cErr != nil {
		return nil, i.cErr
	}
	i.created = append(i.created, req)
	return &gaossync.InboxCreated{ExternalID: req.ExternalID, Status: "open", Created: true}, nil
}
func (i *fakeInbox) GetInboxItem(_ context.Context, _ string, ext string) (*gaossync.InboxItem, error) {
	if i.item == nil {
		return &gaossync.InboxItem{ExternalID: ext, Status: gaossync.InboxOpen}, nil
	}
	return i.item, nil
}

type rig struct {
	m    *Machine
	repo *fakeRepo
	llm  *fakeLLM
	ex   *fakeExec
	ap   *fakeApprovals
	ev   *fakeEvents
	st   *fakeState
	in   *fakeInbox
	now  time.Time
}

func newRig(replies []string, results []RunResult) *rig {
	r := &rig{repo: newRepo(), llm: &fakeLLM{replies: replies}, ex: &fakeExec{results: results}, ap: newApprovals(), ev: &fakeEvents{}, st: &fakeState{}, in: &fakeInbox{}, now: time.Unix(1_700_000_000, 0)}
	r.m = NewMachine(r.repo, r.llm, r.ex, r.ap, r.ev, r.st, r.in, NewRedactor("topsecretvalue"))
	r.m.Now = func() time.Time { return r.now }
	r.m.Sleep = func(ctx context.Context, d time.Duration) error { r.now = r.now.Add(d); return ctx.Err() }
	return r
}

func testJob() Job {
	return Job{ID: "job-1", ProductID: 3, ProductName: "SUT", Branch: "qa-auto/tc-patreg-020", DefaultBranch: "main",
		AllowedHosts: []string{"staging.example.com"}, JobURL: "https://qa-tool.example/jobs/job-1",
		Case: Case{Code: code, Scenario: "Check-in writes the registration number\nsteps...", StoryCode: "US-1", StoryText: "As a clerk", Features: []string{"F-1"}}}
}

var pass = RunResult{Pass: true, Message: code + " passed", Passed: 1}
var fail = RunResult{Message: code + " failed", Failed: 1, Tail: "expected 1 got 2 ghp_abcdefghijklmnopqrstuvwxyz0123456789 topsecretvalue"}

func approveAfter(n int, status, by, text string) func(a *fakeApprovals, ext string, c int) {
	return func(a *fakeApprovals, ext string, c int) {
		if c == n {
			a.Answer(context.Background(), ext, status, by, text)
		}
	}
}

func TestHappyPath(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.ap.onGet = approveAfter(3, ApprovalApproved, "dashboard", "looks good")
	job := testJob()
	job.GaosProductID = "p-6"
	out, err := r.m.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("%v\n%s", err, r.ev.all())
	}
	if out.PRURL != "https://github.com/o/r/pull/7" || out.SpecPath != specPath || out.Repairs != 0 {
		t.Fatalf("%+v", out)
	}
	if !r.repo.pushed || r.repo.prs != 1 || len(r.repo.committed) != 1 || !strings.HasPrefix(r.repo.committed[0], "test(TC-PATREG-020): automate check-in writes") {
		t.Fatalf("repo: %+v", r.repo)
	}
	if len(r.in.created) != 1 || r.in.created[0].ExternalID != "qa-job-1-pr" || len(r.in.created[0].Options) != 3 || r.in.created[0].ExpiresInHours != 72 {
		t.Fatalf("inbox: %+v", r.in.created)
	}
	if !r.ap.rows["qa-job-1-pr"].GaosCreated {
		t.Error("gaos_created not marked")
	}
	if r.st.kv["pr_url"] != "https://github.com/o/r/pull/7" || r.st.kv["spec_path"] != specPath {
		t.Errorf("state %+v", r.st.kv)
	}
	steps := r.ev.all()
	for _, want := range []string{"/prepare:", "/generate:", "/write:", "/run: executor says hi", "/approval:", "/push:", "/pr:", "/done:"} {
		if !strings.Contains(steps, want) {
			t.Errorf("missing event %q in\n%s", want, steps)
		}
	}
	// The first prompt carries the guide and the same-module example, not the other module's.
	first := r.llm.calls[0][0].Content
	if !strings.Contains(first, "guide") || !strings.Contains(first, "TC-PATREG-001") || !strings.Contains(first, code) {
		t.Errorf("prompt lacks context:\n%s", first)
	}
}

func TestRepairLoopThenPass(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec("")), reply(specPath, goodSpec("  await expect(page).toHaveURL(/x/);\n"))}, []RunResult{fail, pass})
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	out, err := r.m.Run(context.Background(), testJob())
	if err != nil || out.Repairs != 1 {
		t.Fatalf("%+v %v\n%s", out, err, r.ev.all())
	}
	if len(r.ex.reqs) != 2 || r.ex.reqs[0].Repair != 0 || r.ex.reqs[1].Repair != 1 {
		t.Errorf("run requests %+v", r.ex.reqs)
	}
	// The repair prompt carries the failing tail with secrets removed.
	repair := r.llm.calls[1][len(r.llm.calls[1])-1].Content
	if !strings.Contains(repair, "expected 1 got 2") || strings.Contains(repair, "ghp_abc") || strings.Contains(repair, "topsecretvalue") {
		t.Errorf("repair prompt: %q", repair)
	}
	if strings.Contains(r.ev.all(), "ghp_abc") {
		t.Error("event log leaked a token")
	}
}

func TestRepairBudgetExhausted(t *testing.T) {
	replies := make([]string, 5)
	for i := range replies {
		replies[i] = reply(specPath, goodSpec(fmt.Sprintf("  // v%d\n", i)))
	}
	r := newRig(replies, []RunResult{fail})
	_, err := r.m.Run(context.Background(), testJob())
	if err == nil || !strings.Contains(err.Error(), "after 3 repairs") {
		t.Fatalf("err=%v", err)
	}
	if len(r.llm.calls) != 4 || len(r.ex.reqs) != 4 {
		t.Errorf("llm calls=%d runs=%d, want 4 and 4 (1 generation + 3 repairs)", len(r.llm.calls), len(r.ex.reqs))
	}
	if r.repo.pushed || len(r.ap.rows) != 0 {
		t.Error("nothing may be pushed or asked after a failing spec")
	}
	if !IsStepErr(err) || !strings.Contains(r.ev.all(), "error/repair") {
		t.Errorf("events: %s", r.ev.all())
	}
}

func TestInvalidReplyIsRepaired(t *testing.T) {
	bad := reply(specPath, goodSpec("  console.log(process.env);\n"))
	r := newRig([]string{bad, reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	if _, err := r.m.Run(context.Background(), testJob()); err != nil {
		t.Fatalf("%v\n%s", err, r.ev.all())
	}
	if _, ok := r.repo.files[specPath]; !ok || strings.Contains(r.repo.files[specPath], "console.log") {
		t.Error("the refused file must never be written")
	}
	if len(r.ex.reqs) != 1 {
		t.Errorf("a refused reply must not reach the executor: %d runs", len(r.ex.reqs))
	}
	if !strings.Contains(r.llm.calls[1][len(r.llm.calls[1])-1].Content, "refused") {
		t.Error("the LLM was not told why")
	}
}

func TestLLMAndExecutorErrors(t *testing.T) {
	r := newRig(nil, nil)
	r.llm.err = errors.New("ai-manager 502")
	if _, err := r.m.Run(context.Background(), testJob()); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("llm: %v", err)
	}
	r = newRig([]string{reply(specPath, goodSpec(""))}, nil)
	r.ex.err = errors.New("executor down")
	if _, err := r.m.Run(context.Background(), testJob()); err == nil || !strings.Contains(err.Error(), "executor down") {
		t.Fatalf("exec: %v", err)
	}
	r = newRig(nil, nil)
	bad := testJob()
	bad.Case.Code = "nope"
	if _, err := r.m.Run(context.Background(), bad); err == nil {
		t.Fatal("bad code accepted")
	}
}

func TestRejectAndExpiry(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.ap.onGet = approveAfter(2, ApprovalRejected, "dashboard", "not needed")
	_, err := r.m.Run(context.Background(), testJob())
	if err == nil || !strings.Contains(err.Error(), "rejected") || r.repo.pushed {
		t.Fatalf("reject: %v pushed=%v", err, r.repo.pushed)
	}

	r = newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	_, err = r.m.Run(context.Background(), testJob())
	if err == nil || !strings.Contains(err.Error(), "expired") || r.repo.pushed {
		t.Fatalf("expiry: %v", err)
	}
	if r.ap.rows["qa-job-1-pr"].Status != ApprovalExpired {
		t.Error("the approval must be closed as expired")
	}
}

func TestChangesRoundThenApprove(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec("")), reply(specPath, goodSpec("  // changed\n"))}, []RunResult{pass})
	r.ap.onGet = func(a *fakeApprovals, ext string, n int) {
		switch {
		case ext == "qa-job-1-pr" && n == 2:
			a.Answer(context.Background(), ext, ApprovalChanges, "dashboard", "use the other locator")
		case ext == "qa-job-1-pr2" && n == 2:
			a.Answer(context.Background(), ext, ApprovalApproved, "dashboard", "")
		}
	}
	out, err := r.m.Run(context.Background(), testJob())
	if err != nil || out.PRURL == "" {
		t.Fatalf("%+v %v\n%s", out, err, r.ev.all())
	}
	last := r.llm.calls[1][len(r.llm.calls[1])-1].Content
	if !strings.Contains(last, "use the other locator") {
		t.Errorf("reviewer text not sent: %q", last)
	}
	if len(r.ap.order) != 2 || r.ap.order[1] != "qa-job-1-pr2" {
		t.Errorf("approvals %v", r.ap.order)
	}
	if !strings.Contains(r.repo.files[specPath], "// changed") {
		t.Error("the revised file was not written")
	}
}

// AI_CONVERSATIONS.md: every call names the job's conversation and labels its messages.
func TestCallsCarryTheConversationAndKinds(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec("")), reply(specPath, goodSpec("  // fixed\n")), reply(specPath, goodSpec("  // changed\n"))}, []RunResult{fail, pass, pass})
	r.ap.onGet = func(a *fakeApprovals, ext string, n int) {
		switch {
		case ext == "qa-job-1-pr" && n == 2:
			a.Answer(context.Background(), ext, ApprovalChanges, "dashboard", "use the other locator")
		case ext == "qa-job-1-pr2" && n == 2:
			a.Answer(context.Background(), ext, ApprovalApproved, "dashboard", "")
		}
	}
	if _, err := r.m.Run(context.Background(), testJob()); err != nil {
		t.Fatalf("%v\n%s", err, r.ev.all())
	}
	if len(r.llm.opts) != 3 {
		t.Fatalf("calls %d", len(r.llm.opts))
	}
	for i, o := range r.llm.opts {
		if c := o.Conversation; c == nil || c.Kind != ai.ConvJob || c.Ref != "job-1" || c.ProductID != 3 || c.Title != code {
			t.Errorf("call %d conversation %+v", i, o.Conversation)
		}
	}
	var kinds []string
	for _, m := range r.llm.calls[2] {
		kinds = append(kinds, m.Kind)
	}
	if got := strings.Join(kinds, ","); got != "prompt,reply,run_result,reply,feedback" {
		t.Errorf("kinds %s", got)
	}
}

func TestChangesLimit(t *testing.T) {
	replies := []string{reply(specPath, goodSpec("")), reply(specPath, goodSpec("//1\n")), reply(specPath, goodSpec("//2\n")), reply(specPath, goodSpec("//3\n"))}
	r := newRig(replies, []RunResult{pass})
	r.ap.onGet = func(a *fakeApprovals, ext string, n int) {
		if n == 2 {
			a.Answer(context.Background(), ext, ApprovalChanges, "dashboard", "again")
		}
	}
	_, err := r.m.Run(context.Background(), testJob())
	if err == nil || !strings.Contains(err.Error(), "too many times") {
		t.Fatalf("%v", err)
	}
}

func TestGaosDoorAnswers(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.in.item = &gaossync.InboxItem{ExternalID: "qa-job-1-pr", Status: gaossync.InboxAnswered, OptionLabel: "Approve", Text: "ok from gaos"}
	job := testJob()
	job.GaosProductID = "p-6"
	out, err := r.m.Run(context.Background(), job)
	if err != nil || out.PRURL == "" {
		t.Fatalf("%+v %v\n%s", out, err, r.ev.all())
	}
	if a := r.ap.rows["qa-job-1-pr"]; a.AnsweredBy != "gaos" || a.Status != ApprovalApproved {
		t.Errorf("%+v", a)
	}
}

func TestGaosOutageFallsBackToDashboard(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.in.cErr = errors.New("could not reach GAOS")
	r.ap.onGet = approveAfter(3, ApprovalApproved, "dashboard", "")
	job := testJob()
	job.GaosProductID = "p-6"
	if _, err := r.m.Run(context.Background(), job); err != nil {
		t.Fatalf("%v\n%s", err, r.ev.all())
	}
	if !strings.Contains(r.ev.all(), "GAOS Inbox is unavailable") {
		t.Error("the outage must be logged")
	}
	// Not integrated: GAOS is never called.
	r = newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	if _, err := r.m.Run(context.Background(), testJob()); err != nil || len(r.in.created) != 0 {
		t.Fatalf("%v created=%d", err, len(r.in.created))
	}
}

func TestPushAndPRFailures(t *testing.T) {
	r := newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	r.repo.pushErr = errors.New("repo is read-only")
	if _, err := r.m.Run(context.Background(), testJob()); err == nil || !strings.Contains(err.Error(), "push failed") || r.repo.prs != 0 {
		t.Fatalf("push: %v", err)
	}
	r = newRig([]string{reply(specPath, goodSpec(""))}, []RunResult{pass})
	r.ap.onGet = approveAfter(2, ApprovalApproved, "dashboard", "")
	r.repo.prErr = errors.New("gh failed")
	if _, err := r.m.Run(context.Background(), testJob()); err == nil || !strings.Contains(err.Error(), "pull request failed") {
		t.Fatalf("pr: %v", err)
	}
}

func TestResumeSkipsToApproval(t *testing.T) {
	r := newRig(nil, nil)
	r.repo.files[specPath] = goodSpec("")
	r.ap.rows["qa-job-1-pr"] = &Approval{ExternalID: "qa-job-1-pr", Status: ApprovalOpen}
	r.ap.order = []string{"qa-job-1-pr"}
	r.ap.onGet = approveAfter(2, ApprovalApproved, "gaos", "")
	job := testJob()
	job.SpecPath = specPath
	out, err := r.m.Resume(context.Background(), job)
	if err != nil || out.PRURL == "" {
		t.Fatalf("%+v %v\n%s", out, err, r.ev.all())
	}
	if len(r.llm.calls) != 0 || len(r.ex.reqs) != 0 {
		t.Error("resume must not call the LLM or the executor")
	}
	job.SpecPath = "../../etc/passwd"
	if _, err := r.m.Resume(context.Background(), job); err == nil {
		t.Error("a bad spec path was accepted on resume")
	}
}

func TestEventsAreRedactedAndClipped(t *testing.T) {
	r := newRig(nil, nil)
	r.m.ev(context.Background(), testJob(), "info", StepRun, "token ghp_abcdefghijklmnopqrstuvwxyz0123456789 "+strings.Repeat("x", 2000))
	got := r.ev.list[0]
	if strings.Contains(got, "ghp_") || len(got) > 1100 {
		t.Errorf("event not redacted/clipped (%d bytes): %.80s", len(got), got)
	}
}

func TestValidateFixtures(t *testing.T) {
	known := FixtureNames("export interface ApiFixtures {\n  vendorApi: X;\n  hisBackendUser: Y;\n}\n")
	if !known["vendorApi"] || !known["hisBackendUser"] {
		t.Fatalf("fixture names not extracted: %v", known)
	}
	spec := func(fx string) GeneratedFile {
		return GeneratedFile{Path: "tests/api/rest/sut-vendor-backend/TC-DMASTER-007.spec.ts",
			Content: "import { test } from '@playwright/test';\ntest('TC-DMASTER-007 ok', async ({ " + fx + " }) => {});\n"}
	}
	o := ValidateOptions{Code: "TC-DMASTER-007", Fixtures: known}
	if err := Validate(spec("vendorApi, request"), o); err != nil {
		t.Fatalf("known fixtures must pass: %v", err)
	}
	if err := Validate(spec("hisVendorBackendMasterDataRest"), o); err == nil {
		t.Fatal("a guessed fixture must be rejected")
	}
	o.Fixtures = nil
	if err := Validate(spec("anything"), o); err != nil {
		t.Fatalf("no fixture info means no check: %v", err)
	}
}

func TestEnvFailure(t *testing.T) {
	if envFailure("Error: Invalid configuration for TEST_ENV=staging:\n x") == "" {
		t.Fatal("config load error must be detected")
	}
	if envFailure("Error: expected 200 received 500") != "" {
		t.Fatal("a failing assertion is not an env failure")
	}
}

func TestValidateAllowsTargetsImportOnly(t *testing.T) {
	mk := func(imp string) GeneratedFile {
		return GeneratedFile{Path: "tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts",
			Content: "import { test } from '@playwright/test';\nimport { x } from '" + imp + "';\ntest('TC-VSYS-001 ok', async () => {});\n"}
	}
	o := ValidateOptions{Code: "TC-VSYS-001"}
	if err := Validate(mk("@config/targets"), o); err != nil {
		t.Fatalf("@config/targets must be allowed: %v", err)
	}
	if err := Validate(mk("@config/env"), o); err == nil {
		t.Fatal("@config/env must stay forbidden")
	}
}

func TestSpecTitle(t *testing.T) {
	src := "test('TC-VSYS-001 the liveness probe answers 200', async () => {});"
	if got := specTitle(src, "TC-VSYS-001"); got != "the liveness probe answers 200" {
		t.Fatalf("got %q", got)
	}
	if specTitle("nothing", "TC-VSYS-001") != "" {
		t.Fatal("no test means empty")
	}
}
