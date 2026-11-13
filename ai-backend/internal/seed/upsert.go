package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type userRow struct {
	ID       string // optional — generated when blank
	Username string
	Role     string
	IsLead   bool
	WhatsApp string
	OdooID   string
	Password string // plaintext; bcrypted before INSERT
	// MustChangePW is the must_change_pw value for a NEW row only.
	MustChangePW bool
}

// upsertUser inserts a new user when the username is unknown. Returns
// (inserted, error) — never overwrites an existing row's password.
// When the row already exists, optional metadata (whatsapp/odoo_id/is_lead)
// is updated so a re-run can backfill changes from taskmgr without resetting
// the password.
func upsertUser(ctx context.Context, db *sql.DB, u userRow) (bool, error) {
	if u.Username == "" {
		return false, errors.New("username required")
	}
	if u.Role == "" {
		u.Role = "Developer"
	}

	// Does this username already exist?
	var existingID string
	err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ? LIMIT 1`, u.Username).Scan(&existingID)
	if err == nil {
		// Update metadata in place but never touch password_hash / must_change_pw.
		// Service accounts are owned by upsertServiceUser: no other seed path
		// may change them (e.g. a taskmgr user with the same username).
		_, uerr := db.ExecContext(ctx,
			`UPDATE users SET role = ?, is_lead = ?, whatsapp = ?, odoo_id = ? WHERE id = ? AND role <> 'Service'`,
			u.Role, boolToTinyint(u.IsLead), u.WhatsApp, u.OdooID, existingID,
		)
		if uerr != nil {
			return false, fmt.Errorf("update metadata: %w", uerr)
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("lookup existing: %w", err)
	}

	// Brand new user.
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	hash, err := hashForSeed(u.Password)
	if err != nil {
		return false, fmt.Errorf("hash password: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO users
			(id, username, role, is_lead, whatsapp, odoo_id,
			 password_hash, must_change_pw, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		u.ID, u.Username, u.Role, boolToTinyint(u.IsLead), u.WhatsApp, u.OdooID, hash,
		boolToTinyint(u.MustChangePW),
	)
	if err != nil {
		return false, fmt.Errorf("insert: %w", err)
	}
	return true, nil
}

func boolToTinyint(b bool) int {
	if b {
		return 1
	}
	return 0
}
