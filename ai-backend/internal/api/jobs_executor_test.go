package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/executor"
)

func TestExecutorParamsWhitelist(t *testing.T) {
	got := executorParams(map[string]interface{}{"triggered_by": "dashboard", "max_steps": 3.0, "roles": []interface{}{"admin"}})
	if len(got) != 2 || got["max_steps"] != 3.0 {
		t.Fatalf("top-level params: %+v", got)
	}
	got = executorParams(map[string]interface{}{"params": map[string]interface{}{"flows": []interface{}{"x"}, "url": "https://prod"}})
	if len(got) != 1 || got["url"] != nil {
		t.Fatalf("nested params must drop unknown keys: %+v", got)
	}
}

func TestQAJobDispatchesToExecutor(t *testing.T) {
	var submitted atomic.Value
	var polls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			submitted.Store(body)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"run-abc","status":"queued"}`))
		case r.URL.Path == "/v1/runs/run-abc":
			if polls.Add(1) < 2 {
				_, _ = w.Write([]byte(`{"id":"run-abc","status":"running"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"run-abc","status":"completed","result":{"summary":{"issues":1},"issues":[{"url":"https://stag/x","type":"skill","skill":"max_length","severity":"high","title":"t","message":"m"}]}}`))
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()

	router := NewRouter(nil, nil, nil, nil)
	router.SetExecutor(executor.New(executor.Config{URL: fake.URL, Token: "t"}), time.Minute)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/jobs/qa/run", strings.NewReader(`{"triggered_by":"test","max_steps":2}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("enqueue status=%d body=%s", rec.Code, rec.Body.String())
	}
	var job jobRecord
	_ = json.Unmarshal(rec.Body.Bytes(), &job)

	deadline := time.Now().Add(15 * time.Second)
	for {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/jobs/"+job.ID, nil))
		_ = json.Unmarshal(rec.Body.Bytes(), &job)
		if job.Status == jobStatusCompleted || job.Status == jobStatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never finished: %+v", job)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if job.Status != jobStatusCompleted || job.Result["executor_run_id"] != "run-abc" {
		t.Fatalf("unexpected job: %+v", job)
	}
	if job.Result["simulated"] != nil {
		t.Fatal("executor job must not be marked simulated")
	}
	body := submitted.Load().(map[string]interface{})
	params := body["params"].(map[string]interface{})
	if body["type"] != "qa_run" || body["job_id"] != job.ID || params["max_steps"] != 2.0 || params["triggered_by"] != nil {
		t.Fatalf("bad submit body: %+v", body)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if !strings.Contains(rec.Body.String(), `"executor":"ok"`) {
		t.Fatalf("health missing executor: %s", rec.Body.String())
	}
}
