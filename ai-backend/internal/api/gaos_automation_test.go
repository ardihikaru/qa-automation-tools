package api

// GAOS_AUTOMATION.md: the two GAOS routes. The bearer guard needs no database; the rest runs on
// a scratch schema (QA_TEST_MYSQL_DSN) and skips without one.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// The bearer guard of the two GAOS automation routes: fail-closed, constant time, no session.
func TestGAOSAutomationRoutesAreBearerGuarded(t *testing.T) {
	if !IsGAOSInboundPath("/api/v1/gaos/automation") || !IsGAOSInboundPath("/api/v1/gaos/jobs/job-1") {
		t.Fatal("the GAOS automation paths must bypass the cookie")
	}
	r := NewRouter(nil, nil, nil, nil)
	// The token unset: 503, the routes stay closed.
	for _, p := range []string{"/api/v1/gaos/automation", "/api/v1/gaos/jobs/job-1"} {
		if w := inboundCall(r, "POST", p, "Bearer x", `{}`); w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: token unset: got %d, want 503", p, w.Code)
		}
	}
	r.SetGAOSInboundToken(inboundTok)
	for _, h := range []string{"", "Bearer ", "Bearer wrong", inboundTok} {
		if w := inboundCall(r, "POST", "/api/v1/gaos/automation", h, `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: got %d, want 401", h, w.Code)
		}
	}
	// The right token but no database: 503, never a panic.
	if w := inboundCall(r, "POST", "/api/v1/gaos/automation", "Bearer "+inboundTok, `{}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no db: got %d, want 503", w.Code)
	}
}

func TestGAOSAutomationMode(t *testing.T) {
	if a, ok := gaosCaseAction("create"); !ok || a != "create-automation" {
		t.Errorf("create: %q %v", a, ok)
	}
	if a, ok := gaosCaseAction("run"); !ok || a != "run" {
		t.Errorf("run: %q %v", a, ok)
	}
	for _, m := range []string{"", "CREATE", "create-automation", "delete", "run "} {
		if _, ok := gaosCaseAction(m); ok {
			t.Errorf("mode %q was accepted", m)
		}
	}
}

// gaosAutomationFixture is a GAOS-integrated product with one case and a configured instance
// (executor, repo pool, AI), so the shared enqueue path really starts a job.
func gaosAutomationFixture(t *testing.T) (*Router, *db.DB) {
	t.Helper()
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	r.SetGAOSInboundToken(inboundTok)
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1'),(2,'OTHER','r2','s2','d2')`)
	must(t, d, `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (1,1,'p-6'),(2,0,'p-7')`)
	must(t, d, `INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_automation) VALUES (1,'auto','u','auto-dir',1,1)`)
	addCase(t, d, "TC-AUTH-001", "AUTH", "planned", "", "P0", `[]`)
	exec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(exec.Close)
	r.SetExecutor(executor.New(executor.Config{URL: exec.URL, Token: "t"}), time.Minute)
	r.SetRepoPool(repopool.New(t.TempDir()))
	r.SetCaseAutomation(CaseAutomationConfig{PoolHostDir: "/pool"})
	r.aiService = ai.NewFromConfig(config.AIConfig{Provider: "ai-manager", ManagerURL: "http://127.0.0.1:1",
		ManagerLoginID: "x", ManagerSecretKey: "y", ManagerProvider: "claude-v3"})
	if !r.aiService.Configured() {
		t.Skip("the fake AI manager did not count as configured")
	}
	return r, d
}

func TestGAOSAutomationRoutes(t *testing.T) {
	r, d := gaosAutomationFixture(t)
	bearer := "Bearer " + inboundTok

	// An unknown or unflagged product: 404.
	for _, p := range []string{"p-7", "p-999", "nonsense"} {
		w := inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"`+p+`","caseCode":"TC-AUTH-001","mode":"create"}`)
		if w.Code != http.StatusNotFound {
			t.Errorf("product %q: got %d, want 404 (%s)", p, w.Code, w.Body)
		}
	}
	// An unknown case: 404.
	w := inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"TC-AUTH-999","mode":"create"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown case: got %d, want 404 (%s)", w.Code, w.Body)
	}
	// A bad body or an unknown mode: 422.
	for _, body := range []string{
		`{`,
		`{"product":"p-6","caseCode":"TC-AUTH-001"}`,
		`{"product":"p-6","caseCode":"TC-AUTH-001","mode":"delete"}`,
		`{"product":"p-6","caseCode":"TC-AUTH-001","mode":"CREATE"}`,
	} {
		w := inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, body)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("body %s: got %d, want 422 (%s)", body, w.Code, w.Body)
		}
	}
	// The wrong method: 405.
	if w := inboundCall(r, "GET", "/api/v1/gaos/automation", bearer, ``); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET automation: got %d, want 405", w.Code)
	}
	if w := inboundCall(r, "POST", "/api/v1/gaos/jobs/job-1", bearer, `{}`); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST job: got %d, want 405", w.Code)
	}

	// A create: 202 with a job id.
	w = inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"create"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create: got %d, want 202 (%s)", w.Code, w.Body)
	}
	var created struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.JobID == "" {
		t.Fatalf("create answered %s (%v)", w.Body, err)
	}

	// The job status route answers the documented shape.
	w = inboundCall(r, "GET", "/api/v1/gaos/jobs/"+created.JobID, bearer, ``)
	if w.Code != http.StatusOK {
		t.Fatalf("job status: got %d, want 200 (%s)", w.Code, w.Body)
	}
	var job map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"job_id", "status", "phase", "job_type", "error", "report"} {
		if _, ok := job[k]; !ok {
			t.Errorf("the job answer has no %q: %s", k, w.Body)
		}
	}
	if job["job_id"] != created.JobID || job["job_type"] != "create" {
		t.Errorf("the job answer is %s", w.Body)
	}
	rep, _ := job["report"].(map[string]any)
	for _, k := range []string{"passed", "failed", "skipped"} {
		if _, ok := rep[k]; !ok {
			t.Errorf("the report has no %q: %s", k, w.Body)
		}
	}
	// An unknown job: 404.
	if w := inboundCall(r, "GET", "/api/v1/gaos/jobs/job-unknown", bearer, ``); w.Code != http.StatusNotFound {
		t.Errorf("unknown job: got %d, want 404", w.Code)
	}

	// A job that is already active for the case: 200 with that job's id (the dedupe).
	// (the create above is still queued or running, so it is the active one).
	_ = d
	w = inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"create"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("an active job: got %d, want 200 (%s)", w.Code, w.Body)
	}
	var busy map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &busy); err != nil || busy["job_id"] != created.JobID {
		t.Errorf("the dedupe answer is not the active job: %s (%v)", w.Body, err)
	}
}

func TestGAOSStatusMapping(t *testing.T) {
	for in, want := range map[jobStatus]string{jobStatusQueued: "queued", jobStatusRunning: "running", jobStatusCompleted: "succeeded", jobStatusFailed: "failed"} {
		if got := gaosStatus(in); got != want {
			t.Errorf("gaosStatus(%s) = %s, want %s", in, got, want)
		}
	}
}

// "*" is valid only for run; for a product with no automated case nothing is started; an active
// run-all is returned, not duplicated.
func TestGAOSRunAll(t *testing.T) {
	r, d := gaosAutomationFixture(t)
	bearer := "Bearer " + inboundTok
	w := inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"*","mode":"create"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("create *: got %d, want 422 (%s)", w.Code, w.Body)
	}
	w = inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"*","mode":"run"}`)
	if w.Code != http.StatusConflict {
		t.Errorf("run * with no automated case: got %d, want 409 (%s)", w.Code, w.Body)
	}
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-all','gaos_run_all','running','{"product_id":1,"case_code":"*","phase":"run TC-AUTH-001 (1/2)"}', NOW(3))`)
	w = inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"*","mode":"run"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"job-all"`) {
		t.Errorf("active run-all: got %d %s, want 200 job-all", w.Code, w.Body)
	}
	w = inboundCall(r, "GET", "/api/v1/gaos/jobs/job-all", bearer, ``)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"phase":"run TC-AUTH-001 (1/2)"`) || !strings.Contains(w.Body.String(), `"status":"running"`) {
		t.Errorf("run-all status: %d %s", w.Code, w.Body)
	}
}

// The shim calls the Machine path only through enqueueCaseAction / the job records: it must not
// reach into caseauto internals or start the Machine itself.
func TestGAOSShimUsesOnlyThePublicPath(t *testing.T) {
	src, err := os.ReadFile("gaos_automation.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"caseauto.NewMachine", "caseauto.Machine", "runCaseCreate", "runExecutorJob", "NewMachine"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("gaos_automation.go reaches past enqueueCaseAction: %q", banned)
		}
	}
}

// An instance that is not configured for case automation answers 503, not 202; a case without a
// spec cannot be run.
func TestGAOSAutomationWithoutConfiguration(t *testing.T) {
	r, _ := gaosAutomationFixture(t)
	bearer := "Bearer " + inboundTok
	// A run without a spec is refused by the shared path.
	w := inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"run"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("run without a spec: got %d, want 409 (%s)", w.Code, w.Body)
	}
	// Without the executor, the repo pool and the host pool dir: 503.
	r.executor = nil
	r.repoPool = nil
	r.caseAuto.hostPool = ""
	for _, mode := range []string{"create", "run"} {
		w := inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"`+mode+`"}`)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("mode %s: got %d, want 503 (%s)", mode, w.Code, w.Body)
		}
	}
}

// A running create whose newest approval is open is awaiting_approval; answered or absent, running.
func TestGAOSJobAwaitingApproval(t *testing.T) {
	r, d := gaosAutomationFixture(t)
	bearer := "Bearer " + inboundTok
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-w','case_create','running','{"product_id":1,"case_code":"TC-AUTH-001"}', NOW(3))`)
	get := func() map[string]any {
		w := inboundCall(r, "GET", "/api/v1/gaos/jobs/job-w", bearer, ``)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d %s", w.Code, w.Body)
		}
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := get(); m["status"] != "running" || m["waiting_s"] != nil {
		t.Errorf("no approval: %v", m)
	}
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at) VALUES ('job-w',1,'TC-AUTH-001','ext-1','open', DATE_SUB(UTC_TIMESTAMP(), INTERVAL 90 SECOND))`)
	m := get()
	if m["status"] != "awaiting_approval" {
		t.Fatalf("open approval: %v", m)
	}
	if ws, _ := m["waiting_s"].(float64); ws < 89 || ws > 200 {
		t.Errorf("waiting_s = %v, want about 90", m["waiting_s"])
	}
	must(t, d, `UPDATE case_approvals SET status = 'approved' WHERE job_id = 'job-w'`)
	if m := get(); m["status"] != "running" || m["waiting_s"] != nil {
		t.Errorf("answered approval: %v", m)
	}
	// A finished job is never awaiting, whatever its approvals say.
	must(t, d, `UPDATE case_approvals SET status = 'open' WHERE job_id = 'job-w'`)
	must(t, d, `UPDATE job_runs SET status = 'failed' WHERE id = 'job-w'`)
	if m := get(); m["status"] != "failed" {
		t.Errorf("failed job: %v", m)
	}
}

func TestGAOSCancelGuardAndShape(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	r.SetGAOSInboundToken(inboundTok)
	for _, h := range []string{"", "Bearer wrong"} {
		if w := inboundCall(r, "POST", "/api/v1/gaos/jobs/job-1/cancel", h, ``); w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: got %d, want 401", h, w.Code)
		}
	}
	// The right token but no database: 503, never a panic.
	if w := inboundCall(r, "POST", "/api/v1/gaos/jobs/job-1/cancel", "Bearer "+inboundTok, ``); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no db: got %d, want 503", w.Code)
	}
}

// A cancel rejects the open approval (source GAOS), fails the job, and a late approve is refused.
func TestGAOSCancel(t *testing.T) {
	r, d := gaosAutomationFixture(t)
	bearer := "Bearer " + inboundTok
	cancel := func(id string) (int, map[string]any) {
		w := inboundCall(r, "POST", "/api/v1/gaos/jobs/"+id+"/cancel", bearer, ``)
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	if w := inboundCall(r, "GET", "/api/v1/gaos/jobs/job-1/cancel", bearer, ``); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET cancel: got %d, want 405", w.Code)
	}
	if code, _ := cancel("job-nope"); code != http.StatusNotFound {
		t.Errorf("unknown job: %d", code)
	}
	// An ended job is answered as it is.
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-done','case_create','completed','{}', NOW(3))`)
	if code, m := cancel("job-done"); code != 200 || m["canceled"] != false || m["status"] != "succeeded" {
		t.Errorf("ended job: %d %v", code, m)
	}
	// A job awaiting approval.
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-w','case_create','running','{"product_id":1,"case_code":"TC-AUTH-001"}', NOW(3))`)
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at) VALUES ('job-w',1,'TC-AUTH-001','qa-job-w-pr','open', UTC_TIMESTAMP())`)
	// Half the job is in memory (its goroutine's record), as for a live job.
	r.jobs.mu.Lock()
	r.jobs.jobs["job-w"] = &jobRecord{ID: "job-w", Type: "case_create", Status: jobStatusRunning, Payload: map[string]interface{}{}}
	r.jobs.mu.Unlock()
	stopped := false
	gaosJobCancels.Store("job-w", context.CancelFunc(func() { stopped = true }))
	code, m := cancel("job-w")
	if code != 200 || m["canceled"] != true || m["status"] != "failed" || m["error"] != "canceled by GAOS" || m["job_id"] != "job-w" {
		t.Fatalf("cancel: %d %v", code, m)
	}
	if !stopped {
		t.Error("the job's context was not canceled")
	}
	store := &caseauto.ApprovalStore{DB: d}
	ap, err := store.Get(context.Background(), "qa-job-w-pr")
	if err != nil || ap.Status != caseauto.ApprovalRejected {
		t.Fatalf("approval after cancel: %+v %v", ap, err)
	}
	// The late approve changes nothing.
	if ok, err := store.Answer(context.Background(), "qa-job-w-pr", caseauto.ApprovalApproved, "dashboard", ""); ok || err != nil {
		t.Errorf("late approve: changed=%v err=%v", ok, err)
	}
	if ap, _ := store.Get(context.Background(), "qa-job-w-pr"); ap.Status != caseauto.ApprovalRejected {
		t.Errorf("late approve reopened it: %s", ap.Status)
	}
	// The job's own goroutine finishing later cannot revive it.
	r.finishJob("job-w", jobStatusCompleted, "", nil)
	w := inboundCall(r, "GET", "/api/v1/gaos/jobs/job-w", bearer, ``)
	if !strings.Contains(w.Body.String(), `"status":"failed"`) || !strings.Contains(w.Body.String(), "canceled by GAOS") {
		t.Errorf("job after a late finish: %s", w.Body)
	}
	// Idempotent: a second cancel changes nothing.
	if code, m := cancel("job-w"); code != 200 || m["canceled"] != false || m["status"] != "failed" {
		t.Errorf("second cancel: %d %v", code, m)
	}
	// A job known only to the database (after a restart) is canceled in its row.
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-db','case_run','running','{}', NOW(3))`)
	if code, m := cancel("job-db"); code != 200 || m["canceled"] != true {
		t.Errorf("db-only cancel: %d %v", code, m)
	}
	var st, er string
	if err := d.QueryRow(context.Background(), `SELECT status, error FROM job_runs WHERE id='job-db'`).Scan(&st, &er); err != nil || st != "failed" || er != "canceled by GAOS" {
		t.Errorf("db-only row: %s %s %v", st, er, err)
	}
}
