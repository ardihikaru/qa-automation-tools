package learnloop

// The proposal machine (LEARNING_LOOP.md decision 3), the body of a policy_propose job:
//
//	prepare -> generate -> (covered: done) -> approval -> (changes: generate again, <= 2) -> push -> pr -> done
//
// Side effects sit behind interfaces so it is unit-tested with fakes. The approval wait is the
// one case_create uses (caseauto.AwaitApproval): one case_approvals record per round, the
// dashboard and the GAOS Inbox as doors, the first answer wins, 72 h.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

// MaxDuration bounds a proposal job: the 72 h approval plus working time.
const MaxDuration = 73 * time.Hour

// PublishRequest is what the publisher writes.
type PublishRequest struct {
	JobID     string
	Branch    string // qa-auto/learn-<job id lower>
	Slug      string // the drafted slug; the publisher picks a free one (FreeSlug)
	Content   string // the full file text
	CommitMsg string
	PRTitle   string
	BodyFor   func(filePath string) string
}

// PublishResult is what was written.
type PublishResult struct {
	FilePath string
	PRURL    string
	Warning  string // e.g. the label could not be added
}

// Publisher writes the policy file on its own branch and opens the PR. The api implementation
// passes repoWriteAllowed before push and PR. OnPushing is called before the push, so a crash
// after it is never re-run (the orphan rule).
type Publisher interface {
	Publish(ctx context.Context, req PublishRequest, onPushing func(filePath string)) (PublishResult, error)
}

// ProposalJob is one proposal.
type ProposalJob struct {
	ID            string
	ProductID     int
	ProductName   string
	GaosProductID string // "" = dashboard door only
	JobURL        string
	PageURL       string
	Findings      []FindingBrief
	Guide         string
	Rules         []RepoFile
	Branch        string
	// Resume, when set, continues a job whose approval was open (or approved) at a restart:
	// the stored draft is used as is, never regenerated.
	Resume *Resume
}

// Resume is the state a restarted proposal continues from.
type Resume struct {
	Draft      Draft
	ExternalID string // the latest approval's external id
	Round      int    // its round (0-based)
	Approved   bool   // that approval is already approved: publish
	ApprovedBy string
}

// Outcome is what a finished proposal produced.
type Outcome struct {
	Decision  string // new | covered
	CoveredBy string
	FilePath  string
	PRURL     string
	Branch    string
	Draft     Draft
}

// Proposer runs proposals. Zero tuning values take the defaults.
type Proposer struct {
	LLM       caseauto.LLM
	Approvals caseauto.Approvals
	Events    caseauto.Events
	State     caseauto.State
	Inbox     caseauto.Inbox // nil = no GAOS door
	Publisher Publisher
	Redact    *caseauto.Redactor

	MaxChangeRounds int // 2
	Poll            time.Duration
	GaosPoll        time.Duration
	TTL             time.Duration
	Now             func() time.Time
	Sleep           func(ctx context.Context, d time.Duration) error
}

// ErrStep is a failure that already has its event.
type ErrStep struct{ Msg string }

func (e *ErrStep) Error() string { return e.Msg }

func (p *Proposer) ev(ctx context.Context, job ProposalJob, level, step, msg string) {
	msg = caseauto.Clip(p.Redact.Redact(msg), 1000)
	if p.Events == nil {
		return
	}
	if err := p.Events.Add(context.WithoutCancel(ctx), job.ID, level, step, msg); err != nil {
		log.Printf("learnloop: job %s: event not stored: %v", job.ID, err)
	}
}

func (p *Proposer) fail(ctx context.Context, job ProposalJob, step, format string, a ...interface{}) error {
	msg := caseauto.Clip(p.Redact.Redact(fmt.Sprintf(format, a...)), 900)
	p.ev(ctx, job, "error", step, msg)
	return &ErrStep{Msg: msg}
}

func (p *Proposer) set(ctx context.Context, job ProposalJob, key string, v interface{}) {
	if p.State != nil {
		p.State.SetPayload(context.WithoutCancel(ctx), job.ID, key, v)
	}
}

func externalID(jobID string, round int) string {
	if round == 0 {
		return fmt.Sprintf("qa-%s-pr", jobID)
	}
	return fmt.Sprintf("qa-%s-pr%d", jobID, round+1)
}

// Run executes the proposal.
func (p *Proposer) Run(ctx context.Context, job ProposalJob) (*Outcome, error) {
	maxRounds := p.MaxChangeRounds
	if maxRounds <= 0 {
		maxRounds = 2
	}
	var d *Draft
	round := 0
	ext := ""
	if r := job.Resume; r != nil {
		dd := r.Draft
		d, round, ext = &dd, r.Round, r.ExternalID
		if r.Approved {
			p.ev(ctx, job, "info", caseauto.StepApproval, "resuming after a backend restart: approved, publishing")
			return p.publish(ctx, job, *d, r.ApprovedBy)
		}
		p.ev(ctx, job, "info", caseauto.StepApproval, "resuming the approval wait after a backend restart")
	} else {
		p.ev(ctx, job, "info", caseauto.StepPrepare, fmt.Sprintf("drafting a policy for %s from %d finding(s)", job.ProductName, len(job.Findings)))
		if len(job.Findings) == 0 {
			return nil, p.fail(ctx, job, caseauto.StepPrepare, "the proposal has no findings")
		}
	}
	changeText := ""
	var prev *Draft
	for {
		if d == nil {
			nd, err := p.draft(ctx, job, changeText, prev)
			if err != nil {
				return nil, err
			}
			d = nd
			if d.Decision == "covered" {
				p.set(ctx, job, "draft", d)
				p.ev(ctx, job, "info", caseauto.StepDone, "already covered by "+d.CoveredBy+"; no policy file is proposed")
				return &Outcome{Decision: "covered", CoveredBy: d.CoveredBy, Draft: *d}, nil
			}
			ext = externalID(job.ID, round)
		}
		content := p.render(job, *d)
		p.set(ctx, job, "draft", d)
		p.set(ctx, job, "spec_path", LearnedPath(d.Slug))
		p.set(ctx, job, "spec_content", content)
		a, err := p.await(ctx, job, *d, ext)
		if err != nil {
			return nil, err
		}
		switch a.Status {
		case caseauto.ApprovalApproved:
			return p.publish(ctx, job, *d, a.AnsweredBy)
		case caseauto.ApprovalRejected:
			return nil, p.fail(ctx, job, caseauto.StepApproval, "rejected%s", byText(a))
		case caseauto.ApprovalExpired:
			return nil, p.fail(ctx, job, caseauto.StepApproval, "approval expired")
		case caseauto.ApprovalChanges:
			if round >= maxRounds {
				return nil, p.fail(ctx, job, caseauto.StepApproval, "changes were requested too many times%s", byText(a))
			}
			p.ev(ctx, job, "info", caseauto.StepApproval, "changes requested"+byText(a))
			prev, changeText = d, a.Text
			d = nil
			round++
		default:
			return nil, p.fail(ctx, job, caseauto.StepApproval, "unexpected approval status %q", a.Status)
		}
	}
}

func byText(a *caseauto.Approval) string {
	s := ""
	if a.AnsweredBy != "" {
		s = " by " + a.AnsweredBy
	}
	if a.Text != "" {
		s += ": " + caseauto.Clip(a.Text, 300)
	}
	return s
}

// draft asks the model once, and once more with the validation error when the answer is refused.
func (p *Proposer) draft(ctx context.Context, job ProposalJob, changeText string, prev *Draft) (*Draft, error) {
	in := DraftInput{ProductName: job.ProductName, Findings: job.Findings, Guide: job.Guide, Rules: job.Rules, ChangeText: changeText, Previous: prev}
	convo := []models.Message{{Role: "user", Content: p.Redact.Redact(Prompt(in)), Kind: "prompt"}}
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		p.ev(ctx, job, "info", caseauto.StepGenerate, fmt.Sprintf("asking claude-v3 for a policy draft (attempt %d)", attempt))
		reply, err := p.LLM.ChatMessages(ctx, convo, ai.ChatOpts{Temperature: 0.2, Feature: "learning-policy", Caller: "qa-learning-loop",
			Conversation: &ai.ConvRef{Kind: ai.ConvJob, Ref: job.ID, ProductID: job.ProductID, Title: "learned policy"}})
		if err != nil {
			return nil, p.fail(ctx, job, caseauto.StepGenerate, "the LLM call failed: %v", err)
		}
		d, err := ParseDraft(p.Redact.Redact(reply))
		if err == nil {
			if d.Decision == "new" {
				p.ev(ctx, job, "info", caseauto.StepGenerate, "drafted "+LearnedPath(d.Slug)+": "+d.Title)
			}
			return d, nil
		}
		lastErr = err
		p.ev(ctx, job, "warn", caseauto.StepGenerate, "the answer was refused: "+err.Error())
		convo = append(convo,
			models.Message{Role: "assistant", Content: p.Redact.Redact(caseauto.Clip(reply, 6000)), Kind: "reply"},
			models.Message{Role: "user", Kind: "refusal", Content: "Your answer was refused: " + err.Error() + ". Answer again with exactly one JSON object in the documented form."})
	}
	return nil, p.fail(ctx, job, caseauto.StepGenerate, "the model's answer was refused twice: %v", lastErr)
}

// render is the full file text for a draft.
func (p *Proposer) render(job ProposalJob, d Draft) string {
	ids := make([]int64, 0, len(job.Findings))
	var ev []string
	for _, f := range job.Findings {
		ids = append(ids, f.ID)
		line := f.Kind
		if len(f.CaseCodes) > 0 {
			codes := f.CaseCodes
			if len(codes) > 5 {
				codes = codes[:5]
			}
			line += ": cases " + strings.Join(codes, ", ")
		}
		ev = append(ev, line+"; QA tools job "+job.ID)
	}
	return RenderFile(d, FileMeta{ProductID: job.ProductID, JobID: job.ID, FindingIDs: ids, Evidence: ev})
}

// await opens the approval round and waits for its answer. GAOS gets the whole policy.
func (p *Proposer) await(ctx context.Context, job ProposalJob, d Draft, ext string) (*caseauto.Approval, error) {
	var inbox caseauto.Inbox
	if job.GaosProductID != "" && p.Inbox != nil {
		inbox = p.Inbox
	}
	gctx := "Product: " + job.ProductName + "\nFile: " + LearnedPath(d.Slug) + "\n\n" + d.Policy
	if room := 1500 - len(gctx) - len("\n\nWhy: ") - len("\nJob: "+job.JobURL); room > 40 && d.Rationale != "" {
		gctx += "\n\nWhy: " + caseauto.Clip(d.Rationale, room)
	}
	if job.JobURL != "" {
		gctx += "\nJob: " + job.JobURL
	}
	a, err := caseauto.AwaitApproval(ctx, caseauto.AwaitDeps{
		Approvals: p.Approvals, Inbox: inbox, Poll: p.Poll, GaosPoll: p.GaosPoll, TTL: p.TTL, Now: p.Now, Sleep: p.Sleep,
		Event: func(level, msg string) { p.ev(ctx, job, level, caseauto.StepApproval, msg) },
	}, caseauto.AwaitRequest{
		JobID: job.ID, ProductID: job.ProductID, ExternalID: ext, GaosProductID: job.GaosProductID, LogPrefix: "learnloop",
		WaitingMsg: "waiting for approval of the learned policy " + LearnedPath(d.Slug) + ": " + d.Title,
		Item: gaossync.InboxCreate{
			Title:    caseauto.Clip(p.Redact.Redact("Approve learned policy for "+job.ProductName), 120),
			Question: caseauto.Clip(p.Redact.Redact("QA tools learned a policy for the automation repo: \""+d.Title+"\". Open a pull request that adds it?"), 500),
			Context:  caseauto.Clip(p.Redact.Redact(gctx), 1500),
			Options: []gaossync.InboxOption{
				{Label: "Approve", Description: "Commit the policy file, push the branch and open the pull request"},
				{Label: "Reject", Description: "Discard the policy; its findings return to the list"},
				{Label: "Needs changes", Description: "Reply with what to change; the policy is redrafted"},
			},
		},
	})
	var af *caseauto.ApprovalFailure
	if errors.As(err, &af) {
		return nil, p.fail(ctx, job, caseauto.StepApproval, "%s", af.Msg)
	}
	return a, err
}

// publish writes the file on its branch and opens the PR.
func (p *Proposer) publish(ctx context.Context, job ProposalJob, d Draft, approvedBy string) (*Outcome, error) {
	if p.Publisher == nil {
		return nil, p.fail(ctx, job, caseauto.StepPush, "publishing is not configured (repo pool)")
	}
	content := p.render(job, d)
	var findingLines []string
	for _, f := range job.Findings {
		findingLines = append(findingLines, fmt.Sprintf("%s (finding #%d)", f.Title, f.ID))
	}
	title := PRTitle(d)
	req := PublishRequest{
		JobID: job.ID, Branch: job.Branch, Slug: d.Slug, Content: content, CommitMsg: title, PRTitle: title,
		BodyFor: func(path string) string {
			return p.Redact.Redact(PRBody(PRInput{ProductName: job.ProductName, JobID: job.ID, FilePath: path, Draft: d, ApprovedBy: approvedBy, Findings: findingLines, PageURL: job.PageURL}))
		},
	}
	p.ev(ctx, job, "info", caseauto.StepWrite, "approved"+map[bool]string{true: " by " + approvedBy, false: ""}[approvedBy != ""]+"; writing the policy file on "+job.Branch)
	res, err := p.Publisher.Publish(ctx, req, func(path string) {
		p.set(ctx, job, "spec_path", path)
		p.set(ctx, job, "branch", job.Branch)
		p.ev(ctx, job, "info", caseauto.StepPush, "pushing "+job.Branch+" with "+path)
	})
	if err != nil {
		return nil, p.fail(ctx, job, caseauto.StepPush, "publishing failed: %v", err)
	}
	if res.Warning != "" {
		p.ev(ctx, job, "warn", caseauto.StepPR, res.Warning)
	}
	p.set(ctx, job, "pr_url", res.PRURL)
	p.ev(ctx, job, "info", caseauto.StepDone, "pull request opened: "+res.PRURL)
	return &Outcome{Decision: "new", FilePath: res.FilePath, PRURL: res.PRURL, Branch: job.Branch, Draft: d}, nil
}

// IsStepErr reports whether err already has its event in the log.
func IsStepErr(err error) bool {
	var s *ErrStep
	return errors.As(err, &s)
}
