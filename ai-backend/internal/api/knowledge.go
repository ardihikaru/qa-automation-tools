package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
)

// SetKnowledge wires the knowledge syncer (PRODUCT_KNOWLEDGE_SYNC.md).
func (r *Router) SetKnowledge(s *knowledge.Syncer) { r.kb = s }

// entityPaths maps the URL segment of an entity collection to its type.
var entityPaths = map[string]string{
	"parts":      knowledge.TypePart,
	"modules":    knowledge.TypeModule,
	"submodules": knowledge.TypeSubmodule,
	"features":   knowledge.TypeFeature,
	"epics":      knowledge.TypeEpic,
	"stories":    knowledge.TypeStory,
	"test-cases": knowledge.TypeTestCase,
	"assistants": knowledge.TypeAssistant,
}

func userID(req *http.Request) string {
	if u, ok := auth.GetUser(req.Context()); ok {
		return u.ID
	}
	return ""
}

// userDisplayName is the name stored with a verification: full name, else username.
func userDisplayName(req *http.Request) string {
	if u, ok := auth.GetUser(req.Context()); ok {
		if u.FullName != "" {
			return u.FullName
		}
		return u.Username
	}
	return ""
}

// writeKnowledgeError maps knowledge errors to HTTP statuses.
func writeKnowledgeError(w http.ResponseWriter, err error) {
	var in *knowledge.InputError
	var cf *knowledge.ConflictError
	var running knowledge.ErrRunning
	switch {
	case errors.Is(err, knowledge.ErrNotFound), errors.Is(err, knowledge.ErrProductNotFound):
		writeJSONError(w, http.StatusNotFound, "not found")
	case errors.As(err, &in):
		writeJSONError(w, http.StatusBadRequest, in.Msg)
	case errors.As(err, &cf):
		writeJSONError(w, http.StatusConflict, cf.Msg)
	case errors.As(err, &running):
		writeJSON(w, http.StatusConflict, map[string]any{"error": running.Error(), "sync_id": running.SyncID})
	case errors.Is(err, knowledge.ErrNotConfigured), errors.Is(err, knowledge.ErrUnknownFormat):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, knowledge.ErrNoAutomation):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, knowledge.ErrNoSucceededSync):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, knowledge.ErrExportTooLarge):
		writeJSONError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, knowledge.ErrUnavailable):
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
	default:
		log.Printf("knowledge: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	}
}

func boolParam(req *http.Request, name string) bool {
	v := req.URL.Query().Get(name)
	return v == "1" || v == "true"
}

func intParam(req *http.Request, name string) int {
	n, _ := strconv.Atoi(req.URL.Query().Get(name))
	return n
}

// handleKnowledge serves /api/products/{id}/knowledge/... (rest is the path after "knowledge").
func (r *Router) handleKnowledge(w http.ResponseWriter, req *http.Request, pid int, rest []string, canWrite bool) {
	s := r.kb
	if s == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "knowledge base not available")
		return
	}
	ctx := req.Context()
	method := req.Method
	deny := func() bool {
		if !canWrite {
			writeJSONError(w, http.StatusForbidden, "your role cannot change the knowledge base")
			return true
		}
		return false
	}
	bad := func() { writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed") }
	if len(rest) == 0 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	switch seg := rest[0]; {
	case seg == "sync" && len(rest) == 1:
		if method != http.MethodPost {
			bad()
			return
		}
		if deny() {
			return
		}
		id, err := s.Start(ctx, pid, knowledge.TriggerManual, userID(req))
		if err != nil {
			writeKnowledgeError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"sync_id": id})

	case seg == "summary" && len(rest) == 1:
		if method != http.MethodGet {
			bad()
			return
		}
		out, err := s.Summary(ctx, pid)
		respond(w, out, err)

	case seg == "coverage" && len(rest) == 1:
		if method != http.MethodGet {
			bad()
			return
		}
		q := req.URL.Query()
		m, err := s.CoverageBy(ctx, pid, q.Get("by"), knowledge.Scope{Part: q.Get("part"), Module: q.Get("module"), Submodule: q.Get("submodule"), Epic: q.Get("epic")})
		respond(w, map[string]any{"by": q.Get("by"), "coverage": m}, err)

	case seg == "exports" && len(rest) == 2 && rest[1] == "modules-checklist":
		// MODULE_CHECKLIST_EXPORT.md. Any authenticated reader; no deny() (the cache write is not a user-data write).
		r.handleChecklistExport(w, req, pid)

	case seg == "tree" && len(rest) == 1:
		if method != http.MethodGet {
			bad()
			return
		}
		out, err := s.Tree(ctx, pid, boolParam(req, "include_removed"))
		respond(w, out, err)

	case seg == "syncs" && len(rest) <= 2:
		if method != http.MethodGet {
			bad()
			return
		}
		if len(rest) == 1 {
			list, err := s.Syncs(ctx, pid, intParam(req, "limit"))
			respond(w, map[string]any{"syncs": list}, err)
			return
		}
		sid, err := strconv.ParseInt(rest[1], 10, 64)
		if err != nil || sid <= 0 {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		out, err := s.Sync(ctx, pid, sid)
		respond(w, out, err)

	case seg == "annotations" && len(rest) == 3:
		et, code := rest[1], rest[2]
		switch method {
		case http.MethodPut:
			if deny() {
				return
			}
			var body knowledge.AnnotationBody
			if !decodeStrictJSON(w, req, 1<<16, &body) {
				return
			}
			out, err := s.PutAnnotation(ctx, pid, et, code, userID(req), body)
			respond(w, out, err)
		case http.MethodDelete:
			if deny() {
				return
			}
			if err := s.DeleteAnnotation(ctx, pid, et, code); err != nil {
				writeKnowledgeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			bad()
		}

	case seg == "epics" && len(rest) == 3 && (rest[2] == "automation" || rest[2] == "automate"):
		// EPIC_AUTOMATION.md. GET is open to any reader; POST needs canWrite (= canVerify).
		r.handleEpicAutomation(w, req, pid, rest[1], rest[2], canWrite)
	case seg == "test-cases" && len(rest) == 2 && rest[1] == "verifiers":
		if method != http.MethodGet {
			bad()
			return
		}
		out, err := s.Verifiers(ctx, pid)
		respond(w, map[string]any{"verifiers": out}, err)

	case seg == "test-cases" && len(rest) == 3 && rest[2] == "verification":
		if method != http.MethodPut {
			bad()
			return
		}
		if deny() {
			return
		}
		var body knowledge.VerificationBody
		if !decodeStrictJSON(w, req, 1<<12, &body) {
			return
		}
		out, err := s.PutVerification(ctx, pid, rest[1], userID(req), userDisplayName(req), body)
		respond(w, out, err)

	case seg == "test-cases" && len(rest) == 4 && rest[2] == "verification" && rest[3] == "history":
		if method != http.MethodGet {
			bad()
			return
		}
		out, err := s.VerificationHistory(ctx, pid, rest[1], intParam(req, "limit"))
		respond(w, out, err)

	case seg == "test-cases" && len(rest) == 3 && (rest[2] == "run" || rest[2] == "create-automation" || rest[2] == "jobs"):
		r.handleCaseAction(w, req, pid, rest[1], rest[2], canWrite)

	case entityPaths[seg] != "" && len(rest) <= 2:
		et := entityPaths[seg]
		draftable := et == knowledge.TypeStory || et == knowledge.TypeTestCase
		if len(rest) == 1 {
			switch method {
			case http.MethodGet:
				r.listKnowledge(w, req, pid, et)
			case http.MethodPost:
				if !draftable {
					bad()
					return
				}
				if deny() {
					return
				}
				body := draftBody(et)
				if !decodeStrictJSON(w, req, 1<<17, body) {
					return
				}
				out, err := s.SaveDraft(ctx, pid, et, "", userID(req), body)
				if err != nil {
					writeKnowledgeError(w, err)
					return
				}
				writeJSON(w, http.StatusCreated, out)
			default:
				bad()
			}
			return
		}
		code := rest[1]
		switch method {
		case http.MethodGet:
			out, err := s.Entity(ctx, pid, et, code)
			if err == nil && et == knowledge.TypeTestCase && out != nil {
				// GAOS_CASE_PROPOSALS.md: the pull requests GAOS linked to this case.
				out["pr_links"] = r.casePRLinks(ctx, pid, code)
			}
			respond(w, out, err)
		case http.MethodPut:
			if !draftable {
				bad()
				return
			}
			if deny() {
				return
			}
			body := draftBody(et)
			if !decodeStrictJSON(w, req, 1<<17, body) {
				return
			}
			out, err := s.SaveDraft(ctx, pid, et, code, userID(req), body)
			respond(w, out, err)
		case http.MethodDelete:
			if !draftable {
				bad()
				return
			}
			if deny() {
				return
			}
			if err := s.DeleteDraft(ctx, pid, et, code); err != nil {
				writeKnowledgeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			bad()
		}

	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

// draftBody returns the strict body type of a draft. The update body has no code field.
func draftBody(et string) any {
	if et == knowledge.TypeStory {
		return &knowledge.StoryBody{}
	}
	return &knowledge.TestCaseBody{}
}

func respond(w http.ResponseWriter, out any, err error) {
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (r *Router) listKnowledge(w http.ResponseWriter, req *http.Request, pid int, et string) {
	q := req.URL.Query()
	f := knowledge.Filter{
		Epic: q.Get("epic"), Story: q.Get("story"), Feature: q.Get("feature"), Status: q.Get("status"),
		Level: q.Get("level"), Priority: q.Get("priority"), Origin: q.Get("origin"), Tag: q.Get("tag"),
		Q: q.Get("q"), Automated: q.Get("automated"), IncludeRemoved: boolParam(req, "include_removed"),
		Page: intParam(req, "page"), PageSize: intParam(req, "page_size"),
		Part: q.Get("part"), Module: q.Get("module"), Submodule: q.Get("submodule"),
		Release: q.Get("release"), Coverage: q.Get("coverage"), Sort: q.Get("sort"),
		Verified: q.Get("verified"), VerifiedBy: q.Get("verified_by"),
	}
	ctx := req.Context()
	switch et {
	case knowledge.TypeStory:
		out, err := r.kb.Stories(ctx, pid, f)
		respond(w, out, err)
	case knowledge.TypeTestCase:
		out, err := r.kb.TestCases(ctx, pid, f)
		respond(w, out, err)
	case knowledge.TypeEpic:
		out, err := r.kb.Epics(ctx, pid, f.IncludeRemoved)
		respond(w, map[string]any{"epics": out}, err)
	case knowledge.TypeFeature:
		out, err := r.kb.Features(ctx, pid, f)
		respond(w, out, err)
	case knowledge.TypeAssistant:
		out, err := r.kb.Assistants(ctx, knowledge.AssistantFilter{ProductID: pid, Kind: q.Get("kind"), Q: f.Q,
			IncludeRemoved: f.IncludeRemoved, Page: f.Page, PageSize: f.PageSize})
		respond(w, out, err)
	case knowledge.TypeModule:
		out, err := r.kb.Modules(ctx, pid, f.IncludeRemoved)
		respond(w, map[string]any{"modules": out}, err)
	default:
		// Parts and sub-modules are browsed through the tree.
		writeJSONError(w, http.StatusNotFound, "use knowledge/tree")
	}
}

// handleAssistants serves GET /api/assistants: the synced agents, skills, rules, commands and
// instructions of every product, filtered by product_id, kind (comma-separated) and q.
func (r *Router) handleAssistants(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.kb == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "knowledge base not available")
		return
	}
	q := req.URL.Query()
	out, err := r.kb.Assistants(req.Context(), knowledge.AssistantFilter{
		ProductID: intParam(req, "product_id"), Kind: q.Get("kind"), Q: q.Get("q"),
		Page: intParam(req, "page"), PageSize: intParam(req, "page_size"),
	})
	respond(w, out, err)
}
