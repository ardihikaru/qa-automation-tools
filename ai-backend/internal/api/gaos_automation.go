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
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
)

const (
	gaosAutomationPath = "/api/v1/gaos/automation"
	gaosJobsPrefix     = "/api/v1/gaos/jobs/"
	// gaosMaxJobIDLen bounds the job id of the status route.
	gaosMaxJobIDLen = 100
	// gaosTriggeredBy marks a job GAOS started: there is no session user on these routes.
	gaosTriggeredBy = "gaos"
	// gaosRunAllType is the job_runs type of a "run every automated case of the product" job. It is
	// not a case type, so a restart marks it failed (FailOrphanedJobs) instead of resuming it.
	gaosRunAllType = "gaos_run_all"
	// gaosAllCases is the caseCode that means "every automated case of the product".
	gaosAllCases = "*"
	// gaosRunAllTimeout bounds one run-all job; GAOS gives up after 40 minutes, this frees the slot.
	gaosRunAllTimeout = 3 * time.Hour
)

// gaosRunAllMu makes "is a run-all active for this product" and "start one" one step.
var gaosRunAllMu sync.Mutex

// gaosStatus maps the job record's status to GAOS's vocabulary: queued, running, succeeded, failed.
func gaosStatus(s jobStatus) string {
	if s == jobStatusCompleted {
		return "succeeded"
	}
	return string(s)
}

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
	if len(body.CaseCode) == 0 || len(body.CaseCode) > 64 {
		writeJSONError(w, http.StatusUnprocessableEntity, "caseCode is required")
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
	if body.CaseCode == gaosAllCases {
		if body.Mode != "run" {
			writeJSONError(w, http.StatusUnprocessableEntity, "caseCode * is only valid with mode run")
			return
		}
		r.gaosRunAll(w, ctx, d, pid)
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
		// The same job is already active: answer with it, as if this call had started it.
		if res.ActiveJobID != "" && e.Code == "case_active" {
			writeJSON(w, http.StatusOK, map[string]any{"job_id": res.ActiveJobID})
			return
		}
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
	phase, ok := jobPhase(ctx, d, jobID)
	if p, _ := job.Payload["phase"].(string); !ok && p != "" {
		phase = p
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":   job.ID,
		"status":   gaosStatus(job.Status),
		"phase":    phase,
		"job_type": gaosJobType(job.Type),
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

// gaosRunAll starts (or finds) the job that runs every automated case of the product, one after
// the other, through the same enqueueCaseAction path as a single run.
func (r *Router) gaosRunAll(w http.ResponseWriter, ctx context.Context, d *db.DB, pid int) {
	gaosRunAllMu.Lock()
	defer gaosRunAllMu.Unlock()
	var active string
	if err := d.QueryRow(ctx, `SELECT COALESCE(MAX(id), '') FROM job_runs WHERE type = ? AND status IN ('queued','running')
		AND CAST(JSON_EXTRACT(payload, '$.product_id') AS UNSIGNED) = ?`, gaosRunAllType, pid).Scan(&active); err != nil {
		log.Printf("gaos run-all: active check: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if active != "" {
		writeJSON(w, http.StatusOK, map[string]any{"job_id": active})
		return
	}
	if r.executor == nil || r.repoPool == nil || r.caseAuto.hostPool == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "case automation is not configured")
		return
	}
	codes, err := automatedCaseCodes(ctx, d, pid)
	if err != nil {
		log.Printf("gaos run-all: list cases: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if len(codes) == 0 {
		writeJSONError(w, http.StatusConflict, "the product has no automated case to run")
		return
	}
	now := time.Now().UTC()
	job := &jobRecord{ID: newJobID(), Type: gaosRunAllType, Status: jobStatusRunning, CreatedAt: now, StartedAt: &now,
		Payload: map[string]interface{}{"product_id": pid, "case_code": gaosAllCases, "triggered_by": gaosTriggeredBy,
			"phase": "queued", "total": len(codes)}}
	if err := r.upsertJobRecordErr(ctx, job); err != nil {
		log.Printf("gaos run-all: enqueue: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	r.jobs.mu.Lock()
	r.jobs.jobs[job.ID] = job
	r.jobs.mu.Unlock()
	go r.runAllCases(job.ID, pid, codes)
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID})
}

// automatedCaseCodes are the product's live cases that have a spec.
func automatedCaseCodes(ctx context.Context, d *db.DB, pid int) ([]string, error) {
	rows, err := d.Query(ctx, `SELECT code FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL
		AND spec_paths IS NOT NULL AND JSON_LENGTH(spec_paths) > 0 ORDER BY code`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// runAllCases is the run-all job body: each case is one ordinary case_run job, started through
// enqueueCaseAction and awaited before the next. A refused case counts as skipped; a case job that
// fails counts as failed; the run-all job itself fails only when nothing can be started.
func (r *Router) runAllCases(jobID string, pid int, codes []string) {
	ctx, cancel := context.WithTimeout(context.Background(), gaosRunAllTimeout)
	defer cancel()
	counts := map[string]int{"passed": 0, "failed": 0, "skipped": 0}
	finish := func(status jobStatus, msg string) {
		r.updateJob(jobID, func(j *jobRecord) {
			now := time.Now().UTC()
			j.Status, j.Error, j.CompletedAt = status, msg, &now
			j.Payload["phase"] = "done"
			j.Payload["report"] = map[string]interface{}{"passed": counts["passed"], "failed": counts["failed"], "skipped": counts["skipped"]}
		})
	}
	d := r.getDB()
	for i, code := range codes {
		r.updateJob(jobID, func(j *jobRecord) {
			j.Payload["phase"] = "run " + code + " (" + strconv.Itoa(i+1) + "/" + strconv.Itoa(len(codes)) + ")"
		})
		res := r.enqueueCaseAction(ctx, d, caseEnqueueIn{PID: pid, Code: code, Action: "run", TriggeredBy: gaosTriggeredBy})
		childID := res.JobID
		if e := res.Err; e != nil {
			switch {
			case e.Code == "case_active" && res.ActiveJobID != "":
				childID = res.ActiveJobID
			case e.Status == http.StatusServiceUnavailable:
				finish(jobStatusFailed, e.Msg)
				return
			default:
				counts["skipped"]++
				continue
			}
		}
		child, ok := r.awaitJob(ctx, childID)
		if !ok {
			finish(jobStatusFailed, "timed out waiting for "+code)
			return
		}
		if child.Status == jobStatusFailed {
			counts["failed"]++
			continue
		}
		c := jobReportCounts(child)
		for _, k := range []string{"passed", "failed", "skipped"} {
			counts[k] += c[k]
		}
	}
	finish(jobStatusCompleted, "")
}

// awaitJob polls a job until it is completed or failed (false when ctx ends first).
func (r *Router) awaitJob(ctx context.Context, id string) (*jobRecord, bool) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		if j, err := r.lookupJob(ctx, id); err == nil && j != nil && (j.Status == jobStatusCompleted || j.Status == jobStatusFailed) {
			return j, true
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-t.C:
		}
	}
}

// gaosJobType is GAOS's name for a job type: create or run.
func gaosJobType(t string) string {
	if t == "case_create" {
		return "create"
	}
	return "run"
}
