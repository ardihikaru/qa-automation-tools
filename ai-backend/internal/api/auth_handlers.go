package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
)

// AuthHandlers groups the four endpoints under /api/auth/*.
type AuthHandlers struct {
	DB         *sql.DB
	CookieName string
	SessionTTL time.Duration
	Secure     bool // SameSite + Secure cookie hardening (set true in prod)
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePwReq struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// Login validates credentials, creates a session row, sets the cookie.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}

	var body loginReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorPayload("invalid request body"))
		return
	}
	username := strings.ToLower(strings.TrimSpace(body.Username))
	if username == "" || body.Password == "" {
		writeJSON(w, http.StatusUnauthorized, errorPayload("Invalid username or password"))
		return
	}

	u, hash, err := lookupForLogin(r, h.DB, username)
	if err != nil || u == nil {
		writeJSON(w, http.StatusUnauthorized, errorPayload("Invalid username or password"))
		return
	}
	if err := auth.Check(hash, body.Password); err != nil {
		writeJSON(w, http.StatusUnauthorized, errorPayload("Invalid username or password"))
		return
	}

	token, expires, err := auth.CreateSession(r.Context(), h.DB, u.ID, h.SessionTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not create session"))
		return
	}
	setSessionCookie(w, h.CookieName, token, expires, h.Secure)

	writeJSON(w, http.StatusOK, map[string]any{
		"user":               u,
		"mustChangePassword": u.MustChangePW,
	})
}

// Logout deletes the session row and clears the cookie.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}
	if c, err := r.Cookie(h.CookieName); err == nil {
		// Best-effort: end any impersonation first so the audit row records
		// "logout"; the session itself is revoked regardless.
		if u, ok := auth.GetUser(r.Context()); ok && u.Impersonator != nil {
			if err := auth.EndImpersonation(r.Context(), h.DB, c.Value, "logout"); err != nil {
				log.Printf("⚠️  impersonate end on logout failed: %v", err)
			} else {
				log.Printf("impersonate stop actor=%s target=%s reason=logout", u.Impersonator.Username, u.Username)
			}
		}
		_ = auth.RevokeSession(r.Context(), h.DB, c.Value)
	}
	setSessionCookie(w, h.CookieName, "", time.Unix(0, 0), h.Secure)
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the user attached to the current session.
func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}
	u, ok := auth.GetUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorPayload("unauthorized"))
		return
	}
	writeJSON(w, http.StatusOK, identityPayload(u))
}

// ChangePassword updates the bcrypt hash and clears must_change_pw.
func (h *AuthHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}
	u, ok := auth.GetUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorPayload("unauthorized"))
		return
	}
	if !auth.RequireRealUser(u) {
		writeJSON(w, http.StatusForbidden, errorPayload("not allowed while impersonating"))
		return
	}
	var body changePwReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorPayload("invalid request body"))
		return
	}
	if msg := validateNewPassword(u.Username, body.CurrentPassword, body.NewPassword); msg != "" {
		writeJSON(w, http.StatusBadRequest, errorPayload(msg))
		return
	}

	var currentHash string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT password_hash FROM users WHERE id = ?`, u.ID,
	).Scan(&currentHash)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		writeJSON(w, http.StatusUnauthorized, errorPayload("unauthorized"))
		return
	}
	if err := auth.Check(currentHash, body.CurrentPassword); err != nil {
		writeJSON(w, http.StatusUnauthorized, errorPayload("Current password is incorrect"))
		return
	}

	newHash, err := auth.Hash(body.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not hash password"))
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET password_hash = ?, must_change_pw = 0 WHERE id = ?`,
		newHash, u.ID,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not save password"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetLanguage is PUT /api/auth/language {"language":"en"|"id"}: sets the caller's own display
// language and answers {"language": "..."}. Refused while impersonating (it would change the target's account).
func (h *AuthHandlers) SetLanguage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method not allowed"))
		return
	}
	u, ok := auth.GetUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorPayload("unauthorized"))
		return
	}
	if !auth.RequireRealUser(u) {
		writeJSON(w, http.StatusForbidden, errorPayload("not allowed while impersonating"))
		return
	}
	if !isJSON(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, errorPayload("content type must be application/json"))
		return
	}
	var body struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorPayload("invalid request body"))
		return
	}
	if body.Language != "en" && body.Language != "id" {
		writeJSON(w, http.StatusBadRequest, errorPayload("language must be en or id"))
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE users SET language = ? WHERE id = ?`, body.Language, u.ID); err != nil {
		log.Printf("auth: set language: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorPayload("could not save language"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"language": body.Language})
}
