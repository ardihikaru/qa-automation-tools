package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Display languages (KNOWLEDGE_TRANSLATION.md). English is the stored catalog language.
const (
	LangEN = "en"
	LangID = "id"
)

// translatable lists, per entity type, the English text fields that get an Indonesian text.
var translatable = map[string][]string{
	TypePart:      {"name"},
	TypeModule:    {"name"},
	TypeSubmodule: {"name"},
	TypeFeature:   {"name", "notes"},
	TypeEpic:      {"title", "area"},
	TypeStory:     {"text"},
	TypeTestCase:  {"scenario"},
}

// translatableTypes fixes an order for loops over the map.
var translatableTypes = []string{TypePart, TypeModule, TypeSubmodule, TypeFeature, TypeEpic, TypeStory, TypeTestCase}

type langCtxKey struct{}

// WithLang returns ctx carrying the effective display language ("en" or "id"). Reads that find
// "id" add the additive `i18n` block to knowledge items.
func WithLang(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, langCtxKey{}, lang)
}

// LangOf is the display language carried by ctx ("en" when none).
func LangOf(ctx context.Context) string {
	if l, _ := ctx.Value(langCtxKey{}).(string); l == LangID {
		return LangID
	}
	return LangEN
}

// ValidLang reports whether l is a supported language code.
func ValidLang(l string) bool { return l == LangEN || l == LangID }

// textHash is the staleness key of an English field: SHA-256 of the trimmed, NFC-normalised text.
func textHash(s string) string {
	sum := sha256.Sum256([]byte(norm.NFC.String(strings.TrimSpace(s))))
	return hex.EncodeToString(sum[:])
}

// i18nRow is one stored translation.
type i18nRow struct {
	text, srcHash, sourceLang, state, model string
	at                                      time.Time
}

// attachI18N adds `i18n: {lang, fields: {field: {text, state, source_lang, model, translated_at}}}` to
// items of entity type et when the context language is Indonesian. A field appears only when a text
// exists; state is machine | original | stale (stale = the English text changed since). keyOf maps a
// field to the item key that holds its English text ("" = the field is not in the item).
func (s *Syncer) attachI18N(ctx context.Context, pid int, et string, items []Item, keyOf func(field string) string) error {
	fields := translatable[et]
	if LangOf(ctx) != LangID || len(items) == 0 || len(fields) == 0 {
		return nil
	}
	codes := make([]any, 0, len(items))
	for _, it := range items {
		if c, ok := it["code"].(string); ok {
			codes = append(codes, c)
		}
	}
	byKey := map[string]i18nRow{}
	for start := 0; start < len(codes); start += 500 {
		b := codes[start:min(start+500, len(codes))]
		args := append([]any{pid, et, LangID}, b...)
		rs, err := s.DB.Query(ctx, `SELECT code, field, text, src_hash, source_lang, state, model, translated_at FROM kb_translations
WHERE product_id = ? AND entity_type = ? AND lang = ? AND code IN (`+inList(len(b))+`)`, args...)
		if err != nil {
			return err
		}
		for rs.Next() {
			var code, field string
			var r i18nRow
			if err := rs.Scan(&code, &field, &r.text, &r.srcHash, &r.sourceLang, &r.state, &r.model, &r.at); err != nil {
				rs.Close()
				return err
			}
			byKey[code+"\x00"+field] = r
		}
		if err := rs.Err(); err != nil {
			rs.Close()
			return err
		}
		rs.Close()
	}
	for _, it := range items {
		code, _ := it["code"].(string)
		out := Item{}
		for _, f := range fields {
			key := keyOf(f)
			if key == "" {
				continue
			}
			r, ok := byKey[code+"\x00"+f]
			if !ok {
				continue
			}
			state := r.state
			if en, _ := it[key].(string); r.srcHash != textHash(en) {
				state = "stale"
			}
			out[f] = Item{"text": r.text, "state": state, "source_lang": r.sourceLang, "model": r.model, "translated_at": r.at.UTC().Format(time.RFC3339)}
		}
		it["i18n"] = Item{"lang": LangID, "fields": out}
	}
	return nil
}

// ownKey is the keyOf of full entity rows: the item key is the field name.
func ownKey(f string) string { return f }

// qMatch builds the `q` condition of a list: the code or an English column matches, or an
// Indonesian text of one of the fields does (any language is searched, whatever the display
// language; a stale text still counts). Returns the SQL and its args.
func qMatch(et, q string, cols ...string) (string, []any) {
	like := likeArg(q)
	parts := []string{"t.code LIKE ?"}
	args := []any{like}
	for _, c := range cols {
		parts = append(parts, "t."+c+" LIKE ?")
		args = append(args, like)
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = "'" + c + "'"
	}
	parts = append(parts, fmt.Sprintf("EXISTS (SELECT 1 FROM kb_translations x WHERE x.product_id = t.product_id AND x.entity_type = ? AND x.code = t.code AND x.lang = 'id' AND x.field IN (%s) AND x.text LIKE ?)", strings.Join(quoted, ",")))
	args = append(args, et, like)
	return "(" + strings.Join(parts, " OR ") + ")", args
}
