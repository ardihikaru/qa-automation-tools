package gaossync

import (
	"context"
	"log"
	"time"
)

const (
	sweeperStartDelay  = 5 * time.Second
	backoffStepDefault = 30 * time.Second
	backoffMax         = 10 * time.Minute
)

// backoffFor is how long a product with a push owed is left alone after its last finished
// attempt: min(attempts * step, 10 min). Zero attempts means no wait.
func backoffFor(attempts int, step time.Duration) time.Duration {
	if attempts <= 0 {
		return 0
	}
	d := time.Duration(attempts) * step
	if d > backoffMax || d < 0 {
		return backoffMax
	}
	return d
}

// StartSweeper runs the outbox sweeper until ctx ends: once after a short delay (so the HTTP
// server is up), then every interval. It never blocks the caller, never panics the process and
// never runs two sweeps at once.
func (s *Syncer) StartSweeper(ctx context.Context, interval time.Duration) {
	if !s.Configured() || interval <= 0 {
		return
	}
	go func() {
		t := time.NewTimer(s.startDelay)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			s.sweepOnce(ctx)
			t.Reset(interval)
		}
	}()
}

// sweepOnce handles every product with a push owed. A product that is no longer flagged or
// configured loses its marker; a flagged one is pushed through the same coalescing start as a
// trigger, unless a push is already running for it or its backoff has not elapsed.
func (s *Syncer) sweepOnce(ctx context.Context) {
	if !s.sweepMu.TryLock() {
		return
	}
	defer s.sweepMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("gaossync: sweeper panicked: %v", r)
		}
	}()
	qctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// LEVEL-triggered safety net, before the marker list is read. A marker is edge-triggered: if the process
	// died between "data stored" and "marker written", no marker exists. So a flagged product whose results or
	// cases changed after its last push ended is owed a push whether or not a marker survived (a never-pushed
	// flagged product too). It is promoted to a marker and handled below like any other owed push. A push that
	// finished after the change clears the condition (gaos_last_sync_at is the push's end, UTC like updated_at);
	// a change DURING a push is covered by its own marker. At worst one redundant push, which GAOS answers
	// `unchanged`.
	if res, err := s.db.Exec(qctx, `UPDATE product_settings s
SET s.gaos_push_requested_at = UTC_TIMESTAMP(6), s.gaos_push_attempts = 0
WHERE s.gaos_integrated = 1 AND s.gaos_product_id <> '' AND s.gaos_push_requested_at IS NULL
  AND (s.gaos_last_sync_at IS NULL
       OR EXISTS (SELECT 1 FROM kb_case_results r WHERE r.product_id = s.product_id AND r.updated_at > s.gaos_last_sync_at)
       OR EXISTS (SELECT 1 FROM kb_test_cases c WHERE c.product_id = s.product_id AND c.updated_at > s.gaos_last_sync_at))`); err != nil {
		log.Printf("gaossync: sweeper: promote changed data to owed pushes: %v", err)
	} else if n := res.RowsAffected(); n > 0 {
		log.Printf("gaossync: sweeper: %d product(s) have data newer than their last push", n)
	}
	rows, err := s.db.Query(qctx, `SELECT product_id, gaos_integrated, gaos_product_id, gaos_push_attempts,
  COALESCE(TIMESTAMPDIFF(SECOND, gaos_last_sync_at, UTC_TIMESTAMP()), 2147483647)
FROM product_settings WHERE gaos_push_requested_at IS NOT NULL`)
	if err != nil {
		log.Printf("gaossync: sweeper: list owed pushes: %v", err)
		return
	}
	type owed struct {
		id         int
		integrated bool
		pid        string
		attempts   int
		sinceLast  int64
	}
	var list []owed
	for rows.Next() {
		var o owed
		if err := rows.Scan(&o.id, &o.integrated, &o.pid, &o.attempts, &o.sinceLast); err != nil {
			log.Printf("gaossync: sweeper: read row: %v", err)
			_ = rows.Close()
			return
		}
		list = append(list, o)
	}
	if err := rows.Err(); err != nil {
		log.Printf("gaossync: sweeper: list owed pushes: %v", err)
	}
	_ = rows.Close()

	for _, o := range list {
		if ctx.Err() != nil {
			return
		}
		if !o.integrated || !ValidProductID(o.pid) {
			if _, err := s.db.Exec(qctx, `UPDATE product_settings SET gaos_push_requested_at = NULL, gaos_push_attempts = 0
WHERE product_id = ? AND gaos_integrated = ? AND gaos_product_id = ?`, o.id, o.integrated, o.pid); err != nil {
				log.Printf("gaossync: sweeper: product %d: clear marker: %v", o.id, err)
			}
			continue
		}
		if time.Duration(o.sinceLast)*time.Second < backoffFor(o.attempts, s.backoffStep) {
			continue
		}
		run, ok := s.begin(o.id)
		if !ok {
			continue // a push is running; a trigger that arrives meanwhile marks it dirty itself
		}
		log.Printf("gaossync: sweeper: re-sending the owed push of product %d (attempts so far %d)", o.id, o.attempts)
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("gaossync: product %d: sweeper push panicked: %v", id, r)
				}
			}()
			s.loop(id, run)
		}(o.id)
	}
}
