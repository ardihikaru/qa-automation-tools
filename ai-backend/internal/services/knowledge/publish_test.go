package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// draftChat answers the ID to EN draft calls: reply gets the source text.
type draftChat struct {
	mu     sync.Mutex
	calls  int
	models []string
	convs  []ai.ConvRef
	reply  func(text string, n int) (string, error)
}

func (f *draftChat) Configured() bool         { return true }
func (f *draftChat) LastUsage() ai.ChatResult { return ai.ChatResult{} }
func (f *draftChat) ChatMessages(_ context.Context, msgs []models.Message, o ai.ChatOpts) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.models = append(f.models, o.Model)
	if o.Conversation != nil {
		f.convs = append(f.convs, *o.Conversation)
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(msgs[len(msgs)-1].Content), &in); err != nil {
		return "", err
	}
	return f.reply(in.Text, f.calls)
}

func jsonReply(lang, text string) string {
	b, _ := json.Marshal(map[string]string{"source_lang": lang, "text": text})
	return string(b)
}

type pubEnv struct {
	s    *Syncer
	chat *draftChat
	repo *fakeRepo
	pid  int
}

func newPubEnv(t *testing.T, model string) *pubEnv {
	t.Helper()
	d := testdb.New(t)
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{}
	repo.set("1111111111111111111111111111111111111111", map[string]string{"docs/modules/A-core.md": modA, "docs/test-catalog/svc/out.md": epic})
	s := &Syncer{DB: d, AI: &fakeAI{}, ReadTree: repo.read, syncTimeout: time.Minute, aiTimeout: time.Minute}
	chat := &draftChat{reply: func(text string, _ int) (string, error) { return jsonReply("en", text), nil }}
	s.SetDraftTranslator(NewTranslator(context.Background(), s, chat, TranslatorConfig{Model: model}))
	e := &pubEnv{s: s, chat: chat, repo: repo, pid: pid}
	e.sync(t)
	return e
}

func (e *pubEnv) sync(t *testing.T) {
	t.Helper()
	id, err := e.s.Start(context.Background(), e.pid, TriggerManual, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitSync(t, e.s.DB, id); r.status != "succeeded" {
		t.Fatalf("sync: %+v", r)
	}
}

func (e *pubEnv) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.s.DB.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *pubEnv) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.s.DB.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}

func story(code, text, lang string) *StoryBody {
	return &StoryBody{Lang: lang, Code: code, EpicCode: "OUT", Text: text, FeatureCodes: []string{"A01.01"}}
}

func tcase(code, storyCode, scenario, lang string) *TestCaseBody {
	return &TestCaseBody{Lang: lang, Code: code, EpicCode: "OUT", StoryCode: storyCode, Scenario: scenario, Polarity: "+", Level: "API", Priority: "P1", Status: "planned"}
}

// ---------------------------------------------------------------- item 7: ID to EN

func TestSaveDraftIDToEN(t *testing.T) {
	e := newPubEnv(t, "claude-haiku-5-5")
	ctx := context.Background()
	e.chat.reply = func(text string, _ int) (string, error) {
		return jsonReply("id", "As a patient I want to search for `Daftar Pasien`"), nil
	}
	it, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S1", "Sebagai pasien saya ingin mencari `Daftar Pasien`", "id"))
	if err != nil {
		t.Fatal(err)
	}
	if it["text"] != "As a patient I want to search for `Daftar Pasien`" {
		t.Fatalf("English must be stored in the normal column: %v", it["text"])
	}
	if e.chat.models[0] != "claude-haiku-5-5" || e.chat.convs[0].Kind != ai.ConvTranslation {
		t.Fatalf("model %v conv %+v", e.chat.models, e.chat.convs)
	}
	var txt, state, src, hash string
	if err := e.s.DB.QueryRow(ctx, `SELECT text, state, source_lang, src_hash FROM kb_translations WHERE product_id = ? AND entity_type = 'story' AND code = 'L-S1' AND field = 'text' AND lang = 'id'`, e.pid).Scan(&txt, &state, &src, &hash); err != nil {
		t.Fatal(err)
	}
	if txt != "Sebagai pasien saya ingin mencari `Daftar Pasien`" || state != "original" || src != "id" || hash != textHash("As a patient I want to search for `Daftar Pasien`") {
		t.Fatalf("original row: %q %s %s", txt, state, src)
	}
	// The ID view shows the typed text as an original (never machine, never stale).
	view, err := e.s.Entity(WithLang(ctx, LangID), e.pid, TypeStory, "L-S1")
	if err != nil {
		t.Fatal(err)
	}
	f := view["i18n"].(Item)["fields"].(Item)["text"].(Item)
	if f["state"] != "original" || f["text"] != txt {
		t.Fatalf("i18n %+v", f)
	}

	// A test case: the scenario is converted the same way; mixed text too.
	e.chat.reply = func(string, int) (string, error) { return jsonReply("mixed", "Given a patient, when saving, then it is saved"), nil }
	c, err := e.s.SaveDraft(ctx, e.pid, TypeTestCase, "", "", tcase("L-C1", "L-S1", "Given pasien, ketika simpan, then tersimpan", "id"))
	if err != nil || c["scenario"] != "Given a patient, when saving, then it is saved" {
		t.Fatalf("case: %v %v", c, err)
	}
	if e.count(t, `SELECT COUNT(*) FROM kb_translations WHERE product_id = ? AND code = 'L-C1' AND field = 'scenario' AND source_lang = 'mixed' AND state = 'original'`, e.pid) != 1 {
		t.Fatal("mixed original not kept")
	}

	// Editing the draft in English (the model says en): stored as typed; the old original reads stale.
	e.chat.reply = func(text string, _ int) (string, error) { return jsonReply("en", text), nil }
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "L-S1", "", story("", "As a patient I want to find myself", "en")); err != nil {
		t.Fatal(err)
	}
	view, _ = e.s.Entity(WithLang(ctx, LangID), e.pid, TypeStory, "L-S1")
	if f := view["i18n"].(Item)["fields"].(Item)["text"].(Item); f["state"] != "stale" {
		t.Fatalf("an edited draft must make its ID original stale: %+v", f)
	}
}

func TestSaveDraftConversionFailures(t *testing.T) {
	e := newPubEnv(t, "claude-haiku-5-5")
	ctx := context.Background()
	local := func() int { return e.count(t, `SELECT COUNT(*) FROM kb_stories WHERE product_id = ? AND origin = 'local'`, e.pid) }

	e.chat.reply = func(string, int) (string, error) { return "", errors.New("ai-manager is down") }
	_, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S1", "Sebagai pasien", "id"))
	var de *DraftTranslateError
	if !errors.As(err, &de) || local() != 0 {
		t.Fatalf("a failed call must fail the save and store nothing: %v (%d rows)", err, local())
	}

	// A broken reply is retried once, then fails.
	e.chat.calls = 0
	e.chat.reply = func(string, int) (string, error) { return "not json", nil }
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S1", "Sebagai pasien", "id")); !errors.As(err, &de) || e.chat.calls != 2 || local() != 0 {
		t.Fatalf("broken reply: %v calls %d", err, e.chat.calls)
	}
	// A reply that drops a code or a Given/When/Then is refused (after the retry).
	e.chat.reply = func(string, int) (string, error) { return jsonReply("id", "Given a patient"), nil }
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeTestCase, "", "", tcase("L-C1", "US-OUT-01", "Given pasien, when simpan, then TC-OUT-001 ok", "id")); !errors.As(err, &de) {
		t.Fatalf("lost token: %v", err)
	}
	// Recovery: the second attempt of the retry is valid.
	e.chat.calls = 0
	e.chat.reply = func(_ string, n int) (string, error) {
		if n == 1 {
			return "oops", nil
		}
		return jsonReply("id", "As a patient"), nil
	}
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S1", "Sebagai pasien", "id")); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestSaveDraftNoConversion(t *testing.T) {
	e := newPubEnv(t, "claude-haiku-5-5")
	ctx := context.Background()
	// A machine caller (no lang) never reaches the model.
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S1", "x", "")); err != nil || e.chat.calls != 0 {
		t.Fatalf("no lang: %v calls %d", err, e.chat.calls)
	}
	// lang=en is sent (a hint only); source en stores the typed text and no translation row.
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S2", "plain English", "en")); err != nil || e.chat.calls != 1 {
		t.Fatalf("en: %v calls %d", err, e.chat.calls)
	}
	if e.count(t, `SELECT COUNT(*) FROM kb_translations WHERE product_id = ? AND code = 'L-S2'`, e.pid) != 0 {
		t.Fatal("english text must not leave an original row")
	}
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S3", "x", "fr")); err == nil {
		t.Fatal("unknown lang accepted")
	}
	// Deleting a draft removes its translations.
	e.chat.reply = func(string, int) (string, error) { return jsonReply("id", "As a patient"), nil }
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", story("L-S4", "Sebagai pasien", "id")); err != nil {
		t.Fatal(err)
	}
	if err := e.s.DeleteDraft(ctx, e.pid, TypeStory, "L-S4"); err != nil || e.count(t, `SELECT COUNT(*) FROM kb_translations WHERE product_id = ? AND code = 'L-S4'`, e.pid) != 0 {
		t.Fatalf("delete: %v", err)
	}

	// Translation off: lang=id is refused, English still saves.
	off := newPubEnv(t, "")
	if _, err := off.s.SaveDraft(ctx, off.pid, TypeStory, "", "", story("L-S1", "Sebagai", "id")); !errors.Is(err, ErrTranslationDisabled) {
		t.Fatalf("disabled: %v", err)
	}
	if _, err := off.s.SaveDraft(ctx, off.pid, TypeStory, "", "", story("L-S1", "plain", "en")); err != nil || off.chat.calls != 0 {
		t.Fatalf("disabled en: %v", err)
	}
}

// ---------------------------------------------------------------- item 9: publications

type fakePR struct {
	state map[string]string
	calls int
}

func (f *fakePR) PRStatus(_ context.Context, _ string, url string) (repopool.PRState, error) {
	f.calls++
	st, ok := f.state[url]
	if !ok {
		return repopool.PRState{}, errors.New("unknown pr")
	}
	return repopool.PRState{State: st}, nil
}

func TestPublicationLifecycleAndReconcile(t *testing.T) {
	e := newPubEnv(t, "")
	ctx := context.Background()
	user := "00000000-0000-0000-0000-000000000001"
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", user, story("L-S1", "draft story", "")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*TestCaseBody{tcase("L-C1", "L-S1", "draft case on draft story", ""), tcase("L-C2", "US-OUT-01", "draft case on repo story", "")} {
		if _, err := e.s.SaveDraft(ctx, e.pid, TypeTestCase, "", user, c); err != nil {
			t.Fatal(err)
		}
	}
	// Blockers: a case with no story, and one on a draft story that is in another epic.
	e.exec(t, `INSERT INTO kb_epics (product_id, code, title, origin, content_hash) VALUES (?, 'OTHER', 'o', 'local', 'x')`, e.pid)
	other := story("L-SO", "other epic story", "")
	other.EpicCode = "OTHER"
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "", "", other); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeTestCase, "", "", tcase("L-C3", "L-SO", "on another epic's draft story", "")); err != nil {
		t.Fatal(err)
	}
	set, err := e.s.CollectDrafts(ctx, e.pid, "OUT")
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Stories) != 1 || len(set.Cases) != 2 || len(set.Blockers) != 1 || set.Blockers[0].Code != "L-C3" {
		t.Fatalf("set: %+v", set)
	}
	if _, err := e.s.CollectDrafts(ctx, e.pid, "NOPE"); err == nil {
		t.Fatal("unknown epic accepted")
	}

	// Queue: locks the drafts, one active publish per product.
	if err := e.s.QueuePublication(ctx, e.pid, "job-1", user, set); err != nil {
		t.Fatal(err)
	}
	if id, _ := e.s.ActivePublishJob(ctx, e.pid); id != "job-1" {
		t.Fatalf("active job %q", id)
	}
	var ce *ConflictError
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeStory, "L-S1", "", story("", "edited", "")); !errors.As(err, &ce) {
		t.Fatalf("edit while queued: %v", err)
	}
	if err := e.s.DeleteDraft(ctx, e.pid, TypeTestCase, "L-C1"); !errors.As(err, &ce) {
		t.Fatalf("delete while queued: %v", err)
	}
	if again, _ := e.s.CollectDrafts(ctx, e.pid, "OUT"); !again.Empty() {
		t.Fatalf("queued drafts offered again: %+v", again)
	}
	mine, err := e.s.JobDrafts(ctx, e.pid, "OUT", "job-1")
	if err != nil || len(mine.Stories) != 1 || len(mine.Cases) != 2 {
		t.Fatalf("job drafts: %+v %v", mine, err)
	}
	if err := e.s.PublicationRunning(ctx, "job-1"); err != nil {
		t.Fatal(err)
	}
	taken, _ := e.s.TakenCodes(ctx, e.pid, "OUT", "job-1")
	if strings.Join(taken, ",") == "" {
		t.Fatal("repo rows must count as taken")
	}
	pairs := []CodePair{{TypeStory, "L-S1", "US-OUT-03"}, {TypeTestCase, "L-C1", "TC-OUT-003"}, {TypeTestCase, "L-C2", "TC-OUT-004"}}
	if err := e.s.PublicationAssign(ctx, "job-1", "qa-auto/kb-out-202611271200", pairs); err != nil {
		t.Fatal(err)
	}
	if t2, _ := e.s.TakenCodes(ctx, e.pid, "OUT", "job-other"); !strings.Contains(strings.Join(t2, ","), "TC-OUT-004") {
		t.Fatalf("assigned codes must be taken by other jobs: %v", t2)
	}
	if t2, _ := e.s.TakenCodes(ctx, e.pid, "OUT", "job-1"); strings.Contains(strings.Join(t2, ","), "TC-OUT-004") {
		t.Fatalf("a job must not count its own codes: %v", t2)
	}
	pubs, _ := e.s.Publications(ctx, e.pid)
	if len(pubs) != 1 || pubs[0].State != PubRunning || pubs[0].JobID != "job-1" || pubs[0].EpicCode != "OUT" || pubs[0].Codes["L-C2"] != "TC-OUT-004" || pubs[0].Branch == "" {
		t.Fatalf("publications: %+v", pubs)
	}
	if err := e.s.PublicationOpened(ctx, "job-1", "https://github.com/o/r/pull/7"); err != nil {
		t.Fatal(err)
	}
	if id, _ := e.s.ActivePublishJob(ctx, e.pid); id != "" {
		t.Fatalf("pr_open is not active: %q", id)
	}
	// Edits are allowed again (pr_open) but the drafts are not offered for another publish.
	if again, _ := e.s.CollectDrafts(ctx, e.pid, "OUT"); !again.Empty() {
		t.Fatal("drafts of an open PR offered again")
	}

	// Data keyed by the draft code, plus history that must stay.
	e.exec(t, `INSERT INTO kb_case_results (product_id, case_code, result, run_at, run_ref, source) VALUES (?, 'L-C1', 'pass', UTC_TIMESTAMP(), 'r', 'case-run')`, e.pid)
	e.exec(t, `INSERT INTO kb_case_verifications (product_id, case_code, verdict, verified_by, verified_at) VALUES (?, 'L-C1', 'valid', ?, UTC_TIMESTAMP())`, e.pid, user)
	e.exec(t, `INSERT INTO kb_case_verification_events (product_id, case_code, verdict, verified_by, verified_at) VALUES (?, 'L-C1', 'valid', ?, UTC_TIMESTAMP())`, e.pid, user)
	e.exec(t, `INSERT INTO kb_annotations (product_id, entity_type, entity_code, note) VALUES (?, 'test_case', 'L-C1', 'remember')`, e.pid)
	e.exec(t, `INSERT INTO kb_translations (product_id, entity_type, code, field, lang, text, src_hash, source_lang, state, model, translated_at) VALUES (?, 'story', 'L-S1', 'text', 'id', 'cerita', 'h', 'id', 'original', 'm', UTC_TIMESTAMP())`, e.pid)
	e.exec(t, `INSERT INTO job_runs (id, type, status, payload) VALUES ('case-job', 'case_run', 'completed', JSON_OBJECT('case_code', 'L-C1'))`)
	e.exec(t, `UPDATE kb_test_cases SET story_code = 'L-S1' WHERE product_id = ? AND code = 'L-C3'`, e.pid) // a leftover draft on the published story

	pr := &fakePR{state: map[string]string{"https://github.com/o/r/pull/7": "OPEN"}}
	e.s.ReconcilePublications(ctx, e.pid, pr)
	if pubs, _ = e.s.Publications(ctx, e.pid); pubs[0].State != PubPROpen {
		t.Fatalf("open PR: %+v", pubs)
	}
	// Merged, but the repo codes are not synced yet: nothing moves.
	pr.state["https://github.com/o/r/pull/7"] = "MERGED"
	e.s.ReconcilePublications(ctx, e.pid, pr)
	if pubs, _ = e.s.Publications(ctx, e.pid); pubs[0].State != PubPROpen || e.count(t, `SELECT COUNT(*) FROM kb_stories WHERE product_id = ? AND code = 'L-S1'`, e.pid) != 1 {
		t.Fatalf("merged but unsynced: %+v", pubs)
	}
	// The repo now holds the new codes (as a reviewer may have edited the story text).
	e.repo.set("2222222222222222222222222222222222222222", map[string]string{"docs/modules/A-core.md": modA, "docs/test-catalog/svc/out.md": epic +
		"- **US-OUT-03** (A01.01) — reviewer edited story\n" +
		"| ☐ | TC-OUT-003 | US-OUT-03 | draft case on draft story | + | API | P1 | planned | | |\n" +
		"| ☐ | TC-OUT-004 | US-OUT-01 | draft case on repo story | + | API | P1 | planned | | |\n"})
	e.sync(t)
	e.s.ReconcilePublications(ctx, e.pid, pr)
	if pubs, _ = e.s.Publications(ctx, e.pid); pubs[0].State != PubMerged {
		t.Fatalf("merged: %+v", pubs)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM kb_stories WHERE product_id = ? AND code = 'L-S1'`,
		`SELECT COUNT(*) FROM kb_test_cases WHERE product_id = ? AND code IN ('L-C1', 'L-C2')`,
		`SELECT COUNT(*) FROM kb_case_results WHERE product_id = ? AND case_code = 'L-C1'`,
		`SELECT COUNT(*) FROM kb_annotations WHERE product_id = ? AND entity_code = 'L-C1'`,
		`SELECT COUNT(*) FROM kb_translations WHERE product_id = ? AND code = 'L-S1'`,
	} {
		if n := e.count(t, q, e.pid); n != 0 {
			t.Errorf("%s -> %d, want 0", q, n)
		}
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM kb_case_results WHERE product_id = ? AND case_code = 'TC-OUT-003'`,
		`SELECT COUNT(*) FROM kb_case_verifications WHERE product_id = ? AND case_code = 'TC-OUT-003'`,
		`SELECT COUNT(*) FROM kb_case_verification_events WHERE product_id = ? AND case_code = 'TC-OUT-003'`,
		`SELECT COUNT(*) FROM kb_annotations WHERE product_id = ? AND entity_type = 'test_case' AND entity_code = 'TC-OUT-003'`,
		`SELECT COUNT(*) FROM kb_translations WHERE product_id = ? AND entity_type = 'story' AND code = 'US-OUT-03' AND state = 'original'`,
		`SELECT COUNT(*) FROM kb_test_cases WHERE product_id = ? AND code = 'L-C3' AND story_code = 'US-OUT-03'`,
	} {
		if n := e.count(t, q, e.pid); n != 1 {
			t.Errorf("%s -> %d, want 1", q, n)
		}
	}
	if e.count(t, `SELECT COUNT(*) FROM job_runs WHERE id = 'case-job' AND JSON_UNQUOTE(JSON_EXTRACT(payload, '$.case_code')) = 'L-C1'`) != 1 {
		t.Error("job_runs must keep the L- code")
	}
	// The re-keyed original no longer matches the reviewer's English: it reads stale.
	view, _ := e.s.Entity(WithLang(ctx, LangID), e.pid, TypeStory, "US-OUT-03")
	if f := view["i18n"].(Item)["fields"].(Item)["text"].(Item); f["state"] != "stale" || f["text"] != "cerita" {
		t.Fatalf("re-keyed original: %+v", f)
	}
	if got := e.s.MovedTo(ctx, e.pid, TypeTestCase, "L-C1"); got != "TC-OUT-003" {
		t.Fatalf("MovedTo %q", got)
	}
	if codes, _ := e.s.JobCodes(ctx, "job-1"); codes["L-S1"] != "US-OUT-03" || len(codes) != 3 {
		t.Fatalf("job codes %+v", codes)
	}
	calls := pr.calls
	e.s.ReconcilePublications(ctx, e.pid, pr)
	if pr.calls != calls {
		t.Fatal("a merged publication is asked about again")
	}
}

func TestPublicationClosedReleasesDrafts(t *testing.T) {
	e := newPubEnv(t, "")
	ctx := context.Background()
	if _, err := e.s.SaveDraft(ctx, e.pid, TypeTestCase, "", "", tcase("L-C1", "US-OUT-01", "scenario", "")); err != nil {
		t.Fatal(err)
	}
	set, _ := e.s.CollectDrafts(ctx, e.pid, "OUT")
	_ = e.s.QueuePublication(ctx, e.pid, "job-9", "", set)
	_ = e.s.PublicationAssign(ctx, "job-9", "qa-auto/kb-out-1", []CodePair{{TypeTestCase, "L-C1", "TC-OUT-009"}})
	_ = e.s.PublicationOpened(ctx, "job-9", "https://github.com/o/r/pull/9")
	pr := &fakePR{state: map[string]string{"https://github.com/o/r/pull/9": "CLOSED"}}
	e.s.ReconcilePublications(ctx, e.pid, pr)
	pubs, _ := e.s.Publications(ctx, e.pid)
	if pubs[0].State != PubFailed || pubs[0].Error == "" || len(pubs[0].Codes) != 0 {
		t.Fatalf("closed: %+v", pubs)
	}
	if taken, _ := e.s.TakenCodes(ctx, e.pid, "OUT", ""); strings.Contains(strings.Join(taken, ","), "TC-OUT-009") {
		t.Fatalf("a failed publication must release its code: %v", taken)
	}
	if again, _ := e.s.CollectDrafts(ctx, e.pid, "OUT"); len(again.Cases) != 1 {
		t.Fatalf("draft must be publishable again: %+v", again)
	}
	// A failed queued publication (job could not run) is released the same way.
	_ = e.s.QueuePublication(ctx, e.pid, "job-10", "", again(t, e))
	_ = e.s.PublicationFailed(ctx, "job-10", "sync first")
	if id, _ := e.s.ActivePublishJob(ctx, e.pid); id != "" {
		t.Fatal("failed publication still active")
	}
}

func again(t *testing.T, e *pubEnv) *DraftSet {
	t.Helper()
	s, err := e.s.CollectDrafts(context.Background(), e.pid, "OUT")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
