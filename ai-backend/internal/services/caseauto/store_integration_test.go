package caseauto

import (
	"context"
	"sync"
	"testing"

	"github.com/qa-automation/ai-backend/internal/testdb"
)

// Skipped unless QA_TEST_MYSQL_DSN points at a scratch MySQL server.
func TestStoresAgainstMySQL(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	es := &EventStore{DB: d, Redact: NewRedactor()}

	// Concurrent writers get distinct, gap-free seq numbers.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := es.Add(ctx, "job-1", "info", StepRun, "line Bearer abcdefgh12345678"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	list, err := es.List(ctx, "job-1", 0, 500)
	if err != nil || len(list) != 8 {
		t.Fatalf("%v %v", list, err)
	}
	for i, e := range list {
		if e.Seq != i+1 || e.Message != "line Bearer ***" {
			t.Errorf("event %d: %+v", i, e)
		}
	}
	if tail, _ := es.List(ctx, "job-1", 6, 500); len(tail) != 2 || tail[0].Seq != 7 {
		t.Errorf("since: %+v", tail)
	}

	as := &ApprovalStore{DB: d}
	if err := as.Open(ctx, "job-1", 1, "TC-A-001", "qa-job-1-pr"); err != nil {
		t.Fatal(err)
	}
	if err := as.Open(ctx, "job-1", 1, "TC-A-001", "qa-job-1-pr"); err != nil {
		t.Fatal("Open must be idempotent:", err)
	}
	if err := as.MarkGaosCreated(ctx, "qa-job-1-pr"); err != nil {
		t.Fatal(err)
	}
	if ok, err := as.Answer(ctx, "qa-job-1-pr", ApprovalApproved, "gaos", "fine"); !ok || err != nil {
		t.Fatalf("first answer: %v %v", ok, err)
	}
	if ok, _ := as.Answer(ctx, "qa-job-1-pr", ApprovalRejected, "dashboard", ""); ok {
		t.Fatal("the second answer must not win")
	}
	a, err := as.Latest(ctx, "job-1")
	if err != nil || a == nil || a.Status != ApprovalApproved || a.AnsweredBy != "gaos" || !a.GaosCreated || a.Text != "fine" {
		t.Fatalf("%+v %v", a, err)
	}
	if ids, _ := as.OpenForJobs(ctx); len(ids) != 0 {
		t.Errorf("open jobs %v", ids)
	}
	if _, _, _, err := as.ByExternal(ctx, "qa-nope-pr"); err == nil {
		t.Error("unknown external id found")
	}
}
