package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MaxImpersonation caps how long one impersonation lasts. It never outlives
// the real session either (see ImpersonationExpiry).
const MaxImpersonation = time.Hour

// ImpersonationExpiry returns min(parentExpires, now+MaxImpersonation) in UTC.
func ImpersonationExpiry(now, parentExpires time.Time) time.Time {
	exp := now.UTC().Add(MaxImpersonation)
	if p := parentExpires.UTC(); p.Before(exp) {
		return p
	}
	return exp
}

// StartImpersonation makes the session parentToken act as targetID. Any
// existing row for the same session is replaced: if it was still live its
// audit row is closed as "superseded"; if it had expired its audit row is left
// as-is (it reads as expired). Runs in one transaction.
func StartImpersonation(ctx context.Context, db *sql.DB, parentToken string, parentExpires time.Time,
	impersonatorID, targetID, ip, ua string) (int64, time.Time, error) {

	now := time.Now().UTC()
	exp := ImpersonationExpiry(now, parentExpires)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := endLocked(ctx, tx, parentToken, "superseded", now); err != nil {
		return 0, time.Time{}, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO impersonation_audit
			(impersonator_id, target_user_id, started_at, expires_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)`,
		impersonatorID, targetID, now, exp, truncate(ip, 64), truncate(ua, 255),
	)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("insert audit: %w", err)
	}
	auditID, err := res.LastInsertId()
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("audit id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO active_impersonations
			(parent_token, impersonator_id, target_user_id, audit_id, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		parentToken, impersonatorID, targetID, auditID, exp,
	); err != nil {
		return 0, time.Time{}, fmt.Errorf("insert active: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, time.Time{}, fmt.Errorf("commit: %w", err)
	}
	return auditID, exp, nil
}

// EndImpersonation removes the session's impersonation row, if any. The audit
// row gets ended_at/ended_reason only when the impersonation was still live.
func EndImpersonation(ctx context.Context, db *sql.DB, parentToken, reason string) error {
	if parentToken == "" {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := endLocked(ctx, tx, parentToken, reason, time.Now().UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// endLocked locks and deletes the active row for parentToken inside tx.
func endLocked(ctx context.Context, tx *sql.Tx, parentToken, reason string, now time.Time) error {
	var (
		auditID int64
		expires time.Time
	)
	err := tx.QueryRowContext(ctx,
		`SELECT audit_id, expires_at FROM active_impersonations WHERE parent_token = ? FOR UPDATE`,
		parentToken,
	).Scan(&auditID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock active: %w", err)
	}

	if expires.After(now) {
		if _, err := tx.ExecContext(ctx,
			`UPDATE impersonation_audit SET ended_at = ?, ended_reason = ? WHERE id = ? AND ended_at IS NULL`,
			now, reason, auditID,
		); err != nil {
			return fmt.Errorf("close audit: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM active_impersonations WHERE parent_token = ?`, parentToken,
	); err != nil {
		return fmt.Errorf("delete active: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
