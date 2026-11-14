// Package epicauto holds the pure decisions of EPIC_AUTOMATION.md: what state a case of a
// batch is in, and what the driver should do next. It touches no database, clock or network;
// the api package loads the facts and applies the decision.
package epicauto

import (
	"sort"
	"strings"
	"time"
)

const (
	MaxOpen         = 2  // jobs of one batch in flight at once
	DefaultMaxCases = 10 // cases taken at start
	MaxCases        = 25
	StopAfter       = 3 // consecutive technical failures that stop a batch
)

// Batch statuses (epic_batches.status).
const (
	BatchRunning   = "running"
	BatchCompleted = "completed"
	BatchCancelled = "cancelled"
	BatchStopped   = "stopped"
)

// State is a case's derived state inside a batch.
type State string

const (
	Pending          State = "pending"           // eligible and waiting for a slot
	Active           State = "active"            // its job is queued or running
	AwaitingApproval State = "awaiting_approval" // active, and the spec waits for a human
	PR               State = "pr"                // the spec was approved and a PR is open
	Declined         State = "declined"          // a human said no, or did not answer
	Failed           State = "failed"            // a technical failure
	NeedsPerson      State = "needs_person"      // failed after a push/PR step: a branch may exist
	Skipped          State = "skipped"           // no job, and no longer eligible
)

// JobFacts is what the api layer reads about the newest case_create job of a case in a batch.
type JobFacts struct {
	ID               string
	Status           string // queued | running | completed | failed (anything else counts as failed)
	Err              string
	PRURL            string
	PushedOrPR       bool // a job_events row of step push or pr exists
	AwaitingApproval bool // a case_approvals row of the job is open
	DoneAt           time.Time
}

// Case is one snapshot entry of a batch.
type Case struct {
	Code     string
	Job      *JobFacts // nil: no job of this batch for the case
	Eligible bool      // meaningful only when Job is nil
}

// HumanOutcome reports whether a failed job's error is the operator's decision (or silence)
// rather than a fault: a rejection, an expired approval, too many change rounds, or an orphan
// that had already passed the approval step.
func HumanOutcome(err string) bool {
	e := strings.TrimSpace(err)
	for _, p := range []string{"rejected", "approval expired", "changes were requested too many times"} {
		if strings.HasPrefix(e, p) {
			return true
		}
	}
	return strings.Contains(e, "already passed the approval step")
}

// Classify maps a case's facts to its state.
func Classify(c Case) State {
	j := c.Job
	if j == nil {
		if c.Eligible {
			return Pending
		}
		return Skipped
	}
	switch j.Status {
	case "queued", "running":
		if j.AwaitingApproval {
			return AwaitingApproval
		}
		return Active
	case "completed":
		if j.PRURL != "" {
			return PR
		}
		return Failed
	}
	switch {
	case j.PushedOrPR:
		return NeedsPerson
	case HumanOutcome(j.Err):
		return Declined
	}
	return Failed
}

// Decision is what the driver does after looking at a batch.
type Decision struct {
	States  []State  // parallel to the input cases
	Status  string   // "" = unchanged, else BatchStopped or BatchCompleted
	Note    string   // for a status change
	Enqueue []string // case codes to enqueue now, in order
}

// Plan decides the next step for a running batch. The order is fixed: a case that needs a
// person stops the batch, three consecutive technical failures stop it, nothing pending or
// active completes it, otherwise slots are filled in snapshot order.
func Plan(cases []Case) Decision {
	d := Decision{States: make([]State, len(cases))}
	active, pending := 0, 0
	type fin struct {
		state State
		at    time.Time
		id    string
	}
	var finished []fin
	for i, c := range cases {
		s := Classify(c)
		d.States[i] = s
		switch s {
		case Active, AwaitingApproval:
			active++
		case Pending:
			pending++
		case PR, Failed, Declined, NeedsPerson:
			finished = append(finished, fin{s, c.Job.DoneAt, c.Job.ID})
		}
	}
	for i, s := range d.States {
		if s == NeedsPerson {
			d.Status, d.Note = BatchStopped, "needs a person: "+cases[i].Code
			return d
		}
	}
	sort.SliceStable(finished, func(a, b int) bool {
		if !finished[a].at.Equal(finished[b].at) {
			return finished[a].at.After(finished[b].at)
		}
		return finished[a].id > finished[b].id
	})
	streak := 0
	for _, f := range finished {
		if f.state == Declined {
			continue // a human outcome neither counts nor resets
		}
		if f.state != Failed {
			break // a PR resets the streak
		}
		streak++
	}
	if streak >= StopAfter {
		d.Status, d.Note = BatchStopped, "3 consecutive failures"
		return d
	}
	if pending == 0 && active == 0 {
		d.Status = BatchCompleted
		return d
	}
	for i, c := range cases {
		if active >= MaxOpen {
			break
		}
		if d.States[i] == Pending {
			d.Enqueue = append(d.Enqueue, c.Code)
			active++
		}
	}
	return d
}
