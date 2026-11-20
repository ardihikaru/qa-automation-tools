package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/conversations"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// AI_CONVERSATIONS.md. Needs QA_TEST_MYSQL_DSN.

var (
	viewer   = &auth.User{ID: "u-viewer", Username: "viewer", Role: "Viewer"}
	qa       = &auth.User{ID: "u-qa", Username: "qa1", Role: "QA"}
	admin    = &auth.User{ID: "u-admin", Username: "boss", Role: "Admin"}
	svcAsAdm = &auth.User{ID: "u-admin", Username: "boss", Role: "Admin", Impersonator: &auth.Actor{ID: "u-svc", Username: "ai-service", Role: "Service"}}
)

func convCall(r *Router, u *auth.User, method, path, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" || method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	if u != nil {
		req = req.WithContext(auth.SetUser(req.Context(), u))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w.Code, m
}

func TestConversationRoutesDB(t *testing.T) {
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	ctx := context.Background()
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'HIS','r1','s1','d1')`)
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at, started_at) VALUES
		('job-live','case_create','running','{"product_id":1,"case_code":"TC-AUTH-001"}',UTC_TIMESTAMP(),UTC_TIMESTAMP()),
		('job-done','case_create','completed','{"product_id":1,"case_code":"TC-AUTH-002"}',UTC_TIMESTAMP(),UTC_TIMESTAMP())`)
	must(t, d, `INSERT INTO kb_syncs (id, product_id, status, trigger_kind, started_at, finished_at, ai_status) VALUES (5,1,'succeeded','manual',UTC_TIMESTAMP(),UTC_TIMESTAMP(),'done')`)
	st := &conversations.Store{DB: d, Screen: conversations.NewScreen()}
	for _, ref := range []ai.ConvRef{{Kind: "job", Ref: "job-live", ProductID: 1, Title: "TC-AUTH-001"}, {Kind: "job", Ref: "job-done", ProductID: 1, Title: "TC-AUTH-002"}, {Kind: "sync", Ref: "5", ProductID: 1, Title: "HIS"}} {
		sent := []models.Message{{Role: "user", Kind: "prompt", Content: "prompt"}}
		if err := st.SaveExchange(ctx, ai.Exchange{Ref: ref, Feature: "f", Sent: sent, Reply: "- a\n  - b\n\n`code`"}); err != nil {
			t.Fatal(err)
		}
	}
	doneID := func() string {
		it, _ := conversations.GetByRef(ctx, d, "job", "job-done")
		return strconv.FormatInt(it.ID, 10)
	}()

	// Reading: the list is open to everyone, the messages to canVerify only.
	if code, m := convCall(r, viewer, "GET", "/api/conversations", ""); code != 200 || m["total"] != float64(3) || m["totals"] == nil {
		t.Fatalf("list %d %v", code, m)
	}
	if code, m := convCall(r, viewer, "GET", "/api/conversations/"+doneID, ""); code != 403 || m["code"] != "forbidden" {
		t.Errorf("viewer detail %d %v", code, m)
	}
	code, m := convCall(r, qa, "GET", "/api/conversations/"+doneID+"?since_seq=1", "")
	if code != 200 || len(m["messages"].([]any)) != 1 || m["conversation"].(map[string]any)["title"] != "TC-AUTH-002" {
		t.Fatalf("qa detail %d %v", code, m)
	}
	if _, m := convCall(r, viewer, "GET", "/api/jobs/job-done/conversation", ""); m["restricted"] != true || len(m["messages"].([]any)) != 0 {
		t.Errorf("viewer job route %v", m)
	}
	if _, m := convCall(r, qa, "GET", "/api/jobs/job-done/conversation", ""); len(m["messages"].([]any)) != 2 {
		t.Errorf("qa job route %v", m)
	}
	if _, m := convCall(r, qa, "GET", "/api/jobs/job-none/conversation", ""); m["conversation"] != nil {
		t.Errorf("no conversation %v", m)
	}
	// The history links the job and the sync to their conversations.
	if _, m := convCall(r, qa, "GET", "/api/history/builds?case=TC-AUTH-002", ""); items(m)[0]["conversation_id"] == nil {
		t.Errorf("builds link %v", m)
	}
	if _, m := convCall(r, qa, "GET", "/api/history/syncs", ""); items(m)[0]["conversation_id"] == nil {
		t.Errorf("syncs link %v", m)
	}

	// Archiving: canVerify, JSON, not while the job runs; audit names the real actor.
	liveID := func() string {
		it, _ := conversations.GetByRef(ctx, d, "job", "job-live")
		return strconv.FormatInt(it.ID, 10)
	}()
	if code, _ := convCall(r, viewer, "POST", "/api/conversations/"+doneID+"/archive", "{}"); code != 403 {
		t.Errorf("viewer archive %d", code)
	}
	req := httptest.NewRequest("POST", "/api/conversations/"+doneID+"/archive", nil)
	req = req.WithContext(auth.SetUser(req.Context(), qa))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 415 {
		t.Errorf("no JSON content type: %d", w.Code)
	}
	if code, m := convCall(r, qa, "POST", "/api/conversations/"+liveID+"/archive", "{}"); code != 409 || m["code"] != "active" {
		t.Errorf("running job archive %d %v", code, m)
	}
	if code, m := convCall(r, svcAsAdm, "POST", "/api/conversations/"+doneID+"/archive", "{}"); code != 200 || m["archived_at"] == nil {
		t.Fatalf("archive %d %v", code, m)
	}
	if code, m := convCall(r, qa, "POST", "/api/conversations/"+doneID+"/archive", "{}"); code != 409 || m["code"] != "already_archived" {
		t.Errorf("twice %d %v", code, m)
	}
	if _, m := convCall(r, qa, "GET", "/api/jobs/job-done/conversation", ""); m["conversation"].(map[string]any)["archived_at"] == nil || len(m["messages"].([]any)) != 0 {
		t.Errorf("archived job route %v", m)
	}
	if _, m := convCall(r, qa, "GET", "/api/history/builds?case=TC-AUTH-002", ""); items(m)[0]["conversation_id"] != nil {
		t.Errorf("archived conversation still linked %v", m)
	}
	if _, m := convCall(r, qa, "GET", "/api/conversations?archived=1", ""); m["total"] != float64(1) {
		t.Errorf("archived list %v", m)
	}
	var actor, as string
	_ = d.QueryRow(ctx, `SELECT actor_id, COALESCE(as_user_id, '') FROM ai_conversation_audit WHERE action = 'archive'`).Scan(&actor, &as)
	if actor != "u-svc" || as != "u-admin" {
		t.Errorf("audit actor %q as %q", actor, as)
	}

	// Prune: a real Admin only, two steps.
	for _, u := range []*auth.User{qa, svcAsAdm} {
		if code, m := convCall(r, u, "GET", "/api/conversations/prune", ""); code != 403 || m["code"] != "real_admin_only" {
			t.Errorf("%s preview %d %v", u.Username, code, m)
		}
		if code, _ := convCall(r, u, "POST", "/api/conversations/prune", `{"confirm":"prune","archived_before":"2099-01-01T00:00:00.000Z"}`); code != 403 {
			t.Errorf("%s prune %d", u.Username, code)
		}
	}
	code, prev := convCall(r, admin, "GET", "/api/conversations/prune", "")
	if code != 200 || prev["conversations"] != float64(1) || prev["archived_before"] == nil {
		t.Fatalf("preview %d %v", code, prev)
	}
	cut := prev["archived_before"].(string)
	for _, bad := range []string{`{}`, `{"confirm":"yes","archived_before":"` + cut + `"}`, `{"confirm":"prune"}`, `{"confirm":"prune","archived_before":"2026-11-21"}`, `{"confirm":"prune","archived_before":"` + cut + `","x":1}`} {
		if code, m := convCall(r, admin, "POST", "/api/conversations/prune", bad); code != 400 || m["code"] != "confirm" {
			t.Errorf("%s: %d %v", bad, code, m)
		}
	}
	code, m = convCall(r, admin, "POST", "/api/conversations/prune", `{"confirm":"prune","archived_before":"`+cut+`"}`)
	if code != 200 || m["conversations"] != float64(1) || m["messages"] != float64(2) {
		t.Fatalf("prune %d %v", code, m)
	}
	if code, m := convCall(r, admin, "POST", "/api/conversations/prune", `{"confirm":"prune","archived_before":"`+cut+`"}`); code != 409 || m["code"] != "nothing" {
		t.Errorf("again %d %v", code, m)
	}
	var jobs int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs`).Scan(&jobs)
	if jobs != 2 {
		t.Errorf("a prune touched job_runs: %d", jobs)
	}
	if code, m := convCall(r, admin, "GET", "/api/conversations/prunes", ""); code != 200 || len(m["items"].([]any)) != 1 {
		t.Errorf("prune log %d %v", code, m)
	}
	if code, _ := convCall(r, qa, "GET", "/api/conversations/prunes", ""); code != 403 {
		t.Errorf("qa prune log %d", code)
	}
	for _, q := range []string{"archived=2", "kind=x", "product=x", "q=a%20b"} {
		if code, _ := convCall(r, qa, "GET", "/api/conversations?"+q, ""); code != 400 {
			t.Errorf("%s: %d", q, code)
		}
	}
	if code, _ := convCall(r, qa, "GET", "/api/conversations/999999", ""); code != 404 {
		t.Errorf("absent %d", code)
	}
}
