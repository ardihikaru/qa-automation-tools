package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// HISTORY_ANSWER.md. DB-backed (QA_TEST_MYSQL_DSN, a scratch mysql:8.0) with a fake GAOS.

type fakeGAOS struct {
	t       *testing.T
	posts   atomic.Int32
	gets    atomic.Int32
	lastReq gaossync.InboxAnswer
	lastURL string
	// answer handles POST .../answer; get handles GET .../inbox/{ext}.
	answer func(w http.ResponseWriter, r *http.Request)
	get    func(w http.ResponseWriter, r *http.Request)
}

func (f *fakeGAOS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+inboundTok {
		f.t.Errorf("no bearer token on %s %s", r.Method, r.URL.Path)
	}
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/answer"):
		f.posts.Add(1)
		f.lastURL = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &f.lastReq)
		if f.answer == nil {
			http.NotFound(w, r)
			return
		}
		f.answer(w, r)
	case r.Method == http.MethodGet:
		f.gets.Add(1)
		if f.get == nil {
			http.NotFound(w, r)
			return
		}
		f.get(w, r)
	default:
		http.NotFound(w, r)
	}
}

const relaySource = `{"kind":"qa-tool","client":"qa-automation-tools","origin":"qa-tool.sismedika.com","actor":{"username":"tester","display":"Test Er"}}`

func answered(ext, label, text, source string) string {
	return `{"externalId":"` + ext + `","status":"answered","choice":1,"optionLabel":"` + label + `","text":"` + text + `","answeredAt":"2026-11-21T10:00:00Z","source":` + source + `}`
}

type relayFix struct {
	r    *Router
	d    *db.DB
	gaos *fakeGAOS
}

// relayFixture: product 1 integrated as p-6, a running case_create job "job-r" waiting on its
// first approval round, which exists in GAOS.
func relayFixture(t *testing.T) relayFix {
	t.Helper()
	d := testdb.New(t)
	fg := &fakeGAOS{t: t}
	srv := httptest.NewServer(fg)
	t.Cleanup(srv.Close)
	r := NewRouter(d, nil, nil, nil)
	r.SetGAOS(gaossync.New(d, srv.URL+"/api/v1", inboundTok))
	r.SetGAOSInboundToken(inboundTok)
	r.approvalRelay.timeout = 400 * time.Millisecond
	r.approvalRelay.reconcile = 400 * time.Millisecond
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'HIS','r1','s1','d1')`)
	must(t, d, `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (1,1,'p-6')`)
	now := time.Now().UTC()
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at, started_at) VALUES ('job-r','case_create','running',?,?,?)`,
		`{"product_id":1,"case_code":"TC-AUTH-001","spec_path":"tests/auth/login.spec.ts","report":{"passed":3,"failed":0,"skipped":1}}`, now, now)
	must(t, d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, gaos_created, created_at) VALUES ('job-r',1,'TC-AUTH-001','qa-job-r-pr','open',1,?)`, now)
	return relayFix{r: r, d: d, gaos: fg}
}

type approvalRow struct {
	status, by, source, actor, detail string
}

func readApproval(t *testing.T, d *db.DB, ext string) approvalRow {
	t.Helper()
	var a approvalRow
	if err := d.QueryRow(context.Background(), `SELECT status, COALESCE(answered_by,''), COALESCE(answer_source,''), COALESCE(answer_actor,''),
		COALESCE(CAST(answer_source_detail AS CHAR),'') FROM case_approvals WHERE external_id = ?`, ext).Scan(&a.status, &a.by, &a.source, &a.actor, &a.detail); err != nil {
		t.Fatal(err)
	}
	return a
}

func jobEventTexts(t *testing.T, d *db.DB, job string) string {
	t.Helper()
	rows, err := d.Query(context.Background(), `SELECT level, message FROM job_events WHERE job_id = ? ORDER BY seq`, job)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var l, m string
		_ = rows.Scan(&l, &m)
		b.WriteString(l + ": " + m + "\n")
	}
	return b.String()
}

func postAnswer(r *Router, ctx context.Context, body string) (*httptest.ResponseRecorder, map[string]any) {
	w := dispatchAs(r, ctx, http.MethodPost, "/api/jobs/job-r/approval", body)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

const histApprove = `{"decision":"approve","text":"lgtm","via":"history","external_id":"qa-job-r-pr"}`

func TestRelay200AppliesAtOnceAndTheLateWebhookIsANoOp(t *testing.T) {
	f := relayFixture(t)
	f.gaos.answer = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(answered("qa-job-r-pr", "Approve", "lgtm", relaySource)))
	}
	w, m := postAnswer(f.r, asUser("QA"), histApprove)
	if w.Code != 200 || m["delivered"] != "gaos" || m["status"] != "approved" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if f.gaos.lastURL != "/api/v1/products/p-6/inbox/qa-job-r-pr/answer" || f.gaos.lastReq.Choice != 1 || f.gaos.lastReq.Text != "lgtm" || f.gaos.lastReq.Actor.Username != "tester" {
		t.Fatalf("request %s %+v", f.gaos.lastURL, f.gaos.lastReq)
	}
	a := readApproval(t, f.d, "qa-job-r-pr")
	if a.status != "approved" || a.source != caseauto.SourceHistory || a.actor != "tester" || a.by != "dashboard" || !strings.Contains(a.detail, "qa-tool.sismedika.com") {
		t.Fatalf("%+v", a)
	}
	// GAOS also delivers the webhook: first answer wins, nothing is overwritten.
	ans := inboundCall(f.r, "POST", "/api/v1/gaos/inbox-answer", "Bearer "+inboundTok,
		`{"externalId":"qa-job-r-pr","productId":"p-6","status":"answered","optionLabel":"Approve","text":"lgtm","source":`+relaySource+`}`)
	if ans.Code != 200 || !strings.Contains(ans.Body.String(), `"changed":false`) {
		t.Fatalf("webhook %d %s", ans.Code, ans.Body)
	}
	if b := readApproval(t, f.d, "qa-job-r-pr"); b.source != caseauto.SourceHistory {
		t.Fatalf("webhook overwrote the source: %+v", b)
	}
	if ev := jobEventTexts(t, f.d, "job-r"); !strings.Contains(ev, "answered from the run history by tester: approve (sent to GAOS)") || strings.Contains(ev, "GAOS Inbox") {
		t.Fatalf("events:\n%s", ev)
	}
}

func TestRelay409AppliesTheFirstAnswer(t *testing.T) {
	f := relayFixture(t)
	other := `{"kind":"gaos","client":"gaos-web","origin":"gaos.sismedika.com","actor":{"username":"budi","display":"Budi"}}`
	f.gaos.answer = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(strings.Replace(answered("qa-job-r-pr", "Reject", "not now", other), `{"externalId"`, `{"error":"already_answered","externalId"`, 1)))
	}
	w, m := postAnswer(f.r, asUser("QA"), histApprove)
	if w.Code != 409 || m["code"] != "answered_in_gaos" || m["status"] != "rejected" || m["actor"] != "Budi" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	a := readApproval(t, f.d, "qa-job-r-pr")
	if a.status != "rejected" || a.source != caseauto.SourceGAOS || a.actor != "Budi" || a.by != "gaos" || !strings.Contains(a.detail, "budi") {
		t.Fatalf("%+v", a)
	}
	if ev := jobEventTexts(t, f.d, "job-r"); !strings.Contains(ev, "answered first in the GAOS Inbox: rejected (by Budi)") {
		t.Fatalf("events:\n%s", ev)
	}
}

func TestRelay409OfOurOwnAnswerIsOurs(t *testing.T) {
	f := relayFixture(t)
	f.gaos.answer = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(answered("qa-job-r-pr", "Approve", "lgtm", relaySource)))
	}
	w, m := postAnswer(f.r, asUser("QA"), histApprove)
	if w.Code != 200 || m["delivered"] != "gaos" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if a := readApproval(t, f.d, "qa-job-r-pr"); a.source != caseauto.SourceHistory || a.status != "approved" {
		t.Fatalf("%+v", a)
	}
}

func TestRelay409InOwnWordsFallsBackLocally(t *testing.T) {
	f := relayFixture(t)
	f.gaos.answer = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"already_answered","externalId":"qa-job-r-pr","status":"answered","choice":4,"optionLabel":"","text":"maybe later"}`))
	}
	w, m := postAnswer(f.r, asUser("QA"), histApprove)
	if w.Code != 200 || m["delivered"] != "local" || !strings.Contains(m["note"].(string), "own words") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestRelayFallsBackLocally(t *testing.T) {
	for _, c := range []struct {
		name   string
		status int
		note   string
	}{
		{"expired", 410, "expired in GAOS"},
		{"old GAOS without the route", 404, "no answer endpoint"},
		{"server error", 500, "HTTP 500"},
		{"bad choice", 422, "configuration error"},
		{"token", 401, "configuration error"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := relayFixture(t)
			f.gaos.answer = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(c.status); _, _ = w.Write([]byte(`{"error":"x","secret":"body"}`)) }
			w, m := postAnswer(f.r, asUser("QA"), histApprove)
			note, _ := m["note"].(string)
			if w.Code != 200 || m["delivered"] != "local" || !strings.Contains(note, c.note) || strings.Contains(note, "secret") || strings.Contains(note, "http://") {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if a := readApproval(t, f.d, "qa-job-r-pr"); a.status != "approved" || a.source != caseauto.SourceHistory || a.actor != "tester" {
				t.Fatalf("%+v", a)
			}
			if ev := jobEventTexts(t, f.d, "job-r"); !strings.Contains(ev, "warn: answered from the run history by tester: approve") {
				t.Fatalf("events:\n%s", ev)
			}
		})
	}
}

func TestRelayTimeout(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}
	t.Run("outcome unknown", func(t *testing.T) {
		f := relayFixture(t)
		f.gaos.answer = slow
		f.gaos.get = func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"externalId":"qa-job-r-pr","status":"open"}`))
		}
		w, m := postAnswer(f.r, asUser("QA"), histApprove)
		if w.Code != 200 || m["delivered"] != "local" || !strings.Contains(m["note"].(string), "unknown") || f.gaos.gets.Load() != 1 {
			t.Fatalf("%d %s gets=%d", w.Code, w.Body, f.gaos.gets.Load())
		}
	})
	t.Run("GAOS had taken it", func(t *testing.T) {
		f := relayFixture(t)
		f.gaos.answer = slow
		f.gaos.get = func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(answered("qa-job-r-pr", "Approve", "lgtm", relaySource)))
		}
		w, m := postAnswer(f.r, asUser("QA"), histApprove)
		if w.Code != 200 || m["delivered"] != "gaos" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if a := readApproval(t, f.d, "qa-job-r-pr"); a.source != caseauto.SourceHistory || a.status != "approved" {
			t.Fatalf("%+v", a)
		}
	})
}

func TestRelayNotAttemptedWhenGAOSIsNotInvolved(t *testing.T) {
	for name, setup := range map[string]string{
		"product not integrated":   `UPDATE product_settings SET gaos_integrated = 0`,
		"item never made in GAOS":  `UPDATE case_approvals SET gaos_created = 0`,
		"GAOS product id rejected": `UPDATE product_settings SET gaos_product_id = '../x'`,
	} {
		t.Run(name, func(t *testing.T) {
			f := relayFixture(t)
			must(t, f.d, setup)
			w, m := postAnswer(f.r, asUser("QA"), histApprove)
			if w.Code != 200 || m["delivered"] != "local" || m["note"] != nil || f.gaos.posts.Load() != 0 {
				t.Fatalf("%d %s posts=%d", w.Code, w.Body, f.gaos.posts.Load())
			}
		})
	}
}

func TestRelayWebhookBeforeOurAnswerIsClaimed(t *testing.T) {
	f := relayFixture(t)
	f.gaos.answer = func(w http.ResponseWriter, r *http.Request) {
		// GAOS delivers its webhook before it answers our call.
		ans := inboundCall(f.r, "POST", "/api/v1/gaos/inbox-answer", "Bearer "+inboundTok,
			`{"externalId":"qa-job-r-pr","productId":"p-6","status":"answered","optionLabel":"Approve","text":"lgtm"}`)
		if ans.Code != 200 {
			t.Errorf("webhook %d", ans.Code)
		}
		_, _ = w.Write([]byte(answered("qa-job-r-pr", "Approve", "lgtm", relaySource)))
	}
	w, m := postAnswer(f.r, asUser("QA"), histApprove)
	if w.Code != 200 || m["delivered"] != "gaos" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	a := readApproval(t, f.d, "qa-job-r-pr")
	if a.status != "approved" || a.source != caseauto.SourceHistory || a.actor != "tester" || !strings.Contains(a.detail, "qa-tool.sismedika.com") {
		t.Fatalf("not claimed: %+v", a)
	}
	if ev := jobEventTexts(t, f.d, "job-r"); !strings.Contains(ev, "came from the run history by tester (relayed)") {
		t.Fatalf("events:\n%s", ev)
	}
}

func TestRelayJobPageAnsweredDuringTheRelay(t *testing.T) {
	f := relayFixture(t)
	f.gaos.answer = func(w http.ResponseWriter, r *http.Request) {
		if jw := dispatchAs(f.r, asUser("Developer"), http.MethodPost, "/api/jobs/job-r/approval", `{"decision":"reject"}`); jw.Code != 200 {
			t.Errorf("job page: %d %s", jw.Code, jw.Body)
		}
		_, _ = w.Write([]byte(answered("qa-job-r-pr", "Approve", "lgtm", relaySource)))
	}
	w, m := postAnswer(f.r, asUser("QA"), histApprove)
	if w.Code != 409 || m["code"] != "answered_here" || m["status"] != "rejected" || m["source"] != caseauto.SourceJobPage {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if a := readApproval(t, f.d, "qa-job-r-pr"); a.status != "rejected" || a.source != caseauto.SourceJobPage {
		t.Fatalf("%+v", a)
	}
	if ev := jobEventTexts(t, f.d, "job-r"); !strings.Contains(ev, "warn: GAOS recorded approved") {
		t.Fatalf("events:\n%s", ev)
	}
}

func TestApprovalRequestRules(t *testing.T) {
	f := relayFixture(t)
	for _, c := range []struct {
		body string
		code int
		key  string
	}{
		{`{"decision":"approve","via":"history"}`, 400, ""},
		{`{"decision":"approve","via":"phone"}`, 400, ""},
		{`{"decision":"approve","via":"history","external_id":"qa-job-r-pr0"}`, 409, "newer_round"},
	} {
		w, m := postAnswer(f.r, asUser("QA"), c.body)
		if w.Code != c.code || (c.key != "" && m["code"] != c.key) {
			t.Errorf("%s: %d %s", c.body, w.Code, w.Body)
		}
	}
	unlock, ok := f.r.approvalRelay.tryLock("qa-job-r-pr")
	if !ok {
		t.Fatal("lock")
	}
	if w, m := postAnswer(f.r, asUser("QA"), histApprove); w.Code != 409 || m["code"] != "in_flight" {
		t.Errorf("in flight: %d %s", w.Code, w.Body)
	}
	unlock()
	if w, _ := postAnswer(f.r, asUser("Viewer"), histApprove); w.Code != 403 {
		t.Errorf("viewer: %d", w.Code)
	}
	if f.gaos.posts.Load() != 0 {
		t.Errorf("GAOS was called %d times", f.gaos.posts.Load())
	}
}

func TestJobPageDoorRecordsItsSourceAndImpersonation(t *testing.T) {
	f := relayFixture(t)
	imp := auth.SetUser(context.Background(), &auth.User{ID: "u2", Username: "ardi", FullName: "Ardi", Role: "QA",
		Impersonator: &auth.Actor{ID: "s1", Username: "ai-service", Role: "Service"}})
	w, m := postAnswer(f.r, imp, `{"decision":"changes","text":"rename the spec"}`)
	if w.Code != 200 || m["delivered"] != "local" || f.gaos.posts.Load() != 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	a := readApproval(t, f.d, "qa-job-r-pr")
	if a.status != "changes" || a.source != caseauto.SourceJobPage || a.actor != "ai-service as ardi" {
		t.Fatalf("%+v", a)
	}
	// A second answer is refused with the code and who answered.
	if w, m := postAnswer(f.r, asUser("QA"), `{"decision":"approve"}`); w.Code != 409 || m["code"] != "already_answered" || m["actor"] != "ai-service as ardi" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	ev := getJSON(t, f.r, "/api/jobs/job-r/events", 200)
	if ap := ev["approval"].(map[string]any); ap["source"] != caseauto.SourceJobPage || ap["actor"] != "ai-service as ardi" || ap["answered_at"] == nil {
		t.Fatalf("events approval %v", ap)
	}
}

func TestRelayImpersonatedActorDisplay(t *testing.T) {
	f := relayFixture(t)
	f.gaos.answer = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(answered("qa-job-r-pr", "Approve", "", `{"kind":"qa-tool","actor":{"username":"ardi","display":"ai-service as ardi"}}`)))
	}
	imp := auth.SetUser(context.Background(), &auth.User{ID: "u2", Username: "ardi", Role: "QA",
		Impersonator: &auth.Actor{ID: "s1", Username: "ai-service", Role: "Service"}})
	if w, _ := postAnswer(f.r, imp, `{"decision":"approve","via":"history","external_id":"qa-job-r-pr"}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if f.gaos.lastReq.Actor.Username != "ardi" || f.gaos.lastReq.Actor.Display != "ai-service as ardi" {
		t.Fatalf("%+v", f.gaos.lastReq.Actor)
	}
	if a := readApproval(t, f.d, "qa-job-r-pr"); a.actor != "ai-service as ardi" {
		t.Fatalf("%+v", a)
	}
}

func TestHistoryApprovalsAndBuildsApproval(t *testing.T) {
	f := relayFixture(t)
	now := time.Now().UTC()
	must(t, f.d, `INSERT INTO job_runs (id, type, status, payload, created_at, started_at) VALUES ('job-p','policy_propose','running',?,?,?)`,
		`{"product_id":1,"spec_path":".claude/rules/learned/x.md","draft":{"title":"Use typed clients"}}`, now, now)
	must(t, f.d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at) VALUES ('job-p',1,'','qa-job-p-pr','changes',?), ('job-p',1,'','qa-job-p-pr2','open',?)`, now.Add(-time.Hour), now)
	must(t, f.d, `INSERT INTO job_runs (id, type, status, payload, created_at, completed_at) VALUES ('job-done','case_create','completed','{"product_id":1,"case_code":"TC-AUTH-002"}',?,?)`, now, now)
	must(t, f.d, `INSERT INTO case_approvals (job_id, product_id, case_code, external_id, status, answer_source, answer_actor, created_at, answered_at) VALUES ('job-done',1,'TC-AUTH-002','qa-job-done-pr','approved','gaos','Budi',?,?)`, now, now)

	m := getJSON(t, f.r, "/api/history/approvals", 200)
	if m["can_answer"] != false { // getJSON runs without a signed-in role
		t.Errorf("can_answer %v", m["can_answer"])
	}
	its := items(m)
	if len(its) != 2 {
		t.Fatalf("%v", its)
	}
	byJob := map[string]map[string]any{}
	for _, it := range its {
		byJob[it["job_id"].(string)] = it
	}
	c := byJob["job-r"]
	if c["job_type"] != "case_create" || c["case_code"] != "TC-AUTH-001" || c["spec_path"] != "tests/auth/login.spec.ts" || c["relay"] != true || c["round"] != float64(1) || c["product_name"] != "HIS" {
		t.Errorf("case_create %v", c)
	}
	if rep, _ := c["report"].(map[string]any); rep["passed"] != float64(3) || rep["skipped"] != float64(1) {
		t.Errorf("report %v", c["report"])
	}
	p := byJob["job-p"]
	if p["job_type"] != "policy_propose" || p["title"] != "Use typed clients" || p["round"] != float64(2) || p["relay"] != false || p["external_id"] != "qa-job-p-pr2" {
		t.Errorf("policy %v", p)
	}
	if w := dispatchAs(f.r, asUser("QC"), http.MethodGet, "/api/history/approvals?product=1", ""); !strings.Contains(w.Body.String(), `"can_answer":true`) {
		t.Errorf("QC: %s", w.Body)
	}
	if w := dispatchAs(f.r, asUser("QC"), http.MethodGet, "/api/history/approvals?product=x", ""); w.Code != 400 {
		t.Errorf("bad product: %d", w.Code)
	}

	b := getJSON(t, f.r, "/api/history/builds?approval=open", 200)
	if bi := items(b); len(bi) != 1 || bi[0]["id"] != "job-r" || bi[0]["approval"].(map[string]any)["status"] != "open" {
		t.Fatalf("approval=open %v", bi)
	}
	all := getJSON(t, f.r, "/api/history/builds", 200)
	for _, it := range items(all) {
		if it["id"] == "job-done" {
			ap := it["approval"].(map[string]any)
			if ap["source"] != "gaos" || ap["actor"] != "Budi" || ap["answered_at"] == nil {
				t.Errorf("job-done approval %v", ap)
			}
		}
	}
	if w := dispatchAs(f.r, asUser("QA"), http.MethodGet, "/api/history/builds?approval=closed", ""); w.Code != 400 {
		t.Errorf("approval=closed: %d", w.Code)
	}
}

func TestExpirySourceIsSystemAndTheMigrationReruns(t *testing.T) {
	f := relayFixture(t)
	st := &caseauto.ApprovalStore{DB: f.d}
	if ok, err := st.Answer(context.Background(), "qa-job-r-pr", caseauto.ApprovalExpired, "gaos", ""); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if a := readApproval(t, f.d, "qa-job-r-pr"); a.source != caseauto.SourceSystem || a.by != "gaos" {
		t.Fatalf("%+v", a)
	}
	if err := db.RunMigrations(f.d.SQL); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
	var n int
	_ = f.d.QueryRow(context.Background(), `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'case_approvals'
		AND column_name IN ('answer_source','answer_actor','answer_source_detail')`).Scan(&n)
	if n != 3 {
		t.Fatalf("columns %d", n)
	}
}
