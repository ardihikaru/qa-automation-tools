package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSettings_NoSecrets(t *testing.T) {
	r := &Router{}
	r.SetSettings(SettingsInfo{
		AIProvider: "ai-manager", ManagerURL: "http://user:pw@10.10.0.4:8099/v1/chat?k=secret",
		ManagerProvider: "claude-v3", NoFallback: true, TimeoutSeconds: 180,
		ExecutorURL: "http://10.10.0.9:8141",
	})
	rec := httptest.NewRecorder()
	r.handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	body := rec.Body.String()
	for _, bad := range []string{"pw", "secret", "user:"} {
		if strings.Contains(body, bad) {
			t.Fatalf("response leaks %q: %s", bad, body)
		}
	}
	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.AI.GatewayHost != "10.10.0.4:8099" || !got.AI.PinnedToClaudeV3 {
		t.Fatalf("unexpected: %+v", got.AI)
	}
}

func TestHandleSettings_PinnedFalseWhenFallbackOn(t *testing.T) {
	r := &Router{}
	r.SetSettings(SettingsInfo{AIProvider: "ai-manager", ManagerProvider: "claude-v3", NoFallback: false})
	rec := httptest.NewRecorder()
	r.handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	var got settingsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.AI.PinnedToClaudeV3 {
		t.Fatal("must not report pinned when fallback is enabled")
	}
}

func TestHandleSettings_MethodNotAllowed(t *testing.T) {
	r := &Router{}
	rec := httptest.NewRecorder()
	r.handleSettings(rec, httptest.NewRequest(http.MethodPost, "/api/settings", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code %d", rec.Code)
	}
}
