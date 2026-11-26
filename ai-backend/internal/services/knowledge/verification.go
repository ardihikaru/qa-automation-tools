package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Verification verdicts (CASE_VERIFICATION_AND_EPIC_TAB.md). "cleared" exists only in the history.
const (
	VerdictValid   = "valid"
	VerdictInvalid = "invalid"
	VerdictCleared = "cleared"
)

// VerificationBody is the PUT .../verification body. Verdict must be present: a JSON string
// ("valid" | "invalid") or null (clear). An absent key is a 400.
type VerificationBody struct {
	Verdict json.RawMessage `json:"verdict"`
	Note    string          `json:"note"`
}

func verificationItem(verdict, by, name string, at time.Time, note string) Item {
	return Item{"verdict": verdict, "verified_by": by, "verified_by_name": name,
		"verified_at": at.UTC().Format(time.RFC3339), "note": note}
}

// PutVerification records (or clears) the human verdict of a live test case. It returns the new
// verification item, or nil when the verdict was cleared. The verification row and its history
// event change in one transaction.
func (s *Syncer) PutVerification(ctx context.Context, pid int, code, userID, userName string, b VerificationBody) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	if !CodeRe.MatchString(code) {
		return nil, ErrNotFound
	}
	if len(b.Verdict) == 0 {
		return nil, badInput("verdict is required (valid, invalid or null)")
	}
	verdict := VerdictCleared
	if string(b.Verdict) != "null" {
		var v string
		if err := json.Unmarshal(b.Verdict, &v); err != nil || (v != VerdictValid && v != VerdictInvalid) {
			return nil, badInput("verdict must be valid, invalid or null")
		}
		verdict = v
	}
	note := strings.TrimSpace(b.Note)
	if utf8.RuneCountInString(note) > 500 {
		return nil, badInput("note must be at most 500 characters")
	}
	var x int
	err := s.DB.QueryRow(ctx, `SELECT 1 FROM kb_test_cases WHERE product_id = ? AND code = ? AND removed_at IS NULL`, pid, code).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(userName) > 191 {
		userName = userName[:191]
	}
	now := time.Now().UTC().Truncate(time.Second)
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if verdict == VerdictCleared {
			if _, err := tx.ExecContext(ctx, `DELETE FROM kb_case_verifications WHERE product_id = ? AND case_code = ?`, pid, code); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `INSERT INTO kb_case_verifications (product_id, case_code, verdict, note, verified_by, verified_by_name, verified_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE verdict = VALUES(verdict), note = VALUES(note), verified_by = VALUES(verified_by), verified_by_name = VALUES(verified_by_name), verified_at = VALUES(verified_at)`,
			pid, code, verdict, note, userID, userName, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO kb_case_verification_events (product_id, case_code, verdict, note, verified_by, verified_by_name, verified_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, pid, code, verdict, note, userID, userName, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	if verdict == VerdictCleared {
		return nil, nil
	}
	return verificationItem(verdict, userID, userName, now, note), nil
}

// attachVerification sets item["verification"] (an Item or nil) on test case items.
func (s *Syncer) attachVerification(ctx context.Context, pid int, items []Item) error {
	byCode := map[string]Item{}
	for start := 0; start < len(items); start += 500 {
		end := min(start+500, len(items))
		args := []any{pid}
		for _, it := range items[start:end] {
			args = append(args, it["code"])
		}
		rs, err := s.DB.Query(ctx, `SELECT case_code, verdict, verified_by, verified_by_name, verified_at, note FROM kb_case_verifications
WHERE product_id = ? AND case_code IN (`+inList(end-start)+`)`, args...)
		if err != nil {
			return err
		}
		for rs.Next() {
			var code, verdict, by, name, note string
			var at time.Time
			if err := rs.Scan(&code, &verdict, &by, &name, &at, &note); err != nil {
				rs.Close()
				return err
			}
			byCode[code] = verificationItem(verdict, by, name, at, note)
		}
		err = rs.Err()
		rs.Close()
		if err != nil {
			return err
		}
	}
	for _, it := range items {
		code, _ := it["code"].(string)
		if v, ok := byCode[code]; ok {
			it["verification"] = v
		} else {
			it["verification"] = nil
		}
	}
	return nil
}

// verificationFilter turns ?verified= and ?verified_by= into a WHERE fragment on kb_test_cases t.
func verificationFilter(verified, verifiedBy string) (string, []any, error) {
	const exists = "EXISTS (SELECT 1 FROM kb_case_verifications v WHERE v.product_id = t.product_id AND v.case_code = t.code"
	var where string
	var args []any
	switch verified {
	case "":
	case "any":
		where = exists + ")"
	case VerdictValid, VerdictInvalid:
		where = exists + " AND v.verdict = ?)"
		args = append(args, verified)
	case "none":
		if verifiedBy != "" {
			return "", nil, badInput("verified_by cannot be combined with verified=none")
		}
		where = "NOT " + exists + ")"
	default:
		return "", nil, badInput("verified must be one of any, valid, invalid, none")
	}
	if verifiedBy != "" && verified != "none" {
		if verified == "" || verified == "any" {
			where = exists + " AND v.verified_by = ?)"
			args = []any{verifiedBy}
		} else {
			where = exists + " AND v.verdict = ? AND v.verified_by = ?)"
			args = []any{verified, verifiedBy}
		}
	}
	return where, args, nil
}

// Verifiers lists the people who currently hold at least one verification in the product.
func (s *Syncer) Verifiers(ctx context.Context, pid int) ([]Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	rs, err := s.DB.Query(ctx, `SELECT v.verified_by, MAX(v.verified_by_name), COUNT(*) FROM kb_case_verifications v
JOIN kb_test_cases t ON t.product_id = v.product_id AND t.code = v.case_code AND t.removed_at IS NULL
WHERE v.product_id = ? GROUP BY v.verified_by ORDER BY MAX(v.verified_by_name), v.verified_by`, pid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []Item{}
	for rs.Next() {
		var id, name string
		var n int
		if err := rs.Scan(&id, &name, &n); err != nil {
			return nil, err
		}
		out = append(out, Item{"id": id, "name": name, "count": n})
	}
	return out, rs.Err()
}

// VerificationHistory returns the events of one test case, newest first, and their total.
func (s *Syncer) VerificationHistory(ctx context.Context, pid int, code string, limit int) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	if !CodeRe.MatchString(code) {
		return nil, ErrNotFound
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var x int
	err := s.DB.QueryRow(ctx, `SELECT 1 FROM kb_test_cases WHERE product_id = ? AND code = ?`, pid, code).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	total, err := s.verificationCount(ctx, pid, code)
	if err != nil {
		return nil, err
	}
	rs, err := s.DB.Query(ctx, `SELECT id, verdict, note, verified_by, verified_by_name, verified_at FROM kb_case_verification_events
WHERE product_id = ? AND case_code = ? ORDER BY id DESC LIMIT ?`, pid, code, limit)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	hist := []Item{}
	for rs.Next() {
		var id int64
		var verdict, note, by, name string
		var at time.Time
		if err := rs.Scan(&id, &verdict, &note, &by, &name, &at); err != nil {
			return nil, err
		}
		hist = append(hist, Item{"id": id, "verdict": verdict, "note": note, "verified_by": by,
			"verified_by_name": name, "verified_at": at.UTC().Format(time.RFC3339)})
	}
	return Item{"history": hist, "total": total}, rs.Err()
}

func (s *Syncer) verificationCount(ctx context.Context, pid int, code string) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT COUNT(*) FROM kb_case_verification_events WHERE product_id = ? AND case_code = ?`, pid, code).Scan(&n)
	return n, err
}

// epicVerification is the per-epic verification and last-run tally, in one grouped query.
type epicVerification struct{ valid, invalid, pass, fail int }

func (s *Syncer) epicVerifications(ctx context.Context, pid int) (map[string]epicVerification, error) {
	rs, err := s.DB.Query(ctx, `SELECT t.epic_code,
  COUNT(CASE WHEN v.verdict = 'valid' THEN 1 END), COUNT(CASE WHEN v.verdict = 'invalid' THEN 1 END),
  COUNT(CASE WHEN r.result = 'pass' THEN 1 END), COUNT(CASE WHEN r.result = 'fail' THEN 1 END)
FROM kb_test_cases t
LEFT JOIN kb_case_verifications v ON v.product_id = t.product_id AND v.case_code = t.code
LEFT JOIN kb_case_results r ON r.product_id = t.product_id AND r.case_code = t.code
WHERE t.product_id = ? AND t.removed_at IS NULL GROUP BY t.epic_code`, pid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string]epicVerification{}
	for rs.Next() {
		var e string
		var v epicVerification
		if err := rs.Scan(&e, &v.valid, &v.invalid, &v.pass, &v.fail); err != nil {
			return nil, err
		}
		out[e] = v
	}
	return out, rs.Err()
}
