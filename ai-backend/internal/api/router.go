package api

import (
	"context"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/services/learning"
	"github.com/qa-automation/ai-backend/internal/services/learnloop"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/services/skills"
)

// Router handles HTTP requests
type Router struct {
	db              *db.DB
	aiService       *ai.Service
	learningService *learning.Service
	skillsService   *skills.Service
	jobs            *jobManager

	// Playwright execution (nil = case_run jobs fail). See jobs_executor.go.
	executor        *executor.Client
	executorTimeout time.Duration

	// Product repo pool (nil = clone/verify disabled). See products.go.
	repoPool *repopool.Pool
	// knowledge runs and serves product knowledge syncs (nil without a DB).
	kb *knowledge.Syncer
	// exports serves the module checklist export from the object store (nil = not configured, 503).
	exports *knowledge.ExportCache
	// gaos pushes flagged products' test cases to GAOS (nil or unconfigured = off). See products_gaos.go.
	gaos *gaossync.Syncer
	// gaosInboundToken guards /api/v1/gaos/* (GAOS_INBOUND.md); empty = disabled.
	gaosInboundToken string
	// resultsToken guards POST /api/v1/results/playwright (CASE_RESULTS.md); empty = disabled.
	resultsToken string
	// onResultsStored is told the product id after an upload stored results (nil = off).
	onResultsStored func(productID int)

	// caseAuto is the state of run / create-automation (CASE_AUTOMATION.md). See case_automation.go.
	caseAuto caseAuto
	// epicDriver is the tick guard of the epic batch driver (EPIC_AUTOMATION.md). See epic_batches.go.
	epicDriver epicDriverState
	// learn is the learning loop's state (LEARNING_LOOP.md). See learning_loop.go.
	learn learnState
	// approvalRelay is the /history answer relay's state (HISTORY_ANSWER.md). See approval_relay.go.
	approvalRelay approvalRelayState

	// Non-secret config snapshot for GET /api/settings. See settings.go.
	settings SettingsInfo

	// Auth wiring — nil when the database is unavailable. When set, all /api/*
	// routes outside the allow-list go through requireAuth.
	authHandlers    *AuthHandlers
	requireAuth     func(http.Handler) http.Handler
	dashboardOrigin string
}

// SetAuth wires the auth handlers + middleware. Pass dashboardOrigin = "*" to
// keep the legacy permissive CORS; otherwise an exact origin (no wildcard) so
// the browser will send the session cookie.
func (r *Router) SetAuth(h *AuthHandlers, mw func(http.Handler) http.Handler, dashboardOrigin string) {
	r.authHandlers = h
	r.requireAuth = mw
	r.dashboardOrigin = dashboardOrigin
}

type jobStatus string

const (
	jobStatusQueued    jobStatus = "queued"
	jobStatusRunning   jobStatus = "running"
	jobStatusCompleted jobStatus = "completed"
	jobStatusFailed    jobStatus = "failed"
)

type jobRecord struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Status      jobStatus              `json:"status"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	DurationMs  int64                  `json:"duration_ms,omitempty"`
}

type jobManager struct {
	mu   sync.RWMutex
	jobs map[string]*jobRecord
}

func newJobManager() *jobManager {
	return &jobManager{
		jobs: map[string]*jobRecord{},
	}
}

// NewRouter creates a new router
func NewRouter(
	db *db.DB,
	aiService *ai.Service,
	learningService *learning.Service,
	skillsService *skills.Service,
) *Router {
	return &Router{
		db:              db,
		aiService:       aiService,
		learningService: learningService,
		skillsService:   skillsService,
		jobs:            newJobManager(),
	}
}

// getDB returns the database wrapper, or nil if not available.
// The wrapper exposes context-first Query/QueryRow/Exec methods backed by
// *sql.DB (MySQL).
func (r *Router) getDB() *db.DB {
	if r.db == nil || r.db.SQL == nil {
		return nil
	}
	return r.db
}

func nullStringOrEmpty(value stdsql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// boolToInt converts a Go bool into 1/0 for MySQL TINYINT(1) bind parameters.
// Some MySQL versions/drivers refuse to compare bool against TINYINT, so we
// always send an int.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parsePagination(queryLimit, queryPage string, defaultLimit int) (page int, limit int) {
	page = 1
	limit = defaultLimit

	if p, err := strconv.Atoi(queryPage); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(queryLimit); err == nil && l > 0 {
		limit = l
	}
	if limit > 100 {
		limit = 100
	}

	return page, limit
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func (r *Router) handleJobs(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/api/jobs" {
		r.handleListJobs(w, req)
		return
	}

	if !strings.HasPrefix(req.URL.Path, "/api/jobs/") {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	jobID := strings.TrimPrefix(req.URL.Path, "/api/jobs/")
	// /api/jobs/{id}/events and /api/jobs/{id}/approval (CASE_AUTOMATION.md).
	if id, sub, ok := strings.Cut(jobID, "/"); ok && id != "" && !strings.Contains(sub, "/") {
		r.handleJobSub(w, req, id, sub)
		return
	}
	if jobID == "" || strings.Contains(jobID, "/") {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	r.handleGetJob(w, req, jobID)
}

func (r *Router) enqueueJob(jobType string, payload map[string]interface{}) *jobRecord {
	job, _ := r.enqueueJobWith(jobType, payload, false)
	return job
}

// enqueueJobTx is enqueueJob for callers that must not run a job whose row was not stored
// (the epic driver derives a batch's state from job_runs): a failed insert returns the error
// and starts nothing.
func (r *Router) enqueueJobTx(jobType string, payload map[string]interface{}) (*jobRecord, error) {
	return r.enqueueJobWith(jobType, payload, true)
}

func (r *Router) enqueueJobWith(jobType string, payload map[string]interface{}, strict bool) (*jobRecord, error) {
	return r.enqueueJobID(newJobID(), jobType, payload, strict)
}

// newJobID is the id of a new job_runs row.
func newJobID() string { return fmt.Sprintf("job-%d", time.Now().UnixNano()) }

// enqueueJobID is enqueueJobWith with an id chosen by the caller (the learning loop reserves its
// findings for the job before the job exists).
func (r *Router) enqueueJobID(id, jobType string, payload map[string]interface{}, strict bool) (*jobRecord, error) {
	job := &jobRecord{
		ID:        id,
		Type:      jobType,
		Status:    jobStatusQueued,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}

	if strict {
		if err := r.upsertJobRecordErr(context.Background(), job); err != nil {
			return nil, err
		}
	}
	r.jobs.mu.Lock()
	r.jobs.jobs[job.ID] = job
	r.jobs.mu.Unlock()
	if !strict {
		r.upsertJobRecord(context.Background(), job)
	}

	if jobType == "case_create" {
		go r.runCaseCreate(job.ID, payload, false)
		return job, nil
	}
	if jobType == learnloop.JobType {
		go r.runPolicyPropose(job.ID, payload, nil)
		return job, nil
	}
	if r.executor != nil {
		go r.runExecutorJob(job.ID, jobType, payload)
		return job, nil
	}

	// No playwright-executor configured: nothing can run the job.
	go r.updateJob(job.ID, func(j *jobRecord) {
		now := time.Now().UTC()
		j.Status = jobStatusFailed
		j.Error = "playwright-executor is not configured"
		j.CompletedAt = &now
	})
	return job, nil
}

func (r *Router) handleGetJob(w http.ResponseWriter, req *http.Request, jobID string) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	r.jobs.mu.RLock()
	job, ok := r.jobs.jobs[jobID]
	r.jobs.mu.RUnlock()

	if ok {
		json.NewEncoder(w).Encode(job)
		return
	}

	db := r.getDB()
	if db == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	row := db.QueryRow(req.Context(), `
		SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms
		FROM job_runs
		WHERE id = ?
	`, jobID)

	jobFromDB, err := scanJobRow(row)
	if err != nil {
		if errors.Is(err, stdsql.ErrNoRows) {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(jobFromDB)
}

func (r *Router) handleListJobs(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	query := req.URL.Query()
	typeFilter := strings.TrimSpace(query.Get("type"))
	statusFilter := strings.TrimSpace(query.Get("status"))
	sortBy := strings.TrimSpace(query.Get("sort_by"))
	page, limit := parsePagination(query.Get("limit"), query.Get("page"), 20)
	offset := (page - 1) * limit

	db := r.getDB()
	if db == nil {
		r.jobs.mu.RLock()
		records := make([]jobRecord, 0, len(r.jobs.jobs))
		for _, item := range r.jobs.jobs {
			if typeFilter != "" && item.Type != typeFilter {
				continue
			}
			if statusFilter != "" && string(item.Status) != statusFilter {
				continue
			}
			records = append(records, *item)
		}
		r.jobs.mu.RUnlock()

		sort.Slice(records, func(i, j int) bool {
			return records[i].CreatedAt.After(records[j].CreatedAt)
		})

		total := len(records)
		totalPages := (total + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}
		start := offset
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": records[start:end],
			"pagination": map[string]interface{}{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
			},
		})
		return
	}

	filters := []string{}
	args := []interface{}{}
	addArg := func(v interface{}) string {
		args = append(args, v)
		return "?"
	}
	if typeFilter != "" {
		filters = append(filters, "type = "+addArg(typeFilter))
	}
	if statusFilter != "" {
		filters = append(filters, "status = "+addArg(statusFilter))
	}

	whereSQL := ""
	if len(filters) > 0 {
		whereSQL = " WHERE " + strings.Join(filters, " AND ")
	}

	// MySQL has no NULLS LAST; we emulate it by sorting the IS NULL flag first.
	orderSQL := " ORDER BY created_at DESC"
	switch sortBy {
	case "created_oldest":
		orderSQL = " ORDER BY created_at ASC"
	case "started_newest":
		orderSQL = " ORDER BY started_at IS NULL, started_at DESC, created_at DESC"
	case "started_oldest":
		orderSQL = " ORDER BY started_at IS NULL, started_at ASC, created_at ASC"
	case "duration_high":
		orderSQL = " ORDER BY duration_ms IS NULL, duration_ms DESC, created_at DESC"
	case "duration_low":
		orderSQL = " ORDER BY duration_ms IS NULL, duration_ms ASC, created_at DESC"
	}

	listSQL := `
		SELECT id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms
		FROM job_runs
	` + whereSQL + orderSQL + " LIMIT ? OFFSET ?"
	listArgs := append(append([]interface{}{}, args...), limit, offset)

	rows, err := db.Query(req.Context(), listSQL, listArgs...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var jobs []jobRecord
	for rows.Next() {
		job, err := scanJobValues(rows)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jobs = append(jobs, job)
	}
	if jobs == nil {
		jobs = []jobRecord{}
	}

	countSQL := "SELECT COUNT(*) FROM job_runs" + whereSQL
	var total int
	if err := db.QueryRow(req.Context(), countSQL, args...).Scan(&total); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": jobs,
		"pagination": map[string]interface{}{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

func (r *Router) upsertJobRecord(ctx context.Context, job *jobRecord) {
	if err := r.upsertJobRecordErr(ctx, job); err != nil {
		log.Printf("failed to upsert job record id=%s: %v", job.ID, err)
	}
}

// upsertJobRecordErr stores the record and returns the database error, if any.
func (r *Router) upsertJobRecordErr(ctx context.Context, job *jobRecord) error {
	db := r.getDB()
	if db == nil || job == nil {
		return nil
	}

	var payload interface{}
	if len(job.Payload) > 0 {
		raw, err := json.Marshal(job.Payload)
		if err != nil {
			return err // a job whose payload cannot be stored would lose its batch id
		}
		payload = raw
	}
	var result interface{}
	if len(job.Result) > 0 {
		raw, err := json.Marshal(job.Result)
		if err == nil {
			result = raw
		}
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO job_runs (id, type, status, payload, result, error, created_at, started_at, completed_at, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			type = VALUES(type),
			status = VALUES(status),
			payload = VALUES(payload),
			result = VALUES(result),
			error = VALUES(error),
			created_at = VALUES(created_at),
			started_at = VALUES(started_at),
			completed_at = VALUES(completed_at),
			duration_ms = VALUES(duration_ms)
	`, job.ID, job.Type, string(job.Status), payload, result, job.Error, job.CreatedAt, job.StartedAt, job.CompletedAt, job.DurationMs); err != nil {
		return err
	}
	return nil
}

type dbRowScanner interface {
	Scan(dest ...interface{}) error
}

// scanJobRow accepts the sql.Row returned by QueryRowContext (or any value
// implementing Scan, which keeps existing call sites working).
func scanJobRow(row dbRowScanner) (jobRecord, error) {
	return scanJobValues(row)
}

func scanJobValues(scanner dbRowScanner) (jobRecord, error) {
	var job jobRecord
	var status string
	var payloadRaw []byte
	var resultRaw []byte
	var errorValue stdsql.NullString
	var startedAt stdsql.NullTime
	var completedAt stdsql.NullTime
	var durationMs stdsql.NullInt64

	if err := scanner.Scan(
		&job.ID, &job.Type, &status, &payloadRaw, &resultRaw, &errorValue,
		&job.CreatedAt, &startedAt, &completedAt, &durationMs,
	); err != nil {
		return job, err
	}

	job.Status = jobStatus(status)
	if len(payloadRaw) > 0 {
		_ = json.Unmarshal(payloadRaw, &job.Payload)
	}
	if len(resultRaw) > 0 {
		_ = json.Unmarshal(resultRaw, &job.Result)
	}
	if errorValue.Valid {
		job.Error = errorValue.String
	}
	if startedAt.Valid {
		v := startedAt.Time
		job.StartedAt = &v
	}
	if completedAt.Valid {
		v := completedAt.Time
		job.CompletedAt = &v
	}
	if durationMs.Valid {
		job.DurationMs = durationMs.Int64
	}

	return job, nil
}

// ServeHTTP handles HTTP requests. Order of operations:
//  1. CORS headers (origin-specific when auth is on, wildcard when off).
//  2. Allow-list paths bypass auth (/health, /api/auth/login, OPTIONS).
//  3. Everything else passes through requireAuth (when wired) before the
//     existing per-path dispatch.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.applyCORS(w, req)

	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Allow-list — bypass auth middleware entirely.
	if r.authHandlers != nil && req.URL.Path == "/api/auth/login" {
		r.authHandlers.Login(w, req)
		return
	}
	if req.URL.Path == "/health" {
		r.serveHealth(w)
		return
	}
	// GAOS inbound: bearer token instead of the session cookie (GAOS_INBOUND.md).
	if IsGAOSInboundPath(req.URL.Path) {
		r.serveGAOSInbound(w, req)
		return
	}
	// CI test results: bearer token as well (CASE_RESULTS.md).
	if IsResultsIngestPath(req.URL.Path) {
		r.serveResultsIngest(w, req)
		return
	}

	// Everything else goes through the auth middleware (if wired) before
	// hitting the existing per-path dispatch.
	dispatch := http.HandlerFunc(r.dispatchAuthenticated)
	if r.requireAuth != nil {
		r.requireAuth(dispatch).ServeHTTP(w, req)
		return
	}
	dispatch.ServeHTTP(w, req)
}

func (r *Router) applyCORS(w http.ResponseWriter, req *http.Request) {
	origin := r.dashboardOrigin
	if origin == "" {
		origin = "*"
	}
	// When CORS includes credentials, the origin MUST be a single value, not "*".
	if origin == "*" && req.Header.Get("Origin") != "" && r.authHandlers != nil {
		origin = req.Header.Get("Origin")
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.authHandlers != nil {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
	}
}

func (r *Router) serveHealth(w http.ResponseWriter) {
	dbStatus := "disconnected"
	if r.db != nil {
		dbStatus = "connected"
	}
	// "minimax" is kept for backward compatibility with existing probes/tests.
	minimaxStatus := "not_configured"
	aiProvider, aiUpstream, aiStatus := "", "", "not_configured"
	if r.aiService != nil {
		aiProvider = r.aiService.ProviderName()
		aiUpstream = r.aiService.Upstream()
		aiStatus = r.aiService.Health(context.Background())
		if aiProvider == ai.ProviderMiniMax && r.aiService.Configured() {
			minimaxStatus = "configured"
		}
	}
	executorStatus := "not_configured"
	if r.executor != nil {
		executorStatus = r.executor.Health(context.Background())
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":      "ok",
		"database":    dbStatus,
		"ai_provider": aiProvider,
		"ai_upstream": aiUpstream,
		"ai":          aiStatus,
		"minimax":     minimaxStatus,
		"executor":    executorStatus,
	})
}

// dispatchAuthenticated routes any path that survived the auth middleware.
// This is the original big switch, minus the CORS/health blocks that moved
// up into ServeHTTP.
func (r *Router) dispatchAuthenticated(w http.ResponseWriter, req *http.Request) {
	// Auth endpoints (other than login, which bypasses auth above).
	if r.authHandlers != nil {
		switch req.URL.Path {
		case "/api/auth/logout":
			r.authHandlers.Logout(w, req)
			return
		case "/api/auth/me":
			r.authHandlers.Me(w, req)
			return
		case "/api/auth/change-password":
			r.authHandlers.ChangePassword(w, req)
			return
		case "/api/auth/impersonate":
			r.authHandlers.Impersonate(w, req)
			return
		case "/api/auth/impersonate/stop":
			r.authHandlers.StopImpersonation(w, req)
			return
		case "/api/users":
			r.authHandlers.ListUsers(w, req)
			return
		}
	}

	// Products: /api/products and /api/products/{id}[/verify|clone|checkout].
	if req.URL.Path == "/api/products" || strings.HasPrefix(req.URL.Path, "/api/products/") {
		r.handleProducts(w, req)
		return
	}

	// EPIC_AUTOMATION.md: /api/epic-batches/{id}/cancel.
	if strings.HasPrefix(req.URL.Path, "/api/epic-batches/") {
		r.handleEpicBatches(w, req)
		return
	}

	// AI_CONVERSATIONS.md: /api/conversations[/{id}[/archive|unarchive]|/prune|/prunes].
	if req.URL.Path == "/api/conversations" || strings.HasPrefix(req.URL.Path, "/api/conversations/") {
		r.handleConversations(w, req)
		return
	}

	// RUN_HISTORY.md: /api/history/{builds,runs,runs/{id},syncs} (read-only).
	if strings.HasPrefix(req.URL.Path, "/api/history/") {
		r.handleHistory(w, req)
		return
	}

	// LEARNING_LOOP.md: /api/learning/{findings,proposals,timeline}[/...]. The legacy
	// /api/learning and /api/learning/navigation stay in the switch below.
	if strings.HasPrefix(req.URL.Path, "/api/learning/") && req.URL.Path != "/api/learning/navigation" {
		r.handleLearningLoop(w, req)
		return
	}

	// Job endpoints include path params (/api/jobs/:id).
	if strings.HasPrefix(req.URL.Path, "/api/jobs/") {
		r.handleJobs(w, req)
		return
	}

	// Health check kept for parity if a downstream caller hits it after auth.
	if req.URL.Path == "/health" {
		r.serveHealth(w)
		return
	}

	// Route request
	switch req.URL.Path {
	// Settings (read-only, non-secret)
	case "/api/settings":
		r.handleSettings(w, req)

	// Skill endpoints
	case "/api/skills":
		r.handleSkills(w, req)
	case "/api/assistants":
		r.handleAssistants(w, req)
	case "/api/skills/logs":
		r.handleSkillLogs(w, req)
	case "/api/skills/stats":
		r.handleSkillStats(w, req)

	// Cost analytics endpoints
	case "/api/cost/overview":
		r.handleCostOverview(w, req)
	case "/api/cost/trend":
		r.handleCostTrend(w, req)
	case "/api/cost/efficiency":
		r.handleCostEfficiency(w, req)
	case "/api/cost/by-feature":
		r.handleCostByFeature(w, req)
	case "/api/cost/by-model":
		r.handleCostByModel(w, req)
	case "/api/cost/recent":
		r.handleCostRecent(w, req)
	case "/api/cost/filters":
		r.handleCostFilters(w, req)
	case "/api/cost/gateway":
		r.handleCostGateway(w, req)

	// Learning endpoints
	case "/api/learning":
		r.handleLearning(w, req)
	case "/api/learning/navigation":
		r.handleLearningNavigation(w, req)

	// Job endpoints
	case "/api/jobs":
		r.handleListJobs(w, req)

	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

// handleSkills handles /api/skills
func (r *Router) handleSkills(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	query := req.URL.Query()
	search := strings.TrimSpace(query.Get("search"))
	severity := strings.TrimSpace(query.Get("severity"))
	sortBy := strings.TrimSpace(query.Get("sort_by"))
	page, limit := parsePagination(query.Get("limit"), query.Get("page"), 20)

	var filterActive *bool
	switch strings.ToLower(strings.TrimSpace(query.Get("is_active"))) {
	case "true":
		v := true
		filterActive = &v
	case "false":
		v := false
		filterActive = &v
	}

	sql := `
		SELECT id, name, description, check_type, check_query, severity, is_builtin, is_active, created_at, product_id
		FROM skills
		WHERE 1=1
	`
	args := []interface{}{}
	// product_id=N: global skills plus that product's; product_id=global: global skills only.
	productWhere, productArgs := "", []interface{}{}
	switch pf := strings.TrimSpace(query.Get("product_id")); {
	case pf == "global":
		productWhere = " AND product_id IS NULL"
	case pf != "":
		pid, err := strconv.Atoi(pf)
		if err != nil || pid <= 0 {
			writeJSONError(w, http.StatusBadRequest, "product_id must be a product id or global")
			return
		}
		productWhere, productArgs = " AND (product_id IS NULL OR product_id = ?)", []interface{}{pid}
	}
	sql += productWhere
	args = append(args, productArgs...)

	if search != "" {
		sql += " AND (name LIKE CONCAT('%', ?, '%') OR description LIKE CONCAT('%', ?, '%') OR check_query LIKE CONCAT('%', ?, '%'))"
		args = append(args, search, search, search)
	}
	if severity != "" && !strings.EqualFold(severity, "all") {
		sql += " AND severity = ?"
		args = append(args, severity)
	}
	if filterActive != nil {
		sql += " AND is_active = ?"
		args = append(args, boolToInt(*filterActive))
	}

	switch sortBy {
	case "name_desc":
		sql += " ORDER BY name DESC"
	case "severity_high":
		sql += " ORDER BY CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END ASC, name ASC"
	case "severity_low":
		sql += " ORDER BY CASE severity WHEN 'low' THEN 1 WHEN 'medium' THEN 2 WHEN 'high' THEN 3 ELSE 4 END ASC, name ASC"
	case "date_oldest":
		sql += " ORDER BY created_at ASC"
	default:
		sql += " ORDER BY is_builtin DESC, name ASC"
	}

	offset := (page - 1) * limit
	sql += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Query(req.Context(), sql, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var skills []models.Skill
	for rows.Next() {
		var skill models.Skill
		if err := rows.Scan(&skill.ID, &skill.Name, &skill.Description, &skill.CheckType, &skill.CheckQuery,
			&skill.Severity, &skill.IsBuiltin, &skill.IsActive, &skill.CreatedAt, &skill.ProductID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		skills = append(skills, skill)
	}

	if skills == nil {
		skills = []models.Skill{}
	}

	countSQL := "SELECT COUNT(*) FROM skills WHERE 1=1" + productWhere
	countArgs := append([]interface{}{}, productArgs...)
	if search != "" {
		countSQL += " AND (name LIKE CONCAT('%', ?, '%') OR description LIKE CONCAT('%', ?, '%') OR check_query LIKE CONCAT('%', ?, '%'))"
		countArgs = append(countArgs, search, search, search)
	}
	if severity != "" && !strings.EqualFold(severity, "all") {
		countSQL += " AND severity = ?"
		countArgs = append(countArgs, severity)
	}
	if filterActive != nil {
		countSQL += " AND is_active = ?"
		countArgs = append(countArgs, boolToInt(*filterActive))
	}

	totalCount := 0
	if err := db.QueryRow(req.Context(), countSQL, countArgs...).Scan(&totalCount); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalPages := (totalCount + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": skills,
		"pagination": map[string]interface{}{
			"page":        page,
			"limit":       limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

// handleSkillLogs handles /api/skills/logs
func (r *Router) handleSkillLogs(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		query := req.URL.Query()
		skillID := strings.TrimSpace(query.Get("skill_id"))
		passed := strings.TrimSpace(query.Get("passed"))
		fromDate := strings.TrimSpace(query.Get("from_date"))
		toDate := strings.TrimSpace(query.Get("to_date"))
		search := strings.TrimSpace(query.Get("search"))
		pageQuery := strings.TrimSpace(query.Get("page"))
		pageFilter := strings.TrimSpace(query.Get("path"))
		page, limit := parsePagination(query.Get("limit"), query.Get("page"), 20)
		if pageQuery != "" {
			if _, err := strconv.Atoi(pageQuery); err != nil && pageFilter == "" {
				// Backward compatibility: old clients used page as exact path filter.
				pageFilter = pageQuery
				page = 1
			}
		}
		offset := (page - 1) * limit

		filters := []string{}
		args := []interface{}{}
		addArg := func(v interface{}) string {
			args = append(args, v)
			return "?"
		}

		if skillID != "" {
			filters = append(filters, "sl.skill_id = "+addArg(skillID))
		}
		if pageFilter != "" {
			filters = append(filters, "sl.page = "+addArg(pageFilter))
		}
		if passed != "" {
			filters = append(filters, "sl.is_passed = "+addArg(boolToInt(strings.EqualFold(passed, "true"))))
		}
		if fromDate != "" {
			filters = append(filters, "sl.created_at >= "+addArg(fromDate))
		}
		if toDate != "" {
			filters = append(filters, "sl.created_at <= "+addArg(toDate))
		}
		if search != "" {
			// Three columns share the same parameter, so bind it three times.
			a1 := addArg(search)
			a2 := addArg(search)
			a3 := addArg(search)
			a4 := addArg(search)
			a5 := addArg(search)
			filters = append(filters,
				"(COALESCE(s.name,'') LIKE CONCAT('%', "+a1+", '%')"+
					" OR sl.page LIKE CONCAT('%', "+a2+", '%')"+
					" OR sl.action LIKE CONCAT('%', "+a3+", '%')"+
					" OR sl.result LIKE CONCAT('%', "+a4+", '%')"+
					" OR COALESCE(sl.error_message,'') LIKE CONCAT('%', "+a5+", '%'))")
		}

		whereSQL := ""
		if len(filters) > 0 {
			whereSQL = " AND " + strings.Join(filters, " AND ")
		}

		sql := `
			SELECT sl.id, sl.skill_id, sl.page, sl.action, sl.result, sl.is_passed, sl.error_message, sl.created_at,
			       COALESCE(s.name, 'Unknown') as skill_name
			FROM skill_logs sl
			LEFT JOIN skills s ON sl.skill_id = s.id
			WHERE 1=1
		` + whereSQL + " ORDER BY sl.created_at DESC LIMIT ? OFFSET ?"
		listArgs := append(append([]interface{}{}, args...), limit, offset)

		rows, err := db.Query(req.Context(), sql, listArgs...)
		if err != nil {
			log.Printf("Error querying skill logs: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type SkillLogWithName struct {
			models.SkillLog
			SkillName string `json:"skill_name,omitempty"`
		}

		var skillLogs []SkillLogWithName
		for rows.Next() {
			var entry SkillLogWithName
			var errorMsg stdsql.NullString
			if err := rows.Scan(&entry.ID, &entry.SkillID, &entry.Page, &entry.Action, &entry.Result,
				&entry.IsPassed, &errorMsg, &entry.CreatedAt, &entry.SkillName); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}
			entry.ErrorMsg = nullStringOrEmpty(errorMsg)
			skillLogs = append(skillLogs, entry)
		}

		if skillLogs == nil {
			skillLogs = []SkillLogWithName{}
		}

		countSQL := `
			SELECT COUNT(*)
			FROM skill_logs sl
			LEFT JOIN skills s ON sl.skill_id = s.id
			WHERE 1=1
		` + whereSQL
		totalCount := 0
		if err := db.QueryRow(req.Context(), countSQL, args...).Scan(&totalCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		totalPages := (totalCount + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": skillLogs,
			"pagination": map[string]interface{}{
				"page":        page,
				"limit":       limit,
				"total":       totalCount,
				"total_pages": totalPages,
			},
		})

	case "POST":
		// Create skill log entry
		var payload struct {
			SkillID  int    `json:"skill_id"`
			Page     string `json:"page"`
			Action   string `json:"action"`
			Result   string `json:"result"`
			IsPassed bool   `json:"is_passed"`
			ErrorMsg string `json:"error_message,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.SkillID == 0 || payload.Page == "" || payload.Action == "" || payload.Result == "" {
			http.Error(w, "skill_id, page, action, and result are required", http.StatusBadRequest)
			return
		}

		res, err := db.Exec(req.Context(), `
			INSERT INTO skill_logs (skill_id, page, action, result, is_passed, error_message)
			VALUES (?, ?, ?, ?, ?, ?)
		`, payload.SkillID, payload.Page, payload.Action, payload.Result, boolToInt(payload.IsPassed), payload.ErrorMsg)
		if err != nil {
			log.Printf("Error inserting skill log: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id := int(res.LastInsertId())
		var createdAt time.Time
		if err := db.QueryRow(req.Context(),
			`SELECT created_at FROM skill_logs WHERE id = ?`, id,
		).Scan(&createdAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         id,
			"skill_id":   payload.SkillID,
			"page":       payload.Page,
			"action":     payload.Action,
			"result":     payload.Result,
			"is_passed":  payload.IsPassed,
			"created_at": createdAt,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSkillStats handles /api/skills/stats
func (r *Router) handleSkillStats(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	type SkillStats struct {
		SkillID    int        `json:"skill_id"`
		SkillName  string     `json:"skill_name"`
		TotalRuns  int        `json:"total_runs"`
		PassedRuns int        `json:"passed_runs"`
		FailedRuns int        `json:"failed_runs"`
		PassRate   float64    `json:"pass_rate"`
		LastRun    *time.Time `json:"last_run,omitempty"`
	}

	query := req.URL.Query()
	search := strings.TrimSpace(query.Get("search"))
	sortBy := strings.TrimSpace(query.Get("sort_by"))
	page, limit := parsePagination(query.Get("limit"), query.Get("page"), 20)
	offset := (page - 1) * limit

	whereSQL := ""
	args := []interface{}{}
	if search != "" {
		whereSQL = " WHERE skill_name LIKE CONCAT('%', ?, '%')"
		args = append(args, search)
	}

	orderSQL := " ORDER BY total_runs DESC, skill_name ASC"
	switch sortBy {
	case "name_asc":
		orderSQL = " ORDER BY skill_name ASC"
	case "name_desc":
		orderSQL = " ORDER BY skill_name DESC"
	case "pass_rate_desc":
		orderSQL = " ORDER BY pass_rate DESC, total_runs DESC"
	case "pass_rate_asc":
		orderSQL = " ORDER BY pass_rate ASC, total_runs DESC"
	case "last_run_newest":
		orderSQL = " ORDER BY last_run IS NULL, last_run DESC, skill_name ASC"
	case "last_run_oldest":
		orderSQL = " ORDER BY last_run IS NULL, last_run ASC, skill_name ASC"
	case "total_runs_asc":
		orderSQL = " ORDER BY total_runs ASC, skill_name ASC"
	}

	// MySQL has no COUNT(*) FILTER (WHERE ...); use SUM(CASE WHEN ...).
	aggregatedSQL := `
		SELECT
			sl.skill_id,
			COALESCE(s.name, 'Unknown') AS skill_name,
			COUNT(*) AS total_runs,
			SUM(CASE WHEN sl.is_passed = 1 THEN 1 ELSE 0 END) AS passed_runs,
			SUM(CASE WHEN sl.is_passed = 0 THEN 1 ELSE 0 END) AS failed_runs,
			CASE WHEN COUNT(*) > 0
				THEN SUM(CASE WHEN sl.is_passed = 1 THEN 1 ELSE 0 END) * 1.0 / COUNT(*)
				ELSE 0
			END AS pass_rate,
			MAX(sl.created_at) AS last_run
		FROM skill_logs sl
		LEFT JOIN skills s ON sl.skill_id = s.id
		GROUP BY sl.skill_id, s.name
	`

	sql := `
		SELECT skill_id, skill_name, total_runs, passed_runs, failed_runs, pass_rate, last_run
		FROM (` + aggregatedSQL + `) AS skill_stats
	` + whereSQL + orderSQL + " LIMIT ? OFFSET ?"
	listArgs := append(append([]interface{}{}, args...), limit, offset)

	rows, err := db.Query(req.Context(), sql, listArgs...)
	if err != nil {
		log.Printf("Error querying skill stats: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var stats []SkillStats
	for rows.Next() {
		var stat SkillStats
		if err := rows.Scan(&stat.SkillID, &stat.SkillName, &stat.TotalRuns, &stat.PassedRuns,
			&stat.FailedRuns, &stat.PassRate, &stat.LastRun); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		stats = append(stats, stat)
	}

	if stats == nil {
		stats = []SkillStats{}
	}

	countSQL := "SELECT COUNT(*) FROM (" + aggregatedSQL + ") AS skill_stats" + whereSQL
	totalCount := 0
	if err := db.QueryRow(req.Context(), countSQL, args...).Scan(&totalCount); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalPages := (totalCount + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": stats,
		"pagination": map[string]interface{}{
			"page":        page,
			"limit":       limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

// handleLearning handles /api/learning
func (r *Router) handleLearning(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		// List learning records with optional filters
		query := req.URL.Query()
		intent := query.Get("intent")
		page := query.Get("page")
		limit := 10
		if l := query.Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}

		sql := `
			SELECT id, intent, action, page, success_score, usage_count,
			       last_success, decay_factor, created_at, updated_at
			FROM learning_cache
			WHERE 1=1
		`
		args := []interface{}{}

		if intent != "" {
			sql += " AND intent LIKE CONCAT('%', ?, '%')"
			args = append(args, intent)
		}

		if page != "" {
			sql += " AND page = ?"
			args = append(args, page)
		}

		sql += " ORDER BY success_score DESC, usage_count DESC LIMIT ?"
		args = append(args, limit)

		rows, err := db.Query(req.Context(), sql, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var records []models.LearningRecord
		for rows.Next() {
			var record models.LearningRecord
			if err := rows.Scan(&record.ID, &record.Intent, &record.Action, &record.Page,
				&record.SuccessScore, &record.UsageCount, &record.LastSuccess,
				&record.DecayFactor, &record.CreatedAt, &record.UpdatedAt); err != nil {
				continue
			}
			records = append(records, record)
		}

		if records == nil {
			records = []models.LearningRecord{}
		}
		json.NewEncoder(w).Encode(records)

	case "POST":
		// Add new learning record
		var payload struct {
			Intent  string `json:"intent"`
			Action  string `json:"action"`
			Page    string `json:"page"`
			Success bool   `json:"success"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.Intent == "" || payload.Action == "" {
			http.Error(w, "intent and action are required", http.StatusBadRequest)
			return
		}

		// Check if record exists
		var existingID int
		err := db.QueryRow(req.Context(), `
			SELECT id FROM learning_cache
			WHERE intent = ? AND action = ? AND page = ?
		`, payload.Intent, payload.Action, payload.Page).Scan(&existingID)

		if err == nil {
			// Update existing record
			scoreDelta := -0.1
			if payload.Success {
				scoreDelta = 0.1
			}
			successFlag := boolToInt(payload.Success)
			_, err = db.Exec(req.Context(), `
				UPDATE learning_cache
				SET success_score = LEAST(GREATEST(success_score + ?, 0.0), 1.0),
				    usage_count = usage_count + 1,
				    last_success = CASE WHEN ? = 1 THEN NOW() ELSE last_success END,
				    updated_at = NOW()
				WHERE id = ?
			`, scoreDelta, successFlag, existingID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      existingID,
				"updated": true,
			})
		} else if errors.Is(err, stdsql.ErrNoRows) {
			// Insert new record
			score := 0.5
			if payload.Success {
				score = 1.0
			}
			successFlag := boolToInt(payload.Success)
			res, err := db.Exec(req.Context(), `
				INSERT INTO learning_cache (intent, action, page, success_score, usage_count, last_success)
				VALUES (?, ?, ?, ?, 1, CASE WHEN ? = 1 THEN NOW() ELSE NULL END)
			`, payload.Intent, payload.Action, payload.Page, score, successFlag)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			id := int(res.LastInsertId())
			var createdAt, updatedAt time.Time
			if err := db.QueryRow(req.Context(),
				`SELECT created_at, updated_at FROM learning_cache WHERE id = ?`, id,
			).Scan(&createdAt, &updatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = updatedAt
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         id,
				"intent":     payload.Intent,
				"action":     payload.Action,
				"page":       payload.Page,
				"success":    payload.Success,
				"created_at": createdAt,
			})
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	case "DELETE":
		// Delete learning record
		id := req.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id parameter required", http.StatusBadRequest)
			return
		}

		var intID int
		fmt.Sscanf(id, "%d", &intID)

		result, err := db.Exec(req.Context(), `DELETE FROM learning_cache WHERE id = ?`, intID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if result.RowsAffected() == 0 {
			writeJSONError(w, http.StatusNotFound, "learning record not found")
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"deleted": true,
			"id":      intID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLearningNavigation handles /api/learning/navigation
func (r *Router) handleLearningNavigation(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		// Get navigation edges from a source page
		sourcePage := req.URL.Query().Get("source")
		if sourcePage == "" {
			http.Error(w, "source parameter required", http.StatusBadRequest)
			return
		}

		edges, err := r.learningService.GetNavigationEdges(req.Context(), sourcePage)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(edges)

	case "POST":
		// Add navigation edge
		var payload struct {
			SourcePage string `json:"source_page"`
			TargetPage string `json:"target_page"`
			Action     string `json:"action"`
			Success    bool   `json:"success"`
			DurationMs int    `json:"duration_ms"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.SourcePage == "" || payload.TargetPage == "" || payload.Action == "" {
			http.Error(w, "source_page, target_page, and action are required", http.StatusBadRequest)
			return
		}

		err := r.learningService.AddNavigationEdge(req.Context(),
			payload.SourcePage, payload.TargetPage, payload.Action, payload.Success, payload.DurationMs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"added":       true,
			"source_page": payload.SourcePage,
			"target_page": payload.TargetPage,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
