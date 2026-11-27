package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func TestTextHashTrimsAndNormalises(t *testing.T) {
	a := textHash("  café ") // precomposed
	b := textHash("café")   // decomposed
	if a != b {
		t.Fatal("NFC and trim must give one hash")
	}
	if textHash("cafe") == a {
		t.Fatal("different text, same hash")
	}
	if len(a) != 64 {
		t.Fatalf("hash length %d", len(a))
	}
}

func TestProtectedTokens(t *testing.T) {
	src := "Given the user opens `Daftar Pasien` at https://x.test/a?b=1 for A01.07 and TC-REG-001, when {nama} is set Then it works"
	got := strings.Join(protectedTokens(src), "|")
	for _, want := range []string{"`Daftar Pasien`", "https://x.test/a?b=1", "A01.07", "TC-REG-001", "{nama}", "Given", "Then"} {
		if !strings.Contains(got, want) {
			t.Errorf("token %q missing from %q", want, got)
		}
	}
	if strings.Contains(got, "|when") {
		t.Errorf("lower-case prose must not be protected: %q", got)
	}
}

func TestCheckItem(t *testing.T) {
	src := "Given `Save` button for TC-A-001 When clicked Then {id} saved"
	ok := batchOut{ID: "1", SourceLang: "en", Text: "Given tombol `Save` untuk TC-A-001 When diklik Then {id} tersimpan"}
	if err := checkItem(src, ok); err != nil {
		t.Fatalf("valid reply rejected: %v", err)
	}
	for name, mut := range map[string]func(batchOut) batchOut{
		"code dropped":     func(b batchOut) batchOut { b.Text = strings.ReplaceAll(b.Text, "TC-A-001", "TC"); return b },
		"backtick dropped": func(b batchOut) batchOut { b.Text = strings.ReplaceAll(b.Text, "`Save`", "Simpan"); return b },
		"gwt translated":   func(b batchOut) batchOut { b.Text = strings.ReplaceAll(b.Text, "Then", "Maka"); return b },
		"placeholder":      func(b batchOut) batchOut { b.Text = strings.ReplaceAll(b.Text, "{id}", "id"); return b },
		"empty":            func(b batchOut) batchOut { b.Text = "  "; return b },
		"bad lang":         func(b batchOut) batchOut { b.SourceLang = "fr"; return b },
	} {
		if err := checkItem(src, mut(ok)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// An Indonesian source is stored unchanged, so only emptiness matters.
	if err := checkItem("Pasien dapat disimpan", batchOut{SourceLang: "id", Text: "x"}); err != nil {
		t.Errorf("id source: %v", err)
	}
}

func TestValidateBatch(t *testing.T) {
	items := []field{{text: "one"}, {text: "two"}}
	good := []batchOut{{ID: "2", SourceLang: "en", Text: "dua"}, {ID: "1", SourceLang: "en", Text: "satu"}}
	out, err := validateBatch(items, good)
	if err != nil || out[0].Text != "satu" || out[1].Text != "dua" {
		t.Fatalf("out of order ids must be matched: %v %+v", err, out)
	}
	for name, bad := range map[string][]batchOut{
		"short":   good[:1],
		"dup":     {{ID: "1", SourceLang: "en", Text: "a"}, {ID: "1", SourceLang: "en", Text: "b"}},
		"unknown": {{ID: "1", SourceLang: "en", Text: "a"}, {ID: "9", SourceLang: "en", Text: "b"}},
		"empty":   {{ID: "1", SourceLang: "en", Text: "a"}, {ID: "2", SourceLang: "en", Text: ""}},
	} {
		if _, err := validateBatch(items, bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestParseReply(t *testing.T) {
	arr := `[{"id":"1","source_lang":"en","text":"x"}]`
	for name, in := range map[string]string{
		"array": arr, "fenced": "```json\n" + arr + "\n```", "object": `{"items":` + arr + `}`, "noise": "Sure!\n" + arr + "\nDone",
	} {
		got, err := parseReply(in)
		if err != nil || len(got) != 1 || got[0].Text != "x" {
			t.Errorf("%s: %v %+v", name, err, got)
		}
	}
	if _, err := parseReply("nope"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestBatches(t *testing.T) {
	var fs []field
	for i := 0; i < 45; i++ {
		fs = append(fs, field{text: "x"})
	}
	b := batches(fs, 20, 6000)
	if len(b) != 3 || len(b[0]) != 20 || len(b[2]) != 5 {
		t.Fatalf("count batches: %d", len(b))
	}
	big := []field{{text: strings.Repeat("a", 4000)}, {text: strings.Repeat("b", 4000)}, {text: "c"}}
	b = batches(big, 20, 6000)
	if len(b) != 2 || len(b[0]) != 1 || len(b[1]) != 2 {
		t.Fatalf("char batches: %v", b)
	}
}

// ---------------------------------------------------------------- DB tests

// fakeChat answers translation calls: each input text becomes "ID: <text>" (codes and tokens kept).
type fakeChat struct {
	mu     sync.Mutex
	calls  int
	models []string
	convs  []ai.ConvRef
	reply  func(in []batchIn) (string, error)
	usage  ai.ChatResult
}

func (f *fakeChat) Configured() bool { return true }
func (f *fakeChat) LastUsage() ai.ChatResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.usage
}
func (f *fakeChat) ChatMessages(_ context.Context, msgs []models.Message, o ai.ChatOpts) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.models = append(f.models, o.Model)
	if o.Conversation != nil {
		f.convs = append(f.convs, *o.Conversation)
	}
	f.usage = ai.ChatResult{Model: o.Model, InputTokens: 10, OutputTokens: 5, CostUSD: 0.001}
	var in []batchIn
	if err := json.Unmarshal([]byte(msgs[len(msgs)-1].Content), &in); err != nil {
		return "", err
	}
	if f.reply != nil {
		return f.reply(in)
	}
	return echoReply(in), nil
}

func echoReply(in []batchIn) string {
	out := make([]batchOut, len(in))
	for i, x := range in {
		out[i] = batchOut{ID: x.ID, SourceLang: "en", Text: "ID: " + x.Text}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

type trEnv struct {
	s    *Syncer
	tr   *Translator
	ai   *fakeChat
	repo *fakeRepo
	pid  int
}

func newTrEnv(t *testing.T) *trEnv {
	t.Helper()
	d := testdb.New(t)
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{}
	repo.set("1111111111111111111111111111111111111111", map[string]string{"docs/modules/A-core.md": modA, "docs/test-catalog/svc/out.md": epic})
	s := &Syncer{DB: d, AI: &fakeAI{}, ReadTree: repo.read, syncTimeout: time.Minute, aiTimeout: time.Minute}
	fc := &fakeChat{}
	tr := NewTranslator(context.Background(), s, fc, TranslatorConfig{Model: "claude-haiku-5-5"})
	s.OnSucceeded = tr.Trigger
	return &trEnv{s: s, tr: tr, ai: fc, repo: repo, pid: pid}
}

func (e *trEnv) sync(t *testing.T) {
	t.Helper()
	id, err := e.s.Start(context.Background(), e.pid, TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitSync(t, e.s.DB, id); r.status != "succeeded" {
		t.Fatalf("sync: %+v", r)
	}
	e.tr.Wait()
}

func (e *trEnv) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.s.DB.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTranslatorAfterSync(t *testing.T) {
	e := newTrEnv(t)
	ctx := context.Background()
	e.sync(t)
	// 1 part, 1 module, 1 submodule?, 2 features (name; notes empty), 1 epic (title, area), 2 stories, 2 cases.
	n := e.count(t, `SELECT COUNT(*) FROM kb_translations WHERE product_id = ? AND lang = 'id' AND state = 'machine'`, e.pid)
	if n == 0 {
		t.Fatal("nothing translated after the sync hook")
	}
	st, err := e.tr.TranslationStatus(ctx, e.pid)
	if err != nil {
		t.Fatal(err)
	}
	cov := st["lang_coverage"].(Item)
	if st["enabled"] != true || st["model"] != "claude-haiku-5-5" || st["running"] != false || cov["missing"] != 0 || cov["stale"] != 0 || cov["translated"] != cov["total"] || cov["total"] != n {
		t.Fatalf("status: %+v", st)
	}
	last := st["last_run"].(Item)
	if last["status"] != "succeeded" || last["trigger"] != "sync" || last["done"] != n || last["input_tokens"] == 0 {
		t.Fatalf("last_run: %+v", last)
	}
	for _, m := range e.ai.models {
		if m != "claude-haiku-5-5" {
			t.Fatalf("model sent %q", m)
		}
	}
	for _, c := range e.ai.convs {
		if c.Kind != ai.ConvTranslation || c.ProductID != e.pid {
			t.Fatalf("conversation ref %+v", c)
		}
	}
	// A second sync of the same commit has nothing pending: no new call, no new run row.
	calls := e.ai.calls
	e.sync(t)
	if e.ai.calls != calls || e.count(t, `SELECT COUNT(*) FROM kb_translation_runs WHERE product_id = ?`, e.pid) != 1 {
		t.Fatalf("unchanged catalog re-translated: calls %d -> %d", calls, e.ai.calls)
	}

	// i18n read shape and search.
	items, err := e.s.Stories(WithLang(ctx, LangID), e.pid, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	first := items["items"].([]Item)[0]
	f := first["i18n"].(Item)["fields"].(Item)["text"].(Item)
	if first["i18n"].(Item)["lang"] != "id" || f["state"] != "machine" || f["source_lang"] != "en" || !strings.HasPrefix(f["text"].(string), "ID: ") || f["model"] != "claude-haiku-5-5" {
		t.Fatalf("i18n: %+v", first["i18n"])
	}
	if first["text"] == f["text"] {
		t.Fatal("English field must stay English")
	}
	en, _ := e.s.Stories(ctx, e.pid, Filter{})
	if _, has := en["items"].([]Item)[0]["i18n"]; has {
		t.Fatal("English reads must not carry i18n")
	}
	// q matches the Indonesian text although the English text lacks it.
	hit, err := e.s.Stories(ctx, e.pid, Filter{Q: "ID: "})
	if err != nil || hit["total"] != 2 {
		t.Fatalf("q over ID text: %v %v", err, hit["total"])
	}
	if miss, _ := e.s.TestCases(ctx, e.pid, Filter{Q: "zzz-nothing"}); miss["total"] != 0 {
		t.Fatal("q must not match everything")
	}
	if cs, _ := e.s.TestCases(ctx, e.pid, Filter{Q: "ID: G1"}); cs["total"] != 1 {
		t.Fatalf("case q: %v", cs["total"])
	}
	if ep, err := e.s.Epics(ctx, e.pid, false, "ID: "); err != nil || len(ep) != 1 {
		t.Fatalf("epics q: %v %d", err, len(ep))
	}
	if ep, _ := e.s.Epics(ctx, e.pid, false, "zzz"); len(ep) != 0 {
		t.Fatal("epics q matched")
	}
	if md, err := e.s.Modules(ctx, e.pid, false, "ID: "); err != nil || len(md) != 1 {
		t.Fatalf("modules q: %v %d", err, len(md))
	}
	if ft, _ := e.s.Features(ctx, e.pid, Filter{Q: "ID: Search"}); ft["total"] == 0 {
		t.Fatal("features q over ID text")
	}
}

func TestTranslatorStaleAndFreshOriginal(t *testing.T) {
	e := newTrEnv(t)
	ctx := context.Background()
	e.sync(t)
	// Edit the English story in the repo: its ID text turns stale until re-translated.
	e.repo.set("2222222222222222222222222222222222222222", map[string]string{"docs/modules/A-core.md": modA,
		"docs/test-catalog/svc/out.md": strings.Replace(epic, "— search", "— search patients", 1)})
	e.tr.cfg.Model = "" // hold translation back to observe the stale state
	e.sync(t)
	items, _ := e.s.Stories(WithLang(ctx, LangID), e.pid, Filter{})
	var st1 Item
	for _, it := range items["items"].([]Item) {
		if it["code"] == "US-OUT-01" {
			st1 = it["i18n"].(Item)["fields"].(Item)["text"].(Item)
		}
	}
	if st1["state"] != "stale" || st1["text"] != "ID: search" {
		t.Fatalf("stale: %+v", st1)
	}
	if cov := mustStatus(t, e)["lang_coverage"].(Item); cov["stale"] != 1 {
		t.Fatalf("coverage: %+v", cov)
	}
	// Re-enable: the manual run re-translates it.
	e.tr.cfg.Model = "claude-haiku-5-5"
	if _, err := e.tr.StartRun(ctx, e.pid); err != nil {
		t.Fatal(err)
	}
	e.tr.Wait()
	var text string
	_ = e.s.DB.QueryRow(ctx, `SELECT text FROM kb_translations WHERE product_id = ? AND code = 'US-OUT-01' AND field = 'text'`, e.pid).Scan(&text)
	if text != "ID: search patients" {
		t.Fatalf("re-translated: %q", text)
	}
	// A fresh original is never replaced by a machine run, and a stale one is.
	h := textHash("search patients")
	if _, err := e.s.DB.Exec(ctx, `UPDATE kb_translations SET text = 'cari pasien', state = 'original', source_lang = 'id' WHERE product_id = ? AND code = 'US-OUT-01'`, e.pid); err != nil {
		t.Fatal(err)
	}
	calls := e.ai.calls
	if _, err := e.tr.StartRun(ctx, e.pid); err != nil {
		t.Fatal(err)
	}
	e.tr.Wait()
	_ = e.s.DB.QueryRow(ctx, `SELECT text FROM kb_translations WHERE product_id = ? AND code = 'US-OUT-01'`, e.pid).Scan(&text)
	if text != "cari pasien" || e.ai.calls != calls {
		t.Fatalf("fresh original touched: %q calls %d -> %d", text, calls, e.ai.calls)
	}
	if _, err := e.s.DB.Exec(ctx, `UPDATE kb_translations SET src_hash = ? WHERE product_id = ? AND code = 'US-OUT-01'`, textHash("old"), e.pid); err != nil {
		t.Fatal(err)
	}
	_ = h
	if _, err := e.tr.StartRun(ctx, e.pid); err != nil {
		t.Fatal(err)
	}
	e.tr.Wait()
	_ = e.s.DB.QueryRow(ctx, `SELECT text FROM kb_translations WHERE product_id = ? AND code = 'US-OUT-01'`, e.pid).Scan(&text)
	if text != "ID: search patients" {
		t.Fatalf("stale original not replaced: %q", text)
	}
}

func mustStatus(t *testing.T, e *trEnv) Item {
	t.Helper()
	st, err := e.tr.TranslationStatus(context.Background(), e.pid)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestTranslatorSourceLangID(t *testing.T) {
	e := newTrEnv(t)
	e.ai.reply = func(in []batchIn) (string, error) {
		out := make([]batchOut, len(in))
		for i, x := range in {
			out[i] = batchOut{ID: x.ID, SourceLang: "id", Text: "whatever the model said"}
		}
		b, _ := json.Marshal(out)
		return string(b), nil
	}
	e.sync(t)
	var state, text, en string
	if err := e.s.DB.QueryRow(context.Background(), `SELECT state, text FROM kb_translations WHERE product_id = ? AND code = 'US-OUT-01'`, e.pid).Scan(&state, &text); err != nil {
		t.Fatal(err)
	}
	_ = e.s.DB.QueryRow(context.Background(), `SELECT text FROM kb_stories WHERE product_id = ? AND code = 'US-OUT-01'`, e.pid).Scan(&en)
	if state != "original" || text != en {
		t.Fatalf("id source: state %q text %q (en %q)", state, text, en)
	}
	items, _ := e.s.Stories(WithLang(context.Background(), LangID), e.pid, Filter{})
	if f := items["items"].([]Item)[0]["i18n"].(Item)["fields"].(Item)["text"].(Item); f["state"] != "original" {
		t.Fatalf("state %v", f["state"])
	}
}

func TestTranslatorSplitsAndRecordsFailures(t *testing.T) {
	e := newTrEnv(t)
	// The model always mangles the story US-OUT-02; everything else is fine.
	e.ai.reply = func(in []batchIn) (string, error) {
		out := make([]batchOut, len(in))
		for i, x := range in {
			txt := "ID: " + x.Text
			if x.Text == "edit" {
				txt = "" // empty text is rejected
			}
			out[i] = batchOut{ID: x.ID, SourceLang: "en", Text: txt}
		}
		b, _ := json.Marshal(out)
		return string(b), nil
	}
	e.sync(t)
	if n := e.count(t, `SELECT COUNT(*) FROM kb_translation_failures WHERE product_id = ?`, e.pid); n != 1 {
		t.Fatalf("failure rows: %d", n)
	}
	var code string
	var attempts int
	var next time.Time
	if err := e.s.DB.QueryRow(context.Background(), `SELECT code, attempts, next_try_at FROM kb_translation_failures WHERE product_id = ?`, e.pid).Scan(&code, &attempts, &next); err != nil {
		t.Fatal(err)
	}
	if code != "US-OUT-02" || attempts != 1 || next.Before(time.Now().Add(50*time.Minute)) {
		t.Fatalf("failure: %s %d %v", code, attempts, next)
	}
	cov := mustStatus(t, e)["lang_coverage"].(Item)
	if cov["failed"] != 1 || cov["missing"] != 1 {
		t.Fatalf("coverage: %+v", cov)
	}
	// A sync trigger honours the backoff (nothing due); a forced run retries and succeeds.
	calls := e.ai.calls
	e.tr.Trigger(e.pid)
	e.tr.Wait()
	if e.ai.calls != calls {
		t.Fatal("failure retried before next_try_at")
	}
	e.ai.reply = nil
	if _, err := e.tr.StartRun(context.Background(), e.pid); err != nil {
		t.Fatal(err)
	}
	e.tr.Wait()
	if n := e.count(t, `SELECT COUNT(*) FROM kb_translation_failures WHERE product_id = ?`, e.pid); n != 0 {
		t.Fatalf("failure not cleared: %d", n)
	}
}

func TestTranslatorTransportErrorAbortsRun(t *testing.T) {
	e := newTrEnv(t)
	e.tr.cfg.BatchFields = 1
	e.ai.reply = func([]batchIn) (string, error) { return "", errors.New("ai-manager: status 503") }
	e.sync(t) // the sync itself must succeed
	st := mustStatus(t, e)
	last := st["last_run"].(Item)
	if last["status"] != "failed" || e.ai.calls != 2 || !strings.Contains(last["error"].(string), "503") {
		t.Fatalf("run: %+v calls %d", last, e.ai.calls)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM kb_translations`); n != 0 {
		t.Fatalf("rows written: %d", n)
	}
}

func TestTranslatorDisabledAndConflict(t *testing.T) {
	e := newTrEnv(t)
	e.tr.cfg.Model = ""
	e.sync(t)
	if e.ai.calls != 0 {
		t.Fatal("disabled translator called the model")
	}
	if _, err := e.tr.StartRun(context.Background(), e.pid); !errors.Is(err, ErrTranslationDisabled) {
		t.Fatalf("err %v", err)
	}
	if st := mustStatus(t, e); st["enabled"] != false || st["model"] != "" {
		t.Fatalf("status: %+v", st)
	}
	e.tr.cfg.Model = "claude-haiku-5-5"
	e.tr.mu.Lock()
	e.tr.running[e.pid] = true
	e.tr.mu.Unlock()
	_, err := e.tr.StartRun(context.Background(), e.pid)
	var cf *ConflictError
	if !errors.As(err, &cf) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestSourceEditedMidRunIsNotWritten(t *testing.T) {
	e := newTrEnv(t)
	ctx := context.Background()
	e.tr.cfg.Model = ""
	e.sync(t)
	e.tr.cfg.Model = "claude-haiku-5-5"
	// While the model "thinks", the English story text changes.
	e.ai.reply = func(in []batchIn) (string, error) {
		_, _ = e.s.DB.Exec(ctx, `UPDATE kb_stories SET text = CONCAT(text, ' (edited)') WHERE product_id = ?`, e.pid)
		return echoReply(in), nil
	}
	if _, err := e.tr.StartRun(ctx, e.pid); err != nil {
		t.Fatal(err)
	}
	e.tr.Wait()
	if n := e.count(t, `SELECT COUNT(*) FROM kb_translations WHERE product_id = ? AND entity_type = 'story'`, e.pid); n != 0 {
		t.Fatalf("stale-source rows written: %d", n)
	}
}

func TestHookPanicDoesNotFailSync(t *testing.T) {
	e := newTrEnv(t)
	e.s.OnSucceeded = func(int) { panic("boom") }
	id, err := e.s.Start(context.Background(), e.pid, TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitSync(t, e.s.DB, id); r.status != "succeeded" {
		t.Fatalf("sync: %+v", r)
	}
}

func TestFailOrphanedTranslationRuns(t *testing.T) {
	e := newTrEnv(t)
	ctx := context.Background()
	if _, err := e.s.DB.Exec(ctx, `INSERT INTO kb_translation_runs (product_id, trigger_kind, status, started_at) VALUES (?, 'manual', 'running', UTC_TIMESTAMP())`, e.pid); err != nil {
		t.Fatal(err)
	}
	e.s.FailOrphanedTranslationRuns(ctx)
	if n := e.count(t, `SELECT COUNT(*) FROM kb_translation_runs WHERE status = 'running'`); n != 0 {
		t.Fatalf("running left: %d", n)
	}
}

var _ = fmt.Sprint
