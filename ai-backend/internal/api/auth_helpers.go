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

// lookupForLogin returns the user + password hash for an enabled account.
// Username is matched case-insensitively via utf8mb4_general_ci.
func lookupForLogin(r *http.Request, db *sql.DB, username string) (*auth.User, string, error) {
	const q = `
		SELECT id, username, COALESCE(full_name, ''), COALESCE(email, ''),
		       COALESCE(whatsapp, ''), COALESCE(odoo_id, ''), role,
		       is_lead, active, must_change_pw, password_hash
		FROM users
		WHERE username = ? AND active = 1
		LIMIT 1
	`
	var u auth.User
	var hash string
	err := db.QueryRowContext(r.Context(), q, username).Scan(
		&u.ID, &u.Username, &u.FullName, &u.Email,
		&u.WhatsApp, &u.OdooID, &u.Role,
		&u.IsLead, &u.Active, &u.MustChangePW, &hash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return &u, hash, nil
}

func setSessionCookie(w http.ResponseWriter, name, value string, expires time.Time, secure bool) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	if value == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorPayload(msg string) map[string]string {
	return map[string]string{"error": msg}
}

// validateNewPassword enforces the same rules the frontend hints at:
// ≥ 8 chars, differs from username, differs from current.
// Returns an empty string when ok, otherwise the message to show the user.
func validateNewPassword(username, current, next string) string {
	next = strings.TrimSpace(next)
	switch {
	case len(next) < 8:
		return "New password must be at least 8 characters"
	case strings.EqualFold(next, username):
		return "New password cannot be your username"
	case next == current:
		return "New password must differ from the current one"
	}
	return ""
}
