package executor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const token = "test-token-0123456789-0123456789-abcd"

func fakeExecutor(t *testing.T, pollsUntilDone int32, submitStatus int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var polls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","busy":false}`))
	})
	mux.HandleFunc("/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		if submitStatus != 0 {
			w.WriteHeader(submitStatus)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
			return
		}
		var req SubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Type != "qa_run" || req.JobID != "job-1" {
			t.Errorf("bad submit body: %+v %v", req, err)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"run-1","status":"queued","job_id":"job-1"}`))
	})
	mux.HandleFunc("/v1/runs/run-1", func(w http.ResponseWriter, r *http.Request) {
		n := polls.Add(1)
		if n < pollsUntilDone {
			_, _ = w.Write([]byte(`{"id":"run-1","status":"running"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"run-1","status":"completed","result":{"summary":{"issues":2}},"duration_ms":42}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &polls
}

func TestNewReturnsNilWithoutURL(t *testing.T) {
	if New(Config{}) != nil {
		t.Fatal("expected nil client for empty URL")
	}
}

func TestSubmitAndWait(t *testing.T) {
	srv, polls := fakeExecutor(t, 3, 0)
	c := New(Config{URL: srv.URL + "/", Token: token})
	ctx := context.Background()

	run, err := c.Submit(ctx, SubmitRequest{JobID: "job-1", Type: "qa_run", Params: map[string]interface{}{"max_steps": 3}})
	if err != nil || run.ID != "run-1" || run.Status != StatusQueued {
		t.Fatalf("submit: %+v %v", run, err)
	}
	var updates int
	final, err := c.Wait(ctx, run.ID, 5*time.Millisecond, func(*Run) { updates++ })
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != StatusCompleted || final.DurationMs != 42 || polls.Load() != 3 || updates != 3 {
		t.Fatalf("unexpected final=%+v polls=%d updates=%d", final, polls.Load(), updates)
	}
	if final.Result["summary"].(map[string]interface{})["issues"].(float64) != 2 {
		t.Fatalf("result not decoded: %+v", final.Result)
	}
}

func TestSubmitErrors(t *testing.T) {
	srv, _ := fakeExecutor(t, 1, http.StatusTooManyRequests)
	if _, err := New(Config{URL: srv.URL, Token: token}).Submit(context.Background(), SubmitRequest{JobID: "job-1", Type: "qa_run"}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("want ErrQueueFull, got %v", err)
	}
	srv2, _ := fakeExecutor(t, 1, 0)
	_, err := New(Config{URL: srv2.URL, Token: "wrong"}).Submit(context.Background(), SubmitRequest{JobID: "job-1", Type: "qa_run"})
	if err == nil || !contains(err.Error(), "HTTP 401") {
		t.Fatalf("want 401 error, got %v", err)
	}
}

func TestWaitHonoursContext(t *testing.T) {
	srv, _ := fakeExecutor(t, 1_000_000, 0)
	c := New(Config{URL: srv.URL, Token: token})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.Wait(ctx, "run-1", 5*time.Millisecond, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline error, got %v", err)
	}
}

func TestHealthCachedAndUnreachable(t *testing.T) {
	srv, _ := fakeExecutor(t, 1, 0)
	c := New(Config{URL: srv.URL, Token: token})
	if got := c.Health(context.Background()); got != "ok" {
		t.Fatalf("health = %q", got)
	}
	srv.Close()
	if got := c.Health(context.Background()); got != "ok" {
		t.Fatalf("health should be cached, got %q", got)
	}
	dead := New(Config{URL: "http://127.0.0.1:1", Token: token})
	if got := dead.Health(context.Background()); got != "unreachable" {
		t.Fatalf("health = %q", got)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// Wait sends `since` and hands each update only the events it has not seen (an executor that
// ignores `since` and resends everything must not produce duplicates).
func TestWaitAsksForNewEventsOnly(t *testing.T) {
	var sinces []string
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinces = append(sinces, r.URL.Query().Get("since"))
		switch polls.Add(1) {
		case 1:
			_, _ = w.Write([]byte(`{"id":"r","status":"running","events":[{"n":1,"level":"info","step":"run","message":"a"},{"n":2,"message":"b"}]}`))
		case 2:
			// A server that ignores since: n 1 and 2 again, plus 3.
			_, _ = w.Write([]byte(`{"id":"r","status":"running","events":[{"n":1,"message":"a"},{"n":2,"message":"b"},{"n":3,"message":"c"}]}`))
		default:
			_, _ = w.Write([]byte(`{"id":"r","status":"completed","events":[{"n":4,"message":"d"}]}`))
		}
	}))
	defer srv.Close()
	c := New(Config{URL: srv.URL, Token: token})
	var seen []int
	final, err := c.Wait(context.Background(), "r", time.Millisecond, func(r *Run) {
		for _, e := range r.Events {
			seen = append(seen, e.N)
		}
	})
	if err != nil || final.Status != StatusCompleted {
		t.Fatalf("%+v %v", final, err)
	}
	if b, _ := json.Marshal(seen); string(b) != "[1,2,3,4]" {
		t.Fatalf("events seen %s", b)
	}
	if strings.Join(sinces, "|") != "|2|3" {
		t.Fatalf("since params %q", strings.Join(sinces, "|"))
	}
}
