package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
)

func TestHandleSettings_NoSecrets(t *testing.T) {
	r := &Router{}
	r.SetSettings(SettingsInfo{
		AIProvider: "ai-manager", ManagerURL: "http://user:pw@10.8.0.4:8099/v1/chat?k=secret",
		ManagerProvider: "claude-v3", NoFallback: true, TimeoutSeconds: 180,
		ExecutorURL: "http://10.8.0.9:8141",
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
	if got.AI.GatewayHost != "10.8.0.4:8099" || !got.AI.PinnedToClaudeV3 {
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

func tzCall(r *Router, role, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/settings/timezone", strings.NewReader(body))
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{ID: "u1", Role: role}))
	rec := httptest.NewRecorder()
	r.handleSettingsTimezone(rec, req)
	return rec
}

func TestSettingsTimezone_Guards(t *testing.T) {
	r := &Router{}
	if c := tzCall(r, "QA", http.MethodPut, `{"timezone":"Asia/Jakarta"}`).Code; c != http.StatusForbidden {
		t.Fatalf("QA: %d", c)
	}
	if c := tzCall(r, "Admin", http.MethodGet, ``).Code; c != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", c)
	}
	for _, body := range []string{`{"timezone":"Mars/Base"}`, `{"timezone":""}`, `{"timezone":"Local"}`, `{"timezone":"UTC","x":1}`} {
		if c := tzCall(r, "DevOps", http.MethodPut, body).Code; c != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, c)
		}
	}
	if c := tzCall(r, "Admin", http.MethodPut, `{"timezone":"Asia/Makassar"}`).Code; c != http.StatusServiceUnavailable {
		t.Fatalf("no DB: %d", c)
	}
	// Without a database the zone falls back to the default.
	rec := httptest.NewRecorder()
	r.handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	var got settingsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Timezone != DefaultTimezone {
		t.Fatalf("timezone %q", got.Timezone)
	}
}

func TestSettingsTimezone_DB(t *testing.T) {
	r := newDBRouter(t)
	get := func() string {
		rec := httptest.NewRecorder()
		r.handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
		var got settingsResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		return got.Timezone
	}
	if tz := get(); tz != "Asia/Jakarta" {
		t.Fatalf("seeded zone %q", tz)
	}
	if c := tzCall(r, "Admin", http.MethodPut, `{"timezone":"Asia/Makassar"}`).Code; c != http.StatusOK {
		t.Fatalf("put: %d", c)
	}
	if tz := get(); tz != "Asia/Makassar" {
		t.Fatalf("zone %q", tz)
	}
	if _, err := r.db.Exec(context.Background(), `DELETE FROM app_settings`); err != nil {
		t.Fatal(err)
	}
	if tz := get(); tz != DefaultTimezone {
		t.Fatalf("missing row: %q", tz)
	}
}

func TestNewJobDetail(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	start, end := now.Add(-90*time.Second), now.Add(-30*time.Second)
	pay := map[string]interface{}{"case_code": "TC-1", "product_id": float64(7)}
	d := newJobDetail(jobRecord{Payload: pay, CreatedAt: now.Add(-time.Hour), StartedAt: &start, CompletedAt: &end}, now)
	if d.CaseCode != "TC-1" || d.ProductID != 7 || d.ElapsedMs != 60000 {
		t.Fatalf("%+v", d)
	}
	if d := newJobDetail(jobRecord{CreatedAt: now.Add(-10 * time.Second), StartedAt: &start}, now); d.ElapsedMs != 90000 || d.CaseCode != "" {
		t.Fatalf("running: %+v", d)
	}
	if d := newJobDetail(jobRecord{CreatedAt: now.Add(-10 * time.Second)}, now); d.ElapsedMs != 10000 {
		t.Fatalf("queued: %+v", d)
	}
	b, _ := json.Marshal(newJobDetail(jobRecord{ID: "j", Status: "queued", CreatedAt: now}, now))
	if !strings.Contains(string(b), `"id":"j"`) || !strings.Contains(string(b), `"elapsed_ms":0`) {
		t.Fatalf("json shape: %s", b)
	}
}
