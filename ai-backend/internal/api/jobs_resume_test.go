package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func TestDecideOrphan(t *testing.T) {
	cases := []struct {
		name string
		f    orphanFacts
		want orphanAction
		msg  string
	}{
		{"run first retry", orphanFacts{Type: "case_run"}, orphanRetryRun, "attempt 2/3"},
		{"run second retry", orphanFacts{Type: "case_run", Retries: 1}, orphanRetryRun, "attempt 3/3"},
		{"run cap", orphanFacts{Type: "case_run", Retries: 2}, orphanFail, "retries exhausted (2 of 2 used)"},
		{"create restart", orphanFacts{Type: "case_create"}, orphanRestartCreate, "retrying from the start, attempt 2/3"},
		{"create cap", orphanFacts{Type: "case_create", Retries: 2}, orphanFail, "retries exhausted"},
		{"create pr known", orphanFacts{Type: "case_create", PRURL: "https://x/pr/1", PushReached: true, Retries: 2}, orphanComplete, ""},
		{"create push reached", orphanFacts{Type: "case_create", PushReached: true}, orphanFail, "may exist"},
		{"create approved", orphanFacts{Type: "case_create", ApprovalStatus: caseauto.ApprovalApproved}, orphanResumeCreateApp, "publish step"},
		{"create changes", orphanFacts{Type: "case_create", ApprovalStatus: caseauto.ApprovalChanges}, orphanFail, "approval step"},
		{"other type", orphanFacts{Type: "docs_generate"}, orphanFail, "backend restarted"},
	}
	for _, c := range cases {
		got := decideOrphan(c.f)
		if got.Action != c.want || !strings.Contains(got.Message, c.msg) {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

// fakeExecutor serves GET /v1/runs/{known} as a completed run and counts POSTs.
func fakeExecutor(t *testing.T, known string, posts *atomic.Int32) *executor.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/v1/runs":
			posts.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"run-new","status":"queued","job_id":"job-r"}`))
		case req.URL.Path == "/v1/runs/"+known || req.URL.Path == "/v1/runs/run-new":
			_, _ = w.Write([]byte(`{"id":"` + strings.TrimPrefix(req.URL.Path, "/v1/runs/") + `","job_id":"job-r","status":"completed","result":{"exit_code":1,"passed":0,"failed":0,"skipped":0,"tail":"x"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"run not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return executor.New(executor.Config{URL: srv.URL, Token: "t"})
}

func TestSubmitOrReattach(t *testing.T) {
	ctx := context.Background()
	mk := func(retries int, runID string, c *executor.Client) *Router {
		r := NewRouter(nil, nil, nil, nil)
		r.SetExecutor(c, time.Minute)
		rec := &jobRecord{ID: "job-r", Type: "case_run", Payload: map[string]interface{}{"retries": retries}}
		if runID != "" {
			rec.Result = map[string]interface{}{"executor_run_id": runID}
		}
		r.jobs.jobs["job-r"] = rec
		return r
	}
	var posts atomic.Int32
	r := mk(1, "run-old", fakeExecutor(t, "run-old", &posts))
	run, err := r.submitOrReattach(ctx, "job-r", "case_run", nil)
	if err != nil || run.ID != "run-old" || posts.Load() != 0 {
		t.Fatalf("re-attach: %+v %v posts=%d", run, err, posts.Load())
	}

	posts.Store(0)
	r = mk(1, "run-gone", fakeExecutor(t, "run-old", &posts)) // 404 -> resubmit
	run, err = r.submitOrReattach(ctx, "job-r", "case_run", nil)
	if err != nil || run.ID != "run-new" || posts.Load() != 1 {
		t.Fatalf("resubmit: %+v %v posts=%d", run, err, posts.Load())
	}
	if got := r.jobs.jobs["job-r"].Result["executor_run_id"]; got != "run-new" {
		t.Errorf("run id not recorded: %v", got)
	}

	posts.Store(0)
	r = mk(0, "run-old", fakeExecutor(t, "run-old", &posts)) // first attempt: never re-attach
	if run, err = r.submitOrReattach(ctx, "job-r", "case_run", nil); err != nil || run.ID != "run-new" {
		t.Fatalf("first attempt: %+v %v", run, err)
	}

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer dead.Close()
	posts.Store(0)
	r = mk(1, "run-old", executor.New(executor.Config{URL: dead.URL, Token: "t"}))
	if _, err = r.submitOrReattach(ctx, "job-r", "case_run", nil); err == nil || !strings.Contains(err.Error(), "cannot re-attach") {
		t.Fatalf("executor error must fail, not resubmit: %v", err)
	}
}

func TestDrainRefusesNewCaseJobs(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	r.BeginDrain()
	if !r.caseAuto.draining.Load() {
		t.Fatal("not draining")
	}
}

// ---- DB-backed (skips without QA_TEST_MYSQL_DSN) ----

func seedJob(t *testing.T, r *Router, id, typ string, status jobStatus, payload, result map[string]interface{}) {
	t.Helper()
	now := time.Now().UTC()
	r.upsertJobRecord(context.Background(), &jobRecord{ID: id, Type: typ, Status: status, Payload: payload, Result: result, CreatedAt: now, StartedAt: &now})
}

func dbJob(t *testing.T, r *Router, id string) jobRecord {
	t.Helper()
	j, err := scanJobRow(r.getDB().QueryRow(context.Background(),
		`SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms FROM job_runs WHERE id = ?`, id))
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func waitTerminal(t *testing.T, r *Router, id string) jobRecord {
	t.Helper()
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if j := dbJob(t, r, id); j.Status == jobStatusCompleted || j.Status == jobStatusFailed {
			return j
		}
	}
	t.Fatalf("job %s never finished: %+v", id, dbJob(t, r, id))
	return jobRecord{}
}

func TestResumeOrphanedJobsAgainstMySQL(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	var posts atomic.Int32
	r := NewRouter(d, nil, nil, nil)
	r.SetExecutor(fakeExecutor(t, "run-old", &posts), time.Minute)
	r.SetCaseAutomation(CaseAutomationConfig{PoolHostDir: "/pool"})
	es := &caseauto.EventStore{DB: d, Redact: caseauto.NewRedactor()}
	aps := &caseauto.ApprovalStore{DB: d}
	base := func(code string) map[string]interface{} {
		return map[string]interface{}{"product_id": 1, "case_code": code, "spec_paths": []string{"tests/web/a/" + code + ".spec.ts"}}
	}
	with := func(m map[string]interface{}, k string, v interface{}) map[string]interface{} { m[k] = v; return m }

	// case_run that had an executor run: re-attached, never resubmitted. It ends "returned no report"
	// (the fake has no report), which proves the completion path ran on the executor's result.
	seedJob(t, r, "job-r", "case_run", jobStatusRunning, with(base("TC-A-001"), "retries", 0), map[string]interface{}{"executor_run_id": "run-old"})
	// case_run past the cap.
	seedJob(t, r, "job-cap", "case_run", jobStatusRunning, with(base("TC-A-002"), "retries", 2), nil)
	// case_create variants.
	seedJob(t, r, "job-pr", "case_create", jobStatusRunning, with(base("TC-A-003"), "pr_url", "https://example.test/pr/1"), nil)
	seedJob(t, r, "job-push", "case_create", jobStatusRunning, base("TC-A-004"), nil)
	_ = es.Add(ctx, "job-push", "info", caseauto.StepPush, "approved; pushing")
	seedJob(t, r, "job-chg", "case_create", jobStatusRunning, base("TC-A-005"), nil)
	_ = aps.Open(ctx, "job-chg", 1, "TC-A-005", "qa-job-chg-pr")
	_, _ = aps.Answer(ctx, "qa-job-chg-pr", caseauto.ApprovalChanges, "u", "x")
	seedJob(t, r, "job-open", "case_create", jobStatusRunning, base("TC-A-006"), nil)
	_ = aps.Open(ctx, "job-open", 1, "TC-A-006", "qa-job-open-pr")
	seedJob(t, r, "job-restart", "case_create", jobStatusRunning, with(base("TC-A-007"), "retries", 1), nil)
	_ = es.Add(ctx, "job-restart", "info", caseauto.StepRun, "running")
	// A job type that is never resumed.
	seedJob(t, r, "job-other", "docs_generate", jobStatusRunning, nil, nil)

	r.FailOrphanedJobs(ctx)
	if j := dbJob(t, r, "job-other"); j.Status != jobStatusFailed || j.Error != "backend restarted while the job was in progress" {
		t.Errorf("other type: %+v", j)
	}
	for _, id := range []string{"job-r", "job-cap", "job-pr", "job-push", "job-chg", "job-restart"} {
		if dbJob(t, r, id).Status != jobStatusRunning {
			t.Errorf("%s must be left for ResumeOrphanedJobs", id)
		}
	}
	if dbJob(t, r, "job-open").Status != jobStatusRunning {
		t.Error("open approval job must survive")
	}

	r.ResumeOrphanedJobs(ctx)
	r.ResumeOrphanedJobs(ctx) // idempotent: held jobs are skipped, nothing is retried twice

	if j := dbJob(t, r, "job-cap"); j.Status != jobStatusFailed || !strings.Contains(j.Error, "retries exhausted") {
		t.Errorf("cap: %+v", j)
	}
	if j := dbJob(t, r, "job-pr"); j.Status != jobStatusCompleted || j.Result["pr_url"] != "https://example.test/pr/1" {
		t.Errorf("pr: %+v", j)
	}
	if j := dbJob(t, r, "job-push"); j.Status != jobStatusFailed || !strings.Contains(j.Error, "may exist") {
		t.Errorf("push: %+v", j)
	}
	if j := dbJob(t, r, "job-chg"); j.Status != jobStatusFailed || !strings.Contains(j.Error, "approval step") {
		t.Errorf("changes: %+v", j)
	}
	if dbJob(t, r, "job-open").Status != jobStatusRunning {
		t.Error("open approval job must not be touched by ResumeOrphanedJobs")
	}
	if a, _ := aps.Latest(ctx, "job-chg"); a == nil || a.Status != caseauto.ApprovalChanges {
		t.Errorf("approvals must be untouched: %+v", a)
	}

	j := waitTerminal(t, r, "job-r")
	if posts.Load() != 0 {
		t.Errorf("re-attach must not submit again (%d POSTs)", posts.Load())
	}
	if !strings.Contains(j.Error, "returned no report") || intFrom(j.Payload["retries"]) != 1 {
		t.Errorf("job-r: %+v", j)
	}
	evs, _ := es.List(ctx, "job-r", 0, 100)
	if len(evs) < 2 || evs[0].Level != "warn" || !strings.Contains(evs[0].Message, "attempt 2/3") || evs[0].Step != caseauto.StepRun {
		t.Errorf("job-r events: %+v", evs)
	}

	// case_create restart: no AI in this router, so the machine fails at once, but the retry
	// bookkeeping and the event sequence are what is checked.
	j = waitTerminal(t, r, "job-restart")
	if intFrom(j.Payload["retries"]) != 2 {
		t.Errorf("job-restart retries: %+v", j.Payload)
	}
	evs, _ = es.List(ctx, "job-restart", 0, 100)
	if len(evs) < 3 || evs[0].Seq != 1 || evs[1].Seq != 2 || evs[1].Level != "warn" ||
		!strings.Contains(evs[1].Message, "attempt 3/3") || evs[1].Step != caseauto.StepRun {
		t.Errorf("job-restart events: %+v", evs)
	}

	// A second restart during the retry: the job is still 'running' with retries=2 -> fails.
	seedJob(t, r, "job-again", "case_run", jobStatusRunning, with(base("TC-A-008"), "retries", 2), nil)
	r2 := NewRouter(d, nil, nil, nil)
	r2.SetExecutor(fakeExecutor(t, "run-old", &posts), time.Minute)
	r2.SetCaseAutomation(CaseAutomationConfig{PoolHostDir: "/pool"})
	r2.ResumeOrphanedJobs(ctx)
	if j := dbJob(t, r, "job-again"); j.Status != jobStatusFailed || !strings.Contains(j.Error, "retries exhausted") {
		t.Errorf("again: %+v", j)
	}
}
