package api

import "net/http"

// handleTranslations serves /api/products/{id}/knowledge/translations/{status|run}
// (KNOWLEDGE_TRANSLATION.md). status is open to any reader; run needs canWrite.
func (r *Router) handleTranslations(w http.ResponseWriter, req *http.Request, pid int, action string, canWrite bool) {
	t := r.kbTranslator
	if t == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "translation not available")
		return
	}
	switch action {
	case "status":
		if req.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		out, err := t.TranslationStatus(req.Context(), pid)
		respond(w, out, err)
	case "run":
		if req.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !canWrite {
			writeJSONError(w, http.StatusForbidden, "your role cannot change the knowledge base")
			return
		}
		id, err := t.StartRun(req.Context(), pid)
		if err != nil {
			writeKnowledgeError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"run_id": id})
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}
