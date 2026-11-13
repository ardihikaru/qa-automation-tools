package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func TestParseCostFilterDefaultsAndErrors(t *testing.T) {
	f, err := parseCostFilter(httptest.NewRequest("GET", "/api/cost/overview", nil))
	if err != nil {
		t.Fatal(err)
	}
	if days := int(f.To.Sub(f.From).Hours() / 24); days < 30 || days > 31 { // DST-safe
		t.Fatalf("default range should be 30 days, got %d", days)
	}
	for _, q := range []string{"from=bad", "to=2026-13-01", "from=2026-02-02&to=2026-01-01", "status=maybe", "from=2020-01-01&to=2026-01-01"} {
		if _, err := parseCostFilter(httptest.NewRequest("GET", "/x?"+q, nil)); err == nil {
			t.Errorf("%q should be rejected", q)
		}
	}
	f, err = parseCostFilter(httptest.NewRequest("GET", "/x?from=2026-11-01&to=2026-11-03&feature=a&status=error", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.To.Sub(f.From).Hours() / 24; got != 3 {
		t.Fatalf("to is inclusive: want 3 days, got %v", got)
	}
	where, args := f.where()
	if where != "WHERE created_at >= ? AND created_at < ? AND feature = ? AND status = ?" || len(args) != 4 {
		t.Fatalf("where = %q args=%v", where, args)
	}
	if w, a := f.where("status", "feature"); w != "WHERE created_at >= ? AND created_at < ?" || len(a) != 2 {
		t.Fatalf("skip failed: %q", w)
	}
}

func TestFillDaysAndP95(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	got := fillDays(from, from.AddDate(0, 0, 3), map[string]costTrendPoint{"2026-11-02": {Date: "2026-11-02", Calls: 4}})
	if len(got) != 3 || got[0].Calls != 0 || got[1].Calls != 4 || got[2].Date != "2026-11-03" {
		t.Fatalf("fillDays: %+v", got)
	}
	for n, want := range map[int]int{1: 0, 20: 18, 100: 94} {
		if p95Offset(n) != want {
			t.Errorf("p95Offset(%d)=%d want %d", n, p95Offset(n), want)
		}
	}
}

func TestCostGatewayNotConfigured(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/cost/gateway", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["available"] != false {
		t.Fatalf("want available=false, got %v", body)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/cost/gateway?month=nope", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad month: %d", w.Code)
	}
}

func TestCostEndpointsWithLedger(t *testing.T) {
	r := &Router{db: testdb.New(t)}
	rec := &ai.SQLRecorder{DB: r.db.SQL}
	now := time.Now()
	rec.RecordCall(ai.CallRecord{At: now, Feature: "classify-issue", Provider: "claude-v3", Model: "m", Status: "ok", InputTokens: 10, OutputTokens: 5, CostUSD: 0.5, CostIDR: 8000, LatencyMS: 100})
	rec.RecordCall(ai.CallRecord{At: now, Feature: "job-analysis", Provider: "claude-v3", Model: "m", Status: "error", Error: "boom", LatencyMS: 300})

	get := func(path string, out any) {
		w := httptest.NewRecorder()
		switch {
		case strings.HasPrefix(path, "/api/cost/overview"):
			r.handleCostOverview(w, httptest.NewRequest("GET", path, nil))
		case strings.HasPrefix(path, "/api/cost/trend"):
			r.handleCostTrend(w, httptest.NewRequest("GET", path, nil))
		default:
			r.handleCostRecent(w, httptest.NewRequest("GET", path, nil))
		}
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		_ = json.Unmarshal(w.Body.Bytes(), out)
	}
	var o costOverview
	get("/api/cost/overview", &o)
	if o.TotalRequests != 2 || o.ErrorCalls != 1 || o.TotalTokens != 15 || o.P95LatencyMS != 300 || o.TotalCost != 0.5 {
		t.Fatalf("overview: %+v", o)
	}
	var tr []costTrendPoint
	get("/api/cost/trend", &tr)
	if len(tr) != 30 || tr[len(tr)-1].Calls != 2 {
		t.Fatalf("trend: len=%d last=%+v", len(tr), tr[len(tr)-1])
	}
	var rc struct {
		Items []costCall `json:"items"`
		Total int        `json:"total"`
	}
	get("/api/cost/recent?status=error", &rc)
	if rc.Total != 1 || rc.Items[0].Error != "boom" {
		t.Fatalf("recent: %+v", rc)
	}
}
