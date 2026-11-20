package learnloop

// Statuses of findings and derived states of proposals (LEARNING_LOOP.md decisions 4 and 5).

import "strings"

// JobType is the job_runs.type of a policy proposal.
const JobType = "policy_propose"

// Finding statuses.
const (
	FindingNew       = "new"
	FindingProposed  = "proposed"
	FindingLearned   = "learned"
	FindingCovered   = "covered"
	FindingDismissed = "dismissed"
)

// FindingStatuses in display order.
var FindingStatuses = []string{FindingNew, FindingProposed, FindingLearned, FindingCovered, FindingDismissed}

// Finding kinds.
const (
	KindReviewChanges   = "review_changes"
	KindReviewRejected  = "review_rejected"
	KindRepairFixed     = "repair_fixed"
	KindRepeatedFailure = "repeated_failure"
	KindManual          = "manual"
)

// Kinds in display order.
var Kinds = []string{KindReviewChanges, KindReviewRejected, KindRepairFixed, KindRepeatedFailure, KindManual}

// KindLabel is the human name of a kind (also the title prefix of a harvested finding).
var KindLabel = map[string]string{
	KindReviewChanges:   "Reviewer asked for changes",
	KindReviewRejected:  "Reviewer rejected a spec",
	KindRepairFixed:     "A repair fixed a failing spec",
	KindRepeatedFailure: "Repeated failure",
	KindManual:          "Entered by hand",
}

// Proposal states (derived, never stored).
const (
	StateDrafting         = "drafting"
	StateAwaitingApproval = "awaiting_approval"
	StatePublishing       = "publishing"
	StateCovered          = "covered"
	StatePROpen           = "pr_open"
	StateLearned          = "learned"
	StateClosed           = "closed"
	StateRejected         = "rejected"
	StateFailed           = "failed"
)

// ProposalStates in display order.
var ProposalStates = []string{StateDrafting, StateAwaitingApproval, StatePublishing, StatePROpen, StateLearned, StateCovered, StateRejected, StateClosed, StateFailed}

// JobFacts is what the state of a proposal is derived from.
type JobFacts struct {
	Status       string // job_runs.status
	Error        string // job_runs.error
	OpenApproval bool   // a case_approvals row of the job is open
	PushReached  bool   // job_events has a push or pr step
	Decision     string // result.decision
	PRURL        string // result.pr_url (or payload.pr_url while running)
	PRState      string // result.pr_state
}

// humanDecisions are the job errors that mean a human (or the approval clock) said no.
var humanDecisions = []string{"rejected", "approval expired", "changes were requested too many times"}

// ProposalState maps a job to its proposal state (decision 4). Unknown combinations are failed.
func ProposalState(f JobFacts) string {
	switch f.Status {
	case "queued", "running":
		switch {
		case f.OpenApproval:
			return StateAwaitingApproval
		case f.PushReached:
			return StatePublishing
		}
		return StateDrafting
	case "completed":
		switch {
		case f.Decision == "covered":
			return StateCovered
		case f.PRState == "MERGED":
			return StateLearned
		case f.PRState == "CLOSED":
			return StateClosed
		case f.PRURL != "":
			return StatePROpen
		}
		return StateFailed
	case "failed":
		e := strings.TrimSpace(f.Error)
		for _, h := range humanDecisions {
			if strings.HasPrefix(e, h) {
				return StateRejected
			}
		}
	}
	return StateFailed
}
