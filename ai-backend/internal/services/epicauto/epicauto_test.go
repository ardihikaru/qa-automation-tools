package epicauto

import (
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

func job(id, status string, mod func(*JobFacts)) *JobFacts {
	j := &JobFacts{ID: id, Status: status, DoneAt: t0}
	if mod != nil {
		mod(j)
	}
	return j
}

func TestClassifyEveryOutcome(t *testing.T) {
	cases := []struct {
		name string
		c    Case
		want State
	}{
		{"no job, eligible", Case{Eligible: true}, Pending},
		{"no job, not eligible", Case{}, Skipped},
		{"queued", Case{Job: job("a", "queued", nil)}, Active},
		{"running", Case{Job: job("a", "running", nil)}, Active},
		{"awaiting approval", Case{Job: job("a", "running", func(j *JobFacts) { j.AwaitingApproval = true })}, AwaitingApproval},
		{"completed with PR", Case{Job: job("a", "completed", func(j *JobFacts) { j.PRURL = "https://x/pr/1" })}, PR},
		{"completed without PR", Case{Job: job("a", "completed", nil)}, Failed},
		{"rejected", Case{Job: job("a", "failed", func(j *JobFacts) { j.Err = "rejected by ardi" })}, Declined},
		{"expired", Case{Job: job("a", "failed", func(j *JobFacts) { j.Err = "approval expired" })}, Declined},
		{"too many changes", Case{Job: job("a", "failed", func(j *JobFacts) { j.Err = "changes were requested too many times (2)" })}, Declined},
		{"orphan after approval", Case{Job: job("a", "failed", func(j *JobFacts) {
			j.Err = "backend restarted; it had already passed the approval step (rejected)"
		})}, Declined},
		{"technical", Case{Job: job("a", "failed", func(j *JobFacts) { j.Err = "the spec did not pass after 3 repairs" })}, Failed},
		{"stopped waiting", Case{Job: job("a", "failed", func(j *JobFacts) { j.Err = "stopped while waiting for approval" })}, Failed},
		{"failed after push", Case{Job: job("a", "failed", func(j *JobFacts) { j.Err = "gh: boom"; j.PushedOrPR = true })}, NeedsPerson},
		{"unknown status", Case{Job: job("a", "weird", nil)}, Failed},
	}
	for _, c := range cases {
		if got := Classify(c.c); got != c.want {
			t.Errorf("%s: Classify = %s, want %s", c.name, got, c.want)
		}
	}
}

func pend(codes ...string) []Case {
	var out []Case
	for _, c := range codes {
		out = append(out, Case{Code: c, Eligible: true})
	}
	return out
}

func TestPlanFillsTwoSlotsInOrder(t *testing.T) {
	d := Plan(pend("TC-A-001", "TC-A-002", "TC-A-003"))
	if !reflect.DeepEqual(d.Enqueue, []string{"TC-A-001", "TC-A-002"}) || d.Status != "" {
		t.Fatalf("Plan = %+v, want to enqueue the first two", d)
	}
	cs := pend("TC-A-001", "TC-A-002", "TC-A-003")
	cs[0].Job = job("j1", "running", nil)
	d = Plan(cs)
	if !reflect.DeepEqual(d.Enqueue, []string{"TC-A-002"}) {
		t.Fatalf("with one active, Plan enqueues %v, want just the next one", d.Enqueue)
	}
	cs[1].Job = job("j2", "queued", nil)
	if d = Plan(cs); len(d.Enqueue) != 0 {
		t.Fatalf("with two active, Plan enqueues %v, want none", d.Enqueue)
	}
}

func TestPlanCompletesOnlyWhenNothingIsPendingOrActive(t *testing.T) {
	cs := []Case{
		{Code: "a", Job: job("1", "completed", func(j *JobFacts) { j.PRURL = "u" })},
		{Code: "b", Job: job("2", "failed", func(j *JobFacts) { j.Err = "rejected" })},
		{Code: "c"}, // skipped
	}
	if d := Plan(cs); d.Status != BatchCompleted {
		t.Fatalf("Plan = %+v, want completed", d)
	}
	cs[2].Eligible = true
	if d := Plan(cs); d.Status != "" || len(d.Enqueue) != 1 {
		t.Fatalf("with a pending case Plan = %+v, want it to enqueue, not complete", d)
	}
}

func TestPlanStopsAfterThreeConsecutiveTechnicalFailures(t *testing.T) {
	fail := func(id string, min int) *JobFacts {
		return job(id, "failed", func(j *JobFacts) { j.Err = "boom"; j.DoneAt = t0.Add(time.Duration(min) * time.Minute) })
	}
	declined := func(id string, min int) *JobFacts {
		return job(id, "failed", func(j *JobFacts) { j.Err = "rejected"; j.DoneAt = t0.Add(time.Duration(min) * time.Minute) })
	}
	pr := func(id string, min int) *JobFacts {
		return job(id, "completed", func(j *JobFacts) { j.PRURL = "u"; j.DoneAt = t0.Add(time.Duration(min) * time.Minute) })
	}
	more := Case{Code: "z", Eligible: true}

	// three failures in a row
	cs := []Case{{Code: "a", Job: fail("1", 1)}, {Code: "b", Job: fail("2", 2)}, {Code: "c", Job: fail("3", 3)}, more}
	if d := Plan(cs); d.Status != BatchStopped || d.Note != "3 consecutive failures" || len(d.Enqueue) != 0 {
		t.Fatalf("Plan = %+v, want stopped after 3 failures with nothing enqueued", d)
	}
	// a declined one in between neither counts nor resets
	cs = []Case{{Code: "a", Job: fail("1", 1)}, {Code: "b", Job: declined("2", 2)}, {Code: "c", Job: fail("3", 3)}, {Code: "d", Job: fail("4", 4)}, more}
	if d := Plan(cs); d.Status != BatchStopped {
		t.Fatalf("Plan = %+v, want stopped: a declined job must not reset the streak", d)
	}
	// two failures are not enough
	cs = []Case{{Code: "a", Job: fail("1", 1)}, {Code: "b", Job: fail("2", 2)}, more}
	if d := Plan(cs); d.Status != "" || len(d.Enqueue) != 1 {
		t.Fatalf("Plan = %+v, want to carry on after 2 failures", d)
	}
	// a PR in the middle resets the streak (newest first: fail, fail, pr, fail)
	cs = []Case{{Code: "a", Job: fail("1", 1)}, {Code: "b", Job: pr("2", 2)}, {Code: "c", Job: fail("3", 3)}, {Code: "d", Job: fail("4", 4)}, more}
	if d := Plan(cs); d.Status != "" {
		t.Fatalf("Plan = %+v, want no stop: a PR resets the streak", d)
	}
}

func TestPlanStopsAtOnceWhenACaseNeedsAPerson(t *testing.T) {
	cs := []Case{
		{Code: "a", Job: job("1", "failed", func(j *JobFacts) { j.Err = "gh: boom"; j.PushedOrPR = true })},
		{Code: "b", Eligible: true},
	}
	d := Plan(cs)
	if d.Status != BatchStopped || d.Note != "needs a person: a" || len(d.Enqueue) != 0 {
		t.Fatalf("Plan = %+v, want an immediate stop naming the case", d)
	}
}
