package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/testdb"
)

// finishCaseRun: a skipped case_run is a completed job with verdict "skipped"; it stores nothing and
// never replaces a pass/fail already stored.
func TestFinishCaseRunSkipped(t *testing.T) {
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	pushed := 0
	r.SetOnResultsStored(func(int) { pushed++ })
	ctx := context.Background()
	must := func(q string, a ...any) {
		if _, err := d.Exec(ctx, q, a...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1')`)
	for _, code := range []string{"TC-VOCBOOK-007", "TC-VOCBOOK-008"} {
		must(`INSERT INTO kb_test_cases (product_id, code, epic_code, spec_paths, content_hash) VALUES (1,?,?,?,?)`, code, "VOCBOOK", `[]`, strings.Repeat("0", 64))
	}
	// A report whose setup test and the case's own test are both skipped; caseStatus overrides the case's status.
	report := func(code, caseStatus string) map[string]interface{} {
		var rep map[string]interface{}
		body := `{"stats":{"startTime":"2026-11-14T10:00:00Z"},"suites":[{"specs":[
		 {"title":"preflight + seed","tests":[{"status":"skipped"}]},
		 {"title":"` + code + ` books","tests":[{"status":"` + caseStatus + `","annotations":[{"type":"skip","description":"the staging DB credential is missing"}]}]}]}]}`
		_ = json.Unmarshal([]byte(body), &rep)
		return map[string]interface{}{"passed": float64(0), "failed": float64(0), "skipped": float64(2), "report": rep}
	}
	payload := func(code string) map[string]interface{} {
		return map[string]interface{}{"product_id": 1, "case_code": code, "spec_paths": []string{"tests/x.spec.ts"}}
	}
	count := func() (n int) {
		_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM kb_case_results`).Scan(&n)
		return
	}

	res := report("TC-VOCBOOK-007", "skipped")
	if err := r.finishCaseRun(ctx, "job-s1", payload("TC-VOCBOOK-007"), res); err != nil {
		t.Fatal(err)
	}
	vd := res["verdict"].(map[string]interface{})
	if vd["result"] != "skipped" || vd["pass"] != false || res["skip_reason"] != "the staging DB credential is missing" {
		t.Fatalf("verdict %+v", res)
	}
	if count() != 0 || pushed != 0 {
		t.Fatalf("a first-time skip must store and push nothing: rows=%d pushed=%d", count(), pushed)
	}
	var lvl, msg string
	if err := d.QueryRow(ctx, `SELECT level, message FROM job_events WHERE job_id='job-s1' ORDER BY seq DESC LIMIT 1`).Scan(&lvl, &msg); err != nil {
		t.Fatal(err)
	}
	if lvl != "warn" || !strings.HasPrefix(msg, "skipped: the staging DB credential is missing") || !strings.Contains(msg, "hint") {
		t.Fatalf("event %s %q", lvl, msg)
	}

	// A stored fail survives a later skip.
	must(`INSERT INTO kb_case_results (product_id, case_code, result, run_at, run_ref, source) VALUES (1,'TC-VOCBOOK-007','fail','2026-11-10 10:00:00','old','case-run')`)
	if err := r.finishCaseRun(ctx, "job-s2", payload("TC-VOCBOOK-007"), report("TC-VOCBOOK-007", "skipped")); err != nil {
		t.Fatal(err)
	}
	var got string
	_ = d.QueryRow(ctx, `SELECT result FROM kb_case_results WHERE case_code='TC-VOCBOOK-007'`).Scan(&got)
	if got != "fail" || pushed != 0 {
		t.Fatalf("stored result %q pushed %d", got, pushed)
	}

	// Setup skipped but the case passed: a pass, stored and pushed.
	pass := report("TC-VOCBOOK-008", "expected")
	if err := r.finishCaseRun(ctx, "job-s3", payload("TC-VOCBOOK-008"), pass); err != nil {
		t.Fatal(err)
	}
	if pass["verdict"].(map[string]interface{})["result"] != "pass" || pushed != 1 || count() != 2 {
		t.Fatalf("pass: %+v pushed=%d rows=%d", pass["verdict"], pushed, count())
	}

	// Case not in the report: the job fails (a wrong title is a defect, not a skip); verdict "none".
	nf := report("TC-VOCBOOK-009", "skipped")
	if err := r.finishCaseRun(ctx, "job-s4", payload("TC-VOCBOOK-007"), nf); err == nil || nf["verdict"].(map[string]interface{})["result"] != "none" {
		t.Fatalf("not found: err=%v verdict=%+v", err, nf["verdict"])
	}
}
