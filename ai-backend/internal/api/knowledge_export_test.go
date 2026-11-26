package api

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/services/objectstore"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// memStore is an in-memory objectstore.Store.
type memStore struct {
	mu   sync.Mutex
	objs map[string][]byte
	fail error
}

func (m *memStore) Stat(_ context.Context, k string) (objectstore.Info, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return objectstore.Info{}, false, m.fail
	}
	b, ok := m.objs[k]
	return objectstore.Info{Size: int64(len(b)), LastModified: time.Now()}, ok, nil
}
func (m *memStore) Get(_ context.Context, k string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return nil, false, m.fail
	}
	b, ok := m.objs[k]
	return b, ok, nil
}
func (m *memStore) Put(_ context.Context, k, _ string, d []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.objs[k] = d
	return nil
}

type fakeExportSrc struct {
	err  error
	name string
}

func (f fakeExportSrc) ChecklistKeyInfo(context.Context, int, bool) (knowledge.KeyInfo, error) {
	return knowledge.KeyInfo{SyncID: 3, ProductName: f.name}, f.err
}
func (f fakeExportSrc) ChecklistSnapshot(context.Context, int, bool) (*knowledge.ChecklistSnapshot, error) {
	return &knowledge.ChecklistSnapshot{ProductName: f.name, SyncID: 3, SyncedAt: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
		Parts: []knowledge.ChecklistPart{{Code: "A", Name: "P", Modules: []knowledge.ChecklistModule{{Code: "A01", Name: "M", Rows: []knowledge.ChecklistRow{{ID: "A01.01", Feature: "f", Status: "PROD"}}}}}}}, nil
}

func exportRouter(src fakeExportSrc, store *memStore) (*Router, *knowledge.ExportCache) {
	r := &Router{kb: &knowledge.Syncer{}}
	var c *knowledge.ExportCache
	if store != nil {
		c = knowledge.NewExportCache(src, store, "dev-test", time.Second)
		r.SetExportCache(c)
	}
	return r, c
}

func callExport(r *Router, method, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/products/1/knowledge/exports/modules-checklist"+query, nil)
	w := httptest.NewRecorder()
	r.handleKnowledge(w, req, 1, []string{"exports", "modules-checklist"}, false) // a read-only role
	return w
}

func TestStrictBool(t *testing.T) {
	cases := []struct {
		q       string
		val, ok bool
	}{
		{"", false, true}, {"?x=1", false, true}, {"?v=0", false, true}, {"?v=1", true, true}, {"?v=true", true, true}, {"?v=false", false, true},
		{"?v=", false, false}, {"?v=yes", false, false}, {"?v=2", false, false}, {"?v=TRUE", false, false}, {"?v=1&v=0", false, false},
	}
	for _, c := range cases {
		val, ok := strictBool(httptest.NewRequest("GET", "/x"+c.q, nil), "v")
		if val != c.val || ok != c.ok {
			t.Errorf("%q: got (%v,%v)", c.q, val, ok)
		}
	}
}

func TestChecklistExportValidation(t *testing.T) {
	r, _ := exportRouter(fakeExportSrc{name: "SUT"}, &memStore{objs: map[string][]byte{}})
	cases := []struct {
		method, query string
		code          int
		msg           string
	}{
		{"POST", "?format=pdf", 405, "method not allowed"},
		{"DELETE", "", 405, "method not allowed"},
		{"GET", "", 400, "format must be pdf, xlsx or csv"},
		{"GET", "?format=docx", 400, "format must be pdf, xlsx or csv"},
		{"GET", "?format=PDF", 400, "format must be pdf, xlsx or csv"},
		{"GET", "?format=pdf&last_update=yes", 400, "last_update must be 0, 1, true or false"},
		{"GET", "?format=pdf&force=2", 400, "force must be 0, 1, true or false"},
		{"GET", "?format=pdf&force=", 400, "force must be 0, 1, true or false"},
		{"GET", "?format=pdf&scope=PROD", 400, "scope must be all or prod"},
		{"GET", "?format=pdf&scope=prod&scope=all", 400, "scope must be given once"},
		{"GET", "?format=pdf&scope=prod&since_from=2026-5", 400, "since_from and since_to must be YYYY-MM"},
		{"GET", "?format=pdf&scope=prod&since_to=2026-13", 400, "since_from and since_to must be YYYY-MM"},
		{"GET", "?format=pdf&since_from=2026-05", 400, "since_from and since_to need scope=prod"},
		{"GET", "?format=pdf&scope=all&since_to=2026-05", 400, "since_from and since_to need scope=prod"},
		{"GET", "?format=pdf&scope=prod&since_from=2026-11&since_to=2026-05", 400, "since_from must not be after since_to"},
		{"GET", "?format=pdf&check_column=no", 400, "check_column must be 0, 1, true or false"},
		{"GET", "?format=pdf&prod_since=unknown", 400, "prod_since=unknown needs scope=prod"},
		{"GET", "?format=pdf&scope=prod&prod_since=unknown&since_from=2026-05", 400, "prod_since=unknown cannot be combined"},
		{"GET", "?format=pdf&scope=prod&prod_since=known", 400, "prod_since must be unknown"},
		{"GET", "?format=pdf&scope=prod&prod_since=unknown&prod_since=unknown", 400, "prod_since must be given once"},
	}
	for _, c := range cases {
		w := callExport(r, c.method, c.query)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.msg) {
			t.Errorf("%s %s: %d %s", c.method, c.query, w.Code, w.Body.String())
		}
	}
	// 405 wins over 400, 400 wins over 503.
	rNil, _ := exportRouter(fakeExportSrc{}, nil)
	if w := callExport(rNil, "POST", ""); w.Code != 405 {
		t.Errorf("405 before 503: %d", w.Code)
	}
	if w := callExport(rNil, "GET", "?format=x"); w.Code != 400 {
		t.Errorf("400 before 503: %d", w.Code)
	}
}

func TestChecklistExportNotConfigured(t *testing.T) {
	r, _ := exportRouter(fakeExportSrc{}, nil)
	w := callExport(r, "GET", "?format=csv")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "object storage not configured") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestChecklistExportErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
		msg  string
	}{
		{knowledge.ErrNotFound, 404, `"not found"`},
		{knowledge.ErrNoSucceededSync, 409, "no knowledge base sync has succeeded yet"},
		{knowledge.ErrExportTooLarge, 413, "catalogue too large to export"},
	}
	for _, c := range cases {
		r, _ := exportRouter(fakeExportSrc{err: c.err}, &memStore{objs: map[string][]byte{}})
		w := callExport(r, "GET", "?format=pdf")
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.msg) {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body.String())
		}
	}
}

func TestChecklistExportHeaders(t *testing.T) {
	store := &memStore{objs: map[string][]byte{}}
	r, c := exportRouter(fakeExportSrc{name: "SUT / <b>"}, store)
	types := map[string]string{"pdf": "application/pdf", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "csv": "text/csv; charset=utf-8"}
	for _, f := range []string{"pdf", "xlsx", "csv"} {
		w := callExport(r, "GET", "?format="+f)
		h := w.Header()
		if w.Code != 200 || h.Get("Content-Type") != types[f] || h.Get("Cache-Control") != "no-store" ||
			h.Get("Content-Disposition") != `attachment; filename="SUT-b-MODULE-CHECKLIST.`+f+`"` ||
			h.Get("Content-Length") != strconv.Itoa(w.Body.Len()) || w.Body.Len() == 0 {
			t.Fatalf("%s: %d %v", f, w.Code, h)
		}
		if want := map[string]string{"pdf": "miss", "xlsx": "hit", "csv": "hit"}[f]; h.Get("X-Export-Cache") != want && f == "pdf" {
			t.Errorf("%s cache = %s", f, h.Get("X-Export-Cache"))
		}
		c.Wait()
	}
	// Everything is stored now: the same request is a hit, a forced one inside 30 s too.
	if got := callExport(r, "GET", "?format=pdf").Header().Get("X-Export-Cache"); got != "hit" {
		t.Errorf("second pdf: %s", got)
	}
	if got := callExport(r, "GET", "?format=pdf&force=1").Header().Get("X-Export-Cache"); got != "hit" {
		t.Errorf("force inside the cooldown: %s", got)
	}
	// A failing store never fails the request.
	store.fail = &objectstore.OpError{Op: "get", Code: "AccessDenied"}
	w := callExport(r, "GET", "?format=csv&last_update=1")
	if w.Code != 200 || w.Header().Get("X-Export-Cache") != "bypass" || !strings.HasPrefix(w.Body.String(), "\xEF\xBB\xBF") {
		t.Errorf("bypass: %d %v", w.Code, w.Header())
	}
	c.Wait()
}

// TestChecklistExportDB runs the endpoint over a real syncer on a scratch schema (QA_TEST_MYSQL_DSN):
// 404 for a missing product, 409 before any succeeded sync, then 200 for a read-only role.
func TestChecklistExportDB(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	res, err := d.Exec(ctx, `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES ('SUT', 'https://github.com/o/r', 'https://s', 'sut', 'main')`)
	if err != nil {
		t.Fatal(err)
	}
	pid := int(res.LastInsertId())
	syncer := knowledge.NewSyncer(d, nil, nil)
	r := &Router{kb: syncer}
	store := &memStore{objs: map[string][]byte{}}
	r.SetExportCache(knowledge.NewExportCache(syncer, store, "dev-test", time.Second))
	get := func(id int) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/x?format=csv&last_update=1", nil)
		w := httptest.NewRecorder()
		r.handleKnowledge(w, req, id, []string{"exports", "modules-checklist"}, false)
		return w
	}
	if w := get(pid + 99); w.Code != 404 {
		t.Fatalf("missing product: %d %s", w.Code, w.Body.String())
	}
	if w := get(pid); w.Code != 409 {
		t.Fatalf("no sync: %d %s", w.Code, w.Body.String())
	}
	sres, err := d.Exec(ctx, `INSERT INTO kb_syncs (product_id, status, trigger_kind, started_at, finished_at, commit_sha) VALUES (?, 'succeeded', 'manual', UTC_TIMESTAMP(), UTC_TIMESTAMP(), REPEAT('a', 40))`, pid)
	if err != nil {
		t.Fatal(err)
	}
	sid := sres.LastInsertId()
	for _, q := range []string{
		`INSERT INTO kb_parts (product_id, code, name, content_hash, first_sync_id, changed_sync_id) VALUES (?, 'A', 'Part A', 'h', ?, ?)`,
		`INSERT INTO kb_modules (product_id, code, part_code, name, content_hash, first_sync_id, changed_sync_id) VALUES (?, 'A01', 'A', 'Mod', 'h', ?, ?)`,
		`INSERT INTO kb_features (product_id, code, module_code, name, release_status, content_hash, first_sync_id, changed_sync_id) VALUES (?, 'A01.01', 'A01', 'Feature one', 'PROD', 'h', ?, ?)`,
	} {
		if _, err := d.Exec(ctx, q, pid, sid, sid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Exec(ctx, `INSERT INTO kb_feature_status_history (product_id, feature_code, release_status, sync_id, changed_at, source) VALUES (?, 'A01.01', 'PROD', ?, '2026-09-01 03:00:00', 'sync')`, pid, sid); err != nil {
		t.Fatal(err)
	}
	w := get(pid)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "A01.01") || !strings.Contains(w.Body.String(), "2026-09-01,sync") {
		t.Fatalf("export: %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Export-Cache") != "miss" {
		t.Errorf("cache = %s", w.Header().Get("X-Export-Cache"))
	}
	r.exports.Wait()
	if w := get(pid); w.Header().Get("X-Export-Cache") != "hit" {
		t.Errorf("second request: %s", w.Header().Get("X-Export-Cache"))
	}
}

func TestChecklistExportScopedFilename(t *testing.T) {
	r, _ := exportRouter(fakeExportSrc{name: "SUT"}, &memStore{objs: map[string][]byte{}})
	cases := map[string]string{
		"?format=csv&scope=all":  "SUT-MODULE-CHECKLIST.csv",
		"?format=csv&scope=prod": "SUT-MODULE-CHECKLIST-PROD.csv",
		"?format=csv&scope=prod&since_from=2026-05&since_to=2026-11": "SUT-MODULE-CHECKLIST-PROD-202605-202611.csv",
		"?format=csv&scope=prod&since_from=2026-05":                  "SUT-MODULE-CHECKLIST-PROD-202605-X.csv",
		"?format=csv&scope=prod&check_column=0":                      "SUT-MODULE-CHECKLIST-PROD-NOCHECK.csv",
		"?format=csv&check_column=1":                                 "SUT-MODULE-CHECKLIST.csv",
		"?format=csv&scope=prod&prod_since=unknown":                  "SUT-MODULE-CHECKLIST-PROD-UNDATED.csv",
	}
	for q, want := range cases {
		w := callExport(r, "GET", q)
		if w.Code != 200 || w.Header().Get("Content-Disposition") != `attachment; filename="`+want+`"` {
			t.Errorf("%s: %d %q", q, w.Code, w.Header().Get("Content-Disposition"))
		}
	}
}
