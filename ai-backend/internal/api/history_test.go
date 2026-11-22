package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// RUN_HISTORY.md. The SQL is MySQL 8 (window functions, JSON); these tests need QA_TEST_MYSQL_DSN.

func getJSON(t *testing.T, r *Router, path string, want int) map[string]any {
	t.Helper()
	w := inboundCall(r, "GET", path, "", "")
	if w.Code != want {
		t.Fatalf("GET %s: %d %s (want %d)", path, w.Code, w.Body, want)
	}
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

func items(m map[string]any) []map[string]any {
	var out []map[string]any
	for _, it := range m["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func TestHistoryRoutesNeedADatabaseAndGET(t *testing.T) {
	r := NewRouter(nil, nil, nil, nil)
	if w := inboundCall(r, "GET", "/api/history/builds", "", ""); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no db: %d", w.Code)
	}
	if w := inboundCall(r, "POST", "/api/history/builds", "", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
}

func TestHistoryBuildsDB(t *testing.T) {
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1'), (2,'OTHER','r2','s2','d2')`)
	must(t, d, `INSERT INTO kb_test_cases (product_id, code, epic_code, spec_paths, content_hash) VALUES (1,'TC-AUTH-001','AUTH','[]',?), (1,'TC-PAT-001','PAT','[]',?)`, hash64, hash64)
	t0 := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	ins := func(id, typ, status, payload string, created time.Time, started, completed *time.Time, result string) {
		var res any
		if result != "" {
			res = result
		}
		must(t, d, `INSERT INTO job_runs (id, type, status, payload, result, created_at, started_at, completed_at) VALUES (?,?,?,?,?,?,?,?)`,
			id, typ, status, payload, res, created, started, completed)
	}
	at := func(d time.Duration) *time.Time { x := t0.Add(d); return &x }
	ev := func(job string, seq int, step string, off time.Duration) {
		must(t, d, `INSERT INTO job_events (job_id, seq, at, level, step, message) VALUES (?,?,?,?,?,?)`, job, seq, t0.Add(off), "info", step, "m")
	}
	// A finished build: queued 10 s, two approval rounds of 1 h and 30 min, done after 2 h.
	ins("job-a", "case_create", "completed", `{"product_id":1,"case_code":"TC-AUTH-001","batch_id":"eb-1","pr_url":""}`, t0.Add(-10*time.Second), at(0), at(2*time.Hour), `{"pr_url":"https://example/pr/1"}`)
	ev("job-a", 1, "prepare", 0)
	ev("job-a", 2, "generate", 5*time.Second)
	ev("job-a", 3, "run", 60*time.Second)
	ev("job-a", 4, "validate", 61*time.Second)
	ev("job-a", 5, "done", 70*time.Second) // the executor's done, mid-job: run
	ev("job-a", 6, "approval", 80*time.Second)
	ev("job-a", 7, "repair", 80*time.Second+time.Hour)
	ev("job-a", 8, "approval", 90*time.Second+time.Hour)
	ev("job-a", 9, "push", 90*time.Second+90*time.Minute)
	ev("job-a", 10, "done", 2*time.Hour-time.Second)
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at, answered_at) VALUES
		('job-a',1,'TC-AUTH-001','qa-job-a-pr','changes',?,?), ('job-a',1,'TC-AUTH-001','qa-job-a-pr2','approved',?,?)`,
		t0.Add(80*time.Second), t0.Add(80*time.Second+time.Hour), t0.Add(90*time.Second+time.Hour), t0.Add(90*time.Second+90*time.Minute))
	// A running build waiting on an open approval since 10 minutes ago.
	now := time.Now().UTC().Truncate(time.Second)
	ins("job-b", "case_create", "running", `{"product_id":1,"case_code":"TC-PAT-001"}`, now.Add(-time.Hour), &[]time.Time{now.Add(-time.Hour)}[0], nil, "")
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at) VALUES ('job-b',1,'TC-PAT-001','qa-job-b-pr','open',?)`, now.Add(-10*time.Minute))
	// A case_run of the other product, and a queued one.
	ins("job-c", "case_run", "completed", `{"product_id":2,"case_code":"TC-X-001"}`, t0.Add(-24*time.Hour), at(-24*time.Hour+5*time.Second), at(-24*time.Hour+65*time.Second), `{"verdict":{"result":"pass"}}`)
	ins("job-d", "case_run", "queued", `{"product_id":1,"case_code":"TC-AUTH-001"}`, now, nil, nil, "")
	// Not a case job: never listed.
	ins("job-e", "other", "completed", `{}`, t0, at(0), at(time.Second), "")
	must(t, d, `INSERT INTO pw_runs (product_id, source, run_ref, job_id, started_at) VALUES (1,'case-create','job-a','job-a',?), (1,'case-create','job-a','job-a',?)`, t0, t0.Add(time.Minute))

	m := getJSON(t, r, "/api/history/builds", 200)
	if m["total"] != float64(4) || m["page"] != float64(1) || m["page_size"] != float64(25) {
		t.Fatalf("list %v", m)
	}
	byID := map[string]map[string]any{}
	for _, it := range items(m) {
		byID[it["id"].(string)] = it
	}
	a := byID["job-a"]
	if a["queue_ms"] != float64(10000) || a["active_ms"] != float64(7200000) || a["waiting_ms"] != float64(5400000) || a["machine_ms"] != float64(1800000) {
		t.Errorf("job-a timing %v %v %v %v", a["queue_ms"], a["active_ms"], a["waiting_ms"], a["machine_ms"])
	}
	if a["epic_code"] != "AUTH" || a["batch_id"] != "eb-1" || a["product_name"] != "SUT" || a["pr_url"] != "https://example/pr/1" || a["approvals"] != float64(2) || a["runs"] != float64(2) {
		t.Errorf("job-a fields %v", a)
	}
	steps := map[string]float64{}
	var order []string
	for _, s := range a["steps"].([]any) {
		sm := s.(map[string]any)
		steps[sm["step"].(string)] = sm["ms"].(float64)
		order = append(order, sm["step"].(string))
	}
	if strings.Join(order, ",") != "prepare,generate,run,repair,approval,push,done" {
		t.Errorf("step order %v", order)
	}
	if steps["run"] != 20000 || steps["approval"] != 5400000 || steps["done"] != 1000 {
		t.Errorf("steps %v", steps)
	}
	b := byID["job-b"]
	if w := b["waiting_ms"].(float64); w < 600000 || w > 660000 {
		t.Errorf("open approval waits until now: %v", w)
	}
	if b["completed_at"] != nil || b["machine_ms"].(float64) < 3000000 {
		t.Errorf("running job %v", b)
	}
	if byID["job-d"]["active_ms"] != nil || byID["job-d"]["queue_ms"] != nil {
		t.Errorf("queued job has no times: %v", byID["job-d"])
	}
	if byID["job-c"]["outcome"] != "pass" {
		t.Errorf("case_run outcome %v", byID["job-c"])
	}
	sum := m["summary"].(map[string]any)
	if sum["count"] != float64(4) || sum["completed"] != float64(2) || sum["running"] != float64(2) || sum["avg_waiting_ms"] != float64(2700000) {
		t.Errorf("summary %v", sum)
	}

	for q, want := range map[string]int{
		"product=2": 1, "product=1&type=case_create": 2, "status=queued": 1, "case=TC-AUTH-001": 2, "epic=PAT": 1,
		"batch=eb-1": 1, "from=2026-11-19&to=2026-11-19": 1, "to=2026-11-19": 1, "page_size=1&page=2": 4,
	} {
		m := getJSON(t, r, "/api/history/builds?"+q, 200)
		if m["total"] != float64(want) {
			t.Errorf("%s: total %v want %d", q, m["total"], want)
		}
		if q == "page_size=1&page=2" && len(items(m)) != 1 {
			t.Errorf("paging: %d items", len(items(m)))
		}
	}
	for _, q := range []string{"product=x", "product=0", "from=2026-13-01", "from=2026-11-20&to=2026-11-19", "type=x", "status=x", "case=a%20b", "batch=x%20y"} {
		getJSON(t, r, "/api/history/builds?"+q, 400)
	}
	getJSON(t, r, "/api/history/nope", 404)
}

const historyReport = `{"stats":{"startTime":"2026-11-14T10:00:00Z","duration":4000},"suites":[{"specs":[
 {"title":"TC-AUTH-001 ok","tests":[{"status":"expected","results":[{"duration":1500}]}]},
 {"title":"TC-AUTH-002 broken","tests":[{"status":"unexpected","results":[{"duration":2500}]}]},
 {"title":"TC-ZZZ-999 unknown","tests":[{"status":"skipped","results":[{"duration":0}]}]}]}]}`

func TestHistoryRunsAndTheIngestDoors(t *testing.T) {
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	r.SetResultsIngestToken(ingestTok)
	ctx := context.Background()
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1')`)
	for _, c := range []string{"TC-AUTH-001", "TC-AUTH-002", "TC-VOCBOOK-007"} {
		must(t, d, `INSERT INTO kb_test_cases (product_id, code, epic_code, spec_paths, content_hash) VALUES (1,?,?,'[]',?)`, c, strings.Split(c, "-")[1], hash64)
	}

	// CI door: the answer keeps every key and adds run_id; a re-sent report keeps its id.
	post := func() map[string]any {
		w := inboundCall(r, "POST", "/api/v1/results/playwright?product=1&run_ref=ci-9", "Bearer "+ingestTok, historyReport)
		if w.Code != 200 {
			t.Fatalf("ingest %d %s", w.Code, w.Body)
		}
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	m1 := post()
	for _, k := range []string{"tests", "cases", "run_ref", "run_at", "stored", "stale", "unmatched", "unmatched_count", "run_id"} {
		if _, ok := m1[k]; !ok {
			t.Errorf("answer lost %q: %v", k, m1)
		}
	}
	if m1["run_id"].(float64) <= 0 || m1["stored"] != float64(2) {
		t.Fatalf("first ingest %v", m1)
	}
	if m2 := post(); m2["run_id"] != m1["run_id"] || m2["stale"] != float64(2) {
		t.Fatalf("re-sent %v", m2)
	}

	// case_run door: a skipped-only report is logged with its case, kb_case_results unchanged.
	var before int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM kb_case_results`).Scan(&before)
	skipped := func(code string) map[string]interface{} {
		var rep map[string]interface{}
		_ = json.Unmarshal([]byte(`{"stats":{"startTime":"2026-11-15T10:00:00Z","duration":900},"suites":[{"specs":[
		 {"title":"`+code+` books","tests":[{"status":"skipped","results":[{"duration":3}]}]}]}]}`), &rep)
		return map[string]interface{}{"report": rep}
	}
	pl := map[string]interface{}{"product_id": 1, "case_code": "TC-VOCBOOK-007", "spec_paths": []string{"tests/x.spec.ts"}}
	if err := r.finishCaseRun(ctx, "job-s1", pl, skipped("TC-VOCBOOK-007")); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM kb_case_results`).Scan(&after)
	if after != before {
		t.Fatalf("a skipped run changed kb_case_results: %d -> %d", before, after)
	}
	// A report without the job's case: logged, and the job still fails.
	if err := r.finishCaseRun(ctx, "job-s2", pl, skipped("TC-AUTH-001")); err == nil {
		t.Fatal("a report without the case must fail the job")
	}

	// case_create door: one row per executor run with its index; a failed executor run logs none.
	failing := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"run-1","status":"queued","job_id":"job-c1"}`))
			return
		}
		if failing {
			_, _ = w.Write([]byte(`{"id":"run-1","status":"failed","error":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"run-1","status":"completed","result":{"passed":1,"report":{"stats":{"startTime":"2026-11-16T10:00:00.500Z","duration":1234},"suites":[{"specs":[{"title":"TC-AUTH-001 ok","tests":[{"status":"expected","results":[{"duration":1200}]}]}]}]}}}`))
	}))
	defer srv.Close()
	r.SetExecutor(executor.New(executor.Config{URL: srv.URL, Token: "t"}), time.Minute)
	pool := repopool.New(t.TempDir())
	r.SetRepoPool(pool)
	r.SetCaseAutomation(CaseAutomationConfig{PoolHostDir: "/host/pool"})
	er := &executorRunner{r: r, jobID: "job-c1", pid: 1}
	ws := caseauto.Workspace{Dir: pool.Root}
	if res, err := er.RunCase(ctx, caseauto.RunRequest{Workspace: ws, CaseCode: "TC-AUTH-001", SpecPath: "tests/web/auth/TC-AUTH-001.spec.ts", Repair: 1}, func(string, string, string) {}); err != nil || !res.Pass {
		t.Fatalf("run case: %+v %v", res, err)
	}
	failing = true
	if _, err := er.RunCase(ctx, caseauto.RunRequest{Workspace: ws, CaseCode: "TC-AUTH-001", SpecPath: "tests/web/auth/TC-AUTH-001.spec.ts", Repair: 2}, func(string, string, string) {}); err == nil {
		t.Fatal("a failed executor run must error")
	}

	// The runs list: ci, case-run x2, case-create x1.
	m := getJSON(t, r, "/api/history/runs", 200)
	if m["total"] != float64(4) || m["since"] == nil {
		t.Fatalf("runs %v", m)
	}
	list := items(m)
	if list[0]["source"] != "case-create" || list[0]["run_index"] != float64(1) || list[0]["duration_ms"] != float64(1234) || list[0]["job_id"] != "job-c1" {
		t.Errorf("newest run %v", list[0])
	}
	sum := m["summary"].(map[string]any)
	if sum["fail"] != float64(1) || sum["pass"] != float64(1) || sum["skipped"] != float64(2) {
		t.Errorf("summary %v", sum)
	}
	for q, want := range map[string]int{
		"source=case-run": 2, "outcome=fail": 1, "job=job-s2": 1, "case=TC-VOCBOOK-007": 1, "epic=AUTH": 3, "from=2026-11-15": 3, "product=1": 4,
	} {
		if got := getJSON(t, r, "/api/history/runs?"+q, 200)["total"]; got != float64(want) {
			t.Errorf("%s: total %v want %d", q, got, want)
		}
	}
	withCase := items(getJSON(t, r, "/api/history/runs?case=TC-AUTH-002", 200))
	if len(withCase) != 1 || withCase[0]["case_result"] != "fail" || withCase[0]["case_duration_ms"] != float64(2500) {
		t.Errorf("case columns %v", withCase)
	}
	for _, q := range []string{"source=x", "outcome=x", "job=a%20b", "epic=%3B"} {
		getJSON(t, r, "/api/history/runs?"+q, 400)
	}

	// Detail: rows by duration, unknown codes marked.
	id := int64(m1["run_id"].(float64))
	det := getJSON(t, r, "/api/history/runs/"+strconv.FormatInt(id, 10), 200)
	rows := det["case_rows"].([]any)
	first, last := rows[0].(map[string]any), rows[len(rows)-1].(map[string]any)
	if len(rows) != 3 || first["case_code"] != "TC-AUTH-002" || first["known"] != true || last["case_code"] != "TC-ZZZ-999" || last["known"] != false {
		t.Errorf("detail rows %v", rows)
	}
	if det["source"] != "ci" || det["run_ref"] != "ci-9" || det["duration_ms"] != float64(4000) {
		t.Errorf("detail %v", det)
	}
	getJSON(t, r, "/api/history/runs/999999", 404)
	getJSON(t, r, "/api/history/runs/x", 404)
}

func TestHistorySyncsDB(t *testing.T) {
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1'), (2,'OTHER','r2','s2','d2')`)
	must(t, d, `INSERT INTO kb_syncs (product_id, status, trigger_kind, started_at, finished_at) VALUES
		(1,'succeeded','manual','2026-11-20 08:00:00','2026-11-20 08:01:30'), (1,'failed','schedule','2026-11-19 08:00:00','2026-11-19 08:00:05'), (2,'running','manual','2026-11-20 09:00:00',NULL)`)
	m := getJSON(t, r, "/api/history/syncs", 200)
	list := items(m)
	if m["total"] != float64(3) || list[0]["status"] != "running" || list[0]["duration_ms"] != nil || list[1]["duration_ms"] != float64(90000) || list[1]["trigger"] != "manual" || list[1]["product_name"] != "SUT" {
		t.Fatalf("syncs %v", m)
	}
	for q, want := range map[string]int{"product=1": 2, "status=failed": 1, "from=2026-11-20": 2} {
		if got := getJSON(t, r, "/api/history/syncs?"+q, 200)["total"]; got != float64(want) {
			t.Errorf("%s: %v want %d", q, got, want)
		}
	}
	getJSON(t, r, "/api/history/syncs?status=x", 400)
}
