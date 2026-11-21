package api

// CASE_AUTOMATION.md section 5: the live log of a case job and its approval gate.
//
//	GET  /api/jobs/{id}/events?since=<seq>  events newer than seq, plus the job's state
//	POST /api/jobs/{id}/approval            {decision: approve|reject|changes, text?, via?, external_id?}
//
// via (HISTORY_ANSWER.md) is the door the answer was given at: job-page (default), history
// (relayed to GAOS when the item exists there, approval_relay.go) or learning.

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
		out["approval"] = map[string]any{"status": a.Status, "gaos_created": a.GaosCreated, "answer_text": a.Text, "answered_by": a.AnsweredBy, "external_id": a.ExternalID,
			"source": a.Source, "actor": a.Actor, "answered_at": a.AnsweredAt}
	}
	writeJSON(w, http.StatusOK, out)
}

type approvalBody struct {
	Decision   string `json:"decision"`
	Text       string `json:"text"`
	Via        string `json:"via"`         // job-page (default) | history | learning
	ExternalID string `json:"external_id"` // the round the human saw; required for via = history
}

var decisionStatus = map[string]string{
	"approve": caseauto.ApprovalApproved,
	"reject":  caseauto.ApprovalRejected,
	"changes": caseauto.ApprovalChanges,
}

// viaEvent is how a local answer's job event names its door.
var viaEvent = map[string]string{
	caseauto.SourceJobPage:  "answered in the dashboard",
	caseauto.SourceHistory:  "answered from the run history",
	caseauto.SourceLearning: "answered on the learning page",
}

// handleJobApproval is the dashboard door of the approval gate (job page, run history, learning page).
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
	source, ok := sourceOfVia[body.Via]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "via must be job-page, history or learning")
		return
	}
	if body.Via == "history" && body.ExternalID == "" {
		writeJSONError(w, http.StatusBadRequest, "external_id is required from the run history")
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
	if body.ExternalID != "" && body.ExternalID != a.ExternalID {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "a newer approval round exists", "code": "newer_round", "status": a.Status})
		return
	}
	already := func(cur *caseauto.Approval) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the approval was already answered", "code": "already_answered",
			"status": cur.Status, "source": cur.Source, "actor": cur.Actor})
	}
	if a.Status != caseauto.ApprovalOpen {
		already(a)
		return
	}
	text = r.caseAuto.redact.Redact(text)
	actor, ga := answerActor(req)
	meta := caseauto.AnswerMeta{By: "dashboard", Text: text, Source: source, Actor: actor}

	note := ""
	if source == caseauto.SourceHistory {
		unlock, ok := r.approvalRelay.tryLock(a.ExternalID)
		if !ok {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "an answer to this approval is being sent", "code": "in_flight", "status": a.Status})
			return
		}
		defer unlock()
		if a.GaosCreated {
			if _, pid, _, err := store.ByExternal(ctx, a.ExternalID); err == nil {
				if gp := r.gaosProductFor(ctx, d, pid); gp != "" && ga.Username != "" {
					out := r.relayAnswer(req, d, a, jobID, gp, body.Decision, status, text, meta, ga)
					if !out.fallback {
						writeJSON(w, out.httpStatus, out.body)
						return
					}
					note = out.note
				}
			}
		}
	}
	changed, err := store.AnswerSourced(ctx, a.ExternalID, status, meta)
	if err != nil {
		log.Printf("job approval: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !changed {
		if cur, err := store.Get(ctx, a.ExternalID); err == nil && cur != nil {
			already(cur)
			return
		}
		already(a)
		return
	}
	msg := viaEvent[source] + " by " + actor + ": " + body.Decision
	level := "info"
	if note != "" {
		msg += "\n" + note
		level = "warn"
	}
	r.addJobEvent(jobID, level, caseauto.StepApproval, msg)
	out := map[string]any{"status": status, "delivered": "local"}
	if note != "" {
		out["note"] = note
	}
	writeJSON(w, http.StatusOK, out)
}
