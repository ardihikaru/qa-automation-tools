package api

// CASE_AUTOMATION.md: run or create the automation of one test case.
//
//	POST /api/products/{id}/knowledge/test-cases/{code}/run               -> 202 {job_id}
//	POST /api/products/{id}/knowledge/test-cases/{code}/create-automation -> 202 {job_id}
//	GET  /api/products/{id}/knowledge/test-cases/{code}/jobs              -> last 10 jobs of the case
//
// case_run goes to playwright-executor and stores its verdict like an uploaded report
// (caseresults, source "case-run") and then triggers the GAOS push. case_create is the
// internal/services/caseauto state machine; the executor is only used to run the spec.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/caseresults"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/services/runhistory"
)

// CaseAutomationConfig wires the feature. PoolHostDir is the repo pool as playwright-executor
// sees it; Secrets are literal values (tokens) that must never reach an event, a log or a prompt.
type CaseAutomationConfig struct {
	PoolHostDir string
	Secrets     []string
}

// caseAuto is the router's state for the feature (zero value = unconfigured).
type caseAuto struct {
	hostPool string
	redact   *caseauto.Redactor
	mu       sync.Mutex  // serialises "is a job active?" + enqueue
	draining atomic.Bool // set at SIGTERM: new case jobs are refused (BeginDrain)
}

// SetCaseAutomation configures run / create-automation. Without PoolHostDir both answer 503.
func (r *Router) SetCaseAutomation(c CaseAutomationConfig) {
	r.caseAuto.hostPool = strings.TrimRight(strings.TrimSpace(c.PoolHostDir), "/")
	r.caseAuto.redact = caseauto.NewRedactor(c.Secrets...)
}

var specPathPrefixes = []string{"tests/web/", "tests/api/rest/"}

// runnableSpec reports whether p is a spec the executor will accept (mq and anything else is refused).
func runnableSpec(p string) bool {
	if !strings.HasSuffix(p, ".spec.ts") || path.Clean(p) != p || strings.Contains(p, "..") || len(p) > 300 {
		return false
	}
	for _, pre := range specPathPrefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

type caseRow struct {
	Scenario  string
	StoryCode string
	SpecPaths []string
}

func (r *Router) loadCase(ctx context.Context, d *db.DB, pid int, code string) (*caseRow, error) {
	var c caseRow
	var scen sql.NullString
	var spec []byte
	var removed bool
	err := d.QueryRow(ctx, `SELECT scenario, COALESCE(story_code, ''), spec_paths, removed_at IS NOT NULL FROM kb_test_cases WHERE product_id = ? AND code = ?`,
		pid, code).Scan(&scen, &c.StoryCode, &spec, &removed)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && removed) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Scenario = scen.String
	_ = json.Unmarshal(spec, &c.SpecPaths)
	return &c, nil
}

type automationRepo struct {
	ID            int
	RootDir       string
	DefaultBranch string
}

func loadAutomationRepo(ctx context.Context, d *db.DB, pid int) (*automationRepo, error) {
	var a automationRepo
	var legacy string
	err := d.QueryRow(ctx, `SELECT r.id, r.root_dir, p.branch FROM product_repos r JOIN products p ON p.id = r.product_id WHERE r.product_id = ? AND r.is_automation = 1`, pid).Scan(&a.ID, &a.RootDir, &legacy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.DefaultBranch = legacy
	if err := d.QueryRow(ctx, `SELECT branch FROM product_repo_branches WHERE repo_id = ? AND is_default = 1`, a.ID).Scan(&a.DefaultBranch); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if a.DefaultBranch == "" {
		a.DefaultBranch = "main"
	}
	return &a, nil
}

// activeCaseJob returns the id of a queued or running case job for the case, or "".
func activeCaseJob(ctx context.Context, d *db.DB, pid int, code string) (string, error) {
	var id string
	err := d.QueryRow(ctx, `SELECT id FROM job_runs WHERE type IN ('case_run','case_create') AND status IN ('queued','running')
		AND JSON_UNQUOTE(JSON_EXTRACT(payload, '$.case_code')) = ? AND CAST(JSON_EXTRACT(payload, '$.product_id') AS UNSIGNED) = ? LIMIT 1`, code, pid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// handleCaseAction serves the three case routes. canVerify is the caller's productRole verdict.
func (r *Router) handleCaseAction(w http.ResponseWriter, req *http.Request, pid int, code, action string, canVerify bool) {
	if !caseauto.CaseCodeRe.MatchString(code) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if action == "jobs" {
		if req.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		r.listCaseJobs(w, req, d, pid, code)
		return
	}
	if req.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !canVerify {
		writeJSONError(w, http.StatusForbidden, "your role cannot run or create automation")
		return
	}
	res := r.enqueueCaseAction(req.Context(), d, caseEnqueueIn{PID: pid, Code: code, Action: action, TriggeredBy: userID(req)})
	if e := res.Err; e != nil {
		if e.RetryAfter {
			w.Header().Set("Retry-After", "30")
		}
		if res.ActiveJobID != "" {
			writeJSON(w, e.Status, map[string]any{"error": e.Msg, "job_id": res.ActiveJobID})
			return
		}
		writeJSONError(w, e.Status, e.Msg)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": res.JobID})
}

// caseEnqueueIn is what enqueueCaseAction needs. BatchID is set only by the epic driver
// (EPIC_AUTOMATION.md); it rides in the job's payload from its first write.
type caseEnqueueIn struct {
	PID         int
	Code        string
	Action      string // "run" | "create-automation"
	TriggeredBy string
	BatchID     string
	BatchSeq    int
}

// caseErr is a refusal, typed so a non-HTTP caller (the epic driver) can act on it.
type caseErr struct {
	Status     int
	Code       string // unconfigured | draining | no_repo | not_found | has_spec | no_spec | case_active | batch_not_running | bad_spec | internal
	Msg        string
	RetryAfter bool
}

type caseEnqueue struct {
	JobID       string
	ActiveJobID string // set with Err.Code == "case_active"
	Err         *caseErr
}

func refuse(status int, code, msg string) caseEnqueue {
	return caseEnqueue{Err: &caseErr{Status: status, Code: code, Msg: msg}}
}

// enqueueCaseAction is the one path that starts a case_run or case_create job: the per-case
// buttons and the epic driver both come through here, so their guards cannot diverge.
func (r *Router) enqueueCaseAction(ctx context.Context, d *db.DB, in caseEnqueueIn) caseEnqueue {
	pid, code, action := in.PID, in.Code, in.Action
	if r.caseAuto.draining.Load() {
		out := refuse(http.StatusServiceUnavailable, "draining", "the server is restarting; try again in a moment")
		out.Err.RetryAfter = true
		return out
	}
	if r.executor == nil || r.repoPool == nil || r.caseAuto.hostPool == "" {
		return refuse(http.StatusServiceUnavailable, "unconfigured", "case automation is not configured (playwright-executor, REPO_POOL_DIR and REPO_POOL_HOST_DIR are needed)")
	}
	if action == "create-automation" && (r.aiService == nil || !r.aiService.Configured()) {
		return refuse(http.StatusServiceUnavailable, "unconfigured", "AI is not configured")
	}
	c, err := r.loadCase(ctx, d, pid, code)
	if err != nil {
		log.Printf("case automation: load case: %v", err)
		return refuse(http.StatusInternalServerError, "internal", "internal error")
	}
	if c == nil {
		return refuse(http.StatusNotFound, "not_found", "not found")
	}
	repo, err := loadAutomationRepo(ctx, d, pid)
	if err != nil {
		log.Printf("case automation: load repo: %v", err)
		return refuse(http.StatusInternalServerError, "internal", "internal error")
	}
	hasSpec := len(c.SpecPaths) > 0
	switch {
	case action == "run" && !hasSpec:
		return refuse(http.StatusConflict, "no_spec", "the case has no automation spec to run")
	case action == "create-automation" && hasSpec:
		return refuse(http.StatusConflict, "has_spec", "the case already has an automation spec")
	case repo == nil:
		return refuse(http.StatusConflict, "no_repo", "the product has no automation repo")
	}
	payload := map[string]interface{}{"product_id": pid, "case_code": code, "triggered_by": in.TriggeredBy}
	jobType := "case_run"
	if action == "run" {
		for _, p := range c.SpecPaths {
			if !runnableSpec(p) {
				return refuse(http.StatusConflict, "bad_spec", "the case's spec cannot be run here (only tests/web and tests/api/rest specs are supported)")
			}
		}
		if len(c.SpecPaths) > 5 {
			c.SpecPaths = c.SpecPaths[:5]
		}
		abs, err := r.repoPool.ResolveDir(repo.RootDir)
		if err != nil {
			return refuse(http.StatusConflict, "no_repo", "the automation repo is not available in the pool")
		}
		hostDir, err := r.repoPool.HostPath(r.caseAuto.hostPool, abs)
		if err != nil {
			return refuse(http.StatusConflict, "no_repo", "the automation repo is not available in the pool")
		}
		payload["spec_paths"] = c.SpecPaths
		payload["repo_dir"] = hostDir
		payload["env_file"] = executorEnvFile(pid)
	} else {
		jobType = "case_create"
		payload["spec_paths"] = []string{}
		payload["branch"] = repopool.AutoBranch(code)
		payload["pr_url"] = ""
	}
	if in.BatchID != "" {
		payload["batch_id"] = in.BatchID
		payload["batch_seq"] = in.BatchSeq
	}
	r.caseAuto.mu.Lock()
	defer r.caseAuto.mu.Unlock()
	if id, err := activeCaseJob(ctx, d, pid, code); err != nil {
		log.Printf("case automation: active check: %v", err)
		return refuse(http.StatusInternalServerError, "internal", "internal error")
	} else if id != "" {
		out := refuse(http.StatusConflict, "case_active", "a job for this case is already active")
		out.ActiveJobID = id
		return out
	}
	if in.BatchID != "" {
		// A cancel holds this mutex while it writes, so after it no job may start.
		var st string
		if err := d.QueryRow(ctx, `SELECT status FROM epic_batches WHERE id = ?`, in.BatchID).Scan(&st); err != nil || st != "running" {
			return refuse(http.StatusConflict, "batch_not_running", "the batch is not running")
		}
	}
	job, err := r.enqueueJobTx(jobType, payload)
	if err != nil {
		log.Printf("case automation: enqueue: %v", err)
		return refuse(http.StatusInternalServerError, "internal", "internal error")
	}
	return caseEnqueue{JobID: job.ID}
}

func executorEnvFile(pid int) string { return fmt.Sprintf("automation-%d.env", pid) }

func (r *Router) listCaseJobs(w http.ResponseWriter, req *http.Request, d *db.DB, pid int, code string) {
	rows, err := d.Query(req.Context(), `SELECT id, type, status, COALESCE(error, ''), created_at, started_at, completed_at,
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.pr_url')), ''), COALESCE(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.branch')), ''),
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(result, '$.verdict.result')), '')
		FROM job_runs WHERE type IN ('case_run','case_create')
		AND JSON_UNQUOTE(JSON_EXTRACT(payload, '$.case_code')) = ? AND CAST(JSON_EXTRACT(payload, '$.product_id') AS UNSIGNED) = ?
		ORDER BY created_at DESC LIMIT 10`, code, pid)
	if err != nil {
		log.Printf("case automation: list jobs: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	type item struct {
		ID          string     `json:"id"`
		Type        string     `json:"type"`
		Status      string     `json:"status"`
		Error       string     `json:"error,omitempty"`
		CreatedAt   time.Time  `json:"created_at"`
		StartedAt   *time.Time `json:"started_at,omitempty"`
		CompletedAt *time.Time `json:"completed_at,omitempty"`
		PRURL       string     `json:"pr_url,omitempty"`
		Branch      string     `json:"branch,omitempty"`
		Outcome     string     `json:"outcome,omitempty"` // case_run verdict: pass|fail|skipped|none
	}
	out := []item{}
	for rows.Next() {
		var it item
		var st, ct sql.NullTime
		if err := rows.Scan(&it.ID, &it.Type, &it.Status, &it.Error, &it.CreatedAt, &st, &ct, &it.PRURL, &it.Branch, &it.Outcome); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if st.Valid {
			it.StartedAt = &st.Time
		}
		if ct.Valid {
			it.CompletedAt = &ct.Time
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
}

// ---- case_run ----

// caseRunParamKeys are the payload keys forwarded to the executor for a case_run.
var caseRunParamKeys = []string{"repo_dir", "case_code", "spec_paths", "env_file", "repair"}

func caseRunParams(payload map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, k := range caseRunParamKeys {
		if v, ok := payload[k]; ok {
			out[k] = v
		}
	}
	return out
}

func (r *Router) eventStore() *caseauto.EventStore {
	d := r.getDB()
	if d == nil {
		return nil
	}
	return &caseauto.EventStore{DB: d, Redact: r.caseAuto.redact}
}

// addJobEvent writes one event; a missing database or store error is only logged.
func (r *Router) addJobEvent(jobID, level, step, msg string) {
	es := r.eventStore()
	if es == nil {
		return
	}
	if err := es.Add(context.Background(), jobID, level, step, msg); err != nil {
		log.Printf("job %s: event not stored: %v", jobID, err)
	}
}

// forwardExecutorEvents copies the executor's new events of a run into job_events.
func (r *Router) forwardExecutorEvents(jobID string, run *executor.Run) {
	for _, e := range run.Events {
		step := e.Step
		if step == "" {
			step = caseauto.StepRun
		}
		r.addJobEvent(jobID, e.Level, step, e.Message)
	}
}

// finishCaseRun turns a completed case_run result into a stored verdict and the GAOS push.
// A returned error fails the job (no usable report, or no test titled with the code).
func (r *Router) finishCaseRun(ctx context.Context, jobID string, payload, result map[string]interface{}) error {
	code, _ := payload["case_code"].(string)
	pid := intFrom(payload["product_id"])
	spec := ""
	if sp, ok := payload["spec_paths"].([]interface{}); ok && len(sp) > 0 {
		spec, _ = sp[0].(string)
	} else if sp, ok := payload["spec_paths"].([]string); ok && len(sp) > 0 {
		spec = sp[0]
	}
	v := caseauto.VerdictFromResult(result, code, spec, time.Now().UTC())
	raw, _ := json.Marshal(result["report"])
	delete(result, "report") // up to 2 MiB: the verdict and counts are kept, the raw report is not
	result["summary"] = map[string]int{"passed": v.Passed, "failed": v.Failed, "skipped": v.Skipped}
	verdict := map[string]interface{}{"pass": v.Pass, "message": v.Message, "result": v.Result}
	result["verdict"] = verdict
	skipReason := ""
	if v.Result == caseresults.Skipped {
		skipReason = caseauto.Clip(r.caseAuto.redact.Redact(v.SkipReason), 400)
		verdict["message"] = caseauto.Clip(r.caseAuto.redact.Redact(v.Message), 500)
		if skipReason != "" {
			verdict["skip_reason"] = skipReason
			result["skip_reason"] = skipReason
		}
	}
	if v.Report == nil {
		return errors.New(v.Message)
	}
	// RUN_HISTORY.md: every report is logged, skipped cases and a report without this case included.
	r.recordRun(ctx, runhistory.RunIn{ProductID: pid, Source: runhistory.SourceCaseRun, RunRef: jobID, JobID: jobID, CaseCode: code}, v.Report)
	if _, ok := v.Report.Results[code]; !ok {
		r.addJobEvent(jobID, "error", caseauto.StepDone, v.Message)
		return errors.New(v.Message)
	}
	if d := r.getDB(); d != nil && pid > 0 {
		rep, err := caseresults.Parse(bytes.NewReader(raw), time.Now().UTC())
		if err == nil {
			// A skip is absence of evidence: never stored, so it cannot replace a pass/fail (CASE_RESULTS.md, Skipped runs).
			for c, res := range rep.Results {
				if res == caseresults.Skipped {
					delete(rep.Results, c)
				}
			}
			out, err := caseresults.Save(ctx, d, pid, rep, jobID, "case-run")
			if err != nil {
				log.Printf("job %s: saving the case result: %v", jobID, err)
			} else if out.Stored > 0 {
				r.notifyResultsStored(pid) // wired to gaossync.Trigger
			}
		}
	}
	if v.Result == caseresults.Skipped {
		msg := "skipped: " + skipReason
		if skipReason == "" {
			msg = "skipped: the report gives no reason"
		}
		if caseresults.HarnessHint(skipReason) {
			msg += " (hint: the HISv3 e2e harness prerequisites, e.g. the staging DB credential file, are missing on the executor host; no result was stored)"
		} else {
			msg += " (no result was stored)"
		}
		r.addJobEvent(jobID, "warn", caseauto.StepDone, msg)
		return nil
	}
	level := "info"
	if !v.Pass {
		level = "warn"
	}
	r.addJobEvent(jobID, level, caseauto.StepDone, v.Message)
	return nil
}

func intFrom(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// ---- case_create ----

// finishJob ends a job record.
func (r *Router) finishJob(id string, status jobStatus, errMsg string, result map[string]interface{}) {
	now := time.Now().UTC()
	r.updateJob(id, func(j *jobRecord) {
		j.Status = status
		j.Error = errMsg
		if result != nil {
			j.Result = result
		}
		j.CompletedAt = &now
		if j.StartedAt == nil {
			j.StartedAt = &now
		}
		j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
	})
}

func (r *Router) markJobRunning(id string) {
	now := time.Now().UTC()
	r.updateJob(id, func(j *jobRecord) {
		j.Status = jobStatusRunning
		if j.StartedAt == nil {
			j.StartedAt = &now
		}
	})
}

// setJobPayload sets one payload key of a job (the caseauto State).
func (r *Router) setJobPayload(id, key string, value interface{}) {
	r.updateJob(id, func(j *jobRecord) {
		if j.Payload == nil {
			j.Payload = map[string]interface{}{}
		}
		j.Payload[key] = value
	})
}

type jobState struct{ r *Router }

func (s jobState) SetPayload(_ context.Context, jobID, key string, value interface{}) {
	s.r.setJobPayload(jobID, key, value)
}

// caseJobFor builds the caseauto job from the database.
func (r *Router) caseJobFor(ctx context.Context, d *db.DB, jobID string, payload map[string]interface{}) (caseauto.Job, *automationRepo, error) {
	pid := intFrom(payload["product_id"])
	code, _ := payload["case_code"].(string)
	var job caseauto.Job
	c, err := r.loadCase(ctx, d, pid, code)
	if err != nil {
		return job, nil, err
	}
	if c == nil {
		return job, nil, errors.New("the case no longer exists")
	}
	repo, err := loadAutomationRepo(ctx, d, pid)
	if err != nil {
		return job, nil, err
	}
	if repo == nil {
		return job, nil, errors.New("the product has no automation repo")
	}
	var name, staging string
	if err := d.QueryRow(ctx, `SELECT name, staging_url FROM products WHERE id = ?`, pid).Scan(&name, &staging); err != nil {
		return job, nil, err
	}
	hosts := []string{}
	if u := parseHost(staging); u != "" {
		hosts = append(hosts, u)
	}
	var storyText sql.NullString
	if c.StoryCode != "" {
		_ = d.QueryRow(ctx, `SELECT text FROM kb_stories WHERE product_id = ? AND code = ?`, pid, c.StoryCode).Scan(&storyText)
	}
	var features []string
	if c.StoryCode != "" {
		if rows, err := d.Query(ctx, `SELECT feature_code FROM kb_story_features WHERE product_id = ? AND story_code = ? ORDER BY feature_code LIMIT 20`, pid, c.StoryCode); err == nil {
			for rows.Next() {
				var f string
				if rows.Scan(&f) == nil {
					features = append(features, f)
				}
			}
			rows.Close()
		}
	}
	var gaosOn bool
	var gaosPID sql.NullString
	_ = d.QueryRow(ctx, `SELECT gaos_integrated, gaos_product_id FROM product_settings WHERE product_id = ?`, pid).Scan(&gaosOn, &gaosPID)
	job = caseauto.Job{
		ID: jobID, ProductID: pid, ProductName: name, DefaultBranch: repo.DefaultBranch, AllowedHosts: hosts,
		Branch: repopool.AutoBranch(code),
		Case:   caseauto.Case{Code: code, Scenario: c.Scenario, StoryCode: c.StoryCode, StoryText: storyText.String, Features: features},
	}
	if gaosOn && gaosPID.Valid && r.gaos.Configured() {
		job.GaosProductID = gaosPID.String
	}
	if o := r.dashboardOrigin; o != "" && o != "*" {
		job.JobURL = strings.TrimRight(o, "/") + "/jobs/" + jobID
	}
	if sp, ok := payload["spec_paths"].([]interface{}); ok && len(sp) > 0 {
		job.SpecPath, _ = sp[0].(string)
	}
	return job, repo, nil
}

func parseHost(raw string) string {
	raw = strings.TrimSpace(raw)
	i := strings.Index(raw, "://")
	if i < 0 {
		return ""
	}
	h := raw[i+3:]
	if j := strings.IndexAny(h, "/?#"); j >= 0 {
		h = h[:j]
	}
	if j := strings.LastIndex(h, ":"); j >= 0 {
		h = h[:j]
	}
	return strings.ToLower(h)
}

// newMachine wires the caseauto machine to this router's services for one product repo.
func (r *Router) newMachine(d *db.DB, jobID string, pid int, repo *automationRepo) *caseauto.Machine {
	var inbox caseauto.Inbox
	if r.gaos.Configured() {
		inbox = r.gaos
	}
	return caseauto.NewMachine(
		&poolRepo{r: r, d: d, repo: repo},
		r.aiService,
		&executorRunner{r: r, jobID: jobID, pid: pid},
		&caseauto.ApprovalStore{DB: d},
		&caseauto.EventStore{DB: d, Redact: r.caseAuto.redact},
		jobState{r},
		inbox,
		r.caseAuto.redact,
	)
}

const caseCreateMaxDuration = 76 * time.Hour // the 72 h approval plus working time

// runCaseCreate is the case_create job body.
func (r *Router) runCaseCreate(id string, payload map[string]interface{}, resume bool) {
	ctx, cancel := context.WithTimeout(context.Background(), caseCreateMaxDuration)
	defer cancel()
	fail := func(err error, step string) {
		msg := r.caseAuto.redact.Redact(err.Error())
		if !caseauto.IsStepErr(err) {
			r.addJobEvent(id, "error", step, msg)
		}
		r.finishJob(id, jobStatusFailed, caseauto.Clip(msg, 1000), nil)
		log.Printf("job %s (case_create) failed: %s", id, msg)
	}
	d := r.getDB()
	if d == nil || r.aiService == nil || r.repoPool == nil || r.executor == nil {
		fail(errors.New("case automation is not available"), caseauto.StepPrepare)
		return
	}
	r.markJobRunning(id)
	job, repo, err := r.caseJobFor(ctx, d, id, payload)
	if err != nil {
		fail(err, caseauto.StepPrepare)
		return
	}
	m := r.newMachine(d, id, job.ProductID, repo)
	var out *caseauto.Outcome
	if resume {
		out, err = m.Resume(ctx, job)
	} else {
		out, err = m.Run(ctx, job)
	}
	if err != nil {
		fail(err, caseauto.StepDone)
		return
	}
	r.finishJob(id, jobStatusCompleted, "", map[string]interface{}{"pr_url": out.PRURL, "spec_path": out.SpecPath, "repairs": out.Repairs})
	log.Printf("job %s (case_create) completed: PR opened", id)
}

// ResumeCaseApprovals restarts the approval wait of case_create jobs whose approval was open
// when the previous process stopped (FailOrphanedJobs leaves those jobs alone).
func (r *Router) ResumeCaseApprovals(ctx context.Context) {
	d := r.getDB()
	if d == nil || r.caseAuto.hostPool == "" {
		return
	}
	ids, err := (&caseauto.ApprovalStore{DB: d}).OpenForJobs(ctx)
	if err != nil {
		log.Printf("resume case approvals: %v", err)
		return
	}
	for _, id := range ids {
		row := d.QueryRow(ctx, `SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms FROM job_runs WHERE id = ?`, id)
		job, err := scanJobRow(row)
		if err != nil || job.Type != "case_create" || job.Status != jobStatusRunning {
			continue
		}
		rec := job
		r.jobs.mu.Lock()
		r.jobs.jobs[id] = &rec
		r.jobs.mu.Unlock()
		log.Printf("resuming the approval of job %s", id)
		go r.runCaseCreate(id, job.Payload, true)
	}
}

// ---- adapters ----

// poolRepo is caseauto.RepoOps over the repo pool. Push and open-PR pass repoWriteAllowed first.
type poolRepo struct {
	r    *Router
	d    *db.DB
	repo *automationRepo
}

func (p *poolRepo) Prepare(ctx context.Context, job caseauto.Job) (caseauto.Workspace, error) {
	pool := p.r.repoPool
	if err := pool.FetchOrigin(ctx, p.repo.RootDir); err != nil {
		return caseauto.Workspace{}, err
	}
	wt, err := pool.AddWorktree(ctx, p.repo.RootDir, job.Branch, job.DefaultBranch)
	if err != nil {
		return caseauto.Workspace{}, err
	}
	if _, err := pool.LinkNodeModules(p.repo.RootDir, wt); err != nil {
		return caseauto.Workspace{}, err
	}
	return caseauto.Workspace{Dir: wt, Branch: job.Branch, Base: job.DefaultBranch}, nil
}

func (p *poolRepo) Open(_ context.Context, job caseauto.Job) (caseauto.Workspace, error) {
	wt, err := p.r.repoPool.OpenWorktree(p.repo.RootDir, job.Branch)
	if err != nil {
		return caseauto.Workspace{}, err
	}
	return caseauto.Workspace{Dir: wt, Branch: job.Branch, Base: job.DefaultBranch}, nil
}

var safeRelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// confined resolves rel inside the worktree and refuses symlink escapes.
func confined(ws caseauto.Workspace, rel string) (string, error) {
	if len(rel) > 300 || !safeRelRe.MatchString(rel) || path.Clean(rel) != rel || strings.Contains(rel, "..") {
		return "", errors.New("invalid path")
	}
	root, err := filepath.EvalSymlinks(ws.Dir)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	probe := abs
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		next := filepath.Dir(probe)
		if next == probe {
			return "", errors.New("path escapes the worktree")
		}
		probe = next
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil || (real != root && !strings.HasPrefix(real, root+string(os.PathSeparator))) {
		return "", errors.New("path escapes the worktree")
	}
	return abs, nil
}

func (p *poolRepo) ReadFile(ws caseauto.Workspace, rel string) ([]byte, error) {
	abs, err := confined(ws, rel)
	if err != nil {
		return nil, err
	}
	fi, err := os.Lstat(abs)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return nil, errors.New("not a readable regular file")
	}
	return os.ReadFile(abs)
}

func (p *poolRepo) ListSpecs(ws caseauto.Workspace) ([]string, error) {
	var out []string
	for _, base := range []string{"tests/web", "tests/api/rest"} {
		root := filepath.Join(ws.Dir, filepath.FromSlash(base))
		_ = filepath.WalkDir(root, func(p string, de os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if de.IsDir() && de.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if de.Type().IsRegular() && strings.HasSuffix(p, ".spec.ts") && len(out) < 2000 {
				if rel, err := filepath.Rel(ws.Dir, p); err == nil {
					out = append(out, filepath.ToSlash(rel))
				}
			}
			return nil
		})
	}
	return out, nil
}

func (p *poolRepo) WriteFile(ws caseauto.Workspace, rel string, data []byte) error {
	// Re-check the path: only a spec under tests/ is ever written.
	if !runnableSpec(rel) || !strings.Contains(rel, "/") {
		return errors.New("refusing to write outside tests/web or tests/api/rest")
	}
	abs, err := confined(ws, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if _, err := confined(ws, rel); err != nil { // the directory now exists: re-resolve
		return err
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (p *poolRepo) DiffStat(ctx context.Context, ws caseauto.Workspace, rel string) (string, error) {
	return p.r.repoPool.DiffStat(ctx, ws.Dir, []string{rel})
}

func (p *poolRepo) Commit(ctx context.Context, ws caseauto.Workspace, message, rel string) error {
	return p.r.repoPool.Commit(ctx, ws.Dir, message, []string{rel})
}

func (p *poolRepo) Push(ctx context.Context, ws caseauto.Workspace) error {
	if err := repoWriteAllowed(ctx, p.d, p.repo.ID, RepoOpPush); err != nil {
		return err
	}
	return p.r.repoPool.Push(ctx, ws.Dir, ws.Branch)
}

func (p *poolRepo) OpenPR(ctx context.Context, ws caseauto.Workspace, title, body string) (string, error) {
	if err := repoWriteAllowed(ctx, p.d, p.repo.ID, RepoOpOpenPR); err != nil {
		return "", err
	}
	return p.r.repoPool.OpenPR(ctx, ws.Dir, ws.Base, ws.Branch, title, body)
}

// executorRunner is caseauto.Executor over playwright-executor (case_run on the worktree).
type executorRunner struct {
	r     *Router
	jobID string
	pid   int
}

func (e *executorRunner) RunCase(ctx context.Context, req caseauto.RunRequest, emit func(level, step, msg string)) (caseauto.RunResult, error) {
	hostDir, err := e.r.repoPool.HostPath(e.r.caseAuto.hostPool, req.Workspace.Dir)
	if err != nil {
		return caseauto.RunResult{}, err
	}
	params := map[string]interface{}{
		"repo_dir": hostDir, "case_code": req.CaseCode, "spec_paths": []string{req.SpecPath},
		"env_file": executorEnvFile(e.pid), "repair": req.Repair,
	}
	rctx, cancel := context.WithTimeout(ctx, e.r.executorTimeout)
	defer cancel()
	run, err := e.r.executor.Submit(rctx, executor.SubmitRequest{JobID: e.jobID, Type: "case_run", Params: params})
	if err != nil {
		return caseauto.RunResult{}, err
	}
	final, err := e.r.executor.Wait(rctx, run.ID, 2*time.Second, func(u *executor.Run) {
		for _, ev := range u.Events {
			emit(ev.Level, ev.Step, ev.Message)
		}
	})
	if err != nil {
		return caseauto.RunResult{}, err
	}
	if final.Status == executor.StatusFailed {
		return caseauto.RunResult{}, fmt.Errorf("playwright-executor: %s", final.Error)
	}
	v := caseauto.VerdictFromResult(final.Result, req.CaseCode, req.SpecPath, time.Now().UTC())
	if v.Report != nil {
		idx := req.Repair
		e.r.recordRun(ctx, runhistory.RunIn{ProductID: e.pid, Source: runhistory.SourceCaseCreate, RunRef: e.jobID, JobID: e.jobID, CaseCode: req.CaseCode, RunIndex: &idx}, v.Report)
	}
	tail, _ := final.Result["tail"].(string)
	return caseauto.RunResult{Pass: v.Pass, Message: v.Message, Tail: tail, Passed: v.Passed, Failed: v.Failed, Skipped: v.Skipped}, nil
}

// answerBy names the caller for the event log.
func answerBy(req *http.Request) string {
	if u, ok := auth.GetUser(req.Context()); ok {
		return u.Username
	}
	return ""
}
