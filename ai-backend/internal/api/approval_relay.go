package api

// HISTORY_ANSWER.md: answer a waiting approval from /history and relay it to GAOS.
//
//	GET  /api/history/approvals?product=   every open approval with the context to decide
//	POST /api/jobs/{id}/approval           {decision, text?, via?, external_id?} (job_events.go)
//
// The relay (via = history, the item exists in GAOS, the product is integrated) calls GAOS's
// POST .../inbox/{externalId}/answer with a 15 s budget: 200 applies the answer here at once,
// 409 applies the answer someone gave first in GAOS, anything else falls back to the local
// answer with a note. The first answer of an approval wins everywhere (ApprovalStore).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseauto"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

const (
	relayTimeoutDefault     = 15 * time.Second
	reconcileTimeoutDefault = 5 * time.Second
	maxWaitingApprovals     = 200
)

// approvalRelayState holds the per-approval in-flight locks and the (test-tunable) budgets.
type approvalRelayState struct {
	locks     sync.Map // external id -> *sync.Mutex
	timeout   time.Duration
	reconcile time.Duration
}

// tryLock takes the in-flight lock of one approval; ok false = another answer is being sent.
func (s *approvalRelayState) tryLock(ext string) (unlock func(), ok bool) {
	v, _ := s.locks.LoadOrStore(ext, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, false
	}
	return func() { mu.Unlock(); s.locks.Delete(ext) }, true
}

// decisionChoice is the 1-based GAOS option index of a decision: the order both callers of
// AwaitApproval create the item with (Approve, Reject, Needs changes).
var decisionChoice = map[string]int{"approve": 1, "reject": 2, "changes": 3}

// sourceOfVia maps the request's via to the recorded source.
var sourceOfVia = map[string]string{
	"":         caseauto.SourceJobPage,
	"job-page": caseauto.SourceJobPage,
	"history":  caseauto.SourceHistory,
	"learning": caseauto.SourceLearning,
}

// gaosProductFor is the GAOS product id of a local product when it is integrated and the GAOS
// client is configured, else "".
func (r *Router) gaosProductFor(ctx context.Context, d *db.DB, pid int) string {
	if d == nil || pid <= 0 || !r.gaos.Configured() {
		return ""
	}
	var on bool
	var gp sql.NullString
	if err := d.QueryRow(ctx, `SELECT gaos_integrated, gaos_product_id FROM product_settings WHERE product_id = ?`, pid).Scan(&on, &gp); err != nil {
		return ""
	}
	if !on || !gp.Valid || !gaossync.ValidProductID(gp.String) {
		return ""
	}
	return gp.String
}

// answerActor is who answers: the stored actor ("ardi", or "ai-service as ardi" when
// impersonating) and the actor sent to GAOS (username = the effective user, display = the real
// one behind an impersonation, else the full name).
func answerActor(req *http.Request) (stored string, ga gaossync.InboxActor) {
	u, ok := auth.GetUser(req.Context())
	if !ok {
		return "", ga
	}
	ga.Username = caseauto.CleanText(u.Username, 64)
	stored = ga.Username
	if u.Impersonator != nil {
		ga.Display = caseauto.CleanText(u.Impersonator.Username+" as "+u.Username, 120)
		stored = ga.Display
	} else if fn := caseauto.CleanText(u.FullName, 120); fn != "" && fn != u.Username {
		ga.Display = fn
	}
	return caseauto.CleanText(stored, 120), ga
}

// applyGaosAnswer records an answer GAOS reported (webhook, or a 409 of the relay) with its
// source object, and writes the job event when it changed the approval. current is the
// approval's status afterwards.
func (r *Router) applyGaosAnswer(ctx context.Context, d *db.DB, ext, jobID, status, text string, detail json.RawMessage, how string) (changed bool, current string, err error) {
	store := &caseauto.ApprovalStore{DB: d}
	text = caseauto.Clip(r.caseAuto.redact.Redact(strings.TrimSpace(text)), 1000)
	actor := caseauto.ActorFromDetail(detail)
	changed, err = store.AnswerSourced(ctx, ext, status, caseauto.AnswerMeta{By: "gaos", Text: text, Source: caseauto.SourceGAOS, Actor: actor, Detail: detail})
	if err != nil {
		return false, "", err
	}
	current = status
	if !changed {
		if a, err := store.Get(ctx, ext); err == nil && a != nil {
			current = a.Status
		}
		return false, current, nil
	}
	msg := how + ": " + status
	if actor != "" {
		msg += " (by " + actor + ")"
	}
	r.addJobEvent(jobID, "info", caseauto.StepApproval, msg)
	return true, current, nil
}

// relayOutcome is what the relay decided; status 0 = fall back to the local answer.
type relayOutcome struct {
	httpStatus int
	body       map[string]any
	fallback   bool
	note       string
}

// relayAnswer sends a /history answer to GAOS and applies the result here (HISTORY_ANSWER.md
// 3.4 and 13.1-13.5). It returns fallback = true (with a note) when the local answer must be
// applied instead.
func (r *Router) relayAnswer(req *http.Request, d *db.DB, a *caseauto.Approval, jobID, gaosPID, decision, status, text string, meta caseauto.AnswerMeta, ga gaossync.InboxActor) relayOutcome {
	store := &caseauto.ApprovalStore{DB: d}
	ext := a.ExternalID
	budget := r.approvalRelay.timeout
	if budget <= 0 {
		budget = relayTimeoutDefault
	}
	base := context.WithoutCancel(req.Context()) // a closed tab must not cut the call in half
	ctx, cancel := context.WithTimeout(base, budget)
	defer cancel()
	res, err := r.gaos.AnswerInboxItem(ctx, gaosPID, ext, gaossync.InboxAnswer{Choice: decisionChoice[decision], Text: text, Actor: ga})

	var f *gaossync.Failure
	if err != nil && errors.As(err, &f) && f.Status == 0 {
		// Timeout or transport error: GAOS may have taken it. Read the item once.
		rb := r.approvalRelay.reconcile
		if rb <= 0 {
			rb = reconcileTimeoutDefault
		}
		gctx, gcancel := context.WithTimeout(base, rb)
		it, gerr := r.gaos.GetInboxItem(gctx, gaosPID, ext)
		gcancel()
		// Only a 200 is ours (GAOS v0.43.0): an answered or expired item read here is adopted.
		if gerr == nil && (it.Status == gaossync.InboxAnswered || it.Status == gaossync.InboxExpired) {
			res, err = &gaossync.InboxAnswerResult{Item: *it, Already: it.Status == gaossync.InboxAnswered, Expired: it.Status == gaossync.InboxExpired}, nil
		}
		if err != nil {
			return relayOutcome{fallback: true, note: "GAOS did not answer in time; the outcome there is unknown, so the answer was applied in the QA tools"}
		}
	}
	if err != nil {
		note := err.Error()
		if errors.As(err, &f) {
			switch f.Status {
			case http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity:
				log.Printf("approval relay: job %s: configuration error: %s", jobID, f.Note)
				note += "; configuration error"
			}
		}
		return relayOutcome{fallback: true, note: "GAOS did not take the answer (" + note + "); it was applied in the QA tools only"}
	}

	it := res.Item
	got, mapped := caseauto.DecisionFromAnswer(it.OptionLabel, string(it.Choice))
	if res.Expired {
		// 410: the item expired in GAOS; its state is adopted (the gate ends as expired).
		_, current, aerr := r.applyGaosAnswer(base, d, ext, jobID, caseauto.ApprovalExpired, "", it.Source, "the GAOS Inbox item had expired")
		if aerr != nil {
			log.Printf("approval relay: job %s: applying the GAOS expiry: %v", jobID, aerr)
		}
		if current == "" {
			current = caseauto.ApprovalExpired
		}
		return relayOutcome{httpStatus: http.StatusConflict, body: map[string]any{
			"error": "the approval expired in GAOS", "code": "expired_in_gaos", "status": current, "source": caseauto.SourceSystem,
		}}
	}
	if res.Already {
		// 409 (or a reconciled read): someone answered first; only a 200 is ours.
		if !mapped {
			return relayOutcome{fallback: true, note: "GAOS was answered in own words, which the gate cannot use; your answer was applied in the QA tools"}
		}
		_, current, aerr := r.applyGaosAnswer(base, d, ext, jobID, got, it.Text, it.Source, "answered first in the GAOS Inbox")
		if aerr != nil {
			log.Printf("approval relay: job %s: applying the GAOS answer: %v", jobID, aerr)
		}
		if current == "" {
			current = got
		}
		return relayOutcome{httpStatus: http.StatusConflict, body: map[string]any{
			"error": "already answered in GAOS", "code": "answered_in_gaos", "status": current,
			"source": caseauto.SourceGAOS, "actor": caseauto.ActorFromDetail(it.Source),
		}}
	}
	if mapped && got != status {
		log.Printf("approval relay: job %s: GAOS reported %q for a %q answer; the submitted decision is applied", jobID, got, status)
	}
	meta.Detail = it.Source
	changed, err := store.AnswerSourced(base, ext, status, meta)
	if err != nil {
		log.Printf("approval relay: job %s: %v", jobID, err)
		return relayOutcome{httpStatus: http.StatusInternalServerError, body: map[string]any{"error": "internal error"}}
	}
	ok := map[string]any{"status": status, "delivered": "gaos"}
	if changed {
		r.addJobEvent(jobID, "info", caseauto.StepApproval, "answered from the run history by "+meta.Actor+": "+decision+" (sent to GAOS)")
		return relayOutcome{httpStatus: http.StatusOK, body: ok}
	}
	cur, err := store.Get(base, ext)
	if err != nil || cur == nil {
		return relayOutcome{httpStatus: http.StatusInternalServerError, body: map[string]any{"error": "internal error"}}
	}
	if cur.Status == status && cur.Source == caseauto.SourceGAOS {
		if claimed, err := store.Claim(base, ext, status, meta); err == nil && claimed {
			r.addJobEvent(jobID, "info", caseauto.StepApproval, "the GAOS answer above came from the run history by "+meta.Actor+" (relayed)")
		}
		return relayOutcome{httpStatus: http.StatusOK, body: ok}
	}
	r.addJobEvent(jobID, "warn", caseauto.StepApproval, "GAOS recorded "+status+" from the run history by "+meta.Actor+
		", but the QA tools had already applied "+cur.Status+" via "+orDash(cur.Source)+"; GAOS shows a different answer")
	return relayOutcome{httpStatus: http.StatusConflict, body: map[string]any{
		"error": "answered in the QA tools first", "code": "answered_here", "status": cur.Status, "source": cur.Source, "actor": cur.Actor,
	}}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---------- GET /api/history/approvals ----------

type waitingReport struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type waitingApproval struct {
	ExternalID  string         `json:"external_id"`
	Round       int            `json:"round"`
	JobID       string         `json:"job_id"`
	JobType     string         `json:"job_type"`
	JobStatus   string         `json:"job_status"`
	ProductID   int            `json:"product_id"`
	ProductName string         `json:"product_name"`
	CaseCode    string         `json:"case_code"`
	Title       string         `json:"title"`
	SpecPath    string         `json:"spec_path"`
	Report      *waitingReport `json:"report"`
	GaosCreated bool           `json:"gaos_created"`
	Relay       bool           `json:"relay"`
	CreatedAt   time.Time      `json:"created_at"`
	WaitingMs   int64          `json:"waiting_ms"`
}

// approvalRound is 1 for "-pr" and N for "-prN".
func approvalRound(ext string) int {
	if m := extRoundRe.FindStringSubmatch(ext); m != nil && m[1] != "" {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

func (r *Router) historyApprovals(w http.ResponseWriter, req *http.Request, d *db.DB) {
	ctx := req.Context()
	q := `SELECT a.external_id, a.job_id, a.product_id, a.case_code, a.gaos_created, a.created_at, COALESCE(p.name, '')
		FROM case_approvals a LEFT JOIN products p ON p.id = a.product_id WHERE a.status = 'open'`
	var args []any
	if s := strings.TrimSpace(req.URL.Query().Get("product")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			writeJSONError(w, http.StatusBadRequest, "product must be a product id")
			return
		}
		q += ` AND a.product_id = ?`
		args = append(args, n)
	}
	rows, err := d.Query(ctx, q+` ORDER BY a.created_at, a.id LIMIT `+strconv.Itoa(maxWaitingApprovals), args...)
	if err != nil {
		historyInternal(w, "approvals", err)
		return
	}
	items := []*waitingApproval{}
	for rows.Next() {
		it := &waitingApproval{}
		if err := rows.Scan(&it.ExternalID, &it.JobID, &it.ProductID, &it.CaseCode, &it.GaosCreated, &it.CreatedAt, &it.ProductName); err != nil {
			rows.Close()
			historyInternal(w, "approvals scan", err)
			return
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		historyInternal(w, "approvals rows", err)
		return
	}
	now := time.Now().UTC()
	gaosOf := map[int]string{}
	for _, it := range items {
		it.Round = approvalRound(it.ExternalID)
		it.WaitingMs = now.Sub(it.CreatedAt).Milliseconds()
		if it.WaitingMs < 0 {
			it.WaitingMs = 0
		}
		gp, seen := gaosOf[it.ProductID]
		if !seen {
			gp = r.gaosProductFor(ctx, d, it.ProductID)
			gaosOf[it.ProductID] = gp
		}
		it.Relay = it.GaosCreated && gp != ""
		job, err := r.lookupJob(ctx, it.JobID)
		if err != nil || job == nil {
			continue
		}
		it.JobType, it.JobStatus = job.Type, string(job.Status)
		p := job.Payload
		it.SpecPath, _ = p["spec_path"].(string)
		if dr, ok := p["draft"]; ok {
			var x struct {
				Title string `json:"title"`
			}
			if raw, err := json.Marshal(dr); err == nil && json.Unmarshal(raw, &x) == nil {
				it.Title = x.Title
			}
		}
		if rep, ok := p["report"]; ok && rep != nil {
			var x waitingReport
			if raw, err := json.Marshal(rep); err == nil && json.Unmarshal(raw, &x) == nil {
				it.Report = &x
			}
		}
	}
	_, canAnswer := productRole(req)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "can_answer": canAnswer})
}
