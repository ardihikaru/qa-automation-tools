package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/executor"
)

func TestRunnableSpec(t *testing.T) {
	for p, want := range map[string]bool{
		"tests/web/patreg/TC-PATREG-020.spec.ts":  true,
		"tests/api/rest/auth/TC-AUTH-001.spec.ts": true,
		"tests/mq/queue/TC-MQ-001.spec.ts":        false,
		"tests/web/../mq/x.spec.ts":               false,
		"tests/web/x.ts":                          false,
		"/etc/passwd":                             false,
		"tests/web//x.spec.ts":                    false,
		"other/tests/web/x.spec.ts":               false,
	} {
		if got := runnableSpec(p); got != want {
			t.Errorf("runnableSpec(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestCaseRunParamsWhitelist(t *testing.T) {
	got := caseRunParams(map[string]interface{}{
		"repo_dir": "/r", "case_code": "TC-A-001", "spec_paths": []string{"tests/web/a.spec.ts"}, "env_file": "automation-1.env",
		"triggered_by": "u", "product_id": 1, "roles": []string{"x"}, "branch": "b",
	})
	if len(got) != 4 || got["triggered_by"] != nil || got["product_id"] != nil {
		t.Fatalf("%+v", got)
	}
}

func asUser(role string) context.Context {
	return auth.SetUser(context.Background(), &auth.User{ID: "u1", Username: "tester", Role: role})
}

func dispatchAs(r *Router, ctx context.Context, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	r.dispatchAuthenticated(w, req)
	return w
}

func TestCaseRoutesWithoutDatabase(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	// No DB: the products handler answers 503 before anything else.
	for _, p := range []string{
		"/api/products/1/knowledge/test-cases/TC-A-001/run",
		"/api/products/1/knowledge/test-cases/TC-A-001/create-automation",
	} {
		if w := dispatchAs(r, asUser("QA"), http.MethodPost, p, ""); w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	// The job routes need a DB too, and never panic without one.
	if w := dispatchAs(r, asUser("QA"), http.MethodGet, "/api/jobs/job-1/events", ""); w.Code != http.StatusServiceUnavailable {
		t.Errorf("events: %d", w.Code)
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodPost, "/api/jobs/job-1/approval", `{"decision":"approve"}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("approval: %d", w.Code)
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodGet, "/api/jobs/job-1/nope", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown sub: %d", w.Code)
	}
}

func TestApprovalPermissionAndBody(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	if w := dispatchAs(r, asUser("Viewer"), http.MethodPost, "/api/jobs/job-1/approval", `{"decision":"approve"}`); w.Code != http.StatusForbidden {
		t.Errorf("viewer: %d", w.Code)
	}
	if w := dispatchAs(r, context.Background(), http.MethodPost, "/api/jobs/job-1/approval", `{"decision":"approve"}`); w.Code != http.StatusForbidden {
		t.Errorf("no user: %d", w.Code)
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodGet, "/api/jobs/job-1/approval", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET approval: %d", w.Code)
	}
}

func TestExistingJobRoutesStillWork(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	// The legacy enqueue routes are gone.
	for _, p := range []string{"/api/jobs/qa/run", "/api/jobs/docs/generate"} {
		if w := dispatchAs(r, asUser("QA"), http.MethodPost, p, `{}`); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	if w := dispatchAs(r, asUser("QA"), http.MethodGet, "/api/jobs/job-unknown", ""); w.Code != http.StatusNotFound {
		t.Errorf("get job: %d", w.Code)
	}
}

func TestInboxAnswerRouteIsGuarded(t *testing.T) {
	if !IsGAOSInboundPath("/api/v1/gaos/inbox-answer") || !auth.IsAuthAllowList("POST", "/api/v1/gaos/inbox-answer") {
		t.Fatal("the inbox-answer path must be on both lists")
	}
	r := NewRouter(nil, nil, nil, nil)
	p := "/api/v1/gaos/inbox-answer"
	if w := inboundCall(r, "POST", p, "Bearer x", `{}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("token unset: %d", w.Code)
	}
	r.SetGAOSInboundToken(inboundTok)
	for _, h := range []string{"", "Bearer wrong"} {
		if w := inboundCall(r, "POST", p, h, `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: %d", h, w.Code)
		}
	}
	if w := inboundCall(r, "POST", p, "Bearer "+inboundTok, `{}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no db: %d", w.Code)
	}
	if w := inboundCall(r, "GET", p, "Bearer "+inboundTok, ``); w.Code != http.StatusServiceUnavailable && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
}

func TestPoolRepoConfinement(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tests/web/a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("guide"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests/web/a/TC-A-001.spec.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws := caseauto.Workspace{Dir: dir}
	p := &poolRepo{}
	if b, err := p.ReadFile(ws, "AGENTS.md"); err != nil || string(b) != "guide" {
		t.Fatalf("read: %q %v", b, err)
	}
	for _, rel := range []string{"link/secret.txt", "../x", "/etc/passwd", "a/../../x", ""} {
		if _, err := p.ReadFile(ws, rel); err == nil {
			t.Errorf("ReadFile(%q) accepted", rel)
		}
	}
	good := "tests/web/new/TC-A-002.spec.ts"
	if err := p.WriteFile(ws, good, []byte("y")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, good)); string(b) != "y" {
		t.Fatal("not written")
	}
	for _, rel := range []string{"package.json", "tests/mq/x.spec.ts", "link/x.spec.ts", "tests/web/../../x.spec.ts", ".github/workflows/x.spec.ts"} {
		if err := p.WriteFile(ws, rel, []byte("z")); err == nil {
			t.Errorf("WriteFile(%q) accepted", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "x.spec.ts")); err == nil {
		t.Fatal("wrote through a symlink")
	}
	// A symlinked spec file is never followed.
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "tests/web/a/TC-A-003.spec.ts")); err != nil {
		t.Fatal(err)
	}
	if err := p.WriteFile(ws, "tests/web/a/TC-A-003.spec.ts", []byte("z")); err == nil {
		t.Error("wrote through a file symlink")
	}
	specs, _ := p.ListSpecs(ws)
	if len(specs) < 2 {
		t.Errorf("specs %v", specs)
	}
}

// A case_run job: params go to the executor as given, executor events are forwarded, and a
// result with no test titled with the code fails the job.
func TestCaseRunJobDispatch(t *testing.T) {
	var submitted atomic.Value
	var polls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/v1/runs":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			submitted.Store(body)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"run-c","status":"queued"}`))
		case req.URL.Path == "/v1/runs/run-c":
			if polls.Add(1) < 2 {
				_, _ = w.Write([]byte(`{"id":"run-c","status":"running","events":[{"n":1,"message":"npm ci"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"run-c","status":"completed","result":{"exit_code":1,"passed":0,"failed":1,"skipped":0,"tail":"x",
				"report":{"stats":{"startTime":"2026-11-14T10:00:00Z"},"suites":[{"specs":[{"title":"TC-OTHER-001 x","tests":[{"status":"unexpected"}]}]}]}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()
	r := NewRouter(nil, nil, nil, nil)
	r.SetExecutor(executor.New(executor.Config{URL: fake.URL, Token: "t"}), time.Minute)
	job := r.enqueueJob("case_run", map[string]interface{}{
		"product_id": 1, "case_code": "TC-A-001", "spec_paths": []string{"tests/web/a/TC-A-001.spec.ts"},
		"repo_dir": "/pool/auto", "env_file": "automation-1.env", "triggered_by": "u1",
	})
	deadline := time.Now().Add(15 * time.Second)
	var rec *jobRecord
	for {
		rec, _ = r.lookupJob(context.Background(), job.ID)
		if rec != nil && (rec.Status == jobStatusCompleted || rec.Status == jobStatusFailed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never finished: %+v", rec)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rec.Status != jobStatusFailed || rec.Error != "no test titled TC-A-001 in tests/web/a/TC-A-001.spec.ts" {
		t.Fatalf("job: %+v", rec)
	}
	body := submitted.Load().(map[string]interface{})
	params := body["params"].(map[string]interface{})
	if body["type"] != "case_run" || params["repo_dir"] != "/pool/auto" || params["env_file"] != "automation-1.env" || params["triggered_by"] != nil {
		t.Fatalf("submitted %+v", body)
	}
	if _, kept := rec.Result["report"]; kept {
		t.Error("the raw report must not be kept in the job result")
	}
	if s, _ := rec.Result["summary"].(map[string]int); s["failed"] != 1 {
		t.Errorf("summary %+v", rec.Result["summary"])
	}
}

func TestExecutorEnvFileName(t *testing.T) {
	if got := executorEnvFile("", 7); got != "automation-7.env" {
		t.Errorf("default: %q", got)
	}
	if got := executorEnvFile("personal", 7); got != "automation-personal-7.env" {
		t.Errorf("personal: %q", got)
	}
	r := NewRouter(nil, nil, nil, nil)
	r.SetExecutorEnvPrefix("personal")
	if got := executorEnvFile(r.executorEnvPrefix, 1); got != "automation-personal-1.env" {
		t.Errorf("router: %q", got)
	}
}
