package api

// GAOS_AUTOMATION.md: the two routes GAOS calls to start a case automation and to watch the job.
//
//	POST /api/v1/gaos/automation   {product, caseCode, mode}  -> 202 {job_id}
//	GET  /api/v1/gaos/jobs/{id}                              -> the job's state
//
// Same guard as the other GAOS routes (serveGAOSInbound: bearer GAOS_INBOUND_TOKEN, fail-closed).
// The work itself is the existing case automation path (enqueueCaseAction) and the existing
// caseauto machine; this file only translates GAOS's request into it and reads the job back.

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

const (
	gaosAutomationPath = "/api/v1/gaos/automation"
	gaosJobsPrefix     = "/api/v1/gaos/jobs/"
	// gaosMaxJobIDLen bounds the job id of the status route.
	gaosMaxJobIDLen = 100
	// gaosTriggeredBy marks a job GAOS started: there is no session user on these routes.
	gaosTriggeredBy = "gaos"
)

// gaosAutomationBody is what GAOS sends. Unknown fields are tolerated (GAOS may add some).
type gaosAutomationBody struct {
	Product  string `json:"product"`
	CaseCode string `json:"caseCode"`
	Mode     string `json:"mode"`
}

// gaosCaseAction maps GAOS's mode to the action the case automation path uses.
func gaosCaseAction(mode string) (string, bool) {
	switch mode {
	case "create":
		return "create-automation", true
	case "run":
		return "run", true
	}
	return "", false
}

// gaosAutomation starts the automation of one case. The order of the checks is fixed:
// body and mode (422), product (404), case (404), then the shared enqueue path (503, 409).
func (r *Router) gaosAutomation(w http.ResponseWriter, req *http.Request) {
	var body gaosAutomationBody
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, gaosInboundMaxBody))
	if err := dec.Decode(&body); err != nil {
		writeJSONError(w, http.StatusUnprocessableEntity, "invalid JSON body")
		return
	}
	action, ok := gaosCaseAction(body.Mode)
	if !ok {
		writeJSONError(w, http.StatusUnprocessableEntity, "mode must be create or run")
		return
	}
	ctx := req.Context()
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	pid, found, err := r.gaosProductLocalID(ctx, body.Product)
	if err != nil {
		log.Printf("gaos automation: product lookup: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown product")
		return
	}
	// An unknown (or removed) case is refused before anything is started.
	c, err := r.loadCase(ctx, d, pid, body.CaseCode)
	if err != nil {
		log.Printf("gaos automation: load case: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if c == nil {
		writeJSONError(w, http.StatusNotFound, "unknown case")
		return
	}
	res := r.enqueueCaseAction(ctx, d, caseEnqueueIn{PID: pid, Code: body.CaseCode, Action: action, TriggeredBy: gaosTriggeredBy})
	if e := res.Err; e != nil {
		if res.ActiveJobID != "" {
			writeJSON(w, e.Status, map[string]any{"error": e.Msg, "job_id": res.ActiveJobID})
			return
		}
		writeJSONError(w, e.Status, e.Msg)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": res.JobID})
}

// gaosJobStatus answers the state of one job: the same job, the same last step and the same
// counts the job page shows (jobPhase, jobReportCounts).
func (r *Router) gaosJobStatus(w http.ResponseWriter, req *http.Request, jobID string) {
	if jobID == "" || len(jobID) > gaosMaxJobIDLen || strings.Contains(jobID, "/") {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx := req.Context()
	job, err := r.lookupJob(ctx, jobID)
	if err != nil {
		log.Printf("gaos job status: lookup: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if job == nil {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}
	phase, _ := jobPhase(ctx, d, jobID)
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":   job.ID,
		"status":   job.Status,
		"phase":    phase,
		"job_type": job.Type,
		"error":    job.Error,
		"report":   jobReportCounts(job),
	})
}

// gaosJobID is the job id of a /api/v1/gaos/jobs/{id} path ("" when the path has none).
func gaosJobID(path string) string {
	if !strings.HasPrefix(path, gaosJobsPrefix) {
		return ""
	}
	return strings.TrimPrefix(path, gaosJobsPrefix)
}
