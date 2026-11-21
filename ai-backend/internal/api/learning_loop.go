package api

// LEARNING_LOOP.md: findings from real automation work become policy proposals (policy_propose
// jobs), approved by a human, published as one .claude/rules/learned-<slug>.md file per policy
// on the product's automation repo, and tracked until the PR is merged.
//
//	GET  /api/learning/findings[?product_id=&status=&kind=&q=&limit=&offset=]
//	POST /api/learning/findings                       {product_id, title, detail, case_code?, job_id?, run_ref?}
//	GET  /api/learning/findings/{id}
//	POST /api/learning/findings/{id}/dismiss|reopen   {reason?}
//	GET  /api/learning/proposals[?product_id=&state=&limit=&offset=]
//	POST /api/learning/proposals                      {product_id, finding_ids}
//	POST /api/learning/proposals/{job id}/check
//	GET  /api/learning/timeline[?product_id=&limit=]
//
// The approval of a proposal is the existing POST /api/jobs/{id}/approval (and the GAOS Inbox).
// /api/learning and /api/learning/navigation (the old crawler's routes) are unchanged.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/learnloop"
)

// learnState is the router's state for the learning loop.
type learnState struct {
	publishMu   sync.Mutex // serialises slug choice + file write across proposals
	tickMu      sync.Mutex
	ticking     bool
	lastHarvest time.Time
	// Test seams (nil/zero = production values).
	llm          caseauto.LLM
	harvestEvery time.Duration
	approvalPoll time.Duration
}

// ci compares job ids across tables created with different default collations.
const ci = " COLLATE utf8mb4_general_ci"

const (
	learnHarvestEvery = 10 * time.Minute
	learnPRCheckEvery = 10 * time.Minute
	learnPRPerTick    = 5
	learnLabel        = "ai-assisted"
)

func (r *Router) learnLLM() caseauto.LLM {
	if r.learn.llm != nil {
		return r.learn.llm
	}
	if r.aiService == nil || !r.aiService.Configured() {
		return nil
	}
	return r.aiService
}

// ---------- routing ----------

var learnPathRe = regexp.MustCompile(`^/api/learning/(findings|proposals|timeline)(?:/([A-Za-z0-9-]{1,191}))?(?:/(dismiss|reopen|check))?$`)

func (r *Router) handleLearningLoop(w http.ResponseWriter, req *http.Request) {
	m := learnPathRe.FindStringSubmatch(req.URL.Path)
	if m == nil {
		writeCodedError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	d := r.getDB()
	if d == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unconfigured", "database unavailable")
		return
	}
	coll, id, action := m[1], m[2], m[3]
	_, canWrite := productRole(req)
	method := req.Method
	write := method == http.MethodPost
	if write && !canWrite {
		writeCodedError(w, http.StatusForbidden, "forbidden", "your role cannot change learning data")
		return
	}
	switch {
	case coll == "timeline" && id == "" && action == "" && method == http.MethodGet:
		r.learnTimeline(w, req, d)
	case coll == "findings" && id == "" && action == "" && method == http.MethodGet:
		r.learnListFindings(w, req, d)
	case coll == "findings" && id == "" && action == "" && write:
		r.learnAddFinding(w, req, d)
	case coll == "findings" && id != "" && action == "" && method == http.MethodGet:
		r.learnGetFinding(w, req, d, id)
	case coll == "findings" && id != "" && (action == "dismiss" || action == "reopen") && write:
		r.learnFindingStatus(w, req, d, id, action)
	case coll == "proposals" && id == "" && action == "" && method == http.MethodGet:
		r.learnListProposals(w, req, d)
	case coll == "proposals" && id == "" && action == "" && write:
		r.learnCreateProposal(w, req, d)
	case coll == "proposals" && id != "" && action == "check" && write:
		r.learnCheckProposal(w, req, d, id)
	default:
		writeCodedError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// queryInt reads an optional non-negative integer; ok is false on a bad value.
func queryInt(req *http.Request, key string, def, max int) (int, bool) {
	s := strings.TrimSpace(req.URL.Query().Get(key))
	if s == "" {
		return def, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || (max > 0 && n > max) {
		return 0, false
	}
	return n, true
}

func productExists(ctx context.Context, d *db.DB, pid int) (bool, error) {
	var one int
	err := d.QueryRow(ctx, `SELECT 1 FROM products WHERE id = ?`, pid).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ---------- findings ----------

func (r *Router) learnListFindings(w http.ResponseWriter, req *http.Request, d *db.DB) {
	q := req.URL.Query()
	pid, ok1 := queryInt(req, "product_id", 0, 0)
	limit, ok2 := queryInt(req, "limit", 50, 200)
	offset, ok3 := queryInt(req, "offset", 0, 0)
	status, kind := q.Get("status"), q.Get("kind")
	if !ok1 || !ok2 || !ok3 || (status != "" && !oneOf(status, learnloop.FindingStatuses...)) || (kind != "" && !oneOf(kind, learnloop.Kinds...)) || len(q.Get("q")) > 200 {
		writeCodedError(w, http.StatusBadRequest, "bad_input", "invalid filter")
		return
	}
	items, total, counts, err := learnloop.ListFindings(req.Context(), d, learnloop.FindingFilter{ProductID: pid, Status: status, Kind: kind, Q: q.Get("q"), Limit: limit, Offset: offset})
	if err != nil {
		log.Printf("learning: list findings: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "counts": counts})
}

func parseFindingID(w http.ResponseWriter, s string) (int64, bool) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		writeCodedError(w, http.StatusNotFound, "not_found", "finding not found")
		return 0, false
	}
	return id, true
}

func (r *Router) learnGetFinding(w http.ResponseWriter, req *http.Request, d *db.DB, s string) {
	id, ok := parseFindingID(w, s)
	if !ok {
		return
	}
	f, ev, err := learnloop.GetFinding(req.Context(), d, id)
	if errors.Is(err, learnloop.ErrNotFound) {
		writeCodedError(w, http.StatusNotFound, "not_found", "finding not found")
		return
	}
	if err != nil {
		log.Printf("learning: get finding: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"finding": f, "evidence": ev})
}

type addFindingBody struct {
	ProductID int    `json:"product_id"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	CaseCode  string `json:"case_code"`
	JobID     string `json:"job_id"`
	RunRef    string `json:"run_ref"`
}

var jobIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,191}$`)

func (r *Router) learnAddFinding(w http.ResponseWriter, req *http.Request, d *db.DB) {
	var b addFindingBody
	if !decodeStrictJSON(w, req, 1<<15, &b) {
		return
	}
	b.Title, b.Detail = strings.TrimSpace(b.Title), strings.TrimSpace(b.Detail)
	b.CaseCode, b.JobID, b.RunRef = strings.TrimSpace(b.CaseCode), strings.TrimSpace(b.JobID), strings.TrimSpace(b.RunRef)
	switch {
	case b.Title == "" || len(b.Title) > 200 || strings.ContainsAny(b.Title, "\r\n"):
		writeCodedError(w, http.StatusBadRequest, "bad_input", "title must be one line of 1-200 characters")
		return
	case b.Detail == "" || len(b.Detail) > 2000:
		writeCodedError(w, http.StatusBadRequest, "bad_input", "detail must be 1-2000 characters")
		return
	case b.CaseCode != "" && !caseauto.CaseCodeRe.MatchString(b.CaseCode):
		writeCodedError(w, http.StatusBadRequest, "bad_input", "case_code must look like TC-XXX-001")
		return
	case b.JobID != "" && !jobIDRe.MatchString(b.JobID):
		writeCodedError(w, http.StatusBadRequest, "bad_input", "invalid job_id")
		return
	case len(b.RunRef) > 120:
		writeCodedError(w, http.StatusBadRequest, "bad_input", "run_ref is limited to 120 characters")
		return
	}
	ctx := req.Context()
	if ok, err := productExists(ctx, d, b.ProductID); err != nil || !ok {
		writeCodedError(w, http.StatusBadRequest, "bad_input", "unknown product")
		return
	}
	red := r.caseAuto.redact
	c := learnloop.Candidate{ProductID: b.ProductID, Kind: learnloop.KindManual, SourceKey: learnloop.ManualSourceKey(), JobID: b.JobID, CaseCode: b.CaseCode,
		RunRef: b.RunRef, Lesson: red.Redact(b.Detail), Excerpt: red.Redact(b.Detail), ObservedAt: time.Now().UTC(), CreatedBy: userID(req), Title: red.Redact(b.Title)}
	id, created, _, err := learnloop.Ingest(ctx, d, c)
	if err != nil {
		log.Printf("learning: add finding: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	f, _, err := learnloop.GetFinding(ctx, d, id)
	if err != nil {
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	log.Printf("learning: finding %d added by user %s (merged=%v)", id, userID(req), !created)
	writeJSON(w, http.StatusCreated, map[string]any{"finding": f, "merged": !created})
}

func (r *Router) learnFindingStatus(w http.ResponseWriter, req *http.Request, d *db.DB, s, action string) {
	id, ok := parseFindingID(w, s)
	if !ok {
		return
	}
	var b struct {
		Reason string `json:"reason"`
	}
	if req.ContentLength != 0 && !decodeStrictJSON(w, req, 1<<12, &b) {
		return
	}
	from, to, note := []string{learnloop.FindingNew}, learnloop.FindingDismissed, strings.TrimSpace(b.Reason)
	if action == "reopen" {
		from, to, note = []string{learnloop.FindingDismissed, learnloop.FindingCovered}, learnloop.FindingNew, "reopened"
	}
	if note == "" {
		note = action + "ed"
	}
	err := learnloop.SetFindingStatus(req.Context(), d, id, from, to, r.caseAuto.redact.Redact(note))
	switch {
	case errors.Is(err, learnloop.ErrNotFound):
		writeCodedError(w, http.StatusNotFound, "not_found", "finding not found")
		return
	case errors.Is(err, learnloop.ErrBadTransition):
		writeCodedError(w, http.StatusConflict, "bad_transition", "the finding cannot be "+map[string]string{"dismiss": "dismissed", "reopen": "reopened"}[action]+" in its current status")
		return
	case err != nil:
		log.Printf("learning: %s finding: %v", action, err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	f, _, _ := learnloop.GetFinding(req.Context(), d, id)
	writeJSON(w, http.StatusOK, f)
}

// ---------- proposals ----------

// Proposal is a policy_propose job as the page shows it.
type Proposal struct {
	ID          string     `json:"id"`
	ProductID   int        `json:"product_id"`
	ProductName string     `json:"product_name"`
	State       string     `json:"state"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
	TriggeredBy string     `json:"triggered_by"`
	FindingIDs  []int64    `json:"finding_ids"`
	Title       string     `json:"title"`
	Slug        string     `json:"slug"`
	FilePath    string     `json:"file_path"`
	Policy      string     `json:"policy"`
	Rationale   string     `json:"rationale"`
	CoveredBy   string     `json:"covered_by"`
	Content     string     `json:"content"`
	Branch      string     `json:"branch"`
	PRURL       string     `json:"pr_url"`
	PRState     string     `json:"pr_state"`
	MergeCommit string     `json:"merge_commit"`
	MergedAt    string     `json:"merged_at"`
	CheckedAt   string     `json:"pr_checked_at"`
	InRepo      bool       `json:"in_repo"`
	Error       string     `json:"error"`
}

func str(m map[string]interface{}, k string) string {
	s, _ := m[k].(string)
	return s
}

func int64s(v interface{}) []int64 {
	var out []int64
	switch xs := v.(type) {
	case []interface{}:
		for _, x := range xs {
			switch n := x.(type) {
			case float64:
				out = append(out, int64(n))
			case int64:
				out = append(out, n)
			case int:
				out = append(out, int64(n))
			case json.Number:
				if i, err := n.Int64(); err == nil {
					out = append(out, i)
				}
			}
		}
	case []int64:
		out = append(out, xs...)
	}
	return out
}

// loadProposals reads the newest 500 policy_propose jobs (of one product when pid > 0).
func loadProposals(ctx context.Context, d *db.DB, pid int, jobID string) ([]Proposal, error) {
	where, args := "j.type = ?", []any{learnloop.JobType}
	if pid > 0 {
		where += " AND CAST(JSON_EXTRACT(j.payload, '$.product_id') AS UNSIGNED) = ?"
		args = append(args, pid)
	}
	if jobID != "" {
		where += " AND j.id = ?"
		args = append(args, jobID)
	}
	rows, err := d.Query(ctx, `SELECT j.id, j.status, COALESCE(j.error, ''), j.payload, j.result, j.created_at, j.completed_at,
		EXISTS(SELECT 1 FROM case_approvals a WHERE a.job_id`+ci+` = j.id`+ci+` AND a.status = 'open'),
		EXISTS(SELECT 1 FROM job_events e WHERE e.job_id`+ci+` = j.id`+ci+` AND e.step IN ('push', 'pr'))
		FROM job_runs j WHERE `+where+` ORDER BY j.created_at DESC, j.id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	var out []Proposal
	for rows.Next() {
		var p Proposal
		var payloadRaw, resultRaw []byte
		var completed sql.NullTime
		var open, pushed bool
		if err := rows.Scan(&p.ID, &p.Status, &p.Error, &payloadRaw, &resultRaw, &p.CreatedAt, &completed, &open, &pushed); err != nil {
			rows.Close()
			return nil, err
		}
		payload, result := map[string]interface{}{}, map[string]interface{}{}
		_ = json.Unmarshal(payloadRaw, &payload)
		_ = json.Unmarshal(resultRaw, &result)
		p.CreatedAt = p.CreatedAt.UTC()
		if completed.Valid {
			t := completed.Time.UTC()
			p.CompletedAt = &t
		}
		p.ProductID = intFrom(payload["product_id"])
		p.TriggeredBy = str(payload, "triggered_by")
		p.FindingIDs = int64s(payload["finding_ids"])
		if dr, ok := payload["draft"].(map[string]interface{}); ok {
			p.Title, p.Slug, p.Policy, p.Rationale, p.CoveredBy = str(dr, "title"), str(dr, "slug"), str(dr, "policy"), str(dr, "rationale"), str(dr, "covered_by")
		}
		p.FilePath = str(result, "file_path")
		if p.FilePath == "" && p.Slug != "" {
			p.FilePath = str(payload, "spec_path")
		}
		p.Content = str(payload, "spec_content")
		p.Branch = str(payload, "branch")
		p.PRURL = str(result, "pr_url")
		if p.PRURL == "" {
			p.PRURL = str(payload, "pr_url")
		}
		p.PRState, p.MergeCommit, p.MergedAt, p.CheckedAt = str(result, "pr_state"), str(result, "merge_commit"), str(result, "merged_at"), str(result, "pr_checked_at")
		if c := str(result, "covered_by"); c != "" {
			p.CoveredBy = c
		}
		p.State = learnloop.ProposalState(learnloop.JobFacts{Status: p.Status, Error: p.Error, OpenApproval: open, PushReached: pushed,
			Decision: str(result, "decision"), PRURL: str(result, "pr_url"), PRState: p.PRState})
		if p.FindingIDs == nil {
			p.FindingIDs = []int64{}
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names := map[int]string{}
	if prow, err := d.Query(ctx, `SELECT id, name FROM products`); err == nil {
		for prow.Next() {
			var id int
			var n string
			if prow.Scan(&id, &n) == nil {
				names[id] = n
			}
		}
		prow.Close()
	}
	inRepo := map[string]bool{}
	if arow, err := d.Query(ctx, `SELECT product_id, path FROM kb_assistants WHERE kind = 'rule' AND removed_at IS NULL AND path LIKE '.claude/rules/learned-%'`); err == nil {
		for arow.Next() {
			var id int
			var path string
			if arow.Scan(&id, &path) == nil {
				inRepo[fmt.Sprintf("%d|%s", id, path)] = true
			}
		}
		arow.Close()
	}
	for i := range out {
		out[i].ProductName = names[out[i].ProductID]
		out[i].InRepo = out[i].FilePath != "" && inRepo[fmt.Sprintf("%d|%s", out[i].ProductID, out[i].FilePath)]
	}
	return out, nil
}

func (r *Router) learnListProposals(w http.ResponseWriter, req *http.Request, d *db.DB) {
	pid, ok1 := queryInt(req, "product_id", 0, 0)
	limit, ok2 := queryInt(req, "limit", 50, 200)
	offset, ok3 := queryInt(req, "offset", 0, 0)
	state := req.URL.Query().Get("state")
	if !ok1 || !ok2 || !ok3 || (state != "" && !oneOf(state, learnloop.ProposalStates...)) {
		writeCodedError(w, http.StatusBadRequest, "bad_input", "invalid filter")
		return
	}
	all, err := loadProposals(req.Context(), d, pid, "")
	if err != nil {
		log.Printf("learning: list proposals: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	counts := map[string]int{}
	for _, s := range learnloop.ProposalStates {
		counts[s] = 0
	}
	items := []Proposal{}
	for _, p := range all {
		counts[p.State]++
		if state == "" || p.State == state {
			items = append(items, p)
		}
	}
	total := len(items)
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "counts": counts})
}

type createProposalBody struct {
	ProductID  int     `json:"product_id"`
	FindingIDs []int64 `json:"finding_ids"`
}

func (r *Router) learnCreateProposal(w http.ResponseWriter, req *http.Request, d *db.DB) {
	var b createProposalBody
	if !decodeStrictJSON(w, req, 1<<12, &b) {
		return
	}
	seen := map[int64]bool{}
	for _, id := range b.FindingIDs {
		if id <= 0 || seen[id] {
			writeCodedError(w, http.StatusBadRequest, "bad_input", "finding_ids must be distinct positive ids")
			return
		}
		seen[id] = true
	}
	if len(b.FindingIDs) < 1 || len(b.FindingIDs) > 10 {
		writeCodedError(w, http.StatusBadRequest, "bad_input", "select 1 to 10 findings")
		return
	}
	ctx := req.Context()
	if r.learnLLM() == nil || r.repoPool == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unconfigured", "the learning loop needs AI and the repo pool")
		return
	}
	if ok, err := productExists(ctx, d, b.ProductID); err != nil || !ok {
		writeCodedError(w, http.StatusBadRequest, "bad_input", "unknown product")
		return
	}
	repo, err := loadAutomationRepo(ctx, d, b.ProductID)
	if err != nil {
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if repo == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unconfigured", "the product has no automation repo")
		return
	}
	if r.caseAuto.draining.Load() {
		w.Header().Set("Retry-After", "30")
		writeCodedError(w, http.StatusServiceUnavailable, "draining", "the server is restarting; try again in a moment")
		return
	}
	jobID := newJobID()
	var busy *learnloop.ErrBusy
	switch err := learnloop.Reserve(ctx, d, b.ProductID, b.FindingIDs, jobID); {
	case errors.As(err, &busy):
		writeCodedError(w, http.StatusConflict, "finding_busy", busy.Error())
		return
	case errors.Is(err, learnloop.ErrNotFound):
		writeCodedError(w, http.StatusBadRequest, "bad_input", "unknown finding id")
		return
	case err != nil:
		log.Printf("learning: reserve findings: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	ids := make([]interface{}, len(b.FindingIDs))
	for i, id := range b.FindingIDs {
		ids[i] = id
	}
	payload := map[string]interface{}{"product_id": b.ProductID, "finding_ids": ids, "triggered_by": userID(req), "pr_url": "",
		"branch": "qa-auto/learn-" + strings.ToLower(jobID)}
	if _, err := r.enqueueJobID(jobID, learnloop.JobType, payload, true); err != nil {
		_ = learnloop.Release(ctx, d, jobID)
		log.Printf("learning: enqueue proposal: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	log.Printf("learning: proposal %s started by user %s for product %d (%d findings)", jobID, userID(req), b.ProductID, len(b.FindingIDs))
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID})
}

func (r *Router) learnCheckProposal(w http.ResponseWriter, req *http.Request, d *db.DB, jobID string) {
	ps, err := loadProposals(req.Context(), d, 0, jobID)
	if err != nil {
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if len(ps) == 0 {
		writeCodedError(w, http.StatusNotFound, "not_found", "proposal not found")
		return
	}
	if ps[0].State != learnloop.StatePROpen {
		writeCodedError(w, http.StatusConflict, "bad_transition", "only a proposal with an open pull request can be checked")
		return
	}
	if r.repoPool == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unconfigured", "the repo pool is not configured")
		return
	}
	if err := r.checkProposalPR(req.Context(), d, ps[0]); err != nil {
		writeCodedError(w, http.StatusBadGateway, "pr_check_failed", "could not read the pull request: "+firstLine(err))
		return
	}
	ps, _ = loadProposals(req.Context(), d, 0, jobID)
	writeJSON(w, http.StatusOK, ps[0])
}

// checkProposalPR polls the PR of one proposal and records its state (decision 10).
func (r *Router) checkProposalPR(ctx context.Context, d *db.DB, p Proposal) error {
	repo, err := loadAutomationRepo(ctx, d, p.ProductID)
	if err != nil || repo == nil {
		return errors.New("the product has no automation repo")
	}
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	st, perr := r.repoPool.PRStatus(cctx, repo.RootDir, p.PRURL)
	now := time.Now().UTC().Format(time.RFC3339)
	if perr != nil {
		_, _ = d.Exec(ctx, `UPDATE job_runs SET result = JSON_SET(COALESCE(result, JSON_OBJECT()), '$.pr_checked_at', ?) WHERE id = ?`, now, p.ID)
		return perr
	}
	_, err = d.Exec(ctx, `UPDATE job_runs SET result = JSON_SET(COALESCE(result, JSON_OBJECT()), '$.pr_checked_at', ?, '$.pr_state', ?, '$.merge_commit', ?, '$.merged_at', ?)
		WHERE id = ? AND type = ?`, now, st.State, st.MergeCommit, st.MergedAt, p.ID, learnloop.JobType)
	if err != nil {
		return err
	}
	r.jobs.mu.Lock()
	if rec := r.jobs.jobs[p.ID]; rec != nil {
		if rec.Result == nil {
			rec.Result = map[string]interface{}{}
		}
		rec.Result["pr_checked_at"], rec.Result["pr_state"], rec.Result["merge_commit"], rec.Result["merged_at"] = now, st.State, st.MergeCommit, st.MergedAt
	}
	r.jobs.mu.Unlock()
	switch st.State {
	case "MERGED":
		log.Printf("learning: proposal %s merged (%s)", p.ID, st.MergeCommit)
		return learnloop.LearnFindings(ctx, d, p.ID)
	case "CLOSED":
		log.Printf("learning: proposal %s closed without merge", p.ID)
		_, err := learnloop.Reconcile(ctx, d)
		return err
	}
	return nil
}

// ---------- timeline ----------

// TimelineItem is one moment of the learning history.
type TimelineItem struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"` // finding_new | finding_<status> | proposal_created | proposal_<state>
	ProductID   int       `json:"product_id"`
	ProductName string    `json:"product_name"`
	FindingID   int64     `json:"finding_id,omitempty"`
	FindingKind string    `json:"finding_kind,omitempty"`
	JobID       string    `json:"job_id,omitempty"`
	Title       string    `json:"title"`
	State       string    `json:"state,omitempty"`
	PRURL       string    `json:"pr_url,omitempty"`
	FilePath    string    `json:"file_path,omitempty"`
	CaseCodes   []string  `json:"case_codes,omitempty"`
	Occurrences int       `json:"occurrences,omitempty"`
}

func (r *Router) learnTimeline(w http.ResponseWriter, req *http.Request, d *db.DB) {
	pid, ok1 := queryInt(req, "product_id", 0, 0)
	limit, ok2 := queryInt(req, "limit", 100, 200)
	offset, ok3 := queryInt(req, "offset", 0, 0)
	if !ok1 || !ok2 || !ok3 {
		writeCodedError(w, http.StatusBadRequest, "bad_input", "invalid filter")
		return
	}
	ctx := req.Context()
	findings, _, _, err := learnloop.ListFindings(ctx, d, learnloop.FindingFilter{ProductID: pid, Limit: 200})
	if err != nil {
		log.Printf("learning: timeline: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	props, err := loadProposals(ctx, d, pid, "")
	if err != nil {
		log.Printf("learning: timeline: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := []TimelineItem{}
	for _, f := range findings {
		var codes []string
		for _, s := range f.Sources {
			if s.CaseCode != "" && !oneOf(s.CaseCode, codes...) {
				codes = append(codes, s.CaseCode)
			}
		}
		items = append(items, TimelineItem{At: f.FirstSeenAt, Kind: "finding_new", ProductID: f.ProductID, ProductName: f.ProductName, FindingID: f.ID,
			FindingKind: f.Kind, Title: f.Title, CaseCodes: codes, Occurrences: f.Occurrences, JobID: firstJob(f.Sources)})
		if f.Status == learnloop.FindingDismissed || f.Status == learnloop.FindingCovered || f.Status == learnloop.FindingLearned {
			items = append(items, TimelineItem{At: f.UpdatedAt, Kind: "finding_" + f.Status, ProductID: f.ProductID, ProductName: f.ProductName, FindingID: f.ID, FindingKind: f.Kind, Title: f.Title})
		}
	}
	for _, p := range props {
		title := p.Title
		if title == "" {
			title = fmt.Sprintf("Policy proposal from %d finding(s)", len(p.FindingIDs))
		}
		items = append(items, TimelineItem{At: p.CreatedAt, Kind: "proposal_created", ProductID: p.ProductID, ProductName: p.ProductName, JobID: p.ID, Title: title, State: p.State})
		var at time.Time
		switch p.State {
		case learnloop.StateLearned:
			if t, err := time.Parse(time.RFC3339, p.MergedAt); err == nil {
				at = t.UTC()
			} else if p.CompletedAt != nil {
				at = *p.CompletedAt
			}
		case learnloop.StatePROpen, learnloop.StateCovered, learnloop.StateRejected, learnloop.StateClosed, learnloop.StateFailed:
			if p.CompletedAt != nil {
				at = *p.CompletedAt
			}
		}
		if !at.IsZero() {
			items = append(items, TimelineItem{At: at, Kind: "proposal_" + p.State, ProductID: p.ProductID, ProductName: p.ProductName, JobID: p.ID, Title: title,
				State: p.State, PRURL: p.PRURL, FilePath: p.FilePath})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.After(items[j].At) })
	// total counts the derived entries (from the newest 200 findings and every proposal) so the
	// dashboard can page through them with offset.
	total := len(items)
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func firstJob(s []learnloop.Source) string {
	for _, x := range s {
		if x.JobID != "" {
			return x.JobID
		}
	}
	return ""
}

// ---------- the proposal job ----------

// repoFiles reads the synced AGENTS.md and .claude/rules/*.md of the product (kb_assistants).
func repoFiles(ctx context.Context, d *db.DB, pid int) (guide string, rules []learnloop.RepoFile, err error) {
	rows, err := d.Query(ctx, `SELECT path, name, COALESCE(body, '') FROM kb_assistants
		WHERE product_id = ? AND removed_at IS NULL AND (path = 'AGENTS.md' OR (kind = 'rule' AND path LIKE '.claude/rules/%')) ORDER BY path`, pid)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f learnloop.RepoFile
		if err := rows.Scan(&f.Path, &f.Title, &f.Body); err != nil {
			return "", nil, err
		}
		if f.Path == "AGENTS.md" {
			guide = f.Body
			continue
		}
		rules = append(rules, f)
	}
	return guide, rules, rows.Err()
}

// proposalJobFor builds the learnloop job from the database.
func (r *Router) proposalJobFor(ctx context.Context, d *db.DB, id string, payload map[string]interface{}) (learnloop.ProposalJob, *automationRepo, error) {
	pid := intFrom(payload["product_id"])
	job := learnloop.ProposalJob{ID: id, ProductID: pid, Branch: str(payload, "branch")}
	if job.Branch == "" {
		job.Branch = "qa-auto/learn-" + strings.ToLower(id)
	}
	repo, err := loadAutomationRepo(ctx, d, pid)
	if err != nil {
		return job, nil, err
	}
	if repo == nil {
		return job, nil, errors.New("the product has no automation repo")
	}
	if err := d.QueryRow(ctx, `SELECT name FROM products WHERE id = ?`, pid).Scan(&job.ProductName); err != nil {
		return job, nil, err
	}
	if job.Findings, err = learnloop.Briefs(ctx, d, int64s(payload["finding_ids"])); err != nil {
		return job, nil, err
	}
	if job.Guide, job.Rules, err = repoFiles(ctx, d, pid); err != nil {
		return job, nil, err
	}
	var gaosOn bool
	var gaosPID sql.NullString
	_ = d.QueryRow(ctx, `SELECT gaos_integrated, gaos_product_id FROM product_settings WHERE product_id = ?`, pid).Scan(&gaosOn, &gaosPID)
	if gaosOn && gaosPID.Valid && r.gaos.Configured() {
		job.GaosProductID = gaosPID.String
	}
	if o := r.dashboardOrigin; o != "" && o != "*" {
		o = strings.TrimRight(o, "/")
		job.JobURL = o + "/jobs/" + id
		job.PageURL = fmt.Sprintf("%s/learning?product=%d", o, pid)
	}
	return job, repo, nil
}

// runPolicyPropose is the policy_propose job body. resume is nil for a new job.
func (r *Router) runPolicyPropose(id string, payload map[string]interface{}, resume *learnloop.Resume) {
	ctx, cancel := context.WithTimeout(context.Background(), learnloop.MaxDuration)
	defer cancel()
	fail := func(err error, step string) {
		msg := r.caseAuto.redact.Redact(err.Error())
		if !learnloop.IsStepErr(err) {
			r.addJobEvent(id, "error", step, msg)
		}
		r.finishJob(id, jobStatusFailed, caseauto.Clip(msg, 1000), nil)
		log.Printf("job %s (%s) failed: %s", id, learnloop.JobType, caseauto.Clip(msg, 200))
		if d := r.getDB(); d != nil {
			_, _ = learnloop.Reconcile(context.Background(), d)
		}
	}
	d := r.getDB()
	llm := r.learnLLM()
	if d == nil || llm == nil {
		fail(errors.New("the learning loop is not available (database or AI not configured)"), caseauto.StepPrepare)
		return
	}
	r.markJobRunning(id)
	job, repo, err := r.proposalJobFor(ctx, d, id, payload)
	if err != nil {
		fail(err, caseauto.StepPrepare)
		return
	}
	job.Resume = resume
	var inbox caseauto.Inbox
	if r.gaos.Configured() {
		inbox = r.gaos
	}
	p := &learnloop.Proposer{LLM: llm, Approvals: &caseauto.ApprovalStore{DB: d}, Events: &caseauto.EventStore{DB: d, Redact: r.caseAuto.redact},
		State: jobState{r}, Inbox: inbox, Redact: r.caseAuto.redact, Poll: r.learn.approvalPoll}
	if r.repoPool != nil {
		p.Publisher = &learnPublisher{r: r, d: d, repo: repo, pid: job.ProductID}
	}
	out, err := p.Run(ctx, job)
	if err != nil {
		fail(err, caseauto.StepDone)
		return
	}
	result := map[string]interface{}{"decision": out.Decision, "covered_by": out.CoveredBy, "file_path": out.FilePath, "pr_url": out.PRURL,
		"branch": out.Branch, "title": out.Draft.Title}
	r.finishJob(id, jobStatusCompleted, "", result)
	log.Printf("job %s (%s) completed: %s", id, learnloop.JobType, out.Decision)
	_, _ = learnloop.Reconcile(context.Background(), d)
}

// ResumeProposals continues policy_propose jobs left queued/running by a dead process
// (FailOrphanedJobs leaves this type alone): an open approval is waited on again with the stored
// draft, an approved one is published, a job with a PR is completed, everything else is failed
// (and its open approvals expired, so no door offers a dead answer).
func (r *Router) ResumeProposals(ctx context.Context) {
	d := r.getDB()
	if d == nil {
		return
	}
	rows, err := d.Query(ctx, `SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms
		FROM job_runs WHERE type = ? AND status IN ('queued', 'running') ORDER BY created_at`, learnloop.JobType)
	if err != nil {
		log.Printf("resume proposals: %v", err)
		return
	}
	var jobs []jobRecord
	for rows.Next() {
		if j, err := scanJobRow(rows); err == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	usable := r.learnLLM() != nil
	var resumed, failed int
	for _, j := range jobs {
		rec := j
		r.jobs.mu.Lock()
		if _, held := r.jobs.jobs[rec.ID]; held {
			r.jobs.mu.Unlock()
			continue
		}
		r.jobs.jobs[rec.ID] = &rec
		r.jobs.mu.Unlock()
		res, why := r.resumeDecision(ctx, d, &rec)
		switch {
		case why == "" && res == nil: // completed: the PR exists
			url := str(rec.Payload, "pr_url")
			r.addJobEvent(rec.ID, "info", caseauto.StepDone, "backend restarted after the pull request was opened: "+url)
			r.finishJob(rec.ID, jobStatusCompleted, "", map[string]interface{}{"decision": "new", "pr_url": url, "file_path": str(rec.Payload, "spec_path"), "branch": str(rec.Payload, "branch")})
			resumed++
		case why != "" || !usable:
			if why == "" {
				why = "the learning loop is not configured on this server"
			}
			msg := orphanBaseMsg + "; " + why
			r.addJobEvent(rec.ID, "error", caseauto.StepApproval, msg)
			r.finishJob(rec.ID, jobStatusFailed, msg, nil)
			_, _ = d.Exec(ctx, `UPDATE case_approvals SET status = 'expired', answered_by = 'system', answered_at = UTC_TIMESTAMP() WHERE job_id = ? AND status = 'open'`, rec.ID)
			failed++
		default:
			log.Printf("resuming proposal %s", rec.ID)
			go r.runPolicyPropose(rec.ID, r.jobPayloadCopy(rec.ID), res)
			resumed++
		}
	}
	if len(jobs) > 0 {
		_, _ = learnloop.Reconcile(ctx, d)
		log.Printf("orphaned proposals: %d resumed, %d failed", resumed, failed)
	}
}

var extRoundRe = regexp.MustCompile(`-pr([0-9]*)$`)

// resumeDecision returns how to continue a job: (nil, "") = complete with its PR; (resume, "")
// = continue; (nil, why) = fail.
func (r *Router) resumeDecision(ctx context.Context, d *db.DB, rec *jobRecord) (*learnloop.Resume, string) {
	if str(rec.Payload, "pr_url") != "" {
		return nil, ""
	}
	es := &caseauto.EventStore{DB: d, Redact: r.caseAuto.redact}
	if p, err := es.Progress(ctx, rec.ID); err != nil || p.PushReached {
		return nil, "it had reached the push step (the branch " + str(rec.Payload, "branch") + " may be pushed: check the automation repo, then start a new proposal)"
	}
	a, err := (&caseauto.ApprovalStore{DB: d}).Latest(ctx, rec.ID)
	if err != nil || a == nil {
		return nil, "it was still drafting; start a new proposal"
	}
	var dr learnloop.Draft
	raw, _ := json.Marshal(rec.Payload["draft"])
	if json.Unmarshal(raw, &dr) != nil || dr.Validate() != nil {
		return nil, "its draft could not be read; start a new proposal"
	}
	round := 0
	if m := extRoundRe.FindStringSubmatch(a.ExternalID); m != nil && m[1] != "" {
		n, _ := strconv.Atoi(m[1])
		round = n - 1
	}
	switch a.Status {
	case caseauto.ApprovalOpen:
		return &learnloop.Resume{Draft: dr, ExternalID: a.ExternalID, Round: round}, ""
	case caseauto.ApprovalApproved:
		return &learnloop.Resume{Draft: dr, ExternalID: a.ExternalID, Round: round, Approved: true, ApprovedBy: a.AnsweredBy}, ""
	}
	return nil, "it had already passed the approval step (" + a.Status + "); start a new proposal"
}

// ---------- publishing ----------

// learnPublisher is learnloop.Publisher over the repo pool; push and PR go through poolRepo,
// which passes repoWriteAllowed first.
type learnPublisher struct {
	r    *Router
	d    *db.DB
	repo *automationRepo
	pid  int
}

func (p *learnPublisher) Publish(ctx context.Context, req learnloop.PublishRequest, onPushing func(string)) (learnloop.PublishResult, error) {
	var out learnloop.PublishResult
	pool := p.r.repoPool
	if err := pool.FetchOrigin(ctx, p.repo.RootDir); err != nil {
		return out, err
	}
	wt, err := pool.AddWorktree(ctx, p.repo.RootDir, req.Branch, p.repo.DefaultBranch)
	if err != nil {
		return out, err
	}
	ws := caseauto.Workspace{Dir: wt, Branch: req.Branch, Base: p.repo.DefaultBranch}
	pr := &poolRepo{r: p.r, d: p.d, repo: p.repo}

	p.r.learn.publishMu.Lock()
	taken, err := p.takenPaths(ctx, req.JobID)
	if err != nil {
		p.r.learn.publishMu.Unlock()
		return out, err
	}
	slug := learnloop.FreeSlug(req.Slug, func(path string) bool {
		if taken[path] {
			return true
		}
		_, err := os.Lstat(filepath.Join(wt, filepath.FromSlash(path)))
		return err == nil
	})
	if slug == "" {
		p.r.learn.publishMu.Unlock()
		return out, fmt.Errorf("no free file name for slug %q (tried -2 to -9)", req.Slug)
	}
	path := learnloop.LearnedPath(slug)
	if err := pr.WriteLearned(ws, path, []byte(req.Content)); err != nil {
		p.r.learn.publishMu.Unlock()
		return out, err
	}
	onPushing(path) // records spec_path: the next publisher sees the name as taken
	p.r.learn.publishMu.Unlock()

	if err := pool.Commit(ctx, wt, req.CommitMsg, []string{path}); err != nil {
		return out, err
	}
	if err := pr.Push(ctx, ws); err != nil {
		return out, err
	}
	url, err := pr.OpenPR(ctx, ws, req.PRTitle, req.BodyFor(path))
	if err != nil {
		return out, err
	}
	out.FilePath, out.PRURL = path, url
	if err := repoWriteAllowed(ctx, p.d, p.repo.ID, RepoOpOpenPR); err == nil {
		if err := pool.LabelPR(ctx, wt, url, learnLabel); err != nil {
			out.Warning = "the label " + learnLabel + " could not be added: " + firstLine(err)
		}
	}
	return out, nil
}

// takenPaths are the policy files other proposals of the product are publishing or have in an
// open PR (decision 9).
func (p *learnPublisher) takenPaths(ctx context.Context, self string) (map[string]bool, error) {
	rows, err := p.d.Query(ctx, `SELECT COALESCE(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.spec_path')), ''), COALESCE(JSON_UNQUOTE(JSON_EXTRACT(result, '$.file_path')), '')
		FROM job_runs WHERE type = ? AND id <> ? AND CAST(JSON_EXTRACT(payload, '$.product_id') AS UNSIGNED) = ?
		AND (status IN ('queued', 'running') OR (status = 'completed' AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(result, '$.pr_url')), '') <> ''
		     AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(result, '$.pr_state')), '') NOT IN ('MERGED', 'CLOSED')))`, learnloop.JobType, self, p.pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[a], out[b] = true, true
	}
	delete(out, "")
	return out, rows.Err()
}

// learnedRulesDir resolves <worktree>/.claude/rules inside the worktree (no symlink escapes).
// create makes it when missing.
func learnedRulesDir(ws caseauto.Workspace, create bool) (string, error) {
	root, err := filepath.EvalSymlinks(ws.Dir)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, ".claude", "rules")
	if create {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(real, root+string(os.PathSeparator)) {
		return "", errors.New("path escapes the worktree")
	}
	return real, nil
}

// ReadLearned implements caseauto.RepoOps: the regular learned-*.md files of .claude/rules.
func (p *poolRepo) ReadLearned(ws caseauto.Workspace) ([]caseauto.LearnedFile, error) {
	dir, err := learnedRulesDir(ws, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []caseauto.LearnedFile
	for _, e := range entries {
		rel := learnloop.LearnedDir + e.Name()
		if !e.Type().IsRegular() || !learnloop.IsLearnedPath(rel) || len(out) >= 50 {
			continue
		}
		fi, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > 64<<10 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, caseauto.LearnedFile{Path: rel, Content: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// WriteLearned creates one new learned policy file; any other path is refused.
func (p *poolRepo) WriteLearned(ws caseauto.Workspace, rel string, data []byte) error {
	if !learnloop.IsLearnedPath(rel) {
		return errors.New("refusing to write anything but a .claude/rules/learned-*.md file")
	}
	dir, err := learnedRulesDir(ws, true)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, strings.TrimPrefix(rel, learnloop.LearnedDir)), os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ---------- the driver ----------

// StartLearningLoop runs the loop's tick (harvest, reconcile, PR polls) until ctx ends.
func (r *Router) StartLearningLoop(ctx context.Context, interval time.Duration) {
	if r.getDB() == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		r.learnTick(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.learnTick(ctx)
			}
		}
	}()
}

// learnTick is one pass of the driver; ticks never overlap.
func (r *Router) learnTick(ctx context.Context) {
	r.learn.tickMu.Lock()
	if r.learn.ticking {
		r.learn.tickMu.Unlock()
		return
	}
	r.learn.ticking = true
	r.learn.tickMu.Unlock()
	defer func() {
		r.learn.tickMu.Lock()
		r.learn.ticking = false
		r.learn.tickMu.Unlock()
	}()
	d := r.getDB()
	if d == nil || ctx.Err() != nil {
		return
	}
	every := r.learn.harvestEvery
	if every <= 0 {
		every = learnHarvestEvery
	}
	if time.Since(r.learn.lastHarvest) >= every {
		hctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		st, err := learnloop.Harvest(hctx, d, time.Now().UTC(), r.caseAuto.redact)
		cancel()
		if err != nil {
			log.Printf("learning: harvest: %v", err)
		} else {
			r.learn.lastHarvest = time.Now()
			if st.Created+st.Joined > 0 {
				log.Printf("learning: harvest: %d new finding(s), %d joined", st.Created, st.Joined)
			}
		}
	}
	if _, err := learnloop.Reconcile(ctx, d); err != nil {
		log.Printf("learning: reconcile: %v", err)
	}
	if r.repoPool == nil {
		return
	}
	props, err := loadProposals(ctx, d, 0, "")
	if err != nil {
		log.Printf("learning: PR poll: %v", err)
		return
	}
	n := 0
	for _, p := range props {
		if p.State != learnloop.StatePROpen || n >= learnPRPerTick || ctx.Err() != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, p.CheckedAt); err == nil && time.Since(t) < learnPRCheckEvery {
			continue
		}
		n++
		if err := r.checkProposalPR(ctx, d, p); err != nil {
			log.Printf("learning: PR poll %s: %s", p.ID, firstLine(err))
		}
	}
}
