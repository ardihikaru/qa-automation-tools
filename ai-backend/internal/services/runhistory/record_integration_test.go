package runhistory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/caseresults"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

func TestRecordIntegration(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	if _, err := d.Exec(ctx, `INSERT INTO products (id, name, repo_url, staging_url, root_dir) VALUES (1,'HIS','r1','s1','d1')`); err != nil {
		t.Fatal(err)
	}
	const report = `{"stats":{"startTime":"2026-11-14T10:00:00.123Z","duration":2500.4},"suites":[{"specs":[
	 {"title":"TC-AUTH-001 a","tests":[{"status":"expected","results":[{"duration":1200}]}]},
	 {"title":"TC-AUTH-002 b","tests":[{"status":"skipped","results":[{"duration":0}]}]}]}]}`
	rep, err := caseresults.Parse(strings.NewReader(report), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	in := RunIn{ProductID: 1, Source: SourceCI, RunRef: "ci-1"}
	id, err := Record(ctx, d, in, rep)
	if err != nil || id == 0 {
		t.Fatalf("record: %d %v", id, err)
	}
	again, err := Record(ctx, d, in, rep)
	if err != nil || again != id {
		t.Fatalf("re-recording the same report must return the same id: %d vs %d (%v)", again, id, err)
	}
	var runs, cases int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM pw_runs`).Scan(&runs)
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM pw_run_cases`).Scan(&cases)
	if runs != 1 || cases != 2 {
		t.Fatalf("runs=%d cases=%d", runs, cases)
	}
	var started time.Time
	var dur int64
	var expected, skipped, n int
	if err := d.QueryRow(ctx, `SELECT started_at, duration_ms, expected, skipped, cases FROM pw_runs WHERE id = ?`, id).Scan(&started, &dur, &expected, &skipped, &n); err != nil {
		t.Fatal(err)
	}
	if started.Nanosecond() != 123_000_000 || dur != 2500 || expected != 1 || skipped != 1 || n != 2 {
		t.Errorf("row: %v %d %d %d %d", started, dur, expected, skipped, n)
	}
	var res string
	var cms int64
	_ = d.QueryRow(ctx, `SELECT result, duration_ms FROM pw_run_cases WHERE run_id = ? AND case_code = 'TC-AUTH-001'`, id).Scan(&res, &cms)
	if res != "pass" || cms != 1200 {
		t.Errorf("case row %s %d", res, cms)
	}

	// Another start (a second executor run of a build) is a new run; a run index is kept.
	rep.StartedAt = rep.StartedAt.Add(time.Minute)
	idx := 2
	id2, err := Record(ctx, d, RunIn{ProductID: 1, Source: SourceCaseCreate, RunRef: "job-1", JobID: "job-1", CaseCode: "TC-AUTH-001", RunIndex: &idx}, rep)
	if err != nil || id2 == id {
		t.Fatalf("second run: %d %v", id2, err)
	}
	var gotIdx int
	_ = d.QueryRow(ctx, `SELECT run_index FROM pw_runs WHERE id = ?`, id2).Scan(&gotIdx)
	if gotIdx != 2 {
		t.Errorf("run_index %d", gotIdx)
	}
	// A different source with the same ref and start is its own delivery.
	if id3, err := Record(ctx, d, RunIn{ProductID: 1, Source: SourceUpload, RunRef: "ci-1"}, rep); err != nil || id3 == id || id3 == id2 {
		t.Fatalf("upload: %d %v", id3, err)
	}
	if _, err := Record(ctx, d, RunIn{Source: SourceCI}, rep); err == nil {
		t.Error("missing product must fail")
	}
}
