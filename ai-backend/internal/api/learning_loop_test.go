package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/learnloop"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// LEARNING_LOOP.md. Route checks run without a database; the loop itself runs on a scratch
// schema (QA_TEST_MYSQL_DSN) against a temporary bare origin and a fake `gh` on PATH. No real
// GitHub, no real LLM.

func TestLearningRoutesWithoutDatabase(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	for _, p := range []string{"/api/learning/findings", "/api/learning/proposals", "/api/learning/timeline"} {
		if w := dispatchAs(r, asUser("QA"), http.MethodGet, p, ""); w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodGet, "/api/learning/nope", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown: %d", w.Code)
	}
}

type learnLLM struct {
	mu      sync.Mutex
	replies []string
	calls   int
}

func (l *learnLLM) ChatMessages(_ context.Context, _ []models.Message, _ ai.ChatOpts) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if len(l.replies) == 0 {
		return "", errors.New("no reply")
	}
	r := l.replies[0]
	l.replies = l.replies[1:]
	return r, nil
}

func lgit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// learnFixture: product 1 with automation repo "auto" (a clone of a local bare origin) in a pool,
// a fake gh whose `pr view` answer is the content of <dir>/view.json.
func learnFixture(t *testing.T, replies ...string) (*Router, *db.DB, *learnLLM, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	d := testdb.New(t)
	tmp := t.TempDir()
	origin := filepath.Join(tmp, "origin.git")
	lgit(t, tmp, "init", "--bare", origin)
	seed := filepath.Join(tmp, "seed")
	lgit(t, tmp, "clone", origin, seed)
	if err := os.WriteFile(filepath.Join(seed, "AGENTS.md"), []byte("# AGENTS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lgit(t, seed, "add", ".")
	lgit(t, seed, "commit", "-m", "init")
	lgit(t, seed, "branch", "-M", "main")
	lgit(t, seed, "push", "origin", "main")
	root := filepath.Join(tmp, "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	lgit(t, root, "clone", origin, "auto")

	ghDir := filepath.Join(tmp, "ghbin")
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$@\" >> \"" + ghDir + "/args\"\ncase \"$2\" in\n  create) echo https://github.com/o/r/pull/5 ;;\n  view) cat \"" + ghDir + "/view.json\" ;;\n  edit) exit 0 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(ghDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghDir, "view.json"), []byte(`{"state":"OPEN","mergedAt":null,"mergeCommit":null}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir, branch) VALUES (1,'SUT','https://github.com/o/r','https://staging.example','auto','main'), (2,'OTHER','r2','s2','d2','main')`)
	must(t, d, `INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_automation) VALUES (1,'auto','https://github.com/o/r','auto',1,1)`)
	llm := &learnLLM{replies: replies}
	r := NewRouter(d, nil, nil, nil)
	r.SetRepoPool(repopool.New(root))
	r.learn.llm = llm
	r.learn.approvalPoll = 20 * time.Millisecond
	return r, d, llm, origin, ghDir
}

func addLearnJob(t *testing.T, d *db.DB, id, status, errText, caseCode string, at time.Time) {
	t.Helper()
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, error, created_at, completed_at) VALUES (?,?,?,?,?,?,?)`,
		id, "case_create", status, `{"product_id":1,"case_code":"`+caseCode+`"}`, errText, at, at)
}

func addEvent(t *testing.T, d *db.DB, job string, seq int, level, step, msg string, at time.Time) {
	t.Helper()
	must(t, d, "INSERT INTO job_events (job_id, seq, `at`, level, step, message) VALUES (?,?,?,?,?,?)", job, seq, at, level, step, msg)
}

func learnCount(t *testing.T, d *db.DB, q string, a ...any) int {
	t.Helper()
	var n int
	if err := d.QueryRow(context.Background(), q, a...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func decodeBody(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func proposalState(t *testing.T, d *db.DB, id string) Proposal {
	t.Helper()
	ps, err := loadProposals(context.Background(), d, 0, id)
	if err != nil || len(ps) != 1 {
		t.Fatalf("%v %d", err, len(ps))
	}
	return ps[0]
}

func TestHarvestIsIdempotentAndDeduplicates(t *testing.T) {
	r, d, _, _, _ := learnFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	// (a) of 2026-11-20: a reviewer asked for changes on two cases with the same lesson.
	for i, c := range []string{"TC-VSYS-003", "TC-VSYS-007"} {
		id := "job-a" + string(rune('1'+i))
		addLearnJob(t, d, id, "completed", "", c, now)
		must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, answered_by, answer_text, created_at, answered_at) VALUES (?,1,?,?,'changes','dashboard',?,?,?)`,
			id, c, "qa-"+id+"-pr", c+": the URL doubles /api/v1 because SUT_VENDOR_BACKEND_URL already ends in /api/v1", now, now)
	}
	// A rejection without a reason teaches nothing.
	addLearnJob(t, d, "job-r", "failed", "rejected by dashboard", "TC-A-001", now)
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at, answered_at) VALUES ('job-r',1,'TC-A-001','qa-job-r-pr','rejected',?,?)`, now, now)
	// A repair that fixed a failure.
	addLearnJob(t, d, "job-f", "completed", "", "TC-VSYS-005", now)
	addEvent(t, d, "job-f", 1, "warn", "run", "the spec did not pass: TC-VSYS-005 failed\nError: expect(received).not.toContain(expected) // goroutine", now)
	addEvent(t, d, "job-f", 2, "info", "run", "the spec passed", now)
	// The same refused reply in three jobs.
	for _, id := range []string{"job-x1", "job-x2", "job-x3"} {
		addLearnJob(t, d, id, "failed", "", "TC-B-00"+id[len(id)-1:], now)
		addEvent(t, d, id, 1, "warn", "generate", "the reply was refused: the spec destructures fixture vendorBackendRest, which is not declared", now)
	}
	// An old approval outside the window.
	addLearnJob(t, d, "job-old", "completed", "", "TC-A-009", now.Add(-40*24*time.Hour))
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, answer_text, created_at, answered_at) VALUES ('job-old',1,'TC-A-009','qa-job-old-pr','changes','an old lesson nobody remembers',?,?)`,
		now.Add(-40*24*time.Hour), now.Add(-40*24*time.Hour))

	r.learnTick(ctx)
	if n := learnCount(t, d, `SELECT COUNT(*) FROM learning_findings`); n != 3 {
		var rows []string
		items, _, _, lerr := learnloop.ListFindings(ctx, d, learnloop.FindingFilter{})
		rows = append(rows, fmt.Sprint(lerr))
		for _, f := range items {
			rows = append(rows, f.Kind+": "+f.Title)
		}
		t.Fatalf("findings %d:\n%s", n, strings.Join(rows, "\n"))
	}
	var occ int
	if err := d.QueryRow(ctx, `SELECT occurrences FROM learning_findings WHERE kind = 'review_changes'`).Scan(&occ); err != nil || occ != 2 {
		t.Fatalf("the two reviews must be one finding seen twice: %d %v", occ, err)
	}
	if n := learnCount(t, d, `SELECT COUNT(*) FROM learning_findings WHERE kind = 'repeated_failure' AND occurrences = 3`); n != 1 {
		t.Fatalf("repeated failure: %d", n)
	}
	if n := learnCount(t, d, `SELECT COUNT(*) FROM learning_findings WHERE kind = 'repair_fixed' AND detail LIKE '%goroutine%'`); n != 1 {
		t.Fatalf("repair: %d", n)
	}
	evidence := learnCount(t, d, `SELECT COUNT(*) FROM learning_evidence`)
	// A second harvest changes nothing.
	r.learn.lastHarvest = time.Time{}
	r.learnTick(ctx)
	if n := learnCount(t, d, `SELECT COUNT(*) FROM learning_evidence`); n != evidence {
		t.Fatalf("re-harvest added evidence: %d -> %d", evidence, n)
	}
	if n := learnCount(t, d, `SELECT SUM(occurrences) FROM learning_findings`); n != 6 {
		t.Fatalf("re-harvest bumped occurrences: %d", n)
	}

	// The API: filter by product and kind; product 2 has none.
	w := dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/learning/findings?product_id=1&kind=review_changes", "")
	var page struct {
		Items  []learnloop.Finding `json:"items"`
		Total  int                 `json:"total"`
		Counts map[string]int      `json:"counts"`
	}
	decodeBody(t, w.Body.Bytes(), &page)
	if w.Code != 200 || page.Total != 1 || page.Items[0].Occurrences != 2 || len(page.Items[0].Sources) != 2 || page.Counts["new"] != 1 || page.Items[0].ProductName != "SUT" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/learning/findings", "")
	decodeBody(t, w.Body.Bytes(), &page)
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("all products: %s", w.Body)
	}
	w = dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/learning/findings?product_id=2", "")
	decodeBody(t, w.Body.Bytes(), &page)
	if page.Total != 0 {
		t.Fatalf("product filter: %s", w.Body)
	}
	if w := dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/learning/findings?status=bogus", ""); w.Code != 400 {
		t.Errorf("bad status: %d", w.Code)
	}
}

func TestManualFindingsDismissReopenAndRoles(t *testing.T) {
	r, d, _, _, _ := learnFixture(t)
	body := `{"product_id":1,"title":"Path traversal needs a raw HTTP request","detail":"Playwright's WHATWG URL layer decodes %2e%2e and normalises dot segments, so path-traversal cases need a raw HTTP request (TC-VSYS-008 left planned).","case_code":"TC-VSYS-008"}`
	if w := dispatchAs(r, asUser("Viewer"), http.MethodPost, "/api/learning/findings", body); w.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d", w.Code)
	}
	w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/findings", body)
	var out struct {
		Finding learnloop.Finding `json:"finding"`
		Merged  bool              `json:"merged"`
	}
	decodeBody(t, w.Body.Bytes(), &out)
	if w.Code != 201 || out.Merged || out.Finding.Kind != "manual" || out.Finding.CreatedBy != "u1" || out.Finding.Title != "Path traversal needs a raw HTTP request" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// The same lesson again is merged.
	w = dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/findings", body)
	decodeBody(t, w.Body.Bytes(), &out)
	if !out.Merged || out.Finding.Occurrences != 2 {
		t.Fatalf("not merged: %s", w.Body)
	}
	for _, bad := range []string{`{"product_id":9,"title":"t","detail":"d"}`, `{"product_id":1,"title":"","detail":"d"}`, `{"product_id":1,"title":"t","detail":"d","case_code":"nope"}`, `{"product_id":1,"title":"t","detail":"d","extra":1}`} {
		if w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/findings", bad); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	id := out.Finding.ID
	p := "/api/learning/findings/" + itoa64(id)
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, p+"/reopen", ""); w.Code != http.StatusConflict {
		t.Errorf("reopen a new finding: %d", w.Code)
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, p+"/dismiss", `{"reason":"a tooling quirk"}`); w.Code != 200 {
		t.Fatalf("dismiss: %d %s", w.Code, w.Body)
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, p+"/reopen", ""); w.Code != 200 {
		t.Fatalf("reopen: %d", w.Code)
	}
	w = dispatchAs(r, asUser("Viewer"), http.MethodGet, p, "")
	var one struct {
		Finding  learnloop.Finding    `json:"finding"`
		Evidence []learnloop.Evidence `json:"evidence"`
	}
	decodeBody(t, w.Body.Bytes(), &one)
	if one.Finding.Status != "new" || len(one.Evidence) != 2 || one.Evidence[0].CaseCode != "TC-VSYS-008" {
		t.Fatalf("%s", w.Body)
	}
	if w := dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/learning/findings/999999", ""); w.Code != 404 {
		t.Errorf("missing: %d", w.Code)
	}
	_ = d
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

const learnPolicy = "Build REST URLs from restServices['sut-vendor-backend'].baseURL. It already ends in /api/v1: never append /api/v1 again."

func draftReply(slug, title string) string {
	b, _ := json.Marshal(map[string]string{"decision": "new", "slug": slug, "title": title, "policy": learnPolicy, "rationale": "A reviewer asked for changes on a doubled prefix."})
	return string(b)
}

func manualFinding(t *testing.T, r *Router, title string) int64 {
	t.Helper()
	w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/findings", `{"product_id":1,"title":"`+title+`","detail":"`+title+` explained in enough words to be a lesson"}`)
	var out struct {
		Finding learnloop.Finding `json:"finding"`
	}
	decodeBody(t, w.Body.Bytes(), &out)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	return out.Finding.ID
}

func TestProposalLoopEndToEnd(t *testing.T) {
	r, d, llm, origin, ghDir := learnFixture(t, draftReply("vendor-url-base", "Build vendor URLs from the registry base URL"))
	ctx := context.Background()
	fid := manualFinding(t, r, "Doubled api v1 prefix")
	if w := dispatchAs(r, asUser("Viewer"), http.MethodPost, "/api/learning/proposals", `{"product_id":1,"finding_ids":[1]}`); w.Code != 403 {
		t.Fatalf("viewer: %d", w.Code)
	}
	w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/proposals", `{"product_id":1,"finding_ids":[`+itoa64(fid)+`]}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var started struct {
		JobID string `json:"job_id"`
	}
	decodeBody(t, w.Body.Bytes(), &started)
	job := started.JobID
	// The finding is reserved: a second proposal from it is refused.
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/proposals", `{"product_id":1,"finding_ids":[`+itoa64(fid)+`]}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "finding_busy") {
		t.Fatalf("busy: %d %s", w.Code, w.Body)
	}
	waitFor(t, "the approval", func() bool { return proposalState(t, d, job).State == learnloop.StateAwaitingApproval })
	p := proposalState(t, d, job)
	if p.FilePath != ".claude/rules/learned-vendor-url-base.md" || !strings.Contains(p.Content, learnPolicy) || p.Title == "" {
		t.Fatalf("%+v", p)
	}
	// The job page shows the exact file under review.
	w = dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/jobs/"+job, "")
	if !strings.Contains(w.Body.String(), "# Learned policy: Build vendor URLs") {
		t.Fatalf("job page: %s", w.Body)
	}
	// Approve through the existing dashboard door.
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/jobs/"+job+"/approval", `{"decision":"approve"}`); w.Code != 200 {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	waitFor(t, "the PR", func() bool { return proposalState(t, d, job).State == learnloop.StatePROpen })
	p = proposalState(t, d, job)
	if p.PRURL != "https://github.com/o/r/pull/5" || p.FilePath != ".claude/rules/learned-vendor-url-base.md" {
		t.Fatalf("%+v", p)
	}
	// The branch on the origin holds only the new file; main is untouched.
	branch := "qa-auto/learn-" + strings.ToLower(job)
	files := lgit(t, origin, "diff", "--name-only", "main", branch)
	if files != ".claude/rules/learned-vendor-url-base.md" {
		t.Fatalf("branch diff: %q", files)
	}
	content := lgit(t, origin, "show", branch+":.claude/rules/learned-vendor-url-base.md")
	if !strings.Contains(content, learnPolicy) || !strings.Contains(content, "job="+job) || strings.Contains(content, "explained in enough words") {
		t.Fatalf("file: %s", content)
	}
	if lgit(t, origin, "ls-tree", "-r", "--name-only", "main") != "AGENTS.md" {
		t.Fatal("main changed")
	}
	args, _ := os.ReadFile(filepath.Join(ghDir, "args"))
	if !strings.Contains(string(args), "pr create --base main --head "+branch) || !strings.Contains(string(args), "Formulir Pull Request") || !strings.Contains(string(args), "--add-label ai-assisted") {
		t.Fatalf("gh args: %s", args)
	}
	if st := findingStatus(t, d, fid); st != "proposed" {
		t.Fatalf("finding %s", st)
	}
	// The PR is merged: a check makes the policy learned.
	if err := os.WriteFile(filepath.Join(ghDir, "view.json"), []byte(`{"state":"MERGED","mergedAt":"2026-11-21T10:00:00Z","mergeCommit":{"oid":"0123456789abcdef0123456789abcdef01234567"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w = dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/proposals/"+job+"/check", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"learned"`) || !strings.Contains(w.Body.String(), "0123456789abcdef") {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
	if st := findingStatus(t, d, fid); st != "learned" {
		t.Fatalf("finding %s", st)
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/proposals/"+job+"/check", ""); w.Code != http.StatusConflict {
		t.Errorf("check a learned proposal: %d", w.Code)
	}
	// The loop closes: a case_create worktree prepared from main after the merge reads the file.
	lgit(t, origin, "update-ref", "refs/heads/main", branch)
	pr := &poolRepo{r: r, d: d, repo: &automationRepo{RootDir: "auto", DefaultBranch: "main"}}
	ws, err := pr.Prepare(ctx, caseauto.Job{Branch: "qa-auto/tc-vsys-009", DefaultBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	learned, err := pr.ReadLearned(ws)
	if err != nil || len(learned) != 1 || !strings.Contains(caseauto.LearnedPolicyPart(learned[0].Content), "never append /api/v1 again") {
		t.Fatalf("%+v %v", learned, err)
	}
	// Lists, filters and the timeline.
	w = dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/learning/proposals?product_id=1&state=learned", "")
	if !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), `"learned":1`) {
		t.Fatalf("proposals: %s", w.Body)
	}
	w = dispatchAs(r, asUser("Viewer"), http.MethodGet, "/api/learning/timeline?product_id=1", "")
	for _, k := range []string{`"kind":"finding_new"`, `"kind":"proposal_created"`, `"kind":"proposal_learned"`, `"kind":"finding_learned"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Errorf("timeline lacks %s: %s", k, w.Body)
		}
	}
	if llm.calls != 1 {
		t.Errorf("LLM calls %d", llm.calls)
	}
}

func findingStatus(t *testing.T, d *db.DB, id int64) string {
	t.Helper()
	var s string
	if err := d.QueryRow(context.Background(), `SELECT status FROM learning_findings WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestProposalCoveredAndGaosRejection(t *testing.T) {
	r, d, _, _, _ := learnFixture(t,
		`{"decision":"covered","covered_by":"AGENTS.md section 2 rule 2","rationale":"typed clients only"}`,
		draftReply("leak-markers-shared", "Import the shared leak-marker list"))
	must(t, d, `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (1,1,'p-6')`)
	r.SetGAOSInboundToken(inboundTok)
	// (c): typed clients are already golden rule 2.
	f1 := manualFinding(t, r, "Specs must use typed clients not raw request get")
	w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/proposals", `{"product_id":1,"finding_ids":[`+itoa64(f1)+`]}`)
	var s1 struct {
		JobID string `json:"job_id"`
	}
	decodeBody(t, w.Body.Bytes(), &s1)
	waitFor(t, "covered", func() bool { return proposalState(t, d, s1.JobID).State == learnloop.StateCovered })
	waitFor(t, "finding covered", func() bool { return findingStatus(t, d, f1) == "covered" })
	if p := proposalState(t, d, s1.JobID); p.CoveredBy != "AGENTS.md section 2 rule 2" {
		t.Fatalf("%+v", p)
	}
	// (d): rejected in the GAOS Inbox: the webhook accepts the proposal's external id.
	f2 := manualFinding(t, r, "Leak marker lists diverged between specs")
	w = dispatchAs(r, asUser("QA"), http.MethodPost, "/api/learning/proposals", `{"product_id":1,"finding_ids":[`+itoa64(f2)+`]}`)
	var s2 struct {
		JobID string `json:"job_id"`
	}
	decodeBody(t, w.Body.Bytes(), &s2)
	waitFor(t, "the approval", func() bool { return proposalState(t, d, s2.JobID).State == learnloop.StateAwaitingApproval })
	ans := inboundCall(r, "POST", "/api/v1/gaos/inbox-answer", "Bearer "+inboundTok,
		`{"externalId":"qa-`+s2.JobID+`-pr","productId":"p-6","status":"answered","optionLabel":"Reject","text":"not now"}`)
	if ans.Code != 200 || !strings.Contains(ans.Body.String(), `"changed":true`) {
		t.Fatalf("webhook: %d %s", ans.Code, ans.Body)
	}
	waitFor(t, "rejected", func() bool { return proposalState(t, d, s2.JobID).State == learnloop.StateRejected })
	waitFor(t, "finding released", func() bool { return findingStatus(t, d, f2) == "new" })
}

func TestResumeProposalsAfterRestart(t *testing.T) {
	r, d, _, _, _ := learnFixture(t)
	f := manualFinding(t, r, "Doubled api v1 prefix again")
	draft := `{"decision":"new","slug":"vendor-url-base","title":"Stored title","policy":"` + learnPolicy + `","rationale":"r"}`
	now := time.Now().UTC()
	// A job waiting for approval when the process died, and one that was still drafting.
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at, started_at) VALUES (?,?,?,?,?,?)`, "job-wait", learnloop.JobType, "running",
		`{"product_id":1,"finding_ids":[`+itoa64(f)+`],"branch":"qa-auto/learn-job-wait","pr_url":"","draft":`+draft+`}`, now, now)
	must(t, d, `UPDATE learning_findings SET status = 'proposed', proposal_job_id = 'job-wait' WHERE id = ?`, f)
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at) VALUES ('job-wait',1,'','qa-job-wait-pr','changes',?), ('job-wait',1,'','qa-job-wait-pr2','open',?)`, now, now)
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-draft',?, 'running', '{"product_id":1,"finding_ids":[]}', ?)`, learnloop.JobType, now)
	r.FailOrphanedJobs(context.Background()) // must leave proposals alone
	if n := learnCount(t, d, `SELECT COUNT(*) FROM job_runs WHERE type = ? AND status = 'running'`, learnloop.JobType); n != 2 {
		t.Fatalf("FailOrphanedJobs touched proposals: %d running", n)
	}
	r.ResumeProposals(context.Background())
	waitFor(t, "drafting job failed", func() bool { return proposalState(t, d, "job-draft").State == learnloop.StateFailed })
	waitFor(t, "resumed wait", func() bool { return proposalState(t, d, "job-wait").State == learnloop.StateAwaitingApproval })
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/jobs/job-wait/approval", `{"decision":"approve"}`); w.Code != 200 {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	waitFor(t, "the PR", func() bool { return proposalState(t, d, "job-wait").State == learnloop.StatePROpen })
	if p := proposalState(t, d, "job-wait"); p.Title != "Stored title" {
		t.Fatalf("the stored draft was not used: %+v", p)
	}
}
