package caseresults

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// Real-shaped Playwright JSON (reporter "json"): a test.fail() pinned with a defect annotation that
// failed as expected (status expected, expectedStatus failed), the same test after the fix (it passed,
// so Playwright calls it unexpected), and an ordinary failure. Invented names only.
const (
	xfailSpec = `{"title":"TC-SMP-008 edit a record rejects a negative quantity","file":"api/graphql/sample-service/records/edit.spec.ts","tests":[{"projectName":"api-graphql","expectedStatus":"failed","status":"expected",
"annotations":[{"type":"defect","description":"DEF-101 a negative quantity is accepted and stored"}],
"results":[{"status":"failed","duration":120,"error":{"message":"Error: expect(received).toBe(expected)\n\nExpected: 400\nReceived: 200"}}]}]}`
	xpassSpec = `{"title":"TC-SMP-008 edit a record rejects a negative quantity","tests":[{"projectName":"api-graphql","expectedStatus":"failed","status":"unexpected",
"annotations":[{"type":"defect","description":"DEF-101: a negative quantity is accepted and stored"}],
"results":[{"status":"passed","duration":90,"error":{"message":"Error: Expected to fail, but passed."}}]}]}`
	failSpec = `{"title":"TC-SMP-009 delete a record","tests":[{"projectName":"api-rest","expectedStatus":"passed","status":"unexpected","annotations":[],
"results":[{"status":"failed","duration":50,"errors":[{"message":"\u001b[31mTimeoutError\u001b[39m: request to https://user:s3cretpass@svc.example.test/x timed out"}]}]}]}`
	passSpec = `{"title":"TC-SMP-010 list records","tests":[{"projectName":"api-rest","expectedStatus":"passed","status":"expected","annotations":[],"results":[{"status":"passed","duration":10}]}]}`
	// A test.fail() with no defect annotation is not a defect we can name.
	unpinned = `{"title":"TC-SMP-011 x","tests":[{"expectedStatus":"failed","status":"expected","annotations":[],"results":[{"status":"failed"}]}]}`
)

func at() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC) }

func TestParseDefectClassification(t *testing.T) {
	rep, err := Parse(strings.NewReader(wrapD(xfailSpec, failSpec, passSpec, unpinned)), at())
	if err != nil {
		t.Fatal(err)
	}
	x := rep.Defects["TC-SMP-008"]
	if x.Kind != DefectXFail || x.DefectID != "DEF-101" || x.Title != "a negative quantity is accepted and stored" ||
		x.Excerpt != "DEF-101 a negative quantity is accepted and stored" {
		t.Errorf("xfail: %+v", x)
	}
	f := rep.Defects["TC-SMP-009"]
	if f.Kind != DefectFail || f.DefectID != "" || !strings.Contains(f.Excerpt, "TimeoutError") || strings.Contains(f.Excerpt, "\x1b") {
		t.Errorf("fail: %+v", f)
	}
	if _, ok := rep.Defects["TC-SMP-010"]; ok {
		t.Error("a passing test is not a defect")
	}
	if _, ok := rep.Defects["TC-SMP-011"]; ok {
		t.Error("an unpinned test.fail() is not a defect")
	}
	// The results API keeps its meaning: an xfail is a pass, a failure is a fail.
	if rep.Results["TC-SMP-008"] != Pass || rep.Results["TC-SMP-009"] != Fail || rep.Results["TC-SMP-010"] != Pass {
		t.Errorf("results changed: %v", rep.Results)
	}

	rep, err = Parse(strings.NewReader(wrapD(xpassSpec)), at())
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Defects["TC-SMP-008"]
	if p.Kind != DefectXPass || p.DefectID != "DEF-101" || p.Title != "a negative quantity is accepted and stored" || !strings.Contains(p.Excerpt, "Expected to fail, but passed") {
		t.Errorf("xpass: %+v", p)
	}
	if rep.Results["TC-SMP-008"] == Pass {
		t.Log("note: an xpass keeps Playwright's unexpected, which the results API calls fail")
	}
}

func TestDefectAnnotationOnAnyTestAndFirstIDWins(t *testing.T) {
	spec := `{"title":"TC-SMP-012 x","tests":[{"expectedStatus":"failed","status":"expected","annotations":[{"type":"note","description":"DEF-999 not me"},{"type":"defect","description":"see DEF-42 and DEF-43: odd total"}],"results":[{"status":"failed"}]}]}`
	rep, err := Parse(strings.NewReader(wrapD(spec)), at())
	if err != nil {
		t.Fatal(err)
	}
	if d := rep.Defects["TC-SMP-012"]; d.DefectID != "DEF-42" {
		t.Errorf("first id must win: %+v", d)
	}
	long := strings.Repeat("a", 300)
	spec = `{"title":"TC-SMP-013 x","tests":[{"expectedStatus":"failed","status":"expected","annotations":[{"type":"defect","description":"DEF-7 ` + long + `"}],"results":[{"status":"failed"}]}]}`
	rep, _ = Parse(strings.NewReader(wrapD(spec)), at())
	if n := len([]rune(rep.Defects["TC-SMP-013"].Title)); n != MaxDefectTitle {
		t.Errorf("title length %d, want %d", n, MaxDefectTitle)
	}
}

func TestMostSeriousDefectWinsPerCase(t *testing.T) {
	both := `{"title":"TC-SMP-014 x","tests":[` +
		`{"projectName":"a","expectedStatus":"failed","status":"expected","annotations":[{"type":"defect","description":"DEF-5 pinned"}],"results":[{"status":"failed"}]},` +
		`{"projectName":"b","expectedStatus":"passed","status":"unexpected","annotations":[],"results":[{"status":"failed","error":{"message":"boom"}}]}]}`
	rep, _ := Parse(strings.NewReader(wrapD(both)), at())
	if rep.Defects["TC-SMP-014"].Kind != DefectFail {
		t.Errorf("%+v", rep.Defects["TC-SMP-014"])
	}
}

func wrapAt(start string, specs ...string) string {
	return `{"stats":{"startTime":"` + start + `"},"suites":[{"title":"x.spec.ts","specs":[` + strings.Join(specs, ",") + `]}]}`
}

func wrapD(specs ...string) string { return wrapAt("2026-11-25T10:00:00.000Z", specs...) }

func defectDB(t *testing.T) (*db.DB, int) {
	t.Helper()
	d := testdb.New(t)
	res, err := d.Exec(context.Background(), `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES ('SUT','https://github.com/o/r','https://s','sut','main')`)
	if err != nil {
		t.Fatal(err)
	}
	return d, int(res.LastInsertId())
}

func saveDefects(t *testing.T, d *db.DB, pid int, start string, specs ...string) *DefectOutcome {
	t.Helper()
	rep, err := Parse(strings.NewReader(wrapAt(start, specs...)), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	out, err := SaveDefects(context.Background(), d, pid, rep, "run-"+start, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSaveDefectsDedupeClearAndOrder(t *testing.T) {
	d, pid := defectDB(t)
	if out := saveDefects(t, d, pid, "2026-11-25T10:00:00Z", xfailSpec); out.Changed != 1 {
		t.Fatalf("first sight: %+v", out)
	}
	// The same state again: nothing changed, even from a later run.
	if out := saveDefects(t, d, pid, "2026-11-25T11:00:00Z", xfailSpec); out.Changed != 0 {
		t.Fatalf("same state: %+v", out)
	}
	// The fix landed: xpass is a change, stored as xpass and not as a plain fail.
	if out := saveDefects(t, d, pid, "2026-11-25T12:00:00Z", xpassSpec); out.Changed != 1 {
		t.Fatalf("xpass: %+v", out)
	}
	var kind string
	if err := d.QueryRow(context.Background(), `SELECT kind FROM qa_defect_state WHERE product_id = ? AND case_code = 'TC-SMP-008'`, pid).Scan(&kind); err != nil || kind != "xpass" {
		t.Fatalf("kind %q %v", kind, err)
	}
	// An older report cannot overwrite it.
	if out := saveDefects(t, d, pid, "2026-11-25T09:00:00Z", xfailSpec); out.Changed != 0 {
		t.Fatalf("stale: %+v", out)
	}
	// The case passes with no annotation: the state is cleared, so its next failure is news.
	pass := strings.Replace(passSpec, "TC-SMP-010", "TC-SMP-008", 1)
	if out := saveDefects(t, d, pid, "2026-11-25T13:00:00Z", pass); out.Cleared != 1 {
		t.Fatalf("clear: %+v", out)
	}
	var n int
	_ = d.QueryRow(context.Background(), `SELECT COUNT(*) FROM qa_defect_state WHERE product_id = ?`, pid).Scan(&n)
	if n != 0 {
		t.Errorf("%d states left", n)
	}
}

func TestSaveDefectsClipsAndHidesSecrets(t *testing.T) {
	d, pid := defectDB(t)
	big := strings.Repeat("x", 6000) + " password=hunter2hunter2 Authorization: Bearer abcdefghijklmnop gaos_0123456789abcdef0123 https://user:s3cretpass@svc.example.test/x eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.abcdefg"
	spec := `{"title":"TC-SMP-015 x","tests":[{"expectedStatus":"passed","status":"unexpected","results":[{"status":"failed","error":{"message":"` + big + `"}}]}]}`
	rep, _ := Parse(strings.NewReader(wrapD(spec)), at())
	rep.Defects["TC-SMP-015"] = Defect{Kind: DefectFail, Title: "t", Excerpt: "head " + "password=hunter2hunter2 Authorization: Bearer abcdefghijklmnop gaos_0123456789abcdef0123 https://user:s3cretpass@svc.example.test/x eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.abcdefg AKIAABCDEFGHIJKLMNOP"}
	if _, err := SaveDefects(context.Background(), d, pid, rep, "r", func(s string) string { return strings.ReplaceAll(s, "s3cretpass", "***") }); err != nil {
		t.Fatal(err)
	}
	var ex string
	if err := d.QueryRow(context.Background(), `SELECT excerpt FROM qa_defect_state WHERE product_id = ?`, pid).Scan(&ex); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"hunter2", "abcdefghijklmnop", "gaos_0123", "s3cretpass", "eyJhbGci", "AKIAABCDEFGHIJKLMNOP"} {
		if strings.Contains(ex, leak) {
			t.Errorf("the excerpt still holds %q: %s", leak, ex)
		}
	}
	// The long failure message is clipped at parse time.
	if n := len(rep.Defects["TC-SMP-015"].Excerpt); n > MaxExcerpt {
		t.Errorf("excerpt %d bytes", n)
	}
	rep2, _ := Parse(strings.NewReader(wrapD(spec)), at())
	if n := len(rep2.Defects["TC-SMP-015"].Excerpt); n > MaxExcerpt {
		t.Errorf("parsed excerpt %d bytes", n)
	}
}
