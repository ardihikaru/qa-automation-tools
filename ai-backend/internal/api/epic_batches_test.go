package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/epicauto"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// EPIC_AUTOMATION.md. The SQL is MySQL-specific (JSON_LENGTH, generated columns), so these
// tests run on a scratch schema (QA_TEST_MYSQL_DSN) and skip without one.

var hash64 = strings.Repeat("0", 64)

func epicFixture(t *testing.T) (*Router, *db.DB) {
	t.Helper()
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'HIS','r1','s1','d1'), (2,'OTHER','r2','s2','d2')`)
	must(t, d, `INSERT INTO kb_epics (product_id, code, content_hash) VALUES (1,'AUTH',?), (1,'PAT',?)`, hash64, hash64)
	return r, d
}

func must(t *testing.T, d *db.DB, q string, a ...any) {
	t.Helper()
	if _, err := d.Exec(context.Background(), q, a...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func addCase(t *testing.T, d *db.DB, code, epic, status, reason, pri, specs string) {
	t.Helper()
	must(t, d, `INSERT INTO kb_test_cases (product_id, code, epic_code, status, reason_category, priority, spec_paths, content_hash) VALUES (1,?,?,?,?,?,?,?)`,
		code, epic, status, reason, pri, specs, hash64)
}

func addJob(t *testing.T, d *db.DB, id, typ, status, errText, caseCode, extraPayload, result string, at time.Time) {
	t.Helper()
	payload := `{"product_id":1,"case_code":"` + caseCode + `"` + extraPayload + `}`
	var res any
	if result != "" {
		res = result
	}
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, result, error, created_at, completed_at) VALUES (?,?,?,?,?,?,?,?)`,
		id, typ, status, payload, res, errText, at, at)
}

func TestEligibleCasesAreFilteredAndOrdered(t *testing.T) {
	_, d := epicFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	addCase(t, d, "TC-AUTH-010", "AUTH", "planned", "", "P2", `[]`)
	addCase(t, d, "TC-AUTH-002", "AUTH", "planned", "", "P0", `[]`)
	addCase(t, d, "TC-AUTH-001", "AUTH", "planned", "", "P0", `[]`)
	addCase(t, d, "TC-AUTH-020", "AUTH", "planned", "", "", `[]`)
	addCase(t, d, "TC-AUTH-030", "AUTH", "planned", "", "P1", `["tests/web/a/TC-AUTH-030.spec.ts"]`) // has a spec
	addCase(t, d, "TC-AUTH-031", "AUTH", "blocked", "defect", "P0", `[]`)                            // a reason
	addCase(t, d, "TC-AUTH-032", "AUTH", "automated", "", "P0", `[]`)                                // status
	addCase(t, d, "TC-PAT-001", "PAT", "planned", "", "P0", `[]`)                                    // other epic
	addCase(t, d, "TC-AUTH-033", "AUTH", "planned", "", "P0", `[]`)                                  // GAOS says wont
	must(t, d, `INSERT INTO gaos_decisions (product_id, case_code, features, decision) VALUES (1,'TC-AUTH-033','[]','wont')`)
	addCase(t, d, "TC-AUTH-034", "AUTH", "planned", "", "P0", `[]`) // removed
	must(t, d, `UPDATE kb_test_cases SET removed_at = NOW() WHERE code = 'TC-AUTH-034'`)

	got, err := eligibleCodes(ctx, d, 1, "AUTH")
	if err != nil {
		t.Fatal(err)
	}
	if want := "TC-AUTH-001,TC-AUTH-002,TC-AUTH-010,TC-AUTH-020"; strings.Join(got, ",") != want {
		t.Fatalf("eligible = %v, want %s (P0 by code, then P2, then no priority)", got, want)
	}

	// job-based exclusions
	addJob(t, d, "j-active", "case_run", "running", "", "TC-AUTH-001", "", "", now)
	addJob(t, d, "j-pr", "case_create", "completed", "", "TC-AUTH-002", "", `{"pr_url":"https://x/pr/1"}`, now)
	addJob(t, d, "j-rej", "case_create", "failed", "rejected by u1", "TC-AUTH-010", "", "", now)
	got, _ = eligibleCodes(ctx, d, 1, "AUTH")
	if strings.Join(got, ",") != "TC-AUTH-020" {
		t.Fatalf("after an active job, a PR and a rejection eligible = %v, want only TC-AUTH-020", got)
	}
	addJob(t, d, "j-t1", "case_create", "failed", "boom", "TC-AUTH-020", "", "", now)
	if got, _ = eligibleCodes(ctx, d, 1, "AUTH"); len(got) != 1 {
		t.Fatalf("one technical failure must not exclude the case, eligible = %v", got)
	}
	addJob(t, d, "j-t2", "case_create", "failed", "boom", "TC-AUTH-020", "", "", now)
	if got, _ = eligibleCodes(ctx, d, 1, "AUTH"); len(got) != 0 {
		t.Fatalf("two technical failures must exclude the case, eligible = %v", got)
	}
}

func TestAFailureAfterAPushIsNeverRetried(t *testing.T) {
	_, d := epicFixture(t)
	addCase(t, d, "TC-AUTH-001", "AUTH", "planned", "", "P0", `[]`)
	addJob(t, d, "j1", "case_create", "failed", "gh pr create: boom", "TC-AUTH-001", "", "", time.Now().UTC())
	must(t, d, `INSERT INTO job_events (job_id, seq, at, level, step, message) VALUES ('j1',1,NOW(3),'info','push','pushed')`)
	got, err := eligibleCodes(context.Background(), d, 1, "AUTH")
	if err != nil || len(got) != 0 {
		t.Fatalf("a job that failed after a push left the case eligible: %v %v", got, err)
	}
}

func TestOnlyOneRunningBatchPerProduct(t *testing.T) {
	_, d := epicFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = createBatch(ctx, d, 1, "AUTH", []string{"TC-AUTH-001"}, "u1")
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		} else if !isDupKey(e) {
			t.Fatalf("a refused batch returned %v, want a duplicate-key error", e)
		}
	}
	if ok != 1 {
		t.Fatalf("%d concurrent batches were created, want exactly 1", ok)
	}
	// another product may have its own, and a finished batch frees the slot
	if _, err := createBatch(ctx, d, 2, "AUTH", []string{"TC-AUTH-001"}, "u1"); err != nil {
		t.Fatalf("a batch of another product was refused: %v", err)
	}
	must(t, d, `UPDATE epic_batches SET status = 'completed' WHERE product_id = 1`)
	if _, err := createBatch(ctx, d, 1, "AUTH", []string{"TC-AUTH-002"}, "u1"); err != nil {
		t.Fatalf("a new batch after the old one completed was refused: %v", err)
	}
}

func newBatch(t *testing.T, d *db.DB, codes ...string) *epicBatch {
	t.Helper()
	id, err := createBatch(context.Background(), d, 1, "AUTH", codes, "u1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := loadBatch(context.Background(), d, id)
	if err != nil || b == nil {
		t.Fatalf("load batch: %v", err)
	}
	return b
}

func batchStatus(t *testing.T, d *db.DB, id string) (string, string) {
	t.Helper()
	var st, note string
	if err := d.QueryRow(context.Background(), `SELECT status, note FROM epic_batches WHERE id = ?`, id).Scan(&st, &note); err != nil {
		t.Fatal(err)
	}
	return st, note
}

func TestAStepStopsAfterThreeFailuresAndCompletesWhenDone(t *testing.T) {
	r, d := epicFixture(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	for i, c := range []string{"TC-AUTH-001", "TC-AUTH-002", "TC-AUTH-003", "TC-AUTH-004"} {
		addCase(t, d, c, "AUTH", "planned", "", "P0", `[]`)
		_ = i
	}
	b := newBatch(t, d, "TC-AUTH-001", "TC-AUTH-002", "TC-AUTH-003", "TC-AUTH-004")
	for i, c := range []string{"TC-AUTH-001", "TC-AUTH-002", "TC-AUTH-003"} {
		addJob(t, d, "jf"+c, "case_create", "failed", "boom", c, `,"batch_id":"`+b.ID+`","batch_seq":`+string(rune('1'+i)), "", base.Add(time.Duration(i)*time.Minute))
	}
	r.epicStep(ctx, d, b)
	if st, note := batchStatus(t, d, b.ID); st != epicauto.BatchStopped || note != "3 consecutive failures" {
		t.Fatalf("after three failures the batch is %q (%q), want stopped", st, note)
	}

	// a batch whose cases all have a PR completes
	must(t, d, `UPDATE epic_batches SET status = 'cancelled' WHERE id = ?`, b.ID)
	addCase(t, d, "TC-AUTH-010", "AUTH", "planned", "", "P1", `[]`)
	b2 := newBatch(t, d, "TC-AUTH-010")
	addJob(t, d, "jpr", "case_create", "completed", "", "TC-AUTH-010", `,"batch_id":"`+b2.ID+`"`, `{"pr_url":"https://x/pr/9"}`, base)
	r.epicStep(ctx, d, b2)
	if st, _ := batchStatus(t, d, b2.ID); st != epicauto.BatchCompleted {
		t.Fatalf("a batch whose only case has a PR is %q, want completed", st)
	}
}

func TestACancelledBatchIsNeverOverwrittenByATick(t *testing.T) {
	r, d := epicFixture(t)
	ctx := context.Background()
	addCase(t, d, "TC-AUTH-001", "AUTH", "planned", "", "P0", `[]`)
	b := newBatch(t, d, "TC-AUTH-001")
	stale := *b // a tick that loaded the batch before the operator cancelled
	req := httptest.NewRequest("POST", "/api/epic-batches/"+b.ID+"/cancel", nil)
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{ID: "u1", Role: "QA"}))
	w := httptest.NewRecorder()
	r.handleEpicBatches(w, req)
	if w.Code != 200 {
		t.Fatalf("cancel answered %d %s, want 200", w.Code, w.Body)
	}
	// the cases are now all skipped/gone: a tick must not turn 'cancelled' into 'completed'
	must(t, d, `UPDATE kb_test_cases SET removed_at = NOW()`)
	r.epicStep(ctx, d, &stale)
	if st, _ := batchStatus(t, d, b.ID); st != epicauto.BatchCancelled {
		t.Fatalf("a tick that raced the cancel left the batch %q, want cancelled", st)
	}
	// cancel twice, unknown id, and a role that cannot
	w = httptest.NewRecorder()
	r.handleEpicBatches(w, req)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "bad_transition") {
		t.Fatalf("cancelling twice answered %d %s, want 409 bad_transition", w.Code, w.Body)
	}
	req2 := httptest.NewRequest("POST", "/api/epic-batches/nope/cancel", nil)
	req2 = req2.WithContext(auth.SetUser(req2.Context(), &auth.User{ID: "u1", Role: "QA"}))
	w = httptest.NewRecorder()
	r.handleEpicBatches(w, req2)
	if w.Code != 404 {
		t.Fatalf("cancelling an unknown batch answered %d, want 404", w.Code)
	}
	req3 := httptest.NewRequest("POST", "/api/epic-batches/"+b.ID+"/cancel", nil)
	req3 = req3.WithContext(auth.SetUser(req3.Context(), &auth.User{ID: "u2", Role: "Viewer"}))
	w = httptest.NewRecorder()
	r.handleEpicBatches(w, req3)
	if w.Code != 403 {
		t.Fatalf("a Viewer cancelling answered %d, want 403", w.Code)
	}
}

func TestStartingABatchValidatesBeforeItSpends(t *testing.T) {
	r, _ := epicFixture(t)
	call := func(epic, body string) (int, map[string]string) {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
		req = req.WithContext(auth.SetUser(req.Context(), &auth.User{ID: "u1", Role: "QA"}))
		w := httptest.NewRecorder()
		r.handleEpicAutomation(w, req, 1, epic, "automate", true)
		var m map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	for _, c := range []struct {
		epic, body, code string
		status           int
	}{
		{"AUTH", `{"max_cases":0}`, "bad_range", 400},
		{"AUTH", `{"max_cases":26}`, "bad_range", 400},
		{"AUTH", `{"max_cases":"x"}`, "bad_range", 400},
		{"AUTH", `{"max_cases":null}`, "bad_range", 400},
		{"AUTH", `{"max_cases":1.5}`, "bad_range", 400},
		{"AUTH", `{"max_open":3}`, "bad_range", 400},
		{"AUTH", `[1]`, "bad_range", 400},
		{"bad epic!", ``, "bad_epic", 400},
		{"NOPE", ``, "unknown_epic", 400},
		{"AUTH", ``, "no_repo", 409}, // the fixture product has no automation repo
	} {
		status, m := call(c.epic, c.body)
		if status != c.status || m["code"] != c.code {
			t.Errorf("POST automate %q %q = %d %v, want %d %s", c.epic, c.body, status, m, c.status, c.code)
		}
	}
	// a role that cannot write
	req := httptest.NewRequest("POST", "/x", nil)
	w := httptest.NewRecorder()
	r.handleEpicAutomation(w, req, 1, "AUTH", "automate", false)
	if w.Code != http.StatusForbidden {
		t.Errorf("a read-only role answered %d, want 403", w.Code)
	}
}

// A configured router (fake AI manager, fake executor, a repo pool in a temp dir) so the real
// enqueue path runs: the job must carry its batch id from its first stored write, a cancelled
// batch must refuse a new job, and an active job for the case must be adopted, not duplicated.
func TestTheEnqueuePathCarriesTheBatchAndHonoursCancel(t *testing.T) {
	r, d := epicFixture(t)
	ctx := context.Background()
	exec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer exec.Close()
	r.SetExecutor(executor.New(executor.Config{URL: exec.URL, Token: "t"}), time.Minute)
	r.SetRepoPool(repopool.New(t.TempDir()))
	r.SetCaseAutomation(CaseAutomationConfig{PoolHostDir: "/pool"})
	r.aiService = ai.NewFromConfig(config.AIConfig{Provider: "ai-manager", ManagerURL: "http://127.0.0.1:1", ManagerLoginID: "x", ManagerSecretKey: "y", ManagerProvider: "claude-v3"})
	if !r.aiService.Configured() {
		t.Skip("the fake AI manager did not count as configured")
	}
	must(t, d, `INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_automation) VALUES (1,'auto','u','auto-dir',1,1)`)
	addCase(t, d, "TC-AUTH-001", "AUTH", "planned", "", "P0", `[]`)
	addCase(t, d, "TC-AUTH-002", "AUTH", "planned", "", "P0", `[]`)
	b := newBatch(t, d, "TC-AUTH-001", "TC-AUTH-002")

	res := r.enqueueCaseAction(ctx, d, caseEnqueueIn{PID: 1, Code: "TC-AUTH-001", Action: "create-automation", TriggeredBy: "u1", BatchID: b.ID, BatchSeq: 1})
	if res.Err != nil || res.JobID == "" {
		t.Fatalf("enqueue = %+v, want a job", res)
	}
	var batchID string
	if err := d.QueryRow(ctx, `SELECT COALESCE(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.batch_id')), '') FROM job_runs WHERE id = ?`, res.JobID).Scan(&batchID); err != nil || batchID != b.ID {
		t.Fatalf("the stored job carries batch id %q (%v), want %q", batchID, err, b.ID)
	}
	// the same case again: adopted as active, not duplicated
	again := r.enqueueCaseAction(ctx, d, caseEnqueueIn{PID: 1, Code: "TC-AUTH-001", Action: "create-automation", TriggeredBy: "u1", BatchID: b.ID, BatchSeq: 1})
	if again.Err == nil || again.Err.Code != "case_active" || again.ActiveJobID != res.JobID {
		t.Fatalf("a second enqueue = %+v, want case_active naming job %s", again, res.JobID)
	}
	// cancel, then no job of the batch may start
	req := httptest.NewRequest("POST", "/api/epic-batches/"+b.ID+"/cancel", nil)
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{ID: "u1", Role: "QA"}))
	w := httptest.NewRecorder()
	r.handleEpicBatches(w, req)
	if w.Code != 200 {
		t.Fatalf("cancel answered %d %s", w.Code, w.Body)
	}
	after := r.enqueueCaseAction(ctx, d, caseEnqueueIn{PID: 1, Code: "TC-AUTH-002", Action: "create-automation", TriggeredBy: "u1", BatchID: b.ID, BatchSeq: 2})
	if after.Err == nil || after.Err.Code != "batch_not_running" {
		t.Fatalf("enqueue after cancel = %+v, want batch_not_running", after)
	}
	var n int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE JSON_UNQUOTE(JSON_EXTRACT(payload, '$.case_code')) = 'TC-AUTH-002'`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d job(s) exist for TC-AUTH-002 after the cancel, want none", n)
	}
	// a driver tick on the cancelled batch does nothing (it is not running)
	r.epicTick(ctx)
	if st, _ := batchStatus(t, d, b.ID); st != epicauto.BatchCancelled {
		t.Fatalf("batch is %q after a tick, want cancelled", st)
	}
	time.Sleep(200 * time.Millisecond) // let the background job fail before the schema is dropped
}
