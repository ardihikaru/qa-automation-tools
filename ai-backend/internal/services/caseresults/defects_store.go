package caseresults

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
)

// DefectOutcome counts what SaveDefects did. Changed is the number of cases whose defect state is new
// or different (a push is owed for them); Cleared the number of states removed because the case passes.
type DefectOutcome struct {
	Changed int
	Cleared int
}

var scrubPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`), "***"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,}`), "***"},
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), "***"},
	{regexp.MustCompile(`(?i)(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`), "${1} ***"},
	{regexp.MustCompile(`(?i)((?:pass(?:word|wd)?|secret|token|api[_-]?key|authorization|cookie)["']?\s*[:=]\s*)["']?[^\s"',;&]+`), "${1}***"},
	{regexp.MustCompile(`(?i)\b(?:gh[pousr]_|github_pat_|gaos_|sk-)[A-Za-z0-9_-]{16,}`), "***"},
}

// Scrub hides what looks like a secret in text that leaves this server (a failure message can
// echo a header, a token or a connection string). It is applied before the caseauto redactor.
func Scrub(s string) string {
	for _, p := range scrubPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// DefectHash is the content of a state that decides whether GAOS must hear of it again. The excerpt
// and the run are left out: they change on every run.
func DefectHash(caseCode string, d Defect) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{caseCode, d.Kind, d.DefectID, d.Title}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// SaveDefects stores the last defect state of every case in the report (QA_DEFECT_TRIAGE.md). A case
// whose tests passed with no defect loses its state, so its next failure is news again. A report older than
// the stored state leaves it alone. redact (may be nil) screens the excerpt and the title before they are stored.
func SaveDefects(ctx context.Context, d *db.DB, productID int, rep *Report, runRef string, redact func(string) string) (*DefectOutcome, error) {
	out := &DefectOutcome{}
	if rep == nil || len(rep.Results) == 0 {
		return out, nil
	}
	extra := redact
	redact = func(s string) string {
		s = Scrub(s)
		if extra != nil {
			s = extra(s)
		}
		return s
	}
	runAt := rep.StartedAt.UTC().Format("2006-01-02 15:04:05")
	err := d.WithTx(ctx, func(tx *sql.Tx) error {
		out.Changed, out.Cleared = 0, 0
		for code, res := range rep.Results {
			if res == Skipped {
				continue
			}
			var oldHash string
			var oldAt time.Time
			err := tx.QueryRowContext(ctx, `SELECT hash, run_at FROM qa_defect_state WHERE product_id = ? AND case_code = ? FOR UPDATE`, productID, code).Scan(&oldHash, &oldAt)
			exists := err == nil
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if exists && rep.StartedAt.UTC().Before(oldAt) {
				continue // a newer run is already stored
			}
			def, has := rep.Defects[code]
			if !has {
				if exists {
					if _, err := tx.ExecContext(ctx, `DELETE FROM qa_defect_state WHERE product_id = ? AND case_code = ?`, productID, code); err != nil {
						return err
					}
					out.Cleared++
				}
				continue
			}
			def.Title = clipRunesN(strings.TrimSpace(redact(def.Title)), MaxDefectTitle)
			def.Excerpt = clipBytesUTF8(strings.TrimSpace(redact(clipBytesUTF8(def.Excerpt, MaxExcerpt))), MaxExcerpt)
			h := DefectHash(code, def)
			if _, err := tx.ExecContext(ctx, `INSERT INTO qa_defect_state (product_id, case_code, kind, defect_id, title, excerpt, run_ref, run_at, hash)
				VALUES (?,?,?,?,?,?,?,?,?)
				ON DUPLICATE KEY UPDATE kind = VALUES(kind), defect_id = VALUES(defect_id), title = VALUES(title), excerpt = VALUES(excerpt),
				 run_ref = VALUES(run_ref), run_at = VALUES(run_at), hash = VALUES(hash)`,
				productID, code, def.Kind, def.DefectID, def.Title, def.Excerpt, runRef, runAt, h); err != nil {
				return err
			}
			if !exists || oldHash != h {
				out.Changed++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
