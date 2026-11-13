package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// COST_TRACKING (all day and month boundaries are UTC, like the DB session): /api/cost/* read the ai_call_log ledger written by ai.Service (one row per AI
// call attempt, success or failure) and, for /gateway, ai-manager's own usage log.

const costDay = "2006-01-02"

// costFilter is the parsed from/to/feature/provider/model/status query.
type costFilter struct {
	From, To time.Time // [From, To) in DB time; To is the day after the requested "to"
	Feature  string
	Provider string
	Model    string
	Status   string
}

func parseCostFilter(req *http.Request) (costFilter, error) {
	q := req.URL.Query()
	y, m, d := time.Now().UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	f := costFilter{
		From:     today.AddDate(0, 0, -29),
		To:       today.AddDate(0, 0, 1),
		Feature:  q.Get("feature"),
		Provider: q.Get("provider"),
		Model:    q.Get("model"),
		Status:   q.Get("status"),
	}
	if v := q.Get("from"); v != "" {
		t, err := time.ParseInLocation(costDay, v, time.UTC)
		if err != nil {
			return f, fmt.Errorf("from must be YYYY-MM-DD")
		}
		f.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.ParseInLocation(costDay, v, time.UTC)
		if err != nil {
			return f, fmt.Errorf("to must be YYYY-MM-DD")
		}
		f.To = t.AddDate(0, 0, 1)
	}
	if !f.To.After(f.From) {
		return f, fmt.Errorf("to must not be before from")
	}
	if f.To.Sub(f.From) > 366*24*time.Hour {
		return f, fmt.Errorf("range must be at most 366 days")
	}
	if f.Status != "" && f.Status != "ok" && f.Status != "error" {
		return f, fmt.Errorf("status must be ok or error")
	}
	return f, nil
}

// where builds the WHERE clause. skip names filters to leave out (e.g. "status").
func (f costFilter) where(skip ...string) (string, []any) {
	skipped := map[string]bool{}
	for _, s := range skip {
		skipped[s] = true
	}
	clauses := []string{"created_at >= ?", "created_at < ?"}
	args := []any{f.From, f.To}
	add := func(name, col, val string) {
		if val != "" && !skipped[name] {
			clauses = append(clauses, col+" = ?")
			args = append(args, val)
		}
	}
	add("feature", "feature", f.Feature)
	add("provider", "provider", f.Provider)
	add("model", "model", f.Model)
	add("status", "status", f.Status)
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// costGate handles method check, DB presence and filter parsing shared by every cost endpoint.
func (r *Router) costGate(w http.ResponseWriter, req *http.Request) (costFilter, bool) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return costFilter{}, false
	}
	if r.getDB() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Database not available")
		return costFilter{}, false
	}
	f, err := parseCostFilter(req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return f, false
	}
	return f, true
}

type costOverview struct {
	TotalRequests int     `json:"total_requests"`
	SuccessCalls  int     `json:"success_calls"`
	ErrorCalls    int     `json:"error_calls"`
	ErrorRate     float64 `json:"error_rate"`
	FallbackCalls int     `json:"fallback_calls"`
	TotalTokens   int64   `json:"total_tokens"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TotalCost     float64 `json:"total_cost"`
	TotalCostIDR  float64 `json:"total_cost_idr"`
	EstimatedCost float64 `json:"estimated_cost"`
	AvgLatencyMS  float64 `json:"avg_latency_ms"`
	P95LatencyMS  int     `json:"p95_latency_ms"`
}

// p95Offset returns the zero-based row offset of the 95th percentile among n ordered rows.
func p95Offset(n int) int {
	if n <= 0 {
		return 0
	}
	return int(math.Ceil(0.95*float64(n))) - 1
}

func (r *Router) handleCostOverview(w http.ResponseWriter, req *http.Request) {
	f, ok := r.costGate(w, req)
	if !ok {
		return
	}
	where, args := f.where()
	var o costOverview
	err := r.db.QueryRow(req.Context(), `SELECT COUNT(*),
  COALESCE(SUM(status='ok'),0), COALESCE(SUM(status='error'),0), COALESCE(SUM(fallback),0),
  COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
  COALESCE(SUM(cost_usd),0), COALESCE(SUM(cost_idr),0),
  COALESCE(SUM(CASE WHEN cost_source<>'' THEN cost_usd ELSE 0 END),0),
  COALESCE(AVG(latency_ms),0)
FROM ai_call_log `+where, args...).Scan(&o.TotalRequests, &o.SuccessCalls, &o.ErrorCalls, &o.FallbackCalls,
		&o.InputTokens, &o.OutputTokens, &o.TotalCost, &o.TotalCostIDR, &o.EstimatedCost, &o.AvgLatencyMS)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	o.TotalTokens = o.InputTokens + o.OutputTokens
	if o.TotalRequests > 0 {
		o.ErrorRate = float64(o.ErrorCalls) / float64(o.TotalRequests)
		err = r.db.QueryRow(req.Context(), "SELECT latency_ms FROM ai_call_log "+where+" ORDER BY latency_ms LIMIT 1 OFFSET ?",
			append(append([]any{}, args...), p95Offset(o.TotalRequests))...).Scan(&o.P95LatencyMS)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, o)
}

type costTrendPoint struct {
	Date    string  `json:"date"`
	Cost    float64 `json:"cost"`
	CostIDR float64 `json:"cost_idr"`
	Calls   int     `json:"calls"`
	Errors  int     `json:"errors"`
}

// fillDays returns one point per day in [from, to), using pts for days that have data.
func fillDays(from, to time.Time, pts map[string]costTrendPoint) []costTrendPoint {
	out := []costTrendPoint{}
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		key := d.Format(costDay)
		p, ok := pts[key]
		if !ok {
			p = costTrendPoint{Date: key}
		}
		out = append(out, p)
	}
	return out
}

func (r *Router) handleCostTrend(w http.ResponseWriter, req *http.Request) {
	f, ok := r.costGate(w, req)
	if !ok {
		return
	}
	where, args := f.where()
	rows, err := r.db.Query(req.Context(), `SELECT DATE_FORMAT(created_at,'%Y-%m-%d'),
  COALESCE(SUM(cost_usd),0), COALESCE(SUM(cost_idr),0), COUNT(*), COALESCE(SUM(status='error'),0)
FROM ai_call_log `+where+` GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	pts := map[string]costTrendPoint{}
	for rows.Next() {
		var p costTrendPoint
		if err := rows.Scan(&p.Date, &p.Cost, &p.CostIDR, &p.Calls, &p.Errors); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		pts[p.Date] = p
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fillDays(f.From, f.To, pts))
}

func (r *Router) handleCostEfficiency(w http.ResponseWriter, req *http.Request) {
	f, ok := r.costGate(w, req)
	if !ok {
		return
	}
	where, args := f.where()
	var out struct {
		Calls     int `json:"calls"`
		Success   int `json:"success"`
		Errors    int `json:"errors"`
		Fallbacks int `json:"fallbacks"`
	}
	err := r.db.QueryRow(req.Context(), `SELECT COUNT(*), COALESCE(SUM(status='ok'),0),
  COALESCE(SUM(status='error'),0), COALESCE(SUM(fallback),0) FROM ai_call_log `+where, args...).
		Scan(&out.Calls, &out.Success, &out.Errors, &out.Fallbacks)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type costGroup struct {
	Feature      string  `json:"feature,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	Model        string  `json:"model,omitempty"`
	Calls        int     `json:"calls"`
	Errors       int     `json:"errors"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	Cost         float64 `json:"cost"`
	CostIDR      float64 `json:"cost_idr"`
}

func (r *Router) costGrouped(w http.ResponseWriter, req *http.Request, by string) {
	f, ok := r.costGate(w, req)
	if !ok {
		return
	}
	where, args := f.where()
	cols, scan := "feature", func(g *costGroup) []any { return []any{&g.Feature} }
	if by == "model" {
		cols, scan = "provider, model", func(g *costGroup) []any { return []any{&g.Provider, &g.Model} }
	}
	rows, err := r.db.Query(req.Context(), `SELECT `+cols+`, COUNT(*), COALESCE(SUM(status='error'),0),
  COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cost_usd),0), COALESCE(SUM(cost_idr),0)
FROM ai_call_log `+where+` GROUP BY `+cols+` ORDER BY SUM(cost_usd) DESC, COUNT(*) DESC LIMIT 50`, args...)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []costGroup{}
	for rows.Next() {
		var g costGroup
		dst := append(scan(&g), &g.Calls, &g.Errors, &g.InputTokens, &g.OutputTokens, &g.Cost, &g.CostIDR)
		if err := rows.Scan(dst...); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (r *Router) handleCostByFeature(w http.ResponseWriter, req *http.Request) {
	r.costGrouped(w, req, "feature")
}

func (r *Router) handleCostByModel(w http.ResponseWriter, req *http.Request) {
	r.costGrouped(w, req, "model")
}

type costCall struct {
	ID           int64     `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Feature      string    `json:"feature"`
	Caller       string    `json:"caller"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	Error        string    `json:"error"`
	Fallback     bool      `json:"fallback"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Cost         float64   `json:"cost"`
	CostIDR      float64   `json:"cost_idr"`
	LatencyMS    int       `json:"latency_ms"`
}

func (r *Router) handleCostRecent(w http.ResponseWriter, req *http.Request) {
	f, ok := r.costGate(w, req)
	if !ok {
		return
	}
	limit, offset := 50, 0
	if v, err := strconv.Atoi(req.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > 200 {
		limit = 200
	}
	if v, err := strconv.Atoi(req.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}
	where, args := f.where()
	var total int
	if err := r.db.QueryRow(req.Context(), "SELECT COUNT(*) FROM ai_call_log "+where, args...).Scan(&total); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := r.db.Query(req.Context(), `SELECT id, created_at, feature, caller, provider, model, status,
  COALESCE(error,''), fallback, input_tokens, output_tokens, cost_usd, cost_idr, latency_ms
FROM ai_call_log `+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []costCall{}
	for rows.Next() {
		var c costCall
		if err := rows.Scan(&c.ID, &c.CreatedAt, &c.Feature, &c.Caller, &c.Provider, &c.Model, &c.Status,
			&c.Error, &c.Fallback, &c.InputTokens, &c.OutputTokens, &c.Cost, &c.CostIDR, &c.LatencyMS); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

// handleCostFilters lists the distinct values for the dashboard's filter dropdowns.
func (r *Router) handleCostFilters(w http.ResponseWriter, req *http.Request) {
	if _, ok := r.costGate(w, req); !ok {
		return
	}
	out := map[string][]string{}
	for key, col := range map[string]string{"features": "feature", "providers": "provider", "models": "model"} {
		rows, err := r.db.Query(req.Context(), "SELECT DISTINCT "+col+" FROM ai_call_log WHERE "+col+" <> '' ORDER BY 1 LIMIT 200")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		vals := []string{}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err == nil {
				vals = append(vals, v)
			}
		}
		rows.Close()
		out[key] = vals
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCostGateway shows ai-manager's own usage log for a month next to what this backend
// recorded locally for the same month. ai-manager is shared (other apps call it too), so the
// numbers are expected to differ; the page shows both rather than a "drift".
func (r *Router) handleCostGateway(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	month := req.URL.Query().Get("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	var from, to time.Time
	if month != "all" {
		t, err := time.ParseInLocation("2006-01", month, time.UTC)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "month must be YYYY-MM or all")
			return
		}
		from, to = t, t.AddDate(0, 1, 0)
	}
	resp := map[string]any{"month": month}

	if r.getDB() != nil {
		local := struct {
			Calls   int     `json:"calls"`
			Errors  int     `json:"errors"`
			Cost    float64 `json:"cost"`
			CostIDR float64 `json:"cost_idr"`
		}{}
		q, args := `SELECT COUNT(*), COALESCE(SUM(status='error'),0), COALESCE(SUM(cost_usd),0), COALESCE(SUM(cost_idr),0) FROM ai_call_log`, []any{}
		if month != "all" {
			q += " WHERE created_at >= ? AND created_at < ?"
			args = append(args, from, to)
		}
		if err := r.db.QueryRow(req.Context(), q, args...).Scan(&local.Calls, &local.Errors, &local.Cost, &local.CostIDR); err == nil {
			resp["local"] = local
		}
	}

	reader := r.aiService.Gateway()
	if reader == nil {
		resp["available"] = false
		resp["error"] = "ai-manager gateway is not configured"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	usage, err := reader.Usage(req.Context(), month)
	if err != nil {
		resp["available"] = false
		resp["error"] = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["available"] = true
	resp["usage"] = usage
	if recent, err := reader.Recent(req.Context(), 100); err == nil {
		resp["recent"] = recent
	}
	writeJSON(w, http.StatusOK, resp)
}
