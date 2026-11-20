package api

// CASE_AUTOMATION.md section 5: the live log of a case job and its approval gate.
//
//	GET  /api/jobs/{id}/events?since=<seq>  events newer than seq, plus the job's state
//	POST /api/jobs/{id}/approval            {decision: approve|reject|changes, text?}

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
)

// lookupJob finds a job in memory, then in the database.
func (r *Router) lookupJob(ctx context.Context, id string) (*jobRecord, error) {
	r.jobs.mu.RLock()
	rec, ok := r.jobs.jobs[id]
	var snap jobRecord
	if ok {
		snap = *rec
	}
	r.jobs.mu.RUnlock()
	if ok {
		return &snap, nil
	}
	d := r.getDB()
	if d == nil {
		return nil, nil
	}
	job, err := scanJobRow(d.QueryRow(ctx, `SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms FROM job_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// handleJobSub serves /api/jobs/{id}/{events|approval}.
func (r *Router) handleJobSub(w http.ResponseWriter, req *http.Request, jobID, sub string) {
	switch sub {
	case "events":
		r.handleJobEvents(w, req, jobID)
	case "approval":
		r.handleJobApproval(w, req, jobID)
	case "conversation":
		r.handleJobConversation(w, req, jobID) // AI_CONVERSATIONS.md
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func (r *Router) handleJobEvents(w http.ResponseWriter, req *http.Request, jobID string) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		log.Printf("job events: lookup: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if job == nil {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}
	since := intParam(req, "since")
	if since < 0 {
		since = 0
	}
	es := &caseauto.EventStore{DB: d}
	events, err := es.List(ctx, jobID, since, 500)
	if err != nil {
		log.Printf("job events: list: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	phase := ""
	if len(events) > 0 {
		phase = events[len(events)-1].Step
	}
	if err := d.QueryRow(ctx, `SELECT step FROM job_events WHERE job_id = ? ORDER BY seq DESC LIMIT 1`, jobID).Scan(&phase); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("job events: phase: %v", err)
	}
	out := map[string]any{"events": events, "status": job.Status, "phase": phase, "job_type": job.Type}
	if job.Error != "" {
		out["error"] = job.Error
	}
	p := job.Payload
	if v, ok := p["case_code"].(string); ok {
		out["case_code"] = v
	}
	if v, ok := p["branch"].(string); ok && v != "" {
		out["branch"] = v
	}
	if v := intFrom(p["product_id"]); v > 0 {
		out["product_id"] = v
	}
	if v, ok := p["pr_url"].(string); ok && v != "" {
		out["pr_url"] = v
	}
	if sp, _ := p["spec_path"].(string); sp != "" {
		content, _ := p["spec_content"].(string)
		out["spec"] = map[string]any{"path": sp, "content": content}
	}
	if rep, ok := p["report"].(map[string]interface{}); ok {
		out["report"] = rep
	} else if rep, ok := job.Result["summary"].(map[string]interface{}); ok {
		out["report"] = rep
	}
	if vd, ok := job.Result["verdict"].(map[string]interface{}); ok {
		if res, _ := vd["result"].(string); res != "" {
			out["outcome"] = res
		}
		if sr, _ := vd["skip_reason"].(string); sr != "" {
			out["skip_reason"] = sr
		}
	}
	if a, err := (&caseauto.ApprovalStore{DB: d}).Latest(ctx, jobID); err == nil && a != nil {
		out["approval"] = map[string]any{"status": a.Status, "gaos_created": a.GaosCreated, "answer_text": a.Text, "answered_by": a.AnsweredBy, "external_id": a.ExternalID}
	}
	writeJSON(w, http.StatusOK, out)
}

type approvalBody struct {
	Decision string `json:"decision"`
	Text     string `json:"text"`
}

var decisionStatus = map[string]string{
	"approve": caseauto.ApprovalApproved,
	"reject":  caseauto.ApprovalRejected,
	"changes": caseauto.ApprovalChanges,
}

// handleJobApproval is the dashboard door of the approval gate.
func (r *Router) handleJobApproval(w http.ResponseWriter, req *http.Request, jobID string) {
	if req.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, canVerify := productRole(req); !canVerify {
		writeJSONError(w, http.StatusForbidden, "your role cannot answer an approval")
		return
	}
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	var body approvalBody
	if !decodeStrictJSON(w, req, 1<<14, &body) {
		return
	}
	status, ok := decisionStatus[body.Decision]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "decision must be approve, reject or changes")
		return
	}
	text := strings.TrimSpace(body.Text)
	if len(text) > 1000 {
		writeJSONError(w, http.StatusBadRequest, "text is limited to 1000 characters")
		return
	}
	if status == caseauto.ApprovalChanges && text == "" {
		writeJSONError(w, http.StatusBadRequest, "say what to change")
		return
	}
	ctx := req.Context()
	store := &caseauto.ApprovalStore{DB: d}
	a, err := store.Latest(ctx, jobID)
	if err != nil {
		log.Printf("job approval: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if a == nil {
		writeJSONError(w, http.StatusNotFound, "this job has no approval")
		return
	}
	changed, err := store.Answer(ctx, a.ExternalID, status, "dashboard", r.caseAuto.redact.Redact(text))
	if err != nil {
		log.Printf("job approval: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !changed {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the approval was already answered", "status": a.Status})
		return
	}
	r.addJobEvent(jobID, "info", caseauto.StepApproval, "answered in the dashboard by "+answerBy(req)+": "+body.Decision)
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}
