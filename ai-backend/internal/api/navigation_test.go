package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/navmap"
)

// NAVIGATION_MAP.md sections 3.5 and 6.

func navRoutesFixture() []navmap.Route {
	mk := func(surface, app, path string) navmap.Route {
		return navmap.Route{Key: navmap.RouteKey(surface, app, path), Surface: surface, App: app, Path: path}
	}
	return []navmap.Route{
		mk("api", "sut-vendor-backend", "/api/v1/service-status"),
		mk("api", "sut-vendor-backend", "/api/v1/kiosk-sut-v2/patients/:rm"),
		mk("api", "sut-vendor-backend", "/api/v1/kiosk-sut-v2/patients/search"),
		mk("web", "sut-frontend", "/auth/login"),
		mk("web", "sut-frontend", "/emr/patient/sl/:questionnaireID"),
	}
}

func TestMatchNavEntries(t *testing.T) {
	routes := navRoutesFixture()
	key := func(p string) string {
		for _, r := range routes {
			if r.Path == p {
				return r.Key
			}
		}
		return ""
	}
	entries := []navEntry{
		{Title: "TC-VSYS-001 probe", File: "tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts", Project: "api-rest", Kind: "api", Method: "GET", Host: "stg", Path: "/api/v1/service-status", Hits: 2},
		{Title: "TC-VSYS-001 probe", File: "tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts", Project: "api-rest", Kind: "api", Method: "GET", Host: "stg", Path: "/api/v1/service-status", Hits: 1},
		// A name in the path matches the parameter route; the literal "search" route wins over it.
		{Title: "TC-VKPAT-002 lookup", File: "tests/api/rest/sut-vendor-backend/k.spec.ts", Project: "api-rest", Kind: "api", Method: "GET", Path: "/api/v1/kiosk-sut-v2/patients/search"},
		{Title: "TC-VKPAT-003 by rm", File: "tests/api/rest/sut-vendor-backend/k.spec.ts", Project: "api-rest", Kind: "api", Method: "GET", Path: "/api/v1/kiosk-sut-v2/patients/RM0001234"},
		// A web spec's GraphQL call: no api app from the folder, and no route: unmatched.
		{Title: "TC-AUTH-011 login", File: "tests/web/sut-frontend/auth/login.spec.ts", Project: "web-sut-frontend-chromium", Kind: "api", Method: "POST", Path: "/user/query"},
		{Title: "TC-AUTH-011 login", File: "tests/web/sut-frontend/auth/login.spec.ts", Project: "web-sut-frontend-mobile-chrome", Kind: "web", Path: "/auth/login"},
		{Title: "TC-SL-001 TC-SL-002 form", File: "tests/web/sut-frontend/emr/sl.spec.ts", Project: "web-sut-frontend-chromium", Kind: "web", Path: "/emr/patient/sl/42"},
		// Dropped: setup project, no code, unknown kind, rejected path.
		{Title: "authenticate", File: "tests/setup/sut-frontend.setup.ts", Project: "setup-sut-frontend", Kind: "web", Path: "/auth/login"},
		{Title: "TC-AUTH-001 seed", File: "tests/setup/vendor-e2e.setup.ts", Project: "vendor-e2e-setup", Kind: "api", Path: "/api/v1/service-status"},
		{Title: "no code here", File: "tests/web/sut-frontend/a.spec.ts", Project: "web-sut-frontend-chromium", Kind: "web", Path: "/auth/login"},
		{Title: "TC-X-001 x", Kind: "ws", Path: "/x"},
		{Title: "TC-X-001 x", Kind: "web", Path: "/a/../b"},
	}
	rows, unmatched := matchNavEntries(entries, routes)
	got := map[string]int{}
	for _, r := range rows {
		got[r.code+" "+r.method+" "+r.key] = r.hits
	}
	want := map[string]int{
		"TC-VSYS-001 GET " + key("/api/v1/service-status"):                3,
		"TC-VKPAT-002 GET " + key("/api/v1/kiosk-sut-v2/patients/search"): 1,
		"TC-VKPAT-003 GET " + key("/api/v1/kiosk-sut-v2/patients/:rm"):    1,
		"TC-AUTH-011  " + key("/auth/login"):                              1,
		"TC-SL-001  " + key("/emr/patient/sl/:questionnaireID"):           1,
		"TC-SL-002  " + key("/emr/patient/sl/:questionnaireID"):           1,
	}
	if len(got) != len(want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	for k, h := range want {
		if got[k] != h {
			t.Errorf("%s: hits %d, want %d (all: %v)", k, got[k], h, got)
		}
	}
	if unmatched != 6 {
		t.Errorf("unmatched %d, want 6", unmatched)
	}
}

// navFixture stores a small inventory for product 1 the way the sync does.
func navFixture(t *testing.T) (*Router, *db.DB) {
	t.Helper()
	r, d := epicFixture(t)
	must(t, d, `INSERT INTO kb_epics (product_id, code, service, content_hash) VALUES (1,'VSYS','vendor-backend',?), (1,'VKAUTH','vendor-backend',?)`, hash64, hash64)
	files := map[string][]byte{
		"src/web/sut-frontend/pages/auth/login.page.ts": []byte("export class LoginPage {\n  protected readonly path = '/auth/login';\n}\n"),
	}
	inv := navmap.Build(navmap.Input{
		Features: []navmap.Feature{{Code: "B06.01", Name: "probe", RepoEntry: "sut-vendor-backend `/api/v1/service-status`"}},
		Epics:    []navmap.Epic{{Code: "VSYS", Service: "vendor-backend"}, {Code: "VKAUTH", Service: "vendor-backend"}},
		Cases: []navmap.Case{
			{Code: "TC-VSYS-001", EpicCode: "VSYS", Polarity: "+", Scenario: "when `GET /api/v1/service-status` then 200"},
			{Code: "TC-VSYS-002", EpicCode: "VSYS", Polarity: "+", Scenario: "when `GET /api/v1/service-status` then no stack"},
			{Code: "TC-VSYS-003", EpicCode: "VSYS", Polarity: "-", Scenario: "when `POST /api/v1/service-status` then 405"},
			{Code: "TC-VSYS-009", EpicCode: "VSYS", Polarity: "+", Scenario: "when `GET /api/v1/service-status` with a header"},
			{Code: "TC-VSYS-010", EpicCode: "VSYS", Polarity: "+", Scenario: "when `GET /api/v1/service-status` twice"},
			{Code: "TC-VSYS-011", EpicCode: "VSYS", Polarity: "+", Scenario: "when `GET /api/v1/service-status` slowly"},
			{Code: "TC-VSYS-012", EpicCode: "VSYS", Polarity: "+", Scenario: "when `PUT /api/v1/service-status`"},
			{Code: "TC-VKAUTH-007", EpicCode: "VKAUTH", Polarity: "+", Scenario: "GET /kiosk-sut-v2/health — 200"},
		},
		Files: files,
	})
	for _, rt := range inv.Routes {
		must(t, d, `INSERT INTO nav_routes (product_id, route_key, surface, app, path, methods, sources, page_objects) VALUES (1,?,?,?,?,?,?,?)`,
			rt.Key, rt.Surface, rt.App, rt.Path, jsonOf(rt.Methods), jsonOf(rt.Sources), jsonOf(rt.PageObjects))
	}
	for _, l := range inv.Links {
		must(t, d, `INSERT INTO nav_links (product_id, route_key, ref_type, ref, source, method) VALUES (1,?,?,?,?,?)`, l.RouteKey, l.RefType, l.Ref, l.Source, l.Method)
	}
	must(t, d, `INSERT INTO nav_builds (product_id, sync_id, commit_sha, nav_version, status, routes, links, problems, built_at) VALUES (1, NULL, ?, 1, 'ok', ?, ?, '[]', UTC_TIMESTAMP())`,
		strings.Repeat("a", 40), len(inv.Routes), len(inv.Links))
	must(t, d, `INSERT INTO kb_features (product_id, code, module_code, name, release_status, content_hash) VALUES (1,'B06.01','B06','probe','PROD',?)`, hash64)

	addCase(t, d, "TC-VSYS-001", "VSYS", "automated", "", "P1", `["tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts"]`)
	addCase(t, d, "TC-VSYS-002", "VSYS", "automated", "", "P1", `["tests/api/rest/sut-vendor-backend/TC-VSYS-002.spec.ts"]`)
	addCase(t, d, "TC-VSYS-003", "VSYS", "automated", "", "P2", `["tests/api/rest/sut-vendor-backend/TC-VSYS-003.spec.ts"]`)
	addCase(t, d, "TC-VSYS-009", "VSYS", "planned", "", "P2", `[]`)
	addCase(t, d, "TC-VSYS-010", "VSYS", "planned", "", "P0", `[]`) // active job: excluded
	addCase(t, d, "TC-VSYS-011", "VSYS", "planned", "", "P0", `[]`) // GAOS says wont: excluded
	addCase(t, d, "TC-VSYS-012", "VSYS", "planned", "", "P1", `[]`) // another method: tier 2
	addCase(t, d, "TC-VKAUTH-007", "VKAUTH", "planned", "", "P1", `[]`)
	must(t, d, `UPDATE kb_test_cases SET automated = 1 WHERE status = 'automated'`)
	must(t, d, `INSERT INTO kb_case_results (product_id, case_code, result, run_at, run_ref, source) VALUES (1,'TC-VSYS-001','pass',UTC_TIMESTAMP(),'pw-1','upload'), (1,'TC-VSYS-002','pass',UTC_TIMESTAMP(),'pw-1','upload')`)
	must(t, d, `INSERT INTO gaos_decisions (product_id, case_code, features, decision) VALUES (1,'TC-VSYS-011','[]','wont')`)
	addJob(t, d, "job-active", "case_create", "running", "", "TC-VSYS-010", "", "", time.Now().UTC())
	addJob(t, d, "job-old", "case_run", "completed", "", "TC-VSYS-001", "", `{"verdict":{"pass":true}}`, time.Now().UTC().Add(-time.Hour))
	return r, d
}

func jsonOf(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func getNav(t *testing.T, r *Router, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(auth.SetUser(req.Context(), &auth.User{ID: "u1", Role: "Viewer"}))
	w := httptest.NewRecorder()
	r.handleProducts(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestNavigationMapAndRoute(t *testing.T) {
	r, d := navFixture(t)
	code, out := getNav(t, r, "/api/products/1/navigation")
	if code != 200 {
		t.Fatalf("map answered %d %v", code, out)
	}
	routes := out["routes"].([]any)
	byPath := map[string]map[string]any{}
	for _, x := range routes {
		m := x.(map[string]any)
		byPath[m["path"].(string)] = m
	}
	ss := byPath["/api/v1/service-status"]
	if ss == nil || ss["status"] != "failing" && ss["status"] != "passing" {
		t.Fatalf("service-status route: %v", ss)
	}
	// TC-VSYS-003 is automated without a result: two passes, no fail -> passing.
	if ss["status"] != "passing" || ss["cases"].(float64) != 7 || ss["automated"].(float64) != 3 || ss["pass"].(float64) != 2 || ss["features"].(float64) != 1 {
		t.Errorf("service-status coverage: %v", ss)
	}
	if h := byPath["/api/v1/kiosk-sut-v2/health"]; h == nil || h["status"] != "planned" {
		t.Errorf("kiosk health: %v", h)
	}
	if l := byPath["/auth/login"]; l == nil || l["status"] != "untested" {
		t.Errorf("login: %v", l)
	}
	if out["build"] == nil {
		t.Error("build missing")
	}

	next := out["next"].([]any)
	var codes []string
	for _, x := range next {
		m := x.(map[string]any)
		codes = append(codes, m["case_code"].(string)+"/"+jsonNum(m["tier"]))
	}
	if strings.Join(codes, ",") != "TC-VSYS-009/1,TC-VSYS-012/2" {
		t.Errorf("next = %v", codes)
	}

	// Filters.
	if code, out := getNav(t, r, "/api/products/1/navigation?surface=web"); code != 200 || len(out["routes"].([]any)) != 1 || len(out["next"].([]any)) != 0 {
		t.Errorf("surface=web: %d %v", code, out)
	}
	if code, out := getNav(t, r, "/api/products/1/navigation?status=planned"); code != 200 || len(out["routes"].([]any)) != 1 {
		t.Errorf("status=planned: %d %v", code, out)
	}
	if code, out := getNav(t, r, "/api/products/1/navigation?q=vkauth-007"); code != 200 || len(out["routes"].([]any)) != 1 {
		t.Errorf("q by case code: %d %v", code, out)
	}
	if code, _ := getNav(t, r, "/api/products/1/navigation?surface=graphql"); code != 400 {
		t.Errorf("bad surface answered %d", code)
	}
	if code, _ := getNav(t, r, "/api/products/1/navigation?status=green"); code != 400 {
		t.Errorf("bad status answered %d", code)
	}
	if code, _ := getNav(t, r, "/api/products/99/navigation"); code != 404 {
		t.Errorf("unknown product answered %d", code)
	}

	// Drill-down.
	key := ss["key"].(string)
	code, rt := getNav(t, r, "/api/products/1/navigation/routes/"+key)
	if code != 200 {
		t.Fatalf("route answered %d %v", code, rt)
	}
	if f := rt["features"].([]any); len(f) != 1 || f[0].(map[string]any)["code"] != "B06.01" {
		t.Errorf("features: %v", f)
	}
	var lastJob any
	for _, x := range rt["cases"].([]any) {
		m := x.(map[string]any)
		if m["code"] == "TC-VSYS-001" {
			lastJob = m["last_job"]
			if m["result"] != "pass" || m["run_at"] == nil {
				t.Errorf("TC-VSYS-001: %v", m)
			}
		}
	}
	if lj, ok := lastJob.(map[string]any); !ok || lj["id"] != "job-old" {
		t.Errorf("last job: %v", lastJob)
	}
	if code, _ := getNav(t, r, "/api/products/1/navigation/routes/"+strings.Repeat("0", 40)); code != 404 {
		t.Errorf("unknown route answered %d", code)
	}
	if code, _ := getNav(t, r, "/api/products/1/navigation/routes/nope"); code != 404 {
		t.Errorf("bad key answered %d", code)
	}

	// Observations: stored by route key, idempotent per executor run, unmatched not stored.
	ctx := context.Background()
	nav := map[string]any{"version": 1, "entries": []any{
		map[string]any{"title": "TC-VSYS-001 probe", "file": "tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts", "project": "api-rest", "kind": "api", "method": "GET", "host": "stg", "path": "/api/v1/service-status", "hits": 2},
		map[string]any{"title": "TC-VSYS-001 probe", "file": "tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts", "project": "api-rest", "kind": "api", "method": "GET", "host": "stg", "path": "/api/v1/patients/Budi%20Santoso/1990-01-01"},
	}}
	for i := 0; i < 2; i++ {
		cnt := storeNavObservations(ctx, d, 1, "job-old", "run-1", nav)
		if cnt.Stored != 1 || cnt.Unmatched != 1 {
			t.Fatalf("store %d: %+v", i, cnt)
		}
	}
	var n, hits int
	_ = d.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(hits), 0) FROM nav_observations WHERE product_id = 1`).Scan(&n, &hits)
	if n != 1 || hits != 2 {
		t.Errorf("observations: %d rows, %d hits", n, hits)
	}
	var raw int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM nav_observations WHERE route_key NOT IN (SELECT route_key FROM nav_routes)`).Scan(&raw)
	if raw != 0 {
		t.Error("an unmatched observation was stored")
	}
	if cnt := storeNavObservations(ctx, d, 1, "job-old", "run-1", map[string]any{"version": 2}); cnt.Stored != 0 {
		t.Errorf("unknown version stored %+v", cnt)
	}
	_, out = getNav(t, r, "/api/products/1/navigation")
	for _, x := range out["routes"].([]any) {
		m := x.(map[string]any)
		if m["path"] == "/api/v1/service-status" && (m["observed_hits"].(float64) != 2 || m["last_observed_at"] == nil) {
			t.Errorf("observed: %v", m)
		}
	}
	if out["totals"].(map[string]any)["observed"].(float64) != 1 {
		t.Errorf("totals: %v", out["totals"])
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
