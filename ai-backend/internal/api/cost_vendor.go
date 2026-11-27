package api

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// COST_VENDOR: groups the ai_call_log ledger by AI vendor (derived from the model id, because the
// gateway provider is always "claude-v3") with a friendly model label. The mapping is code, not
// data, so a new model family needs one case here.

const (
	vendorAnthropic = "Anthropic"
	vendorOther     = "Other"
)

var claudeModelRe = regexp.MustCompile(`^claude-(opus|sonnet|haiku|fable|mythos)-(\d+(?:-\d+)?)`)

// vendorForModel maps a model id (and the gateway provider as a hint) to a vendor name.
func vendorForModel(provider, model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "claude"):
		return vendorAnthropic
	case m == "" && strings.HasPrefix(strings.ToLower(provider), "claude"):
		return vendorAnthropic
	}
	return vendorOther
}

// modelLabel gives a human label: claude-haiku-5-5 -> "Haiku 5.5", claude-opus-5 -> "Opus 5".
// Unknown ids are returned unchanged; an empty id becomes "unknown".
func modelLabel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return "unknown"
	}
	if g := claudeModelRe.FindStringSubmatch(m); g != nil {
		family := strings.ToUpper(g[1][:1]) + g[1][1:]
		return family + " " + strings.ReplaceAll(g[2], "-", ".")
	}
	return model
}

type vendorFeature struct {
	Feature      string  `json:"feature"`
	Calls        int     `json:"calls"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	Cost         float64 `json:"cost"`
	CostIDR      float64 `json:"cost_idr"`
}

type vendorModel struct {
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	Label        string          `json:"label"`
	Calls        int             `json:"calls"`
	Errors       int             `json:"errors"`
	InputTokens  int64           `json:"input_tokens"`
	OutputTokens int64           `json:"output_tokens"`
	Cost         float64         `json:"cost"`
	CostIDR      float64         `json:"cost_idr"`
	AvgCost      float64         `json:"avg_cost_per_call"`
	AvgCostIDR   float64         `json:"avg_cost_idr_per_call"`
	SharePct     float64         `json:"share_pct"` // of total cost in the range (calls when the total cost is 0)
	Features     []vendorFeature `json:"features"`
}

type vendorGroup struct {
	Vendor       string        `json:"vendor"`
	Calls        int           `json:"calls"`
	Errors       int           `json:"errors"`
	InputTokens  int64         `json:"input_tokens"`
	OutputTokens int64         `json:"output_tokens"`
	Cost         float64       `json:"cost"`
	CostIDR      float64       `json:"cost_idr"`
	AvgCost      float64       `json:"avg_cost_per_call"`
	AvgCostIDR   float64       `json:"avg_cost_idr_per_call"`
	SharePct     float64       `json:"share_pct"`
	Models       []vendorModel `json:"models"`
}

type vendorReport struct {
	TotalCost    float64       `json:"total_cost"`
	TotalCostIDR float64       `json:"total_cost_idr"`
	TotalCalls   int           `json:"total_calls"`
	Vendors      []vendorGroup `json:"vendors"`
}

func pct(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	return part / whole * 100
}

// buildVendorReport nests (provider, model, feature) rows under vendors. Rows may come in any order.
func buildVendorReport(rows []vendorRow) vendorReport {
	rep := vendorReport{Vendors: []vendorGroup{}}
	vIdx := map[string]int{}
	mIdx := map[string]int{}
	for _, r := range rows {
		rep.TotalCost += r.Cost
		rep.TotalCostIDR += r.CostIDR
		rep.TotalCalls += r.Calls
		v := vendorForModel(r.Provider, r.Model)
		vi, ok := vIdx[v]
		if !ok {
			vi = len(rep.Vendors)
			vIdx[v] = vi
			rep.Vendors = append(rep.Vendors, vendorGroup{Vendor: v, Models: []vendorModel{}})
		}
		vg := &rep.Vendors[vi]
		key := v + "\x00" + r.Provider + "\x00" + r.Model
		mi, ok := mIdx[key]
		if !ok {
			mi = len(vg.Models)
			mIdx[key] = mi
			vg.Models = append(vg.Models, vendorModel{Provider: r.Provider, Model: r.Model, Label: modelLabel(r.Model), Features: []vendorFeature{}})
		}
		mg := &vg.Models[mi]
		mg.Features = append(mg.Features, vendorFeature{Feature: r.Feature, Calls: r.Calls,
			InputTokens: r.In, OutputTokens: r.Out, Cost: r.Cost, CostIDR: r.CostIDR})
		for _, t := range []*struct {
			c, e   *int
			i, o   *int64
			cu, ci *float64
		}{{&mg.Calls, &mg.Errors, &mg.InputTokens, &mg.OutputTokens, &mg.Cost, &mg.CostIDR},
			{&vg.Calls, &vg.Errors, &vg.InputTokens, &vg.OutputTokens, &vg.Cost, &vg.CostIDR}} {
			*t.c += r.Calls
			*t.e += r.Errors
			*t.i += r.In
			*t.o += r.Out
			*t.cu += r.Cost
			*t.ci += r.CostIDR
		}
	}
	share := func(cost, calls float64) float64 {
		if rep.TotalCost > 0 {
			return pct(cost, rep.TotalCost)
		}
		return pct(calls, float64(rep.TotalCalls))
	}
	for i := range rep.Vendors {
		vg := &rep.Vendors[i]
		vg.SharePct = share(vg.Cost, float64(vg.Calls))
		vg.AvgCost, vg.AvgCostIDR = avgPerCall(vg.Cost, vg.CostIDR, vg.Calls)
		for j := range vg.Models {
			mg := &vg.Models[j]
			mg.SharePct = share(mg.Cost, float64(mg.Calls))
			mg.AvgCost, mg.AvgCostIDR = avgPerCall(mg.Cost, mg.CostIDR, mg.Calls)
			sort.SliceStable(mg.Features, func(a, b int) bool { return mg.Features[a].Cost > mg.Features[b].Cost })
		}
		sort.SliceStable(vg.Models, func(a, b int) bool {
			return costFirst(vg.Models[a].Cost, vg.Models[a].Calls, vg.Models[b].Cost, vg.Models[b].Calls)
		})
	}
	sort.SliceStable(rep.Vendors, func(a, b int) bool {
		return costFirst(rep.Vendors[a].Cost, rep.Vendors[a].Calls, rep.Vendors[b].Cost, rep.Vendors[b].Calls)
	})
	return rep
}

func avgPerCall(usd, idr float64, calls int) (float64, float64) {
	if calls == 0 {
		return 0, 0
	}
	return usd / float64(calls), idr / float64(calls)
}

func costFirst(ca float64, na int, cb float64, nb int) bool {
	if ca != cb {
		return ca > cb
	}
	return na > nb
}

type vendorRow struct {
	Provider, Model, Feature string
	Calls, Errors            int
	In, Out                  int64
	Cost, CostIDR            float64
}

// handleCostByVendor: GET /api/cost/by-vendor, same from/to/feature/provider/model/status filters as the other cost endpoints.
func (r *Router) handleCostByVendor(w http.ResponseWriter, req *http.Request) {
	f, ok := r.costGate(w, req)
	if !ok {
		return
	}
	where, args := f.where()
	rows, err := r.db.Query(req.Context(), `SELECT provider, model, feature, COUNT(*), COALESCE(SUM(status='error'),0),
  COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cost_usd),0), COALESCE(SUM(cost_idr),0)
FROM ai_call_log `+where+` GROUP BY provider, model, feature LIMIT 2000`, args...)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var all []vendorRow
	for rows.Next() {
		var v vendorRow
		if err := rows.Scan(&v.Provider, &v.Model, &v.Feature, &v.Calls, &v.Errors, &v.In, &v.Out, &v.Cost, &v.CostIDR); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		all = append(all, v)
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, buildVendorReport(all))
}
