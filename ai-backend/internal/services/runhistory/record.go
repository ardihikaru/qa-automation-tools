// Package runhistory logs every Playwright report the backend receives and computes how long a
// build spent in each step (docs/designs/RUN_HISTORY.md). It writes only pw_runs and pw_run_cases;
// kb_case_results (the latest result per case, sent to GAOS) is never read or written here.
package runhistory

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseresults"
)

// Sources of a Playwright run.
const (
	SourceCI         = "ci"
	SourceUpload     = "upload"
	SourceCaseRun    = "case-run"
	SourceCaseCreate = "case-create"
)

// RunIn says where a report came from. RunIndex is the case_create executor-run counter (nil otherwise).
type RunIn struct {
	ProductID int
	Source    string
	RunRef    string
	JobID     string
	CaseCode  string
	RunIndex  *int
}

// TimeLayout is how started_at is written: UTC with milliseconds.
const TimeLayout = "2006-01-02 15:04:05.000"

const caseChunk = 500

// Record stores one run and its per-case rows in one transaction and returns the run's id. A
// report already recorded (same product, source, run_ref and start) returns the existing id and
// writes nothing. rep is never modified.
func Record(ctx context.Context, d *db.DB, in RunIn, rep *caseresults.Report) (int64, error) {
	if d == nil || rep == nil {
		return 0, errors.New("runhistory: no database or report")
	}
	if in.ProductID <= 0 || in.Source == "" {
		return 0, errors.New("runhistory: product and source are required")
	}
	codes := make([]string, 0, len(rep.Results))
	for c := range rep.Results {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	var dur, idx interface{}
	if rep.Stats.DurationMs != nil {
		dur = *rep.Stats.DurationMs
	}
	if in.RunIndex != nil {
		idx = *in.RunIndex
	}
	var id int64
	err := d.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO pw_runs
			(product_id, source, run_ref, job_id, case_code, run_index, started_at, duration_ms, tests, expected, unexpected, flaky, skipped, cases)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
			in.ProductID, in.Source, in.RunRef, in.JobID, in.CaseCode, idx, rep.StartedAt.UTC().Format(TimeLayout), dur,
			rep.Tests, rep.Stats.Expected, rep.Stats.Unexpected, rep.Stats.Flaky, rep.Stats.Skipped, len(codes))
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		// Without clientFoundRows MySQL reports 1 for a new row and 0 when the key already existed.
		if n, _ := res.RowsAffected(); n != 1 {
			return nil
		}
		for start := 0; start < len(codes); start += caseChunk {
			end := start + caseChunk
			if end > len(codes) {
				end = len(codes)
			}
			args := make([]interface{}, 0, (end-start)*7)
			for _, c := range codes[start:end] {
				cs := rep.Cases[c]
				args = append(args, id, in.ProductID, c, rep.Results[c], cs.DurationMs, cs.Tests, cs.Attempts)
			}
			q := `INSERT INTO pw_run_cases (run_id, product_id, case_code, result, duration_ms, tests, attempts) VALUES ` +
				strings.TrimSuffix(strings.Repeat("(?,?,?,?,?,?,?),", end-start), ",")
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}
