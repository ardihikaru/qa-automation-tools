package api

// GAOS_AUTOMATION.md: the two GAOS routes. The bearer guard needs no database; the rest runs on
// a scratch schema (QA_TEST_MYSQL_DSN) and skips without one.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/ai"
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
	if job["job_id"] != created.JobID || job["job_type"] != "case_create" {
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

	// A job that is already active for the case: 409, and the answer names it.
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-active','case_run','running','{"product_id":1,"case_code":"TC-AUTH-001"}', NOW(3))`)
	w = inboundCall(r, "POST", "/api/v1/gaos/automation", bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"create"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("an active job: got %d, want 409 (%s)", w.Code, w.Body)
	}
	var busy map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &busy); err != nil || busy["job_id"] == "" {
		t.Errorf("the 409 answer has no job id: %s (%v)", w.Body, err)
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
