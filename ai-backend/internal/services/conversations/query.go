package conversations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
)

// Item is a conversation without its messages.
type Item struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	Ref            string     `json:"ref"`
	ProductID      *int       `json:"product_id"`
	ProductName    string     `json:"product_name"`
	Feature        string     `json:"feature"`
	Title          string     `json:"title"`
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	MessageCount   int        `json:"message_count"`
	Bytes          int64      `json:"bytes"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ArchivedAt     *time.Time `json:"archived_at"`
	ArchivedByName string     `json:"archived_by_name"`
}

// Message is one stored message.
type Message struct {
	Seq       int       `json:"seq"`
	Role      string    `json:"role"`
	Kind      string    `json:"kind"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Filter selects a list.
type Filter struct {
	Archived  bool
	Kind      string
	ProductID int
	Q         string
	Page      int
	Size      int
}

const itemCols = `c.id, c.kind, c.ref, c.product_id, COALESCE(p.name, ''), c.feature, c.title, c.provider, c.model,
	c.message_count, c.bytes, c.created_at, c.updated_at, c.archived_at, COALESCE(u.username, '')`

const itemFrom = ` FROM ai_conversations c LEFT JOIN products p ON p.id = c.product_id
	LEFT JOIN users u ON u.id = c.archived_by COLLATE utf8mb4_general_ci`

type scanner interface{ Scan(...interface{}) error }

func scanItem(sc scanner) (*Item, error) {
	var it Item
	var pid sql.NullInt64
	var arch sql.NullTime
	if err := sc.Scan(&it.ID, &it.Kind, &it.Ref, &pid, &it.ProductName, &it.Feature, &it.Title, &it.Provider, &it.Model,
		&it.MessageCount, &it.Bytes, &it.CreatedAt, &it.UpdatedAt, &arch, &it.ArchivedByName); err != nil {
		return nil, err
	}
	if pid.Valid {
		n := int(pid.Int64)
		it.ProductID = &n
	}
	if arch.Valid {
		it.ArchivedAt = &arch.Time
	}
	return &it, nil
}

// List returns one page of conversations, newest activity first.
func List(ctx context.Context, d *db.DB, f Filter) ([]*Item, int, error) {
	where := []string{"c.archived_at IS NULL"}
	if f.Archived {
		where[0] = "c.archived_at IS NOT NULL"
	}
	var args []interface{}
	if f.Kind != "" {
		where = append(where, "c.kind = ?")
		args = append(args, f.Kind)
	}
	if f.ProductID > 0 {
		where = append(where, "c.product_id = ?")
		args = append(args, f.ProductID)
	}
	if f.Q != "" {
		where = append(where, "(c.ref = ? OR c.title = ?)")
		args = append(args, f.Q, f.Q)
	}
	w := " WHERE " + strings.Join(where, " AND ")
	var total int
	if err := d.QueryRow(ctx, `SELECT COUNT(*) FROM ai_conversations c`+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := " ORDER BY c.updated_at DESC, c.id DESC"
	if f.Archived {
		order = " ORDER BY c.archived_at DESC, c.id DESC"
	}
	rows, err := d.Query(ctx, `SELECT `+itemCols+itemFrom+w+order+` LIMIT ? OFFSET ?`, append(args, f.Size, (f.Page-1)*f.Size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// Get returns one conversation, nil when absent.
func Get(ctx context.Context, d *db.DB, id int64) (*Item, error) {
	it, err := scanItem(d.QueryRow(ctx, `SELECT `+itemCols+itemFrom+` WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return it, err
}

// GetByRef returns the conversation of a job or sync, nil when absent.
func GetByRef(ctx context.Context, d *db.DB, kind, ref string) (*Item, error) {
	it, err := scanItem(d.QueryRow(ctx, `SELECT `+itemCols+itemFrom+` WHERE c.kind = ? AND c.ref = ?`, kind, ref))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return it, err
}

// Messages returns the messages with seq > since, in order.
func Messages(ctx context.Context, d *db.DB, id int64, since int) ([]Message, error) {
	rows, err := d.Query(ctx, `SELECT seq, role, kind, content, created_at FROM ai_messages WHERE conversation_id = ? AND seq > ? ORDER BY seq`, id, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Seq, &m.Role, &m.Kind, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Archive outcomes.
var (
	ErrNotFound        = errors.New("not_found")
	ErrAlreadyArchived = errors.New("already_archived")
	ErrNotArchived     = errors.New("not_archived")
	ErrActive          = errors.New("active")
)

// ownerActive is true while the job or sync that owns c can still add messages (11.5).
const ownerActive = `(
	(c.kind = 'job' AND EXISTS (SELECT 1 FROM job_runs j WHERE j.id = c.ref COLLATE utf8mb4_general_ci AND j.status IN ('queued', 'running')))
	OR (c.kind = 'sync' AND EXISTS (SELECT 1 FROM kb_syncs s WHERE CAST(s.id AS CHAR) = c.ref AND (s.status = 'running' OR s.ai_status = 'pending'))))`

// Archive hides a conversation; one conditional statement, so the owner check cannot go stale.
func Archive(ctx context.Context, d *db.DB, id int64, actorID string) error {
	res, err := d.Exec(ctx, `UPDATE ai_conversations c SET c.archived_at = CURRENT_TIMESTAMP(3), c.archived_by = ?
		WHERE c.id = ? AND c.archived_at IS NULL AND NOT `+ownerActive, actorID, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 1 {
		return nil
	}
	it, err := Get(ctx, d, id)
	switch {
	case err != nil:
		return err
	case it == nil:
		return ErrNotFound
	case it.ArchivedAt != nil:
		return ErrAlreadyArchived
	}
	return ErrActive
}

// Unarchive lists a conversation again.
func Unarchive(ctx context.Context, d *db.DB, id int64) error {
	res, err := d.Exec(ctx, `UPDATE ai_conversations SET archived_at = NULL, archived_by = NULL WHERE id = ? AND archived_at IS NOT NULL`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 1 {
		return nil
	}
	it, err := Get(ctx, d, id)
	if err != nil {
		return err
	}
	if it == nil {
		return ErrNotFound
	}
	return ErrNotArchived
}

// Totals are counts over a set of conversations.
type Totals struct {
	Conversations int   `json:"conversations"`
	Messages      int   `json:"messages"`
	Bytes         int64 `json:"bytes"`
}

// Sizes returns the totals of archived and of all conversations.
func Sizes(ctx context.Context, d *db.DB) (archived, all Totals, err error) {
	err = d.QueryRow(ctx, `SELECT COALESCE(SUM(archived_at IS NOT NULL), 0), COALESCE(SUM(IF(archived_at IS NOT NULL, message_count, 0)), 0),
		COALESCE(SUM(IF(archived_at IS NOT NULL, bytes, 0)), 0), COUNT(*), COALESCE(SUM(message_count), 0), COALESCE(SUM(bytes), 0)
		FROM ai_conversations`).Scan(&archived.Conversations, &archived.Messages, &archived.Bytes, &all.Conversations, &all.Messages, &all.Bytes)
	return
}

// Preview is what a prune would delete now; Before is the newest archived_at included (nil = nothing).
func Preview(ctx context.Context, d *db.DB) (Totals, *time.Time, error) {
	var t Totals
	var before sql.NullTime
	err := d.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(message_count), 0), COALESCE(SUM(bytes), 0), MAX(archived_at)
		FROM ai_conversations WHERE archived_at IS NOT NULL`).Scan(&t.Conversations, &t.Messages, &t.Bytes, &before)
	if err != nil || !before.Valid {
		return t, nil, err
	}
	return t, &before.Time, nil
}

// PruneBatch is how many conversations one prune transaction deletes.
const PruneBatch = 50

// MaxAuditIDs caps the ids kept in a prune's audit detail.
const MaxAuditIDs = 5000

// Prune deletes the conversations archived at or before cutoff (and their messages), in batches.
// It returns what it deleted, also when a batch failed.
func Prune(ctx context.Context, d *db.DB, cutoff time.Time) (Totals, []int64, error) {
	var t Totals
	ids := []int64{}
	for {
		var batch []int64
		var msgs int
		var bytes int64
		err := d.WithTx(ctx, func(tx *sql.Tx) error {
			batch, msgs, bytes = nil, 0, 0
			rows, err := tx.QueryContext(ctx, `SELECT id, message_count, bytes FROM ai_conversations
				WHERE archived_at IS NOT NULL AND archived_at <= ? ORDER BY id LIMIT ? FOR UPDATE`, cutoff, PruneBatch)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id int64
				var n int
				var b int64
				if err := rows.Scan(&id, &n, &b); err != nil {
					rows.Close()
					return err
				}
				batch = append(batch, id)
				msgs += n
				bytes += b
			}
			rows.Close()
			if err := rows.Err(); err != nil || len(batch) == 0 {
				return err
			}
			args := make([]interface{}, len(batch))
			for i, id := range batch {
				args[i] = id
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM ai_conversations WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")+`)`, args...)
			return err
		})
		if err != nil {
			return t, ids, err
		}
		if len(batch) == 0 {
			return t, ids, nil
		}
		t.Conversations += len(batch)
		t.Messages += msgs
		t.Bytes += bytes
		for _, id := range batch {
			if len(ids) < MaxAuditIDs {
				ids = append(ids, id)
			}
		}
	}
}

// AuditEntry is one append-only audit row.
type AuditEntry struct {
	ID             int64           `json:"id"`
	At             time.Time       `json:"at"`
	Action         string          `json:"action"`
	ActorID        string          `json:"actor_id"`
	ActorName      string          `json:"actor_name"`
	AsUserID       *string         `json:"as_user_id"`
	ConversationID *int64          `json:"conversation_id"`
	Detail         json.RawMessage `json:"detail"`
}

// Audit appends one row. asUser is the impersonated identity ("" when none).
func Audit(ctx context.Context, d *db.DB, action, actorID, actorName, asUser string, convID int64, detail interface{}) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	var as, cid interface{}
	if asUser != "" {
		as = asUser
	}
	if convID > 0 {
		cid = convID
	}
	_, err = d.Exec(ctx, `INSERT INTO ai_conversation_audit (action, actor_id, actor_name, as_user_id, conversation_id, detail) VALUES (?,?,?,?,?,?)`,
		action, actorID, clipStr(actorName, 100), as, cid, string(raw))
	return err
}

// AuditLog returns the newest audit rows (prunes only when pruneOnly).
func AuditLog(ctx context.Context, d *db.DB, pruneOnly bool, limit int) ([]AuditEntry, error) {
	q := `SELECT id, at, action, actor_id, actor_name, as_user_id, conversation_id, detail FROM ai_conversation_audit`
	if pruneOnly {
		q += ` WHERE action = 'prune'`
	}
	rows, err := d.Query(ctx, q+` ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var as sql.NullString
		var cid sql.NullInt64
		var detail []byte
		if err := rows.Scan(&e.ID, &e.At, &e.Action, &e.ActorID, &e.ActorName, &as, &cid, &detail); err != nil {
			return nil, err
		}
		if as.Valid {
			e.AsUserID = &as.String
		}
		if cid.Valid {
			e.ConversationID = &cid.Int64
		}
		e.Detail = json.RawMessage(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}
