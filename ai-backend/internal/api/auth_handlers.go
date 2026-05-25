package api

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	writeJSON(w, http.StatusOK, map[string]any{
		"user":               u,
		"mustChangePassword": u.MustChangePW,
	})
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
