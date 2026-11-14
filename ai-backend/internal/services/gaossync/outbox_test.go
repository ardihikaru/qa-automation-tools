package gaossync

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
)

func marker(t *testing.T, d *db.DB, pid int) (set bool, attempts int) {
	t.Helper()
	var at sql.NullTime
	if err := d.QueryRow(context.Background(), `SELECT gaos_push_requested_at, gaos_push_attempts FROM product_settings WHERE product_id = ?`, pid).Scan(&at, &attempts); err != nil {
		t.Fatal(err)
	}
	return at.Valid, attempts
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func idle(s *Syncer) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running) == 0
}

func seedOne(t *testing.T, d *db.DB) int {
	t.Helper()
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	return pid
}

func ageLastSync(t *testing.T, d *db.DB, pid, seconds int) {
	t.Helper()
	exec(t, d, `UPDATE product_settings SET gaos_last_sync_at = UTC_TIMESTAMP() - INTERVAL ? SECOND WHERE product_id = ?`, seconds, pid)
}

func TestBackoffRule(t *testing.T) {
	step := 30 * time.Second
	for _, c := range []struct {
		n    int
		want time.Duration
	}{{0, 0}, {1, 30 * time.Second}, {2, time.Minute}, {19, 9*time.Minute + 30*time.Second}, {20, 10 * time.Minute}, {500, 10 * time.Minute}} {
		if got := backoffFor(c.n, step); got != c.want {
			t.Errorf("attempts %d: got %v want %v", c.n, got, c.want)
		}
	}
}

// (a) the marker is written before the push and a delivered push clears it.
func TestTriggerWritesMarkerBeforePushAndOKClearsIt(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	var duringPush bool
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if n == 1 {
			duringPush, _ = marker(t, d, pid)
		}
		return 200, nil, defaultAnswer(r)
	}
	s.Trigger(pid)
	waitFor(t, "push and clear", func() bool { set, _ := marker(t, d, pid); return !set && idle(s) && len(f.requests()) == 2 })
	if !duringPush {
		t.Fatal("the marker must be persisted before the first request")
	}
}

func TestPartialAlsoClearsTheMarker(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if n == 1 {
			return 200, nil, `{"created":0,"unchanged":0,"rejected":[{"caseId":"TC-AUTH-001","reason":"bad-field"}],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":"","catHead":1}`
		}
		return 200, nil, defaultAnswer(r)
	}
	s.Trigger(pid)
	waitFor(t, "clear", func() bool { set, _ := marker(t, d, pid); return !set && idle(s) && len(f.requests()) >= 1 })
	if st, _, _ := outcome(t, d, pid); st != "partial" {
		t.Fatalf("want partial, got %q", st)
	}
}

// (b) a restart: the marker is in the DB, a brand new Syncer delivers it.
func TestRestartSweeperDeliversTheOwedPush(t *testing.T) {
	f := newFake(t)
	old, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	exec(t, d, `UPDATE product_settings SET gaos_push_requested_at = UTC_TIMESTAMP(6) WHERE product_id = ?`, pid)
	_ = old // the process that died owing the push
	fresh := New(d, f.srv.URL+"/api/v1", goodToken)
	fresh.startDelay = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fresh.StartSweeper(ctx, time.Hour)
	waitFor(t, "delivery after restart", func() bool { set, _ := marker(t, d, pid); return !set && idle(fresh) })
	if got := len(f.requests()); got != 2 {
		t.Fatalf("want 1 pass x 2 requests, got %d", got)
	}
	if st, _, _ := outcome(t, d, pid); st != "ok" {
		t.Fatalf("outcome %q", st)
	}
}

// (c) a failed push keeps the marker and counts attempts; the backoff is honoured; GAOS back = delivered.
func TestFailedPushKeepsMarkerBackoffThenDelivers(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	s.backoffStep = 2 * time.Second
	pid := seedOne(t, d)
	status := 500
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		f.mu.Lock()
		st := status
		f.mu.Unlock()
		if st != 200 {
			return st, nil, `{}`
		}
		return 200, nil, defaultAnswer(r)
	}
	setStatus := func(v int) { f.mu.Lock(); status = v; f.mu.Unlock() }
	ctx := context.Background()

	s.Trigger(pid)
	waitFor(t, "first failure", func() bool { _, a := marker(t, d, pid); return a == 1 && idle(s) })
	if set, _ := marker(t, d, pid); !set {
		t.Fatal("a failed push must keep the marker")
	}
	n := len(f.requests())

	s.sweepOnce(ctx) // inside the backoff (1 attempt x 2 s, last attempt just now)
	time.Sleep(150 * time.Millisecond)
	if len(f.requests()) != n || !idle(s) {
		t.Fatal("the sweeper must not push inside the backoff")
	}

	ageLastSync(t, d, pid, 10)
	setStatus(401)
	s.sweepOnce(ctx)
	waitFor(t, "second failure", func() bool { _, a := marker(t, d, pid); return a == 2 && idle(s) })

	s.sweepOnce(ctx) // attempts 2 -> 4 s backoff, just failed
	time.Sleep(150 * time.Millisecond)
	if !idle(s) {
		t.Fatal("backoff must grow with the attempts")
	}

	ageLastSync(t, d, pid, 10)
	setStatus(200)
	s.sweepOnce(ctx)
	waitFor(t, "delivery", func() bool { set, a := marker(t, d, pid); return !set && a == 0 && idle(s) })
	if st, _, _ := outcome(t, d, pid); st != "ok" {
		t.Fatalf("outcome %q", st)
	}
}

// (d) a trigger arriving during a pass is not lost.
func TestTriggerDuringPassKeepsItsMarkerUntilTheRerun(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	var first sql.NullTime
	_ = d.QueryRow(context.Background(), `SELECT gaos_push_requested_at FROM product_settings WHERE product_id = ?`, pid).Scan(&first)
	var afterFirstPass bool
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		switch n {
		case 1: // the second request arrives while the first pass runs
			var before sql.NullTime
			_ = d.QueryRow(context.Background(), `SELECT gaos_push_requested_at FROM product_settings WHERE product_id = ?`, pid).Scan(&before)
			time.Sleep(5 * time.Millisecond)
			s.Trigger(pid)
			waitFor(t, "second marker", func() bool {
				var now sql.NullTime
				_ = d.QueryRow(context.Background(), `SELECT gaos_push_requested_at FROM product_settings WHERE product_id = ?`, pid).Scan(&now)
				return now.Valid && now.Time.After(before.Time)
			})
			// let the trigger goroutine mark the run dirty
			waitFor(t, "dirty", func() bool { s.mu.Lock(); defer s.mu.Unlock(); st := s.running[pid]; return st != nil && st.dirty })
		case 3: // first request of the re-run: the first pass must have left the marker
			afterFirstPass, _ = marker(t, d, pid)
		}
		return 200, nil, defaultAnswer(r)
	}
	s.Trigger(pid)
	waitFor(t, "both passes", func() bool { set, _ := marker(t, d, pid); return !set && idle(s) && len(f.requests()) == 4 })
	if !afterFirstPass {
		t.Fatal("a request that arrived during a pass must keep its marker past that pass")
	}
}

// (e) a marker on an unflagged (or unconfigured) product is cleared and nothing is sent.
func TestSweeperClearsMarkerOfUnflaggedProduct(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	exec(t, d, `UPDATE product_settings SET gaos_integrated = 0, gaos_push_requested_at = UTC_TIMESTAMP(6), gaos_push_attempts = 3 WHERE product_id = ?`, pid)
	exec(t, d, `UPDATE products SET name = 'other', root_dir = 'other', repo_url = 'https://github.com/o/other' WHERE id = ?`, pid)
	pid2 := seedOne(t, d)
	exec(t, d, `UPDATE product_settings SET gaos_product_id = '', gaos_push_requested_at = UTC_TIMESTAMP(6) WHERE product_id = ?`, pid2)
	s.sweepOnce(context.Background())
	for _, p := range []int{pid, pid2} {
		if set, a := marker(t, d, p); set || a != 0 {
			t.Fatalf("product %d: marker must be cleared (%v, %d)", p, set, a)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if len(f.requests()) != 0 {
		t.Fatal("nothing may be sent for an unflagged product")
	}
}

// (f) Trigger and Retry on an unflagged product, and Retry on an ok product, set no marker.
func TestNoMarkerWhenNothingIsOwed(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	off := seedProduct(t, d, false)
	exec(t, d, `UPDATE products SET name = 'other', root_dir = 'other', repo_url = 'https://github.com/o/other' WHERE id = ?`, off)
	s.Trigger(off)
	s.Retry(off)
	ok := seedOne(t, d)
	exec(t, d, `UPDATE product_settings SET gaos_last_status = 'ok' WHERE product_id = ?`, ok)
	s.Retry(ok)
	time.Sleep(300 * time.Millisecond)
	for _, p := range []int{off, ok} {
		if set, _ := marker(t, d, p); set {
			t.Fatalf("product %d must have no marker", p)
		}
	}
	if len(f.requests()) != 0 {
		t.Fatal("nothing may be sent")
	}
}

// (g) a marker-write failure never fails the caller and the in-memory push still runs.
func TestMarkerWriteFailureStillPushes(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	exec(t, d, `CREATE TRIGGER gaos_no_marker BEFORE UPDATE ON product_settings FOR EACH ROW
IF NEW.gaos_push_requested_at IS NOT NULL AND OLD.gaos_push_requested_at IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'no marker'; END IF`)
	s.Trigger(pid)
	waitFor(t, "push without a marker", func() bool { st, _, _ := outcome(t, d, pid); return st == "ok" && idle(s) })
	if set, _ := marker(t, d, pid); set {
		t.Fatal("the marker write was meant to fail")
	}
}

// (h) the sweeper never duplicates a running push.
func TestSweeperSkipsAProductWhosePushIsRunning(t *testing.T) {
	f := newFake(t)
	f.delay = 200 * time.Millisecond
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	s.Trigger(pid)
	waitFor(t, "first request", func() bool { return len(f.requests()) >= 1 })
	exec(t, d, `UPDATE product_settings SET gaos_push_requested_at = gaos_push_requested_at WHERE product_id = ?`, pid)
	s.sweepOnce(context.Background())
	waitFor(t, "done", func() bool { set, _ := marker(t, d, pid); return !set && idle(s) })
	time.Sleep(100 * time.Millisecond)
	if got := len(f.requests()); got != 2 {
		t.Fatalf("one pass only expected, got %d requests", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.maxIn != 1 {
		t.Fatalf("overlap: %d", f.maxIn)
	}
}

// (i) two quick uploads: at most one running push plus one re-run.
func TestTwoQuickUploadsAtMostOneRerun(t *testing.T) {
	f := newFake(t)
	f.delay = 100 * time.Millisecond
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	s.Trigger(pid)
	s.Trigger(pid)
	s.Trigger(pid)
	waitFor(t, "settled", func() bool { set, _ := marker(t, d, pid); return !set && idle(s) && len(f.requests()) >= 2 })
	time.Sleep(300 * time.Millisecond)
	if got := len(f.requests()); got > 4 {
		t.Fatalf("at most two passes expected, got %d requests", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.maxIn != 1 {
		t.Fatalf("overlap: %d", f.maxIn)
	}
}

// The marker is durable BEFORE Trigger returns to its caller. Found on production: with the settings read
// and the marker write inside the goroutine, a restart ~100 ms after an upload lost the owed push.
func TestTriggerPersistsTheMarkerBeforeItReturns(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	release := make(chan struct{})
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		<-release // hold the push, so only the synchronous part of Trigger can have run
		return 200, nil, defaultAnswer(r)
	}
	s.Trigger(pid)
	if set, _ := marker(t, d, pid); !set {
		t.Fatal("Trigger returned before the push marker was persisted: a restart now would lose the delivery")
	}
	close(release)
	waitFor(t, "the held push to finish and clear the marker", func() bool { set, _ := marker(t, d, pid); return !set && idle(s) })
}

// The level-triggered net: data newer than the last push is owed a push even when no marker survived
// (the process died between "data stored" and "marker written").
func TestSweeperPushesDataNewerThanTheLastPush(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedOne(t, d)
	// A push finished 10 minutes ago, and a result was stored just now with no marker written.
	exec(t, d, `UPDATE product_settings SET gaos_last_sync_at = UTC_TIMESTAMP() - INTERVAL 600 SECOND, gaos_last_status = 'ok' WHERE product_id = ?`, pid)
	exec(t, d, `UPDATE kb_test_cases SET updated_at = UTC_TIMESTAMP() - INTERVAL 700 SECOND WHERE product_id = ?`, pid)
	exec(t, d, `INSERT INTO kb_case_results (product_id, case_code, result, run_at, run_ref, source) VALUES (?, 'TC-AUTH-001', 'pass', UTC_TIMESTAMP() - INTERVAL 5 SECOND, 'run-1', 'test')`, pid)
	if set, _ := marker(t, d, pid); set {
		t.Fatal("precondition: no marker")
	}
	s.sweepOnce(context.Background())
	waitFor(t, "the sweeper to push data newer than the last push", func() bool { set, _ := marker(t, d, pid); return !set && idle(s) && len(f.requests()) == 2 })

	// And NOT when the last push ended after the data changed: nothing is owed, nothing is sent.
	before := len(f.requests())
	s.sweepOnce(context.Background())
	time.Sleep(200 * time.Millisecond)
	if got := len(f.requests()); got != before {
		t.Fatalf("a product whose last push is newer than its data was pushed again (%d -> %d requests)", before, got)
	}
	// An unflagged product with fresh data is never pushed.
	res, err := d.Exec(context.Background(), `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES ('Unflagged','https://github.com/o/r2','https://s','x','main')`)
	if err != nil {
		t.Fatal(err)
	}
	other := int(res.LastInsertId())
	exec(t, d, `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (?, 0, 'p-7')`, other)
	seedCase(t, d, other, "TC-OTH-001", "OTH", "", "planned")
	s.sweepOnce(context.Background())
	time.Sleep(200 * time.Millisecond)
	if got := len(f.requests()); got != before {
		t.Fatalf("an unflagged product was pushed (%d -> %d requests)", before, got)
	}
}
