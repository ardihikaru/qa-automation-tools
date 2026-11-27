package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func TestKnowledgeLangParam(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want string
		ok   bool
	}{
		{"/x", "en", true},
		{"/x?lang=id", "id", true},
		{"/x?lang=en", "en", true},
		{"/x?lang=fr", "", false},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		got, ok := knowledgeLang(rec, req)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got %q %v", tc.url, got, ok)
		}
		if !ok && rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", tc.url, rec.Code)
		}
	}
	// The user's language is the default; ?lang beats it.
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{Language: "id"}))
	if got, _ := knowledgeLang(httptest.NewRecorder(), req); got != "id" {
		t.Errorf("user language: %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/x?lang=en", nil)
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{Language: "id"}))
	if got, _ := knowledgeLang(httptest.NewRecorder(), req); got != "en" {
		t.Errorf("?lang must win: %q", got)
	}
}

func TestLanguageAndTranslationEndpointsIT(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	syncer := knowledge.NewSyncer(d, nil, nil)
	tr := knowledge.NewTranslator(ctx, syncer, nil, knowledge.TranslatorConfig{}) // disabled: no model
	r := NewRouter(d, nil, nil, nil)
	r.SetKnowledge(syncer)
	r.SetTranslator(tr)
	r.SetAuth(&AuthHandlers{DB: d.SQL, CookieName: "qa_session", SessionTTL: 12 * time.Hour},
		auth.Require(auth.Config{DB: d.SQL, CookieName: "qa_session"}), "http://localhost:6173")
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	e := &itEnv{t: t, sql: d.SQL, srv: srv, prefix: "tr" + uuid.NewString()[:8] + "-"}
	e.user("admin", "Admin", true)
	e.user("other", "Admin", true)
	c := e.login("admin")

	user := func() map[string]any {
		_, body := c.do(http.MethodGet, "/api/auth/me", nil)
		return body["user"].(map[string]any)
	}
	if got := user()["language"]; got != "en" {
		t.Fatalf("default language %v", got)
	}
	if st, body := c.do(http.MethodPut, "/api/auth/language", map[string]string{"language": "id"}); st != 200 || body["language"] != "id" {
		t.Fatalf("set: %d %v", st, body)
	}
	if got := user()["language"]; got != "id" {
		t.Fatalf("language after set %v", got)
	}
	if st, _ := c.do(http.MethodPut, "/api/auth/language", map[string]string{"language": "fr"}); st != 400 {
		t.Fatalf("bad language: %d", st)
	}
	if st, _ := c.do(http.MethodGet, "/api/auth/language", nil); st != 405 {
		t.Fatalf("GET: %d", st)
	}
	// Impersonating: the effective user's language is read, and it cannot be changed.
	if st, _ := c.do(http.MethodPost, "/api/auth/impersonate", map[string]string{"username": e.prefix + "other"}); st != 200 {
		t.Fatalf("impersonate: %d", st)
	}
	if got := user()["language"]; got != "en" {
		t.Fatalf("impersonated language %v", got)
	}
	if st, _ := c.do(http.MethodPut, "/api/auth/language", map[string]string{"language": "id"}); st != 403 {
		t.Fatalf("set while impersonating: %d", st)
	}
	_, _ = c.do(http.MethodPost, "/api/auth/impersonate/stop", map[string]string{})

	res, err := d.Exec(ctx, `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES ('SUT', 'https://github.com/o/r', 'https://s', 'sut', 'main')`)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/products/" + strconv.Itoa(int(res.LastInsertId())) + "/knowledge"
	st, body := c.do(http.MethodGet, base+"/translations/status", nil)
	cov, _ := body["lang_coverage"].(map[string]any)
	if st != 200 || body["enabled"] != false || body["model"] != "" || body["running"] != false || body["last_run"] != nil || cov["total"] != float64(0) {
		t.Fatalf("status: %d %v", st, body)
	}
	if st, _ := c.do(http.MethodPost, base+"/translations/run", map[string]string{}); st != 503 {
		t.Fatalf("run while disabled: %d", st)
	}
	if st, _ := c.do(http.MethodGet, base+"/stories?lang=zz", nil); st != 400 {
		t.Fatalf("bad ?lang: %d", st)
	}
	if st, _ := c.do(http.MethodGet, base+"/epics?lang=id&q=x", nil); st != 200 {
		t.Fatalf("epics q: %d", st)
	}
	if st, _ := c.do(http.MethodGet, base+"/modules?lang=id&q=x", nil); st != 200 {
		t.Fatalf("modules q: %d", st)
	}
}
