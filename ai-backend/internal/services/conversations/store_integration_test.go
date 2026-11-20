package conversations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func mustExec(t *testing.T, d *db.DB, q string, a ...any) {
	t.Helper()
	if _, err := d.Exec(context.Background(), q, a...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func kinds(ms []Message) string {
	var k []string
	for _, m := range ms {
		k = append(k, m.Kind)
	}
	return strings.Join(k, ",")
}

// A case_create conversation as the machine builds it, round by round (AI_CONVERSATIONS.md 11.1).
func TestStoreBuildsTheConversationOnce(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	st := &Store{DB: d, Screen: NewScreen("supersecretvalue")}
	ref := ai.ConvRef{Kind: ai.ConvJob, Ref: "job-1", ProductID: 0, Title: "TC-AUTH-001"}
	save := func(sent []models.Message, reply string, err error) {
		t.Helper()
		if e := st.SaveExchange(ctx, ai.Exchange{Ref: ref, Feature: "case-automation", Provider: "claude-v3", Model: "opus", Sent: sent, Reply: reply, Err: err}); e != nil {
			t.Fatal(e)
		}
	}
	convo := []models.Message{{Role: "user", Kind: "prompt", Content: "rules and the case; token=supersecretvalue"}}
	save(convo, "FILE: a\n```ts\nx\n```", nil)
	convo = append(convo, models.Message{Role: "assistant", Kind: "reply", Content: "FILE: a\n```ts\nx\n```"},
		models.Message{Role: "user", Kind: "refusal", Content: "Your reply was refused: no FILE line"})
	save(convo, "- item\n  - nested\n\n`inline`", nil)
	convo = append(convo, models.Message{Role: "assistant", Kind: "reply", Content: "- item\n  - nested\n\n`inline`"},
		models.Message{Role: "user", Kind: "run_result", Content: "The spec was run against staging and did not pass"})
	save(convo, "", errors.New("upstream 502 Bearer abcdefghijklmnop"))
	save(convo, "third", nil) // the retry after the error: only the reply is new
	convo = append(convo, models.Message{Role: "assistant", Kind: "reply", Content: "third"},
		models.Message{Role: "user", Kind: "feedback", Content: "A reviewer asked for changes: use the other locator"})
	save(convo, "fourth", nil)

	it, err := GetByRef(ctx, d, "job", "job-1")
	if err != nil || it == nil {
		t.Fatalf("get: %v %v", it, err)
	}
	ms, err := Messages(ctx, d, it.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(ms); got != "prompt,reply,refusal,reply,run_result,error,reply,feedback,reply" {
		t.Fatalf("kinds %s", got)
	}
	if it.MessageCount != 9 || it.Model != "opus" || it.Title != "TC-AUTH-001" || it.Feature != "case-automation" {
		t.Errorf("item %+v", it)
	}
	for _, m := range ms {
		if strings.Contains(m.Content, "supersecretvalue") || strings.Contains(m.Content, "abcdefghijklmnop") {
			t.Errorf("secret stored at seq %d: %q", m.Seq, m.Content)
		}
	}
	if newer, _ := Messages(ctx, d, it.ID, 7); len(newer) != 2 || newer[0].Seq != 8 {
		t.Errorf("since_seq: %+v", newer)
	}

	// A resume rebuilds the conversation: one note, the rebuilt messages once, then rounds match again.
	rebuilt := []models.Message{convo[0], {Role: "assistant", Kind: "reply", Content: "render of the file"},
		{Role: "user", Kind: "feedback", Content: "A reviewer asked for changes: again"}}
	save(rebuilt, "fifth", nil)
	rebuilt = append(rebuilt, models.Message{Role: "assistant", Kind: "reply", Content: "fifth"}, models.Message{Role: "user", Kind: "run_result", Content: "failed again"})
	save(rebuilt, "sixth", nil)
	ms, _ = Messages(ctx, d, it.ID, 9)
	if got := kinds(ms); got != "rebuilt,reply,feedback,reply,run_result,reply" {
		t.Fatalf("after resume %s", got)
	}
}

func TestArchiveGuardsAndPrune(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	st := &Store{DB: d, Screen: NewScreen()}
	mustExec(t, d, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'SUT','r1','s1','d1')`)
	mustExec(t, d, `INSERT INTO job_runs (id, type, status, created_at) VALUES ('job-run','case_create','running',UTC_TIMESTAMP()), ('job-done','case_create','completed',UTC_TIMESTAMP())`)
	mustExec(t, d, `INSERT INTO kb_syncs (id, product_id, status, trigger_kind, started_at, ai_status) VALUES (7,1,'succeeded','manual',UTC_TIMESTAMP(),'pending')`)
	mustExec(t, d, `INSERT INTO pw_runs (product_id, source, run_ref, job_id, started_at) VALUES (1,'case-create','job-done','job-done',UTC_TIMESTAMP(3))`)
	mustExec(t, d, `INSERT INTO ai_call_log (feature, status) VALUES ('case-automation','ok')`)
	for _, r := range []ai.ConvRef{{Kind: "job", Ref: "job-run"}, {Kind: "job", Ref: "job-done"}, {Kind: "sync", Ref: "7", ProductID: 1}, {Kind: "call", Ref: "call-1"}} {
		if err := st.SaveExchange(ctx, ai.Exchange{Ref: r, Sent: []models.Message{{Role: "user", Content: "q"}}, Reply: "a"}); err != nil {
			t.Fatal(err)
		}
	}
	id := func(kind, ref string) int64 {
		it, _ := GetByRef(ctx, d, kind, ref)
		return it.ID
	}
	if err := Archive(ctx, d, id("job", "job-run"), "u1"); !errors.Is(err, ErrActive) {
		t.Errorf("running job: %v", err)
	}
	if err := Archive(ctx, d, id("sync", "7"), "u1"); !errors.Is(err, ErrActive) {
		t.Errorf("sync with a pending summary: %v", err)
	}
	mustExec(t, d, `UPDATE kb_syncs SET ai_status = 'done' WHERE id = 7`)
	for _, c := range [][2]string{{"job", "job-done"}, {"sync", "7"}} {
		if err := Archive(ctx, d, id(c[0], c[1]), "u1"); err != nil {
			t.Errorf("archive %v: %v", c, err)
		}
	}
	if err := Archive(ctx, d, id("sync", "7"), "u1"); !errors.Is(err, ErrAlreadyArchived) {
		t.Errorf("twice: %v", err)
	}
	if err := Archive(ctx, d, 999, "u1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent: %v", err)
	}
	live, n, _ := List(ctx, d, Filter{Page: 1, Size: 25})
	arch, na, _ := List(ctx, d, Filter{Archived: true, Page: 1, Size: 25})
	if n != 2 || na != 2 || len(live) != 2 || len(arch) != 2 || arch[0].ArchivedAt == nil {
		t.Fatalf("lists: %d %d", n, na)
	}
	if one, _, _ := List(ctx, d, Filter{Q: "call-1", Page: 1, Size: 25}); len(one) != 1 {
		t.Errorf("q filter %d", len(one))
	}

	totals, cutoff, err := Preview(ctx, d)
	if err != nil || cutoff == nil || totals.Conversations != 2 || totals.Messages != 4 {
		t.Fatalf("preview %+v %v %v", totals, cutoff, err)
	}
	// Archived after the preview (a later millisecond): kept by the prune that used the preview.
	time.Sleep(5 * time.Millisecond)
	if err := Archive(ctx, d, id("call", "call-1"), "u1"); err != nil {
		t.Fatal(err)
	}
	got, ids, err := Prune(ctx, d, *cutoff)
	if err != nil || got.Conversations != 2 || got.Messages != 4 || len(ids) != 2 {
		t.Fatalf("prune %+v %v %v", got, ids, err)
	}
	if it, _ := GetByRef(ctx, d, "call", "call-1"); it == nil || it.ArchivedAt == nil {
		t.Error("a conversation archived after the preview was pruned")
	}
	var msgs, jobs, runs, calls int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM ai_messages`).Scan(&msgs)
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs`).Scan(&jobs)
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM pw_runs`).Scan(&runs)
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM ai_call_log`).Scan(&calls)
	if msgs != 4 || jobs != 2 || runs != 1 || calls != 1 {
		t.Errorf("after prune: messages %d jobs %d runs %d call log %d", msgs, jobs, runs, calls)
	}
	if err := Unarchive(ctx, d, id("call", "call-1")); err != nil {
		t.Fatal(err)
	}
	if err := Unarchive(ctx, d, id("call", "call-1")); !errors.Is(err, ErrNotArchived) {
		t.Errorf("unarchive twice: %v", err)
	}
	if err := Audit(ctx, d, "prune", "u1", "admin", "", 0, map[string]any{"conversations": 2}); err != nil {
		t.Fatal(err)
	}
	if log, err := AuditLog(ctx, d, true, 10); err != nil || len(log) != 1 || log[0].Action != "prune" {
		t.Errorf("audit %+v %v", log, err)
	}
	// New activity on an archived conversation lists it again.
	_ = Archive(ctx, d, id("call", "call-1"), "u1")
	_ = st.SaveExchange(ctx, ai.Exchange{Ref: ai.ConvRef{Kind: "call", Ref: "call-1"}, Sent: []models.Message{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a"}, {Role: "user", Content: "q2"}}, Reply: "a2"})
	if it, _ := GetByRef(ctx, d, "call", "call-1"); it.ArchivedAt != nil || it.MessageCount != 4 {
		t.Errorf("new activity: %+v", it)
	}
}
