package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
)

func loadGolden(t *testing.T) map[string][]byte {
	t.Helper()
	root := "../services/knowledge/testdata/hisv3-521618a"
	files := map[string][]byte{}
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = b
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func kcall(t *testing.T, r *Router, role, method, path string, body any, want int) map[string]any {
	t.Helper()
	code, b := call(t, r, role, method, path, body)
	if code != want {
		t.Fatalf("%s %s as %s: want %d, got %d: %s", method, path, role, want, code, b)
	}
	out := map[string]any{}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func waitKB(t *testing.T, r *Router, base string, id float64) map[string]any {
	t.Helper()
	for i := 0; i < 500; i++ {
		s := kcall(t, r, "QA", http.MethodGet, fmt.Sprintf("%s/syncs/%d", base, int64(id)), nil, http.StatusOK)
		if s["status"] != "running" && s["ai_status"] != "pending" {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sync did not finish")
	return nil
}

func TestKnowledgeAPI(t *testing.T) {
	r := newDBRouter(t)
	golden := loadGolden(t)
	sy := knowledge.NewSyncer(r.db, nil, nil)
	sy.ReadTree = func(context.Context, int, string, string, func(string) bool, int64) (string, map[string][]byte, error) {
		files := map[string][]byte{".claude/agents/e2e-test-engineer.md": []byte("---\nname: e2e-test-engineer\ndescription: Writes E2E specs.\n---\nBody\n"), "AGENTS.md": []byte("# Guide\n")}
		for k, v := range golden {
			files[k] = v
		}
		return "521618a000000000000000000000000000000000", files, nil
	}
	r.SetKnowledge(sy)
	p := createHISv3(t, r)
	base := fmt.Sprintf("/api/products/%d/knowledge", p.ID)

	// Format: only Admin/DevOps may set it; must be registered.
	kcall(t, r, "Developer", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"knowledge_format": knowledge.HISv3FormatName}, http.StatusForbidden)
	kcall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"knowledge_format": "nope"}, http.StatusBadRequest)
	sum := kcall(t, r, "QA", http.MethodGet, base+"/summary", nil, http.StatusOK)
	if sum["configured"] != false {
		t.Fatalf("summary before format: %v", sum)
	}
	kcall(t, r, "QA", http.MethodPost, base+"/sync", nil, http.StatusBadRequest)
	kcall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"knowledge_format": knowledge.HISv3FormatName}, http.StatusOK)

	// Viewer cannot sync; QA can.
	kcall(t, r, "Viewer", http.MethodPost, base+"/sync", nil, http.StatusForbidden)
	started := kcall(t, r, "QA", http.MethodPost, base+"/sync", nil, http.StatusAccepted)
	s := waitKB(t, r, base, started["sync_id"].(float64))
	if s["status"] != "succeeded" {
		t.Fatalf("sync: %v", s)
	}
	counts := s["counts"].(map[string]any)
	if counts["features"] != float64(444) || counts["test_cases"] != float64(1659) || counts["automated"] != float64(456) || counts["stories"] != float64(246) {
		t.Fatalf("counts: %v", counts)
	}

	// Second manual sync: unchanged, empty diff.
	started = kcall(t, r, "QA", http.MethodPost, base+"/sync", nil, http.StatusAccepted)
	s = waitKB(t, r, base, started["sync_id"].(float64))
	d := s["diff"].(map[string]any)
	if d["unchanged"] != true || d["total"].(map[string]any)["added"] != float64(0) {
		t.Fatalf("second diff: %v", d)
	}

	sum = kcall(t, r, "Viewer", http.MethodGet, base+"/summary", nil, http.StatusOK)
	if sum["configured"] != true || sum["latest_sync"] == nil || sum["repo"] == nil {
		t.Fatalf("summary: %v", sum)
	}

	tree := kcall(t, r, "Viewer", http.MethodGet, base+"/tree", nil, http.StatusOK)
	if parts := tree["parts"].([]any); len(parts) != 4 {
		t.Fatalf("tree parts: %d", len(parts))
	}

	// Lists and filters.
	tcs := kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases?status=automated&page_size=10", nil, http.StatusOK)
	if tcs["total"] != float64(456) || len(tcs["items"].([]any)) != 10 {
		t.Fatalf("test-cases: total %v", tcs["total"])
	}
	first := tcs["items"].([]any)[0].(map[string]any)
	if first["automated"] != true || first["source_url"] == nil || first["type"] != "test_case" {
		t.Fatalf("tc item: %v", first)
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases?level=BOGUS", nil, http.StatusBadRequest)
	st := kcall(t, r, "Viewer", http.MethodGet, base+"/stories?feature=A01.07", nil, http.StatusOK)
	if st["total"].(float64) < 1 {
		t.Fatalf("stories of A01.07: %v", st["total"])
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/epics", nil, http.StatusOK)

	// Details.
	f := kcall(t, r, "Viewer", http.MethodGet, base+"/features/A01.07", nil, http.StatusOK)
	if len(f["stories"].([]any)) == 0 {
		t.Fatalf("feature detail: %v", f)
	}
	story := kcall(t, r, "Viewer", http.MethodGet, base+"/stories/US-OUTREG-05", nil, http.StatusOK)
	if len(story["features"].([]any)) == 0 || len(story["test_cases"].([]any)) == 0 {
		t.Fatalf("story detail: %v", story)
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/stories/US-NOPE-01", nil, http.StatusNotFound)
	kcall(t, r, "Viewer", http.MethodGet, base+"/modules/A01", nil, http.StatusOK)
	kcall(t, r, "Viewer", http.MethodGet, base+"/parts/A", nil, http.StatusOK)
	kcall(t, r, "Viewer", http.MethodGet, base+"/epics/OUTREG", nil, http.StatusOK)

	// Coverage at every level.
	cov := kcall(t, r, "Viewer", http.MethodGet, base+"/coverage?by=module", nil, http.StatusOK)["coverage"].(map[string]any)
	a01 := cov["A01"].(map[string]any)
	if a01["test_cases"].(float64) < 1 || a01["state"] == "" {
		t.Fatalf("A01 coverage: %v", a01)
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/coverage?by=bogus", nil, http.StatusBadRequest)
	mods := kcall(t, r, "Viewer", http.MethodGet, base+"/modules", nil, http.StatusOK)["modules"].([]any)
	if len(mods) != 39 || mods[0].(map[string]any)["coverage"] == nil {
		t.Fatalf("modules: %d", len(mods))
	}
	feats := kcall(t, r, "Viewer", http.MethodGet, base+"/features?module=A01&page_size=500", nil, http.StatusOK)
	untested := kcall(t, r, "Viewer", http.MethodGet, base+"/features?coverage=untested&page_size=500", nil, http.StatusOK)
	if feats["total"].(float64) < 1 || untested["total"].(float64) < 300 {
		t.Fatalf("features: module A01 %v, untested %v", feats["total"], untested["total"])
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/features?coverage=meh", nil, http.StatusBadRequest)
	firstFeat := kcall(t, r, "Viewer", http.MethodGet, base+"/features?page_size=1", nil, http.StatusOK)["items"].([]any)[0].(map[string]any)
	if firstFeat["coverage"].(map[string]any)["test_cases"].(float64) == 0 {
		t.Fatalf("default sort must put tested features first: %v", firstFeat["code"])
	}
	kcall(t, r, "Viewer", http.MethodGet, base+"/features?sort=bogus", nil, http.StatusBadRequest)
	fd := kcall(t, r, "Viewer", http.MethodGet, base+"/features/A01.07", nil, http.StatusOK)
	cases := 0
	for _, st := range fd["stories"].([]any) {
		cases += len(st.(map[string]any)["test_cases"].([]any))
		if st.(map[string]any)["coverage"] == nil {
			t.Fatal("story coverage missing")
		}
	}
	if cases == 0 || fd["coverage"].(map[string]any)["test_cases"].(float64) == 0 {
		t.Fatalf("feature detail cases: %d %v", cases, fd["coverage"])
	}
	modTC := kcall(t, r, "Viewer", http.MethodGet, base+"/test-cases?module=A01&automated=0&page_size=5", nil, http.StatusOK)
	if modTC["total"].(float64) < 1 {
		t.Fatalf("test cases of A01 not automated: %v", modTC["total"])
	}
	mod := kcall(t, r, "Viewer", http.MethodGet, base+"/modules/A01", nil, http.StatusOK)
	if mod["coverage"].(map[string]any)["test_cases"] != a01["test_cases"] || mod["features"].([]any)[0].(map[string]any)["coverage"] == nil {
		t.Fatalf("module detail coverage: %v", mod["coverage"])
	}
	stCov := kcall(t, r, "Viewer", http.MethodGet, base+"/stories?coverage=full,partial&page_size=500", nil, http.StatusOK)
	if stCov["total"].(float64) < 1 {
		t.Fatalf("stories with automation: %v", stCov["total"])
	}

	// Assistant files: per product and across products.
	al := kcall(t, r, "Viewer", http.MethodGet, base+"/assistants?kind=agent", nil, http.StatusOK)
	if al["total"] != float64(1) || al["kinds"].(map[string]any)["instructions"] != float64(1) {
		t.Fatalf("assistants: %v", al)
	}
	agent := al["items"].([]any)[0].(map[string]any)
	if _, hasBody := agent["body"]; hasBody || agent["source_url"] == nil || agent["product_name"] != "HISv3" {
		t.Fatalf("assistant item: %v", agent)
	}
	ad := kcall(t, r, "Viewer", http.MethodGet, base+"/assistants/"+agent["code"].(string), nil, http.StatusOK)
	if ad["body"] != "Body\n" {
		t.Fatalf("assistant detail: %v", ad)
	}
	kcall(t, r, "QA", http.MethodPut, base+"/annotations/assistant/"+agent["code"].(string), map[string]any{"note": "ok"}, http.StatusOK)
	kcall(t, r, "Viewer", http.MethodGet, base+"/assistants?kind=bogus", nil, http.StatusBadRequest)
	creq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/assistants?product_id=%d&kind=agent,instructions", p.ID), nil)
	creq = creq.WithContext(auth.SetUser(creq.Context(), &auth.User{Role: "Viewer"}))
	crec := httptest.NewRecorder()
	r.handleAssistants(crec, creq)
	if crec.Code != http.StatusOK || !strings.Contains(crec.Body.String(), `"total":2`) {
		t.Fatalf("cross-product assistants: %d %s", crec.Code, crec.Body.String())
	}

	// Annotations.
	kcall(t, r, "Viewer", http.MethodPut, base+"/annotations/story/US-OUTREG-05", map[string]any{"note": "x"}, http.StatusForbidden)
	kcall(t, r, "QA", http.MethodPut, base+"/annotations/story/US-OUTREG-05", map[string]any{"note": "flaky on staging", "tags": []string{"flaky"}}, http.StatusOK)
	kcall(t, r, "QA", http.MethodPut, base+"/annotations/story/US-NOPE-01", map[string]any{"note": "x"}, http.StatusNotFound)
	kcall(t, r, "QA", http.MethodPut, base+"/annotations/bogus/US-OUTREG-05", map[string]any{"note": "x"}, http.StatusBadRequest)
	tagged := kcall(t, r, "Viewer", http.MethodGet, base+"/stories?tag=flaky", nil, http.StatusOK)
	if tagged["total"] != float64(1) {
		t.Fatalf("tag filter: %v", tagged["total"])
	}

	// Drafts: synced rows are read-only, local drafts are editable.
	kcall(t, r, "QA", http.MethodPut, base+"/stories/US-OUTREG-05", map[string]any{"epic_code": "OUTREG", "text": "x"}, http.StatusConflict)
	kcall(t, r, "QA", http.MethodDelete, base+"/test-cases/TC-OUTREG-001", nil, http.StatusConflict)
	kcall(t, r, "QA", http.MethodPost, base+"/stories", map[string]any{"code": "US-X-01", "epic_code": "OUTREG", "text": "x"}, http.StatusBadRequest)
	kcall(t, r, "QA", http.MethodPost, base+"/stories", map[string]any{"code": "L-OUT-1", "epic_code": "OUTREG", "text": "Draft story", "feature_codes": []string{"A01.07"}}, http.StatusCreated)
	kcall(t, r, "QA", http.MethodPost, base+"/stories", map[string]any{"code": "L-OUT-1", "epic_code": "OUTREG", "text": "dup"}, http.StatusConflict)
	kcall(t, r, "QA", http.MethodPost, base+"/test-cases", map[string]any{"code": "L-TC-1", "epic_code": "OUTREG", "story_code": "L-OUT-1", "scenario": "Given…", "polarity": "+", "level": "API", "priority": "P1", "status": "blocked"}, http.StatusBadRequest)
	kcall(t, r, "QA", http.MethodPost, base+"/test-cases", map[string]any{"code": "L-TC-1", "epic_code": "OUTREG", "story_code": "L-OUT-1", "scenario": "Given…", "polarity": "+", "level": "API", "priority": "P1", "status": "planned"}, http.StatusCreated)
	kcall(t, r, "QA", http.MethodDelete, base+"/stories/L-OUT-1", nil, http.StatusConflict)
	kcall(t, r, "QA", http.MethodPut, base+"/test-cases/L-TC-1", map[string]any{"epic_code": "OUTREG", "scenario": "Changed", "polarity": "-", "level": "WEB", "priority": "P2", "status": "planned"}, http.StatusOK)
	kcall(t, r, "QA", http.MethodDelete, base+"/test-cases/L-TC-1", nil, http.StatusNoContent)
	kcall(t, r, "QA", http.MethodDelete, base+"/stories/L-OUT-1", nil, http.StatusNoContent)

	// Syncs history, and a sync id from another product is 404.
	hist := kcall(t, r, "Viewer", http.MethodGet, base+"/syncs", nil, http.StatusOK)
	if len(hist["syncs"].([]any)) != 2 {
		t.Fatalf("history: %v", hist)
	}
	kcall(t, r, "Viewer", http.MethodGet, fmt.Sprintf("/api/products/%d/knowledge/syncs/1", p.ID+1000), nil, http.StatusNotFound)
	kcall(t, r, "Viewer", http.MethodGet, base+"/nope", nil, http.StatusNotFound)
}
