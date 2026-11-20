package api

// AI_CONVERSATIONS.md: stored AI conversations.
//
//	GET  /api/conversations?archived=0|1&kind&product&q    list (every signed-in user; metadata only)
//	GET  /api/conversations/{id}?since_seq=N               one conversation with its messages (canVerify)
//	POST /api/conversations/{id}/archive | /unarchive      canVerify
//	GET  /api/conversations/prune                          preview (real Admin)
//	POST /api/conversations/prune                          {"confirm":"prune","archived_before":"…"} (real Admin)
//	GET  /api/conversations/prunes                         prune audit (real Admin)
//	GET  /api/jobs/{id}/conversation?since_seq=N           the job's conversation (job_events.go dispatch)

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/conversations"
)

// PruneTimeLayout is the cut-off format: UTC with milliseconds (archived_at is DATETIME(3)).
const PruneTimeLayout = "2006-01-02T15:04:05.000Z07:00"

var convQRe = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,64}$`)

// convActor is who really acts (the impersonator when there is one) and the identity used.
func convActor(req *http.Request) (actorID, actorName, asUser string) {
	u, ok := auth.GetUser(req.Context())
	if !ok {
		return "", "", ""
	}
	if u.Impersonator != nil {
		return u.Impersonator.ID, u.Impersonator.Username, u.ID
	}
	return u.ID, u.Username, ""
}

// realAdmin is an Admin acting as itself: an impersonated session (Service included) never is.
func realAdmin(req *http.Request) bool {
	u, ok := auth.GetUser(req.Context())
	return ok && auth.RequireRealUser(u) && u.Role == "Admin"
}

func (r *Router) handleConversations(w http.ResponseWriter, req *http.Request) {
	d := r.getDB()
	if d == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unavailable", "database unavailable")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/conversations"), "/")
	parts := strings.Split(rest, "/")
	switch {
	case rest == "":
		if req.Method != http.MethodGet {
			writeCodedError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		r.listConversations(w, req)
	case rest == "prune":
		r.handlePrune(w, req)
	case rest == "prunes":
		if req.Method != http.MethodGet {
			writeCodedError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		if !realAdmin(req) {
			writeCodedError(w, http.StatusForbidden, "real_admin_only", "only an Admin acting as itself can see the prune log")
			return
		}
		entries, err := conversations.AuditLog(req.Context(), d, true, 100)
		if err != nil {
			convInternal(w, "prune log", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": entries})
	default:
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 || len(parts) > 2 {
			writeCodedError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		switch {
		case len(parts) == 1 && req.Method == http.MethodGet:
			r.getConversation(w, req, id)
		case len(parts) == 2 && (parts[1] == "archive" || parts[1] == "unarchive"):
			r.archiveConversation(w, req, id, parts[1])
		case len(parts) == 1 || len(parts) == 2:
			if len(parts) == 2 {
				writeCodedError(w, http.StatusNotFound, "not_found", "not found")
				return
			}
			writeCodedError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		}
	}
}

func convInternal(w http.ResponseWriter, what string, err error) {
	log.Printf("conversations: %s: %v", what, err)
	writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
}

func (r *Router) listConversations(w http.ResponseWriter, req *http.Request) {
	v := req.URL.Query()
	f := conversations.Filter{}
	f.Page, f.Size = parsePagination(v.Get("page_size"), v.Get("page"), 25)
	switch v.Get("archived") {
	case "", "0":
	case "1":
		f.Archived = true
	default:
		writeCodedError(w, http.StatusBadRequest, "bad_request", "archived must be 0 or 1")
		return
	}
	if k := v.Get("kind"); k != "" {
		if !oneOf(k, "job", "sync", "call") {
			writeCodedError(w, http.StatusBadRequest, "bad_request", "kind must be job, sync or call")
			return
		}
		f.Kind = k
	}
	if p := strings.TrimSpace(v.Get("product")); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			writeCodedError(w, http.StatusBadRequest, "bad_request", "product must be a product id")
			return
		}
		f.ProductID = n
	}
	if q := strings.TrimSpace(v.Get("q")); q != "" {
		if !convQRe.MatchString(q) {
			writeCodedError(w, http.StatusBadRequest, "bad_request", "q must be a job id, sync id or case code")
			return
		}
		f.Q = q
	}
	d := r.getDB()
	items, total, err := conversations.List(req.Context(), d, f)
	if err != nil {
		convInternal(w, "list", err)
		return
	}
	archived, all, err := conversations.Sizes(req.Context(), d)
	if err != nil {
		convInternal(w, "sizes", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": f.Page, "page_size": f.Size,
		"totals": map[string]any{"archived": archived, "all": all}})
}

func sinceSeq(req *http.Request) int {
	n, err := strconv.Atoi(req.URL.Query().Get("since_seq"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (r *Router) getConversation(w http.ResponseWriter, req *http.Request, id int64) {
	if _, canVerify := productRole(req); !canVerify {
		writeCodedError(w, http.StatusForbidden, "forbidden", "your role cannot read conversations")
		return
	}
	d := r.getDB()
	it, err := conversations.Get(req.Context(), d, id)
	if err != nil {
		convInternal(w, "get", err)
		return
	}
	if it == nil {
		writeCodedError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	msgs, err := conversations.Messages(req.Context(), d, id, sinceSeq(req))
	if err != nil {
		convInternal(w, "messages", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": it, "messages": msgs})
}

// handleJobConversation serves GET /api/jobs/{id}/conversation: one shape, {conversation, messages[, restricted]}.
func (r *Router) handleJobConversation(w http.ResponseWriter, req *http.Request, jobID string) {
	if req.Method != http.MethodGet {
		writeCodedError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	d := r.getDB()
	if d == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unavailable", "database unavailable")
		return
	}
	it, err := conversations.GetByRef(req.Context(), d, "job", jobID)
	if err != nil {
		convInternal(w, "job conversation", err)
		return
	}
	out := map[string]any{"conversation": it, "messages": []conversations.Message{}}
	if it == nil || it.ArchivedAt != nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if _, canVerify := productRole(req); !canVerify {
		out["restricted"] = true
		writeJSON(w, http.StatusOK, out)
		return
	}
	msgs, err := conversations.Messages(req.Context(), d, it.ID, sinceSeq(req))
	if err != nil {
		convInternal(w, "job conversation messages", err)
		return
	}
	out["messages"] = msgs
	writeJSON(w, http.StatusOK, out)
}

func (r *Router) archiveConversation(w http.ResponseWriter, req *http.Request, id int64, action string) {
	if req.Method != http.MethodPost {
		writeCodedError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	if !isJSON(req) {
		writeCodedError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send Content-Type: application/json")
		return
	}
	if _, canVerify := productRole(req); !canVerify {
		writeCodedError(w, http.StatusForbidden, "forbidden", "your role cannot archive conversations")
		return
	}
	ctx := req.Context()
	d := r.getDB()
	actorID, actorName, asUser := convActor(req)
	var err error
	if action == "archive" {
		err = conversations.Archive(ctx, d, id, actorID)
	} else {
		err = conversations.Unarchive(ctx, d, id)
	}
	switch {
	case errors.Is(err, conversations.ErrNotFound):
		writeCodedError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	case errors.Is(err, conversations.ErrAlreadyArchived):
		writeCodedError(w, http.StatusConflict, "already_archived", "the conversation is already archived")
		return
	case errors.Is(err, conversations.ErrNotArchived):
		writeCodedError(w, http.StatusConflict, "not_archived", "the conversation is not archived")
		return
	case errors.Is(err, conversations.ErrActive):
		writeCodedError(w, http.StatusConflict, "active", "the job or sync is still working; archive it when it has finished")
		return
	case err != nil:
		convInternal(w, action, err)
		return
	}
	if err := conversations.Audit(ctx, d, action, actorID, actorName, asUser, id, map[string]any{}); err != nil {
		log.Printf("conversations: audit %s %d: %v", action, id, err)
	}
	it, err := conversations.Get(ctx, d, id)
	if err != nil {
		convInternal(w, "get", err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

type pruneBody struct {
	Confirm        *string `json:"confirm"`
	ArchivedBefore *string `json:"archived_before"`
}

func (r *Router) handlePrune(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodPost {
		writeCodedError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	if !realAdmin(req) {
		writeCodedError(w, http.StatusForbidden, "real_admin_only", "only an Admin acting as itself can prune conversations")
		return
	}
	ctx := req.Context()
	d := r.getDB()
	if req.Method == http.MethodGet {
		t, before, err := conversations.Preview(ctx, d)
		if err != nil {
			convInternal(w, "prune preview", err)
			return
		}
		var b any
		if before != nil {
			b = before.UTC().Format(PruneTimeLayout)
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversations": t.Conversations, "messages": t.Messages, "bytes": t.Bytes, "archived_before": b})
		return
	}
	if !isJSON(req) {
		writeCodedError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send Content-Type: application/json")
		return
	}
	var body pruneBody
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || body.Confirm == nil || *body.Confirm != "prune" || body.ArchivedBefore == nil {
		writeCodedError(w, http.StatusBadRequest, "confirm", `send {"confirm":"prune","archived_before":"<from the preview>"}`)
		return
	}
	cutoff, err := time.Parse(PruneTimeLayout, *body.ArchivedBefore)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, "confirm", "archived_before must be the preview's value (UTC with milliseconds)")
		return
	}
	cutoff = cutoff.UTC()
	got, ids, perr := conversations.Prune(ctx, d, cutoff)
	actorID, actorName, asUser := convActor(req)
	if got.Conversations > 0 || perr != nil {
		detail := map[string]any{"archived_before": cutoff.Format(PruneTimeLayout), "conversations": got.Conversations, "messages": got.Messages, "bytes": got.Bytes, "ids": ids}
		if perr != nil {
			detail["error"] = "stopped after a failing batch"
		}
		if err := conversations.Audit(ctx, d, "prune", actorID, actorName, asUser, 0, detail); err != nil {
			log.Printf("conversations: prune audit: %v", err)
		}
		log.Printf("conversations: %s pruned %d archived conversation(s), %d message(s), %d bytes", actorName, got.Conversations, got.Messages, got.Bytes)
	}
	if perr != nil {
		log.Printf("conversations: prune: %v", perr)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "the prune stopped after a failing batch", "code": "internal",
			"conversations": got.Conversations, "messages": got.Messages, "bytes": got.Bytes})
		return
	}
	if got.Conversations == 0 {
		writeCodedError(w, http.StatusConflict, "nothing", "no archived conversation matches")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": got.Conversations, "messages": got.Messages, "bytes": got.Bytes})
}
