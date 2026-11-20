package learnloop

// The harvester (LEARNING_LOOP.md decision 11): reads what the case automation already wrote
// (case_approvals, job_events, job_runs) and turns it into findings. Pure candidate builders
// first, then the SQL.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
)

// HarvestWindow is how far back every harvest looks.
const HarvestWindow = 30 * 24 * time.Hour

// RepeatThreshold is the number of distinct jobs from which a failure signature is a finding.
const RepeatThreshold = 3

// Candidate is one piece of evidence that may become, or join, a finding.
type Candidate struct {
	ProductID  int
	Kind       string
	SourceKey  string
	JobID      string
	CaseCode   string
	RunRef     string
	Lesson     string // the text that is fingerprinted and shown as the finding's detail
	Excerpt    string // what this piece of evidence says (<= 1000)
	ObservedAt time.Time
	CreatedBy  string // "system" or a user id
	Title      string // "" = derived from kind and lesson
}

// Title is "<kind label>: <first line of the lesson>" (<= 160 bytes).
func Title(kind, lesson string) string {
	first := strings.TrimSpace(lesson)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	label := KindLabel[kind]
	if label == "" {
		label = kind
	}
	return clip(label+": "+oneLine(first), 160)
}

// ApprovalRow is an answered case approval with text.
type ApprovalRow struct {
	ExternalID string
	JobID      string
	ProductID  int
	CaseCode   string
	Status     string // changes | rejected
	Text       string
	AnsweredAt time.Time
}

// ApprovalCandidate turns a reviewer's answer into a candidate.
func ApprovalCandidate(a ApprovalRow) (Candidate, bool) {
	text := strings.TrimSpace(a.Text)
	if text == "" || a.ProductID <= 0 {
		return Candidate{}, false
	}
	kind := KindReviewChanges
	switch a.Status {
	case caseauto.ApprovalChanges:
	case caseauto.ApprovalRejected:
		kind = KindReviewRejected
	default:
		return Candidate{}, false
	}
	return Candidate{ProductID: a.ProductID, Kind: kind, SourceKey: "approval:" + a.ExternalID, JobID: a.JobID, CaseCode: a.CaseCode,
		Lesson: text, Excerpt: clip(text, 1000), ObservedAt: a.AnsweredAt}, true
}

// EventRow is a job event of a case_create job that names a failure, or the pass.
type EventRow struct {
	JobID     string
	ProductID int
	CaseCode  string
	Seq       int
	Message   string
	At        time.Time
}

const passedMsg = "the spec passed"

// isFailureMsg reports whether an event message is a failure the harvester reads.
func isFailureMsg(m string) bool {
	l := strings.ToLower(m)
	return strings.HasPrefix(l, PrefixRefused) || strings.HasPrefix(l, PrefixNotPass)
}

// RepairCandidates finds case_create jobs in which a failure was followed (higher seq) by a
// pass. The lesson is the signatures of the failures before the last pass (<= 3); a job whose
// failures have no signature ("TC-X failed" only) teaches nothing and is skipped.
func RepairCandidates(events []EventRow) []Candidate {
	byJob := map[string][]EventRow{}
	var order []string
	for _, e := range events {
		if _, ok := byJob[e.JobID]; !ok {
			order = append(order, e.JobID)
		}
		byJob[e.JobID] = append(byJob[e.JobID], e)
	}
	var out []Candidate
	for _, id := range order {
		evs := byJob[id]
		sort.Slice(evs, func(i, j int) bool { return evs[i].Seq < evs[j].Seq })
		lastPass := -1
		for i, e := range evs {
			if e.Message == passedMsg {
				lastPass = i
			}
		}
		if lastPass < 0 {
			continue
		}
		var sigs []string
		seen := map[string]bool{}
		for _, e := range evs[:lastPass] {
			if !isFailureMsg(e.Message) {
				continue
			}
			if s, ok := Signature(e.Message); ok && !seen[Fingerprint(s)] && len(sigs) < 3 {
				seen[Fingerprint(s)] = true
				sigs = append(sigs, s)
			}
		}
		if len(sigs) == 0 || evs[0].ProductID <= 0 {
			continue
		}
		lesson := strings.Join(sigs, "\n")
		out = append(out, Candidate{ProductID: evs[0].ProductID, Kind: KindRepairFixed, SourceKey: "job:" + id + ":repair", JobID: id,
			CaseCode: evs[0].CaseCode, Lesson: lesson, Excerpt: clip(lesson, 1000), ObservedAt: evs[lastPass].At})
	}
	return out
}

// FailureMsg is one failure text of one job (an event or the job's error).
type FailureMsg struct {
	JobID     string
	ProductID int
	CaseCode  string
	Message   string
	At        time.Time
}

// RepeatedCandidates groups failure signatures per product; a signature seen in at least
// RepeatThreshold distinct jobs yields one candidate per job (they all join one finding).
func RepeatedCandidates(msgs []FailureMsg) []Candidate {
	type group struct {
		sig  string
		jobs map[string]FailureMsg
		ids  []string
	}
	groups := map[string]*group{}
	var keys []string
	for _, m := range msgs {
		if m.ProductID <= 0 {
			continue
		}
		sig, ok := Signature(m.Message)
		if !ok {
			continue
		}
		fp := Fingerprint(sig)
		k := fmt.Sprintf("%d|%s", m.ProductID, fp)
		g := groups[k]
		if g == nil {
			g = &group{sig: sig, jobs: map[string]FailureMsg{}}
			groups[k] = g
			keys = append(keys, k)
		}
		if _, dup := g.jobs[m.JobID]; !dup {
			g.jobs[m.JobID] = m
			g.ids = append(g.ids, m.JobID)
		}
	}
	var out []Candidate
	for _, k := range keys {
		g := groups[k]
		if len(g.ids) < RepeatThreshold {
			continue
		}
		fp12 := Fingerprint(g.sig)[:12]
		for _, id := range g.ids {
			m := g.jobs[id]
			out = append(out, Candidate{ProductID: m.ProductID, Kind: KindRepeatedFailure, SourceKey: "job:" + id + ":sig:" + fp12, JobID: id,
				CaseCode: m.CaseCode, Lesson: g.sig, Excerpt: clip(strings.TrimSpace(m.Message), 1000), ObservedAt: m.At})
		}
	}
	return out
}

// ---- SQL ----

// HarvestStats counts what one harvest did.
type HarvestStats struct {
	Candidates int
	Created    int
	Joined     int
}

// ci is the collation the job tables are compared in (job_events and case_approvals were
// created with the server default, job_runs with utf8mb4_general_ci).
const ci = " COLLATE utf8mb4_general_ci"

// Harvest scans the window and ingests every candidate. Redact screens the texts.
func Harvest(ctx context.Context, d *db.DB, now time.Time, red *caseauto.Redactor) (HarvestStats, error) {
	var st HarvestStats
	since := now.Add(-HarvestWindow)
	var cands []Candidate

	rows, err := d.Query(ctx, `SELECT a.external_id, a.job_id, a.product_id, a.case_code, a.status, COALESCE(a.answer_text, ''), a.answered_at
		FROM case_approvals a JOIN job_runs j ON j.id`+ci+` = a.job_id`+ci+`
		WHERE a.status IN ('changes', 'rejected') AND COALESCE(a.answer_text, '') <> '' AND a.answered_at >= ? AND j.type = 'case_create'
		ORDER BY a.answered_at LIMIT 2000`, since)
	if err != nil {
		return st, fmt.Errorf("harvest approvals: %w", err)
	}
	for rows.Next() {
		var a ApprovalRow
		if err := rows.Scan(&a.ExternalID, &a.JobID, &a.ProductID, &a.CaseCode, &a.Status, &a.Text, &a.AnsweredAt); err != nil {
			rows.Close()
			return st, err
		}
		if c, ok := ApprovalCandidate(a); ok {
			cands = append(cands, c)
		}
	}
	rows.Close()

	rows, err = d.Query(ctx, `SELECT j.id, CAST(COALESCE(JSON_EXTRACT(j.payload, '$.product_id'), 0) AS UNSIGNED),
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.payload, '$.case_code')), ''), e.seq, e.message, e.`+"`at`"+`
		FROM job_runs j JOIN job_events e ON e.job_id`+ci+` = j.id`+ci+`
		WHERE j.type = 'case_create' AND j.created_at >= ?
		  AND ((e.step = 'run' AND (e.message LIKE 'the spec did not pass:%' OR e.message = 'the spec passed'))
		    OR (e.step IN ('generate', 'repair') AND e.message LIKE 'the reply was refused:%'))
		ORDER BY j.id, e.seq LIMIT 20000`, since)
	if err != nil {
		return st, fmt.Errorf("harvest events: %w", err)
	}
	var events []EventRow
	var failures []FailureMsg
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.JobID, &e.ProductID, &e.CaseCode, &e.Seq, &e.Message, &e.At); err != nil {
			rows.Close()
			return st, err
		}
		events = append(events, e)
		if isFailureMsg(e.Message) {
			failures = append(failures, FailureMsg{JobID: e.JobID, ProductID: e.ProductID, CaseCode: e.CaseCode, Message: e.Message, At: e.At})
		}
	}
	rows.Close()
	cands = append(cands, RepairCandidates(events)...)

	rows, err = d.Query(ctx, `SELECT id, CAST(COALESCE(JSON_EXTRACT(payload, '$.product_id'), 0) AS UNSIGNED),
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.case_code')), ''), COALESCE(error, ''), COALESCE(completed_at, created_at)
		FROM job_runs WHERE type IN ('case_create', 'case_run') AND status = 'failed' AND created_at >= ? LIMIT 5000`, since)
	if err != nil {
		return st, fmt.Errorf("harvest job errors: %w", err)
	}
	for rows.Next() {
		var m FailureMsg
		if err := rows.Scan(&m.JobID, &m.ProductID, &m.CaseCode, &m.Message, &m.At); err != nil {
			rows.Close()
			return st, err
		}
		if strings.TrimSpace(m.Message) != "" && !IsHumanOutcome(m.Message) {
			failures = append(failures, m)
		}
	}
	rows.Close()
	cands = append(cands, RepeatedCandidates(failures)...)

	st.Candidates = len(cands)
	for _, c := range cands {
		c.Lesson = red.Redact(c.Lesson)
		c.Excerpt = red.Redact(c.Excerpt)
		_, created, skipped, err := Ingest(ctx, d, c)
		if err != nil {
			return st, err
		}
		switch {
		case skipped:
		case created:
			st.Created++
		default:
			st.Joined++
		}
	}
	return st, nil
}

// Ingest stores one candidate (decision 11): a known source_key is skipped; else, in one
// transaction, the product's open finding with the same fingerprint is joined (occurrences,
// last_seen_at) or a finding is created, and the evidence row is inserted.
func Ingest(ctx context.Context, d *db.DB, c Candidate) (findingID int64, created, skipped bool, err error) {
	var one int
	switch err := d.QueryRow(ctx, `SELECT 1 FROM learning_evidence WHERE source_key = ?`, c.SourceKey).Scan(&one); {
	case err == nil:
		return 0, false, true, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, false, false, err
	}
	if c.CreatedBy == "" {
		c.CreatedBy = "system"
	}
	if c.ObservedAt.IsZero() {
		c.ObservedAt = time.Now().UTC()
	}
	lesson := clip(strings.TrimSpace(c.Lesson), 2000)
	title := c.Title
	if title == "" {
		title = Title(c.Kind, lesson)
	}
	fp := Fingerprint(lesson)
	err = d.WithTx(ctx, func(tx *sql.Tx) error {
		created = false
		err := tx.QueryRowContext(ctx, `SELECT id FROM learning_findings WHERE product_id = ? AND fingerprint = ? AND status IN ('new', 'proposed')
			ORDER BY id LIMIT 1 FOR UPDATE`, c.ProductID, fp).Scan(&findingID)
		if errors.Is(err, sql.ErrNoRows) {
			res, err := tx.ExecContext(ctx, `INSERT INTO learning_findings (product_id, kind, status, title, detail, fingerprint, occurrences, created_by, first_seen_at, last_seen_at)
				VALUES (?, ?, 'new', ?, ?, ?, 1, ?, ?, ?)`, c.ProductID, c.Kind, clip(title, 200), lesson, fp, c.CreatedBy, c.ObservedAt, c.ObservedAt)
			if err != nil {
				return err
			}
			findingID, _ = res.LastInsertId()
			created = true
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO learning_evidence (finding_id, source_key, job_id, case_code, run_ref, excerpt, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, findingID, c.SourceKey, clip(c.JobID, 191), clip(c.CaseCode, 64), clip(c.RunRef, 120), clip(c.Excerpt, 1000), c.ObservedAt); err != nil {
			return err
		}
		if !created {
			_, err := tx.ExecContext(ctx, `UPDATE learning_findings SET occurrences = occurrences + 1, last_seen_at = GREATEST(last_seen_at, ?) WHERE id = ?`, c.ObservedAt, findingID)
			return err
		}
		return nil
	})
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return 0, false, true, nil // a concurrent ingest stored the same source first
	}
	return findingID, created, false, err
}

// ManualSourceKey is the source key of a hand-entered finding.
func ManualSourceKey() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "manual:" + hex.EncodeToString(b[:])
}
