package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
)

// Chatter is the LLM seam of the Translator (*ai.Service satisfies it; tests use a fake).
type Chatter interface {
	Configured() bool
	ChatMessages(ctx context.Context, msgs []models.Message, opts ai.ChatOpts) (string, error)
	LastUsage() ai.ChatResult
}

// TranslatorConfig tunes a Translator. Zero values take the spec defaults.
type TranslatorConfig struct {
	Model       string        // AI_TRANSLATION_MODEL; "" disables translation
	Concurrency int           // products translated in parallel (1 or 2)
	BatchFields int           // fields per call (20)
	BatchChars  int           // characters per call (6000)
	Pause       time.Duration // pause between calls (1 s)
}

// Errors of the translation API.
var (
	ErrTranslationDisabled = errors.New("translation is disabled (AI_TRANSLATION_MODEL is empty or the AI service is not configured)")
)

// Failure backoff after attempt n (1-based): 1 h, 6 h, 24 h, 24 h; the fifth failure gives up.
var failureBackoff = []time.Duration{time.Hour, 6 * time.Hour, 24 * time.Hour, 24 * time.Hour}

const (
	maxAttempts      = 5
	translateFeature = "knowledge-translation"
)

// Translator fills kb_translations in the background (KNOWLEDGE_TRANSLATION.md). It never blocks
// or fails a sync: Trigger returns at once and every error stays in the run record and the log.
type Translator struct {
	s   *Syncer
	ai  Chatter
	cfg TranslatorConfig
	ctx context.Context
	sem chan struct{}
	now func() time.Time

	mu      sync.Mutex
	running map[int]bool
	dirty   map[int]bool
	wg      sync.WaitGroup
}

// NewTranslator returns nil without a Syncer. ctx bounds every background run.
func NewTranslator(ctx context.Context, s *Syncer, ai Chatter, cfg TranslatorConfig) *Translator {
	if s == nil {
		return nil
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.Concurrency > 2 {
		cfg.Concurrency = 2
	}
	if cfg.BatchFields < 1 {
		cfg.BatchFields = 20
	}
	if cfg.BatchChars < 1 {
		cfg.BatchChars = 6000
	}
	if cfg.Pause < 0 {
		cfg.Pause = 0
	}
	cfg.Model = strings.TrimSpace(cfg.Model)
	return &Translator{s: s, ai: ai, cfg: cfg, ctx: ctx, sem: make(chan struct{}, cfg.Concurrency), now: time.Now,
		running: map[int]bool{}, dirty: map[int]bool{}}
}

// Enabled reports whether translation runs: a model is set and the AI service is configured.
func (t *Translator) Enabled() bool {
	return t != nil && t.cfg.Model != "" && t.ai != nil && t.ai.Configured()
}

// Model is the configured translation model ("" = disabled).
func (t *Translator) Model() string {
	if t == nil {
		return ""
	}
	return t.cfg.Model
}

// Wait blocks until every background run ended (tests).
func (t *Translator) Wait() { t.wg.Wait() }

// Trigger is the Syncer.OnSucceeded hook: it starts a run for the product when something is
// pending and returns at once. A run already going for the product is asked to go once more.
func (t *Translator) Trigger(productID int) {
	if !t.Enabled() {
		return
	}
	if _, err := t.start(t.ctx, productID, "sync", false, false); err != nil {
		log.Printf("knowledge: translation trigger for product %d: %v", productID, err)
	}
}

// StartRun starts a forced run (POST .../translations/run): backoff and give-up are ignored. It
// returns the run id, ErrTranslationDisabled, ErrNotFound, or a *ConflictError while one runs.
func (t *Translator) StartRun(ctx context.Context, productID int) (int64, error) {
	if !t.Enabled() {
		return 0, ErrTranslationDisabled
	}
	if err := t.s.productExists(ctx, productID); err != nil {
		return 0, err
	}
	return t.start(ctx, productID, "manual", true, true)
}

// start plans a run and launches it in the background. quiet = false skips run rows with nothing to
// do (sync trigger); manual = true reports a running run as a conflict.
func (t *Translator) start(ctx context.Context, pid int, trigger string, force, manual bool) (int64, error) {
	t.mu.Lock()
	if t.running[pid] {
		if !manual {
			t.dirty[pid] = true
			t.mu.Unlock()
			return 0, nil
		}
		t.mu.Unlock()
		return 0, &ConflictError{Msg: "a translation run is already in progress for this product"}
	}
	t.running[pid] = true
	t.mu.Unlock()
	release := func() {
		t.mu.Lock()
		delete(t.running, pid)
		delete(t.dirty, pid)
		t.mu.Unlock()
	}
	pl, err := t.plan(ctx, pid, force)
	if err != nil {
		release()
		return 0, err
	}
	if len(pl.pending) == 0 && !manual {
		release()
		return 0, nil
	}
	res, err := t.s.DB.Exec(ctx, `INSERT INTO kb_translation_runs (product_id, trigger_kind, status, total, started_at) VALUES (?, ?, 'running', ?, UTC_TIMESTAMP())`, pid, trigger, len(pl.pending))
	if err != nil {
		release()
		return 0, err
	}
	runID := res.LastInsertId()
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("knowledge: translation run %d panicked: %v", runID, r)
				t.finishRun(runID, &runState{err: fmt.Sprintf("panic: %v", r)})
				release()
			}
		}()
		t.execute(runID, pid, pl, trigger, force)
		// A sync landed while the run was going: look once more.
		t.mu.Lock()
		again := t.dirty[pid]
		t.mu.Unlock()
		release()
		if again {
			if _, err := t.start(t.ctx, pid, "sync", false, false); err != nil {
				log.Printf("knowledge: translation re-run for product %d: %v", pid, err)
			}
		}
	}()
	return runID, nil
}

// ---------------------------------------------------------------- planning

// field is one English text awaiting (or holding) a translation.
type field struct {
	et, code, name, text, hash string
}

type coverage struct{ total, translated, stale, missing, failed int }

type plan struct {
	pending []field
	cov     coverage
}

// plan reads every active translatable field, the stored translations and the failures, and returns
// the fields to translate (missing or stale; failures only when due unless force) plus the coverage.
func (t *Translator) plan(ctx context.Context, pid int, force bool) (*plan, error) {
	type key struct{ et, code, name string }
	stored := map[key]string{}
	rs, err := t.s.DB.Query(ctx, `SELECT entity_type, code, field, src_hash FROM kb_translations WHERE product_id = ? AND lang = 'id'`, pid)
	if err != nil {
		return nil, err
	}
	for rs.Next() {
		var k key
		var h string
		if err := rs.Scan(&k.et, &k.code, &k.name, &h); err != nil {
			rs.Close()
			return nil, err
		}
		stored[k] = h
	}
	if err := rs.Err(); err != nil {
		rs.Close()
		return nil, err
	}
	rs.Close()
	type fail struct {
		attempts int
		next     time.Time
	}
	fails := map[key]fail{}
	rs, err = t.s.DB.Query(ctx, `SELECT entity_type, code, field, attempts, next_try_at FROM kb_translation_failures WHERE product_id = ?`, pid)
	if err != nil {
		return nil, err
	}
	for rs.Next() {
		var k key
		var f fail
		if err := rs.Scan(&k.et, &k.code, &k.name, &f.attempts, &f.next); err != nil {
			rs.Close()
			return nil, err
		}
		fails[k] = f
	}
	if err := rs.Err(); err != nil {
		rs.Close()
		return nil, err
	}
	rs.Close()

	now := t.now().UTC()
	pl := &plan{}
	for _, et := range translatableTypes {
		for _, col := range translatable[et] {
			rows, err := t.s.DB.Query(ctx, fmt.Sprintf("SELECT code, %s FROM %s WHERE product_id = ? AND removed_at IS NULL AND %s IS NOT NULL AND TRIM(%s) <> '' ORDER BY code", col, tables[et].name, col, col), pid)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var code, text string
				if err := rows.Scan(&code, &text); err != nil {
					rows.Close()
					return nil, err
				}
				text = strings.TrimSpace(text)
				k := key{et, code, col}
				h := textHash(text)
				pl.cov.total++
				sh, has := stored[k]
				if has && sh == h {
					pl.cov.translated++
					continue
				}
				if has {
					pl.cov.stale++
				} else {
					pl.cov.missing++
				}
				f, failed := fails[k]
				if failed {
					pl.cov.failed++
					if !force && (f.attempts >= maxAttempts || f.next.After(now)) {
						continue
					}
				}
				pl.pending = append(pl.pending, field{et: et, code: code, name: col, text: text, hash: h})
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return pl, nil
}

// ---------------------------------------------------------------- batching and prompt

type batchIn struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type batchOut struct {
	ID         string `json:"id"`
	SourceLang string `json:"source_lang"`
	Text       string `json:"text"`
}

const translateSystem = `You translate text of a software QA test catalog (a hospital information system) for Indonesian-speaking QA staff.

The user message is a JSON array of {"id": string, "text": string}. Reply with ONLY a JSON array (no prose, no code fences) holding exactly one object per input, in the same order: {"id": the same id, "source_lang": "en" | "id" | "mixed", "text": string}.

source_lang is the language the input text is written in:
- "en": text is the natural Indonesian translation of the input.
- "id": the input is already Indonesian; text is the input unchanged.
- "mixed": text is the input with only its English portions translated to Indonesian; the Indonesian portions stay as they are.

Keep these in English, exactly as written, inside the Indonesian text: the keywords Given / When / Then; UI element, button, menu, tab and field labels; field names; codes (for example TC-ABC-001, US-ABC-01, A01.07); URLs; identifiers; anything in backticks or {curly braces}. Translate only the prose around them. Do not add, drop or reorder them. Never add explanations.`

// batches splits fields into calls of at most n fields and chars characters (a longer field goes alone).
func batches(fields []field, n, chars int) [][]field {
	var out [][]field
	var cur []field
	size := 0
	for _, f := range fields {
		l := len([]rune(f.text))
		if len(cur) > 0 && (len(cur) >= n || size+l > chars) {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, f)
		size += l
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func buildRequest(items []field) []models.Message {
	in := make([]batchIn, len(items))
	for i, f := range items {
		in[i] = batchIn{ID: strconv.Itoa(i + 1), Text: f.text}
	}
	b, _ := json.Marshal(in)
	return []models.Message{{Role: "system", Content: translateSystem}, {Role: "user", Content: string(b)}}
}

// parseReply decodes the model reply: a JSON array, or an object holding it as "items"; code
// fences around it are tolerated.
func parseReply(text string) ([]batchOut, error) {
	s := strings.TrimSpace(text)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	var arr []batchOut
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return arr, nil
	}
	var obj struct {
		Items []batchOut `json:"items"`
	}
	if err := json.Unmarshal([]byte(s), &obj); err == nil && obj.Items != nil {
		return obj.Items, nil
	}
	if i, j := strings.Index(s, "["), strings.LastIndex(s, "]"); i >= 0 && j > i {
		if err := json.Unmarshal([]byte(s[i:j+1]), &arr); err == nil {
			return arr, nil
		}
	}
	return nil, errors.New("reply is not a JSON array of {id, source_lang, text}")
}

var (
	reCode  = regexp.MustCompile(`\b[A-Z][A-Z0-9]{0,5}(?:-[A-Z0-9]+)+\b|\b[A-Z]\d{2}(?:\.\d+)*\b`)
	reTick  = regexp.MustCompile("`[^`\n]+`")
	reHole  = regexp.MustCompile(`\{[^{}\s]+\}`)
	reURL   = regexp.MustCompile(`https?://[^\s)>\]"'` + "`" + `]+`)
	reGWT   = regexp.MustCompile(`\b(?:Given|When|Then)\b`)
	protect = []*regexp.Regexp{reTick, reHole, reURL, reCode, reGWT}
)

// protectedTokens are the substrings of src that must survive translation: codes, backtick spans,
// {placeholders}, URLs and the Given / When / Then keywords.
func protectedTokens(src string) []string {
	seen := map[string]bool{}
	var out []string
	for _, re := range protect {
		for _, m := range re.FindAllString(src, -1) {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// checkItem validates one reply object against its source text.
func checkItem(src string, r batchOut) error {
	switch r.SourceLang {
	case "en", "id", "mixed":
	default:
		return fmt.Errorf("source_lang %q is not en, id or mixed", r.SourceLang)
	}
	if strings.TrimSpace(r.Text) == "" {
		return errors.New("empty text")
	}
	if r.SourceLang == "id" {
		return nil // the source is stored unchanged
	}
	for _, tok := range protectedTokens(src) {
		if !strings.Contains(r.Text, tok) {
			return fmt.Errorf("the translation lost %q", clipRunes(tok, 60))
		}
	}
	return nil
}

// validateBatch matches the reply to the request by id and checks every item. Any problem rejects
// the whole batch.
func validateBatch(items []field, reply []batchOut) ([]batchOut, error) {
	if len(reply) != len(items) {
		return nil, fmt.Errorf("reply has %d items, want %d", len(reply), len(items))
	}
	byID := map[string]batchOut{}
	for _, r := range reply {
		if _, dup := byID[r.ID]; dup {
			return nil, fmt.Errorf("duplicate id %q", r.ID)
		}
		byID[r.ID] = r
	}
	out := make([]batchOut, len(items))
	for i, f := range items {
		r, ok := byID[strconv.Itoa(i+1)]
		if !ok {
			return nil, fmt.Errorf("id %d is missing", i+1)
		}
		if err := checkItem(f.text, r); err != nil {
			return nil, fmt.Errorf("id %d (%s %s %s): %w", i+1, f.et, f.code, f.name, err)
		}
		out[i] = r
	}
	return out, nil
}

// ---------------------------------------------------------------- run

type runState struct {
	mu         sync.Mutex
	total      int
	done       int
	failed     int
	in, out    int
	cost       float64
	warned     bool
	transport  int // consecutive transport errors
	aborted    bool
	err        string
	calls      int
	convTitle  string
	runID      int64
	pid        int
	force      bool
	trigger    string
	lastCallAt time.Time
}

// transportError is a failed call (ai-manager down, timeout, model rejected), as opposed to a reply
// the validator rejects.
type transportError struct{ err error }

func (e *transportError) Error() string { return e.err.Error() }

func (t *Translator) execute(runID int64, pid int, pl *plan, trigger string, force bool) {
	st := &runState{runID: runID, pid: pid, total: len(pl.pending), trigger: trigger, force: force}
	select {
	case t.sem <- struct{}{}:
		defer func() { <-t.sem }()
	case <-t.ctx.Done():
		st.err = "stopped"
		t.finishRun(runID, st)
		return
	}
	var name sql.NullString
	_ = t.s.DB.QueryRow(t.ctx, `SELECT name FROM products WHERE id = ?`, pid).Scan(&name)
	st.convTitle = name.String
	for _, b := range batches(pl.pending, t.cfg.BatchFields, t.cfg.BatchChars) {
		if t.ctx.Err() != nil {
			st.mu.Lock()
			st.err, st.aborted = "stopped", true
			st.mu.Unlock()
			break
		}
		t.process(st, b)
		st.mu.Lock()
		stop := st.aborted
		st.mu.Unlock()
		if stop {
			break
		}
	}
	t.finishRun(runID, st)
}

// process translates one batch; a rejected reply splits it in halves (down to single fields, which
// then go to kb_translation_failures).
func (t *Translator) process(st *runState, items []field) {
	st.mu.Lock()
	if st.aborted {
		st.mu.Unlock()
		return
	}
	st.mu.Unlock()
	res, err := t.call(st, items)
	if err == nil {
		n := t.write(st, items, res)
		st.mu.Lock()
		st.done += n
		st.transport = 0
		st.mu.Unlock()
		return
	}
	var te *transportError
	if errors.As(err, &te) {
		st.mu.Lock()
		st.transport++
		if st.transport >= 2 {
			st.aborted = true
			st.err = clipRunes(err.Error(), 400)
		}
		abort := st.aborted
		st.mu.Unlock()
		if !abort {
			t.recordFailures(st, items, err)
		}
		return
	}
	st.mu.Lock()
	st.transport = 0
	st.mu.Unlock()
	if len(items) == 1 {
		t.recordFailures(st, items, err)
		return
	}
	mid := len(items) / 2
	t.process(st, items[:mid])
	t.process(st, items[mid:])
}

// call sends one batch and validates the reply.
func (t *Translator) call(st *runState, items []field) ([]batchOut, error) {
	if t.cfg.Pause > 0 && st.calls > 0 {
		select {
		case <-time.After(t.cfg.Pause):
		case <-t.ctx.Done():
			return nil, &transportError{t.ctx.Err()}
		}
	}
	st.calls++
	text, err := t.ai.ChatMessages(t.ctx, buildRequest(items), ai.ChatOpts{
		Temperature: 0.1, Feature: translateFeature, Model: t.cfg.Model,
		Conversation: &ai.ConvRef{Kind: ai.ConvTranslation, Ref: strconv.FormatInt(st.runID, 10), ProductID: st.pid, Title: st.convTitle},
	})
	if err != nil {
		return nil, &transportError{err}
	}
	u := t.ai.LastUsage()
	st.mu.Lock()
	st.in += u.InputTokens
	st.out += u.OutputTokens
	st.cost += u.CostUSD
	// ai-manager echoes the requested model, so this proves little; it still catches a gateway that
	// reports another one. Once per run, never an error.
	if u.Model != "" && u.Model != t.cfg.Model && !st.warned {
		st.warned = true
		log.Printf("knowledge: translation run %d: asked for model %q, the reply says %q", st.runID, t.cfg.Model, u.Model)
	}
	st.mu.Unlock()
	reply, err := parseReply(text)
	if err != nil {
		return nil, err
	}
	return validateBatch(items, reply)
}

// write stores the validated translations in one transaction and returns how many were written.
// A field whose English text changed meanwhile, or that holds a fresh original, is skipped.
func (t *Translator) write(st *runState, items []field, res []batchOut) int {
	written := 0
	ctx := context.WithoutCancel(t.ctx)
	err := t.s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		written = 0
		for i, f := range items {
			var cur sql.NullString
			err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE product_id = ? AND code = ? AND removed_at IS NULL", f.name, tables[f.et].name), st.pid, f.code).Scan(&cur)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if textHash(cur.String) != f.hash {
				continue // the source changed during the run; the next run takes it
			}
			var oldState, oldHash string
			err = tx.QueryRowContext(ctx, `SELECT state, src_hash FROM kb_translations WHERE product_id = ? AND entity_type = ? AND code = ? AND field = ? AND lang = 'id' FOR UPDATE`,
				st.pid, f.et, f.code, f.name).Scan(&oldState, &oldHash)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil && oldState == "original" && oldHash == f.hash {
				continue // a machine run never replaces a fresh original
			}
			r := res[i]
			state, text := "machine", strings.TrimSpace(r.Text)
			if r.SourceLang == "id" {
				state, text = "original", f.text
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO kb_translations (product_id, entity_type, code, field, lang, text, src_hash, source_lang, state, model, translated_at)
VALUES (?, ?, ?, ?, 'id', ?, ?, ?, ?, ?, UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE text = VALUES(text), src_hash = VALUES(src_hash), source_lang = VALUES(source_lang), state = VALUES(state), model = VALUES(model), translated_at = VALUES(translated_at)`,
				st.pid, f.et, f.code, f.name, clipBytes(text, maxText), f.hash, r.SourceLang, state, clipRunes(t.cfg.Model, 64)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM kb_translation_failures WHERE product_id = ? AND entity_type = ? AND code = ? AND field = ?`, st.pid, f.et, f.code, f.name); err != nil {
				return err
			}
			written++
		}
		return nil
	})
	if err != nil {
		log.Printf("knowledge: translation run %d: write batch: %v", st.runID, err)
		st.mu.Lock()
		st.failed += len(items)
		st.mu.Unlock()
		return 0
	}
	return written
}

// recordFailures notes items in kb_translation_failures with the next backoff step.
func (t *Translator) recordFailures(st *runState, items []field, cause error) {
	ctx := context.WithoutCancel(t.ctx)
	msg := clipRunes(cause.Error(), 400)
	now := t.now().UTC()
	for _, f := range items {
		var attempts int
		err := t.s.DB.QueryRow(ctx, `SELECT attempts FROM kb_translation_failures WHERE product_id = ? AND entity_type = ? AND code = ? AND field = ?`, st.pid, f.et, f.code, f.name).Scan(&attempts)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("knowledge: translation run %d: read failure: %v", st.runID, err)
			continue
		}
		attempts++
		if attempts > maxAttempts {
			attempts = maxAttempts
		}
		next := now.Add(failureBackoff[min(attempts, len(failureBackoff))-1])
		if _, err := t.s.DB.Exec(ctx, `INSERT INTO kb_translation_failures (product_id, entity_type, code, field, attempts, next_try_at, last_error) VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE attempts = VALUES(attempts), next_try_at = VALUES(next_try_at), last_error = VALUES(last_error)`,
			st.pid, f.et, f.code, f.name, attempts, next, msg); err != nil {
			log.Printf("knowledge: translation run %d: write failure: %v", st.runID, err)
		}
	}
	st.mu.Lock()
	st.failed += len(items)
	st.mu.Unlock()
}

func (t *Translator) finishRun(runID int64, st *runState) {
	st.mu.Lock()
	status := "succeeded"
	if st.err != "" {
		status = "failed"
	}
	done, failed, in, out, cost, errText := st.done, st.failed, st.in, st.out, st.cost, st.err
	st.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := t.s.DB.Exec(ctx, `UPDATE kb_translation_runs SET status = ?, done = ?, failed = ?, input_tokens = ?, output_tokens = ?, cost_usd = ?, finished_at = UTC_TIMESTAMP(), error = ? WHERE id = ?`,
		status, done, failed, in, out, cost, clipRunes(errText, 400), runID); err != nil {
		log.Printf("knowledge: translation run %d: write result: %v", runID, err)
	}
	log.Printf("knowledge: translation run %d %s: %d done, %d failed, %d+%d tokens, $%.4f", runID, status, done, failed, in, out, cost)
}

// FailOrphanedTranslationRuns marks runs a previous process left running as failed (startup).
func (s *Syncer) FailOrphanedTranslationRuns(ctx context.Context) {
	if s == nil {
		return
	}
	if res, err := s.DB.Exec(ctx, `UPDATE kb_translation_runs SET status = 'failed', error = 'backend restarted while the run was in progress', finished_at = UTC_TIMESTAMP() WHERE status = 'running'`); err != nil {
		log.Printf("knowledge: reconcile orphaned translation runs: %v", err)
	} else if n := res.RowsAffected(); n > 0 {
		log.Printf("knowledge: marked %d orphaned translation run(s) as failed", n)
	}
}

// ---------------------------------------------------------------- status

// TranslationStatus is GET .../knowledge/translations/status:
//
//	{enabled, model, lang_coverage: {total, translated, stale, missing, failed}, running, last_run}
//
// total counts the active non-empty translatable fields; translated + stale + missing = total;
// failed is the part of stale + missing that has a failure row. last_run is null before the first run.
func (t *Translator) TranslationStatus(ctx context.Context, pid int) (Item, error) {
	if err := t.s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	pl, err := t.plan(ctx, pid, false)
	if err != nil {
		return nil, err
	}
	var last Item
	var id int64
	var trig, status, errText string
	var total, done, failed, in, out int
	var cost float64
	var started time.Time
	var finished sql.NullTime
	err = t.s.DB.QueryRow(ctx, `SELECT id, trigger_kind, status, total, done, failed, input_tokens, output_tokens, cost_usd, started_at, finished_at, error
FROM kb_translation_runs WHERE product_id = ? ORDER BY id DESC LIMIT 1`, pid).Scan(&id, &trig, &status, &total, &done, &failed, &in, &out, &cost, &started, &finished, &errText)
	switch {
	case err == nil:
		last = Item{"id": id, "trigger": trig, "status": status, "total": total, "done": done, "failed": failed,
			"input_tokens": in, "output_tokens": out, "cost_usd": cost, "started_at": started.UTC().Format(time.RFC3339), "finished_at": nil, "error": errText}
		if finished.Valid {
			last["finished_at"] = finished.Time.UTC().Format(time.RFC3339)
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return nil, err
	}
	t.mu.Lock()
	running := t.running[pid]
	t.mu.Unlock()
	if last != nil && last["status"] == "running" {
		running = true
	}
	var lastOut any
	if last != nil {
		lastOut = last
	}
	return Item{
		"enabled": t.Enabled(), "model": t.Model(), "running": running, "last_run": lastOut,
		"lang_coverage": Item{"total": pl.cov.total, "translated": pl.cov.translated, "stale": pl.cov.stale, "missing": pl.cov.missing, "failed": pl.cov.failed},
	}, nil
}
