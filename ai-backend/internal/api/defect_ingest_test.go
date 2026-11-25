package api

// QA_DEFECT_TRIAGE.md: the Playwright ingest stores defect states and tells the GAOS push; the
// service-repos route. DB-backed (scratch MySQL), skipped without one.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/qa-automation/ai-backend/internal/testdb"
)

const defectReport = `{"stats":{"startTime":"2026-11-25T10:00:00Z"},"suites":[{"specs":[
 {"title":"TC-SMP-008 edit","tests":[{"expectedStatus":"failed","status":"expected","annotations":[{"type":"defect","description":"DEF-101 a negative quantity is accepted"}],"results":[{"status":"failed","error":{"message":"Authorization: Bearer abcdefghijklmnop12"}}]}]},
 {"title":"TC-SMP-009 delete","tests":[{"expectedStatus":"passed","status":"unexpected","results":[{"status":"failed","error":{"message":"boom"}}]}]}]}]}`

func TestIngestStoresDefectsAndNotifies(t *testing.T) {
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	r.SetResultsIngestToken(ingestTok)
	must := func(q string, a ...any) {
		if _, err := d.Exec(context.Background(), q, a...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1')`)
	for _, c := range []string{"TC-SMP-008", "TC-SMP-009"} {
		must(`INSERT INTO kb_test_cases (product_id, code, epic_code, spec_paths, content_hash) VALUES (1,?,'SMP','[]',?)`, c, strings.Repeat("0", 64))
	}
	var mu sync.Mutex
	hooks := 0
	r.SetOnResultsStored(func(int) { mu.Lock(); hooks++; mu.Unlock() })
	post := func() {
		if w := inboundCall(r, "POST", "/api/v1/results/playwright?product=1&run_ref=ci-1", "Bearer "+ingestTok, defectReport); w.Code != 200 {
			t.Fatalf("ingest: %d %s", w.Code, w.Body)
		}
	}
	post()
	rows, err := d.Query(context.Background(), `SELECT case_code, kind, defect_id, excerpt FROM qa_defect_state WHERE product_id = 1 ORDER BY case_code`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var c, k, id, ex string
		if err := rows.Scan(&c, &k, &id, &ex); err != nil {
			t.Fatal(err)
		}
		got = append(got, c+"/"+k+"/"+id)
		if strings.Contains(ex, "abcdefghijklmnop12") {
			t.Errorf("the excerpt holds a token: %s", ex)
		}
	}
	if strings.Join(got, ",") != "TC-SMP-008/xfail/DEF-101,TC-SMP-009/fail/" {
		t.Errorf("states: %v", got)
	}
	// The same report again: results are stale, defects unchanged; the hook already ran once.
	mu.Lock()
	first := hooks
	mu.Unlock()
	post()
	mu.Lock()
	defer mu.Unlock()
	if first < 1 || hooks != first {
		t.Errorf("hook calls: after first %d, after repeat %d", first, hooks)
	}
}

func TestServiceReposRoute(t *testing.T) {
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	if _, err := d.Exec(context.Background(), `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(context.Background(), `INSERT INTO product_repos (product_id, name, repo_url, root_dir) VALUES (1,'sample-service','https://github.com/acme-org/sample-service','rr1')`); err != nil {
		t.Fatal(err)
	}
	call := func(method, body string, manage bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/products/1/service-repos", strings.NewReader(body))
		r.handleServiceRepos(w, req, 1, manage)
		return w
	}
	if w := call("PUT", `{"service_dir":"sample-app","repos":["sample-service"]}`, false); w.Code != http.StatusForbidden {
		t.Errorf("a role that cannot manage: %d", w.Code)
	}
	for _, bad := range []string{`{"service_dir":"../x","repos":[]}`, `{"service_dir":"sample-app","repos":["nope"]}`, `{"service_dir":"sample-app","repos":["sample-service","sample-service"]}`} {
		if w := call("PUT", bad, true); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if w := call("PUT", `{"service_dir":"sample-app","repos":["sample-service"]}`, true); w.Code != 200 {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	if w := call("GET", ``, false); w.Code != 200 || !strings.Contains(w.Body.String(), `{"items":[{"service_dir":"sample-app","repos":["sample-service"]}]}`) {
		t.Errorf("get: %d %s", w.Code, w.Body)
	}
	if w := call("PUT", `{"service_dir":"sample-app","repos":[]}`, true); w.Code != 200 {
		t.Fatalf("clear: %d", w.Code)
	}
	if w := call("GET", ``, false); !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Errorf("after clear: %s", w.Body)
	}
	if w := call("DELETE", ``, true); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: %d", w.Code)
	}
}
