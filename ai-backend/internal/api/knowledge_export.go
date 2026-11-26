package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/qa-automation/ai-backend/internal/services/knowledge"
)

// SetExportCache wires the module checklist export (MODULE_CHECKLIST_EXPORT.md). A nil cache
// leaves the endpoint answering 503. Callers must pass a real *ExportCache or nothing, never a
// typed nil stored in an interface.
func (r *Router) SetExportCache(c *knowledge.ExportCache) { r.exports = c }

// strictBool reads an optional query parameter that must be absent, 0, 1, true or false.
// ok is false (and the caller answers 400) for any other value, including an empty one.
func strictBool(req *http.Request, name string) (val bool, ok bool) {
	vs, present := req.URL.Query()[name]
	if !present {
		return false, true
	}
	if len(vs) != 1 {
		return false, false
	}
	switch vs[0] {
	case "1", "true":
		return true, true
	case "0", "false":
		return false, true
	}
	return false, false
}

// handleChecklistExport serves GET /api/products/{id}/knowledge/exports/modules-checklist.
// Check order: 405, 400, 404, 503, 409, 413 (the last two come from the cache as errors).
func (r *Router) handleChecklistExport(w http.ResponseWriter, req *http.Request, pid int) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var q knowledge.ExportQuery
	q.Format = req.URL.Query().Get("format")
	switch q.Format {
	case "pdf", "xlsx", "csv":
	default:
		writeJSONError(w, http.StatusBadRequest, "format must be pdf, xlsx or csv")
		return
	}
	var ok bool
	if q.LastUpdate, ok = strictBool(req, "last_update"); !ok {
		writeJSONError(w, http.StatusBadRequest, "last_update must be 0, 1, true or false")
		return
	}
	if q.Force, ok = strictBool(req, "force"); !ok {
		writeJSONError(w, http.StatusBadRequest, "force must be 0, 1, true or false")
		return
	}
	if msg := parseChecklistFilter(req, &q.Filter); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if r.exports == nil {
		// 404 comes before 503: an unknown product is not "storage not configured".
		if err := r.kb.ProductExists(req.Context(), pid); errors.Is(err, knowledge.ErrNotFound) {
			writeKnowledgeError(w, err)
			return
		}
		writeJSONError(w, http.StatusServiceUnavailable, "object storage not configured")
		return
	}
	res, err := r.exports.Serve(req.Context(), pid, q)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", res.ContentType)
	h.Set("Content-Length", strconv.Itoa(len(res.Body)))
	// Filename is built from [A-Za-z0-9._-] only (knowledge.ExportBaseName), so it needs no escaping.
	h.Set("Content-Disposition", `attachment; filename="`+res.Filename+`"`)
	h.Set("X-Export-Cache", res.Cache)
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Body)
}

// parseChecklistFilter reads scope (all|prod, default all), since_from and since_to (YYYY-MM,
// optional, only with scope=prod, from <= to), prod_since=unknown (only with scope=prod and no
// range: PROD rows without a known date) and check_column. It returns a 400 message, or "".
func parseChecklistFilter(req *http.Request, f *knowledge.ChecklistFilter) string {
	qv := req.URL.Query()
	for _, k := range []string{"scope", "since_from", "since_to", "prod_since"} {
		if len(qv[k]) > 1 {
			return k + " must be given once"
		}
	}
	switch qv.Get("scope") {
	case "", knowledge.ScopeAll:
		f.Scope = knowledge.ScopeAll
	case knowledge.ScopeProd:
		f.Scope = knowledge.ScopeProd
	default:
		return "scope must be all or prod"
	}
	f.From, f.To = qv.Get("since_from"), qv.Get("since_to")
	for _, m := range []string{f.From, f.To} {
		if m != "" && !knowledge.ValidMonth(m) {
			return "since_from and since_to must be YYYY-MM"
		}
	}
	if f.Ranged() && f.Scope != knowledge.ScopeProd {
		return "since_from and since_to need scope=prod"
	}
	if f.From != "" && f.To != "" && f.From > f.To {
		return "since_from must not be after since_to"
	}
	switch qv.Get("prod_since") {
	case "":
	case "unknown":
		if f.Scope != knowledge.ScopeProd {
			return "prod_since=unknown needs scope=prod"
		}
		if f.Ranged() {
			return "prod_since=unknown cannot be combined with since_from or since_to"
		}
		f.UndatedOnly = true
	default:
		return "prod_since must be unknown"
	}
	check, ok := strictBool(req, "check_column")
	if !ok {
		return "check_column must be 0, 1, true or false"
	}
	if _, present := req.URL.Query()["check_column"]; present && !check {
		f.HideCheck = true
	}
	if f.ShowDevPeriod, ok = strictBool(req, "dev_period"); !ok {
		return "dev_period must be 0, 1, true or false"
	}
	return ""
}
