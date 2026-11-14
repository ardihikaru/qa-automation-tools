// Package gaossync pushes a product's test cases to the GAOS kanban (GAOS_INTEGRATION.md).
package gaossync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
)

const (
	batchSize      = 200
	requestTimeout = 30 * time.Second
	pushBudget     = 5 * time.Minute
	sampleMax      = 20
	noteMax        = 300
)

// Errors returned by Start.
var (
	ErrNotConfigured = errors.New("GAOS integration is not configured on this server")
	ErrNotIntegrated = errors.New("product is not integrated with GAOS")
	ErrBusy          = errors.New("a push to GAOS is already running for this product")
)

// Reject is one rejected case as GAOS reported it.
type Reject struct {
	CaseID string `json:"caseId"`
	Reason string `json:"reason"`
}

// Result is the summary of one push. Status is ok, partial or failed.
type Result struct {
	Status           string    `json:"status"`
	Note             string    `json:"note"`
	Sent             int       `json:"sent"`
	Skipped          int       `json:"skipped"`
	Created          int       `json:"created"`
	Unchanged        int       `json:"unchanged"`
	ExistsDiffers    int       `json:"exists_differs"`
	Rejected         int       `json:"rejected"`
	RejectedSample   []Reject  `json:"rejected_sample"`
	ResultsApplied   int       `json:"results_applied"`
	ResultsUnchanged int       `json:"results_unchanged"`
	ResultsStale     int       `json:"results_stale"` // runs GAOS already holds a newer one for: informational
	EpicsAdded       int       `json:"epics_added"`
	CardsAdded       int       `json:"cards_added"`
	CardsDeferred    int       `json:"cards_deferred"`
	CatHead          string    `json:"cat_head"`
	Batches          int       `json:"batches"`
	StartedAt        time.Time `json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
}

// Run is one started push (possibly several passes when triggers arrived meanwhile).
type Run struct {
	Done <-chan struct{}
	done chan struct{}
	mu   sync.Mutex
	res  Result
}

// Result is the result of the last pass; call it after Done is closed.
func (r *Run) Result() Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.res
}

func (r *Run) set(res Result) {
	r.mu.Lock()
	r.res = res
	r.mu.Unlock()
}

type state struct {
	dirty bool
	run   *Run
}

// Syncer pushes products to GAOS. Safe for concurrent use.
type Syncer struct {
	db         *db.DB
	cl         *client
	configured bool

	budget time.Duration

	slot    chan struct{} // one push at a time per token
	mu      sync.Mutex
	running map[int]*state

	backoffStep time.Duration // sweeper backoff per failed attempt (backoffStepDefault)
	sweepMu     sync.Mutex    // at most one sweep at a time
	startDelay  time.Duration // sweeper delay before its first sweep
}

// New returns a Syncer; with an empty or invalid URL/token it reports Configured() false and
// never sends. An invalid value is logged by variable name only (never the value).
func New(d *db.DB, apiURL, token string) *Syncer {
	s := &Syncer{db: d, budget: pushBudget, slot: make(chan struct{}, 1), running: map[int]*state{}, backoffStep: backoffStepDefault, startDelay: sweeperStartDelay}
	base, err := ValidateConfig(apiURL, token)
	switch {
	case err != nil:
		log.Printf("gaossync: not configured: %v", err)
	case base != "":
		s.cl = &client{base: base, token: token, hc: newHTTPClient(), timeout: requestTimeout, sleep: sleepCtx}
		s.cl.hc.Transport = noProxyTransport()
		s.configured = true
	}
	return s
}

// Configured reports whether a valid GAOS_API_URL and GAOS_API_TOKEN are set.
func (s *Syncer) Configured() bool { return s != nil && s.configured && s.db != nil }

type settings struct {
	integrated bool
	productID  string
	lastStatus string
	requested  sql.NullTime // the outbox marker: gaos_push_requested_at
}

func (s *Syncer) loadSettings(ctx context.Context, productID int) (settings, error) {
	var st settings
	err := s.db.QueryRow(ctx, `SELECT gaos_integrated, gaos_product_id, gaos_last_status, gaos_push_requested_at FROM product_settings WHERE product_id = ?`, productID).
		Scan(&st.integrated, &st.productID, &st.lastStatus, &st.requested)
	if errors.Is(err, sql.ErrNoRows) {
		return settings{}, nil
	}
	return st, err
}

// Start begins a manual push in the background. It fails with ErrNotConfigured, ErrNotIntegrated
// or ErrBusy; otherwise the Run completes when the push (and any coalesced re-run) ends.
func (s *Syncer) Start(productID int) (*Run, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := s.loadSettings(ctx, productID)
	if err != nil {
		return nil, err
	}
	if !st.integrated || st.productID == "" {
		return nil, ErrNotIntegrated
	}
	run, ok := s.begin(productID)
	if !ok {
		return nil, ErrBusy
	}
	go s.loop(productID, run)
	return run, nil
}

// Trigger starts a push in the background, or asks the running one to run once more.
// It never blocks and never fails; a product that is not integrated is ignored.
func (s *Syncer) Trigger(productID int) { s.trigger(productID, false) }

// Retry is Trigger for an unchanged catalog: it pushes only when the last outcome is not ok.
func (s *Syncer) Retry(productID int) { s.trigger(productID, true) }

func (s *Syncer) trigger(productID int, onlyIfNotOK bool) {
	if !s.Configured() {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("gaossync: trigger for product %d panicked: %v", productID, r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := s.loadSettings(ctx, productID)
		if err != nil {
			log.Printf("gaossync: product %d: read settings: %v", productID, err)
			return
		}
		if !st.integrated || st.productID == "" || (onlyIfNotOK && st.lastStatus == "ok") {
			return
		}
		s.markRequested(ctx, productID)
		if run, ok := s.startOrCoalesce(productID); ok {
			s.loop(productID, run)
		}
	}()
}

// markRequested persists "a push is owed" (the outbox marker) before the in-memory push starts.
// A failure is logged (no secrets) and swallowed: the in-memory push still runs, best effort.
func (s *Syncer) markRequested(ctx context.Context, productID int) {
	if _, err := s.db.Exec(ctx, `UPDATE product_settings SET gaos_push_attempts = IF(gaos_push_requested_at IS NULL, 0, gaos_push_attempts), gaos_push_requested_at = UTC_TIMESTAMP(6) WHERE product_id = ?`, productID); err != nil {
		log.Printf("gaossync: product %d: persist push request: %v", productID, err)
	}
}

// startOrCoalesce marks a running push dirty (false) or registers a new run the caller must loop (true).
func (s *Syncer) startOrCoalesce(productID int) (*Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, busy := s.running[productID]; busy {
		cur.dirty = true
		return nil, false
	}
	run := newRun()
	s.running[productID] = &state{run: run}
	return run, true
}

func newRun() *Run {
	r := &Run{done: make(chan struct{})}
	r.Done = r.done
	return r
}

func (s *Syncer) begin(productID int) (*Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.running[productID]; busy {
		return nil, false
	}
	run := newRun()
	s.running[productID] = &state{run: run}
	return run, true
}

// loop runs passes until no trigger arrived during the last one. Clearing dirty and removing the
// entry happen under one lock hold, so a trigger can never land on a finished entry.
func (s *Syncer) loop(productID int, run *Run) {
	defer close(run.done)
	for {
		run.set(s.pass(productID))
		s.mu.Lock()
		st := s.running[productID]
		if st != nil && st.dirty {
			st.dirty = false
			s.mu.Unlock()
			continue
		}
		delete(s.running, productID)
		s.mu.Unlock()
		return
	}
}

// pass is one push with panic recovery. It returns an empty Result (Status "") when the
// product is not integrated any more and nothing was done.
func (s *Syncer) pass(productID int) (res Result) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("gaossync: product %d: push panicked: %v", productID, r)
			s.noteFailure(productID)
			res = Result{Status: "failed", Note: "internal error", FinishedAt: time.Now().UTC()}
		}
	}()
	wait := time.NewTimer(s.budget)
	defer wait.Stop()
	select {
	case s.slot <- struct{}{}:
		defer func() { <-s.slot }()
	case <-wait.C:
		s.noteFailure(productID)
		return Result{Status: "failed", Note: "time budget reached waiting for another push", FinishedAt: time.Now().UTC()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.budget)
	defer cancel()

	st, err := s.loadSettings(ctx, productID)
	if err != nil {
		log.Printf("gaossync: product %d: read settings: %v", productID, err)
		s.noteFailure(productID)
		return Result{Status: "failed", Note: "internal error", FinishedAt: time.Now().UTC()}
	}
	if !st.integrated || st.productID == "" || !ValidProductID(st.productID) {
		return Result{}
	}
	token := st.requested // the request this pass serves; a newer marker survives it
	res = Result{StartedAt: time.Now().UTC(), RejectedSample: []Reject{}}
	cat, err := s.readCatalog(ctx, productID)
	if err != nil {
		log.Printf("gaossync: product %d: read catalog: %v", productID, err)
		res.Status, res.Note = "failed", "internal error"
	} else {
		stopped := s.push(ctx, productID, st.productID, cat, &res)
		if stopped {
			return Result{} // flag or id changed mid-push: nothing is recorded
		}
	}
	res.FinishedAt = time.Now().UTC()
	// Clear the marker before the outcome is visible: once a caller sees the outcome, the push is
	// over (and a crash between the two leaves a stale outcome, never a lost push).
	if res.Status == "ok" || res.Status == "partial" {
		s.complete(productID, st.productID, token)
	}
	s.store(productID, st.productID, &res)
	return res
}

// noteFailure counts a failed pass that never reached store (slot budget, settings read error,
// panic) so the sweeper's backoff sees it: one attempt and a fresh last-attempt time, only while
// a push is owed.
func (s *Syncer) noteFailure(productID int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `UPDATE product_settings SET gaos_push_attempts = gaos_push_attempts + 1, gaos_last_sync_at = UTC_TIMESTAMP()
WHERE product_id = ? AND gaos_push_requested_at IS NOT NULL`, productID); err != nil {
		log.Printf("gaossync: product %d: count failed attempt: %v", productID, err)
	}
}

// complete clears the outbox marker of a delivered pass, only if no newer request arrived since
// the pass read it (the marker still equals, or is older than, the token).
func (s *Syncer) complete(productID int, pid string, token sql.NullTime) {
	if !token.Valid {
		return // a manual push with nothing owed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `UPDATE product_settings SET gaos_push_requested_at = NULL, gaos_push_attempts = 0
WHERE product_id = ? AND gaos_integrated = 1 AND gaos_product_id = ? AND gaos_push_requested_at <= ?`, productID, pid, token.Time); err != nil {
		log.Printf("gaossync: product %d: clear push request: %v", productID, err)
	}
}

// store writes the outcome with a fresh context, only when the flag is still on and the id is
// still the one the push used.
func (s *Syncer) store(productID int, pid string, res *Result) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A failed pass counts an attempt while a push is owed (the sweeper's backoff reads it).
	if _, err := s.db.Exec(ctx, `UPDATE product_settings SET gaos_last_sync_at = UTC_TIMESTAMP(), gaos_last_status = ?, gaos_last_note = ?,
  gaos_push_attempts = gaos_push_attempts + IF(? = 'failed' AND gaos_push_requested_at IS NOT NULL, 1, 0)
WHERE product_id = ? AND gaos_integrated = 1 AND gaos_product_id = ?`, res.Status, res.Note, res.Status, productID, pid); err != nil {
		log.Printf("gaossync: product %d: store outcome: %v", productID, err)
	}
}

// stillCurrent re-reads the flag and the id before a request.
func (s *Syncer) stillCurrent(ctx context.Context, productID int, pid string) bool {
	st, err := s.loadSettings(ctx, productID)
	if err != nil {
		return true // a read error must not turn into a silent stop; the request will report its own failure
	}
	return st.integrated && st.productID == pid
}

func (s *Syncer) push(ctx context.Context, productID int, pid string, cat *catalog, res *Result) (stopped bool) {
	plan := buildPlan(cat)
	res.Skipped = plan.skipped
	res.Sent = len(plan.cases)

	casesPath := "/products/" + pid + "/qa/cases"
	resultsPath := "/products/" + pid + "/qa/results"
	nCase := batches(len(plan.cases))
	nRes := 0
	rejectedOther := map[string]bool{} // caseIds GAOS refused for a reason other than exists-differs
	addRejects := func(list []rejection) {
		for _, x := range list {
			if x.Reason == "exists-differs" {
				res.ExistsDiffers++
				continue
			}
			if x.Reason == "stale" { // GAOS holds a newer run for the case: not a failure
				res.ResultsStale++
				continue
			}
			res.Rejected++
			if len(res.RejectedSample) < sampleMax {
				res.RejectedSample = append(res.RejectedSample, Reject{CaseID: clipRunes(x.CaseID, 80), Reason: clipRunes(x.Reason, 80)})
			}
		}
	}
	fail := func(f *Failure) {
		res.Status, res.Note = "failed", clipNote(f.Note)
		if ctx.Err() != nil {
			res.Note = clipNote("time budget reached; " + f.Note)
		}
	}

	for i := 0; i < len(plan.cases); i += batchSize {
		if !s.stillCurrent(ctx, productID, pid) {
			return true
		}
		end := min(i+batchSize, len(plan.cases))
		chunk := plan.cases[i:end]
		step := fmt.Sprintf("cases batch %d/%d", i/batchSize+1, nCase)
		body := casesBody{Source: sourceName, Epics: plan.epicsFor(chunk), Cases: chunk}
		var out casesResponse
		if f := s.cl.post(ctx, casesPath, step, body, &out); f != nil {
			fail(f)
			return false
		}
		if err := out.check(len(chunk)); err != nil {
			fail(&Failure{Note: step + ": GAOS answered 200 with an unreadable or incomplete body"})
			return false
		}
		res.Batches++
		res.Created += *out.Created
		res.Unchanged += *out.Unchanged
		res.EpicsAdded += out.EpicsAdded
		res.CardsAdded += out.CardsAdded
		// GAOS gives a reason, not a count: the epics that got no card are the difference.
		if out.CardsDeferred != "" && out.EpicsAdded > out.CardsAdded {
			res.CardsDeferred += out.EpicsAdded - out.CardsAdded
		}
		res.CatHead = rawString(out.CatHead, res.CatHead)
		addRejects(*out.Rejected)
		for _, x := range *out.Rejected {
			if x.Reason != "exists-differs" {
				rejectedOther[x.CaseID] = true
			}
		}
	}

	var items []resultItem
	for _, c := range plan.cases {
		if !rejectedOther[c.CaseID] {
			run, has := cat.results[c.CaseID]
			items = append(items, resultItem{CaseID: c.CaseID, Implemented: c.Implemented, Spec: c.Spec}.withRun(run, has))
		}
	}
	nRes = batches(len(items))
	for i := 0; i < len(items); i += batchSize {
		if !s.stillCurrent(ctx, productID, pid) {
			return true
		}
		end := min(i+batchSize, len(items))
		step := fmt.Sprintf("results batch %d/%d", i/batchSize+1, nRes)
		var out resultsResponse
		if f := s.cl.post(ctx, resultsPath, step, resultsBody{Source: sourceName, Items: items[i:end]}, &out); f != nil {
			fail(f)
			return false
		}
		if err := out.check(end - i); err != nil {
			fail(&Failure{Note: step + ": GAOS answered 200 with an unreadable or incomplete body"})
			return false
		}
		res.Batches++
		res.ResultsApplied += *out.Applied
		res.ResultsUnchanged += *out.Unchanged
		res.CatHead = rawString(out.CatHead, res.CatHead)
		addRejects(*out.Rejected)
	}

	switch {
	case res.Sent == 0:
		res.Status, res.Note = "ok", clipNote(fmt.Sprintf("nothing to send (skipped %d)", res.Skipped))
	default:
		res.Status = "ok"
		if res.Rejected > 0 {
			res.Status = "partial"
		}
		note := fmt.Sprintf("sent %d cases (created %d, unchanged %d, exists-differs %d), skipped %d, results applied %d; rejected %d",
			res.Sent, res.Created, res.Unchanged, res.ExistsDiffers, res.Skipped, res.ResultsApplied, res.Rejected)
		if res.ResultsStale > 0 {
			note += fmt.Sprintf("; %d results older than GAOS's", res.ResultsStale)
		}
		if plan.noSpec > 0 {
			note += fmt.Sprintf("; %d automated cases without a usable spec", plan.noSpec)
		}
		res.Note = clipNote(note)
	}
	return false
}

func batches(n int) int { return (n + batchSize - 1) / batchSize }

func rawString(raw json.RawMessage, prev string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return prev
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	return clipRunes(s, 80)
}
