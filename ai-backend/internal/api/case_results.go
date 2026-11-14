package api

// CASE_RESULTS.md: per-case results from a Playwright JSON report.
//   POST /api/products/{id}/case-results       cookie auth (Admin, DevOps, Developer, QA, QC): upload from the dashboard.
//   POST /api/v1/results/playwright?product=N  bearer RESULTS_INGEST_TOKEN: the automation repo's CI.
// The body is the report (results.json) itself. Both store the latest result per case in
// kb_case_results; GET /api/v1/gaos/results then serves them to GAOS.

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/caseresults"
)

const caseResultsMaxBody = 24 << 20 // below nginx client_max_body_size (25m)

// IsResultsIngestPath is the bearer-authenticated CI route; auth.IsAuthAllowList mirrors it.
func IsResultsIngestPath(path string) bool { return path == "/api/v1/results/playwright" }

// SetResultsIngestToken enables the CI route. Empty = disabled (503, fail-closed).
func (r *Router) SetResultsIngestToken(token string) { r.resultsToken = token }

// ResultsIngestEnabled reports whether the CI token is set.
func (r *Router) ResultsIngestEnabled() bool { return r.resultsToken != "" }

func (r *Router) serveResultsIngest(w http.ResponseWriter, req *http.Request) {
	if r.resultsToken == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "results ingest is not configured")
		return
	}
	if !bearerMatches(req.Header.Get("Authorization"), r.resultsToken) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="qa-results"`)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if req.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pid, err := strconv.Atoi(req.URL.Query().Get("product"))
	if err != nil || pid < 1 {
		writeJSONError(w, http.StatusBadRequest, "product must be a product id")
		return
	}
	r.ingestReport(w, req, pid, "ci")
}

// handleCaseResults serves POST /api/products/{id}/case-results (cookie auth already done).
func (r *Router) handleCaseResults(w http.ResponseWriter, req *http.Request, pid int, canVerify bool) {
	if req.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !canVerify {
		writeJSONError(w, http.StatusForbidden, "your role cannot upload test results")
		return
	}
	r.ingestReport(w, req, pid, "upload")
}

func (r *Router) ingestReport(w http.ResponseWriter, req *http.Request, pid int, source string) {
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx := req.Context()
	var one int
	if err := d.QueryRow(ctx, `SELECT 1 FROM products WHERE id = ?`, pid).Scan(&one); err != nil {
		writeJSONError(w, http.StatusNotFound, "unknown product")
		return
	}
	runRef := strings.TrimSpace(req.URL.Query().Get("run_ref"))
	if len(runRef) > 120 || !asciiPrintable(runRef) {
		writeJSONError(w, http.StatusBadRequest, "run_ref must be printable ASCII, at most 120 characters")
		return
	}
	rep, err := caseresults.Parse(io.LimitReader(req.Body, caseResultsMaxBody+1), time.Now().UTC())
	switch {
	case errors.Is(err, caseresults.ErrNotReport):
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil:
		writeJSONError(w, http.StatusBadRequest, "invalid or too large report: "+firstLine(err))
		return
	}
	if runRef == "" {
		runRef = caseresults.DefaultRunRef(rep.StartedAt)
	}
	out, err := caseresults.Save(ctx, d, pid, rep, runRef, source)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "store failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tests": rep.Tests, "cases": len(rep.Results), "run_ref": runRef, "run_at": rep.StartedAt.Format(time.RFC3339),
		"stored": out.Stored, "stale": out.Stale, "unmatched": out.Unmatched, "unmatched_count": out.UnmatchedN,
	})
}
