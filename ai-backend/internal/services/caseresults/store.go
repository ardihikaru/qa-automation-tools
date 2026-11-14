package caseresults

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
)

// Outcome counts what Save did.
type Outcome struct {
	Stored     int      `json:"stored"`
	Stale      int      `json:"stale"`     // kept: the stored result is from a newer run
	Unmatched  []string `json:"unmatched"` // codes that are not active cases of the product (first 20)
	UnmatchedN int      `json:"unmatched_count"`
}

// Save stores the latest result per case. A case keeps its newest run: a report started before the
// stored run_at leaves the row alone. Codes that are not active cases of the product are reported, not stored.
func Save(ctx context.Context, d *db.DB, productID int, rep *Report, runRef, source string) (*Outcome, error) {
	out := &Outcome{Unmatched: []string{}}
	if len(rep.Results) == 0 {
		return out, nil
	}
	codes := make([]any, 0, len(rep.Results)+1)
	codes = append(codes, productID)
	for c := range rep.Results {
		codes = append(codes, c)
	}
	rows, err := d.Query(ctx, `SELECT code FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL AND code IN (`+
		strings.TrimSuffix(strings.Repeat("?,", len(codes)-1), ",")+`)`, codes...)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return nil, err
		}
		known[c] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	runAt := rep.StartedAt.UTC().Format("2006-01-02 15:04:05")
	err = d.WithTx(ctx, func(tx *sql.Tx) error {
		out.Stored, out.Stale, out.UnmatchedN, out.Unmatched = 0, 0, 0, []string{}
		for code, res := range rep.Results {
			if !known[code] {
				out.UnmatchedN++
				if len(out.Unmatched) < 20 {
					out.Unmatched = append(out.Unmatched, code)
				}
				continue
			}
			r, err := tx.ExecContext(ctx, `INSERT INTO kb_case_results (product_id, case_code, result, run_at, run_ref, source)
				VALUES (?,?,?,?,?,?)
				ON DUPLICATE KEY UPDATE
				 result = IF(VALUES(run_at) >= run_at, VALUES(result), result),
				 run_ref = IF(VALUES(run_at) >= run_at, VALUES(run_ref), run_ref),
				 source = IF(VALUES(run_at) >= run_at, VALUES(source), source),
				 run_at = GREATEST(run_at, VALUES(run_at))`,
				productID, code, res, runAt, runRef, source)
			if err != nil {
				return err
			}
			// MySQL reports 0 affected rows when nothing changed (same run re-sent) and 2 for an update.
			if n, _ := r.RowsAffected(); n == 0 {
				out.Stale++
			} else {
				out.Stored++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DefaultRunRef is used when the caller gives none: "pw-<unix start>".
func DefaultRunRef(t time.Time) string { return "pw-" + strconv.FormatInt(t.Unix(), 10) }
