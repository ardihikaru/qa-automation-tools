package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

const inboundTok = "gaos_abcdefabcdefabcdefabcdefabcdefabcdef"

func inboundCall(r *Router, method, path, authz, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGAOSInboundFailClosed(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	if w := inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6", "Bearer x", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("token unset: got %d, want 503", w.Code)
	}
	r.SetGAOSInboundToken(inboundTok)
	for _, h := range []string{"", "Bearer ", "Bearer wrong", "Basic " + inboundTok, inboundTok} {
		if w := inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6", h, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: got %d, want 401", h, w.Code)
		}
	}
	// Right token but no database: 503, not a panic.
	if w := inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6", "Bearer "+inboundTok, ""); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no db: got %d, want 503", w.Code)
	}
}

func TestGAOSInboundAllowList(t *testing.T) {
	for _, p := range []string{"/api/v1/gaos/results", "/api/v1/gaos/catalog"} {
		if !auth.IsAuthAllowList("GET", p) || !IsGAOSInboundPath(p) {
			t.Errorf("%s must be on both lists", p)
		}
	}
	if auth.IsAuthAllowList("GET", "/api/v1/gaos/other") || IsGAOSInboundPath("/api/v1/gaos/") {
		t.Error("only the two exact paths may bypass the cookie")
	}
}

func TestBearerMatches(t *testing.T) {
	if !bearerMatches("Bearer "+inboundTok, inboundTok) || !bearerMatches("bearer "+inboundTok, inboundTok) {
		t.Fatal("valid token refused")
	}
	if bearerMatches("Bearer "+inboundTok+"x", inboundTok) || bearerMatches("", inboundTok) {
		t.Fatal("invalid token accepted")
	}
}

func TestParseGAOSCursor(t *testing.T) {
	for in, want := range map[string]bool{"": true, "0:0": true, "1763000000:42": true, "x": false, "1:": false, "-1:2": false, "a:b": false} {
		if _, _, ok := parseGAOSCursor(in); ok != want {
			t.Errorf("%q: ok=%v want %v", in, ok, want)
		}
	}
}

func TestGAOSUsableSpec(t *testing.T) {
	cases := map[string]string{
		`["tests/api/a.spec.ts"]`:              "tests/api/a.spec.ts",
		`[]`:                                   "",
		`["/abs.ts"]`:                          "",
		`["a/../b.ts"]`:                        "",
		`["https://x/y"]`:                      "",
		`["` + strings.Repeat("a", 241) + `"]`: "",
		`not json`:                             "",
	}
	for in, want := range cases {
		if got := gaosUsableSpec([]byte(in)); got != want {
			t.Errorf("%.30s: got %q want %q", in, got, want)
		}
	}
}

func TestValidGAOSCase(t *testing.T) {
	ok := gaosCatalogCase{CaseID: "TC-AUTH-001", Decision: "planned"}
	if r := validGAOSCase(&ok); r != "" {
		t.Fatalf("valid case refused: %s", r)
	}
	for name, c := range map[string]gaosCatalogCase{
		"bad-case-id":  {CaseID: "x", Decision: "planned"},
		"bad-decision": {CaseID: "TC-AUTH-001", Decision: "maybe"},
		"bad-reason":   {CaseID: "TC-AUTH-001", Decision: "wont", Reason: strings.Repeat("r", 301)},
		"bad-features": {CaseID: "TC-AUTH-001", Decision: "planned", Features: make([]string, 13)},
	} {
		if got := validGAOSCase(&c); got != name {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

// DB-backed (skips without QA_TEST_MYSQL_DSN): scoping, paging, strict-free catalog store.
func TestGAOSInboundDB(t *testing.T) {
	d := testdb.New(t)
	gaosCursorLagSeconds = 0
	t.Cleanup(func() { gaosCursorLagSeconds = 60 })
	r := NewRouter(d, nil, nil, nil)
	r.SetGAOSInboundToken(inboundTok)
	ctx := context.Background()
	must := func(q string, a ...any) {
		if _, err := d.Exec(ctx, q, a...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'HIS','r1','s1','d1'),(2,'Other','r2','s2','d2')`)
	must(`INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (1,1,'p-6'),(2,0,'p-7')`)
	for i, code := range []string{"TC-AUTH-001", "TC-AUTH-002", "TC-AUTH-003"} {
		spec := `[]`
		auto := 0
		if i == 0 {
			spec, auto = `["tests/a.spec.ts"]`, 1
		}
		must(`INSERT INTO kb_test_cases (product_id, code, epic_code, spec_paths, automated, content_hash) VALUES (1,?,?,?,?,?)`, code, "AUTH", spec, auto, strings.Repeat("0", 64))
	}
	bearer := "Bearer " + inboundTok
	if w := inboundCall(r, "GET", "/api/v1/gaos/results?product=p-7", bearer, ""); w.Code != 404 {
		t.Fatalf("unflagged product: got %d, want 404", w.Code)
	}
	var seen []string
	after := ""
	for i := 0; i < 5; i++ {
		w := inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6&limit=2&after="+after, bearer, "")
		if w.Code != 200 {
			t.Fatalf("results: %d %s", w.Code, w.Body)
		}
		var pg gaosResultsPage
		_ = json.Unmarshal(w.Body.Bytes(), &pg)
		for _, it := range pg.Items {
			seen = append(seen, it.CaseID)
		}
		if pg.Next == "" || pg.Product != "p-6" {
			t.Fatalf("bad page %+v", pg)
		}
		after = pg.Next
		if !pg.More {
			break
		}
	}
	if len(seen) != 3 {
		t.Fatalf("paging saw %v", seen)
	}
	body := `{"product":{"id":"p-6"},"cases":[
	 {"caseId":"TC-AUTH-001","epic":"AUTH","decision":"planned","features":["A13.03"],"catRev":5,"implemented":true,"future":1},
	 {"caseId":"TC-AUTH-999","decision":"wont","reason":"x:y"},
	 {"caseId":"TC-AUTH-001","decision":"wont"},
	 {"caseId":"bad","decision":"planned"}],"catHead":5}`
	w := inboundCall(r, "POST", "/api/v1/gaos/catalog", bearer, body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"accepted":1`) || !strings.Contains(w.Body.String(), "unknown-case") || !strings.Contains(w.Body.String(), "bad-case-id") {
		t.Fatalf("catalog: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "duplicate-case") {
		t.Fatalf("duplicate not rejected: %s", w.Body)
	}
	// An older catRev must not overwrite a newer one.
	inboundCall(r, "POST", "/api/v1/gaos/catalog", bearer, `{"product":{"id":"p-6"},"cases":[{"caseId":"TC-AUTH-001","decision":"wont","catRev":2}]}`)
	var dec string
	_ = d.QueryRow(ctx, `SELECT decision FROM gaos_decisions WHERE case_code='TC-AUTH-001'`).Scan(&dec)
	if dec != "planned" {
		t.Fatalf("stale push overwrote: %s", dec)
	}
	// Tombstone: a removed case is sent as implemented:false without a spec.
	must(`UPDATE kb_test_cases SET removed_at = NOW() WHERE code = 'TC-AUTH-001'`)
	w = inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6", bearer, "")
	var pg gaosResultsPage
	_ = json.Unmarshal(w.Body.Bytes(), &pg)
	found := false
	for _, it := range pg.Items {
		if it.CaseID == "TC-AUTH-001" {
			found = true
			if it.Implemented || it.Spec != "" {
				t.Fatalf("removed case not a tombstone: %+v", it)
			}
		}
	}
	if !found {
		t.Fatalf("removed case missing from results: %s", w.Body)
	}
	must(`UPDATE kb_test_cases SET removed_at = NULL WHERE code = 'TC-AUTH-001'`)

	// Decisions view: stored row compared with our derivation (no story/features here, so module/features match;
	// GAOS sent features [A13.03] which we do not derive).
	r.SetGAOS(gaossync.New(d, "", ""))
	dreq := httptest.NewRequest("GET", "/api/products/1/gaos/decisions?diff=1", nil)
	dw := httptest.NewRecorder()
	r.handleGAOSDecisions(dw, dreq, 1)
	if dw.Code != 200 || !strings.Contains(dw.Body.String(), `"differing"`) {
		t.Fatalf("decisions: %d %s", dw.Code, dw.Body)
	}
	var n int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM gaos_decisions WHERE product_id=1`).Scan(&n)
	if n != 1 {
		t.Fatalf("stored %d decisions", n)
	}
}

func TestDiffDerived(t *testing.T) {
	a := gaossync.Derived{Epic: "AUTH", Module: "A13", Features: []string{"A13.02", "A13.01"}, Level: "API"}
	b := gaossync.Derived{Epic: "AUTH", Module: "A13", Features: []string{"A13.01", "A13.02"}, Level: "API"}
	if d := diffDerived(a, &b); len(d) != 0 {
		t.Fatalf("order must not matter: %v", d)
	}
	b.Module, b.Features = "A14", []string{"A13.01"}
	if d := diffDerived(a, &b); len(d) != 2 || d[0] != "module" || d[1] != "features" {
		t.Fatalf("got %v", d)
	}
	if d := diffDerived(a, nil); len(d) != 0 {
		t.Fatalf("no derivation = no diff: %v", d)
	}
}
