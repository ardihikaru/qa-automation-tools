package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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

// ValidateSession looks up the token and returns the identity the request
// acts as. Expired or unknown tokens, and inactive real users, return
// ErrSessionInvalid.
//
// When the session has an active, unexpired impersonation whose real user is
// still an impersonator role and whose target exists and is active, the
// returned User is the target with Impersonator set. In every other case the
// real user is returned with Impersonator nil — never a bare target.
func ValidateSession(ctx context.Context, db *sql.DB, token string) (*User, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}

	const q = `
		SELECT u.id, u.username, COALESCE(u.full_name, ''), COALESCE(u.email, ''),
		       COALESCE(u.whatsapp, ''), COALESCE(u.odoo_id, ''), u.role,
		       u.is_lead, u.active, u.must_change_pw, u.language, s.expires_at AS s_expires_at,
		       ai.expires_at AS ai_expires_at,
		       t.id, t.username, t.full_name, t.email, t.whatsapp, t.odoo_id,
		       t.role, t.is_lead, t.active, t.language
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN active_impersonations ai
		       ON ai.parent_token = s.token AND ai.expires_at > ?
		LEFT JOIN users t ON t.id = ai.target_user_id
		WHERE s.token = ? AND s.expires_at > UTC_TIMESTAMP()
		LIMIT 1
	`
	var (
		u                                  User
		aiExpires                          sql.NullTime
		tID, tUser, tName, tMail, tWA, tOd sql.NullString
		tRole                              sql.NullString
		tLead, tActive                     sql.NullBool
		tLang                              sql.NullString
	)
	// Connections are pinned to UTC (db.NormalizeDSN), so Go timestamps and
	// UTC_TIMESTAMP() agree; ai.expires_at is compared with Go's now as well.
	err := db.QueryRowContext(ctx, q, time.Now().UTC(), token).Scan(
		&u.ID, &u.Username, &u.FullName, &u.Email,
		&u.WhatsApp, &u.OdooID, &u.Role,
		&u.IsLead, &u.Active, &u.MustChangePW, &u.Language, &u.SessionExpires,
		&aiExpires,
		&tID, &tUser, &tName, &tMail, &tWA, &tOd,
		&tRole, &tLead, &tActive, &tLang,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		log.Printf("⚠️  auth: session query failed: %v", err)
		return nil, fmt.Errorf("query session: %w", err)
	}
	if !u.Active {
		return nil, ErrSessionInvalid
	}
	u.SessionToken = token

	impersonating := aiExpires.Valid && tID.Valid &&
		tActive.Valid && tActive.Bool && ImpersonatorRoles[u.Role]
	if !impersonating {
		return &u, nil
	}

	return &User{
		ID:           tID.String,
		Username:     tUser.String,
		FullName:     tName.String,
		Email:        tMail.String,
		WhatsApp:     tWA.String,
		OdooID:       tOd.String,
		Role:         tRole.String,
		IsLead:       tLead.Valid && tLead.Bool,
		Active:       true,
		MustChangePW: false,
		Language:     normLang(tLang.String),
		Impersonator: &Actor{
			ID:       u.ID,
			Username: u.Username,
			FullName: u.FullName,
			Role:     u.Role,
		},
		SessionToken:   token,
		SessionExpires: u.SessionExpires,
	}, nil
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

// normLang maps anything but "id" to "en".
func normLang(l string) string {
	if l == "id" {
		return "id"
	}
	return "en"
}
