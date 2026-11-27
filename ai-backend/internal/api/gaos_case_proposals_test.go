package api

// GAOS_CASE_PROPOSALS.md. The guard and the envelope need no database; the writes run on a scratch
// schema (QA_TEST_MYSQL_DSN) and skip without one.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func TestGAOSCaseProposalsIsBearerGuarded(t *testing.T) {
	if !IsGAOSInboundPath(gaosCaseProposalsPath) {
		t.Fatal("the case-proposals path must bypass the cookie")
	}
	r := NewRouter(nil, nil, nil, nil)
	if w := inboundCall(r, "POST", gaosCaseProposalsPath, "Bearer x", `{}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("token unset: got %d, want 503", w.Code)
	}
	r.SetGAOSInboundToken(inboundTok)
	if w := inboundCall(r, "POST", gaosCaseProposalsPath, "Bearer wrong", `{}`); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: got %d, want 401", w.Code)
	}
	if w := inboundCall(r, "POST", gaosCaseProposalsPath, "Bearer "+inboundTok, `{}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no db: got %d, want 503", w.Code)
	}
}

func TestGAOSDraftCodeIsPositionalAndLocal(t *testing.T) {
	a := gaosDraftCode("p-2", "taskmgr:ABC-12", 0)
	if a != gaosDraftCode("p-2", "taskmgr:ABC-12", 0) {
		t.Error("the same bundle and index must give the same code")
	}
	if a == gaosDraftCode("p-2", "taskmgr:ABC-12", 1) || a == gaosDraftCode("p-3", "taskmgr:ABC-12", 0) {
		t.Error("another index or product must give another code")
	}
	for i := 0; i < gaosMaxProposals; i++ {
		if c := gaosDraftCode("p-2", "pr:acme/backend#42", i); !knowledge.LocalCodeRe.MatchString(c) {
			t.Errorf("%q is not a local draft code", c)
		}
	}
}

func TestGAOSProposalsEnvelope(t *testing.T) {
	ok := gaosProposalsBody{Product: "p-2", BundleKey: "taskmgr:ABC-12", TaskCode: "ABC-12",
		PRURLs: []string{"https://github.com/acme/backend/pull/42"}, Cases: []gaosProposalCase{{Action: "link"}}}
	if why := validateProposals(&ok); why != "" {
		t.Fatalf("a good envelope was refused: %s", why)
	}
	bad := []func(b *gaosProposalsBody){
		func(b *gaosProposalsBody) { b.BundleKey = "ABC-12" },
		func(b *gaosProposalsBody) { b.TaskCode = "a/../b" },
		func(b *gaosProposalsBody) { b.PRURLs = nil },
		func(b *gaosProposalsBody) { b.PRURLs = []string{"http://github.com/acme/backend/pull/42"} },
		func(b *gaosProposalsBody) { b.PRURLs = []string{"https://evil.example/acme/backend/pull/42"} },
		func(b *gaosProposalsBody) { b.Cases = nil },
		func(b *gaosProposalsBody) { b.Cases = make([]gaosProposalCase, 21) },
	}
	for i, f := range bad {
		b := ok
		f(&b)
		if validateProposals(&b) == "" {
			t.Errorf("bad envelope %d was accepted", i)
		}
	}
}

func proposalsFixture(t *testing.T) *Router {
	t.Helper()
	d := testdb.New(t)
	r := NewRouter(d, nil, nil, nil)
	r.SetGAOSInboundToken(inboundTok)
	r.SetKnowledge(&knowledge.Syncer{DB: d})
	must(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1')`)
	must(t, d, `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (1,1,'p-6')`)
	must(t, d, `INSERT INTO kb_epics (product_id, code, title, content_hash) VALUES (1,'OUTREG','Outpatient registration',?)`, hash64)
	addCase(t, d, "TC-OUTREG-004", "OUTREG", "planned", "", "P1", `[]`)
	return r
}

func proposals(t *testing.T, r *Router, body string) (int, []gaosProposalResult) {
	t.Helper()
	w := inboundCall(r, "POST", gaosCaseProposalsPath, "Bearer "+inboundTok, body)
	var out struct{ Results []gaosProposalResult }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Results
}

func TestGAOSCaseProposalsWrites(t *testing.T) {
	r := proposalsFixture(t)
	body := `{"product":"p-6","bundleKey":"taskmgr:ABC-12","taskCode":"ABC-12",
 "prUrls":["https://github.com/acme/backend/pull/42","https://github.com/acme/web/pull/7"],
 "cases":[{"action":"create","epic":"OUTREG","title":"Register a walk-in","scenario":"Given ...","polarity":"+","level":"E2E","priority":"P2"},
  {"action":"link","caseCode":"TC-OUTREG-004","note":"re-test the date filter"},
  {"action":"create","epic":"NOPE","title":"x","polarity":"+","level":"E2E","priority":"P2"},
  {"action":"create","epic":"OUTREG","title":"x","polarity":"?","level":"E2E","priority":"P2"},
  {"action":"link","caseCode":"TC-OUTREG-999"},
  {"action":"delete"}]}`
	code, res := proposals(t, r, body)
	if code != http.StatusOK || len(res) != 6 {
		t.Fatalf("got %d with %d results, want 200 with 6", code, len(res))
	}
	want := []string{"created", "linked", "refused", "refused", "refused", "refused"}
	for i, o := range want {
		if res[i].Outcome != o || res[i].Index != i {
			t.Errorf("item %d: %+v, want %s", i, res[i], o)
		}
	}
	if res[0].CaseCode != gaosDraftCode("p-6", "taskmgr:ABC-12", 0) {
		t.Errorf("the draft code is %q", res[0].CaseCode)
	}
	d := r.getDB()
	var origin, status, scen, detail string
	if err := d.QueryRow(context.Background(), `SELECT origin, status, scenario, reason_detail FROM kb_test_cases WHERE product_id=1 AND code=?`, res[0].CaseCode).Scan(&origin, &status, &scen, &detail); err != nil {
		t.Fatal(err)
	}
	if origin != "local" || status != "planned" || !strings.HasPrefix(scen, "Register a walk-in") || !strings.Contains(detail, "https://github.com/acme/web/pull/7") {
		t.Errorf("the draft is origin=%s status=%s scenario=%q detail=%q", origin, status, scen, detail)
	}
	var n int
	_ = d.QueryRow(context.Background(), `SELECT COUNT(*) FROM kb_case_pr_links WHERE product_id=1 AND case_code='TC-OUTREG-004'`).Scan(&n)
	if n != 2 {
		t.Errorf("%d links, want one per pull request URL (2)", n)
	}
	// A replay updates the draft and adds no link.
	_, res = proposals(t, r, body)
	if res[0].Outcome != "updated" || res[1].Outcome != "linked" {
		t.Errorf("a replay answered %+v / %+v, want updated / linked", res[0], res[1])
	}
	_ = d.QueryRow(context.Background(), `SELECT COUNT(*) FROM kb_case_pr_links WHERE product_id=1`).Scan(&n)
	var drafts int
	_ = d.QueryRow(context.Background(), `SELECT COUNT(*) FROM kb_test_cases WHERE product_id=1 AND origin='local'`).Scan(&drafts)
	if n != 2 || drafts != 1 {
		t.Errorf("a replay left %d links and %d drafts, want 2 and 1", n, drafts)
	}
	// The case page lists its links.
	if links := r.casePRLinks(context.Background(), 1, "TC-OUTREG-004"); len(links) != 2 {
		t.Errorf("the case lists %d links, want 2", len(links))
	}
	// An unknown product is 404; a malformed body 400.
	if c, _ := proposals(t, r, strings.Replace(body, `"p-6"`, `"p-9"`, 1)); c != http.StatusNotFound {
		t.Errorf("unknown product: %d, want 404", c)
	}
	if c, _ := proposals(t, r, `{"product":"p-6","extra":1}`); c != http.StatusBadRequest {
		t.Errorf("unknown field: %d, want 400", c)
	}
}
