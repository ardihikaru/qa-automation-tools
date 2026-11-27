package api

// Restart handling of drafts_publish jobs (KNOWLEDGE_TRANSLATION.md, step 7 and open question 3).
// FailOrphanedJobs leaves the type alone; ResumeDraftPublishes decides per job, conservatively:
//   - the pull request exists (payload.pr_url): the job is completed and the publication is pr_open;
//   - a push or PR step was reached without a PR url: the branch may be on the remote, so the job and
//     the publication fail (the drafts become publishable again; a retry makes a fresh branch);
//   - otherwise nothing left the machine: the job runs again from the start on the same branch name
//     (the worktree and local branch of that name are replaced), at most maxOrphanRetries times.

import (
	"context"
	"fmt"
	"log"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
)

type publishOrphanFacts struct {
	Retries     int
	PRURL       string
	PushReached bool
}

func decidePublishOrphan(f publishOrphanFacts) orphanDecision {
	fail := func(format string, a ...interface{}) orphanDecision {
		return orphanDecision{Action: orphanFail, Message: fmt.Sprintf(orphanBaseMsg+"; "+format, a...)}
	}
	switch {
	case f.PRURL != "":
		return orphanDecision{Action: orphanComplete}
	case f.PushReached:
		return fail("it had already reached the push step, so it was not re-run (the branch may be pushed and a pull request may exist: check the automation repo; the drafts can be published again)")
	case f.Retries >= maxOrphanRetries:
		return fail("automatic retries exhausted (%d of %d used)", f.Retries, maxOrphanRetries)
	}
	d := orphanDecision{Action: orphanRestartCreate, Attempt: f.Retries + 2}
	d.Message = fmt.Sprintf("backend restarted while this job was running; retrying from the start on the same branch, attempt %d/%d", d.Attempt, maxOrphanRetries+1)
	return d
}

// ResumeDraftPublishes applies decidePublishOrphan to every queued or running drafts_publish job.
func (r *Router) ResumeDraftPublishes(ctx context.Context) {
	d := r.getDB()
	if d == nil {
		return
	}
	rows, err := d.Query(ctx, `SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms
		FROM job_runs WHERE type = ? AND status IN ('queued', 'running') ORDER BY created_at`, JobTypeDraftsPublish)
	if err != nil {
		log.Printf("resume draft publishes: %v", err)
		return
	}
	var jobs []jobRecord
	for rows.Next() {
		if j, err := scanJobRow(rows); err == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	var retried, failed, completed int
	for _, j := range jobs {
		rec := j
		r.jobs.mu.Lock()
		if _, held := r.jobs.jobs[rec.ID]; held {
			r.jobs.mu.Unlock()
			continue
		}
		r.jobs.jobs[rec.ID] = &rec
		r.jobs.mu.Unlock()

		f := publishOrphanFacts{Retries: intFrom(rec.Payload["retries"]), PRURL: str(rec.Payload, "pr_url")}
		if p, err := (&caseauto.EventStore{DB: d, Redact: r.caseAuto.redact}).Progress(ctx, rec.ID); err != nil {
			f.PushReached = true // unknown progress: never risk a second branch or PR
			log.Printf("resume draft publishes: job %s: reading events: %v", rec.ID, err)
		} else {
			f.PushReached = p.PushReached
		}
		dec := decidePublishOrphan(f)
		usable := r.kb != nil && r.publishRepo() != nil
		switch {
		case dec.Action == orphanComplete:
			r.addJobEvent(rec.ID, "info", caseauto.StepDone, "backend restarted after the pull request was opened: "+f.PRURL)
			if r.kb != nil {
				_ = r.kb.PublicationOpened(ctx, rec.ID, f.PRURL)
			}
			r.finishJob(rec.ID, jobStatusCompleted, "", map[string]interface{}{"pr_url": f.PRURL, "branch": str(rec.Payload, "branch"), "epic_code": str(rec.Payload, "epic_code")})
			completed++
		case dec.Action == orphanFail || !usable:
			msg := dec.Message
			if dec.Action != orphanFail {
				msg = orphanBaseMsg + "; publishing is not configured on this server"
			}
			r.addJobEvent(rec.ID, "error", caseauto.StepPrepare, msg)
			if r.kb != nil {
				_ = r.kb.PublicationFailed(ctx, rec.ID, msg)
			}
			r.finishJob(rec.ID, jobStatusFailed, msg, nil)
			failed++
		default:
			r.setJobPayload(rec.ID, "retries", f.Retries+1)
			r.addJobEvent(rec.ID, "warn", caseauto.StepPrepare, dec.Message)
			go r.runDraftsPublish(rec.ID, r.jobPayloadCopy(rec.ID))
			retried++
		}
	}
	if len(jobs) > 0 {
		log.Printf("orphaned draft publishes: %d retried, %d failed, %d completed", retried, failed, completed)
	}
}
