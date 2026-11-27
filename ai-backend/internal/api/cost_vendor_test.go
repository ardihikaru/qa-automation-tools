package api

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func TestModelLabelAndVendor(t *testing.T) {
	for in, want := range map[string]string{
		"claude-haiku-5-5": "Haiku 5.5", "claude-sonnet-5-5": "Sonnet 5.5", "claude-opus-5": "Opus 5",
		"claude-fable-5-1": "Fable 5.1", "claude-opus-5-5-20260101": "Opus 5.5", "gpt-5": "gpt-5", "": "unknown",
	} {
		if got := modelLabel(in); got != want {
			t.Errorf("modelLabel(%q)=%q want %q", in, got, want)
		}
	}
	for _, c := range []struct{ prov, model, want string }{
		{"claude-v3", "claude-haiku-5-5", vendorAnthropic}, {"claude-v3", "", vendorAnthropic},
		{"antigravity", "gemini", vendorOther}, {"", "", vendorOther},
	} {
		if got := vendorForModel(c.prov, c.model); got != c.want {
			t.Errorf("vendorForModel(%q,%q)=%q want %q", c.prov, c.model, got, c.want)
		}
	}
}

func TestBuildVendorReport(t *testing.T) {
	rep := buildVendorReport([]vendorRow{
		{"claude-v3", "claude-haiku-5-5", "translation", 8, 1, 800, 400, 0.2, 3200},
		{"claude-v3", "claude-opus-5", "classify-issue", 2, 0, 100, 50, 0.6, 9600},
		{"claude-v3", "claude-opus-5", "job-analysis", 1, 0, 100, 50, 0.2, 3200},
		{"x", "mystery", "f", 1, 0, 0, 0, 0, 0},
	})
	if len(rep.Vendors) != 2 || rep.TotalCalls != 12 || math.Abs(rep.TotalCost-1.0) > 1e-9 {
		t.Fatalf("report: %+v", rep)
	}
	a := rep.Vendors[0]
	if a.Vendor != vendorAnthropic || a.Calls != 11 || len(a.Models) != 2 || math.Abs(a.SharePct-100) > 1e-9 {
		t.Fatalf("anthropic: %+v", a)
	}
	if a.Models[0].Label != "Opus 5" || a.Models[0].Calls != 3 || math.Abs(a.Models[0].SharePct-80) > 1e-9 {
		t.Fatalf("opus: %+v", a.Models[0])
	}
	if f := a.Models[0].Features; len(f) != 2 || f[0].Feature != "classify-issue" {
		t.Fatalf("features: %+v", f)
	}
	if h := a.Models[1]; h.Label != "Haiku 5.5" || math.Abs(h.AvgCost-0.025) > 1e-9 || h.Features[0].Feature != "translation" {
		t.Fatalf("haiku: %+v", h)
	}
	if o := rep.Vendors[1]; o.Vendor != vendorOther || o.Models[0].Label != "mystery" {
		t.Fatalf("other: %+v", o)
	}
	if empty := buildVendorReport(nil); empty.Vendors == nil || len(empty.Vendors) != 0 {
		t.Fatalf("empty must be [] not null: %+v", empty)
	}
}

func TestCostByVendorWithLedger(t *testing.T) {
	r := &Router{db: testdb.New(t)}
	rec := &ai.SQLRecorder{DB: r.db.SQL}
	now := time.Now()
	rec.RecordCall(ai.CallRecord{At: now, Feature: "translation", Provider: "claude-v3", Model: "claude-haiku-5-5", Status: "ok", InputTokens: 10, OutputTokens: 5, CostUSD: 0.1, CostIDR: 1600})
	rec.RecordCall(ai.CallRecord{At: now, Feature: "classify-issue", Provider: "claude-v3", Model: "claude-opus-5", Status: "ok", InputTokens: 20, OutputTokens: 5, CostUSD: 0.3, CostIDR: 4800})

	get := func(path string, out any) {
		w := httptest.NewRecorder()
		if strings.HasPrefix(path, "/api/cost/by-vendor") {
			r.handleCostByVendor(w, httptest.NewRequest("GET", path, nil))
		} else {
			r.handleCostByModel(w, httptest.NewRequest("GET", path, nil))
		}
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		_ = json.Unmarshal(w.Body.Bytes(), out)
	}
	var rep vendorReport
	get("/api/cost/by-vendor", &rep)
	if len(rep.Vendors) != 1 || len(rep.Vendors[0].Models) != 2 || rep.Vendors[0].Models[0].Label != "Opus 5" || rep.TotalCalls != 2 {
		t.Fatalf("by-vendor: %+v", rep)
	}
	get("/api/cost/by-vendor?feature=translation", &rep)
	if len(rep.Vendors[0].Models) != 1 || rep.Vendors[0].Models[0].Label != "Haiku 5.5" {
		t.Fatalf("feature filter: %+v", rep)
	}
	var bm []costGroup
	get("/api/cost/by-model", &bm)
	if len(bm) != 2 || bm[0].Vendor != vendorAnthropic || bm[0].ModelLabel != "Opus 5" {
		t.Fatalf("by-model: %+v", bm)
	}
}
