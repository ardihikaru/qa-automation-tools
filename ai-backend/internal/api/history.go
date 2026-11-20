package api

// RUN_HISTORY.md: read-only run history for every signed-in user.
//
//	GET /api/history/builds     case_create / case_run jobs with queue, machine and waiting-for-approval time
//	GET /api/history/runs       every Playwright report received (pw_runs)
//	GET /api/history/runs/{id}  one run with its per-case rows
//	GET /api/history/syncs      knowledge syncs with their duration
//
// Builds are derived from job_runs, job_events and case_approvals (nothing is stored for them).
// job_events and case_approvals use the server's default collation, job_runs utf8mb4_general_ci:
// they are read with "job_id IN (?)" or compared with an explicit COLLATE, never joined bare.

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/services/runhistory"
)

var (
	historyBatchRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	historyJobRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// historyQuery collects a list's WHERE clauses and their arguments.
type historyQuery struct {
	page, size int
	where      []string
	args       []interface{}
}

func (q *historyQuery) add(clause string, args ...interface{}) {
	q.where = append(q.where, clause)
	q.args = append(q.args, args...)
}

func (q *historyQuery) sql() string {
	if len(q.where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(q.where, " AND ")
}

// parseHistoryCommon reads page, page_size, product, from and to. col is the time column of the
// date filter and productExpr the product id expression. A message means 400.
func parseHistoryCommon(req *http.Request, col, productExpr string) (*historyQuery, int, string) {
	v := req.URL.Query()
	q := &historyQuery{}
	q.page, q.size = parsePagination(v.Get("page_size"), v.Get("page"), 25)
	pid := 0
	if s := strings.TrimSpace(v.Get("product")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return nil, 0, "product must be a product id"
		}
		pid = n
		q.add(productExpr+" = ?", n)
	}
	var from, to time.Time
	for _, p := range []struct {
		key string
		dst *time.Time
	}{{"from", &from}, {"to", &to}} {
		s := strings.TrimSpace(v.Get(p.key))
		if s == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, 0, p.key + " must be a date (YYYY-MM-DD)"
		}
		*p.dst = t
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return nil, 0, "from must not be after to"
	}
	if !from.IsZero() {
		q.add(col+" >= ?", from.Format("2006-01-02 15:04:05"))
	}
	if !to.IsZero() {
		q.add(col+" < ?", to.AddDate(0, 0, 1).Format("2006-01-02 15:04:05"))
	}
	return q, pid, ""
}

// codeParam validates an optional case/epic code parameter.
func codeParam(req *http.Request, key string) (string, bool) {
	s := strings.TrimSpace(req.URL.Query().Get(key))
	if s == "" {
		return "", true
	}
	return s, knowledge.CodeRe.MatchString(s)
}

func oneOf(s string, allowed ...string) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

func (r *Router) handleHistory(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/history"), "/")
	switch {
	case rest == "builds":
		r.historyBuilds(w, req, d)
	case rest == "runs":
		r.historyRuns(w, req, d)
	case strings.HasPrefix(rest, "runs/"):
		id, err := strconv.ParseInt(strings.TrimPrefix(rest, "runs/"), 10, 64)
		if err != nil || id <= 0 {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		r.historyRun(w, req, d, id)
	case rest == "syncs":
		r.historySyncs(w, req, d)
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func historyInternal(w http.ResponseWriter, what string, err error) {
	log.Printf("history: %s: %v", what, err)
	writeJSONError(w, http.StatusInternalServerError, "internal error")
}

func inPlaceholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func msPtr(v int64) *int64 { return &v }

// ---------- builds ----------

const (
	jobPID    = `CAST(JSON_EXTRACT(j.payload, '$.product_id') AS UNSIGNED)`
	jobCase   = `JSON_UNQUOTE(JSON_EXTRACT(j.payload, '$.case_code'))`
	jobBatch  = `JSON_UNQUOTE(JSON_EXTRACT(j.payload, '$.batch_id'))`
	jobWaitSQ = `(SELECT COALESCE(SUM(GREATEST(TIMESTAMPDIFF(SECOND, GREATEST(a.created_at, j.started_at),
		LEAST(COALESCE(a.answered_at, j.completed_at), j.completed_at)), 0)), 0)
		FROM case_approvals a WHERE a.job_id COLLATE utf8mb4_general_ci = j.id COLLATE utf8mb4_general_ci)`
)

type historyBuild struct {
	ID          string               `json:"id"`
	Type        string               `json:"type"`
	Status      string               `json:"status"`
	Error       string               `json:"error"`
	ProductID   int                  `json:"product_id"`
	ProductName string               `json:"product_name"`
	CaseCode    string               `json:"case_code"`
	EpicCode    string               `json:"epic_code"`
	BatchID     string               `json:"batch_id"`
	PRURL       string               `json:"pr_url"`
	Outcome     string               `json:"outcome"`
	CreatedAt   time.Time            `json:"created_at"`
	StartedAt   *time.Time           `json:"started_at"`
	CompletedAt *time.Time           `json:"completed_at"`
	QueueMs     *int64               `json:"queue_ms"`
	ActiveMs    *int64               `json:"active_ms"`
	WaitingMs   int64                `json:"waiting_ms"`
	MachineMs   *int64               `json:"machine_ms"`
	Approvals   int                  `json:"approvals"`
	Runs        int                  `json:"runs"`
	Steps       []runhistory.StepMs  `json:"steps"`
	sums        []runhistory.StepSum // from job_events
	lastAt      *time.Time           // the job's last event
}

func (r *Router) historyBuilds(w http.ResponseWriter, req *http.Request, d *db.DB) {
	q, _, msg := parseHistoryCommon(req, "j.created_at", jobPID)
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	v := req.URL.Query()
	q.add("j.type IN ('case_create', 'case_run')")
	if t := v.Get("type"); t != "" {
		if !oneOf(t, "case_create", "case_run") {
			writeJSONError(w, http.StatusBadRequest, "type must be case_create or case_run")
			return
		}
		q.add("j.type = ?", t)
	}
	if s := v.Get("status"); s != "" {
		if !oneOf(s, "queued", "running", "completed", "failed") {
			writeJSONError(w, http.StatusBadRequest, "status must be queued, running, completed or failed")
			return
		}
		q.add("j.status = ?", s)
	}
	code, ok := codeParam(req, "case")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "case is not a valid code")
		return
	}
	if code != "" {
		q.add(jobCase+" = ?", code)
	}
	epic, ok := codeParam(req, "epic")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "epic is not a valid code")
		return
	}
	if epic != "" {
		q.add(`EXISTS (SELECT 1 FROM kb_test_cases t WHERE t.product_id = `+jobPID+` AND t.code = `+jobCase+` AND t.epic_code = ?)`, epic)
	}
	if b := strings.TrimSpace(v.Get("batch")); b != "" {
		if !historyBatchRe.MatchString(b) {
			writeJSONError(w, http.StatusBadRequest, "batch is not a valid batch id")
			return
		}
		q.add(jobBatch+" = ?", b)
	}
	ctx := req.Context()
	where := q.sql()

	var total int
	if err := d.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs j`+where, q.args...).Scan(&total); err != nil {
		historyInternal(w, "builds count", err)
		return
	}
	var sum struct {
		count, completed, failed, running int
		active, waiting, machine          sql.NullFloat64
	}
	err := d.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(status = 'completed'), 0), COALESCE(SUM(status = 'failed'), 0),
		COALESCE(SUM(status IN ('queued', 'running')), 0),
		AVG(CASE WHEN fin THEN active END), AVG(CASE WHEN fin THEN waiting END), AVG(CASE WHEN fin THEN GREATEST(active - waiting, 0) END)
		FROM (SELECT j.status,
		        (j.status IN ('completed', 'failed') AND j.started_at IS NOT NULL AND j.completed_at IS NOT NULL) AS fin,
		        GREATEST(TIMESTAMPDIFF(SECOND, j.started_at, j.completed_at), 0) * 1000 AS active,
		        `+jobWaitSQ+` * 1000 AS waiting
		      FROM job_runs j`+where+`) x`, q.args...).Scan(&sum.count, &sum.completed, &sum.failed, &sum.running, &sum.active, &sum.waiting, &sum.machine)
	if err != nil {
		historyInternal(w, "builds summary", err)
		return
	}
	avg := func(f sql.NullFloat64) *int64 {
		if !f.Valid {
			return nil
		}
		return msPtr(int64(f.Float64 + 0.5))
	}

	rows, err := d.Query(ctx, `SELECT j.id, j.type, j.status, COALESCE(j.error, ''), j.created_at, j.started_at, j.completed_at,
		COALESCE(`+jobPID+`, 0), COALESCE(`+jobCase+`, ''), COALESCE(`+jobBatch+`, ''),
		COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(j.result, '$.pr_url')), ''), JSON_UNQUOTE(JSON_EXTRACT(j.payload, '$.pr_url')), ''),
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j.result, '$.verdict.result')), ''),
		COALESCE(p.name, ''), COALESCE(t.epic_code, '')
		FROM job_runs j
		LEFT JOIN products p ON p.id = `+jobPID+`
		LEFT JOIN kb_test_cases t ON t.product_id = `+jobPID+` AND t.code = `+jobCase+
		where+` ORDER BY j.created_at DESC, j.id DESC LIMIT ? OFFSET ?`, append(append([]interface{}{}, q.args...), q.size, (q.page-1)*q.size)...)
	if err != nil {
		historyInternal(w, "builds list", err)
		return
	}
	items := []*historyBuild{}
	byID := map[string]*historyBuild{}
	for rows.Next() {
		b := &historyBuild{Steps: []runhistory.StepMs{}}
		var st, ct sql.NullTime
		if err := rows.Scan(&b.ID, &b.Type, &b.Status, &b.Error, &b.CreatedAt, &st, &ct, &b.ProductID, &b.CaseCode, &b.BatchID,
			&b.PRURL, &b.Outcome, &b.ProductName, &b.EpicCode); err != nil {
			rows.Close()
			historyInternal(w, "builds scan", err)
			return
		}
		if st.Valid {
			b.StartedAt = &st.Time
		}
		if ct.Valid {
			b.CompletedAt = &ct.Time
		}
		items = append(items, b)
		byID[b.ID] = b
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		historyInternal(w, "builds rows", err)
		return
	}
	if err := enrichBuilds(ctx, d, items, byID, time.Now().UTC()); err != nil {
		historyInternal(w, "builds enrich", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": q.page, "page_size": q.size,
		"summary": map[string]any{
			"count": sum.count, "completed": sum.completed, "failed": sum.failed, "running": sum.running,
			"avg_active_ms": avg(sum.active), "avg_waiting_ms": avg(sum.waiting), "avg_machine_ms": avg(sum.machine),
		},
	})
}

// enrichBuilds adds step times, approval waits and run counts to one page of builds.
func enrichBuilds(ctx context.Context, d *db.DB, items []*historyBuild, byID map[string]*historyBuild, now time.Time) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]interface{}, len(items))
	for i, b := range items {
		ids[i] = b.ID
	}
	in := inPlaceholders(len(ids))

	rows, err := d.Query(ctx, `SELECT job_id, step, last, SUM(ms), MAX(at) FROM (
		  SELECT job_id, step, at, (LEAD(at) OVER w) IS NULL AS last,
		         COALESCE(TIMESTAMPDIFF(MICROSECOND, at, LEAD(at) OVER w), 0) DIV 1000 AS ms
		  FROM job_events WHERE job_id IN (`+in+`) WINDOW w AS (PARTITION BY job_id ORDER BY seq)
		) x GROUP BY job_id, step, last`, ids...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, step string
		var last bool
		var ms int64
		var at time.Time
		if err := rows.Scan(&id, &step, &last, &ms, &at); err != nil {
			rows.Close()
			return err
		}
		b := byID[id]
		if b == nil {
			continue
		}
		b.sums = append(b.sums, runhistory.StepSum{Step: step, Last: last, Ms: ms})
		if last {
			t := at
			b.lastAt = &t
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	type window struct {
		created  time.Time
		answered *time.Time
	}
	approvals := map[string][]window{}
	rows, err = d.Query(ctx, `SELECT job_id, created_at, answered_at FROM case_approvals WHERE job_id IN (`+in+`)`, ids...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var c time.Time
		var a sql.NullTime
		if err := rows.Scan(&id, &c, &a); err != nil {
			rows.Close()
			return err
		}
		wd := window{created: c}
		if a.Valid {
			t := a.Time
			wd.answered = &t
		}
		approvals[id] = append(approvals[id], wd)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = d.Query(ctx, `SELECT job_id, COUNT(*) FROM pw_runs WHERE job_id IN (`+in+`) GROUP BY job_id`, ids...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return err
		}
		if b := byID[id]; b != nil {
			b.Runs = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, b := range items {
		end := now
		if b.CompletedAt != nil {
			end = *b.CompletedAt
		}
		if b.lastAt != nil {
			for i := range b.sums {
				if b.sums[i].Last {
					b.sums[i].Ms += runhistory.Clip(end.Sub(*b.lastAt).Milliseconds())
				}
			}
		}
		b.Steps = runhistory.Breakdown(b.Type, b.sums)
		b.Approvals = len(approvals[b.ID])
		if b.StartedAt == nil {
			continue
		}
		start := *b.StartedAt
		b.QueueMs = msPtr(runhistory.Clip(start.Sub(b.CreatedAt).Milliseconds()))
		active := runhistory.Clip(end.Sub(start).Milliseconds())
		b.ActiveMs = msPtr(active)
		for _, a := range approvals[b.ID] {
			from, to := a.created, end
			if a.answered != nil && a.answered.Before(end) {
				to = *a.answered
			}
			if from.Before(start) {
				from = start
			}
			b.WaitingMs += runhistory.Clip(to.Sub(from).Milliseconds())
		}
		b.MachineMs = msPtr(runhistory.Clip(active - b.WaitingMs))
	}
	return nil
}

// ---------- Playwright runs ----------

const runOutcome = `CASE WHEN r.unexpected > 0 THEN 'fail' WHEN r.expected + r.flaky > 0 THEN 'pass' ELSE 'skipped' END`

type historyRunRow struct {
	ID             int64     `json:"id"`
	ProductID      int       `json:"product_id"`
	ProductName    string    `json:"product_name"`
	Source         string    `json:"source"`
	RunRef         string    `json:"run_ref"`
	JobID          string    `json:"job_id"`
	CaseCode       string    `json:"case_code"`
	RunIndex       *int      `json:"run_index"`
	StartedAt      time.Time `json:"started_at"`
	DurationMs     *int64    `json:"duration_ms"`
	Tests          int       `json:"tests"`
	Passed         int       `json:"passed"`
	Failed         int       `json:"failed"`
	Flaky          int       `json:"flaky"`
	Skipped        int       `json:"skipped"`
	Cases          int       `json:"cases"`
	Outcome        string    `json:"outcome"`
	CaseResult     *string   `json:"case_result,omitempty"`
	CaseDurationMs *int64    `json:"case_duration_ms,omitempty"`
}

const runCols = `r.id, r.product_id, COALESCE(p.name, ''), r.source, r.run_ref, r.job_id, r.case_code, r.run_index, r.started_at,
	r.duration_ms, r.tests, r.expected, r.unexpected, r.flaky, r.skipped, r.cases, ` + runOutcome

func scanRun(sc dbRowScanner, extra ...interface{}) (*historyRunRow, error) {
	var x historyRunRow
	var idx sql.NullInt64
	var dur sql.NullInt64
	dst := []interface{}{&x.ID, &x.ProductID, &x.ProductName, &x.Source, &x.RunRef, &x.JobID, &x.CaseCode, &idx, &x.StartedAt,
		&dur, &x.Tests, &x.Passed, &x.Failed, &x.Flaky, &x.Skipped, &x.Cases, &x.Outcome}
	if err := sc.Scan(append(dst, extra...)...); err != nil {
		return nil, err
	}
	if idx.Valid {
		n := int(idx.Int64)
		x.RunIndex = &n
	}
	if dur.Valid {
		x.DurationMs = msPtr(dur.Int64)
	}
	return &x, nil
}

func (r *Router) historyRuns(w http.ResponseWriter, req *http.Request, d *db.DB) {
	q, pid, msg := parseHistoryCommon(req, "r.started_at", "r.product_id")
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	v := req.URL.Query()
	if s := v.Get("source"); s != "" {
		if !oneOf(s, runhistory.SourceCI, runhistory.SourceUpload, runhistory.SourceCaseRun, runhistory.SourceCaseCreate) {
			writeJSONError(w, http.StatusBadRequest, "source must be ci, upload, case-run or case-create")
			return
		}
		q.add("r.source = ?", s)
	}
	if o := v.Get("outcome"); o != "" {
		if !oneOf(o, "pass", "fail", "skipped") {
			writeJSONError(w, http.StatusBadRequest, "outcome must be pass, fail or skipped")
			return
		}
		q.add(runOutcome+" = ?", o)
	}
	if j := strings.TrimSpace(v.Get("job")); j != "" {
		if !historyJobRe.MatchString(j) {
			writeJSONError(w, http.StatusBadRequest, "job is not a valid job id")
			return
		}
		q.add("r.job_id = ?", j)
	}
	code, ok := codeParam(req, "case")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "case is not a valid code")
		return
	}
	if code != "" {
		q.add("EXISTS (SELECT 1 FROM pw_run_cases c WHERE c.run_id = r.id AND c.case_code = ?)", code)
	}
	epic, ok := codeParam(req, "epic")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "epic is not a valid code")
		return
	}
	if epic != "" {
		q.add(`EXISTS (SELECT 1 FROM pw_run_cases c JOIN kb_test_cases t ON t.product_id = c.product_id AND t.code = c.case_code
			WHERE c.run_id = r.id AND t.epic_code = ?)`, epic)
	}
	ctx := req.Context()
	where := q.sql()

	var total, nPass, nFail, nSkip int
	var avgDur sql.NullFloat64
	if err := d.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(o = 'pass'), 0), COALESCE(SUM(o = 'fail'), 0), COALESCE(SUM(o = 'skipped'), 0), AVG(duration_ms)
		FROM (SELECT `+runOutcome+` AS o, r.duration_ms FROM pw_runs r`+where+`) x`, q.args...).Scan(&total, &nPass, &nFail, &nSkip, &avgDur); err != nil {
		historyInternal(w, "runs summary", err)
		return
	}
	var since sql.NullTime
	sinceSQL, sinceArgs := `SELECT MIN(created_at) FROM pw_runs`, []interface{}{}
	if pid > 0 {
		sinceSQL += ` WHERE product_id = ?`
		sinceArgs = append(sinceArgs, pid)
	}
	if err := d.QueryRow(ctx, sinceSQL, sinceArgs...).Scan(&since); err != nil {
		historyInternal(w, "runs since", err)
		return
	}

	sel, args := `SELECT `+runCols, []interface{}{}
	from := ` FROM pw_runs r LEFT JOIN products p ON p.id = r.product_id`
	if code != "" {
		sel += `, cc.result, cc.duration_ms`
		from += ` LEFT JOIN pw_run_cases cc ON cc.run_id = r.id AND cc.case_code = ?`
		args = append(args, code)
	}
	args = append(append(args, q.args...), q.size, (q.page-1)*q.size)
	rows, err := d.Query(ctx, sel+from+where+` ORDER BY r.started_at DESC, r.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		historyInternal(w, "runs list", err)
		return
	}
	defer rows.Close()
	items := []*historyRunRow{}
	for rows.Next() {
		var cres sql.NullString
		var cdur sql.NullInt64
		var extra []interface{}
		if code != "" {
			extra = []interface{}{&cres, &cdur}
		}
		x, err := scanRun(rows, extra...)
		if err != nil {
			historyInternal(w, "runs scan", err)
			return
		}
		if cres.Valid {
			s := cres.String
			x.CaseResult = &s
		}
		if cdur.Valid {
			x.CaseDurationMs = msPtr(cdur.Int64)
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		historyInternal(w, "runs rows", err)
		return
	}
	var sinceOut *time.Time
	if since.Valid {
		sinceOut = &since.Time
	}
	var avgOut *int64
	if avgDur.Valid {
		avgOut = msPtr(int64(avgDur.Float64 + 0.5))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": q.page, "page_size": q.size, "since": sinceOut,
		"summary": map[string]any{"count": total, "pass": nPass, "fail": nFail, "skipped": nSkip, "avg_duration_ms": avgOut},
	})
}

type historyRunCase struct {
	CaseCode   string `json:"case_code"`
	EpicCode   string `json:"epic_code"`
	Known      bool   `json:"known"`
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
	Tests      int    `json:"tests"`
	Attempts   int    `json:"attempts"`
}

func (r *Router) historyRun(w http.ResponseWriter, req *http.Request, d *db.DB, id int64) {
	ctx := req.Context()
	run, err := scanRun(d.QueryRow(ctx, `SELECT `+runCols+` FROM pw_runs r LEFT JOIN products p ON p.id = r.product_id WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		historyInternal(w, "run", err)
		return
	}
	rows, err := d.Query(ctx, `SELECT c.case_code, COALESCE(t.epic_code, ''), (t.id IS NOT NULL AND t.removed_at IS NULL),
		c.result, c.duration_ms, c.tests, c.attempts
		FROM pw_run_cases c LEFT JOIN kb_test_cases t ON t.product_id = c.product_id AND t.code = c.case_code
		WHERE c.run_id = ? ORDER BY c.duration_ms DESC, c.case_code`, id)
	if err != nil {
		historyInternal(w, "run cases", err)
		return
	}
	defer rows.Close()
	cases := []historyRunCase{}
	for rows.Next() {
		var c historyRunCase
		if err := rows.Scan(&c.CaseCode, &c.EpicCode, &c.Known, &c.Result, &c.DurationMs, &c.Tests, &c.Attempts); err != nil {
			historyInternal(w, "run cases scan", err)
			return
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		historyInternal(w, "run cases rows", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		*historyRunRow
		CaseRows []historyRunCase `json:"case_rows"`
	}{run, cases})
}

// ---------- knowledge syncs ----------

type historySync struct {
	ID          int        `json:"id"`
	ProductID   int        `json:"product_id"`
	ProductName string     `json:"product_name"`
	Status      string     `json:"status"`
	Trigger     string     `json:"trigger"`
	RepoName    string     `json:"repo_name"`
	Branch      string     `json:"branch"`
	CommitSHA   string     `json:"commit_sha"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	DurationMs  *int64     `json:"duration_ms"`
	Error       string     `json:"error"`
}

func (r *Router) historySyncs(w http.ResponseWriter, req *http.Request, d *db.DB) {
	q, _, msg := parseHistoryCommon(req, "s.started_at", "s.product_id")
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if s := req.URL.Query().Get("status"); s != "" {
		if !oneOf(s, "running", "succeeded", "failed", "skipped") {
			writeJSONError(w, http.StatusBadRequest, "status must be running, succeeded, failed or skipped")
			return
		}
		q.add("s.status = ?", s)
	}
	ctx := req.Context()
	where := q.sql()
	var total int
	if err := d.QueryRow(ctx, `SELECT COUNT(*) FROM kb_syncs s`+where, q.args...).Scan(&total); err != nil {
		historyInternal(w, "syncs count", err)
		return
	}
	rows, err := d.Query(ctx, `SELECT s.id, s.product_id, COALESCE(p.name, ''), s.status, s.trigger_kind, s.repo_name, s.branch,
		COALESCE(s.commit_sha, ''), s.started_at, s.finished_at, COALESCE(s.error, '')
		FROM kb_syncs s LEFT JOIN products p ON p.id = s.product_id`+where+` ORDER BY s.started_at DESC, s.id DESC LIMIT ? OFFSET ?`,
		append(append([]interface{}{}, q.args...), q.size, (q.page-1)*q.size)...)
	if err != nil {
		historyInternal(w, "syncs list", err)
		return
	}
	defer rows.Close()
	items := []historySync{}
	for rows.Next() {
		var s historySync
		var fin sql.NullTime
		if err := rows.Scan(&s.ID, &s.ProductID, &s.ProductName, &s.Status, &s.Trigger, &s.RepoName, &s.Branch, &s.CommitSHA, &s.StartedAt, &fin, &s.Error); err != nil {
			historyInternal(w, "syncs scan", err)
			return
		}
		if fin.Valid {
			s.FinishedAt = &fin.Time
			s.DurationMs = msPtr(runhistory.Clip(fin.Time.Sub(s.StartedAt).Milliseconds()))
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		historyInternal(w, "syncs rows", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": q.page, "page_size": q.size})
}
