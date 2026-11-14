package api

// CASE_AUTOMATION.md section 5 "Orphans": a backend restart does not fail case_run / case_create
// jobs any more. ResumeOrphanedJobs (called at startup after the executor, GAOS and AI wiring)
// retries them, at most maxOrphanRetries times per job (payload.retries), so a deploy that
// collides with a live run costs a retry instead of the run.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/executor"
)

// maxOrphanRetries is the number of automatic retries a job gets (3 attempts in all).
const maxOrphanRetries = 2

// canResumeCaseJobs reports whether case jobs left by a dead process can be resumed here.
func (r *Router) canResumeCaseJobs() bool {
	return r.getDB() != nil && r.executor != nil && r.caseAuto.hostPool != ""
}

type orphanAction int

const (
	orphanFail            orphanAction = iota // mark failed with the message
	orphanComplete                            // the PR already exists: mark completed
	orphanRetryRun                            // case_run: re-attach to the executor run or resubmit
	orphanRestartCreate                       // case_create: the machine restarts from prepare
	orphanResumeCreateApp                     // case_create: approved before the restart, publish the written spec
)

// orphanFacts is everything the decision needs; the DB reads happen before, so it is pure.
type orphanFacts struct {
	Type           string
	Retries        int
	PRURL          string // payload.pr_url
	PushReached    bool   // job_events has step push or pr
	ApprovalStatus string // latest non-open case_approvals status of the job, "" = none
}

type orphanDecision struct {
	Action  orphanAction
	Message string // the job error (fail) or the warn event (retry)
	Attempt int    // 1-based attempt number the retry will be, of maxOrphanRetries+1
}

const orphanBaseMsg = "backend restarted while the job was in progress"

func decideOrphan(f orphanFacts) orphanDecision {
	fail := func(format string, a ...interface{}) orphanDecision {
		return orphanDecision{Action: orphanFail, Message: fmt.Sprintf(orphanBaseMsg+"; "+format, a...)}
	}
	switch f.Type {
	case "case_run", "case_create":
	default:
		return orphanDecision{Action: orphanFail, Message: orphanBaseMsg}
	}
	if f.Type == "case_create" {
		if f.PRURL != "" {
			return orphanDecision{Action: orphanComplete}
		}
		if f.PushReached {
			return fail("it had already reached the push/pull request step, so it was not re-run (the branch may be pushed and a pull request may exist: check the automation repo, then start a new job)")
		}
	}
	if f.Retries >= maxOrphanRetries {
		return fail("automatic retries exhausted (%d of %d used)", f.Retries, maxOrphanRetries)
	}
	d := orphanDecision{Attempt: f.Retries + 2}
	d.Message = fmt.Sprintf("backend restarted while this job was running; retrying from the start, attempt %d/%d", d.Attempt, maxOrphanRetries+1)
	switch {
	case f.Type == "case_run":
		d.Action = orphanRetryRun
	case f.ApprovalStatus == caseauto.ApprovalApproved:
		d.Action = orphanResumeCreateApp
		d.Message = fmt.Sprintf("backend restarted after the spec was approved; resuming at the publish step, attempt %d/%d", d.Attempt, maxOrphanRetries+1)
	case f.ApprovalStatus != "":
		// A regenerated spec would inherit the old answer (the external id is per job): refuse.
		return fail("it had already passed the approval step (%s); start a new job", f.ApprovalStatus)
	default:
		d.Action = orphanRestartCreate
	}
	return d
}

// submitOrReattach is the executor step of a case_run: after a restart (payload.retries > 0) a
// known executor run of this job is re-attached instead of resubmitted, so a spec never runs twice.
func (r *Router) submitOrReattach(ctx context.Context, jobID, runType string, params map[string]interface{}) (*executor.Run, error) {
	if prev := r.priorExecutorRun(jobID); prev != "" {
		run, err := r.executor.Get(ctx, prev)
		switch {
		case err == nil && (run.JobID == "" || run.JobID == jobID):
			r.addJobEvent(jobID, "info", caseauto.StepRun, "re-attached to the playwright-executor run started before the restart (its earlier events are repeated below)")
			return run, nil
		case err == nil || errors.Is(err, executor.ErrRunNotFound):
			r.addJobEvent(jobID, "warn", caseauto.StepRun, "playwright-executor no longer has the run started before the restart; submitting it again")
		default:
			return nil, fmt.Errorf("cannot re-attach to the playwright-executor run started before the restart: %w", err)
		}
	}
	run, err := r.executor.Submit(ctx, executor.SubmitRequest{JobID: jobID, Type: runType, Params: params})
	if err != nil {
		return nil, err
	}
	// Remember the run id at once (not only when it starts) so a restart can re-attach.
	r.updateJob(jobID, func(j *jobRecord) {
		j.Result = map[string]interface{}{"executor_run_id": run.ID}
	})
	r.addJobEvent(jobID, "info", caseauto.StepRun, "submitted to playwright-executor")
	return run, nil
}

// priorExecutorRun is the executor run id of a job that was retried after a restart, else "".
func (r *Router) priorExecutorRun(jobID string) string {
	r.jobs.mu.RLock()
	defer r.jobs.mu.RUnlock()
	j := r.jobs.jobs[jobID]
	if j == nil || intFrom(j.Payload["retries"]) <= 0 {
		return ""
	}
	id, _ := j.Result["executor_run_id"].(string)
	return id
}

// ResumeOrphanedJobs decides, for every case_run / case_create job still queued or running in the
// database, between a retry and a failure (decideOrphan). It skips jobs with an open approval
// (ResumeCaseApprovals owns those) and jobs this process already holds. Only counts are logged.
func (r *Router) ResumeOrphanedJobs(ctx context.Context) {
	d := r.getDB()
	if d == nil || !r.canResumeCaseJobs() {
		return
	}
	rows, err := d.Query(ctx, `SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms
		FROM job_runs WHERE type IN ('case_run', 'case_create') AND status IN ('queued', 'running') ORDER BY created_at`)
	if err != nil {
		log.Printf("resume orphaned jobs: %v", err)
		return
	}
	var jobs []jobRecord
	for rows.Next() {
		if j, err := scanJobRow(rows); err == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	if len(jobs) == 0 {
		return
	}
	open := map[string]bool{}
	openKnown := true
	if ids, err := (&caseauto.ApprovalStore{DB: d}).OpenForJobs(ctx); err != nil {
		log.Printf("resume orphaned jobs: reading open approvals: %v", err)
		openKnown = false
	} else {
		for _, id := range ids {
			open[id] = true
		}
	}
	var retried, failed, completed int
	for _, j := range jobs {
		rec := j
		r.jobs.mu.RLock()
		_, held := r.jobs.jobs[rec.ID]
		r.jobs.mu.RUnlock()
		if held || open[rec.ID] || (rec.Type == "case_create" && !openKnown) {
			continue
		}
		switch r.resumeOrphan(ctx, &rec) {
		case orphanFail:
			failed++
		case orphanComplete:
			completed++
		default:
			retried++
		}
	}
	log.Printf("orphaned case jobs: %d retried, %d failed, %d completed", retried, failed, completed)
}

// resumeOrphan applies decideOrphan to one job and returns the action taken.
func (r *Router) resumeOrphan(ctx context.Context, rec *jobRecord) orphanAction {
	d := r.getDB()
	step := caseauto.StepRun
	f := orphanFacts{Type: rec.Type, Retries: intFrom(rec.Payload["retries"])}
	if rec.Type == "case_create" {
		step = caseauto.StepPrepare
		f.PRURL, _ = rec.Payload["pr_url"].(string)
		es := &caseauto.EventStore{DB: d, Redact: r.caseAuto.redact}
		if p, err := es.Progress(ctx, rec.ID); err != nil {
			// Unknown progress: never risk a second PR.
			f.PushReached = true
			log.Printf("resume orphaned jobs: job %s: reading events: %v", rec.ID, err)
		} else {
			f.PushReached = p.PushReached
			if p.LastStep != "" {
				step = p.LastStep
			}
		}
		if a, err := (&caseauto.ApprovalStore{DB: d}).Latest(ctx, rec.ID); err != nil {
			f.PushReached = true
			log.Printf("resume orphaned jobs: job %s: reading approval: %v", rec.ID, err)
		} else if a != nil {
			f.ApprovalStatus = a.Status
		}
	}
	dec := decideOrphan(f)

	r.jobs.mu.Lock()
	r.jobs.jobs[rec.ID] = rec
	r.jobs.mu.Unlock()

	switch dec.Action {
	case orphanFail:
		r.addJobEvent(rec.ID, "error", step, dec.Message)
		r.finishJob(rec.ID, jobStatusFailed, dec.Message, nil)
	case orphanComplete:
		spec, _ := rec.Payload["spec_path"].(string)
		r.addJobEvent(rec.ID, "info", caseauto.StepDone, "backend restarted after the pull request was opened: "+f.PRURL)
		r.finishJob(rec.ID, jobStatusCompleted, "", map[string]interface{}{"pr_url": f.PRURL, "spec_path": spec})
	default:
		r.setJobPayload(rec.ID, "retries", f.Retries+1)
		r.addJobEvent(rec.ID, "warn", step, dec.Message)
		payload := r.jobPayloadCopy(rec.ID)
		switch dec.Action {
		case orphanRetryRun:
			go r.runExecutorJob(rec.ID, rec.Type, payload)
		case orphanResumeCreateApp:
			go r.runCaseCreate(rec.ID, payload, true)
		default:
			go r.runCaseCreate(rec.ID, payload, false)
		}
	}
	return dec.Action
}

func (r *Router) jobPayloadCopy(id string) map[string]interface{} {
	r.jobs.mu.RLock()
	defer r.jobs.mu.RUnlock()
	out := map[string]interface{}{}
	if j := r.jobs.jobs[id]; j != nil {
		for k, v := range j.Payload {
			out[k] = v
		}
	}
	return out
}

// BeginDrain makes the case routes refuse new jobs (503) while the server shuts down.
func (r *Router) BeginDrain() { r.caseAuto.draining.Store(true) }

// DrainTimeout bounds the graceful shutdown of in-flight HTTP requests.
const DrainTimeout = 20 * time.Second
