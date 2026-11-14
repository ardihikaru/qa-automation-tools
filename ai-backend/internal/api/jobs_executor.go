package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/executor"
)

// Job type -> playwright-executor run type.
var executorRunTypes = map[string]string{
	"case_run": "case_run", // CASE_AUTOMATION.md
}

// SetExecutor makes case_run jobs dispatch to playwright-executor. timeout bounds queueing + execution per job.
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
// into the job record, then post-processes the result.
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

	params := caseRunParams(payload)
	run, err := r.executor.Submit(ctx, executor.SubmitRequest{JobID: id, Type: runType, Params: params})
	if err != nil {
		fail(err)
		return
	}
	log.Printf("job %s (%s) submitted to playwright-executor as %s", id, jobType, run.ID)

	r.addJobEvent(id, "info", "run", "submitted to playwright-executor")
	final, err := r.executor.Wait(ctx, run.ID, 2*time.Second, func(u *executor.Run) {
		r.forwardExecutorEvents(id, u)
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

	if err := r.finishCaseRun(ctx, id, payload, result); err != nil {
		r.updateJob(id, func(j *jobRecord) { j.Result = result })
		fail(err)
		return
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
