package api

// CASE_AUTOMATION.md section 5: the GAOS door of the approval gate.
//
//	POST /api/v1/gaos/inbox-answer  {externalId, productId, status, choice, optionLabel, text, answeredAt}
//
// Same guard as the other GAOS routes (serveGAOSInbound: bearer GAOS_INBOUND_TOKEN, fail-closed).
// Idempotent: the first answer of an approval wins; a repeat or a late answer changes nothing.

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

const gaosInboxAnswerPath = "/api/v1/gaos/inbox-answer"

var inboxExternalRe = regexp.MustCompile(`^qa-[A-Za-z0-9-]{1,60}-pr[0-9]{0,2}$`)

type inboxAnswerBody struct {
	ExternalID  string          `json:"externalId"`
	ProductID   string          `json:"productId"`
	Status      string          `json:"status"`
	Choice      json.RawMessage `json:"choice"`
	OptionLabel string          `json:"optionLabel"`
	Text        string          `json:"text"`
	AnsweredAt  json.RawMessage `json:"answeredAt"`
	// Source is GAOS's report of who answered and through which client (HISTORY_ANSWER.md);
	// stored verbatim when GAOS sends it.
	Source json.RawMessage `json:"source"`
}

func (r *Router) gaosInboxAnswer(w http.ResponseWriter, req *http.Request) {
	var body inboxAnswerBody
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)) // unknown fields tolerated: GAOS may add some
	if err := dec.Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !inboxExternalRe.MatchString(body.ExternalID) {
		writeJSONError(w, http.StatusBadRequest, "bad externalId")
		return
	}
	ctx := req.Context()
	store := &caseauto.ApprovalStore{DB: r.getDB()}
	jobID, pid, _, err := store.ByExternal(ctx, body.ExternalID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "unknown approval")
		return
	}
	// The GAOS product must be the approval's product (and flagged), or the answer is refused.
	local, ok, err := r.gaosProductLocalID(ctx, body.ProductID)
	if err != nil {
		log.Printf("gaos inbox-answer: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok || local != pid {
		writeJSONError(w, http.StatusNotFound, "unknown approval")
		return
	}
	var status string
	switch body.Status {
	case gaossync.InboxAnswered:
		var ok bool
		if status, ok = caseauto.DecisionFromAnswer(body.OptionLabel, string(body.Choice)); !ok {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": false, "ignored": "unrecognised answer"})
			return
		}
	case gaossync.InboxExpired:
		status = caseauto.ApprovalExpired
	default:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": false, "ignored": "not answered"})
		return
	}
	changed, current, err := r.applyGaosAnswer(ctx, r.getDB(), body.ExternalID, jobID, status, body.Text, body.Source, "answered in the GAOS Inbox")
	if err != nil {
		log.Printf("gaos inbox-answer: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": changed, "status": current})
}
