package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

type histRow struct {
	code, status, source string
	prev                 *string
	syncID               int64
}

func histOf(t *testing.T, d *db.DB, pid int) []histRow {
	t.Helper()
	rs, err := d.Query(context.Background(), `SELECT feature_code, release_status, source, prev_status, sync_id FROM kb_feature_status_history WHERE product_id = ? ORDER BY id`, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	var out []histRow
	for rs.Next() {
		var h histRow
		var sid *int64
		if err := rs.Scan(&h.code, &h.status, &h.source, &h.prev, &sid); err != nil {
			t.Fatal(err)
		}
		if sid != nil {
			h.syncID = *sid
		}
		out = append(out, h)
	}
	return out
}

func histCount(t *testing.T, d *db.DB, pid int) int { return len(histOf(t, d, pid)) }

// historySyncer returns a Syncer without AI and a helper that runs one manual sync to completion.
func historySyncer(t *testing.T, d *db.DB, pid int, repo *fakeRepo) func() int64 {
	s := &Syncer{DB: d, ReadTree: repo.read, syncTimeout: time.Minute, aiTimeout: time.Minute}
	return func() int64 {
		t.Helper()
		id, err := s.Start(context.Background(), pid, TriggerManual, "")
		if err != nil {
			t.Fatal(err)
		}
		if r := waitSync(t, d, id); r.status != "succeeded" {
			t.Fatalf("sync %d: %+v", id, r)
		}
		return id
	}
}

func TestStatusHistory(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{}
	run := historySyncer(t, d, pid, repo)
	files := func(mod string) map[string]string {
		return map[string]string{"docs/modules/A-core.md": mod, "docs/test-catalog/svc/out.md": epic}
	}

	// First sync: one backfill row per feature, no prev_status.
	repo.set("1111111111111111111111111111111111111111", files(modA))
	id1 := run()
	h := histOf(t, d, pid)
	if len(h) != 2 {
		t.Fatalf("first sync: %+v", h)
	}
	for _, r := range h {
		if r.source != "backfill" || r.prev != nil || r.syncID != id1 {
			t.Errorf("first sync row: %+v", r)
		}
	}

	// Same commit again: nothing.
	run()
	if n := histCount(t, d, pid); n != 2 {
		t.Fatalf("same commit wrote history: %d", n)
	}

	// A new feature in a later sync is a 'sync' row.
	modB := modA + "| A01.03 | Pasien | Delete | app | `DEV-DONE` | 2026-02-01 | | | | | | |\n"
	repo.set("2222222222222222222222222222222222222222", files(modB))
	id2 := run()
	h = histOf(t, d, pid)
	if len(h) != 3 || h[2].code != "A01.03" || h[2].source != "sync" || h[2].prev != nil || h[2].syncID != id2 {
		t.Fatalf("new feature: %+v", h)
	}

	// Status change: exactly one sync row with the right prev_status.
	modC := strings.Replace(modB, "`UNKNOWN` | —", "`PROD` | 2026-03-01", 1)
	repo.set("3333333333333333333333333333333333333333", files(modC))
	id3 := run()
	h = histOf(t, d, pid)
	if len(h) != 4 || h[3].code != "A01.02" || h[3].status != "PROD" || h[3].prev == nil || *h[3].prev != "UNKNOWN" || h[3].source != "sync" || h[3].syncID != id3 {
		t.Fatalf("status change: %+v", h)
	}

	// A notes-only change: no row.
	modD := strings.Replace(modC, "| A01.03 | Pasien | Delete | app | `DEV-DONE` | 2026-02-01 | | | | | | |", "| A01.03 | Pasien | Delete | app | `DEV-DONE` | 2026-02-01 | | | a note | | | |", 1)
	repo.set("4444444444444444444444444444444444444444", files(modD))
	run()
	if n := histCount(t, d, pid); n != 4 {
		t.Fatalf("notes-only change wrote history: %d", n)
	}

	// Rewritten content hashes with equal statuses (parser-version bump): no row.
	if _, err := d.Exec(ctx, `UPDATE kb_features SET content_hash = REPEAT('0', 64) WHERE product_id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	repo.set("4545454545454545454545454545454545454545", files(modD))
	run()
	if n := histCount(t, d, pid); n != 4 {
		t.Fatalf("hash rewrite wrote history: %d", n)
	}

	// Remove then restore with the same status: no row.
	repo.set("5555555555555555555555555555555555555555", files(strings.Replace(modD, "| A01.03 | Pasien | Delete | app | `DEV-DONE` | 2026-02-01 | | | a note | | | |\n", "", 1)))
	run()
	repo.set("6666666666666666666666666666666666666666", files(modD))
	run()
	if n := histCount(t, d, pid); n != 4 {
		t.Fatalf("restore with equal status wrote history: %d", n)
	}

	// Remove, then restore with a different status: one row.
	repo.set("7777777777777777777777777777777777777777", files(strings.Replace(modD, "| A01.03 | Pasien | Delete | app | `DEV-DONE` | 2026-02-01 | | | a note | | | |\n", "", 1)))
	run()
	repo.set("8888888888888888888888888888888888888888", files(strings.Replace(modD, "`DEV-DONE` | 2026-02-01", "`PROD` | 2026-04-01", 1)))
	id8 := run()
	h = histOf(t, d, pid)
	if len(h) != 5 || h[4].code != "A01.03" || h[4].status != "PROD" || h[4].prev == nil || *h[4].prev != "DEV-DONE" || h[4].syncID != id8 {
		t.Fatalf("restore with new status: %+v", h)
	}
}

// TestStatusHistoryBackfillMigration runs the full migration list again over a database that
// already has features: the backfill statement must add rows only where the current status has none.
func TestStatusHistoryBackfillMigration(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{}
	run := historySyncer(t, d, pid, repo)
	repo.set("1111111111111111111111111111111111111111", map[string]string{"docs/modules/A-core.md": modA, "docs/test-catalog/svc/out.md": epic})
	run()

	// Simulate a database from before the history table: features, no history.
	if _, err := d.Exec(ctx, `DELETE FROM kb_feature_status_history WHERE product_id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(d.SQL); err != nil {
		t.Fatal(err)
	}
	h := histOf(t, d, pid)
	if len(h) != 2 {
		t.Fatalf("backfill: %+v", h)
	}
	for _, r := range h {
		if r.source != "backfill" || r.prev != nil || r.syncID == 0 {
			t.Errorf("backfill row: %+v", r)
		}
	}
	// A second run changes nothing.
	if err := db.RunMigrations(d.SQL); err != nil {
		t.Fatal(err)
	}
	if n := histCount(t, d, pid); n != 2 {
		t.Fatalf("second run: %d rows", n)
	}
	// A status changed by the old binary during the deploy window: it writes the status and a newer
	// changed_sync_id but no history row. The next run adds a row with the new status.
	id2 := run()
	if _, err := d.Exec(ctx, `UPDATE kb_features SET release_status = 'DEV-DONE', changed_sync_id = ? WHERE product_id = ? AND code = 'A01.02'`, id2, pid); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(d.SQL); err != nil {
		t.Fatal(err)
	}
	h = histOf(t, d, pid)
	if len(h) != 3 || h[2].code != "A01.02" || h[2].status != "DEV-DONE" || h[2].source != "backfill" {
		t.Fatalf("deploy window: %+v", h)
	}
}

func TestChecklistSnapshotDB(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	pid := setupProduct(t, d, TestCatalogFormatName)
	repo := &fakeRepo{}
	s := &Syncer{DB: d, ReadTree: repo.read, syncTimeout: time.Minute, aiTimeout: time.Minute}
	run := historySyncer(t, d, pid, repo)

	// No such product, then no succeeded sync.
	if _, err := s.ChecklistKeyInfo(ctx, pid+99, false); err != ErrNotFound {
		t.Fatalf("missing product: %v", err)
	}
	if _, err := s.ChecklistKeyInfo(ctx, pid, false); err != ErrNoSucceededSync {
		t.Fatalf("no sync (key info): %v", err)
	}
	if _, err := s.ChecklistSnapshot(ctx, pid, true); err != ErrNoSucceededSync {
		t.Fatalf("no sync (snapshot): %v", err)
	}

	files := func(mod string) map[string]string {
		return map[string]string{"docs/modules/A-core.md": mod, "docs/test-catalog/svc/out.md": epic}
	}
	repo.set("1111111111111111111111111111111111111111", files(modA))
	id1 := run()
	ki, err := s.ChecklistKeyInfo(ctx, pid, true)
	if err != nil || ki.SyncID != id1 || ki.ProductName != "SUT" || ki.HistoryMaxID == 0 {
		t.Fatalf("key info: %+v %v", ki, err)
	}
	if k0, _ := s.ChecklistKeyInfo(ctx, pid, false); k0.HistoryMaxID != 0 {
		t.Errorf("watermark without the option: %+v", k0)
	}
	snap, err := s.ChecklistSnapshot(ctx, pid, true)
	if err != nil || snap.SyncID != id1 || snap.FeatureCount() != 2 || snap.Skipped != 0 || snap.SyncedAt.IsZero() || snap.CommitSHA != strings.Repeat("1", 40) {
		t.Fatalf("snapshot: %+v %v", snap, err)
	}
	for _, r := range snap.Parts[0].Modules[0].Rows {
		if r.DateSource != "backfill" || r.StatusSince == nil {
			t.Errorf("first-sync row should carry a backfill date: %+v", r)
		}
	}
	plain, _ := s.ChecklistSnapshot(ctx, pid, false)
	if plain.Parts[0].Modules[0].Rows[0].StatusSince != nil {
		t.Error("dates present without the option")
	}

	// A sync committing while snapshots are read: every snapshot is internally consistent.
	modB := modA + "| A01.03 | Pasien | Delete | app | `DEV-DONE` | 2026-02-01 | | | | | | |\n"
	repo.set("2222222222222222222222222222222222222222", files(modB))
	stop := make(chan struct{})
	bad := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			sn, err := s.ChecklistSnapshot(ctx, pid, true)
			if err != nil {
				bad <- err.Error()
				return
			}
			if (sn.SyncID == id1 && sn.FeatureCount() != 2) || (sn.SyncID > id1 && sn.FeatureCount() != 3) {
				bad <- "inconsistent snapshot"
				return
			}
		}
	}()
	id2 := run()
	close(stop)
	<-done
	select {
	case m := <-bad:
		t.Fatal(m)
	default:
	}
	if sn, _ := s.ChecklistSnapshot(ctx, pid, true); sn.SyncID != id2 || sn.FeatureCount() != 3 {
		t.Fatalf("after sync: %+v", sn)
	}
}
