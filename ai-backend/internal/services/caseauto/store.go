package caseauto

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/db"
)

// Event is one row of job_events as the API serves it.
type Event struct {
	Seq     int       `json:"seq"`
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Step    string    `json:"step"`
	Message string    `json:"message"`
}

// EventStore implements Events over job_events. The text is clipped here as a last line of
// defence; callers redact first (Machine.ev does).
type EventStore struct {
	DB     *db.DB
	Redact *Redactor
}

// Add appends an event with the next seq of the job. Two writers racing for the same seq retry.
func (s *EventStore) Add(ctx context.Context, jobID, level, step, message string) error {
	if s == nil || s.DB == nil {
		return errors.New("no database")
	}
	switch level {
	case "info", "warn", "error":
	default:
		level = "info"
	}
	message = Clip(s.Redact.Redact(message), 1000)
	var err error
	for attempt := 0; attempt < 12; attempt++ {
		_, err = s.DB.Exec(ctx, "INSERT INTO job_events (job_id, seq, `at`, level, step, message) "+
			"SELECT ?, COALESCE(MAX(seq), 0) + 1, UTC_TIMESTAMP(3), ?, ?, ? FROM job_events WHERE job_id = ?",
			jobID, level, Clip(step, 32), message, jobID)
		var me *mysql.MySQLError
		// 1062 duplicate seq and 1213 deadlock (two writers locking the same job's index gap) are
		// both "somebody else got in first": back off a little and take the next seq.
		if err == nil || !errors.As(err, &me) || (me.Number != 1062 && me.Number != 1213) {
			return err
		}
		time.Sleep(time.Duration(5+rand.Intn(20)*(attempt+1)) * time.Millisecond)
	}
	return err
}

// List returns the events with seq > since, oldest first (at most limit).
func (s *EventStore) List(ctx context.Context, jobID string, since, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.DB.Query(ctx, "SELECT seq, `at`, level, step, message FROM job_events WHERE job_id = ? AND seq > ? ORDER BY seq LIMIT ?", jobID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Seq, &e.At, &e.Level, &e.Step, &e.Message); err != nil {
			return nil, err
		}
		e.At = e.At.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// ApprovalStore implements Approvals over case_approvals.
type ApprovalStore struct{ DB *db.DB }

// Open inserts the open approval; a second call with the same externalID is a no-op.
func (s *ApprovalStore) Open(ctx context.Context, jobID string, productID int, caseCode, externalID string) error {
	_, err := s.DB.Exec(ctx, `INSERT IGNORE INTO case_approvals (job_id, product_id, case_code, external_id, status, created_at)
		VALUES (?, ?, ?, ?, 'open', UTC_TIMESTAMP())`, jobID, productID, caseCode, externalID)
	return err
}

const approvalCols = `external_id, status, COALESCE(answered_by, ''), COALESCE(answer_text, ''), gaos_created`

func scanApproval(row interface{ Scan(...any) error }) (*Approval, error) {
	var a Approval
	if err := row.Scan(&a.ExternalID, &a.Status, &a.AnsweredBy, &a.Text, &a.GaosCreated); err != nil {
		return nil, err
	}
	return &a, nil
}

// Get reads one approval by its external id; nil when there is none.
func (s *ApprovalStore) Get(ctx context.Context, externalID string) (*Approval, error) {
	a, err := scanApproval(s.DB.QueryRow(ctx, `SELECT `+approvalCols+` FROM case_approvals WHERE external_id = ?`, externalID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// Latest reads the newest approval of a job; nil when there is none.
func (s *ApprovalStore) Latest(ctx context.Context, jobID string) (*Approval, error) {
	a, err := scanApproval(s.DB.QueryRow(ctx, `SELECT `+approvalCols+` FROM case_approvals WHERE job_id = ? ORDER BY id DESC LIMIT 1`, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// ByExternal returns the owner of an external id: the job, product and case it belongs to.
func (s *ApprovalStore) ByExternal(ctx context.Context, externalID string) (jobID string, productID int, caseCode string, err error) {
	err = s.DB.QueryRow(ctx, `SELECT job_id, product_id, case_code FROM case_approvals WHERE external_id = ?`, externalID).Scan(&jobID, &productID, &caseCode)
	return
}

// MarkGaosCreated records that the GAOS Inbox item exists.
func (s *ApprovalStore) MarkGaosCreated(ctx context.Context, externalID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE case_approvals SET gaos_created = 1 WHERE external_id = ?`, externalID)
	return err
}

// Answer closes an open approval; it reports whether this call changed it (first answer wins).
func (s *ApprovalStore) Answer(ctx context.Context, externalID, status, by, text string) (bool, error) {
	switch status {
	case ApprovalApproved, ApprovalRejected, ApprovalChanges, ApprovalExpired:
	default:
		return false, errors.New("invalid approval status")
	}
	res, err := s.DB.Exec(ctx, `UPDATE case_approvals SET status = ?, answered_by = ?, answer_text = ?, answered_at = UTC_TIMESTAMP()
		WHERE external_id = ? AND status = 'open'`, status, by, Clip(strings.TrimSpace(text), 1000), externalID)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

// OpenForJobs lists the job ids that have an open approval (resumed after a restart).
func (s *ApprovalStore) OpenForJobs(ctx context.Context) ([]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT job_id FROM case_approvals WHERE status = 'open'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
