package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

const gaosTestToken = "gaos_0123456789abcdef0123456789abcdef01"

type gaosFake struct {
	srv   *httptest.Server
	mu    sync.Mutex
	paths []string
	delay time.Duration
}

func newGAOSFake(t *testing.T) *gaosFake {
	f := &gaosFake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Cases []any `json:"cases"`
			Items []any `json:"items"`
		}
		_ = json.Unmarshal(raw, &body)
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		d := f.delay
		f.mu.Unlock()
		time.Sleep(d)
		if strings.HasSuffix(r.URL.Path, "/qa/cases") {
			fmt.Fprintf(w, `{"created":%d,"unchanged":0,"rejected":[],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":0,"catHead":"h"}`, len(body.Cases))
			return
		}
		fmt.Fprintf(w, `{"applied":%d,"unchanged":0,"rejected":[],"catHead":"h"}`, len(body.Items))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *gaosFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

// gaosRouter is a DB router wired to a fake GAOS (or unconfigured when f is nil).
func gaosRouter(t *testing.T, f *gaosFake) *Router {
	r := newDBRouter(t)
	if f != nil {
		r.SetGAOS(gaossync.New(r.db, f.srv.URL+"/api/v1", gaosTestToken))
	} else {
		r.SetGAOS(gaossync.New(r.db, "", ""))
	}
	return r
}

func seedGAOSCase(t *testing.T, r *Router, productID int) {
	t.Helper()
	if _, err := r.db.Exec(context.Background(), `INSERT INTO kb_epics (product_id, code, title, service, content_hash) VALUES (?, 'AUTH', 'Auth', '', 'h')`, productID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(context.Background(), `INSERT INTO kb_test_cases (product_id, code, epic_code, scenario, polarity, level, priority, status, spec_paths, content_hash)
VALUES (?, 'TC-AUTH-001', 'AUTH', 'G W T', '+', 'API', 'P1', 'planned', '[]', 'h')`, productID); err != nil {
		t.Fatal(err)
	}
}

func gaosOf(p map[string]any) map[string]any { return p["gaos"].(map[string]any) }

func getProduct(t *testing.T, r *Router, id int) map[string]any {
	t.Helper()
	code, b := call(t, r, "QA", http.MethodGet, fmt.Sprintf("/api/products/%d", id), nil)
	if code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

func waitGAOSStatus(t *testing.T, r *Router, id int, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		g := gaosOf(getProduct(t, r, id))
		if g["last_status"] == want {
			return g
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("status never became %q: %v", want, gaosOf(getProduct(t, r, id)))
	return nil
}

func TestProductsGAOSJSONShapeAndDefaults(t *testing.T) {
	f := newGAOSFake(t)
	r := gaosRouter(t, f)
	p := createSUT(t, r)
	g := gaosOf(getProduct(t, r, p.ID))
	if g["integrated"] != false || g["product_id"] != "" || g["last_sync_at"] != nil || g["last_status"] != "" || g["last_note"] != "" || len(g) != 5 {
		t.Fatalf("defaults: %v", g)
	}
	code, b := call(t, r, "QA", http.MethodGet, "/api/products", nil)
	var list struct {
		Products   []map[string]any `json:"products"`
		Configured bool             `json:"gaos_configured"`
	}
	_ = json.Unmarshal(b, &list)
	if code != 200 || !list.Configured || len(list.Products) != 1 || gaosOf(list.Products[0])["integrated"] != false {
		t.Fatalf("list: %d %s", code, b)
	}
	un := gaosRouter(t, nil)
	_, b = call(t, un, "QA", http.MethodGet, "/api/products", nil)
	_ = json.Unmarshal(b, &list)
	if list.Configured {
		t.Fatal("an unconfigured server must say gaos_configured:false")
	}
	if got := New0(); got {
		t.Fatal("a Router without a pusher is not configured")
	}
}

// New0 reports gaos_configured of a router that never had SetGAOS called (nil pusher).
func New0() bool { var r Router; return r.gaos.Configured() }

func TestProductsGAOSRoleRule(t *testing.T) {
	f := newGAOSFake(t)
	r := gaosRouter(t, f)
	p := createSUT(t, r)
	url := fmt.Sprintf("/api/products/%d", p.ID)
	for _, role := range []string{"Developer", "QA", "QC"} {
		kcall(t, r, role, http.MethodPut, url, map[string]any{"gaos_integrated": true, "gaos_product_id": "p-6"}, http.StatusForbidden)
		kcall(t, r, role, http.MethodPut, url, map[string]any{"gaos_product_id": "p-6"}, http.StatusForbidden)
	}
	if g := gaosOf(getProduct(t, r, p.ID)); g["integrated"] != false || g["product_id"] != "" {
		t.Fatalf("a refused change must save nothing: %v", g)
	}
	// Sending the current value is not a change.
	kcall(t, r, "Developer", http.MethodPut, url, map[string]any{"gaos_integrated": false, "gaos_product_id": ""}, http.StatusOK)
	kcall(t, r, "DevOps", http.MethodPut, url, map[string]any{"gaos_integrated": true, "gaos_product_id": "p-6"}, http.StatusOK)
	kcall(t, r, "Developer", http.MethodPut, url, map[string]any{"gaos_integrated": true, "gaos_product_id": "p-6"}, http.StatusOK)
	kcall(t, r, "Developer", http.MethodPut, url, map[string]any{"gaos_integrated": false}, http.StatusForbidden)
	kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_integrated": false}, http.StatusOK)
	kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_flag": true}, http.StatusBadRequest)
}

func TestProductsGAOSTurnOnNeedsIDAndConfiguredServer(t *testing.T) {
	f := newGAOSFake(t)
	r := gaosRouter(t, f)
	p := createSUT(t, r)
	url := fmt.Sprintf("/api/products/%d", p.ID)
	for _, bad := range []string{"6", "p-", "P-6", "p-6/x", "p-6 ", "p-" + strings.Repeat("1", 50)} {
		kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_integrated": true, "gaos_product_id": bad}, http.StatusBadRequest)
	}
	kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_product_id": "x"}, http.StatusBadRequest) // shape is checked with the flag off too
	e := kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_integrated": true}, http.StatusBadRequest)
	if !strings.Contains(fmt.Sprint(e["error"]), "gaos_product_id is required") {
		t.Fatalf("%v", e)
	}
	if g := gaosOf(getProduct(t, r, p.ID)); g["integrated"] != false {
		t.Fatalf("%v", g)
	}

	un := gaosRouter(t, nil)
	p2 := createSUT(t, un)
	e = kcall(t, un, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p2.ID), map[string]any{"gaos_integrated": true, "gaos_product_id": "p-6"}, http.StatusBadRequest)
	if !strings.Contains(fmt.Sprint(e["error"]), "not configured") {
		t.Fatalf("%v", e)
	}
	// With the flag off, an unconfigured server still stores the id.
	kcall(t, un, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p2.ID), map[string]any{"gaos_product_id": "p-6"}, http.StatusOK)
	if got := gaosOf(getProduct(t, un, p2.ID))["product_id"]; got != "p-6" {
		t.Fatalf("%v", got)
	}
	if f.count() != 0 {
		t.Fatal("nothing may be sent")
	}
}

func TestProductsGAOSTurnOnPushesAndIDChangeResetsOutcome(t *testing.T) {
	f := newGAOSFake(t)
	r := gaosRouter(t, f)
	p := createSUT(t, r)
	seedGAOSCase(t, r, p.ID)
	url := fmt.Sprintf("/api/products/%d", p.ID)
	kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_integrated": true, "gaos_product_id": "p-6"}, http.StatusOK)
	g := waitGAOSStatus(t, r, p.ID, "ok")
	if g["integrated"] != true || g["product_id"] != "p-6" || g["last_sync_at"] == nil || !strings.HasPrefix(fmt.Sprint(g["last_note"]), "sent 1 cases") {
		t.Fatalf("%v", g)
	}
	f.mu.Lock()
	first := append([]string(nil), f.paths...)
	f.mu.Unlock()
	if len(first) != 2 || first[0] != "/api/v1/products/p-6/qa/cases" {
		t.Fatalf("%v", first)
	}
	// Saving the same values again does not push.
	kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_integrated": true, "gaos_product_id": "p-6", "name": "SUT"}, http.StatusOK)
	time.Sleep(200 * time.Millisecond)
	if f.count() != 2 {
		t.Fatalf("unchanged settings must not push: %d", f.count())
	}
	// A new id resets the outcome and pushes to the new product.
	kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_product_id": "p-7"}, http.StatusOK)
	waitGAOSStatus(t, r, p.ID, "ok")
	f.mu.Lock()
	last := f.paths[len(f.paths)-1]
	f.mu.Unlock()
	if f.count() != 4 || !strings.Contains(last, "/products/p-7/") {
		t.Fatalf("%d %s", f.count(), last)
	}
	// Turning off keeps the id and the outcome.
	kcall(t, r, "Admin", http.MethodPut, url, map[string]any{"gaos_integrated": false}, http.StatusOK)
	if g := gaosOf(getProduct(t, r, p.ID)); g["integrated"] != false || g["product_id"] != "p-7" || g["last_status"] != "ok" {
		t.Fatalf("%v", g)
	}
}

func TestProductsGAOSSyncEndpoint(t *testing.T) {
	f := newGAOSFake(t)
	r := gaosRouter(t, f)
	p := createSUT(t, r)
	seedGAOSCase(t, r, p.ID)
	sync := fmt.Sprintf("/api/products/%d/gaos/sync", p.ID)
	kcall(t, r, "Admin", http.MethodGet, sync, nil, http.StatusMethodNotAllowed)
	for _, role := range []string{"Developer", "QA", "QC"} {
		kcall(t, r, role, http.MethodPost, sync, nil, http.StatusForbidden)
	}
	kcall(t, r, "Admin", http.MethodPost, sync, nil, http.StatusConflict) // not integrated
	if f.count() != 0 {
		t.Fatal("nothing may be sent")
	}
	kcall(t, r, "Admin", http.MethodPut, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"gaos_integrated": true, "gaos_product_id": "p-6"}, http.StatusOK)
	waitGAOSStatus(t, r, p.ID, "ok")

	res := kcall(t, r, "DevOps", http.MethodPost, sync, nil, http.StatusOK)
	if res["status"] != "ok" || res["sent"] != float64(1) || res["created"] != float64(1) {
		t.Fatalf("%v", res)
	}
	for _, k := range []string{"note", "skipped", "exists_differs", "rejected", "rejected_sample", "results_applied", "batches"} {
		if _, ok := res[k]; !ok {
			t.Fatalf("result lacks %q: %v", k, res)
		}
	}

	// A running push makes the next call 409.
	f.mu.Lock()
	f.delay = 400 * time.Millisecond
	f.mu.Unlock()
	before := f.count()
	done := make(chan map[string]any, 1)
	go func() { done <- kcall(t, r, "Admin", http.MethodPost, sync, nil, http.StatusOK) }()
	for deadline := time.Now().Add(5 * time.Second); f.count() == before; {
		if time.Now().After(deadline) {
			t.Fatal("the push never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	kcall(t, r, "Admin", http.MethodPost, sync, nil, http.StatusConflict)
	if res := <-done; res["status"] != "ok" {
		t.Fatalf("%v", res)
	}

	un := gaosRouter(t, nil)
	p2 := createSUT(t, un)
	kcall(t, un, "Admin", http.MethodPost, fmt.Sprintf("/api/products/%d/gaos/sync", p2.ID), nil, http.StatusBadRequest)
}
