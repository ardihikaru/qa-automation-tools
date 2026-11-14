package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/executor"
)

// Job type -> playwright-executor run type.
var executorRunTypes = map[string]string{
	"qa_run":        "qa_run",
	"docs_generate": "docs_generate",
	"case_run":      "case_run", // CASE_AUTOMATION.md
}

// Params a job payload may forward to the executor; everything else in the
// payload (triggered_by, source, ...) stays backend-side.
var executorParamKeys = []string{"roles", "max_steps", "interactive_skills", "flows"}

// SetExecutor makes /api/jobs/qa/run and /api/jobs/docs/generate dispatch to
// playwright-executor. timeout bounds queueing + execution per job.
func (r *Router) SetExecutor(c *executor.Client, timeout time.Duration) {
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	r.executor = c
	r.executorTimeout = timeout
}

// FailOrphanedJobs marks jobs left queued/running by a previous backend
// process as failed: their polling goroutine died with that process.
func (r *Router) FailOrphanedJobs(ctx context.Context) {
	db := r.getDB()
	if db == nil {
		return
	}
	res, err := db.Exec(ctx, `
		UPDATE job_runs
		SET status = 'failed', error = 'backend restarted while the job was in progress', completed_at = UTC_TIMESTAMP()
		WHERE status IN ('queued', 'running')
		AND NOT (type = 'case_create' AND EXISTS (SELECT 1 FROM case_approvals a WHERE a.job_id = job_runs.id AND a.status = 'open'))`)
	if err != nil {
		// case_approvals may not exist yet (migration pending): fall back to the plain statement.
		res, err = db.Exec(ctx, `
		UPDATE job_runs
		SET status = 'failed', error = 'backend restarted while the job was in progress', completed_at = UTC_TIMESTAMP()
		WHERE status IN ('queued', 'running')`)
	}
	if err != nil {
		log.Printf("failed to reconcile orphaned jobs: %v", err)
		return
	}
	if n := res.RowsAffected(); n > 0 {
		log.Printf("marked %d orphaned job(s) as failed", n)
	}
}

func executorParams(payload map[string]interface{}) map[string]interface{} {
	src := payload
	if nested, ok := payload["params"].(map[string]interface{}); ok {
		src = nested
	}
	out := map[string]interface{}{}
	for _, k := range executorParamKeys {
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	return out
}

func (r *Router) updateJob(id string, mutate func(*jobRecord)) *jobRecord {
	r.jobs.mu.Lock()
	rec := r.jobs.jobs[id]
	if rec == nil {
		r.jobs.mu.Unlock()
		return nil
	}
	mutate(rec)
	snapshot := *rec
	r.jobs.mu.Unlock()
	r.upsertJobRecord(context.Background(), &snapshot)
	return &snapshot
}

// runExecutorJob submits the job to playwright-executor, mirrors its progress
// into the job record, then post-processes the result (issues, AI summary).
func (r *Router) runExecutorJob(id, jobType string, payload map[string]interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), r.executorTimeout)
	defer cancel()

	fail := func(err error) {
		now := time.Now().UTC()
		r.updateJob(id, func(j *jobRecord) {
			j.Status = jobStatusFailed
			j.Error = err.Error()
			j.CompletedAt = &now
			if j.StartedAt != nil {
				j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
			}
		})
		log.Printf("job %s (%s) failed: %v", id, jobType, err)
	}

	runType, ok := executorRunTypes[jobType]
	if !ok {
		fail(fmt.Errorf("job type %q is not supported by playwright-executor", jobType))
		return
	}

	params := executorParams(payload)
	if jobType == "case_run" {
		params = caseRunParams(payload)
	}
	run, err := r.executor.Submit(ctx, executor.SubmitRequest{JobID: id, Type: runType, Params: params})
	if err != nil {
		fail(err)
		return
	}
	log.Printf("job %s (%s) submitted to playwright-executor as %s", id, jobType, run.ID)

	if jobType == "case_run" {
		r.addJobEvent(id, "info", "run", "submitted to playwright-executor")
	}
	final, err := r.executor.Wait(ctx, run.ID, pollEvery(jobType), func(u *executor.Run) {
		if jobType == "case_run" {
			r.forwardExecutorEvents(id, u)
		}
		if u.Status != executor.StatusRunning {
			return
		}
		r.jobs.mu.RLock()
		already := r.jobs.jobs[id].Status == jobStatusRunning
		r.jobs.mu.RUnlock()
		if already {
			return
		}
		now := time.Now().UTC()
		r.updateJob(id, func(j *jobRecord) {
			j.Status = jobStatusRunning
			j.StartedAt = &now
			j.Result = map[string]interface{}{"executor_run_id": run.ID}
		})
	})
	if err != nil {
		fail(err)
		return
	}

	result := final.Result
	if result == nil {
		result = map[string]interface{}{}
	}
	result["executor_run_id"] = final.ID
	if final.Status == executor.StatusFailed {
		r.updateJob(id, func(j *jobRecord) { j.Result = result })
		fail(fmt.Errorf("playwright-executor: %s", final.Error))
		return
	}

	switch jobType {
	case "case_run":
		if err := r.finishCaseRun(ctx, id, payload, result); err != nil {
			r.updateJob(id, func(j *jobRecord) { j.Result = result })
			fail(err)
			return
		}
	case "qa_run":
		result["issues_recorded"] = r.recordExecutorIssues(ctx, id, result)
	case "docs_generate":
		if md, err := r.summarizeDocsRun(ctx, result); err != nil {
			result["ai_error"] = err.Error()
		} else {
			result["documentation_markdown"] = md
		}
	}

	now := time.Now().UTC()
	r.updateJob(id, func(j *jobRecord) {
		j.Status = jobStatusCompleted
		j.Result = result
		j.CompletedAt = &now
		if j.StartedAt == nil {
			j.StartedAt = &now
		}
		j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
	})
	log.Printf("job %s (%s) completed via playwright-executor", id, jobType)
}

// pollEvery is the executor poll period: case runs are watched live (2 s), the rest as before.
func pollEvery(jobType string) time.Duration {
	if jobType == "case_run" {
		return 2 * time.Second
	}
	return 3 * time.Second
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// recordExecutorIssues inserts qa_run issues into raw_issues, skipping ones
// that already exist unfixed (same page, action and text). Returns the count.
func (r *Router) recordExecutorIssues(ctx context.Context, jobID string, result map[string]interface{}) int {
	db := r.getDB()
	list, _ := result["issues"].([]interface{})
	if db == nil || len(list) == 0 {
		return 0
	}
	inserted := 0
	for _, raw := range list {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		str := func(k string) string { s, _ := m[k].(string); return s }
		page := clip(pathOnly(str("url")), 255)
		if page == "" {
			page = "unknown-page"
		}
		action := str("skill")
		if action == "" {
			action = str("type")
		}
		action = clip(action, 255)
		text := str("title")
		if msg := str("message"); msg != "" {
			text += ": " + msg
		}
		severity := str("severity")
		if severity == "" {
			severity = "medium"
		}

		var exists int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*) FROM raw_issues
			WHERE page = ? AND action = ? AND error_text = ? AND (is_fixed = 0 OR is_fixed IS NULL)`,
			page, action, text).Scan(&exists); err == nil && exists > 0 {
			continue
		}
		notes := fmt.Sprintf("playwright-executor job %s, role %s", jobID, str("role"))
		if _, err := db.Exec(ctx, `
			INSERT INTO raw_issues (page, action, error_type, error_text, category, severity, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			page, action, clip("executor:"+str("type"), 128), text, "frontend", severity, notes); err != nil {
			log.Printf("job %s: insert issue failed: %v", jobID, err)
			continue
		}
		inserted++
	}
	return inserted
}

func pathOnly(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[j:]
		}
		return "/"
	}
	return u
}

// summarizeDocsRun turns recorded flows into Markdown through ai.Service
// (ai-manager, pinned to claude-v3). Screenshots paths are dropped from the prompt.
func (r *Router) summarizeDocsRun(ctx context.Context, result map[string]interface{}) (string, error) {
	if r.aiService == nil || !r.aiService.Configured() {
		return "", fmt.Errorf("AI not configured")
	}
	flows, _ := result["flows"].([]interface{})
	if len(flows) == 0 {
		return "", fmt.Errorf("no flows recorded")
	}
	type step struct {
		Action  string `json:"action"`
		Target  string `json:"target,omitempty"`
		OK      bool   `json:"ok"`
		URL     string `json:"url"`
		Title   string `json:"title,omitempty"`
		Heading string `json:"heading,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	type flow struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		Description string `json:"description"`
		Steps       []step `json:"steps"`
	}
	var compact []flow
	raw, _ := json.Marshal(flows)
	if err := json.Unmarshal(raw, &compact); err != nil {
		return "", err
	}
	data, _ := json.Marshal(compact)

	prompt := "You are writing user documentation for HISv3, a hospital information system. " +
		"Below are UI flows recorded by an automated browser (one per role, with the page URL, title and main heading after each step; ok=false means the step failed). " +
		"Write concise Markdown: one section per flow with a one-paragraph purpose, numbered steps a user follows, and a short 'Known problems' list only for failed steps. " +
		"Do not invent screens that are not in the data.\n\nFLOWS JSON:\n" + clip(string(data), 60000)

	text, err := r.aiService.ChatMessages(ctx, []models.Message{{Role: "user", Content: prompt}}, ai.ChatOpts{Temperature: 0.3, Feature: "job-analysis"})
	if err != nil {
		return "", err
	}
	return text, nil
}
