package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// Draft write-back, database part (KNOWLEDGE_TRANSLATION.md, "Write-back design" steps 2, 7 and 8).
// Git, the job and the HTTP routes live in internal/api/knowledge_publish.go; the pure rendering and
// the round-trip gate in publish_render.go.

// Publication states (kb_publications.state).
const (
	PubQueued  = "queued"
	PubRunning = "running"
	PubPROpen  = "pr_open"
	PubMerged  = "merged"
	PubFailed  = "failed"
)

// Blocker says why a draft cannot be published now.
type Blocker struct {
	Type string `json:"type"`
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// DraftSet is the drafts of one epic that a publish would write (RenderRequest minus the codes).
type DraftSet struct {
	Epic     string
	Stories  []RenderStory
	Cases    []RenderCase
	Blockers []Blocker
}

// Empty reports whether nothing can be published.
func (d *DraftSet) Empty() bool { return len(d.Stories) == 0 && len(d.Cases) == 0 }

// openStates are the states in which a draft is in a publication (not publishable again).
const openStates = `'queued','running','pr_open'`

// CollectDrafts lists the publishable drafts of an epic: origin local, active, with English text, in no
// open publication. A case whose story is a draft is included only with that story (same epic and
// itself publishable); a case with no usable story is a Blocker, as is a story whose feature is gone.
func (s *Syncer) CollectDrafts(ctx context.Context, pid int, epic string) (*DraftSet, error) {
	return s.collectDrafts(ctx, pid, epic, "")
}

// JobDrafts is CollectDrafts for the job that runs a queued publication: the drafts of that job's
// rows (which CollectDrafts skips as locked), with the same blockers, read fresh.
func (s *Syncer) JobDrafts(ctx context.Context, pid int, epic, jobID string) (*DraftSet, error) {
	return s.collectDrafts(ctx, pid, epic, jobID)
}

func (s *Syncer) collectDrafts(ctx context.Context, pid int, epic, jobID string) (*DraftSet, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	if !s.activeExists(ctx, queryRower{s.DB}, pid, TypeEpic, epic) {
		return nil, badInput("epic_code %q is not an active epic of this product", epic)
	}
	set := &DraftSet{Epic: epic}
	// locked: drafts in another open publication. mine (job mode): the drafts of jobID's own rows.
	locked, mine := map[string]bool{}, map[string]bool{}
	load := func(into map[string]bool, q string, args ...any) error {
		rs, err := s.DB.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rs.Close()
		for rs.Next() {
			var et, c string
			if err := rs.Scan(&et, &c); err != nil {
				return err
			}
			into[et+"\x00"+c] = true
		}
		return rs.Err()
	}
	if err := load(locked, `SELECT entity_type, local_code FROM kb_publications WHERE product_id = ? AND state IN (`+openStates+`) AND job_id <> ?`, pid, jobID); err != nil {
		return nil, err
	}
	if jobID != "" {
		if err := load(mine, `SELECT entity_type, local_code FROM kb_publications WHERE product_id = ? AND job_id = ?`, pid, jobID); err != nil {
			return nil, err
		}
	}
	skip := func(et, code string) bool {
		return locked[et+"\x00"+code] || (jobID != "" && !mine[et+"\x00"+code])
	}

	// stories
	rs, err := s.DB.Query(ctx, `SELECT code, COALESCE(text, '') FROM kb_stories WHERE product_id = ? AND epic_code = ? AND origin = 'local' AND removed_at IS NULL ORDER BY created_at, id`, pid, epic)
	if err != nil {
		return nil, err
	}
	okStory := map[string]bool{}
	for rs.Next() {
		var st RenderStory
		if err := rs.Scan(&st.LocalCode, &st.Text); err != nil {
			rs.Close()
			return nil, err
		}
		switch {
		case skip(TypeStory, st.LocalCode):
			continue // in another publication: neither a draft to publish nor a blocker
		case strings.TrimSpace(st.Text) == "":
			set.Blockers = append(set.Blockers, Blocker{TypeStory, st.LocalCode, "the story has no English text"})
			continue
		}
		set.Stories = append(set.Stories, st)
		okStory[st.LocalCode] = true
	}
	if err := rs.Err(); err != nil {
		rs.Close()
		return nil, err
	}
	rs.Close()
	for i := range set.Stories {
		fr, err := s.DB.Query(ctx, `SELECT feature_code FROM kb_story_features WHERE product_id = ? AND story_code = ? ORDER BY feature_code`, pid, set.Stories[i].LocalCode)
		if err != nil {
			return nil, err
		}
		for fr.Next() {
			var f string
			if err := fr.Scan(&f); err != nil {
				fr.Close()
				return nil, err
			}
			set.Stories[i].FeatureCodes = append(set.Stories[i].FeatureCodes, f)
		}
		fr.Close()
	}

	// cases
	rs, err = s.DB.Query(ctx, `SELECT code, COALESCE(story_code, ''), COALESCE(scenario, ''), polarity, level, priority, status, spec_paths,
COALESCE(reason_category, ''), COALESCE(reason_detail, '') FROM kb_test_cases WHERE product_id = ? AND epic_code = ? AND origin = 'local' AND removed_at IS NULL ORDER BY created_at, id`, pid, epic)
	if err != nil {
		return nil, err
	}
	type cand struct{ c RenderCase }
	var cands []cand
	for rs.Next() {
		var c RenderCase
		var spec []byte
		if err := rs.Scan(&c.LocalCode, &c.StoryRef, &c.Scenario, &c.Polarity, &c.Level, &c.Priority, &c.Status, &spec, &c.ReasonCategory, &c.ReasonDetail); err != nil {
			rs.Close()
			return nil, err
		}
		_ = json.Unmarshal(spec, &c.SpecPaths)
		cands = append(cands, cand{c})
	}
	if err := rs.Err(); err != nil {
		rs.Close()
		return nil, err
	}
	rs.Close()
	for _, cd := range cands {
		c := cd.c
		block := func(msg string) { set.Blockers = append(set.Blockers, Blocker{TypeTestCase, c.LocalCode, msg}) }
		switch {
		case skip(TypeTestCase, c.LocalCode):
			continue
		case strings.TrimSpace(c.Scenario) == "":
			block("the test case has no English scenario")
			continue
		case c.StoryRef == "":
			block("the test case has no story; the catalog needs one")
			continue
		}
		if LocalCodeRe.MatchString(c.StoryRef) {
			if !okStory[c.StoryRef] {
				block(fmt.Sprintf("its story %s is a draft that is not published with this epic", c.StoryRef))
				continue
			}
		} else if !s.activeExists(ctx, queryRower{s.DB}, pid, TypeStory, c.StoryRef) {
			block(fmt.Sprintf("its story %s is not an active story", c.StoryRef))
			continue
		}
		set.Cases = append(set.Cases, c)
	}
	return set, nil
}

// queryRower adapts *db.DB (context-first QueryRow) to the QueryRowContext shape activeExists takes.
type queryRower struct {
	d interface {
		QueryRow(ctx context.Context, q string, args ...any) *sql.Row
	}
}

func (q queryRower) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return q.d.QueryRow(ctx, query, args...)
}

// TakenCodes lists the repo codes the allocation must skip for an epic: every kb_* row (removed ones
// too) and the repo codes of non-failed publications.
func (s *Syncer) TakenCodes(ctx context.Context, pid int, epic, exceptJob string) ([]string, error) {
	var out []string
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`SELECT code FROM kb_stories WHERE product_id = ? AND code LIKE ?`, []any{pid, "US-" + likeEscape(epic) + "-%"}},
		{`SELECT code FROM kb_test_cases WHERE product_id = ? AND code LIKE ?`, []any{pid, "TC-" + likeEscape(epic) + "-%"}},
		{`SELECT repo_code FROM kb_publications WHERE product_id = ? AND repo_code LIKE ? AND state <> 'failed' AND job_id <> ?`, []any{pid, "%-" + likeEscape(epic) + "-%", exceptJob}},
	} {
		rs, err := s.DB.Query(ctx, q.sql, q.args...)
		if err != nil {
			return nil, err
		}
		for rs.Next() {
			var c string
			if err := rs.Scan(&c); err != nil {
				rs.Close()
				return nil, err
			}
			out = append(out, c)
		}
		if err := rs.Err(); err != nil {
			rs.Close()
			return nil, err
		}
		rs.Close()
	}
	return out, nil
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// ErrPublishActive is the 409 of a second publish while one is queued or running for the product.
var ErrPublishActive = errors.New("a publish is already queued or running for this product")

// ErrSyncRunning is the 409 of a publish while a sync is running.
var ErrSyncRunning = errors.New("a sync is running for this product; wait for it to finish")

// ActivePublishJob returns the job id of a queued or running publication of the product, or "".
func (s *Syncer) ActivePublishJob(ctx context.Context, pid int) (string, error) {
	var id sql.NullString
	err := s.DB.QueryRow(ctx, `SELECT job_id FROM kb_publications WHERE product_id = ? AND state IN ('queued','running') LIMIT 1`, pid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id.String, err
}

// SyncRunning reports whether a knowledge sync is running for the product.
func (s *Syncer) SyncRunning(ctx context.Context, pid int) (bool, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT COUNT(*) FROM kb_syncs WHERE product_id = ? AND status = 'running'`, pid).Scan(&n)
	return n > 0, err
}

// LatestSyncCommit is the commit of the product's newest succeeded sync ("" = none).
func (s *Syncer) LatestSyncCommit(ctx context.Context, pid int) (string, error) {
	var sha sql.NullString
	err := s.DB.QueryRow(ctx, `SELECT commit_sha FROM kb_syncs WHERE product_id = ? AND status = 'succeeded' ORDER BY id DESC LIMIT 1`, pid).Scan(&sha)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return sha.String, err
}

// QueuePublication records the drafts of set as one queued publication under jobID.
func (s *Syncer) QueuePublication(ctx context.Context, pid int, jobID, userID string, set *DraftSet) error {
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		ins := func(et, code string) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO kb_publications (product_id, job_id, entity_type, local_code, epic_code, state, created_by) VALUES (?, ?, ?, ?, ?, 'queued', ?)`,
				pid, jobID, et, code, set.Epic, nullIfEmpty(userID))
			return err
		}
		for _, st := range set.Stories {
			if err := ins(TypeStory, st.LocalCode); err != nil {
				return err
			}
		}
		for _, c := range set.Cases {
			if err := ins(TypeTestCase, c.LocalCode); err != nil {
				return err
			}
		}
		return nil
	})
}

// PublicationRunning marks the job's publication running.
func (s *Syncer) PublicationRunning(ctx context.Context, jobID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE kb_publications SET state = 'running' WHERE job_id = ? AND state IN ('queued','running')`, jobID)
	return err
}

// PublicationAssign records the branch and the repo codes of a running publication.
func (s *Syncer) PublicationAssign(ctx context.Context, jobID, branch string, pairs []CodePair) error {
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE kb_publications SET branch = ? WHERE job_id = ?`, branch, jobID); err != nil {
			return err
		}
		for _, p := range pairs {
			if _, err := tx.ExecContext(ctx, `UPDATE kb_publications SET repo_code = ? WHERE job_id = ? AND entity_type = ? AND local_code = ?`, p.Repo, jobID, p.Type, p.Local); err != nil {
				return err
			}
		}
		return nil
	})
}

// PublicationOpened records the pull request: the publication is now waiting for a merge.
func (s *Syncer) PublicationOpened(ctx context.Context, jobID, prURL string) error {
	_, err := s.DB.Exec(ctx, `UPDATE kb_publications SET state = 'pr_open', pr_url = ? WHERE job_id = ? AND state IN ('queued','running')`, prURL, jobID)
	return err
}

// PublicationFailed ends a publication that did not reach an open PR. Its repo codes are released.
func (s *Syncer) PublicationFailed(ctx context.Context, jobID, reason string) error {
	_, err := s.DB.Exec(ctx, `UPDATE kb_publications SET state = 'failed', error = ?, repo_code = NULL WHERE job_id = ? AND state IN ('queued','running')`, clipRunes(reason, 400), jobID)
	return err
}

// checkNotPublishing returns a ConflictError when the draft is in a queued or running publication.
func (s *Syncer) checkNotPublishing(ctx context.Context, pid int, et, code string) error {
	if et != TypeStory && et != TypeTestCase {
		return nil
	}
	var n int
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*) FROM kb_publications WHERE product_id = ? AND entity_type = ? AND local_code = ? AND state IN ('queued','running')`, pid, et, code).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return &ConflictError{Msg: "this draft is being published; it cannot be changed or deleted until the publish ends"}
	}
	return nil
}

// Publication is one publish (the rows of one job) as the API serves it.
type Publication struct {
	ID       int64             `json:"id"`
	EpicCode string            `json:"epic_code"`
	State    string            `json:"state"`
	PRURL    string            `json:"pr_url"`
	Branch   string            `json:"branch"`
	JobID    string            `json:"job_id"`
	Codes    map[string]string `json:"codes"`
	Error    string            `json:"error,omitempty"`
}

// pubState folds the states of the rows of one job: an unfinished row decides, else a waiting PR,
// else merged when every row merged, else failed.
func pubState(states map[string]int) string {
	switch {
	case states[PubRunning] > 0:
		return PubRunning
	case states[PubQueued] > 0:
		return PubQueued
	case states[PubPROpen] > 0:
		return PubPROpen
	case states[PubFailed] > 0 && states[PubMerged] == 0:
		return PubFailed
	case states[PubMerged] > 0:
		return PubMerged
	}
	return PubFailed
}

// Publications lists the publishes of the product, newest first (at most 100).
func (s *Syncer) Publications(ctx context.Context, pid int) ([]Publication, error) {
	rs, err := s.DB.Query(ctx, `SELECT id, job_id, local_code, COALESCE(repo_code, ''), epic_code, branch, pr_url, state, error FROM kb_publications
WHERE product_id = ? ORDER BY id DESC LIMIT 2000`, pid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	type acc struct {
		p      Publication
		states map[string]int
	}
	byJob := map[string]*acc{}
	var order []string
	for rs.Next() {
		var id int64
		var job, local, repo, epic, branch, pr, state, errMsg string
		if err := rs.Scan(&id, &job, &local, &repo, &epic, &branch, &pr, &state, &errMsg); err != nil {
			return nil, err
		}
		a := byJob[job]
		if a == nil {
			a = &acc{p: Publication{ID: id, EpicCode: epic, JobID: job, Codes: map[string]string{}}, states: map[string]int{}}
			byJob[job] = a
			order = append(order, job)
		}
		if id < a.p.ID {
			a.p.ID = id
		}
		a.states[state]++
		if repo != "" {
			a.p.Codes[local] = repo
		}
		if branch != "" {
			a.p.Branch = branch
		}
		if pr != "" {
			a.p.PRURL = pr
		}
		if errMsg != "" {
			a.p.Error = errMsg
		}
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}
	out := make([]Publication, 0, len(order))
	for _, j := range order {
		a := byJob[j]
		a.p.State = pubState(a.states)
		out = append(out, a.p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}

// JobCodes is the local to repo code mapping of one publish job (empty before the codes exist).
func (s *Syncer) JobCodes(ctx context.Context, jobID string) (map[string]string, error) {
	rs, err := s.DB.Query(ctx, `SELECT local_code, repo_code FROM kb_publications WHERE job_id = ? AND repo_code IS NOT NULL`, jobID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string]string{}
	for rs.Next() {
		var l, r string
		if err := rs.Scan(&l, &r); err != nil {
			return nil, err
		}
		out[l] = r
	}
	return out, rs.Err()
}

// MovedTo resolves a published draft code to its repo code ("" = it was never published and merged).
func (s *Syncer) MovedTo(ctx context.Context, pid int, et, local string) string {
	var repo sql.NullString
	_ = s.DB.QueryRow(ctx, `SELECT repo_code FROM kb_publications WHERE product_id = ? AND entity_type = ? AND local_code = ? AND state = 'merged' ORDER BY id DESC LIMIT 1`, pid, et, local).Scan(&repo)
	return repo.String
}

// ---------------------------------------------------------------- reconcile

// PRChecker reads a pull request's state (*repopool.Pool).
type PRChecker interface {
	PRStatus(ctx context.Context, rootDir, prURL string) (repopool.PRState, error)
}

// ReconcilePublications is the sync hook half of the write-back: for each pull request still open
// in kb_publications it asks the PR checker. Merged and the repo code is now a synced row: the
// results, verifications, annotations and translations move to the repo code and the L- draft is
// deleted, in one transaction per PR. Closed without a merge: the publication fails and its drafts
// are publishable again. job_runs and case_approvals keep the L- code. Errors are logged only.
func (s *Syncer) ReconcilePublications(ctx context.Context, pid int, pr PRChecker) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("knowledge: reconcile publications (product %d): panic: %v", pid, p)
		}
	}()
	if s == nil || pr == nil {
		return
	}
	var root sql.NullString
	if err := s.DB.QueryRow(ctx, `SELECT root_dir FROM product_repos WHERE product_id = ? AND is_automation = 1`, pid).Scan(&root); err != nil || root.String == "" {
		return
	}
	rs, err := s.DB.Query(ctx, `SELECT DISTINCT pr_url FROM kb_publications WHERE product_id = ? AND state = 'pr_open' AND pr_url <> ''`, pid)
	if err != nil {
		log.Printf("knowledge: reconcile publications (product %d): %v", pid, err)
		return
	}
	var urls []string
	for rs.Next() {
		var u string
		if rs.Scan(&u) == nil {
			urls = append(urls, u)
		}
	}
	rs.Close()
	for _, u := range urls {
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		st, err := pr.PRStatus(cctx, root.String, u)
		cancel()
		if err != nil {
			log.Printf("knowledge: reconcile publications (product %d): pr status: %v", pid, err)
			continue
		}
		switch st.State {
		case "CLOSED":
			if _, err := s.DB.Exec(ctx, `UPDATE kb_publications SET state = 'failed', error = 'the pull request was closed without merging', repo_code = NULL WHERE product_id = ? AND pr_url = ? AND state = 'pr_open'`, pid, u); err != nil {
				log.Printf("knowledge: reconcile publications (product %d): %v", pid, err)
			}
		case "MERGED":
			n, err := s.reconcileMerged(ctx, pid, u)
			if err != nil {
				log.Printf("knowledge: reconcile publications (product %d): %v", pid, err)
			} else if n > 0 {
				log.Printf("knowledge: product %d: %d published draft(s) moved to their repo codes", pid, n)
			}
		}
	}
}

// reconcileMerged moves the drafts of one merged PR whose repo code has been synced. The rest stay
// pr_open and are tried again after the next sync.
func (s *Syncer) reconcileMerged(ctx context.Context, pid int, prURL string) (int, error) {
	moved := 0
	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		moved = 0
		rs, err := tx.QueryContext(ctx, `SELECT id, entity_type, local_code, COALESCE(repo_code, '') FROM kb_publications WHERE product_id = ? AND pr_url = ? AND state = 'pr_open' ORDER BY id FOR UPDATE`, pid, prURL)
		if err != nil {
			return err
		}
		type pub struct {
			id            int64
			et, local, to string
		}
		var pubs []pub
		for rs.Next() {
			var p pub
			if err := rs.Scan(&p.id, &p.et, &p.local, &p.to); err != nil {
				rs.Close()
				return err
			}
			pubs = append(pubs, p)
		}
		if err := rs.Err(); err != nil {
			rs.Close()
			return err
		}
		rs.Close()
		// Stories first: a case's story_code is re-pointed before the story row goes.
		sort.SliceStable(pubs, func(i, j int) bool { return pubs[i].et == TypeStory && pubs[j].et != TypeStory })
		for _, p := range pubs {
			if p.to == "" {
				continue
			}
			var origin string
			err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT origin FROM %s WHERE product_id = ? AND code = ? AND removed_at IS NULL", tables[p.et].name), pid, p.to).Scan(&origin)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && origin != "sync") {
				continue // not synced yet
			}
			if err != nil {
				return err
			}
			if err := s.rekey(ctx, tx, pid, p.et, p.local, p.to); err != nil {
				return fmt.Errorf("move %s to %s: %w", p.local, p.to, err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE kb_publications SET state = 'merged' WHERE id = ?`, p.id); err != nil {
				return err
			}
			moved++
		}
		return nil
	})
	return moved, err
}

// rekey moves everything keyed by a draft code to the repo code and deletes the draft. An `original`
// translation whose English text differs from the repo's reads as stale by itself (src_hash).
func (s *Syncer) rekey(ctx context.Context, tx *sql.Tx, pid int, et, from, to string) error {
	exec := func(q string, args ...any) error { _, err := tx.ExecContext(ctx, q, args...); return err }
	if et == TypeTestCase {
		for _, t := range []string{"kb_case_results", "kb_case_verifications", "kb_case_verification_events", "kb_case_pr_links"} {
			if err := exec(fmt.Sprintf("UPDATE %s SET case_code = ? WHERE product_id = ? AND case_code = ?", t), to, pid, from); err != nil {
				return err
			}
		}
	}
	if err := exec(`UPDATE kb_annotations SET entity_code = ? WHERE product_id = ? AND entity_type = ? AND entity_code = ?`, to, pid, et, from); err != nil {
		return err
	}
	if err := exec(`UPDATE kb_translations SET code = ? WHERE product_id = ? AND entity_type = ? AND code = ?`, to, pid, et, from); err != nil {
		return err
	}
	if et == TypeStory {
		if err := exec(`UPDATE kb_test_cases SET story_code = ? WHERE product_id = ? AND story_code = ? AND origin = 'local'`, to, pid, from); err != nil {
			return err
		}
		if err := exec(`DELETE FROM kb_story_features WHERE product_id = ? AND story_code = ? AND origin = 'local'`, pid, from); err != nil {
			return err
		}
	}
	return exec(fmt.Sprintf("DELETE FROM %s WHERE product_id = ? AND code = ? AND origin = 'local'", tables[et].name), pid, from)
}
