package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/epicauto"
)

// EPIC_AUTOMATION.md: "Automate epic". A batch is a snapshot list of an epic's eligible cases;
// the driver enqueues them through enqueueCaseAction (the same path as the per-case button) two
// at a time. A case's state is derived from the case_create job that carries the batch id.

var epicCodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// epicDriverBusy keeps ticks from overlapping.
type epicDriverState struct{ busy atomic.Bool }

func writeCodedError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// ---------- eligibility ----------

// jobExclusions returns the case codes that a batch must not start: an active job, a PR that
// awaits merge, a human "no", a job that failed after a push/PR step, or two technical failures.
func jobExclusions(ctx context.Context, d *db.DB, pid int) (map[string]bool, error) {
	rows, err := d.Query(ctx, `
		SELECT COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.payload, '$.case_code')), ''), j.type, j.status, COALESCE(j.error, ''),
		       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.result, '$.pr_url')), ''),
		       EXISTS(SELECT 1 FROM job_events e WHERE e.job_id COLLATE utf8mb4_general_ci = j.id COLLATE utf8mb4_general_ci AND e.step IN ('push', 'pr'))
		FROM job_runs j
		WHERE j.type IN ('case_run', 'case_create') AND CAST(JSON_EXTRACT(j.payload, '$.product_id') AS UNSIGNED) = ?`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	tech := map[string]int{}
	for rows.Next() {
		var code, typ, status, errText, pr string
		var pushed bool
		if err := rows.Scan(&code, &typ, &status, &errText, &pr, &pushed); err != nil {
			return nil, err
		}
		if code == "" {
			continue
		}
		switch {
		case status == "queued" || status == "running":
			out[code] = true
		case typ != "case_create":
		case status == "completed" && pr != "":
			out[code] = true
		case status == "failed" && pushed:
			out[code] = true
		case status == "failed" && epicauto.HumanOutcome(errText):
			out[code] = true
		case status == "failed":
			tech[code]++
		}
	}
	for code, n := range tech {
		if n >= 2 {
			out[code] = true
		}
	}
	return out, rows.Err()
}

// eligibleCaseWhere is the row predicate of an eligible case over kb_test_cases t (EPIC_AUTOMATION.md
// decision 2); the job exclusions (jobExclusions) are applied on top. Shared with the navigation
// map's "next to automate" list (NAVIGATION_MAP.md section 6).
const eligibleCaseWhere = `t.removed_at IS NULL AND t.status = 'planned'
		  AND t.reason_category = '' AND JSON_LENGTH(t.spec_paths) = 0
		  AND NOT EXISTS (SELECT 1 FROM gaos_decisions g WHERE g.product_id = t.product_id AND g.case_code = t.code AND g.decision <> 'planned')`

// eligibleCodes lists the epic's eligible case codes in work order: priority P0..P3 first,
// then no priority, then by code.
func eligibleCodes(ctx context.Context, d *db.DB, pid int, epic string) ([]string, error) {
	excluded, err := jobExclusions(ctx, d, pid)
	if err != nil {
		return nil, err
	}
	rows, err := d.Query(ctx, `
		SELECT t.code FROM kb_test_cases t
		WHERE t.product_id = ? AND t.epic_code = ? AND `+eligibleCaseWhere+`
		ORDER BY FIELD(t.priority, 'P0', 'P1', 'P2', 'P3') = 0, FIELD(t.priority, 'P0', 'P1', 'P2', 'P3'), t.code COLLATE utf8mb4_bin`, pid, epic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		if !excluded[code] {
			out = append(out, code)
		}
	}
	return out, rows.Err()
}

func activeEpic(ctx context.Context, d *db.DB, pid int, epic string) (bool, error) {
	var one int
	err := d.QueryRow(ctx, `SELECT 1 FROM kb_epics WHERE product_id = ? AND code = ? AND removed_at IS NULL`, pid, epic).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ---------- batches ----------

type epicBatch struct {
	ID        string
	ProductID int
	Epic      string
	Status    string
	Cases     []string
	StartedBy string
	Note      string
	CreatedAt time.Time
}

func scanBatch(sc interface{ Scan(...any) error }) (*epicBatch, error) {
	var b epicBatch
	var cases []byte
	if err := sc.Scan(&b.ID, &b.ProductID, &b.Epic, &b.Status, &cases, &b.StartedBy, &b.Note, &b.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(cases, &b.Cases)
	return &b, nil
}

const batchCols = `id, product_id, epic_code, status, cases, started_by, note, created_at`

func loadBatch(ctx context.Context, d *db.DB, id string) (*epicBatch, error) {
	b, err := scanBatch(d.QueryRow(ctx, `SELECT `+batchCols+` FROM epic_batches WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

func newestBatch(ctx context.Context, d *db.DB, pid int, epic string) (*epicBatch, error) {
	b, err := scanBatch(d.QueryRow(ctx, `SELECT `+batchCols+` FROM epic_batches WHERE product_id = ? AND epic_code = ? ORDER BY created_at DESC, id DESC LIMIT 1`, pid, epic))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

// batchJobs returns the newest case_create job of each case of the batch.
func batchJobs(ctx context.Context, d *db.DB, batchID string) (map[string]*epicauto.JobFacts, error) {
	rows, err := d.Query(ctx, `
		SELECT j.id, COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.payload, '$.case_code')), ''), j.status, COALESCE(j.error, ''),
		       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.result, '$.pr_url')), ''), j.created_at, j.completed_at,
		       EXISTS(SELECT 1 FROM job_events e WHERE e.job_id COLLATE utf8mb4_general_ci = j.id COLLATE utf8mb4_general_ci AND e.step IN ('push', 'pr')),
		       EXISTS(SELECT 1 FROM case_approvals a WHERE a.job_id COLLATE utf8mb4_general_ci = j.id COLLATE utf8mb4_general_ci AND a.status = 'open')
		FROM job_runs j
		WHERE j.type = 'case_create' AND JSON_UNQUOTE(JSON_EXTRACT(j.payload, '$.batch_id')) = ?
		ORDER BY j.created_at, j.id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*epicauto.JobFacts{}
	for rows.Next() {
		var f epicauto.JobFacts
		var code string
		var created time.Time
		var done sql.NullTime
		if err := rows.Scan(&f.ID, &code, &f.Status, &f.Err, &f.PRURL, &created, &done, &f.PushedOrPR, &f.AwaitingApproval); err != nil {
			return nil, err
		}
		f.DoneAt = created
		if done.Valid {
			f.DoneAt = done.Time
		}
		out[code] = &f // later rows are newer, so the newest wins
	}
	return out, rows.Err()
}

// batchCases builds the planner's input for a batch from fresh facts.
func batchCases(ctx context.Context, d *db.DB, b *epicBatch) ([]epicauto.Case, error) {
	jobs, err := batchJobs(ctx, d, b.ID)
	if err != nil {
		return nil, err
	}
	elig, err := eligibleCodes(ctx, d, b.ProductID, b.Epic)
	if err != nil {
		return nil, err
	}
	ok := map[string]bool{}
	for _, c := range elig {
		ok[c] = true
	}
	out := make([]epicauto.Case, len(b.Cases))
	for i, code := range b.Cases {
		out[i] = epicauto.Case{Code: code, Job: jobs[code], Eligible: ok[code]}
	}
	return out, nil
}

// setBatchStatus changes a RUNNING batch's status; it reports whether this call changed it, so a
// tick that raced a cancel never overwrites 'cancelled'.
func setBatchStatus(ctx context.Context, d *db.DB, id, status, note string) (bool, error) {
	res, err := d.Exec(ctx, `UPDATE epic_batches SET status = ?, note = ? WHERE id = ? AND status = 'running'`, status, clip(note, 300), id)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() == 1, nil
}

// isDupKey reports a MySQL duplicate-key error (1062).
func isDupKey(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type batchCaseView struct {
	Code   string `json:"code"`
	Seq    int    `json:"seq"`
	State  string `json:"state"`
	JobID  string `json:"job_id"`
	Detail string `json:"detail"`
}

func (r *Router) batchView(ctx context.Context, d *db.DB, b *epicBatch) (map[string]any, error) {
	cases, err := batchCases(ctx, d, b)
	if err != nil {
		return nil, err
	}
	dec := epicauto.Plan(cases)
	views := make([]batchCaseView, len(cases))
	for i, c := range cases {
		v := batchCaseView{Code: c.Code, Seq: i + 1, State: string(dec.States[i])}
		if c.Job != nil {
			v.JobID = c.Job.ID
			if dec.States[i] == epicauto.Failed || dec.States[i] == epicauto.Declined || dec.States[i] == epicauto.NeedsPerson {
				v.Detail = clip(r.caseAuto.redact.Redact(c.Job.Err), 300)
			}
		}
		views[i] = v
	}
	return map[string]any{
		"id": b.ID, "status": b.Status, "note": b.Note, "started_by": b.StartedBy,
		"created_at": b.CreatedAt.UTC().Format(time.RFC3339), "max_open": epicauto.MaxOpen, "cases": views,
	}, nil
}

// ---------- routes ----------

// handleEpicAutomation serves GET .../epics/{epic}/automation and POST .../epics/{epic}/automate.
func (r *Router) handleEpicAutomation(w http.ResponseWriter, req *http.Request, pid int, epic, action string, canWrite bool) {
	if !epicCodeRe.MatchString(epic) {
		writeCodedError(w, http.StatusBadRequest, "bad_epic", "not a valid epic code")
		return
	}
	d := r.getDB()
	if d == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unconfigured", "database unavailable")
		return
	}
	ctx := req.Context()
	switch {
	case action == "automation" && req.Method == http.MethodGet:
		r.epicStatus(ctx, w, d, pid, epic)
	case action == "automate" && req.Method == http.MethodPost:
		if !canWrite {
			writeCodedError(w, http.StatusForbidden, "forbidden", "your role cannot run or create automation")
			return
		}
		r.epicStart(ctx, w, req, d, pid, epic)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (r *Router) epicStatus(ctx context.Context, w http.ResponseWriter, d *db.DB, pid int, epic string) {
	ok, err := activeEpic(ctx, d, pid, epic)
	if err != nil {
		log.Printf("epic automation: epic: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if !ok {
		writeCodedError(w, http.StatusBadRequest, "unknown_epic", "no such epic in this product")
		return
	}
	repo, err := loadAutomationRepo(ctx, d, pid)
	if err != nil {
		log.Printf("epic automation: repo: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := map[string]any{"repo": repo != nil, "eligible": 0, "sample": []string{}, "batch": nil}
	if repo != nil {
		codes, err := eligibleCodes(ctx, d, pid, epic)
		if err != nil {
			log.Printf("epic automation: eligible: %v", err)
			writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		out["eligible"] = len(codes)
		if len(codes) > 10 {
			codes = codes[:10]
		}
		if codes == nil {
			codes = []string{}
		}
		out["sample"] = codes
	}
	b, err := newestBatch(ctx, d, pid, epic)
	if err == nil && b != nil {
		var v map[string]any
		if v, err = r.batchView(ctx, d, b); err == nil {
			out["batch"] = v
		}
	}
	if err != nil {
		log.Printf("epic automation: batch: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (r *Router) epicStart(ctx context.Context, w http.ResponseWriter, req *http.Request, d *db.DB, pid int, epic string) {
	if r.caseAuto.draining.Load() {
		w.Header().Set("Retry-After", "30")
		writeCodedError(w, http.StatusServiceUnavailable, "draining", "the server is restarting; try again in a moment")
		return
	}
	max := epicauto.DefaultMaxCases
	body, err := io.ReadAll(io.LimitReader(req.Body, 4096))
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, "bad_range", "unreadable body")
		return
	}
	if len(strings.TrimSpace(string(body))) > 0 {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(body, &m); err != nil {
			writeCodedError(w, http.StatusBadRequest, "bad_range", "the body must be a JSON object")
			return
		}
		for k, v := range m {
			if k != "max_cases" {
				writeCodedError(w, http.StatusBadRequest, "bad_range", "unknown field "+k)
				return
			}
			var n int
			if err := json.Unmarshal(v, &n); err != nil || string(v) == "null" {
				writeCodedError(w, http.StatusBadRequest, "bad_range", "max_cases must be an integer")
				return
			}
			max = n
		}
	}
	if max < 1 || max > epicauto.MaxCases {
		writeCodedError(w, http.StatusBadRequest, "bad_range", fmt.Sprintf("max_cases must be between 1 and %d", epicauto.MaxCases))
		return
	}
	ok, err := activeEpic(ctx, d, pid, epic)
	if err != nil {
		log.Printf("epic automation: epic: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if !ok {
		writeCodedError(w, http.StatusBadRequest, "unknown_epic", "no such epic in this product")
		return
	}
	repo, err := loadAutomationRepo(ctx, d, pid)
	if err != nil {
		log.Printf("epic automation: repo: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if repo == nil {
		writeCodedError(w, http.StatusConflict, "no_repo", "the product has no automation repo")
		return
	}
	codes, err := eligibleCodes(ctx, d, pid, epic)
	if err != nil {
		log.Printf("epic automation: eligible: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if len(codes) == 0 {
		writeCodedError(w, http.StatusConflict, "nothing_eligible", "no planned case in this epic is waiting for automation")
		return
	}
	if len(codes) > max {
		codes = codes[:max]
	}
	if r.executor == nil || r.repoPool == nil || r.caseAuto.hostPool == "" || r.aiService == nil || !r.aiService.Configured() {
		writeCodedError(w, http.StatusServiceUnavailable, "unconfigured", "case automation is not configured")
		return
	}
	id, err := createBatch(ctx, d, pid, epic, codes, userID(req))
	if err != nil {
		if isDupKey(err) {
			msg := "a batch is already running for this product"
			var running string
			if e := d.QueryRow(ctx, `SELECT epic_code FROM epic_batches WHERE product_id = ? AND status = 'running' LIMIT 1`, pid).Scan(&running); e == nil {
				msg = "a batch is already running for this product (epic " + running + ")"
			}
			writeCodedError(w, http.StatusConflict, "batch_active", msg)
			return
		}
		log.Printf("epic automation: insert: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	log.Printf("epic batch %s %s: started by %s with %d case(s)", id, epic, userID(req), len(codes))
	writeJSON(w, http.StatusAccepted, map[string]any{"batch_id": id, "planned": codes})
}

// createBatch stores a running batch; the unique key refuses a second running batch of the product.
func createBatch(ctx context.Context, d *db.DB, pid int, epic string, codes []string, startedBy string) (string, error) {
	var rnd [3]byte
	_, _ = rand.Read(rnd[:])
	id := fmt.Sprintf("eb-%d-%s", time.Now().Unix(), hex.EncodeToString(rnd[:]))
	raw, _ := json.Marshal(codes)
	_, err := d.Exec(ctx, `INSERT INTO epic_batches (id, product_id, epic_code, status, cases, started_by) VALUES (?, ?, ?, 'running', ?, ?)`,
		id, pid, epic, string(raw), clip(startedBy, 64))
	return id, err
}

// handleEpicBatches serves POST /api/epic-batches/{id}/cancel.
func (r *Router) handleEpicBatches(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, "/api/epic-batches/")
	id, action, ok := strings.Cut(rest, "/")
	if !ok || id == "" || action != "cancel" || strings.Contains(id, "/") {
		writeCodedError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if req.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, canVerify := productRole(req); !canVerify {
		writeCodedError(w, http.StatusForbidden, "forbidden", "your role cannot run or create automation")
		return
	}
	d := r.getDB()
	if d == nil {
		writeCodedError(w, http.StatusServiceUnavailable, "unconfigured", "database unavailable")
		return
	}
	ctx := req.Context()
	b, err := loadBatch(ctx, d, id)
	if err != nil {
		log.Printf("epic automation: load batch: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if b == nil {
		writeCodedError(w, http.StatusNotFound, "not_found", "no such batch")
		return
	}
	// The same mutex enqueueCaseAction holds while it checks the batch: after this returns no job
	// of the batch can start.
	r.caseAuto.mu.Lock()
	changed, err := setBatchStatus(ctx, d, id, epicauto.BatchCancelled, "cancelled by "+userID(req))
	r.caseAuto.mu.Unlock()
	if err != nil {
		log.Printf("epic automation: cancel: %v", err)
		writeCodedError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if !changed {
		writeCodedError(w, http.StatusConflict, "bad_transition", "only a running batch can be cancelled")
		return
	}
	log.Printf("epic batch %s %s: cancelled by %s", id, b.Epic, userID(req))
	b, _ = loadBatch(ctx, d, id)
	v, err := r.batchView(ctx, d, b)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": epicauto.BatchCancelled})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ---------- driver ----------

// StartEpicDriver ticks every interval until ctx ends. Call it after ResumeOrphanedJobs.
func (r *Router) StartEpicDriver(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.epicTick(ctx)
			}
		}
	}()
}

// epicTick looks at every running batch once. Ticks never overlap.
func (r *Router) epicTick(ctx context.Context) {
	if !r.epicDriver.busy.CompareAndSwap(false, true) {
		return
	}
	defer r.epicDriver.busy.Store(false)
	d := r.getDB()
	if d == nil || ctx.Err() != nil {
		return
	}
	rows, err := d.Query(ctx, `SELECT `+batchCols+` FROM epic_batches WHERE status = 'running' ORDER BY created_at, id`)
	if err != nil {
		log.Printf("epic driver: list: %v", err)
		return
	}
	var batches []*epicBatch
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			log.Printf("epic driver: scan: %v", err)
			continue
		}
		batches = append(batches, b)
	}
	rows.Close()
	for _, b := range batches {
		if ctx.Err() != nil || r.caseAuto.draining.Load() {
			return
		}
		r.epicStep(ctx, d, b)
	}
}

func (r *Router) epicStep(ctx context.Context, d *db.DB, b *epicBatch) {
	cases, err := batchCases(ctx, d, b)
	if err != nil {
		log.Printf("epic driver: batch %s: %v", b.ID, err)
		return
	}
	dec := epicauto.Plan(cases)
	if dec.Status != "" {
		if changed, err := setBatchStatus(ctx, d, b.ID, dec.Status, dec.Note); err != nil {
			log.Printf("epic driver: batch %s: %v", b.ID, err)
		} else if changed {
			log.Printf("epic batch %s %s: %s %s", b.ID, b.Epic, dec.Status, dec.Note)
		}
		return
	}
	seq := map[string]int{}
	for i, c := range b.Cases {
		seq[c] = i + 1
	}
	for _, code := range dec.Enqueue {
		res := r.enqueueCaseAction(ctx, d, caseEnqueueIn{
			PID: b.ProductID, Code: code, Action: "create-automation",
			TriggeredBy: b.StartedBy, BatchID: b.ID, BatchSeq: seq[code],
		})
		if res.Err == nil {
			log.Printf("epic batch %s %s: enqueued %s job %s", b.ID, b.Epic, code, res.JobID)
			continue
		}
		switch res.Err.Code {
		case "draining", "unconfigured", "batch_not_running":
			return // nothing more this tick; not a failure of the case
		case "case_active", "has_spec", "not_found":
			// shows as active or skipped on the next look
		default:
			log.Printf("epic batch %s %s: %s not enqueued: %s", b.ID, b.Epic, code, res.Err.Msg)
		}
	}
}
