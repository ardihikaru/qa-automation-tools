package learnloop

// Findings in MySQL (LEARNING_LOOP.md section 4): reads for the API, the status changes of
// decision 5, and the reconcile that releases findings when their proposal ends.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
)

// Source is one job/case a finding was seen in.
type Source struct {
	JobID    string `json:"job_id"`
	CaseCode string `json:"case_code"`
}

// Finding is a finding as the API serves it.
type Finding struct {
	ID            int64     `json:"id"`
	ProductID     int       `json:"product_id"`
	ProductName   string    `json:"product_name"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	Title         string    `json:"title"`
	Detail        string    `json:"detail"`
	Occurrences   int       `json:"occurrences"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	ProposalJobID string    `json:"proposal_job_id"`
	CreatedBy     string    `json:"created_by"`
	StatusNote    string    `json:"status_note"`
	Sources       []Source  `json:"sources"`
}

// Evidence is one evidence row.
type Evidence struct {
	SourceKey  string    `json:"source_key"`
	JobID      string    `json:"job_id"`
	CaseCode   string    `json:"case_code"`
	RunRef     string    `json:"run_ref"`
	Excerpt    string    `json:"excerpt"`
	ObservedAt time.Time `json:"observed_at"`
}

// FindingFilter narrows ListFindings. Zero values mean "any".
type FindingFilter struct {
	ProductID int
	Status    string
	Kind      string
	Q         string
	Limit     int
	Offset    int
}

const findingCols = `f.id, f.product_id, COALESCE(p.name, ''), f.kind, f.status, f.title, f.detail, f.occurrences, f.first_seen_at, f.last_seen_at,
	f.updated_at, COALESCE(f.proposal_job_id, ''), f.created_by, f.status_note`

func scanFinding(row interface{ Scan(...any) error }) (Finding, error) {
	var f Finding
	err := row.Scan(&f.ID, &f.ProductID, &f.ProductName, &f.Kind, &f.Status, &f.Title, &f.Detail, &f.Occurrences, &f.FirstSeenAt, &f.LastSeenAt,
		&f.UpdatedAt, &f.ProposalJobID, &f.CreatedBy, &f.StatusNote)
	f.FirstSeenAt, f.LastSeenAt, f.UpdatedAt = f.FirstSeenAt.UTC(), f.LastSeenAt.UTC(), f.UpdatedAt.UTC()
	f.Sources = []Source{}
	return f, err
}

func (fl FindingFilter) where(withStatus bool) (string, []any) {
	w := []string{"1=1"}
	var args []any
	if fl.ProductID > 0 {
		w = append(w, "f.product_id = ?")
		args = append(args, fl.ProductID)
	}
	if withStatus && fl.Status != "" {
		w = append(w, "f.status = ?")
		args = append(args, fl.Status)
	}
	if fl.Kind != "" {
		w = append(w, "f.kind = ?")
		args = append(args, fl.Kind)
	}
	if q := strings.TrimSpace(fl.Q); q != "" {
		w = append(w, "(f.title LIKE ? OR f.detail LIKE ?)")
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		args = append(args, like, like)
	}
	return strings.Join(w, " AND "), args
}

// ListFindings returns a page of findings (newest last_seen_at first), the total for the filter,
// and the count per status (over the filter without its status).
func ListFindings(ctx context.Context, d *db.DB, fl FindingFilter) ([]Finding, int, map[string]int, error) {
	if fl.Limit <= 0 || fl.Limit > 200 {
		fl.Limit = 50
	}
	where, args := fl.where(true)
	var total int
	if err := d.QueryRow(ctx, `SELECT COUNT(*) FROM learning_findings f WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, nil, err
	}
	rows, err := d.Query(ctx, `SELECT `+findingCols+` FROM learning_findings f LEFT JOIN products p ON p.id = f.product_id
		WHERE `+where+` ORDER BY f.last_seen_at DESC, f.id DESC LIMIT ? OFFSET ?`, append(args, fl.Limit, fl.Offset)...)
	if err != nil {
		return nil, 0, nil, err
	}
	items := []Finding{}
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			rows.Close()
			return nil, 0, nil, err
		}
		items = append(items, f)
	}
	rows.Close()
	if err := attachSources(ctx, d, items); err != nil {
		return nil, 0, nil, err
	}
	counts := map[string]int{}
	for _, s := range FindingStatuses {
		counts[s] = 0
	}
	cw, cargs := fl.where(false)
	crow, err := d.Query(ctx, `SELECT f.status, COUNT(*) FROM learning_findings f WHERE `+cw+` GROUP BY f.status`, cargs...)
	if err != nil {
		return nil, 0, nil, err
	}
	defer crow.Close()
	for crow.Next() {
		var s string
		var n int
		if err := crow.Scan(&s, &n); err != nil {
			return nil, 0, nil, err
		}
		counts[s] = n
	}
	return items, total, counts, crow.Err()
}

// attachSources fills up to 5 distinct (job, case) sources per finding, newest first.
func attachSources(ctx context.Context, d *db.DB, items []Finding) error {
	if len(items) == 0 {
		return nil
	}
	idx := map[int64]int{}
	ph := make([]string, len(items))
	args := make([]any, len(items))
	for i, f := range items {
		idx[f.ID] = i
		ph[i] = "?"
		args[i] = f.ID
	}
	rows, err := d.Query(ctx, `SELECT finding_id, job_id, case_code FROM learning_evidence WHERE finding_id IN (`+strings.Join(ph, ",")+`)
		ORDER BY observed_at DESC, id DESC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var s Source
		if err := rows.Scan(&id, &s.JobID, &s.CaseCode); err != nil {
			return err
		}
		f := &items[idx[id]]
		if len(f.Sources) >= 5 || (s.JobID == "" && s.CaseCode == "") {
			continue
		}
		dup := false
		for _, x := range f.Sources {
			if x == s {
				dup = true
			}
		}
		if !dup {
			f.Sources = append(f.Sources, s)
		}
	}
	return rows.Err()
}

// ErrNotFound is a missing finding.
var ErrNotFound = errors.New("not found")

// GetFinding returns a finding with up to 50 evidence rows (newest first).
func GetFinding(ctx context.Context, d *db.DB, id int64) (*Finding, []Evidence, error) {
	f, err := scanFinding(d.QueryRow(ctx, `SELECT `+findingCols+` FROM learning_findings f LEFT JOIN products p ON p.id = f.product_id WHERE f.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	items := []Finding{f}
	if err := attachSources(ctx, d, items); err != nil {
		return nil, nil, err
	}
	rows, err := d.Query(ctx, `SELECT source_key, job_id, case_code, run_ref, excerpt, observed_at FROM learning_evidence WHERE finding_id = ?
		ORDER BY observed_at DESC, id DESC LIMIT 50`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	ev := []Evidence{}
	for rows.Next() {
		var e Evidence
		if err := rows.Scan(&e.SourceKey, &e.JobID, &e.CaseCode, &e.RunRef, &e.Excerpt, &e.ObservedAt); err != nil {
			return nil, nil, err
		}
		e.ObservedAt = e.ObservedAt.UTC()
		ev = append(ev, e)
	}
	return &items[0], ev, rows.Err()
}

// ErrBadTransition is a status change the finding's current status does not allow.
var ErrBadTransition = errors.New("bad transition")

// SetFindingStatus moves a finding from one of the allowed statuses to status (dismiss: new →
// dismissed; reopen: dismissed|covered → new). The note is kept for the page.
func SetFindingStatus(ctx context.Context, d *db.DB, id int64, from []string, to, note string) error {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(from)), ",")
	args := []any{to, clip(note, 300), id}
	for _, s := range from {
		args = append(args, s)
	}
	res, err := d.Exec(ctx, `UPDATE learning_findings SET status = ?, status_note = ? WHERE id = ? AND status IN (`+ph+`)`, args...)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		var one int
		if err := d.QueryRow(ctx, `SELECT 1 FROM learning_findings WHERE id = ?`, id).Scan(&one); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return ErrBadTransition
	}
	return nil
}

// ErrBusy is returned by Reserve when a finding is not new (or not of the product).
type ErrBusy struct{ IDs []int64 }

func (e *ErrBusy) Error() string {
	s := make([]string, len(e.IDs))
	for i, id := range e.IDs {
		s[i] = fmt.Sprint(id)
	}
	return "finding(s) not available for a new proposal: " + strings.Join(s, ", ")
}

// Reserve marks the findings proposed for jobID in one transaction: every id must be a new
// finding of the product, else nothing changes and *ErrBusy (or ErrNotFound) is returned.
func Reserve(ctx context.Context, d *db.DB, productID int, ids []int64, jobID string) error {
	if len(ids) == 0 {
		return ErrNotFound
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	return d.WithTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, product_id, status FROM learning_findings WHERE id IN (`+ph+`) FOR UPDATE`, args...)
		if err != nil {
			return err
		}
		seen := map[int64]bool{}
		var busy []int64
		for rows.Next() {
			var id int64
			var pid int
			var st string
			if err := rows.Scan(&id, &pid, &st); err != nil {
				rows.Close()
				return err
			}
			seen[id] = true
			if pid != productID || st != FindingNew {
				busy = append(busy, id)
			}
		}
		rows.Close()
		if len(seen) != len(ids) {
			return ErrNotFound
		}
		if len(busy) > 0 {
			sort.Slice(busy, func(i, j int) bool { return busy[i] < busy[j] })
			return &ErrBusy{IDs: busy}
		}
		_, err = tx.ExecContext(ctx, `UPDATE learning_findings SET status = 'proposed', proposal_job_id = ?, status_note = '' WHERE id IN (`+ph+`)`,
			append([]any{jobID}, args...)...)
		return err
	})
}

// Release returns a job's proposed findings to new (an enqueue that failed after Reserve).
func Release(ctx context.Context, d *db.DB, jobID string) error {
	_, err := d.Exec(ctx, `UPDATE learning_findings SET status = 'new' WHERE proposal_job_id = ? AND status = 'proposed'`, jobID)
	return err
}

// Briefs loads what the model is told about the findings (in id order).
func Briefs(ctx context.Context, d *db.DB, ids []int64) ([]FindingBrief, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := d.Query(ctx, `SELECT id, kind, title, detail FROM learning_findings WHERE id IN (`+ph+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	var out []FindingBrief
	idx := map[int64]int{}
	for rows.Next() {
		var b FindingBrief
		if err := rows.Scan(&b.ID, &b.Kind, &b.Title, &b.Detail); err != nil {
			rows.Close()
			return nil, err
		}
		idx[b.ID] = len(out)
		out = append(out, b)
	}
	rows.Close()
	ev, err := d.Query(ctx, `SELECT finding_id, case_code, excerpt FROM learning_evidence WHERE finding_id IN (`+ph+`) ORDER BY observed_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer ev.Close()
	for ev.Next() {
		var id int64
		var code, ex string
		if err := ev.Scan(&id, &code, &ex); err != nil {
			return nil, err
		}
		b := &out[idx[id]]
		if code != "" && !contains(b.CaseCodes, code) {
			b.CaseCodes = append(b.CaseCodes, code)
		}
		if ex != "" && len(b.Excerpts) < 3 && ex != b.Detail {
			b.Excerpts = append(b.Excerpts, ex)
		}
	}
	return out, ev.Err()
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Reconcile applies decision 5 to every proposed finding whose proposal job has ended:
// covered → covered, merged → learned, rejected/failed/closed → new. Conditional updates, so it
// is safe to run on every tick and after any path that ended a job.
func Reconcile(ctx context.Context, d *db.DB) (int, error) {
	rows, err := d.Query(ctx, `SELECT f.id, j.status, COALESCE(j.error, ''), COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.result, '$.decision')), ''),
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.result, '$.pr_url')), ''), COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.result, '$.pr_state')), '')
		FROM learning_findings f JOIN job_runs j ON j.id`+ci+` = f.proposal_job_id`+ci+`
		WHERE f.status = 'proposed' AND j.status IN ('completed', 'failed')`)
	if err != nil {
		return 0, err
	}
	type change struct {
		id     int64
		to     string
		note   string
		jobErr string
	}
	var changes []change
	for rows.Next() {
		var id int64
		var f JobFacts
		if err := rows.Scan(&id, &f.Status, &f.Error, &f.Decision, &f.PRURL, &f.PRState); err != nil {
			rows.Close()
			return 0, err
		}
		switch st := ProposalState(f); st {
		case StateCovered:
			changes = append(changes, change{id: id, to: FindingCovered, note: "already covered by an existing rule"})
		case StateLearned:
			changes = append(changes, change{id: id, to: FindingLearned})
		case StatePROpen:
		case StateRejected, StateClosed, StateFailed:
			changes = append(changes, change{id: id, to: FindingNew, note: "the last proposal ended: " + st})
		}
	}
	rows.Close()
	n := 0
	for _, c := range changes {
		res, err := d.Exec(ctx, `UPDATE learning_findings SET status = ?, status_note = ? WHERE id = ? AND status = 'proposed'`, c.to, clip(c.note, 300), c.id)
		if err != nil {
			return n, err
		}
		n += int(res.RowsAffected())
	}
	return n, nil
}

// LearnFindings marks a merged proposal's findings learned (also done by Reconcile).
func LearnFindings(ctx context.Context, d *db.DB, jobID string) error {
	_, err := d.Exec(ctx, `UPDATE learning_findings SET status = 'learned', status_note = '' WHERE proposal_job_id = ? AND status = 'proposed'`, jobID)
	return err
}
