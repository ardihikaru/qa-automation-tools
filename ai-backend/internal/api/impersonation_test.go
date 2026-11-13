package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/auth"
)

// These tests cover every check that runs before the database is touched,
// so AuthHandlers.DB stays nil. DB-backed paths live in
// impersonation_integration_test.go.

func callAs(u *auth.User, h http.HandlerFunc, method, ct, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if u != nil {
		req = req.WithContext(auth.SetUser(req.Context(), u))
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func errMsg(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var p map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	return p["error"]
}

func TestImpersonationHandlers_PreDBChecks(t *testing.T) {
	h := &AuthHandlers{CookieName: "qa_session"}
	admin := &auth.User{ID: "a", Username: "admin", Role: "Admin"}
	service := &auth.User{ID: "s", Username: "ai-service", Role: "Service"}
	qa := &auth.User{ID: "q", Username: "qa1", Role: "QA"}
	impersonating := &auth.User{ID: "q", Username: "qa1", Role: "QA", Impersonator: &auth.Actor{ID: "s", Username: "ai-service", Role: "Service"}}
	impersonatingAdmin := &auth.User{ID: "a2", Username: "admin2", Role: "Admin", Impersonator: &auth.Actor{ID: "s", Role: "Service"}}
	const js = "application/json"

	cases := []struct {
		name       string
		u          *auth.User
		h          http.HandlerFunc
		method, ct string
		body       string
		status     int
		msg        string
	}{
		{"users wrong method", admin, h.ListUsers, http.MethodPost, js, "", 405, "method not allowed"},
		{"users non-impersonator", qa, h.ListUsers, http.MethodGet, "", "", 403, "forbidden"},
		{"users while impersonating", impersonatingAdmin, h.ListUsers, http.MethodGet, "", "", 403, "forbidden"},

		{"start wrong method", admin, h.Impersonate, http.MethodGet, js, "", 405, "method not allowed"},
		{"start form content type", admin, h.Impersonate, http.MethodPost, "text/plain", `{"username":"qa1"}`, 415, "content type must be application/json"},
		{"start no content type", service, h.Impersonate, http.MethodPost, "", `{"username":"qa1"}`, 415, "content type must be application/json"},
		{"start non-impersonator", qa, h.Impersonate, http.MethodPost, js, `{"username":"x"}`, 403, "forbidden"},
		{"start nested", impersonatingAdmin, h.Impersonate, http.MethodPost, js, `{"username":"x"}`, 403, "forbidden"},
		{"start no user", nil, h.Impersonate, http.MethodPost, js, `{"username":"x"}`, 403, "forbidden"},
		{"start bad json", admin, h.Impersonate, http.MethodPost, js, `{`, 400, "invalid request body"},
		{"start empty username", admin, h.Impersonate, http.MethodPost, js, `{"username":"  "}`, 400, "invalid request body"},

		{"stop wrong method", impersonating, h.StopImpersonation, http.MethodGet, js, "", 405, "method not allowed"},
		{"stop form content type", impersonating, h.StopImpersonation, http.MethodPost, "application/x-www-form-urlencoded", "", 415, "content type must be application/json"},
		{"stop not impersonating", admin, h.StopImpersonation, http.MethodPost, js, `{}`, 400, "not impersonating"},

		{"change password while impersonating", impersonating, h.ChangePassword, http.MethodPost, js, `{"currentPassword":"a","newPassword":"bbbbbbbb"}`, 403, "not allowed while impersonating"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := callAs(c.u, c.h, c.method, c.ct, c.body)
			if w.Code != c.status {
				t.Fatalf("status: got %d want %d (body %s)", w.Code, c.status, w.Body.String())
			}
			if got := errMsg(t, w); got != c.msg {
				t.Fatalf("error: got %q want %q", got, c.msg)
			}
		})
	}
}

func TestMe_ImpersonatorShape(t *testing.T) {
	h := &AuthHandlers{}
	u := &auth.User{ID: "q", Username: "qa1", Role: "QA",
		Impersonator: &auth.Actor{ID: "s", Username: "ai-service", Role: "Service"}}
	w := callAs(u, h.Me, http.MethodGet, "", "")
	var p struct {
		User struct {
			Username     string      `json:"username"`
			Impersonator *auth.Actor `json:"impersonator"`
		} `json:"user"`
		MustChange bool `json:"mustChangePassword"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.User.Username != "qa1" || p.User.Impersonator == nil || p.User.Impersonator.Username != "ai-service" || p.MustChange {
		t.Fatalf("unexpected /me body: %s", w.Body.String())
	}

	// Not impersonating: the key is absent, not null.
	w = callAs(&auth.User{ID: "a", Username: "admin", Role: "Admin"}, h.Me, http.MethodGet, "", "")
	if strings.Contains(w.Body.String(), "impersonator") {
		t.Fatalf("impersonator key must be absent: %s", w.Body.String())
	}
	// Session internals never leak.
	if strings.Contains(w.Body.String(), "session") {
		t.Fatalf("session fields must not be serialised: %s", w.Body.String())
	}
}
