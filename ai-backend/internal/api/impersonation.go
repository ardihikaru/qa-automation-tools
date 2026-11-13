package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
)

// Impersonation endpoints (docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md):
//   GET  /api/users                  — user list for the "Log in as…" picker
//   POST /api/auth/impersonate       — act as another user
//   POST /api/auth/impersonate/stop  — return to the real user

type impersonateReq struct {
	Username string `json:"username"`
}

type userListItem struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	FullName string `json:"fullName,omitempty"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
}

// ListUsers returns every user (never the password hash) to impersonators.
func (h *AuthHandlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}
	u, _ := auth.GetUser(r.Context())
	if !auth.CanImpersonate(u) {
		writeJSON(w, http.StatusForbidden, errorPayload("forbidden"))
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, username, COALESCE(full_name, ''), role, active
		FROM users ORDER BY username`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not list users"))
		return
	}
	defer rows.Close()
	out := []userListItem{}
	for rows.Next() {
		var it userListItem
		if err := rows.Scan(&it.ID, &it.Username, &it.FullName, &it.Role, &it.Active); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorPayload("could not list users"))
			return
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not list users"))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Impersonate makes the current session act as another user. The cookie is
// not changed: the impersonation is a row keyed by the real session token.
func (h *AuthHandlers) Impersonate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}
	if !isJSON(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, errorPayload("content type must be application/json"))
		return
	}
	u, _ := auth.GetUser(r.Context())
	if !auth.CanImpersonate(u) {
		writeJSON(w, http.StatusForbidden, errorPayload("forbidden"))
		return
	}
	var body impersonateReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorPayload("invalid request body"))
		return
	}
	username := strings.ToLower(strings.TrimSpace(body.Username))
	if username == "" {
		writeJSON(w, http.StatusBadRequest, errorPayload("invalid request body"))
		return
	}

	var targetID, targetUsername string
	var active bool
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, username, active FROM users WHERE username = ? LIMIT 1`, username,
	).Scan(&targetID, &targetUsername, &active)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, errorPayload("user not found"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not start impersonation"))
		return
	}
	if targetID == u.ID {
		writeJSON(w, http.StatusBadRequest, errorPayload("cannot impersonate yourself"))
		return
	}
	if !active {
		writeJSON(w, http.StatusConflict, errorPayload("user is inactive"))
		return
	}

	auditID, _, err := auth.StartImpersonation(r.Context(), h.DB, u.SessionToken, u.SessionExpires,
		u.ID, targetID, clientIP(r), r.UserAgent())
	if err != nil {
		log.Printf("⚠️  impersonate start failed actor=%s target=%s: %v", u.Username, targetUsername, err)
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not start impersonation"))
		return
	}
	log.Printf("impersonate start actor=%s target=%s audit=%d", u.Username, targetUsername, auditID)
	h.writeIdentity(w, r, u.SessionToken)
}

// StopImpersonation returns the session to the real user.
func (h *AuthHandlers) StopImpersonation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}
	if !isJSON(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, errorPayload("content type must be application/json"))
		return
	}
	u, _ := auth.GetUser(r.Context())
	if u == nil || u.Impersonator == nil {
		writeJSON(w, http.StatusBadRequest, errorPayload("not impersonating"))
		return
	}
	if err := auth.EndImpersonation(r.Context(), h.DB, u.SessionToken, "stop"); err != nil {
		log.Printf("⚠️  impersonate stop failed actor=%s target=%s: %v", u.Impersonator.Username, u.Username, err)
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not stop impersonation"))
		return
	}
	log.Printf("impersonate stop actor=%s target=%s reason=stop", u.Impersonator.Username, u.Username)
	h.writeIdentity(w, r, u.SessionToken)
}

// writeIdentity re-reads the session (same code path as /me) and writes the
// shared {user, mustChangePassword} shape, or 401 when it no longer validates.
func (h *AuthHandlers) writeIdentity(w http.ResponseWriter, r *http.Request, token string) {
	fresh, err := auth.ValidateSession(r.Context(), h.DB, token)
	if err != nil || fresh == nil {
		setSessionCookie(w, h.CookieName, "", time.Unix(0, 0), h.Secure)
		writeJSON(w, http.StatusUnauthorized, errorPayload("unauthorized"))
		return
	}
	writeJSON(w, http.StatusOK, identityPayload(fresh))
}

func identityPayload(u *auth.User) map[string]any {
	return map[string]any{
		"user":               u,
		"mustChangePassword": u.MustChangePW,
	}
}

// isJSON enforces a JSON content type so cross-origin callers need a CORS
// preflight (which applyCORS only grants to DASHBOARD_ORIGIN).
func isJSON(r *http.Request) bool {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	return strings.HasPrefix(ct, "application/json")
}

// clientIP prefers the edge's X-Real-IP, falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
