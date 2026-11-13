package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// Summarizer is the LLM seam for the change summary (*ai.Service satisfies it).
type Summarizer interface {
	Configured() bool
	ChatMessages(ctx context.Context, msgs []models.Message, opts ai.ChatOpts) (string, error)
}

// TreeReader reads a repo tree (repopool.Pool.ReadTree, replaced in tests).
type TreeReader func(ctx context.Context, repoID int, repoURL, branch, prefix string, maxBytes int64) (string, map[string][]byte, error)

// Syncer runs knowledge syncs.
type Syncer struct {
	DB       *db.DB
	AI       Summarizer
	ReadTree TreeReader

	syncTimeout time.Duration
	aiTimeout   time.Duration
}

// NewSyncer returns nil without a DB. A nil pool leaves ReadTree nil: syncs then fail with ErrUnavailable.
func NewSyncer(d *db.DB, pool *repopool.Pool, s Summarizer) *Syncer {
	if d == nil {
		return nil
	}
	sy := &Syncer{DB: d, AI: s, syncTimeout: 10 * time.Minute, aiTimeout: 3 * time.Minute}
	if pool != nil {
		sy.ReadTree = pool.ReadTree
	}
	return sy
}

// Errors returned by Start.
var (
	ErrProductNotFound = errors.New("product not found")
	ErrNotConfigured   = errors.New("no catalog format is set for this product")
	ErrUnknownFormat   = errors.New("the product's catalog format is not supported")
	ErrNoAutomation    = errors.New("the product has no automation repo")
	ErrUnavailable     = errors.New("repo pool or database not available")
)

// ErrRunning is returned when the product already has a running sync.
type ErrRunning struct{ SyncID int64 }

func (e ErrRunning) Error() string { return "a sync is already running" }

const (
	TriggerManual   = "manual"
	TriggerSchedule = "schedule"
	treeMaxBytes    = 32 << 20
)

type target struct {
	productID   int
	productName string
	format      Format
	repoID      int
	repoName    string
	repoURL     string
	branch      string
}

// Target loads what a sync of the product would read. Exposed for the summary endpoint.
func (s *Syncer) resolve(ctx context.Context, productID int) (*target, error) {
	t := &target{productID: productID}
	var fmtName sql.NullString
	var legacyBranch string
	err := s.DB.QueryRow(ctx, `SELECT p.name, p.branch, s.knowledge_format FROM products p
LEFT JOIN product_settings s ON s.product_id = p.id WHERE p.id = ?`, productID).Scan(&t.productName, &legacyBranch, &fmtName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	if !fmtName.Valid || fmtName.String == "" {
		return nil, ErrNotConfigured
	}
	f, ok := Lookup(fmtName.String)
	if !ok {
		return nil, ErrUnknownFormat
	}
	t.format = f
	err = s.DB.QueryRow(ctx, `SELECT id, name, repo_url FROM product_repos WHERE product_id = ? AND is_automation = 1`, productID).Scan(&t.repoID, &t.repoName, &t.repoURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoAutomation
	}
	if err != nil {
		return nil, err
	}
	err = s.DB.QueryRow(ctx, `SELECT branch FROM product_repo_branches WHERE repo_id = ? AND is_default = 1`, t.repoID).Scan(&t.branch)
	if errors.Is(err, sql.ErrNoRows) {
		t.branch = legacyBranch
	} else if err != nil {
		return nil, err
	}
	if t.branch == "" {
		t.branch = "main"
	}
	return t, nil
}

// Start begins a sync in the background and returns its id.
func (s *Syncer) Start(ctx context.Context, productID int, trigger, userID string) (int64, error) {
	id, _, err := s.start(ctx, productID, trigger, userID)
	return id, err
}

func (s *Syncer) start(ctx context.Context, productID int, trigger, userID string) (int64, <-chan struct{}, error) {
	if s == nil || s.DB == nil || s.ReadTree == nil {
		return 0, nil, ErrUnavailable
	}
	t, err := s.resolve(ctx, productID)
	if err != nil {
		return 0, nil, err
	}
	res, err := s.DB.Exec(ctx, `INSERT INTO kb_syncs (product_id, repo_id, repo_name, repo_url, branch, format, parser_version, status, trigger_kind, triggered_by, started_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'running', ?, ?, UTC_TIMESTAMP())`,
		productID, t.repoID, t.repoName, t.repoURL, t.branch, t.format.Name(), t.format.Version(), trigger, nullIfEmpty(userID))
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			var running int64
			_ = s.DB.QueryRow(ctx, `SELECT id FROM kb_syncs WHERE product_id = ? AND status = 'running'`, productID).Scan(&running)
			return 0, nil, ErrRunning{SyncID: running}
		}
		return 0, nil, err
	}
	id := res.LastInsertId()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.run(id, t, trigger)
	}()
	return id, done, nil
}

// finish writes a terminal failed/skipped status with a fresh context, so an expired sync context
// can never leave the row running.
func (s *Syncer) finish(id int64, status, errText, sha string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.DB.Exec(ctx, `UPDATE kb_syncs SET status = ?, error = ?, commit_sha = COALESCE(?, commit_sha), finished_at = UTC_TIMESTAMP() WHERE id = ? AND status = 'running'`,
		status, nullIfEmpty(clipRunes(errText, 1000)), nullIfEmpty(sha), id); err != nil {
		log.Printf("knowledge: sync %d: write %s: %v", id, status, err)
	}
}

type lastSuccess struct {
	repoID  sql.NullInt64
	branch  string
	format  string
	version int
	sha     sql.NullString
}

func (s *Syncer) run(id int64, t *target, trigger string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.syncTimeout)
	defer cancel()
	terminal := false
	defer func() {
		if r := recover(); r != nil {
			log.Printf("knowledge: sync %d panicked: %v", id, r)
			s.finish(id, "failed", "internal error", "")
			return
		}
		if !terminal {
			s.finish(id, "failed", "sync stopped unexpectedly", "")
		}
	}()
	fail := func(msg string) {
		terminal = true
		log.Printf("knowledge: sync %d (product %d) failed: %s", id, t.productID, msg)
		s.finish(id, "failed", msg, "")
	}

	sha, files, err := s.ReadTree(ctx, t.repoID, t.repoURL, t.branch, "docs", treeMaxBytes)
	if err != nil {
		if errors.Is(err, repopool.ErrBusy) {
			fail("the repo mirror is busy; try again")
		} else {
			fail("read repo: " + err.Error())
		}
		return
	}

	var prev lastSuccess
	err = s.DB.QueryRow(ctx, `SELECT repo_id, branch, format, parser_version, commit_sha FROM kb_syncs
WHERE product_id = ? AND status = 'succeeded' ORDER BY id DESC LIMIT 1`, t.productID).Scan(&prev.repoID, &prev.branch, &prev.format, &prev.version, &prev.sha)
	hasPrev := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail("read previous sync: " + err.Error())
		return
	}
	sameCommit := hasPrev && prev.sha.String == sha && prev.version == t.format.Version()
	if trigger == TriggerSchedule && sameCommit && prev.repoID.Int64 == int64(t.repoID) && prev.branch == t.branch && prev.format == t.format.Name() {
		terminal = true
		s.finish(id, "skipped", "", sha)
		return
	}

	snap, problems, err := t.format.Parse(files)
	if err != nil {
		fail(err.Error())
		return
	}

	var diff *Diff
	var counts Counts
	aiPending := false
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var x int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM products WHERE id = ? FOR UPDATE`, t.productID).Scan(&x); err != nil {
			return err
		}
		probs := append([]Problem{}, problems...)
		d, err := apply(ctx, tx, t.productID, id, snap, func(p Problem) { probs = append(probs, p) })
		if err != nil {
			return err
		}
		d.Unchanged = sameCommit
		c, err := countActive(ctx, tx, t.productID)
		if err != nil {
			return err
		}
		aiPending = !d.Empty() && s.AI != nil && s.AI.Configured()
		aiStatus := "none"
		if aiPending {
			aiStatus = "pending"
		}
		cj, _ := json.Marshal(c)
		dj, _ := json.Marshal(d)
		pj, _ := json.Marshal(summarizeProblems(probs))
		_, err = tx.ExecContext(ctx, `UPDATE kb_syncs SET status = 'succeeded', finished_at = UTC_TIMESTAMP(), commit_sha = ?, counts = ?, diff = ?, problems = ?, ai_status = ? WHERE id = ?`,
			sha, string(cj), string(dj), string(pj), aiStatus, id)
		diff, counts = d, c
		return err
	})
	if err != nil {
		fail("store: " + err.Error())
		return
	}
	terminal = true
	log.Printf("knowledge: sync %d (product %d) succeeded at %.7s: +%d ~%d -%d ↺%d", id, t.productID, sha,
		diff.Total["added"], diff.Total["updated"], diff.Total["removed"], diff.Total["restored"])
	_ = counts
	if aiPending {
		prevSHA := ""
		if hasPrev {
			prevSHA = prev.sha.String
		}
		s.summarize(id, t, prevSHA, sha, snap, diff, len(problems))
	}
}

const summarySystem = "You summarise changes to a product test catalog for QA engineers. Reply with 3–8 markdown bullets in English. No preamble."

func (s *Syncer) summarize(id int64, t *target, prevSHA, sha string, snap *Snapshot, d *Diff, problemCount int) {
	ctx, cancel := context.WithTimeout(context.Background(), s.aiTimeout)
	defer cancel()
	labels := labelsOf(snap)
	var b strings.Builder
	fmt.Fprintf(&b, "Product: %s\nCatalog commit: %.7s → %.7s\n", t.productName, prevSHA, sha)
	fmt.Fprintf(&b, "Totals: added %d, updated %d, removed %d, restored %d. Parse problems: %d.\n\nChanged entities:\n",
		d.Total["added"], d.Total["updated"], d.Total["removed"], d.Total["restored"], problemCount)
	n := 0
	for _, et := range EntityTypes {
		for _, kind := range []string{"added", "updated", "restored", "removed"} {
			for _, code := range d.ByType[et][kind] {
				if n >= 100 {
					break
				}
				n++
				fmt.Fprintf(&b, "- %s %s %s: %s\n", kind, et, code, clipRunes(labels[et+"/"+code], 200))
			}
		}
	}
	prompt := clipRunes(b.String(), 60000)
	text, err := s.AI.ChatMessages(ctx, []models.Message{{Role: "system", Content: summarySystem}, {Role: "user", Content: prompt}}, ai.ChatOpts{Temperature: 0.3})
	wctx, wcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer wcancel()
	if err != nil {
		log.Printf("knowledge: sync %d summary failed: %v", id, err)
		_, _ = s.DB.Exec(wctx, `UPDATE kb_syncs SET ai_status = 'failed', ai_error = ? WHERE id = ?`, clipRunes(err.Error(), 400), id)
		return
	}
	_, _ = s.DB.Exec(wctx, `UPDATE kb_syncs SET ai_status = 'done', ai_summary = ? WHERE id = ?`, clipBytes(strings.TrimSpace(text), 1<<20), id)
}

func labelsOf(s *Snapshot) map[string]string {
	m := map[string]string{}
	for _, x := range s.Parts {
		m[TypePart+"/"+x.Code] = x.Name
	}
	for _, x := range s.Modules {
		m[TypeModule+"/"+x.Code] = x.Name
	}
	for _, x := range s.Submodules {
		m[TypeSubmodule+"/"+x.Code] = x.Name
	}
	for _, x := range s.Features {
		m[TypeFeature+"/"+x.Code] = x.Name
	}
	for _, x := range s.Epics {
		m[TypeEpic+"/"+x.Code] = x.Title
	}
	for _, x := range s.Stories {
		m[TypeStory+"/"+x.Code] = x.Text
	}
	for _, x := range s.TestCases {
		m[TypeTestCase+"/"+x.Code] = x.Scenario
	}
	return m
}

// FailOrphanedSyncs marks syncs and summaries left in progress by a previous process as failed.
func (s *Syncer) FailOrphanedSyncs(ctx context.Context) {
	if s == nil {
		return
	}
	if res, err := s.DB.Exec(ctx, `UPDATE kb_syncs SET status = 'failed', error = 'backend restarted while the sync was in progress', finished_at = UTC_TIMESTAMP() WHERE status = 'running'`); err != nil {
		log.Printf("knowledge: reconcile orphaned syncs: %v", err)
	} else if n := res.RowsAffected(); n > 0 {
		log.Printf("knowledge: marked %d orphaned sync(s) as failed", n)
	}
	if _, err := s.DB.Exec(ctx, `UPDATE kb_syncs SET ai_status = 'failed', ai_error = 'backend restarted before the summary finished' WHERE ai_status = 'pending'`); err != nil {
		log.Printf("knowledge: reconcile orphaned summaries: %v", err)
	}
}

// StartScheduler re-syncs every configured product each interval, one product at a time. The first
// tick is one interval after start. interval <= 0 or no pool disables it.
func (s *Syncer) StartScheduler(ctx context.Context, interval time.Duration) {
	if s == nil || interval <= 0 || s.ReadTree == nil {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.syncAll(ctx)
			}
		}
	}()
	log.Printf("   Knowledge: scheduled sync every %s", interval)
}

func (s *Syncer) syncAll(ctx context.Context) {
	rs, err := s.DB.Query(ctx, `SELECT s.product_id, s.knowledge_format FROM product_settings s WHERE s.knowledge_format IS NOT NULL AND s.knowledge_format <> '' ORDER BY s.product_id`)
	if err != nil {
		log.Printf("knowledge: scheduler: %v", err)
		return
	}
	var ids []int
	for rs.Next() {
		var id int
		var f string
		if rs.Scan(&id, &f) == nil {
			if _, ok := Lookup(f); ok {
				ids = append(ids, id)
			}
		}
	}
	rs.Close()
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		_, done, err := s.start(ctx, id, TriggerSchedule, "")
		if err != nil {
			var running ErrRunning
			if !errors.As(err, &running) && !errors.Is(err, ErrNoAutomation) {
				log.Printf("knowledge: scheduled sync of product %d: %v", id, err)
			}
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return
		}
	}
}
