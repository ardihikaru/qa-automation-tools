package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ErrSessionInvalid is returned when a cookie token is missing, expired, or
// belongs to a deactivated user. Callers map this to HTTP 401.
var ErrSessionInvalid = errors.New("session invalid")

// CreateSession issues a fresh 64-char hex token and stores it.
func CreateSession(ctx context.Context, db *sql.DB, userID string, ttl time.Duration) (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, fmt.Errorf("read random: %w", err)
	}
	token := hex.EncodeToString(buf)
	expires := time.Now().UTC().Add(ttl)

	_, err := db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expires,
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("insert session: %w", err)
	}
	return token, expires, nil
}

// ValidateSession looks up the token and returns the owning user.
// Expired or unknown tokens return ErrSessionInvalid.
func ValidateSession(ctx context.Context, db *sql.DB, token string) (*User, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}

	const q = `
		SELECT u.id, u.username, COALESCE(u.full_name, ''), COALESCE(u.email, ''),
		       COALESCE(u.whatsapp, ''), COALESCE(u.odoo_id, ''), u.role,
		       u.is_lead, u.active, u.must_change_pw
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > UTC_TIMESTAMP()
		LIMIT 1
	`
	var u User
	err := db.QueryRowContext(ctx, q, token).Scan(
		&u.ID, &u.Username, &u.FullName, &u.Email,
		&u.WhatsApp, &u.OdooID, &u.Role,
		&u.IsLead, &u.Active, &u.MustChangePW,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	if !u.Active {
		return nil, ErrSessionInvalid
	}
	return &u, nil
}

// RevokeSession deletes a single session row. Safe to call with an unknown
// token (returns nil) — used by /api/auth/logout.
func RevokeSession(ctx context.Context, db *sql.DB, token string) error {
	if token == "" {
		return nil
	}
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// PurgeExpiredSessions removes stale rows. Called best-effort at startup.
func PurgeExpiredSessions(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= UTC_TIMESTAMP()`)
	return err
}
