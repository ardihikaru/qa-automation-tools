// Package caseauto is the "create automation" state machine of CASE_AUTOMATION.md:
//
//	prepare -> generate -> write -> run -> (repair <= 3) -> approval -> push -> pr -> done
//
// Everything with a side effect is behind an interface (repo, LLM, executor, approvals, events,
// GAOS Inbox) so the machine is unit-tested with fakes. The only LLM path is ai.Service
// (ai-manager, claude-v3). Every event, log line and LLM-bound text goes through Redactor.
package caseauto

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

// Steps of the event log (job_events.step).
const (
	StepPrepare  = "prepare"
	StepGenerate = "generate"
	StepWrite    = "write"
	StepRun      = "run"
	StepRepair   = "repair"
	StepApproval = "approval"
	StepPush     = "push"
	StepPR       = "pr"
	StepDone     = "done"
)

// Approval statuses (case_approvals.status).
const (
	ApprovalOpen     = "open"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalChanges  = "changes"
	ApprovalExpired  = "expired"
)

// Case is what the LLM is told about the test case.
type Case struct {
	Code      string
	Scenario  string
	StoryCode string
	StoryText string
	Features  []string
}

// Job is one create-automation run.
type Job struct {
	ID            string
	ProductID     int
	ProductName   string
	Case          Case
	Branch        string   // qa-auto/<code-lower>
	DefaultBranch string   // the PR base
	AllowedHosts  []string // hosts a literal URL in the spec may use (the staging host)
	JobURL        string   // link to the job page, for the PR body
	GaosProductID string   // "" = the product is not GAOS-integrated: dashboard door only
	SpecPath      string   // set on resume: the file already written
}

// Workspace identifies the prepared worktree to the adapters.
type Workspace struct {
	Dir    string // backend path of the worktree
	Branch string
	Base   string
}

// RepoOps is the git side. The api adapter guards Push and OpenPR with repoWriteAllowed.
type RepoOps interface {
	Prepare(ctx context.Context, job Job) (Workspace, error)
	Open(ctx context.Context, job Job) (Workspace, error)
	ReadFile(ws Workspace, rel string) ([]byte, error)
	ListSpecs(ws Workspace) ([]string, error)
	WriteFile(ws Workspace, rel string, data []byte) error
	DiffStat(ctx context.Context, ws Workspace, rel string) (string, error)
	Commit(ctx context.Context, ws Workspace, message, rel string) error
	Push(ctx context.Context, ws Workspace) error
	OpenPR(ctx context.Context, ws Workspace, title, body string) (string, error)
}

// LLM is satisfied by *ai.Service: the only way this package talks to a model.
type LLM interface {
	ChatMessages(ctx context.Context, messages []models.Message, opts ai.ChatOpts) (string, error)
}

// RunRequest asks the executor to run the case's spec in a workspace.
type RunRequest struct {
	Workspace Workspace
	CaseCode  string
	SpecPath  string
	Repair    int
}

// RunResult is the verdict of one run.
type RunResult struct {
	Pass    bool
	Message string
	Tail    string
	Passed  int
	Failed  int
	Skipped int
}

// Executor runs a spec (case_run on the worktree). emit forwards the executor's own events.
type Executor interface {
	RunCase(ctx context.Context, req RunRequest, emit func(level, step, msg string)) (RunResult, error)
}

// Approval is one row of case_approvals.
type Approval struct {
	ExternalID  string
	Status      string
	AnsweredBy  string
	Text        string
	GaosCreated bool
}

// Approvals is the approval record shared by the two doors.
type Approvals interface {
	// Open creates the open approval; it is idempotent on externalID.
	Open(ctx context.Context, jobID string, productID int, caseCode, externalID string) error
	Get(ctx context.Context, externalID string) (*Approval, error)
	MarkGaosCreated(ctx context.Context, externalID string) error
	// Answer sets the status only while it is open; it reports whether this call did.
	Answer(ctx context.Context, externalID, status, by, text string) (bool, error)
}

// Events is the job_events sink.
type Events interface {
	Add(ctx context.Context, jobID, level, step, message string) error
}

// State persists facts of the job (payload keys: spec_path, spec_content, report, ...).
type State interface {
	SetPayload(ctx context.Context, jobID, key string, value interface{})
}

// Inbox is the GAOS Inbox client (*gaossync.Syncer).
type Inbox interface {
	CreateInboxItem(ctx context.Context, gaosPID string, req gaossync.InboxCreate) (*gaossync.InboxCreated, error)
	GetInboxItem(ctx context.Context, gaosPID, externalID string) (*gaossync.InboxItem, error)
}

// Outcome is what a finished job produced.
type Outcome struct {
	PRURL    string
	SpecPath string
	Repairs  int
}

// Machine runs create jobs. Zero values of the tuning fields take the defaults of NewMachine.
type Machine struct {
	Repo      RepoOps
	LLM       LLM
	Exec      Executor
	Approvals Approvals
	Events    Events
	State     State
	Inbox     Inbox // nil = no GAOS door
	Redact    *Redactor

	MaxRepairs      int           // repair rounds after the first generation (3)
	MaxChangeRounds int           // "needs changes" rounds the human may ask (2)
	Poll            time.Duration // approval record poll (2 s)
	GaosPoll        time.Duration // GAOS Inbox poll fallback (30 s)
	ApprovalTTL     time.Duration // 72 h
	Now             func() time.Time
	Sleep           func(ctx context.Context, d time.Duration) error
}

// NewMachine returns a Machine with the production defaults.
func NewMachine(repo RepoOps, llm LLM, exec Executor, ap Approvals, ev Events, st State, inbox Inbox, red *Redactor) *Machine {
	return &Machine{Repo: repo, LLM: llm, Exec: exec, Approvals: ap, Events: ev, State: st, Inbox: inbox, Redact: red,
		MaxRepairs: 3, MaxChangeRounds: 2, Poll: 2 * time.Second, GaosPoll: 30 * time.Second, ApprovalTTL: 72 * time.Hour}
}

func (m *Machine) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Machine) sleep(ctx context.Context, d time.Duration) error {
	if m.Sleep != nil {
		return m.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ev writes one redacted, clipped event. A failing store never stops the job.
func (m *Machine) ev(ctx context.Context, job Job, level, step, msg string) {
	msg = Clip(m.Redact.Redact(msg), 1000)
	if m.Events == nil {
		return
	}
	if err := m.Events.Add(context.WithoutCancel(ctx), job.ID, level, step, msg); err != nil {
		log.Printf("caseauto: job %s: event not stored: %v", job.ID, err)
	}
}

func (m *Machine) setPayload(ctx context.Context, job Job, key string, v interface{}) {
	if m.State != nil {
		m.State.SetPayload(context.WithoutCancel(ctx), job.ID, key, v)
	}
}

// stepErr is a failure that already has its event.
type stepErr struct{ msg string }

func (e *stepErr) Error() string { return e.msg }

func (m *Machine) fail(ctx context.Context, job Job, step, format string, a ...interface{}) error {
	msg := Clip(m.Redact.Redact(fmt.Sprintf(format, a...)), 900)
	m.ev(ctx, job, "error", step, msg)
	return &stepErr{msg: msg}
}

// run is the state of one Run call.
type run struct {
	job    Job
	ws     Workspace
	convo  []models.Message
	file   GeneratedFile
	result RunResult
	opts   ValidateOptions
	runs   int // executor runs so far (the "repair" param)
}

// Run executes the whole machine for a new job.
func (m *Machine) Run(ctx context.Context, job Job) (*Outcome, error) {
	if !CaseCodeRe.MatchString(job.Case.Code) {
		return nil, m.fail(ctx, job, StepPrepare, "invalid case code")
	}
	m.ev(ctx, job, "info", StepPrepare, "preparing the worktree on branch "+job.Branch)
	ws, err := m.Repo.Prepare(ctx, job)
	if err != nil {
		return nil, m.fail(ctx, job, StepPrepare, "could not prepare the worktree: %v", err)
	}
	r := &run{job: job, ws: ws}
	if err := m.initialConvo(ctx, r); err != nil {
		return nil, err
	}
	if err := m.build(ctx, r, StepGenerate); err != nil {
		return nil, err
	}
	return m.finish(ctx, r)
}

// Resume continues a job whose approval was open when the backend restarted: it skips to the
// approval step using the file already written in the worktree.
func (m *Machine) Resume(ctx context.Context, job Job) (*Outcome, error) {
	if job.SpecPath == "" || !PathRe(job.Case.Code).MatchString(job.SpecPath) {
		return nil, m.fail(ctx, job, StepApproval, "cannot resume: the job has no valid spec path")
	}
	ws, err := m.Repo.Open(ctx, job)
	if err != nil {
		return nil, m.fail(ctx, job, StepApproval, "cannot resume: %v", err)
	}
	data, err := m.Repo.ReadFile(ws, job.SpecPath)
	if err != nil {
		return nil, m.fail(ctx, job, StepApproval, "cannot resume: the spec file is gone")
	}
	r := &run{job: job, ws: ws, file: GeneratedFile{Path: job.SpecPath, Content: string(data)}, result: RunResult{Pass: true, Message: "passed before the restart"}}
	m.ev(ctx, job, "info", StepApproval, "resuming after a backend restart")
	return m.finish(ctx, r)
}

// finish runs approval (and change rounds), then push and PR.
func (m *Machine) finish(ctx context.Context, r *run) (*Outcome, error) {
	job := r.job
	for round := 0; ; round++ {
		ext := fmt.Sprintf("qa-%s-pr", job.ID)
		if round > 0 {
			ext = fmt.Sprintf("qa-%s-pr%d", job.ID, round+1)
		}
		ap, err := m.approve(ctx, r, ext)
		if err != nil {
			return nil, err
		}
		switch ap.Status {
		case ApprovalApproved:
			return m.publish(ctx, r, ap)
		case ApprovalRejected:
			return nil, m.fail(ctx, job, StepApproval, "rejected%s", byText(ap))
		case ApprovalExpired:
			return nil, m.fail(ctx, job, StepApproval, "approval expired")
		case ApprovalChanges:
			if round >= m.MaxChangeRounds {
				return nil, m.fail(ctx, job, StepApproval, "changes were requested too many times%s", byText(ap))
			}
			m.ev(ctx, job, "info", StepApproval, "changes requested"+byText(ap))
			if r.convo == nil {
				if err := m.initialConvo(ctx, r); err != nil {
					return nil, err
				}
				r.convo = append(r.convo, models.Message{Role: "assistant", Content: render(r.file)})
			}
			r.convo = append(r.convo, models.Message{Role: "user", Content: m.Redact.Redact("A reviewer asked for changes:\n" + Clip(ap.Text, 1000) +
				"\n\nReturn the complete corrected file in the same format.")})
			if err := m.build(ctx, r, StepRepair); err != nil {
				return nil, err
			}
		default:
			return nil, m.fail(ctx, job, StepApproval, "unexpected approval status %q", ap.Status)
		}
	}
}

func byText(a *Approval) string {
	s := ""
	if a.AnsweredBy != "" {
		s = " by " + a.AnsweredBy
	}
	if a.Text != "" {
		s += ": " + Clip(a.Text, 300)
	}
	return s
}

func render(f GeneratedFile) string {
	return "FILE: " + f.Path + "\n```ts\n" + strings.TrimRight(f.Content, "\n") + "\n```"
}

const systemRules = `You write ONE Playwright test file (TypeScript, @playwright/test) that automates a single manual test case of SUT, a hospital information system.
Rules:
- Reply with exactly one line "FILE: <path>" followed by exactly one fenced code block holding the whole file. No other code blocks.
- <path> must be tests/web/<module>/<CASE_CODE>.spec.ts (browser UI) or tests/api/rest/<module>/<CASE_CODE>.spec.ts (REST), <module> lower case.
- Every test(...) title must start with the case code, followed by a space and a short description. No test.only/skip/fixme.
- Follow the repo guide and copy the helper imports, fixtures and style of the example specs. Do not invent helpers that the examples do not show.
- Never hard-code URLs, hosts or credentials: use what the examples use (config, fixtures, process.env.NAME for a single named variable). Never print, log or iterate process.env.
- No child processes, no eval, no dynamic import or require, no file-system access.
- At most 20 KiB.`

// initialConvo reads the repo guide and two style examples and builds the first prompt.
func (m *Machine) initialConvo(ctx context.Context, r *run) error {
	job := r.job
	var guide string
	if b, err := m.Repo.ReadFile(r.ws, "AGENTS.md"); err == nil {
		guide = Clip(string(b), 12000)
	} else {
		m.ev(ctx, job, "warn", StepGenerate, "no AGENTS.md in the repo; continuing without it")
	}
	specs, err := m.Repo.ListSpecs(r.ws)
	if err != nil {
		return m.fail(ctx, job, StepGenerate, "could not list the repo's specs: %v", err)
	}
	var examples []string
	r.opts = ValidateOptions{Code: job.Case.Code, AllowedHosts: job.AllowedHosts, AllowedImports: map[string]bool{}}
	for _, p := range pickExamples(specs, job.Case.Code, 2) {
		b, err := m.Repo.ReadFile(r.ws, p)
		if err != nil {
			continue
		}
		src := Clip(string(b), 6000)
		for _, imp := range ImportSpecs(src) {
			if !strings.HasPrefix(imp, ".") {
				r.opts.AllowedImports[imp] = true
			}
		}
		examples = append(examples, "// "+p+"\n"+src)
	}
	if len(examples) == 0 {
		m.ev(ctx, job, "warn", StepGenerate, "the repo has no example specs to copy from")
	}
	// The repo's fixture declarations: the only names a spec may destructure. Without them the
	// generator guesses (e.g. hisVendorBackendMasterDataRest) and the first run dies at load time.
	var fixtureSrc []string
	for _, fp := range []string{"src/fixtures/api.fixtures.ts", "src/fixtures/web.fixtures.ts"} {
		if b, err := m.Repo.ReadFile(r.ws, fp); err == nil {
			fixtureSrc = append(fixtureSrc, string(b))
		}
	}
	r.opts.Fixtures = FixtureNames(fixtureSrc...)
	c := job.Case
	var sb strings.Builder
	sb.WriteString(systemRules)
	fmt.Fprintf(&sb, "\n\nCASE_CODE: %s\nSCENARIO:\n%s\n", c.Code, Clip(c.Scenario, 4000))
	if c.StoryCode != "" || c.StoryText != "" {
		fmt.Fprintf(&sb, "USER STORY %s: %s\n", c.StoryCode, Clip(c.StoryText, 1500))
	}
	if len(c.Features) > 0 {
		fmt.Fprintf(&sb, "FEATURES: %s\n", strings.Join(c.Features, ", "))
	}
	if guide != "" {
		sb.WriteString("\nREPO GUIDE (AGENTS.md):\n" + guide + "\n")
	}
	if len(r.opts.Fixtures) > 0 {
		names := make([]string, 0, len(r.opts.Fixtures))
		for n := range r.opts.Fixtures {
			names = append(names, n)
		}
		sort.Strings(names)
		sb.WriteString("\nAVAILABLE FIXTURES (a test callback may destructure ONLY these, plus page, request, context, browser, baseURL; never invent a fixture or a client that is not shown): " + strings.Join(names, ", ") + "\n")
		for _, src := range fixtureSrc {
			sb.WriteString("\nFIXTURE FILE (clipped):\n```ts\n" + Clip(src, 5000) + "\n```\n")
		}
	}
	for _, e := range examples {
		sb.WriteString("\nEXAMPLE SPEC:\n```ts\n" + e + "\n```\n")
	}
	if len(job.AllowedHosts) > 0 {
		sb.WriteString("\nThe only host a literal URL may name is: " + strings.Join(job.AllowedHosts, ", ") + "\n")
	}
	r.convo = []models.Message{{Role: "user", Content: m.Redact.Redact(sb.String())}}
	return nil
}

// pickExamples prefers specs of the same module (the code's middle token), else the first ones.
func pickExamples(specs []string, code string, n int) []string {
	sorted := append([]string(nil), specs...)
	sort.Strings(sorted)
	tok := strings.ToLower(strings.Split(code, "-")[1])
	var same, rest []string
	for _, s := range sorted {
		if strings.Contains(s, "/"+tok+"/") {
			same = append(same, s)
		} else {
			rest = append(rest, s)
		}
	}
	out := append(same, rest...)
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// build runs the generate/write/run loop until the spec passes or the repair budget is spent.
// first is StepGenerate for a new job or StepRepair after a reviewer asked for changes.
func (m *Machine) build(ctx context.Context, r *run, first string) error {
	job := r.job
	step := first
	for i := 0; ; i++ {
		if i > m.MaxRepairs {
			return m.fail(ctx, job, StepRepair, "the spec still fails after %d repairs: %s", m.MaxRepairs, r.result.Message)
		}
		if i > 0 {
			step = StepRepair
		}
		m.ev(ctx, job, "info", step, fmt.Sprintf("asking claude-v3 (%s, round %d)", step, i+1))
		reply, err := m.LLM.ChatMessages(ctx, r.convo, ai.ChatOpts{Temperature: 0.2, Feature: "case-automation", Caller: "qa-case-automation"})
		if err != nil {
			return m.fail(ctx, job, step, "the LLM call failed: %v", err)
		}
		r.convo = append(r.convo, models.Message{Role: "assistant", Content: m.Redact.Redact(reply)})
		f, err := ParseReply(reply)
		if err == nil {
			err = Validate(f, r.opts)
		}
		if err != nil {
			m.ev(ctx, job, "warn", step, "the reply was refused: "+err.Error())
			r.convo = append(r.convo, models.Message{Role: "user", Content: "Your reply was refused: " + err.Error() + ". Return the complete corrected file in the required format."})
			r.result = RunResult{Message: "invalid reply: " + err.Error()}
			continue
		}
		m.ev(ctx, job, "info", StepWrite, "writing "+f.Path)
		if err := m.Repo.WriteFile(r.ws, f.Path, []byte(f.Content)); err != nil {
			return m.fail(ctx, job, StepWrite, "could not write the spec: %v", err)
		}
		r.file = f
		m.setPayload(ctx, job, "spec_path", f.Path)
		m.setPayload(ctx, job, "spec_content", m.Redact.Redact(f.Content))
		m.setPayload(ctx, job, "spec_paths", []string{f.Path})
		m.ev(ctx, job, "info", StepRun, "running "+f.Path)
		res, err := m.Exec.RunCase(ctx, RunRequest{Workspace: r.ws, CaseCode: job.Case.Code, SpecPath: f.Path, Repair: r.runs}, func(level, st, msg string) {
			if st == "" {
				st = StepRun
			}
			m.ev(ctx, job, level, st, msg)
		})
		r.runs++
		if err != nil {
			return m.fail(ctx, job, StepRun, "the executor run failed: %v", err)
		}
		r.result = res
		m.setPayload(ctx, job, "report", map[string]int{"passed": res.Passed, "failed": res.Failed, "skipped": res.Skipped})
		if res.Pass {
			m.ev(ctx, job, "info", StepRun, "the spec passed")
			return nil
		}
		m.ev(ctx, job, "warn", StepRun, "the spec did not pass: "+res.Message)
		if why := envFailure(res.Tail); why != "" {
			// A config/environment failure is not the spec's fault: another LLM round cannot fix it.
			return m.fail(ctx, job, StepRun, "the run failed before any test was collected (%s); fix the automation repo's environment (env file on the executor host), then retry", why)
		}
		r.convo = append(r.convo, models.Message{Role: "user", Content: m.Redact.Redact("The spec was run against staging and did not pass: " + res.Message +
			"\nOutput tail:\n" + Clip(res.Tail, 3500) + "\n\nFix the spec. Return the complete corrected file in the required format.")})
	}
}

// envFailure recognises output of a run that died while loading the automation repo's
// configuration (missing/invalid env values), as opposed to a failing test. "" = not one.
func envFailure(tail string) string {
	for _, marker := range []string{"Invalid configuration for TEST_ENV", "Expected values in env/.env.", "Cannot find module", "ERR_MODULE_NOT_FOUND"} {
		if strings.Contains(tail, marker) {
			return marker
		}
	}
	return ""
}

// approve opens the approval, asks GAOS too when integrated, and waits for the first answer.
func (m *Machine) approve(ctx context.Context, r *run, ext string) (*Approval, error) {
	job := r.job
	stat, err := m.Repo.DiffStat(ctx, r.ws, r.file.Path)
	if err != nil {
		stat = "(diff stat unavailable)"
	}
	if err := m.Approvals.Open(ctx, job.ID, job.ProductID, job.Case.Code, ext); err != nil {
		return nil, m.fail(ctx, job, StepApproval, "could not open the approval: %v", err)
	}
	m.ev(ctx, job, "info", StepApproval, "waiting for approval of "+r.file.Path+"\n"+Clip(stat, 600))
	if existing, err := m.Approvals.Get(ctx, ext); err == nil && existing != nil && existing.Status != ApprovalOpen {
		return existing, nil
	}
	if job.GaosProductID != "" && m.Inbox != nil {
		req := gaossync.InboxCreate{
			ExternalID: ext,
			Title:      Clip(m.Redact.Redact(fmt.Sprintf("Approve automation PR for %s", job.Case.Code)), 120),
			Question:   Clip(m.Redact.Redact(fmt.Sprintf("QA tools generated and ran an automation spec for %s (%s). Open a pull request on the automation repo?", job.Case.Code, r.result.Message)), 500),
			Context:    Clip(m.Redact.Redact(fmt.Sprintf("Product: %s\nSpec: %s\n%s\nJob: %s", job.ProductName, r.file.Path, stat, job.JobURL)), 1500),
			Options: []gaossync.InboxOption{
				{Label: "Approve", Description: "Commit, push the branch and open the pull request"},
				{Label: "Reject", Description: "Discard the generated spec"},
				{Label: "Needs changes", Description: "Reply with what to change; the spec is regenerated"},
			},
			ExpiresInHours: int(m.ApprovalTTL / time.Hour),
		}
		if _, err := m.Inbox.CreateInboxItem(ctx, job.GaosProductID, req); err != nil {
			m.ev(ctx, job, "warn", StepApproval, "GAOS Inbox is unavailable ("+err.Error()+"); approve in the dashboard")
		} else {
			_ = m.Approvals.MarkGaosCreated(ctx, ext)
			m.ev(ctx, job, "info", StepApproval, "also asked in the GAOS Inbox")
		}
	}
	deadline := m.now().Add(m.ApprovalTTL)
	var lastGaos time.Time
	for {
		a, err := m.Approvals.Get(ctx, ext)
		switch {
		case err != nil:
			log.Printf("caseauto: job %s: reading the approval: %v", job.ID, err)
		case a == nil:
			return nil, m.fail(ctx, job, StepApproval, "the approval record disappeared")
		case a.Status != ApprovalOpen:
			return a, nil
		case m.Inbox != nil && job.GaosProductID != "" && a.GaosCreated && m.now().Sub(lastGaos) >= m.GaosPoll:
			lastGaos = m.now()
			if it, err := m.Inbox.GetInboxItem(ctx, job.GaosProductID, ext); err == nil {
				switch it.Status {
				case gaossync.InboxAnswered:
					if st, ok := DecisionFromAnswer(it.OptionLabel, string(it.Choice)); ok {
						_, _ = m.Approvals.Answer(ctx, ext, st, "gaos", Clip(it.Text, 1000))
						continue
					}
				case gaossync.InboxExpired:
					_, _ = m.Approvals.Answer(ctx, ext, ApprovalExpired, "gaos", "")
					continue
				}
			}
		}
		if m.now().After(deadline) {
			if _, err := m.Approvals.Answer(ctx, ext, ApprovalExpired, "system", ""); err != nil {
				log.Printf("caseauto: job %s: expiring the approval: %v", job.ID, err)
			}
			return &Approval{ExternalID: ext, Status: ApprovalExpired}, nil
		}
		if err := m.sleep(ctx, m.Poll); err != nil {
			return nil, fmt.Errorf("stopped while waiting for approval: %w", err)
		}
	}
}

// DecisionFromAnswer maps a GAOS Inbox answer (the option label, else the choice) to an approval
// status. ok is false for an answer that is none of the three options.
func DecisionFromAnswer(optionLabel, choice string) (string, bool) {
	for _, s := range []string{optionLabel, choice} {
		l := strings.ToLower(strings.Trim(strings.TrimSpace(s), `"`))
		switch {
		case l == "":
		case strings.Contains(l, "approve"):
			return ApprovalApproved, true
		case strings.Contains(l, "reject"):
			return ApprovalRejected, true
		case strings.Contains(l, "change"):
			return ApprovalChanges, true
		}
	}
	return "", false
}

// publish commits, pushes and opens the pull request after an approval.
func (m *Machine) publish(ctx context.Context, r *run, ap *Approval) (*Outcome, error) {
	job := r.job
	title := firstLine(job.Case.Scenario)
	if title == "" {
		title = job.Case.Code
	}
	title = Clip(m.Redact.Redact(title), 80)
	msg := fmt.Sprintf("test(%s): automate %s", job.Case.Code, title)
	m.ev(ctx, job, "info", StepPush, "approved"+byText(ap)+"; committing and pushing "+job.Branch)
	if err := m.Repo.Commit(ctx, r.ws, msg, r.file.Path); err != nil {
		return nil, m.fail(ctx, job, StepPush, "commit failed: %v", err)
	}
	if err := m.Repo.Push(ctx, r.ws); err != nil {
		return nil, m.fail(ctx, job, StepPush, "push failed: %v", err)
	}
	m.ev(ctx, job, "info", StepPR, "opening the pull request")
	body := m.Redact.Redact(fmt.Sprintf("Automates **%s** (product %s).\n\n%s\n\nRun on staging: %s (passed %d, failed %d, skipped %d).\nApproved%s.\nJob: %s\n\nGenerated by the QA automation tools; reviewed by a human before this PR was opened.",
		job.Case.Code, job.ProductName, Clip(job.Case.Scenario, 1500), r.result.Message, r.result.Passed, r.result.Failed, r.result.Skipped, byText(ap), job.JobURL))
	url, err := m.Repo.OpenPR(ctx, r.ws, msg, body)
	if err != nil {
		return nil, m.fail(ctx, job, StepPR, "opening the pull request failed: %v", err)
	}
	m.setPayload(ctx, job, "pr_url", url)
	m.ev(ctx, job, "info", StepDone, "pull request opened: "+url)
	return &Outcome{PRURL: url, SpecPath: r.file.Path, Repairs: r.runs - 1}, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// IsStepErr reports whether err already has its event in the log.
func IsStepErr(err error) bool {
	var s *stepErr
	return errors.As(err, &s)
}
