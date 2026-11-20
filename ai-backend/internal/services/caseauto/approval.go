package caseauto

// The approval wait shared by the case_create machine and the learning loop's policy proposals
// (LEARNING_LOOP.md decision 6): one record in case_approvals, two doors (the dashboard and,
// when the product is GAOS-integrated, a GAOS Inbox item), the first answer wins, 72 h expiry.

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

// AwaitDeps are the collaborators of AwaitApproval. Zero durations take the Machine defaults.
type AwaitDeps struct {
	Approvals Approvals
	Inbox     Inbox // nil = no GAOS door
	Poll      time.Duration
	GaosPoll  time.Duration
	TTL       time.Duration
	Now       func() time.Time
	Sleep     func(ctx context.Context, d time.Duration) error
	// Event writes one job event of step approval (already redacted by the caller's sink).
	Event func(level, msg string)
}

// AwaitRequest is one approval round.
type AwaitRequest struct {
	JobID         string
	ProductID     int
	CaseCode      string // "" for a request that is not about one case (a policy proposal)
	ExternalID    string // qa-<job id>-pr[N]: the GAOS Inbox externalId too
	GaosProductID string // "" = dashboard door only
	Item          gaossync.InboxCreate
	WaitingMsg    string // the event written once the record is open
	LogPrefix     string // "caseauto" / "learnloop", for the server log
}

// ApprovalFailure is a failure of the approval step that the caller reports as a step error
// (the record could not be opened, or it vanished). A stopped wait is returned as a plain error.
type ApprovalFailure struct{ Msg string }

func (e *ApprovalFailure) Error() string { return e.Msg }

func (d AwaitDeps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d AwaitDeps) sleep(ctx context.Context, dur time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, dur)
	}
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (d AwaitDeps) event(level, msg string) {
	if d.Event != nil {
		d.Event(level, msg)
	}
}

// AwaitApproval opens the approval (idempotent on the external id), asks GAOS too when the
// product is integrated, and waits for the first answer or the deadline.
func AwaitApproval(ctx context.Context, d AwaitDeps, req AwaitRequest) (*Approval, error) {
	if d.Poll <= 0 {
		d.Poll = 2 * time.Second
	}
	if d.GaosPoll <= 0 {
		d.GaosPoll = 30 * time.Second
	}
	if d.TTL <= 0 {
		d.TTL = 72 * time.Hour
	}
	ext := req.ExternalID
	if err := d.Approvals.Open(ctx, req.JobID, req.ProductID, req.CaseCode, ext); err != nil {
		return nil, &ApprovalFailure{Msg: fmt.Sprintf("could not open the approval: %v", err)}
	}
	d.event("info", req.WaitingMsg)
	if existing, err := d.Approvals.Get(ctx, ext); err == nil && existing != nil && existing.Status != ApprovalOpen {
		return existing, nil
	}
	if req.GaosProductID != "" && d.Inbox != nil {
		item := req.Item
		item.ExternalID = ext
		item.ExpiresInHours = int(d.TTL / time.Hour)
		if _, err := d.Inbox.CreateInboxItem(ctx, req.GaosProductID, item); err != nil {
			d.event("warn", "GAOS Inbox is unavailable ("+err.Error()+"); approve in the dashboard")
		} else {
			_ = d.Approvals.MarkGaosCreated(ctx, ext)
			d.event("info", "also asked in the GAOS Inbox")
		}
	}
	deadline := d.now().Add(d.TTL)
	var lastGaos time.Time
	for {
		a, err := d.Approvals.Get(ctx, ext)
		switch {
		case err != nil:
			log.Printf("%s: job %s: reading the approval: %v", req.LogPrefix, req.JobID, err)
		case a == nil:
			return nil, &ApprovalFailure{Msg: "the approval record disappeared"}
		case a.Status != ApprovalOpen:
			return a, nil
		case d.Inbox != nil && req.GaosProductID != "" && a.GaosCreated && d.now().Sub(lastGaos) >= d.GaosPoll:
			lastGaos = d.now()
			if it, err := d.Inbox.GetInboxItem(ctx, req.GaosProductID, ext); err == nil {
				switch it.Status {
				case gaossync.InboxAnswered:
					if st, ok := DecisionFromAnswer(it.OptionLabel, string(it.Choice)); ok {
						_, _ = d.Approvals.Answer(ctx, ext, st, "gaos", Clip(it.Text, 1000))
						continue
					}
				case gaossync.InboxExpired:
					_, _ = d.Approvals.Answer(ctx, ext, ApprovalExpired, "gaos", "")
					continue
				}
			}
		}
		if d.now().After(deadline) {
			if _, err := d.Approvals.Answer(ctx, ext, ApprovalExpired, "system", ""); err != nil {
				log.Printf("%s: job %s: expiring the approval: %v", req.LogPrefix, req.JobID, err)
			}
			return &Approval{ExternalID: ext, Status: ApprovalExpired}, nil
		}
		if err := d.sleep(ctx, d.Poll); err != nil {
			return nil, fmt.Errorf("stopped while waiting for approval: %w", err)
		}
	}
}
