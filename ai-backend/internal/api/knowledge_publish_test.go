package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
)

// KNOWLEDGE_TRANSLATION.md items 8 and 9: the publish route, the drafts_publish job against a fake
// git (nothing is pushed, no PR is opened), the restart decision. DB-backed tests need QA_TEST_MYSQL_DSN.

func TestDecidePublishOrphan(t *testing.T) {
	cases := []struct {
		name string
		f    publishOrphanFacts
		want orphanAction
	}{
		{"pr exists", publishOrphanFacts{PRURL: "https://github.com/o/r/pull/1", PushReached: true}, orphanComplete},
		{"pushed, no pr", publishOrphanFacts{PushReached: true}, orphanFail},
		{"before the push", publishOrphanFacts{}, orphanRestartCreate},
		{"second attempt", publishOrphanFacts{Retries: 1}, orphanRestartCreate},
		{"retries exhausted", publishOrphanFacts{Retries: maxOrphanRetries}, orphanFail},
	}
	for _, c := range cases {
		d := decidePublishOrphan(c.f)
		if d.Action != c.want {
			t.Errorf("%s: action %d, want %d (%s)", c.name, d.Action, c.want, d.Message)
		}
		if c.want == orphanFail && !strings.HasPrefix(d.Message, orphanBaseMsg) {
			t.Errorf("%s: message %q", c.name, d.Message)
		}
	}
}

type fakeGit struct {
	mu      sync.Mutex
	calls   []string
	head    string
	tree    map[string][]byte
	wt      string
	block   chan struct{}
	pushErr error
	branch  string
}

func (g *fakeGit) rec(s string) {
	g.mu.Lock()
	g.calls = append(g.calls, s)
	g.mu.Unlock()
}
func (g *fakeGit) FetchOrigin(context.Context, string) error {
	g.rec("fetch")
	if g.block != nil {
		<-g.block
	}
	return nil
}
func (g *fakeGit) RemoteHead(context.Context, string, string) (string, error) {
	g.rec("head")
	return g.head, nil
}
func (g *fakeGit) AddWorktree(_ context.Context, _, branch, base string) (string, error) {
	g.rec("worktree " + branch + " from " + base)
	g.branch = branch
	for rel, b := range g.tree {
		p := filepath.Join(g.wt, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return "", err
		}
	}
	return g.wt, nil
}
func (g *fakeGit) RemoveWorktree(context.Context, string, string) error { g.rec("remove"); return nil }
func (g *fakeGit) Commit(_ context.Context, _, msg string, paths []string) error {
	g.rec("commit " + strings.Join(paths, ","))
	return nil
}
func (g *fakeGit) Push(_ context.Context, _, branch string) error {
	g.rec("push " + branch)
	return g.pushErr
}
func (g *fakeGit) OpenPR(_ context.Context, _, base, head, title, body string) (string, error) {
	g.rec("pr " + base + " <- " + head)
	if !strings.Contains(body, "L-C1") || !strings.Contains(body, "TC-APPT-010") {
		return "", fmt.Errorf("the PR body lacks the code mapping: %q", body)
	}
	return "https://github.com/o/auto-test/pull/5", nil
}

func (g *fakeGit) has(prefix string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, c := range g.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

type pubFix struct {
	r   *Router
	sy  *knowledge.Syncer
	git *fakeGit
	pid int
}

const goldenSHA = "5a3c1e0000000000000000000000000000000000"

func newPubFix(t *testing.T) *pubFix {
	t.Helper()
	d := newDBRouter(t).db
	r := NewRouter(d, nil, nil, nil)
	golden := loadGolden(t)
	sy := knowledge.NewSyncer(d, nil, nil)
	sy.ReadTree = func(context.Context, int, string, string, func(string) bool, int64) (string, map[string][]byte, error) {
		return goldenSHA, golden, nil
	}
	r.SetKnowledge(sy)
	p := createSplit(t, r, "https://staging.example.com")
	mustCall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"knowledge_format": knowledge.TestCatalogFormatName}, http.StatusOK)
	id, err := sy.Start(context.Background(), p.ID, knowledge.TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		var st string
		_ = d.QueryRow(context.Background(), `SELECT status FROM kb_syncs WHERE id = ?`, id).Scan(&st)
		if st == "succeeded" {
			break
		}
		if st == "failed" || i == 499 {
			t.Fatalf("sync status %q", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
	g := &fakeGit{head: goldenSHA, tree: map[string][]byte{}, wt: t.TempDir()}
	for k, v := range golden {
		g.tree[k] = v
	}
	r.pubGit = g
	return &pubFix{r: r, sy: sy, git: g, pid: p.ID}
}

func (f *pubFix) drafts(t *testing.T, scenario string) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.sy.SaveDraft(ctx, f.pid, knowledge.TypeStory, "", "", &knowledge.StoryBody{Code: "L-S1", EpicCode: "APPT", Text: "As a nurse I want to x", FeatureCodes: []string{"A01.07"}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*knowledge.TestCaseBody{
		{Code: "L-C1", EpicCode: "APPT", StoryCode: "L-S1", Scenario: scenario, Polarity: "+", Level: "API", Priority: "P1", Status: "planned"},
		{Code: "L-C2", EpicCode: "APPT", StoryCode: "US-APPT-01", Scenario: "second", Polarity: "-", Level: "WEB", Priority: "P2", Status: "planned"},
	} {
		if _, err := f.sy.SaveDraft(ctx, f.pid, knowledge.TypeTestCase, "", "", c); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *pubFix) base() string { return fmt.Sprintf("/api/products/%d/knowledge", f.pid) }

func (f *pubFix) waitJob(t *testing.T, id string) string {
	t.Helper()
	for i := 0; i < 500; i++ {
		var st string
		if err := f.r.db.QueryRow(context.Background(), `SELECT status FROM job_runs WHERE id = ?`, id).Scan(&st); err == nil && (st == "completed" || st == "failed") {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return ""
}

func TestPublishGuardRefusesNonAutomationRepo(t *testing.T) {
	r := newDBRouter(t)
	p := createSplit(t, r, "https://staging.example.com") // an AI workspace repo and a separate automation repo
	for _, repo := range p.Repos {
		err := publishGuard(context.Background(), r, repo.ID)
		if repo.IsAutomation && err != nil {
			t.Errorf("automation repo refused: %v", err)
		}
		if !repo.IsAutomation && !errors.Is(err, ErrWriteNotAllowed) {
			t.Errorf("repo %s: %v, want ErrWriteNotAllowed", repo.Name, err)
		}
	}
}

func TestPublishDraftsJob(t *testing.T) {
	f := newPubFix(t)
	f.drafts(t, "Given a booking, when the nurse saves | edits it, then it is `kept`")
	base := f.base()

	kcall(t, f.r, "Viewer", http.MethodPost, base+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusForbidden)
	kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": ""}, http.StatusBadRequest)
	kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": "NOPE"}, http.StatusBadRequest)
	kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": "APPT", "x": 1}, http.StatusBadRequest)
	if f.git.has("fetch") {
		t.Fatal("a refused request touched git")
	}
	out := kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusAccepted)
	id, _ := out["job_id"].(string)
	if id == "" {
		t.Fatalf("response %v", out)
	}
	if st := f.waitJob(t, id); st != "completed" {
		var msg string
		_ = f.r.db.QueryRow(context.Background(), `SELECT COALESCE(error, '') FROM job_runs WHERE id = ?`, id).Scan(&msg)
		t.Fatalf("job %s: %s", st, msg)
	}

	// Git: fetch, head check, a qa-auto branch from the default branch, one file committed, pushed
	// (the same branch, never the default), PR into the default branch, worktree removed.
	want := []string{"fetch", "head", "worktree qa-auto/kb-appt-", "commit docs/test-catalog/clinic-backend/appointments.md", "push qa-auto/kb-appt-", "pr main <- qa-auto/kb-appt-", "remove"}
	if len(f.git.calls) != len(want) {
		t.Fatalf("git calls %v", f.git.calls)
	}
	for i, w := range want {
		if !strings.HasPrefix(f.git.calls[i], w) {
			t.Errorf("call %d = %q, want prefix %q", i, f.git.calls[i], w)
		}
	}
	if !strings.Contains(f.git.calls[2], "from main") {
		t.Errorf("worktree call %q", f.git.calls[2])
	}
	file, _ := os.ReadFile(filepath.Join(f.git.wt, "docs/test-catalog/clinic-backend/appointments.md"))
	for _, frag := range []string{"| ☐ | TC-APPT-010 | US-APPT-06 | Given a booking, when the nurse saves \\| edits it, then it is `kept` |", "TC-APPT-011 | US-APPT-01 | second", "- **US-APPT-06** ([A01.07]"} {
		if !strings.Contains(string(file), frag) {
			t.Errorf("file lacks %q", frag)
		}
	}

	// The two endpoints the dashboard calls.
	pubs := kcall(t, f.r, "Viewer", http.MethodGet, base+"/publications", nil, http.StatusOK)["publications"].([]any)
	if len(pubs) != 1 {
		t.Fatalf("publications %v", pubs)
	}
	p := pubs[0].(map[string]any)
	codes := p["codes"].(map[string]any)
	if p["state"] != "pr_open" || p["pr_url"] != "https://github.com/o/auto-test/pull/5" || p["epic_code"] != "APPT" || p["job_id"] != id ||
		!strings.HasPrefix(p["branch"].(string), "qa-auto/kb-appt-") || codes["L-S1"] != "US-APPT-06" || codes["L-C1"] != "TC-APPT-010" || codes["L-C2"] != "TC-APPT-011" || p["id"] == nil {
		t.Fatalf("publication %v", p)
	}
	// The job page reads the mapping from /jobs/{id}/events.
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+id+"/events", nil)
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{Role: "QA"}))
	rec := httptest.NewRecorder()
	f.r.handleJobSub(rec, req, id, "events")
	var ev map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &ev)
	if ev["job_type"] != "drafts_publish" || ev["status"] != "completed" || ev["pr_url"] != "https://github.com/o/auto-test/pull/5" || ev["codes"].(map[string]any)["L-C1"] != "TC-APPT-010" {
		t.Fatalf("events: %s", rec.Body.String())
	}
	if evs, _ := ev["events"].([]any); len(evs) < 4 {
		t.Fatalf("live log has %d events", len(evs))
	}
	// The drafts are in an open publication: nothing left to publish.
	kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusUnprocessableEntity)
}

func TestPublishOnePerProductAndSyncFirst(t *testing.T) {
	f := newPubFix(t)
	f.drafts(t, "Given a, when b, then c")
	base := f.base()
	f.git.block = make(chan struct{})
	out := kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusAccepted)
	first := out["job_id"].(string)
	// Same product, any epic: 409 with the running job.
	conflict := kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusConflict)
	if conflict["job_id"] != first {
		t.Fatalf("409 body %v", conflict)
	}
	// While queued or running the drafts cannot be edited or deleted.
	code, _ := call(t, f.r, "QA", http.MethodDelete, base+"/test-cases/L-C2", nil)
	if code != http.StatusConflict {
		t.Fatalf("delete of a publishing draft: %d", code)
	}
	f.git.head = "9999999999999999999999999999999999999999" // the repo moved since the last sync
	close(f.git.block)
	if st := f.waitJob(t, first); st != "failed" {
		t.Fatalf("status %s", st)
	}
	var msg string
	_ = f.r.db.QueryRow(context.Background(), `SELECT error FROM job_runs WHERE id = ?`, first).Scan(&msg)
	if !strings.Contains(msg, "sync first") {
		t.Fatalf("error %q", msg)
	}
	if f.git.has("worktree") || f.git.has("push") || f.git.has("pr ") {
		t.Fatalf("git calls after a failed precondition: %v", f.git.calls)
	}
	pubs := kcall(t, f.r, "QA", http.MethodGet, base+"/publications", nil, http.StatusOK)["publications"].([]any)
	if pubs[0].(map[string]any)["state"] != "failed" {
		t.Fatalf("publication %v", pubs[0])
	}
	// The drafts are publishable again, and the retry makes it.
	f.git.head = goldenSHA
	f.git.calls = nil
	out = kcall(t, f.r, "QA", http.MethodPost, base+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusAccepted)
	if st := f.waitJob(t, out["job_id"].(string)); st != "completed" {
		t.Fatalf("retry %s", st)
	}
}

func TestPublishRenderRefusalCommitsNothing(t *testing.T) {
	f := newPubFix(t)
	f.drafts(t, "Given a\x01b, when c, then d") // a control character the catalog cannot carry
	out := kcall(t, f.r, "QA", http.MethodPost, f.base()+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusAccepted)
	id := out["job_id"].(string)
	if st := f.waitJob(t, id); st != "failed" {
		t.Fatalf("status %s", st)
	}
	var msg string
	_ = f.r.db.QueryRow(context.Background(), `SELECT error FROM job_runs WHERE id = ?`, id).Scan(&msg)
	if !strings.Contains(msg, "field scenario") {
		t.Fatalf("error does not name the field: %q", msg)
	}
	if f.git.has("commit") || f.git.has("push") || f.git.has("pr ") || !f.git.has("remove") {
		t.Fatalf("git calls %v", f.git.calls)
	}
	// Failed: the codes were released and the draft is publishable (after the text is fixed).
	if _, err := f.sy.SaveDraft(context.Background(), f.pid, knowledge.TypeTestCase, "L-C1", "", &knowledge.TestCaseBody{EpicCode: "APPT", StoryCode: "L-S1", Scenario: "fixed", Polarity: "+", Level: "API", Priority: "P1", Status: "planned"}); err != nil {
		t.Fatalf("edit after a failed publish: %v", err)
	}
}

func TestPublishPushFailureFailsThePublication(t *testing.T) {
	f := newPubFix(t)
	f.drafts(t, "Given a, when b, then c")
	f.git.pushErr = errors.New("remote rejected")
	out := kcall(t, f.r, "QA", http.MethodPost, f.base()+"/publish", map[string]any{"epic_code": "APPT"}, http.StatusAccepted)
	if st := f.waitJob(t, out["job_id"].(string)); st != "failed" {
		t.Fatalf("status %s", st)
	}
	if f.git.has("pr ") {
		t.Fatal("a PR was opened after a failed push")
	}
	pubs := kcall(t, f.r, "QA", http.MethodGet, f.base()+"/publications", nil, http.StatusOK)["publications"].([]any)
	if pubs[0].(map[string]any)["state"] != "failed" {
		t.Fatalf("publication %v", pubs[0])
	}
}

func TestResumeDraftPublishes(t *testing.T) {
	f := newPubFix(t)
	f.drafts(t, "Given a, when b, then c")
	set, _ := f.sy.CollectDrafts(context.Background(), f.pid, "APPT")
	ctx := context.Background()
	d := f.r.db
	add := func(id, payload string) {
		must(t, d, `INSERT INTO job_runs (id, type, status, payload) VALUES (?, 'drafts_publish', 'running', ?)`, id, payload)
	}
	// 1. A job with a PR: completed, publication pr_open.
	add("job-pr", `{"product_id":1,"epic_code":"APPT","pr_url":"https://github.com/o/r/pull/3","branch":"qa-auto/kb-appt-1"}`)
	if err := f.sy.QueuePublication(ctx, f.pid, "job-pr", "", set); err != nil {
		t.Fatal(err)
	}
	// 2. A job that reached the push step without a PR: failed, drafts released.
	add("job-push", `{"product_id":1,"epic_code":"APPT","branch":"qa-auto/kb-appt-2"}`)
	must(t, d, `INSERT INTO job_events (job_id, seq, at, level, step, message) VALUES ('job-push',1,NOW(3),'info','push','pushing')`)
	set2 := &knowledge.DraftSet{Epic: "APPT", Cases: set.Cases[:1]}
	must(t, d, `UPDATE kb_publications SET state = 'failed' WHERE job_id = 'job-pr'`) // free the drafts for the second queue
	if err := f.sy.QueuePublication(ctx, f.pid, "job-push", "", set2); err != nil {
		t.Fatal(err)
	}
	must(t, d, `UPDATE kb_publications SET state = 'running' WHERE job_id = 'job-pr'`)

	f.r.ResumeDraftPublishes(ctx)
	status := func(id string) string {
		var s string
		_ = d.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = ?`, id).Scan(&s)
		return s
	}
	if status("job-pr") != "completed" || status("job-push") != "failed" {
		t.Fatalf("job-pr %s, job-push %s", status("job-pr"), status("job-push"))
	}
	var st string
	_ = d.QueryRow(ctx, `SELECT state FROM kb_publications WHERE job_id = 'job-pr' LIMIT 1`).Scan(&st)
	if st != "pr_open" {
		t.Fatalf("publication of the job with a PR: %s", st)
	}
	_ = d.QueryRow(ctx, `SELECT state FROM kb_publications WHERE job_id = 'job-push' LIMIT 1`).Scan(&st)
	if st != "failed" {
		t.Fatalf("publication of the pushed job: %s", st)
	}
	// FailOrphanedJobs leaves the type to the resume step.
	add("job-left", `{"product_id":1,"epic_code":"APPT"}`)
	f.r.FailOrphanedJobs(ctx)
	if status("job-left") != "running" {
		t.Fatalf("FailOrphanedJobs touched a drafts_publish job: %s", status("job-left"))
	}
}
