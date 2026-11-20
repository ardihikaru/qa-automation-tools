package conversations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/ai"
)

// Store is the SQL ConversationStore (ai_conversations, ai_messages).
type Store struct {
	DB     *db.DB
	Screen *Screen
}

type row struct {
	role, kind, content, hash string
}

// SaveExchange appends what is new in one call (AI_CONVERSATIONS.md 11.1): the sent messages
// after the common prefix with the previous call, then the reply or an error note.
func (s *Store) SaveExchange(ctx context.Context, ex ai.Exchange) error {
	if s == nil || s.DB == nil {
		return errors.New("no database")
	}
	ref := ex.Ref
	if ref.Kind == "" || ref.Ref == "" {
		return errors.New("conversation reference missing")
	}
	sentHashes := make([]string, len(ex.Sent))
	redacted := make([]string, len(ex.Sent))
	for i, m := range ex.Sent {
		redacted[i] = s.Screen.Redact(m.Content)
		sentHashes[i] = Hash(m.Role, redacted[i])
	}
	var replyRed string
	newLast := append([]string(nil), sentHashes...)
	if ex.Err == nil {
		replyRed = s.Screen.Redact(ex.Reply)
		newLast = append(newLast, Hash("assistant", replyRed))
	}
	lastJSON, _ := json.Marshal(newLast)
	var pid interface{}
	if ref.ProductID > 0 {
		pid = ref.ProductID
	}
	title := clipStr(ref.Title, 200)
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO ai_conversations (kind, ref, product_id, feature, title, last_hashes)
			VALUES (?,?,?,?,?,'[]') ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`, ref.Kind, ref.Ref, pid, clipStr(ex.Feature, 64), title)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		var lastRaw []byte
		var count int
		var archived bool
		if err := tx.QueryRowContext(ctx, `SELECT last_hashes, message_count, archived_at IS NOT NULL FROM ai_conversations WHERE id = ? FOR UPDATE`, id).
			Scan(&lastRaw, &count, &archived); err != nil {
			return err
		}
		var last []string
		_ = json.Unmarshal(lastRaw, &last)
		p, rebuilt := Plan(last, sentHashes)
		var rows []row
		if rebuilt {
			rows = append(rows, row{RoleNote, KindRebuilt, RebuiltNote, Hash(RoleNote, RebuiltNote)})
		}
		for i := p; i < len(ex.Sent); i++ {
			m := ex.Sent[i]
			first := count == 0 && len(rows) == 0
			rows = append(rows, row{m.Role, DeriveKind(m.Role, m.Kind, first), Clip(redacted[i]), sentHashes[i]})
		}
		if ex.Err == nil {
			rows = append(rows, row{"assistant", KindReply, Clip(replyRed), newLast[len(newLast)-1]})
		} else {
			msg := "error: " + clipStr(s.Screen.Redact(ex.Err.Error()), 1000)
			rows = append(rows, row{RoleNote, KindError, msg, Hash(RoleNote, msg)})
		}
		var bytes int64
		args := make([]interface{}, 0, len(rows)*6)
		for i, r := range rows {
			args = append(args, id, count+i+1, r.role, r.kind, r.content, r.hash)
			bytes += int64(len(r.content))
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO ai_messages (conversation_id, seq, role, kind, content, content_hash) VALUES `+
			strings.TrimSuffix(strings.Repeat("(?,?,?,?,?,?),", len(rows)), ","), args...); err != nil {
			return err
		}
		if archived {
			log.Printf("ai conversations: %s/%s received a message while archived; it is listed again", ref.Kind, ref.Ref)
		}
		_, err = tx.ExecContext(ctx, `UPDATE ai_conversations SET message_count = message_count + ?, bytes = bytes + ?,
			updated_at = CURRENT_TIMESTAMP(3), last_hashes = ?, provider = ?, model = ?,
			feature = IF(feature = '', ?, feature), title = IF(title = '', ?, title), product_id = COALESCE(product_id, ?),
			archived_at = NULL, archived_by = NULL WHERE id = ?`,
			len(rows), bytes, string(lastJSON), clipStr(ex.Provider, 64), clipStr(ex.Model, 128), clipStr(ex.Feature, 64), title, pid, id)
		return err
	})
}

func clipStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !validTail(s) {
		s = s[:len(s)-1]
	}
	return s
}

func validTail(s string) bool { return strings.ToValidUTF8(s, "") == s }
