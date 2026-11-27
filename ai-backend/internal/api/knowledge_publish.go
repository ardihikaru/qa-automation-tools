package api

// KNOWLEDGE_TRANSLATION.md, "Write-back design": local drafts (stories and test cases) become a pull
// request on the product's automation repo. Human-triggered, one epic per publish.
//
//	POST /api/products/{id}/knowledge/publish        {epic_code}  -> 202 {job_id}
//	GET  /api/products/{id}/knowledge/publications                -> 200 {publications:[{id,epic_code,state,pr_url,branch,job_id,codes}]}
//
// The work is a `drafts_publish` job (job_runs + job_events, shown on /jobs/:id). It reuses the
// case-automation git path (repopool worktree on a qa-auto/ branch, commit, push, pull request) and
// the repoWriteAllowed guard; it never pushes the default branch and never forces. The pure parts
// (codes, rendering, the round-trip gate) are knowledge.RenderDrafts; the state is kb_publications.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// JobTypeDraftsPublish is the job_runs type of a draft publish.
const JobTypeDraftsPublish = "drafts_publish"

const publishMaxDuration = 30 * time.Minute

// publishGit is the git side of a publish (*repopool.Pool; tests use a fake). Push and OpenPR are
// write-class: the job passes repoWriteAllowed before each.
type publishGit interface {
	FetchOrigin(ctx context.Context, rel string) error
	RemoteHead(ctx context.Context, rootDir, branch string) (string, error)
	AddWorktree(ctx context.Context, rootDir, branch, base string) (string, error)
	RemoveWorktree(ctx context.Context, rootDir, branch string) error
	Commit(ctx context.Context, wt, message string, paths []string) error
	Push(ctx context.Context, wt, branch string) error
	OpenPR(ctx context.Context, wt, base, head, title, body string) (string, error)
}

// publishRepo returns the git implementation, or nil when no repo pool is configured.
func (r *Router) publishRepo() publishGit {
	if r.pubGit != nil {
		return r.pubGit
	}
	if r.repoPool != nil {
		return r.repoPool
	}
	return nil
}

// handlePublish serves the publish and publications routes.
func (r *Router) handlePublish(w http.ResponseWriter, req *http.Request, pid int, action string, canWrite bool) {
	s := r.kb
	switch action {
	case "publications":
		if req.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		list, err := s.Publications(req.Context(), pid)
		respond(w, map[string]any{"publications": list}, err)
	case "publish":
		if req.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !canWrite {
			writeJSONError(w, http.StatusForbidden, "your role cannot change the knowledge base")
			return
		}
		var body struct {
			EpicCode string `json:"epic_code"`
		}
		if !decodeStrictJSON(w, req, 1<<12, &body) {
			return
		}
		r.startPublish(w, req, pid, strings.TrimSpace(body.EpicCode))
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func (r *Router) startPublish(w http.ResponseWriter, req *http.Request, pid int, epic string) {
	ctx := req.Context()
	d := r.getDB()
	git := r.publishRepo()
	if d == nil || git == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "publishing is not available (database or REPO_POOL_DIR missing)")
		return
	}
	if r.caseAuto.draining.Load() {
		w.Header().Set("Retry-After", "30")
		writeJSONError(w, http.StatusServiceUnavailable, "the server is restarting; try again in a moment")
		return
	}
	if epic == "" {
		writeJSONError(w, http.StatusBadRequest, "epic_code is required")
		return
	}
	repo, err := loadAutomationRepo(ctx, d, pid)
	if err != nil {
		log.Printf("publish: load repo: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if repo == nil {
		writeJSONError(w, http.StatusConflict, "the product has no automation repo")
		return
	}
	if err := publishGuard(ctx, r, repo.ID); err != nil {
		publishGuardError(w, err)
		return
	}
	r.pubMu.Lock()
	defer r.pubMu.Unlock()
	if id, err := r.kb.ActivePublishJob(ctx, pid); err != nil {
		log.Printf("publish: active check: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	} else if id != "" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": knowledge.ErrPublishActive.Error(), "job_id": id})
		return
	}
	if running, err := r.kb.SyncRunning(ctx, pid); err != nil {
		log.Printf("publish: sync check: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	} else if running {
		writeJSONError(w, http.StatusConflict, knowledge.ErrSyncRunning.Error())
		return
	}
	set, err := r.kb.CollectDrafts(ctx, pid, epic)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if set.Empty() {
		msg := "there are no drafts to publish in this epic"
		if len(set.Blockers) > 0 {
			msg = "no draft of this epic can be published yet"
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": msg, "blockers": set.Blockers})
		return
	}
	jobID := newJobID()
	if err := r.kb.QueuePublication(ctx, pid, jobID, userID(req), set); err != nil {
		log.Printf("publish: queue: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	payload := map[string]interface{}{"product_id": pid, "epic_code": epic, "triggered_by": userID(req), "pr_url": ""}
	if _, err := r.enqueueJobID(jobID, JobTypeDraftsPublish, payload, true); err != nil {
		log.Printf("publish: enqueue: %v", err)
		_ = r.kb.PublicationFailed(context.Background(), jobID, "the job could not be queued")
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID})
}

// publishGuard is the write guard of a publish: the three write-class operations must all be allowed.
func publishGuard(ctx context.Context, r *Router, repoID int) error {
	d := r.getDB()
	for _, op := range []RepoOp{RepoOpCreateBranch, RepoOpPush, RepoOpOpenPR} {
		if err := repoWriteAllowed(ctx, d, repoID, op); err != nil {
			return err
		}
	}
	return nil
}

func publishGuardError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrWriteNotAllowed):
		writeJSONError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrRepoNotFound):
		writeJSONError(w, http.StatusConflict, "the product has no automation repo")
	default:
		log.Printf("publish: guard: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	}
}

// ---------------------------------------------------------------- the job

const maxDocsBytes = 32 << 20

// readDocsTree reads the worktree's docs/*.md the way a sync reads the mirror (regular files only).
func readDocsTree(wt string) (map[string][]byte, error) {
	files := map[string][]byte{}
	total := 0
	root := filepath.Join(wt, "docs")
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() || !de.Type().IsRegular() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		rel, err := filepath.Rel(wt, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if total += len(b); total > maxDocsBytes {
			return errors.New("docs/ is too large to read")
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	return files, err
}

// writeCatalogFile replaces one file of the worktree (confined, never through a symlink).
func writeCatalogFile(wt, rel string, data []byte) error {
	abs, err := confined(caseauto.Workspace{Dir: wt}, rel)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func publishBranch(epic string, now time.Time) string {
	return repopool.AutoBranchPrefix + "kb-" + strings.ToLower(epic) + "-" + now.UTC().Format("200601021504")
}

// runDraftsPublish is the drafts_publish job body (spec steps 1 to 6 and 7: guard, fetch and sync
// precondition, branch and worktree, codes, render, round-trip gate, commit, push, pull request).
func (r *Router) runDraftsPublish(id string, payload map[string]interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), publishMaxDuration)
	defer cancel()
	defer registerJobCancel(id, cancel)()
	pid := intFrom(payload["product_id"])
	epic := str(payload, "epic_code")
	d := r.getDB()
	git := r.publishRepo()
	var repo *automationRepo
	branch := str(payload, "branch")
	ev := func(level, step, msg string) { r.addJobEvent(id, level, step, msg) }
	cleanup := func() {
		if git != nil && repo != nil && branch != "" {
			cctx, c := context.WithTimeout(context.Background(), 2*time.Minute)
			defer c()
			if err := git.RemoveWorktree(cctx, repo.RootDir, branch); err != nil {
				log.Printf("job %s (%s): removing the worktree: %v", id, JobTypeDraftsPublish, err)
			}
		}
	}
	fail := func(err error, step string) {
		msg := r.caseAuto.redact.Redact(err.Error())
		ev("error", step, msg)
		if r.kb != nil {
			if e := r.kb.PublicationFailed(context.Background(), id, msg); e != nil {
				log.Printf("job %s: marking the publication failed: %v", id, e)
			}
		}
		cleanup()
		r.finishJob(id, jobStatusFailed, caseauto.Clip(msg, 1000), nil)
		log.Printf("job %s (%s) failed: %s", id, JobTypeDraftsPublish, caseauto.Clip(msg, 200))
	}
	if d == nil || r.kb == nil || git == nil {
		fail(errors.New("publishing is not available (database or REPO_POOL_DIR missing)"), caseauto.StepPrepare)
		return
	}
	r.markJobRunning(id)
	if err := r.kb.PublicationRunning(ctx, id); err != nil {
		fail(err, caseauto.StepPrepare)
		return
	}
	var err error
	if repo, err = loadAutomationRepo(ctx, d, pid); err != nil || repo == nil {
		if err == nil {
			err = errors.New("the product has no automation repo")
		}
		fail(err, caseauto.StepPrepare)
		return
	}
	// Guard (spec step 1): before the branch exists, then again before push and PR.
	if err := repoWriteAllowed(ctx, d, repo.ID, RepoOpCreateBranch); err != nil {
		fail(err, caseauto.StepPrepare)
		return
	}

	// Preconditions (spec step 2).
	ev("info", caseauto.StepPrepare, "checking that the catalog is in sync with "+repo.DefaultBranch)
	if running, err := r.kb.SyncRunning(ctx, pid); err != nil || running {
		if err == nil {
			err = knowledge.ErrSyncRunning
		}
		fail(err, caseauto.StepPrepare)
		return
	}
	if err := git.FetchOrigin(ctx, repo.RootDir); err != nil {
		fail(fmt.Errorf("fetching the automation repo failed: %w", err), caseauto.StepPrepare)
		return
	}
	head, err := git.RemoteHead(ctx, repo.RootDir, repo.DefaultBranch)
	if err != nil {
		fail(fmt.Errorf("reading %s failed: %w", repo.DefaultBranch, err), caseauto.StepPrepare)
		return
	}
	last, err := r.kb.LatestSyncCommit(ctx, pid)
	if err != nil {
		fail(err, caseauto.StepPrepare)
		return
	}
	if last != head {
		short := func(s string) string {
			if len(s) > 7 {
				return s[:7]
			}
			return s
		}
		fail(fmt.Errorf("sync first: the knowledge base was last synced at %q, the repo's %s is at %s", short(last), repo.DefaultBranch, short(head)), caseauto.StepPrepare)
		return
	}
	set, err := r.kb.JobDrafts(ctx, pid, epic, id)
	if err != nil {
		fail(err, caseauto.StepPrepare)
		return
	}
	if len(set.Blockers) > 0 {
		var parts []string
		for _, b := range set.Blockers {
			parts = append(parts, b.Code+": "+b.Msg)
		}
		fail(fmt.Errorf("drafts cannot be published: %s", strings.Join(parts, "; ")), caseauto.StepPrepare)
		return
	}
	if set.Empty() {
		fail(errors.New("the drafts of this publish no longer exist"), caseauto.StepPrepare)
		return
	}

	// Branch and worktree (step 3). A restart reuses the branch name chosen first.
	if branch == "" {
		branch = publishBranch(epic, time.Now())
		r.setJobPayload(id, "branch", branch)
	}
	if err := repopool.ValidateBranch(branch); err != nil || branch == repo.DefaultBranch || !strings.HasPrefix(branch, repopool.AutoBranchPrefix) {
		fail(errors.New("invalid publish branch"), caseauto.StepPrepare)
		return
	}
	ev("info", caseauto.StepPrepare, "preparing the worktree on branch "+branch)
	wt, err := git.AddWorktree(ctx, repo.RootDir, branch, repo.DefaultBranch)
	if err != nil {
		fail(fmt.Errorf("could not prepare the worktree: %w", err), caseauto.StepPrepare)
		return
	}

	// Codes, render and the round-trip gate (steps 4 to 6).
	files, err := readDocsTree(wt)
	if err != nil {
		fail(fmt.Errorf("reading the catalog failed: %w", err), caseauto.StepWrite)
		return
	}
	taken, err := r.kb.TakenCodes(ctx, pid, epic, id)
	if err != nil {
		fail(err, caseauto.StepWrite)
		return
	}
	rendered, err := knowledge.RenderDrafts(files, knowledge.RenderRequest{EpicCode: epic, Stories: set.Stories, Cases: set.Cases, Taken: taken})
	if err != nil {
		fail(fmt.Errorf("rendering the drafts failed: %w", err), caseauto.StepWrite)
		return
	}
	if err := writeCatalogFile(wt, rendered.File, rendered.Content); err != nil {
		fail(fmt.Errorf("writing %s failed: %w", rendered.File, err), caseauto.StepWrite)
		return
	}
	codes := map[string]string{}
	for _, p := range rendered.Pairs {
		codes[p.Local] = p.Repo
	}
	if err := r.kb.PublicationAssign(ctx, id, branch, rendered.Pairs); err != nil {
		fail(err, caseauto.StepWrite)
		return
	}
	ev("info", caseauto.StepWrite, fmt.Sprintf("wrote %d stories and %d test cases to %s; the parser reads them back unchanged", len(set.Stories), len(set.Cases), rendered.File))
	for _, p := range rendered.Pairs {
		ev("info", caseauto.StepWrite, p.Local+" -> "+p.Repo)
	}

	// Commit, push, pull request.
	ev("info", caseauto.StepPush, "committing and pushing "+branch)
	msg := fmt.Sprintf("docs(catalog): add %d stories and %d test cases to %s", len(set.Stories), len(set.Cases), epic)
	if err := git.Commit(ctx, wt, msg, []string{rendered.File}); err != nil {
		fail(fmt.Errorf("commit failed: %w", err), caseauto.StepPush)
		return
	}
	if err := repoWriteAllowed(ctx, d, repo.ID, RepoOpPush); err != nil {
		fail(err, caseauto.StepPush)
		return
	}
	if err := git.Push(ctx, wt, branch); err != nil {
		fail(fmt.Errorf("push failed: %w", err), caseauto.StepPush)
		return
	}
	ev("info", caseauto.StepPR, "opening the pull request")
	if err := repoWriteAllowed(ctx, d, repo.ID, RepoOpOpenPR); err != nil {
		fail(err, caseauto.StepPR)
		return
	}
	title := fmt.Sprintf("docs(catalog): publish QA tool drafts for epic %s", epic)
	prURL, err := git.OpenPR(ctx, wt, repo.DefaultBranch, branch, title, r.publishPRBody(ctx, d, id, epic, str(payload, "triggered_by"), rendered.Pairs))
	if err != nil {
		fail(fmt.Errorf("opening the pull request failed: %w", err), caseauto.StepPR)
		return
	}
	r.setJobPayload(id, "pr_url", prURL)
	if err := r.kb.PublicationOpened(context.Background(), id, prURL); err != nil {
		log.Printf("job %s: recording the pull request: %v", id, err)
	}
	ev("info", caseauto.StepDone, "pull request opened: "+prURL)
	cleanup()
	r.finishJob(id, jobStatusCompleted, "", map[string]interface{}{"pr_url": prURL, "branch": branch, "epic_code": epic, "codes": codes})
	log.Printf("job %s (%s) completed: PR opened", id, JobTypeDraftsPublish)
}

// publishPRBody lists the local to repo codes and the author. It passes the redactor.
func (r *Router) publishPRBody(ctx context.Context, d *db.DB, jobID, epic, userID string, pairs []knowledge.CodePair) string {
	author := "the QA tool"
	if userID != "" {
		var name sql.NullString
		if err := d.QueryRow(ctx, `SELECT COALESCE(NULLIF(full_name, ''), username) FROM users WHERE id = ?`, userID).Scan(&name); err == nil && name.String != "" {
			author = name.String
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Drafts of epic %s published from the QA tool by %s (job %s). The text is English; each draft code maps to the repo code it was given:\n\n", epic, author, jobID)
	b.WriteString("| Draft | Repo code |\n| --- | --- |\n")
	for _, p := range pairs {
		fmt.Fprintf(&b, "| %s | %s |\n", p.Local, p.Repo)
	}
	b.WriteString("\nAfter this is merged and the catalog is synced, results, verifications and notes of each draft move to its repo code. Closing the PR without merging leaves the drafts in the QA tool.\n")
	return r.caseAuto.redact.Redact(b.String())
}
