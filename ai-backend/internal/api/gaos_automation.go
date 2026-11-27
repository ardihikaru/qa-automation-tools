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
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
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
	// gaosCancelSuffix ends the cancel route: /api/v1/gaos/jobs/{id}/cancel.
	gaosCancelSuffix = "/cancel"
	// gaosCanceledMsg is the error of a job GAOS canceled, and the text of its rejected approvals.
	gaosCanceledMsg = "canceled by GAOS"
)

// gaosJobCancels holds the cancel func of every job whose goroutine is running in this process.
var gaosJobCancels sync.Map

// gaosCancelMu makes "is the job still live" and "mark it canceled" one step.
var gaosCancelMu sync.Mutex

// registerJobCancel records a running job's cancel func; the returned func removes it.
func registerJobCancel(id string, cancel context.CancelFunc) func() {
	gaosJobCancels.Store(id, cancel)
	return func() { gaosJobCancels.Delete(id) }
}

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
	// GAOS_RELEASE_B.md §3.2: the deployed repository's kind, and whether GAOS asks for a
	// pinned auto-merge of the approved spec. Both optional; absent = today's behaviour.
	Kind      string `json:"kind"`
	AutoMerge *bool  `json:"auto_merge"`
}

// The repository kinds (GAOS_RELEASE_B.md §3.2).
const (
	gaosKindBE        = "be"
	gaosKindFullstack = "fullstack"
)

// gaosKindLevels are the case levels a run-all of a kind runs; nil = every level.
func gaosKindLevels(kind string) []string {
	if kind == gaosKindBE {
		return []string{"API", "GQL"}
	}
	return nil
}

// gaosKindFitsLevel reports whether a create of a case of level may be asked with kind.
func gaosKindFitsLevel(kind, level string) bool {
	switch kind {
	case gaosKindBE:
		return level == "API"
	case gaosKindFullstack:
		return level == "WEB" || level == "E2E"
	}
	return true
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
	if body.Kind != "" && body.Kind != gaosKindBE && body.Kind != gaosKindFullstack {
		writeJSONError(w, http.StatusUnprocessableEntity, "kind must be be or fullstack")
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
		r.gaosRunAll(w, ctx, d, pid, body.Kind)
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
	in := caseEnqueueIn{PID: pid, Code: body.CaseCode, Action: action, TriggeredBy: gaosTriggeredBy, Kind: body.Kind}
	if body.Mode == "create" {
		if body.Kind != "" {
			level, err := caseLevel(ctx, d, pid, body.CaseCode)
			if err != nil {
				log.Printf("gaos automation: case level: %v", err)
				writeJSONError(w, http.StatusInternalServerError, "lookup failed")
				return
			}
			if !gaosKindFitsLevel(body.Kind, level) {
				writeJSONError(w, http.StatusUnprocessableEntity, "kind does not match the case level")
				return
			}
		}
		in.AutoMerge = body.AutoMerge != nil && *body.AutoMerge
	}
	res := r.enqueueCaseAction(ctx, d, in)
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
	out := map[string]any{
		"job_id":   job.ID,
		"status":   gaosStatus(job.Status),
		"phase":    phase,
		"job_type": gaosJobType(job.Type),
		"error":    job.Error,
		"report":   jobReportCounts(job),
	}
	// The pull request the job opened, once it has one (the payload while the job runs, the result
	// after it), and the job's page; both only when known. GAOS lists the pull request on its
	// Pull requests page and accepts the page link only on the host it is configured to trust.
	if u := gaosPRURL(job); u != "" {
		out["pr_url"] = u
	}
	// GAOS_RELEASE_B.md §3.3: the commit pushed after the approval, and the auto-merge outcome.
	if v := gaosPayloadString(job, "pushed_sha"); v != "" {
		out["pushed_sha"] = v
	}
	if v := gaosPayloadString(job, "auto_merge"); v != "" {
		out["auto_merge"] = v
	} else if req, _ := job.Payload["auto_merge_requested"].(bool); req && gaosPRURL(job) != "" &&
		job.Status != jobStatusRunning && job.Status != jobStatusQueued {
		// The job ended (a restart completed it) between opening the pull request and the
		// auto-merge step: nothing was armed, and GAOS is told so (GAOS_RELEASE_B.md §3.3).
		out["auto_merge"] = "refused:restarted"
	}
	if o := strings.TrimRight(r.dashboardOrigin, "/"); o != "" && o != "*" {
		out["job_url"] = o + "/jobs/" + job.ID
	}
	// A running create that waits for a human is not "running" to GAOS: its poll must not time out.
	if job.Status == jobStatusRunning {
		if age, open := latestApprovalOpen(ctx, d, jobID); open {
			out["status"] = "awaiting_approval"
			out["waiting_s"] = age
		}
	}
	writeJSON(w, http.StatusOK, out)
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
func (r *Router) gaosRunAll(w http.ResponseWriter, ctx context.Context, d *db.DB, pid int, kind string) {
	gaosRunAllMu.Lock()
	defer gaosRunAllMu.Unlock()
	var active string
	// One active run-all per product and kind: a be run-all is not a fullstack one.
	if err := d.QueryRow(ctx, `SELECT COALESCE(MAX(id), '') FROM job_runs WHERE type = ? AND status IN ('queued','running')
		AND CAST(JSON_EXTRACT(payload, '$.product_id') AS UNSIGNED) = ?
		AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.kind')), '') = ?`, gaosRunAllType, pid, kind).Scan(&active); err != nil {
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
	codes, err := automatedCaseCodes(ctx, d, pid, gaosKindLevels(kind)...)
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
			"phase": "queued", "total": len(codes), "kind": kind}}
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

// automatedCaseCodes are the product's live cases that have a spec; with levels, only cases of
// those levels (a run-all of kind be, GAOS_RELEASE_B.md §3.2).
func automatedCaseCodes(ctx context.Context, d *db.DB, pid int, levels ...string) ([]string, error) {
	q := `SELECT code FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL
		AND spec_paths IS NOT NULL AND JSON_LENGTH(spec_paths) > 0`
	args := []interface{}{pid}
	if len(levels) > 0 {
		q += ` AND level IN (?` + strings.Repeat(`,?`, len(levels)-1) + `)`
		for _, l := range levels {
			args = append(args, l)
		}
	}
	rows, err := d.Query(ctx, q+` ORDER BY code`, args...)
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
	canceled := func() bool {
		j, _ := r.lookupJob(ctx, jobID)
		return j != nil && j.Status == jobStatusFailed && j.Error == gaosCanceledMsg
	}
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
		if canceled() {
			return // GAOS canceled the job: stop after the case that was in flight
		}
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

// latestApprovalOpen reports whether the job's newest approval is still open, and its age in
// seconds. Any read error or no approval answers false (the status stays running).
func latestApprovalOpen(ctx context.Context, d *db.DB, jobID string) (int64, bool) {
	var status string
	var age int64
	err := d.QueryRow(ctx, `SELECT status, GREATEST(TIMESTAMPDIFF(SECOND, created_at, UTC_TIMESTAMP()), 0) FROM case_approvals WHERE job_id = ? ORDER BY id DESC LIMIT 1`, jobID).Scan(&status, &age)
	if err != nil {
		return 0, false
	}
	return age, status == "open"
}

// gaosJobCancel cancels a job that is still queued, running or awaiting approval: its open
// approvals are rejected first (so nothing can be approved, and the Machine, which reads the
// approval before it commits, can never reach push), then its context is canceled and the job is
// marked failed "canceled by GAOS". A job that already ended is answered as it is.
func (r *Router) gaosJobCancel(w http.ResponseWriter, req *http.Request, jobID string) {
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
	gaosCancelMu.Lock()
	defer gaosCancelMu.Unlock()
	job, err := r.lookupJob(ctx, jobID)
	if err != nil {
		log.Printf("gaos job cancel: lookup: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if job == nil {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}
	if job.Status != jobStatusQueued && job.Status != jobStatusRunning {
		writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "status": gaosStatus(job.Status), "error": job.Error, "canceled": false})
		return
	}
	if err := r.rejectOpenApprovals(ctx, d, jobID); err != nil {
		log.Printf("gaos job cancel: approvals: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := time.Now().UTC()
	if r.updateJob(jobID, func(j *jobRecord) {
		j.Status, j.Error, j.CompletedAt = jobStatusFailed, gaosCanceledMsg, &now
		if j.StartedAt != nil {
			j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
		}
	}) == nil {
		// Not in memory (read back from the database): write the row itself.
		if _, err := d.Exec(ctx, `UPDATE job_runs SET status = 'failed', error = ?, completed_at = UTC_TIMESTAMP() WHERE id = ? AND status IN ('queued','running')`, gaosCanceledMsg, jobID); err != nil {
			log.Printf("gaos job cancel: mark failed: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if c, ok := gaosJobCancels.Load(jobID); ok {
		c.(context.CancelFunc)()
	}
	// A spec whose auto-merge is (or may be) armed is disarmed: a canceled card never merges
	// (GAOS_RELEASE_B.md §4.2). A failure is logged; the job stays canceled either way.
	if am := gaosPayloadString(job, "auto_merge"); (am == caseauto.AutoMergeEnabled || am == autoMergePending) && r.repoPool != nil {
		if u := gaosPRURL(job); u != "" {
			if err := r.repoPool.DisableAutoMerge(context.WithoutCancel(ctx), u); err != nil {
				log.Printf("gaos job cancel: disable auto-merge: %v", err)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "status": "failed", "error": gaosCanceledMsg, "canceled": true})
}

// rejectOpenApprovals answers every open approval of the job as rejected, source GAOS. The answer
// is the store's own first-answer-wins update, so a late approve finds the row closed.
func (r *Router) rejectOpenApprovals(ctx context.Context, d *db.DB, jobID string) error {
	rows, err := d.Query(ctx, `SELECT external_id FROM case_approvals WHERE job_id = ? AND status = 'open'`, jobID)
	if err != nil {
		return err
	}
	var exts []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			rows.Close()
			return err
		}
		exts = append(exts, e)
	}
	rows.Close()
	store := &caseauto.ApprovalStore{DB: d}
	for _, e := range exts {
		if _, err := store.AnswerSourced(ctx, e, caseauto.ApprovalRejected, caseauto.AnswerMeta{By: "gaos", Text: gaosCanceledMsg, Source: caseauto.SourceGAOS}); err != nil {
			return err
		}
	}
	return nil
}

// gaosPRURL is the pull request url a job recorded: payload.pr_url while it runs (caseauto sets it
// when it opens the pull request), result.pr_url once it ends. "" when there is none or it is not a string.
func gaosPRURL(job *jobRecord) string {
	for _, m := range []map[string]interface{}{job.Payload, job.Result} {
		if u, ok := m["pr_url"].(string); ok && u != "" {
			return u
		}
	}
	return ""
}

// caseLevel is a case's level (API, GQL, WEB, E2E; "" when unset).
func caseLevel(ctx context.Context, d *db.DB, pid int, code string) (string, error) {
	var level string
	err := d.QueryRow(ctx, `SELECT level FROM kb_test_cases WHERE product_id = ? AND code = ?`, pid, code).Scan(&level)
	return level, err
}

// gaosPayloadString is a string the job recorded under key, in its payload or its result.
func gaosPayloadString(job *jobRecord, key string) string {
	for _, m := range []map[string]interface{}{job.Payload, job.Result} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
