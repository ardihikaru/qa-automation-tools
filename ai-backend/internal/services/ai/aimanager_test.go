package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

func newTestManager(t *testing.T, h http.HandlerFunc, timeout time.Duration) ChatProvider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewAIManagerProvider(AIManagerConfig{
		URL: srv.URL + "/", LoginID: "qa", SecretKey: "s3cret", Provider: "antigravity", Timeout: timeout,
	})
}

func TestAIManagerChatSuccess(t *testing.T) {
	p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "qa" || pass != "s3cret" {
			t.Errorf("basic auth missing or wrong: ok=%v user=%q", ok, user)
		}
		var body aiManagerChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Caller != "qa-automation" || body.Provider != "antigravity" || len(body.Messages) != 1 {
			t.Errorf("unexpected body: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "backend", "provider": "antigravity", "model": "gemini",
			"input_tokens": 12, "output_tokens": 1, "cost_usd": 0.0001,
		})
	}, 5*time.Second)

	res, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "hi"}}, ChatOpts{Temperature: 0.7})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Text != "backend" || res.Provider != "antigravity" || res.InputTokens != 12 || res.CostUSD != 0.0001 {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestAIManagerChatErrorStatuses(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{401, `{"error":"unauthorized"}`, "status 401: unauthorized"},
		{502, `{"error":"all providers failed"}`, "status 502: all providers failed"},
		{503, `not json`, "status 503: not json"},
	} {
		p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}, 5*time.Second)
		_, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "x"}}, ChatOpts{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: got err %v, want contains %q", tc.status, err, tc.want)
		}
	}
}

func TestAIManagerEmptyText(t *testing.T) {
	p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":"  "}`))
	}, 5*time.Second)
	if _, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "x"}}, ChatOpts{}); err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestAIManagerTimeout(t *testing.T) {
	p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"text":"late"}`))
	}, 50*time.Millisecond)
	if _, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "x"}}, ChatOpts{}); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestAIManagerNotConfigured(t *testing.T) {
	p := NewAIManagerProvider(AIManagerConfig{URL: "http://127.0.0.1:1"})
	if p.Configured() {
		t.Fatal("expected not configured without credentials")
	}
	if got := p.Health(context.Background()); got != "not_configured" {
		t.Errorf("health = %q", got)
	}
}

func TestAIManagerHealthCached(t *testing.T) {
	hits := 0
	p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/health" {
			hits++
			_, _ = w.Write([]byte(`{"manager_status":"ok"}`))
		}
	}, 5*time.Second)
	for i := 0; i < 3; i++ {
		if got := p.Health(context.Background()); got != "ok" {
			t.Fatalf("health = %q", got)
		}
	}
	if hits != 1 {
		t.Errorf("health probe hits = %d, want 1 (cached)", hits)
	}
}

func TestServiceChatRecordsUsage(t *testing.T) {
	p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":"Frontend.","provider":"claude","input_tokens":3,"output_tokens":1,"cost_usd":0.5}`))
	}, 5*time.Second)
	svc := NewServiceWithProvider(p, 5*time.Second)
	cat, err := svc.ClassifyIssue("TypeError: x is undefined")
	if err != nil {
		t.Fatalf("ClassifyIssue: %v", err)
	}
	if cat != "frontend" {
		t.Errorf("category = %q, want frontend (normalised)", cat)
	}
	if u := svc.LastUsage(); u.Provider != "claude" || u.CostUSD != 0.5 {
		t.Errorf("usage = %+v", u)
	}
}

func TestClassifyIssueFallsBackToHeuristicOnAIError(t *testing.T) {
	p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"all providers failed"}`))
	}, 5*time.Second)
	svc := NewServiceWithProvider(p, 5*time.Second)
	cat, err := svc.ClassifyIssue("500 Internal Server Error from /api/patients")
	if err != nil {
		t.Fatalf("ClassifyIssue returned error, want heuristic fallback: %v", err)
	}
	if cat != "backend" {
		t.Errorf("category = %q, want backend (heuristic)", cat)
	}
}

func TestAIManagerPinnedProviderNoFallback(t *testing.T) {
	var got aiManagerChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"text":"ok","provider":"claude-v3","model":"claude-opus-5"}`))
	}))
	defer srv.Close()
	p := NewAIManagerProvider(AIManagerConfig{URL: srv.URL, LoginID: "a", SecretKey: "b", Provider: "claude-v3", NoFallback: true})
	res, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "x"}}, ChatOpts{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.Provider != "claude-v3" || !got.NoFallback {
		t.Errorf("request provider=%q no_fallback=%v, want claude-v3/true", got.Provider, got.NoFallback)
	}
	if res.Provider != "claude-v3" || p.Upstream() != "claude-v3" {
		t.Errorf("result provider=%q upstream=%q", res.Provider, p.Upstream())
	}
}

func TestAIManagerRejectsFallbackReply(t *testing.T) {
	for _, body := range []string{
		`{"text":"x","provider":"antigravity","fallback":true}`,
		`{"text":"x","provider":"antigravity"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		p := NewAIManagerProvider(AIManagerConfig{URL: srv.URL, LoginID: "a", SecretKey: "b", Provider: "claude-v3", NoFallback: true})
		if _, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "x"}}, ChatOpts{}); err == nil || !strings.Contains(err.Error(), "pinned") {
			t.Errorf("body %s: err = %v, want pinned-provider error", body, err)
		}
		srv.Close()
	}
}

func TestAIManagerHealthUpstreamDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"manager_status":"ok","providers":{"antigravity":true,"claude-v3":false}}`))
	}))
	defer srv.Close()
	p := NewAIManagerProvider(AIManagerConfig{URL: srv.URL, LoginID: "a", SecretKey: "b", Provider: "claude-v3"})
	if got := p.Health(context.Background()); got != "upstream_disabled:claude-v3" {
		t.Errorf("health = %q", got)
	}
}

func TestAIManagerModelOverride(t *testing.T) {
	for _, tc := range []struct{ cfgModel, optModel, want string }{
		{"", "", ""},
		{"cfg-model", "", "cfg-model"},
		{"cfg-model", "claude-haiku-5-5", "claude-haiku-5-5"},
		{"", "claude-haiku-5-5", "claude-haiku-5-5"},
	} {
		var got aiManagerChatRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&got)
			_, _ = w.Write([]byte(`{"text":"ok","provider":"claude-v3"}`))
		}))
		p := NewAIManagerProvider(AIManagerConfig{URL: srv.URL, LoginID: "qa", SecretKey: "s", Provider: "claude-v3", Model: tc.cfgModel, NoFallback: true, Timeout: 5 * time.Second})
		_, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "hi"}}, ChatOpts{Model: tc.optModel})
		srv.Close()
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if got.Model != tc.want || got.Provider != "claude-v3" || !got.NoFallback {
			t.Errorf("%+v: sent model=%q provider=%q no_fallback=%v", tc, got.Model, got.Provider, got.NoFallback)
		}
	}
}

func TestAIManagerPrefersObservedModel(t *testing.T) {
	for _, tc := range []struct{ observed, want string }{{"claude-haiku-5-5", "claude-haiku-5-5"}, {"", "claude-opus-5"}} {
		p := newTestManager(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"text": "ok", "provider": "antigravity", "model": "claude-opus-5", "observed_model": tc.observed,
			})
		}, 5*time.Second)
		res, err := p.Chat(context.Background(), []models.Message{{Role: "user", Content: "hi"}}, ChatOpts{})
		if err != nil || res.Model != tc.want {
			t.Errorf("observed=%q: model=%q err=%v, want %q", tc.observed, res.Model, err, tc.want)
		}
	}
}
