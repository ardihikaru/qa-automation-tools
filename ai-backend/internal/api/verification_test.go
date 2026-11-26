package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
)

// vcall is kcall with a named user, so verifications carry an id and a name.
func vcall(t *testing.T, r *Router, u auth.User, method, path string, body any, want int) map[string]any {
	t.Helper()
	var rd *bytes.Reader
	if s, ok := body.(string); ok {
		rd = bytes.NewReader([]byte(s))
	} else {
		b, _ := json.Marshal(body)
		if body == nil {
			b = nil
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req = req.WithContext(auth.SetUser(req.Context(), &u))
	rec := httptest.NewRecorder()
	r.handleProducts(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s as %s: want %d, got %d: %s", method, path, u.Role, want, rec.Code, rec.Body.String())
	}
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return out
}

func TestVerificationAPI(t *testing.T) {
	r := newDBRouter(t)
	golden := loadGolden(t)
	sy := knowledge.NewSyncer(r.db, nil, nil)
	sy.ReadTree = func(context.Context, int, string, string, func(string) bool, int64) (string, map[string][]byte, error) {
		files := map[string][]byte{}
		for k, v := range golden {
			files[k] = v
		}
		return "5a3c1e0000000000000000000000000000000000", files, nil
	}
	r.SetKnowledge(sy)
	p := createSUT(t, r)
	base := fmt.Sprintf("/api/products/%d/knowledge", p.ID)
	kcall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"knowledge_format": knowledge.TestCatalogFormatName}, http.StatusOK)
	started := kcall(t, r, "QA", http.MethodPost, base+"/sync", nil, http.StatusAccepted)
	if s := waitKB(t, r, base, started["sync_id"].(float64)); s["status"] != "succeeded" {
		t.Fatalf("sync: %v", s)
	}
	list := kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases?page_size=3", nil, http.StatusOK)
	total := list["total"].(float64)
	items := list["items"].([]any)
	c1 := items[0].(map[string]any)["code"].(string)
	c2 := items[1].(map[string]any)["code"].(string)
	epic := items[0].(map[string]any)["epic_code"].(string)
	if items[0].(map[string]any)["verification"] != nil {
		t.Fatalf("fresh case must be unverified: %v", items[0])
	}

	ann := auth.User{ID: "u-ann", FullName: "Ann", Username: "ann", Role: "QA"}
	bob := auth.User{ID: "u-bob", Username: "bob", Role: "Developer"}
	url := func(c string) string { return base + "/test-cases/" + c + "/verification" }

	// Permissions, strict body, unknown case.
	vcall(t, r, auth.User{ID: "v", Role: "Viewer"}, http.MethodPut, url(c1), map[string]any{"verdict": "valid"}, http.StatusForbidden)
	vcall(t, r, ann, http.MethodPut, url(c1), map[string]any{"verdict": "maybe"}, http.StatusBadRequest)
	vcall(t, r, ann, http.MethodPut, url(c1), `{}`, http.StatusBadRequest)
	vcall(t, r, ann, http.MethodPut, url(c1), `{"verdict":"valid","x":1}`, http.StatusBadRequest)
	vcall(t, r, ann, http.MethodPut, url(c1), map[string]any{"verdict": "valid", "note": string(make([]rune, 501))}, http.StatusBadRequest)
	vcall(t, r, ann, http.MethodPut, url("NOPE-999"), map[string]any{"verdict": "valid"}, http.StatusNotFound)
	vcall(t, r, ann, http.MethodGet, url(c1), nil, http.StatusMethodNotAllowed)

	// Mark, overwrite, filters, verifiers.
	v := vcall(t, r, ann, http.MethodPut, url(c1), map[string]any{"verdict": "valid", "note": "ok"}, http.StatusOK)
	if v["verdict"] != "valid" || v["verified_by"] != "u-ann" || v["verified_by_name"] != "Ann" || v["note"] != "ok" {
		t.Fatalf("verification: %v", v)
	}
	if _, err := time.Parse(time.RFC3339, v["verified_at"].(string)); err != nil {
		t.Fatalf("verified_at: %v", v["verified_at"])
	}
	vcall(t, r, bob, http.MethodPut, url(c2), map[string]any{"verdict": "invalid"}, http.StatusOK)
	vcall(t, r, bob, http.MethodPut, url(c1), map[string]any{"verdict": "invalid", "note": "changed"}, http.StatusOK) // latest verifier wins

	count := func(q string) float64 {
		return kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases?"+q, nil, http.StatusOK)["total"].(float64)
	}
	if count("verified=any") != 2 || count("verified=valid") != 0 || count("verified=invalid") != 2 || count("verified=none") != total-2 {
		t.Fatalf("verified filters: any=%v valid=%v invalid=%v none=%v", count("verified=any"), count("verified=valid"), count("verified=invalid"), count("verified=none"))
	}
	if count("verified_by=u-bob") != 2 || count("verified_by=u-ann") != 0 || count("verified=invalid&verified_by=u-bob&epic="+epic) < 1 {
		t.Fatal("verified_by filter")
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases?verified=bogus", nil, http.StatusBadRequest)
	kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases?verified=none&verified_by=u-bob", nil, http.StatusBadRequest)

	got := kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases/"+c1, nil, http.StatusOK)
	ver := got["verification"].(map[string]any)
	if ver["verdict"] != "invalid" || ver["verified_by_name"] != "bob" || got["verification_count"] != float64(2) {
		t.Fatalf("detail: %v", got)
	}
	vs := kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases/verifiers", nil, http.StatusOK)["verifiers"].([]any)
	if len(vs) != 1 || vs[0].(map[string]any)["id"] != "u-bob" || vs[0].(map[string]any)["count"] != float64(2) {
		t.Fatalf("verifiers: %v", vs)
	}

	// Epics: tallies from one grouped query.
	if _, err := r.db.Exec(context.Background(), `INSERT INTO kb_case_results (product_id, case_code, result, run_at) VALUES (?, ?, 'pass', UTC_TIMESTAMP()), (?, ?, 'fail', UTC_TIMESTAMP())`, p.ID, c1, p.ID, c2); err != nil {
		t.Fatal(err)
	}
	var vInvalid, lastPass, lastFail float64
	for _, e := range kcall(t, r, "Viewer", http.MethodGet, base+"/epics", nil, http.StatusOK)["epics"].([]any) {
		m := e.(map[string]any)
		vInvalid += m["verified_invalid"].(float64)
		lastPass += m["last_pass"].(float64)
		lastFail += m["last_fail"].(float64)
		if _, ok := m["verified_valid"]; !ok || m["test_cases"] == nil {
			t.Fatalf("epic item: %v", m)
		}
	}
	if vInvalid != 2 || lastPass != 1 || lastFail != 1 {
		t.Fatalf("epic tallies: invalid=%v pass=%v fail=%v", vInvalid, lastPass, lastFail)
	}

	// Clear: row deleted, event kept; history newest first.
	vcall(t, r, ann, http.MethodPut, url(c1), map[string]any{"verdict": nil}, http.StatusOK)
	if count("verified=any") != 1 {
		t.Fatal("clear must remove the verification")
	}
	h := kcall(t, r, "Viewer", http.MethodGet, url(c1)+"/history", nil, http.StatusOK)
	hist := h["history"].([]any)
	if h["total"] != float64(3) || len(hist) != 3 || hist[0].(map[string]any)["verdict"] != "cleared" || hist[2].(map[string]any)["verdict"] != "valid" {
		t.Fatalf("history: %v", h)
	}
	if h2 := kcall(t, r, "Viewer", http.MethodGet, url(c1)+"/history?limit=1", nil, http.StatusOK); len(h2["history"].([]any)) != 1 || h2["total"] != float64(3) {
		t.Fatalf("history limit: %v", h2)
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases/NOPE-999/verification/history", nil, http.StatusNotFound)
	if got := kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases/"+c1, nil, http.StatusOK); got["verification"] != nil || got["verification_count"] != float64(3) {
		t.Fatalf("detail after clear: %v", got)
	}
}
