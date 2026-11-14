package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

const ingestTok = "results-token-0123456789abcdef0123456789"

const pwReportJSON = `{"stats":{"startTime":"2026-11-14T10:00:00Z"},"suites":[{"specs":[
 {"title":"TC-AUTH-001 ok","tests":[{"status":"expected"}]},
 {"title":"TC-AUTH-002 broken","tests":[{"status":"unexpected"}]},
 {"title":"TC-ZZZ-999 unknown","tests":[{"status":"expected"}]}]}]}`

func TestResultsIngestAuth(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	if w := inboundCall(r, "POST", "/api/v1/results/playwright?product=1", "Bearer x", "{}"); w.Code != 503 {
		t.Fatalf("unset: %d", w.Code)
	}
	r.SetResultsIngestToken(ingestTok)
	for _, h := range []string{"", "Bearer nope", "Bearer " + inboundTok} {
		if w := inboundCall(r, "POST", "/api/v1/results/playwright?product=1", h, "{}"); w.Code != 401 {
			t.Errorf("%q: %d", h, w.Code)
		}
	}
	if w := inboundCall(r, "GET", "/api/v1/results/playwright?product=1", "Bearer "+ingestTok, ""); w.Code != 405 {
		t.Errorf("GET: %d", w.Code)
	}
	if w := inboundCall(r, "POST", "/api/v1/results/playwright?product=x", "Bearer "+ingestTok, "{}"); w.Code != 400 {
		t.Errorf("bad product: %d", w.Code)
	}
	if !auth.IsAuthAllowList("POST", "/api/v1/results/playwright") || !IsResultsIngestPath("/api/v1/results/playwright") {
		t.Error("allow-list mismatch")
	}
}

func TestCaseResultsDB(t *testing.T) {
	d := testdb.New(t)
	gaosCursorLagSeconds = 0
	t.Cleanup(func() { gaosCursorLagSeconds = 60 })
	r := NewRouter(d, nil, nil, nil)
	r.SetGAOSInboundToken(inboundTok)
	r.SetResultsIngestToken(ingestTok)
	must := func(q string, a ...any) {
		if _, err := d.Exec(context.Background(), q, a...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'HIS','r1','s1','d1')`)
	must(`INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (1,1,'p-6')`)
	for _, code := range []string{"TC-AUTH-001", "TC-AUTH-002", "TC-AUTH-003"} {
		must(`INSERT INTO kb_test_cases (product_id, code, epic_code, spec_paths, content_hash) VALUES (1,?,?,?,?)`, code, "AUTH", `[]`, strings.Repeat("0", 64))
	}
	post := func() map[string]any {
		w := inboundCall(r, "POST", "/api/v1/results/playwright?product=1&run_ref=ci-77", "Bearer "+ingestTok, pwReportJSON)
		if w.Code != 200 {
			t.Fatalf("ingest: %d %s", w.Code, w.Body)
		}
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	m := post()
	if m["stored"] != float64(2) || m["unmatched_count"] != float64(1) || m["run_ref"] != "ci-77" {
		t.Fatalf("outcome %v", m)
	}
	if m := post(); m["stored"] != float64(0) || m["stale"] != float64(2) {
		t.Fatalf("re-sending the same run must change nothing: %v", m)
	}

	w := inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6", "Bearer "+inboundTok, "")
	var pg gaosResultsPage
	_ = json.Unmarshal(w.Body.Bytes(), &pg)
	got := map[string]gaosResultItem{}
	for _, it := range pg.Items {
		got[it.CaseID] = it
	}
	if got["TC-AUTH-001"].Result != "pass" || got["TC-AUTH-001"].RunAt != 1794650400 || got["TC-AUTH-001"].RunRef != "ci-77" {
		t.Fatalf("pass item: %+v", got["TC-AUTH-001"])
	}
	if got["TC-AUTH-002"].Result != "fail" || got["TC-AUTH-003"].Result != "" || got["TC-AUTH-003"].RunAt != 0 {
		t.Fatalf("items: %+v", got)
	}

	// An older report must not replace a newer result.
	old := strings.Replace(pwReportJSON, "2026-11-14T10:00:00Z", "2026-11-13T10:00:00Z", 1)
	inboundCall(r, "POST", "/api/v1/results/playwright?product=1", "Bearer "+ingestTok, old)
	var res string
	_ = d.QueryRow(context.Background(), `SELECT result FROM kb_case_results WHERE case_code='TC-AUTH-002'`).Scan(&res)
	if res != "fail" {
		t.Fatalf("older run overwrote: %s", res)
	}

	// A new result moves the case past the GAOS cursor.
	after := pg.Next
	if w := inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6&after="+after, "Bearer "+inboundTok, ""); !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("nothing new expected: %s", w.Body)
	}
	time.Sleep(1100 * time.Millisecond) // updated_at has 1 s resolution; production holds rows back for 60 s
	newer := strings.Replace(strings.Replace(pwReportJSON, "2026-11-14T10:00:00Z", "2026-11-14T11:00:00Z", 1), `"unexpected"`, `"expected"`, 1)
	inboundCall(r, "POST", "/api/v1/results/playwright?product=1", "Bearer "+ingestTok, newer)
	w = inboundCall(r, "GET", "/api/v1/gaos/results?product=p-6&after="+after, "Bearer "+inboundTok, "")
	if !strings.Contains(w.Body.String(), `"TC-AUTH-002"`) || !strings.Contains(w.Body.String(), `"result":"pass"`) {
		t.Fatalf("new result not re-sent: %s", w.Body)
	}
	if w := inboundCall(r, "POST", "/api/v1/results/playwright?product=99", "Bearer "+ingestTok, pwReportJSON); w.Code != 404 {
		t.Fatalf("unknown product: %d", w.Code)
	}
}
