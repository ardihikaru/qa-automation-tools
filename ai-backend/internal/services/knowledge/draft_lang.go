package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
)

// Draft ID to EN conversion (KNOWLEDGE_TRANSLATION.md, "Draft in ID"). The knowledge base is
// English: a draft typed in Indonesian is converted on save, the English goes to the normal columns
// and the typed text is kept in kb_translations (state "original").

// DraftTranslateError is a failed draft conversion (the API answers 502 and nothing was saved).
type DraftTranslateError struct{ Err error }

func (e *DraftTranslateError) Error() string {
	return "the draft text could not be converted to English: " + e.Err.Error()
}
func (e *DraftTranslateError) Unwrap() error { return e.Err }

// SetDraftTranslator wires the model used by SaveDraft (the Translator built beside the Syncer).
// Nil, or a disabled Translator, leaves English drafts saving as they do and refuses `lang=id`.
func (s *Syncer) SetDraftTranslator(t *Translator) { s.drafts = t }

const draftSystem = `You convert text of a software QA test catalog (a hospital information system) to English.

The user message is a JSON object {"text": string}. Reply with ONLY a JSON object (no prose, no code fences): {"source_lang": "en" | "id" | "mixed", "text": string}.

source_lang is the language the input is written in:
- "en": the input is already English; text is the input unchanged.
- "id": the input is Indonesian; text is its natural English translation.
- "mixed": the input mixes both; text is the whole input in English.

Keep these exactly as written, inside the English text: the keywords Given / When / Then; UI element, button, menu, tab and field labels; field names; codes (for example TC-ABC-001, US-ABC-01, A01.07); URLs; identifiers; anything in backticks or {curly braces}. Translate only the prose around them. Do not add, drop or reorder them. Keep line breaks. Never add explanations.`

type draftReply struct {
	SourceLang string `json:"source_lang"`
	Text       string `json:"text"`
}

func parseDraftReply(text string) (draftReply, error) {
	s := strings.TrimSpace(text)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
	}
	var r draftReply
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i && json.Unmarshal([]byte(s[i:j+1]), &r) == nil {
			return r, checkDraftReply("", r)
		}
		return r, errors.New("reply is not a JSON object {source_lang, text}")
	}
	return r, nil
}

// checkDraftReply validates a reply against its source (src == "" skips the token check).
func checkDraftReply(src string, r draftReply) error {
	switch r.SourceLang {
	case "en", "id", "mixed":
	default:
		return fmt.Errorf("source_lang %q is not en, id or mixed", r.SourceLang)
	}
	if strings.TrimSpace(r.Text) == "" {
		return errors.New("empty text")
	}
	if r.SourceLang == "en" {
		return nil // stored as typed
	}
	for _, tok := range protectedTokens(src) {
		if !strings.Contains(r.Text, tok) {
			return fmt.Errorf("the conversion lost %q", clipRunes(tok, 60))
		}
	}
	return nil
}

// draftToEnglish converts one text. english is the text to store, srcLang what the model found.
// An invalid reply is retried once; a failed call is not.
func (t *Translator) draftToEnglish(ctx context.Context, pid int, text string) (english, srcLang string, err error) {
	in, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{text})
	msgs := []models.Message{{Role: "system", Content: draftSystem}, {Role: "user", Content: string(in)}}
	ref := fmt.Sprintf("draft-%d-%d", pid, time.Now().UnixNano())
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		reply, cerr := t.ai.ChatMessages(ctx, msgs, ai.ChatOpts{
			Temperature: 0.1, Feature: translateFeature, Model: t.cfg.Model,
			Conversation: &ai.ConvRef{Kind: ai.ConvTranslation, Ref: ref + fmt.Sprintf("-%d", attempt), ProductID: pid, Title: "draft"},
		})
		if cerr != nil {
			return "", "", &DraftTranslateError{Err: cerr}
		}
		r, perr := parseDraftReply(reply)
		if perr == nil {
			perr = checkDraftReply(text, r)
		}
		if perr != nil {
			last = perr
			continue
		}
		if r.SourceLang == "en" {
			return text, "en", nil
		}
		return strings.TrimSpace(r.Text), r.SourceLang, nil
	}
	return "", "", &DraftTranslateError{Err: last}
}

// draftText is the translatable field of a draft body: a pointer to it, its translations field name.
func draftText(raw any) (lang string, text *string, field string) {
	switch b := raw.(type) {
	case *StoryBody:
		return b.Lang, &b.Text, "text"
	case *TestCaseBody:
		return b.Lang, &b.Scenario, "scenario"
	}
	return "", nil, ""
}

// draftOriginal is the Indonesian text a save keeps next to its English conversion.
type draftOriginal struct {
	field, text, srcLang string
}

// convertDraft runs the conversion for a body that carries a language hint (`lang` en or id; an
// empty hint is a machine caller and is never sent to the model). On id or mixed text the body is
// rewritten to English and the original is returned. The text is never converted when translation
// is off: `lang=id` then fails with ErrTranslationDisabled and English saves as typed.
func (s *Syncer) convertDraft(ctx context.Context, pid int, raw any) (*draftOriginal, error) {
	lang, text, field := draftText(raw)
	if text == nil || lang == "" {
		return nil, nil
	}
	if !ValidLang(lang) {
		return nil, badInput("lang must be en or id")
	}
	trimmed := strings.TrimSpace(*text)
	if trimmed == "" || len(trimmed) > maxText {
		return nil, nil // the normal validation reports it
	}
	if !s.drafts.Enabled() {
		if lang == LangID {
			return nil, ErrTranslationDisabled
		}
		return nil, nil
	}
	cctx, cancel := context.WithTimeout(ctx, s.aiTimeout)
	defer cancel()
	en, src, err := s.drafts.draftToEnglish(cctx, pid, trimmed)
	if err != nil {
		return nil, err
	}
	if src == "en" {
		return nil, nil
	}
	*text = en
	return &draftOriginal{field: field, text: trimmed, srcLang: src}, nil
}

// storeDraftOriginal keeps the typed text next to the (already validated) English text in tx.
func (s *Syncer) storeDraftOriginal(ctx context.Context, tx *sql.Tx, pid int, et, code, english string, o *draftOriginal) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO kb_translations (product_id, entity_type, code, field, lang, text, src_hash, source_lang, state, model, translated_at)
VALUES (?, ?, ?, ?, 'id', ?, ?, ?, 'original', ?, UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE text = VALUES(text), src_hash = VALUES(src_hash), source_lang = VALUES(source_lang), state = 'original', model = VALUES(model), translated_at = VALUES(translated_at)`,
		pid, et, code, o.field, clipBytes(o.text, maxText), textHash(english), o.srcLang, clipRunes(s.drafts.Model(), 64))
	return err
}
