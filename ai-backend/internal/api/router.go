package api

import (
	"context"
	"crypto/sha256"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
	"github.com/qa-automation/ai-backend/internal/services/learning"
	"github.com/qa-automation/ai-backend/internal/services/skills"
)

// Router handles HTTP requests
type Router struct {
	db              *db.DB
	aiService       *ai.Service
	learningService *learning.Service
	skillsService   *skills.Service
	jobs            *jobManager

	// Playwright execution (nil = simulated jobs). See jobs_executor.go.
	executor        *executor.Client
	executorTimeout time.Duration

	// Product repo pool (nil = clone/verify disabled). See products.go.
	repoPool *repopool.Pool

	// Non-secret config snapshot for GET /api/settings. See settings.go.
	settings SettingsInfo

	// Auth wiring — nil when the database is unavailable. When set, all /api/*
	// routes outside the allow-list go through requireAuth.
	authHandlers   *AuthHandlers
	requireAuth    func(http.Handler) http.Handler
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
// The wrapper exposes pgx-compatible Query/QueryRow/Exec methods backed by
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

	switch req.URL.Path {
	case "/api/jobs/qa/run":
		r.handleRunQAJob(w, req)
		return
	case "/api/jobs/docs/generate":
		r.handleGenerateDocsJob(w, req)
		return
	}

	if !strings.HasPrefix(req.URL.Path, "/api/jobs/") {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	jobID := strings.TrimPrefix(req.URL.Path, "/api/jobs/")
	if jobID == "" || strings.Contains(jobID, "/") {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	r.handleGetJob(w, req, jobID)
}

func (r *Router) enqueueJob(jobType string, payload map[string]interface{}) *jobRecord {
	job := &jobRecord{
		ID:        fmt.Sprintf("job-%d", time.Now().UnixNano()),
		Type:      jobType,
		Status:    jobStatusQueued,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}

	r.jobs.mu.Lock()
	r.jobs.jobs[job.ID] = job
	r.jobs.mu.Unlock()
	r.upsertJobRecord(context.Background(), job)

	if r.executor != nil {
		go r.runExecutorJob(job.ID, jobType, payload)
		return job
	}

	go func(id string) {
		startedAt := time.Now().UTC()
		r.jobs.mu.Lock()
		record := r.jobs.jobs[id]
		record.Status = jobStatusRunning
		record.StartedAt = &startedAt
		r.jobs.mu.Unlock()

		// No playwright-executor configured: simulate minimal async work.
		time.Sleep(200 * time.Millisecond)

		completedAt := time.Now().UTC()
		r.jobs.mu.Lock()
		record = r.jobs.jobs[id]
		record.Status = jobStatusCompleted
		record.CompletedAt = &completedAt
		record.DurationMs = completedAt.Sub(startedAt).Milliseconds()
		record.Result = map[string]interface{}{
			"message":   fmt.Sprintf("%s job completed", jobType),
			"simulated": true,
		}
		r.jobs.mu.Unlock()
		r.upsertJobRecord(context.Background(), record)
	}(job.ID)

	return job
}

func (r *Router) handleRunQAJob(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	payload := map[string]interface{}{}
	if req.Body != nil {
		_ = json.NewDecoder(req.Body).Decode(&payload)
	}

	job := r.enqueueJob("qa_run", payload)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(job)
}

func (r *Router) handleGenerateDocsJob(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	payload := map[string]interface{}{}
	if req.Body != nil {
		_ = json.NewDecoder(req.Body).Decode(&payload)
	}

	job := r.enqueueJob("docs_generate", payload)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(job)
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
	db := r.getDB()
	if db == nil || job == nil {
		return
	}

	var payload interface{}
	if len(job.Payload) > 0 {
		raw, err := json.Marshal(job.Payload)
		if err == nil {
			payload = raw
		}
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
		log.Printf("failed to upsert job record id=%s: %v", job.ID, err)
	}
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
	// Flow endpoints
	case "/api/flows":
		r.handleFlows(w, req)

	// Settings (read-only, non-secret)
	case "/api/settings":
		r.handleSettings(w, req)

	// Role endpoints
	case "/api/roles":
		r.handleRoles(w, req)

	// Docs endpoints
	case "/api/docs":
		r.handleDocs(w, req)

	// Video endpoints
	case "/api/videos":
		r.handleVideos(w, req)

	// Issue endpoints
	case "/api/issues":
		r.handleIssues(w, req)
	case "/api/issues/status":
		r.handleIssueStatus(w, req)
	case "/api/issues/notes":
		r.handleIssueNotes(w, req)

	// Skill endpoints
	case "/api/skills":
		r.handleSkills(w, req)
	case "/api/skills/logs":
		r.handleSkillLogs(w, req)
	case "/api/skills/stats":
		r.handleSkillStats(w, req)

	// Search endpoint
	case "/api/search":
		r.handleSearch(w, req)

	// Cost analytics endpoints
	case "/api/cost/overview":
		r.handleCostOverview(w, req)
	case "/api/cost/trend":
		r.handleCostTrend(w, req)
	case "/api/cost/efficiency":
		r.handleCostEfficiency(w, req)
	case "/api/cost/by-feature":
		r.handleCostByFeature(w, req)

	// Analytics endpoints
	case "/api/analytics":
		r.handleAnalytics(w, req)

	// Learning endpoints
	case "/api/learning":
		r.handleLearning(w, req)
	case "/api/learning/navigation":
		r.handleLearningNavigation(w, req)
	case "/api/learning/sequences":
		r.handleLearningSequences(w, req)
	case "/api/learning/context":
		r.handleLearningContext(w, req)
	case "/api/learning/stats":
		r.handleLearningStats(w, req)

	// AI Cache endpoints
	case "/api/ai-cache":
		r.handleAICache(w, req)

	// Embedding Cache endpoints
	case "/api/embedding-cache":
		r.handleEmbeddingCache(w, req)

	// Goal Events endpoints
	case "/api/goal-events":
		r.handleGoalEvents(w, req)

	// Job endpoints
	case "/api/jobs":
		r.handleListJobs(w, req)
	case "/api/jobs/qa/run":
		r.handleRunQAJob(w, req)
	case "/api/jobs/docs/generate":
		r.handleGenerateDocsJob(w, req)

	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

// handleFlows handles /api/flows
func (r *Router) handleFlows(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	switch req.Method {
	case "GET":
		// List flows
		rows, err := db.Query(req.Context(), `
			SELECT id, role_id, name, description, created_at, updated_at
			FROM flows
			ORDER BY created_at DESC
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var flows []models.Flow
		for rows.Next() {
			var flow models.Flow
			if err := rows.Scan(&flow.ID, &flow.RoleID, &flow.Name, &flow.Description, &flow.CreatedAt, &flow.UpdatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			flows = append(flows, flow)
		}

		json.NewEncoder(w).Encode(flows)

	case "POST":
		// Create flow
		var flow models.Flow
		if err := json.NewDecoder(req.Body).Decode(&flow); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate required fields
		if flow.RoleID == 0 {
			http.Error(w, "role_id is required", http.StatusBadRequest)
			return
		}
		if flow.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		res, err := db.Exec(req.Context(), `
			INSERT INTO flows (role_id, name, description)
			VALUES (?, ?, ?)
		`, flow.RoleID, flow.Name, flow.Description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		flow.ID = int(res.LastInsertId())
		if err := db.QueryRow(req.Context(),
			`SELECT created_at, updated_at FROM flows WHERE id = ?`, flow.ID,
		).Scan(&flow.CreatedAt, &flow.UpdatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Set proper headers and status before encoding
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(flow)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRoles handles /api/roles
func (r *Router) handleRoles(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		rows, err := db.Query(req.Context(), `
			SELECT id, name, description, created_at
			FROM roles
			ORDER BY name
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var roles []models.Role
		for rows.Next() {
			var role models.Role
			if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			roles = append(roles, role)
		}

		if roles == nil {
			roles = []models.Role{}
		}
		json.NewEncoder(w).Encode(roles)

	case "POST":
		var role models.Role
		if err := json.NewDecoder(req.Body).Decode(&role); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if role.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		res, err := db.Exec(req.Context(), `
			INSERT INTO roles (name, description)
			VALUES (?, ?)
		`, role.Name, role.Description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		role.ID = int(res.LastInsertId())
		if err := db.QueryRow(req.Context(),
			`SELECT created_at FROM roles WHERE id = ?`, role.ID,
		).Scan(&role.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(role)

	case "DELETE":
		id := req.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id parameter required", http.StatusBadRequest)
			return
		}

		var intID int
		fmt.Sscanf(id, "%d", &intID)

		result, err := db.Exec(req.Context(), `DELETE FROM roles WHERE id = ?`, intID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if result.RowsAffected() == 0 {
			writeJSONError(w, http.StatusNotFound, "role not found")
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

// handleIssues handles /api/issues
func (r *Router) handleIssues(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	switch req.Method {
	case "GET":
		// List issues with optional filters
		w.Header().Set("Content-Type", "application/json")

		query := req.URL.Query()
		filterFixed := query.Get("is_fixed")
		category := query.Get("category")
		target := query.Get("target")
		search := query.Get("search")
		sortBy := query.Get("sort_by")
		page := 1
		limit := 20

		// Parse pagination
		if p := query.Get("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
			if page < 1 {
				page = 1
			}
		}
		if l := query.Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
			if limit < 1 || limit > 100 {
				limit = 20
			}
		}

		// Base SQL query (MySQL: LIKE is case-insensitive on _ci collations).
		sql := `SELECT i.id, i.page, i.action, i.error_type, i.error_text,
			       i.category, i.severity, i.is_fixed, i.notes, i.created_at
			FROM raw_issues i
			WHERE 1=1`
		args := []interface{}{}

		// Filter by fixed status
		if filterFixed != "" {
			if strings.EqualFold(filterFixed, "true") {
				sql += " AND i.is_fixed = 1"
			} else {
				// Legacy rows can have NULL is_fixed; treat them as open/unfixed.
				sql += " AND (i.is_fixed = 0 OR i.is_fixed IS NULL)"
			}
		}

		// Filter by category
		if category != "" && category != "all" {
			sql += " AND i.category = ?"
			args = append(args, category)
		}

		// Filter by target (frontend/backend via suggestions)
		if target != "" {
			sql += " AND i.id IN (SELECT issue_id FROM issue_suggestions WHERE target = ?)"
			args = append(args, target)
		}

		// Search by page, action, or error text
		if search != "" {
			sql += " AND (i.page LIKE CONCAT('%', ?, '%') OR i.action LIKE CONCAT('%', ?, '%') OR i.error_text LIKE CONCAT('%', ?, '%'))"
			args = append(args, search, search, search)
		}

		// Sorting
		switch sortBy {
		case "date_oldest":
			sql += " ORDER BY i.created_at ASC"
		case "severity_high":
			sql += " ORDER BY CASE i.severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END ASC"
		case "severity_low":
			sql += " ORDER BY CASE i.severity WHEN 'low' THEN 1 WHEN 'medium' THEN 2 WHEN 'high' THEN 3 ELSE 4 END ASC"
		default:
			sql += " ORDER BY i.created_at DESC"
		}

		// Calculate offset for pagination
		offset := (page - 1) * limit
		sql += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)

		rows, err := db.Query(req.Context(), sql, args...)
		if err != nil {
			log.Printf("Error querying issues: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type IssueWithSuggestions struct {
			models.RawIssue
			Suggestions []models.IssueSuggestion `json:"suggestions"`
		}

		var issues []IssueWithSuggestions
		for rows.Next() {
			var issue models.RawIssue
			var errorType stdsql.NullString
			var errorText stdsql.NullString
			var categoryValue stdsql.NullString
			var severityValue stdsql.NullString
			var isFixed stdsql.NullBool
			var notesValue stdsql.NullString
			if err := rows.Scan(
				&issue.ID, &issue.Page, &issue.Action, &errorType, &errorText,
				&categoryValue, &severityValue, &isFixed, &notesValue, &issue.CreatedAt,
			); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}
			issue.ErrorType = nullStringOrEmpty(errorType)
			issue.ErrorText = nullStringOrEmpty(errorText)
			issue.Category = nullStringOrEmpty(categoryValue)
			issue.Severity = nullStringOrEmpty(severityValue)
			issue.IsFixed = isFixed.Valid && isFixed.Bool
			issue.Notes = nullStringOrEmpty(notesValue)

			// Fetch suggestions for this issue
			suggestions, err := r.getSuggestionsForIssue(issue.ID)
			if err != nil {
				log.Printf("Error fetching suggestions: %v", err)
			}

			issues = append(issues, IssueWithSuggestions{
				RawIssue:    issue,
				Suggestions: suggestions,
			})
		}

		// Get total count for pagination
		var totalCount int
		countSql := `SELECT COUNT(*) FROM raw_issues WHERE 1=1`
		countArgs := []interface{}{}

		if filterFixed != "" {
			if strings.EqualFold(filterFixed, "true") {
				countSql += " AND is_fixed = 1"
			} else {
				// Keep count logic identical with list query semantics.
				countSql += " AND (is_fixed = 0 OR is_fixed IS NULL)"
			}
		}
		if category != "" && category != "all" {
			countSql += " AND category = ?"
			countArgs = append(countArgs, category)
		}
		if search != "" {
			countSql += " AND (page LIKE CONCAT('%', ?, '%') OR action LIKE CONCAT('%', ?, '%') OR error_text LIKE CONCAT('%', ?, '%'))"
			countArgs = append(countArgs, search, search, search)
		}

		db.QueryRow(req.Context(), countSql, countArgs...).Scan(&totalCount)

		// Calculate pagination info
		totalPages := (totalCount + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}

		// Return empty array instead of null if no results
		if issues == nil {
			issues = []IssueWithSuggestions{}
		}

		// Return with pagination info
		response := map[string]interface{}{
			"data": issues,
			"pagination": map[string]interface{}{
				"page":        page,
				"limit":       limit,
				"total":       totalCount,
				"total_pages": totalPages,
			},
		}
		json.NewEncoder(w).Encode(response)

	case "POST":
		// Create issue
		var issue models.RawIssue
		if err := json.NewDecoder(req.Body).Decode(&issue); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		issue.Page = strings.TrimSpace(issue.Page)
		issue.Action = strings.TrimSpace(issue.Action)
		issue.Severity = strings.TrimSpace(issue.Severity)

		// Validate required fields - page and action are required
		if issue.Page == "" {
			http.Error(w, "page is required", http.StatusBadRequest)
			return
		}
		if issue.Action == "" {
			http.Error(w, "action is required", http.StatusBadRequest)
			return
		}
		if issue.Severity == "" {
			issue.Severity = "medium"
		}

		res, err := db.Exec(req.Context(), `
			INSERT INTO raw_issues (page, action, error_type, error_text, category, severity)
			VALUES (?, ?, ?, ?, ?, ?)
		`, issue.Page, issue.Action, issue.ErrorType, issue.ErrorText, issue.Category, issue.Severity)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		issue.ID = int(res.LastInsertId())
		if err := db.QueryRow(req.Context(),
			`SELECT created_at FROM raw_issues WHERE id = ?`, issue.ID,
		).Scan(&issue.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Generate category using AI only when service is available
		if issue.Category == "" && issue.ErrorText != "" && r.aiService != nil {
			category, _ := r.aiService.ClassifyIssue(issue.ErrorText)
			issue.Category = category
			db.Exec(req.Context(), `UPDATE raw_issues SET category = ? WHERE id = ?`, category, issue.ID)
		}

		// Set proper headers and status before encoding
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(issue)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getSuggestionsForIssue fetches suggestions for a specific issue
func (r *Router) getSuggestionsForIssue(issueID int) ([]models.IssueSuggestion, error) {
	db := r.getDB()
	if db == nil {
		return []models.IssueSuggestion{}, nil
	}

	rows, err := db.Query(context.Background(), `
		SELECT id, issue_id, suggestion, confidence, target, file_path, diff
		FROM issue_suggestions
		WHERE issue_id = ?
	`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []models.IssueSuggestion
	for rows.Next() {
		var sug models.IssueSuggestion
		if err := rows.Scan(&sug.ID, &sug.IssueID, &sug.Suggestion, &sug.Confidence, &sug.Target, &sug.FilePath, &sug.Diff); err != nil {
			log.Printf("Scan suggestion error: %v", err)
			continue
		}
		suggestions = append(suggestions, sug)
	}

	if suggestions == nil {
		suggestions = []models.IssueSuggestion{}
	}
	return suggestions, nil
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
		SELECT id, name, description, check_type, check_query, severity, is_builtin, is_active, created_at
		FROM skills
		WHERE 1=1
	`
	args := []interface{}{}

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
			&skill.Severity, &skill.IsBuiltin, &skill.IsActive, &skill.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		skills = append(skills, skill)
	}

	if skills == nil {
		skills = []models.Skill{}
	}

	countSQL := "SELECT COUNT(*) FROM skills WHERE 1=1"
	countArgs := []interface{}{}
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

// handleSearch handles /api/search
func (r *Router) handleSearch(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := req.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query parameter required", http.StatusBadRequest)
		return
	}

	// Check if database is available
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	// Search in learning cache - simple substring match (no vector index in MySQL).
	rows, err := db.Query(req.Context(), `
		SELECT intent, action, page, success_score, usage_count
		FROM learning_cache
		WHERE intent LIKE CONCAT('%', ?, '%')
		ORDER BY success_score DESC, usage_count DESC
		LIMIT 10
	`, query)
	if err != nil {
		log.Printf("Search: Database query error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type SearchResult struct {
		Intent     string  `json:"intent"`
		Action     string  `json:"action"`
		Page       string  `json:"page"`
		Score      float64 `json:"score"`
		UsageCount int     `json:"usage_count"`
	}

	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.Intent, &result.Action, &result.Page, &result.Score, &result.UsageCount); err != nil {
			continue
		}
		results = append(results, result)
	}

	// Return empty array instead of null if no results
	if results == nil {
		results = []SearchResult{}
	}

	json.NewEncoder(w).Encode(results)
}

// handleCostOverview handles /api/cost/overview
func (r *Router) handleCostOverview(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var overview models.CostOverview
	var totalRequests int64
	var totalTokens int64
	var inputTokens int64
	var outputTokens int64
	var totalCost float64
	var cacheHits int64
	var totalCount int64

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	err := db.QueryRow(req.Context(), `
		SELECT
			COUNT(*),
			COALESCE(SUM(input_tokens + output_tokens), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cost_estimate), 0),
			COALESCE(SUM(CASE WHEN cache_hit = 1 THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM request_logs
	`).Scan(
		&totalRequests, &totalTokens, &inputTokens, &outputTokens,
		&totalCost, &cacheHits, &totalCount,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	overview.TotalRequests = int(totalRequests)
	overview.TotalTokens = int(totalTokens)
	overview.InputTokens = int(inputTokens)
	overview.OutputTokens = int(outputTokens)
	overview.TotalCost = totalCost
	if totalCount > 0 {
		overview.CacheHitRate = float64(cacheHits) / float64(totalCount)
	} else {
		overview.CacheHitRate = 0
	}

	json.NewEncoder(w).Encode(overview)
}

// handleCostTrend handles /api/cost/trend
func (r *Router) handleCostTrend(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	rows, err := db.Query(req.Context(), `
		SELECT DATE(created_at) AS d, SUM(cost_estimate) AS cost
		FROM request_logs
		WHERE created_at > NOW() - INTERVAL 30 DAY
		GROUP BY DATE(created_at)
		ORDER BY d
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var trends []models.CostTrend
	for rows.Next() {
		var trend models.CostTrend
		if err := rows.Scan(&trend.Date, &trend.Cost); err != nil {
			continue
		}
		trends = append(trends, trend)
	}

	// Return empty array instead of null if no results
	if trends == nil {
		trends = []models.CostTrend{}
	}

	json.NewEncoder(w).Encode(trends)
}

// handleCostEfficiency handles /api/cost/efficiency
func (r *Router) handleCostEfficiency(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var stats struct {
		Hit  int `json:"hit"`
		Miss int `json:"miss"`
	}

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	err := db.QueryRow(req.Context(), `
		SELECT
			COALESCE(SUM(CASE WHEN cache_hit = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN cache_hit = 0 THEN 1 ELSE 0 END), 0)
		FROM request_logs
	`).Scan(&stats.Hit, &stats.Miss)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(stats)
}

// handleCostByFeature handles /api/cost/by-feature
func (r *Router) handleCostByFeature(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	rows, err := db.Query(req.Context(), `
		SELECT COALESCE(endpoint, 'unknown') as feature, SUM(cost_estimate) as cost
		FROM request_logs
		GROUP BY endpoint
		ORDER BY cost DESC
		LIMIT 20
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []models.CostByFeature
	for rows.Next() {
		var result models.CostByFeature
		if err := rows.Scan(&result.Feature, &result.Cost); err != nil {
			continue
		}
		results = append(results, result)
	}

	// Return empty array instead of null if no results
	if results == nil {
		results = []models.CostByFeature{}
	}

	json.NewEncoder(w).Encode(results)
}

// handleDocs handles /api/docs
func (r *Router) handleDocs(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	type DocInfo struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Product     string `json:"product,omitempty"`
		Description string `json:"description,omitempty"`
	}

	query := req.URL.Query()
	search := strings.ToLower(strings.TrimSpace(query.Get("search")))
	productFilter := strings.TrimSpace(query.Get("product"))
	sortBy := strings.TrimSpace(query.Get("sort_by"))
	page, limit := parsePagination(query.Get("limit"), query.Get("page"), 20)

	var docs []DocInfo

	// Walk through product-documentations directory
	productDir := "../product-documentations"
	entries, err := os.ReadDir(productDir)
	if err != nil {
		log.Printf("Error reading product-documentations: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []DocInfo{},
			"pagination": map[string]interface{}{
				"page":        page,
				"limit":       limit,
				"total":       0,
				"total_pages": 1,
			},
		})
		return
	}

	for _, productEntry := range entries {
		if !productEntry.IsDir() {
			continue
		}

		productName := productEntry.Name()
		productPath := filepath.Join(productDir, productName)

		docEntries, err := os.ReadDir(productPath)
		if err != nil {
			continue
		}

		for _, docEntry := range docEntries {
			if docEntry.IsDir() || filepath.Ext(docEntry.Name()) == ".md" {
				docName := strings.TrimSuffix(docEntry.Name(), ".md")
				docs = append(docs, DocInfo{
					Name:    docName,
					Path:    filepath.Join(productPath, docEntry.Name()),
					Product: productName,
				})
			}
		}
	}

	filtered := make([]DocInfo, 0, len(docs))
	for _, doc := range docs {
		if productFilter != "" && !strings.EqualFold(doc.Product, productFilter) {
			continue
		}
		if search != "" {
			content := strings.ToLower(doc.Name + " " + doc.Product + " " + doc.Path + " " + doc.Description)
			if !strings.Contains(content, search) {
				continue
			}
		}
		filtered = append(filtered, doc)
	}

	switch sortBy {
	case "name_desc":
		sort.SliceStable(filtered, func(i, j int) bool {
			return strings.ToLower(filtered[i].Name) > strings.ToLower(filtered[j].Name)
		})
	case "product_asc":
		sort.SliceStable(filtered, func(i, j int) bool {
			if strings.EqualFold(filtered[i].Product, filtered[j].Product) {
				return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
			}
			return strings.ToLower(filtered[i].Product) < strings.ToLower(filtered[j].Product)
		})
	case "product_desc":
		sort.SliceStable(filtered, func(i, j int) bool {
			if strings.EqualFold(filtered[i].Product, filtered[j].Product) {
				return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
			}
			return strings.ToLower(filtered[i].Product) > strings.ToLower(filtered[j].Product)
		})
	default:
		sort.SliceStable(filtered, func(i, j int) bool {
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		})
	}

	totalCount := len(filtered)
	totalPages := (totalCount + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	offset := (page - 1) * limit
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	pageData := filtered[offset:end]
	if pageData == nil {
		pageData = []DocInfo{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": pageData,
		"pagination": map[string]interface{}{
			"page":        page,
			"limit":       limit,
			"total":       totalCount,
			"total_pages": totalPages,
		},
	})
}

// handleVideos handles /api/videos
func (r *Router) handleVideos(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	type VideoInfo struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Size int64  `json:"size,omitempty"`
	}

	var videos []VideoInfo

	// Walk through playwright-docs/output/videos directory
	videoDir := "../playwright-docs/output/videos"
	entries, err := os.ReadDir(videoDir)
	if err != nil {
		log.Printf("Error reading videos directory: %v", err)
		json.NewEncoder(w).Encode([]VideoInfo{})
		return
	}

	for _, domainEntry := range entries {
		if !domainEntry.IsDir() {
			continue
		}

		domainName := domainEntry.Name()
		domainPath := filepath.Join(videoDir, domainName)

		videoFiles, err := os.ReadDir(domainPath)
		if err != nil {
			continue
		}

		for _, videoFile := range videoFiles {
			if !videoFile.IsDir() && (filepath.Ext(videoFile.Name()) == ".webm" || filepath.Ext(videoFile.Name()) == ".mp4") {
				fileInfo, err := videoFile.Info()
				var size int64
				if err == nil {
					size = fileInfo.Size()
				}

				videos = append(videos, VideoInfo{
					Name: videoFile.Name(),
					Path: filepath.Join(domainPath, videoFile.Name()),
					Size: size,
				})
			}
		}
	}

	if videos == nil {
		videos = []VideoInfo{}
	}
	json.NewEncoder(w).Encode(videos)
}

// handleAnalytics handles /api/analytics
func (r *Router) handleAnalytics(w http.ResponseWriter, req *http.Request) {
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

	type AnalyticsResponse struct {
		Flows           int `json:"flows"`
		Issues          int `json:"issues"`
		Skills          int `json:"skills"`
		GoalEvents      int `json:"goal_events"`
		NavigationEdges int `json:"navigation_edges"`
		ActionSequences int `json:"action_sequences"`
		TotalRecords    int `json:"total_records"`
	}

	response := AnalyticsResponse{}

	// Get flows count
	err := db.QueryRow(req.Context(), `SELECT COUNT(*) FROM flows`).Scan(&response.Flows)
	if err != nil {
		log.Printf("Error counting flows: %v", err)
	}

	// Get issues count
	err = db.QueryRow(req.Context(), `SELECT COUNT(*) FROM raw_issues`).Scan(&response.Issues)
	if err != nil {
		log.Printf("Error counting issues: %v", err)
	}

	// Get skills count
	err = db.QueryRow(req.Context(), `SELECT COUNT(*) FROM skills`).Scan(&response.Skills)
	if err != nil {
		log.Printf("Error counting skills: %v", err)
	}

	// Get goal events count
	err = db.QueryRow(req.Context(), `SELECT COUNT(*) FROM goal_events`).Scan(&response.GoalEvents)
	if err != nil {
		log.Printf("Error counting goal events: %v", err)
	}

	// Get navigation edges count
	err = db.QueryRow(req.Context(), `SELECT COUNT(*) FROM navigation_edges`).Scan(&response.NavigationEdges)
	if err != nil {
		log.Printf("Error counting navigation edges: %v", err)
	}

	// Get action sequences count
	err = db.QueryRow(req.Context(), `SELECT COUNT(*) FROM action_sequences`).Scan(&response.ActionSequences)
	if err != nil {
		log.Printf("Error counting action sequences: %v", err)
	}

	// Calculate total records
	response.TotalRecords = response.Flows + response.Issues + response.Skills +
		response.GoalEvents + response.NavigationEdges + response.ActionSequences

	json.NewEncoder(w).Encode(response)
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

// handleAICache handles /api/ai-cache
func (r *Router) handleAICache(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		// Get cached response by hash
		hash := req.URL.Query().Get("hash")
		if hash == "" {
			http.Error(w, "hash parameter required", http.StatusBadRequest)
			return
		}

		var cached struct {
			ID         int       `json:"id"`
			InputHash  string    `json:"input_hash"`
			InputText  string    `json:"input_text"`
			OutputText string    `json:"output_text"`
			Model      string    `json:"model"`
			Cost       float64   `json:"cost_estimate"`
			CreatedAt  time.Time `json:"created_at"`
		}

		err := db.QueryRow(req.Context(), `
			SELECT id, input_hash, input_text, output_text, model, cost_estimate, created_at
			FROM ai_cache
			WHERE input_hash = ?
		`, hash).Scan(&cached.ID, &cached.InputHash, &cached.InputText, &cached.OutputText, &cached.Model, &cached.Cost, &cached.CreatedAt)

		if err != nil {
			http.Error(w, "cache not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(cached)

	case "POST":
		// Store AI response in cache
		var payload struct {
			InputText  string  `json:"input_text"`
			OutputText string  `json:"output_text"`
			Model      string  `json:"model,omitempty"`
			Cost       float64 `json:"cost,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.InputText == "" || payload.OutputText == "" {
			http.Error(w, "input_text and output_text are required", http.StatusBadRequest)
			return
		}

		// Generate hash
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(payload.InputText)))

		// Check if exists
		var existingID int
		err := db.QueryRow(req.Context(), `SELECT id FROM ai_cache WHERE input_hash = ?`, hash).Scan(&existingID)

		if err == nil {
			// Return existing without inserting
			json.NewEncoder(w).Encode(map[string]interface{}{
				"cached":  true,
				"id":      existingID,
				"hash":    hash,
				"message": "Cache hit",
			})
			return
		}

		// Insert new cache entry
		model := payload.Model
		if model == "" {
			model = "MiniMax-M2.5"
		}

		res, err := db.Exec(req.Context(), `
			INSERT INTO ai_cache (input_hash, input_text, output_text, model, cost_estimate)
			VALUES (?, ?, ?, ?, ?)
		`, hash, payload.InputText, payload.OutputText, model, payload.Cost)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id := int(res.LastInsertId())
		var createdAt time.Time
		if err := db.QueryRow(req.Context(),
			`SELECT created_at FROM ai_cache WHERE id = ?`, id,
		).Scan(&createdAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         id,
			"hash":       hash,
			"cached":     false,
			"created_at": createdAt,
		})

	case "DELETE":
		// Clear cache or specific entry
		hash := req.URL.Query().Get("hash")
		if hash != "" {
			result, err := db.Exec(req.Context(), `DELETE FROM ai_cache WHERE input_hash = ?`, hash)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if result.RowsAffected() == 0 {
				writeJSONError(w, http.StatusNotFound, "ai cache entry not found")
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "hash": hash})
		} else {
			// Clear all
			_, err := db.Exec(req.Context(), `DELETE FROM ai_cache`)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "all": true})
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleEmbeddingCache handles /api/embedding-cache
func (r *Router) handleEmbeddingCache(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		// Get cached embedding by hash
		hash := req.URL.Query().Get("hash")
		if hash == "" {
			http.Error(w, "hash parameter required", http.StatusBadRequest)
			return
		}

		var cached struct {
			ID        int       `json:"id"`
			TextHash  string    `json:"text_hash"`
			Text      string    `json:"text"`
			Model     string    `json:"model"`
			CreatedAt time.Time `json:"created_at"`
		}

		err := db.QueryRow(req.Context(), `
			SELECT id, text_hash, text, model, created_at
			FROM embedding_cache
			WHERE text_hash = ?
		`, hash).Scan(&cached.ID, &cached.TextHash, &cached.Text, &cached.Model, &cached.CreatedAt)

		if err != nil {
			http.Error(w, "cache not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(cached)

	case "POST":
		// Store embedding in cache
		var payload struct {
			Text      string    `json:"text"`
			Embedding []float64 `json:"embedding"`
			Model     string    `json:"model,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}

		// Generate hash (simplified - in production use proper vector encoding)
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(payload.Text)))

		// Check if exists
		var existingID int
		err := db.QueryRow(req.Context(), `SELECT id FROM embedding_cache WHERE text_hash = ?`, hash).Scan(&existingID)

		if err == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"cached":  true,
				"id":      existingID,
				"hash":    hash,
				"message": "Cache hit",
			})
			return
		}

		model := payload.Model
		if model == "" {
			model = "emb-embedding-01"
		}

		// MySQL JSON column requires a JSON-encoded string, not a Go slice.
		embeddingJSON, err := json.Marshal(payload.Embedding)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		res, err := db.Exec(req.Context(), `
			INSERT INTO embedding_cache (text_hash, text, embedding, model)
			VALUES (?, ?, ?, ?)
		`, hash, payload.Text, string(embeddingJSON), model)
		if err != nil {
			log.Printf("Embedding cache insert error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id := int(res.LastInsertId())
		var createdAt time.Time
		if err := db.QueryRow(req.Context(),
			`SELECT created_at FROM embedding_cache WHERE id = ?`, id,
		).Scan(&createdAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         id,
			"hash":       hash,
			"cached":     false,
			"created_at": createdAt,
		})

	case "DELETE":
		// Clear cache
		hash := req.URL.Query().Get("hash")
		if hash != "" {
			result, err := db.Exec(req.Context(), `DELETE FROM embedding_cache WHERE text_hash = ?`, hash)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if result.RowsAffected() == 0 {
				writeJSONError(w, http.StatusNotFound, "embedding cache entry not found")
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "hash": hash})
		} else {
			_, err := db.Exec(req.Context(), `DELETE FROM embedding_cache`)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "all": true})
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLearningStats handles GET /api/learning/stats
func (r *Router) handleLearningStats(w http.ResponseWriter, req *http.Request) {
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

	stats, err := r.learningService.GetLearningStats(req.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(stats)
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

// handleLearningSequences handles /api/learning/sequences
func (r *Router) handleLearningSequences(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		// Get action sequences for a goal
		goal := req.URL.Query().Get("goal")
		if goal == "" {
			http.Error(w, "goal parameter required", http.StatusBadRequest)
			return
		}

		sequences, err := r.learningService.GetActionSequences(req.Context(), goal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(sequences)

	case "POST":
		// Add action sequence
		var payload struct {
			Goal       string `json:"goal"`
			Steps      string `json:"steps"`
			Success    bool   `json:"success"`
			DurationMs int    `json:"duration_ms"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.Goal == "" || payload.Steps == "" {
			http.Error(w, "goal and steps are required", http.StatusBadRequest)
			return
		}

		err := r.learningService.AddActionSequence(req.Context(),
			payload.Goal, payload.Steps, payload.Success, payload.DurationMs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"added": true,
			"goal":  payload.Goal,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLearningContext handles /api/learning/context
func (r *Router) handleLearningContext(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		// Get context record
		contextKey := req.URL.Query().Get("key")
		contextValue := req.URL.Query().Get("value")
		if contextKey == "" || contextValue == "" {
			http.Error(w, "key and value parameters required", http.StatusBadRequest)
			return
		}

		record, err := r.learningService.GetContextRecord(req.Context(), contextKey, contextValue)
		if err != nil {
			http.Error(w, "context record not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(record)

	case "POST":
		// Add context record
		var payload struct {
			ContextKey   string `json:"context_key"`
			ContextValue string `json:"context_value"`
			BestAction   string `json:"best_action"`
			Success      bool   `json:"success"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.ContextKey == "" || payload.ContextValue == "" {
			http.Error(w, "context_key and context_value are required", http.StatusBadRequest)
			return
		}

		err := r.learningService.AddContextRecord(req.Context(),
			payload.ContextKey, payload.ContextValue, payload.BestAction, payload.Success)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"added":       true,
			"context_key": payload.ContextKey,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleIssueStatus handles POST /api/issues/status
func (r *Router) handleIssueStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var payload struct {
		ID      int  `json:"id"`
		IsFixed bool `json:"is_fixed"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if payload.ID == 0 {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(req.Context(), `
		UPDATE raw_issues
		SET is_fixed = ?
		WHERE id = ?
	`, boolToInt(payload.IsFixed), payload.ID)

	if err != nil {
		log.Printf("Error updating issue status: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "issue not found")
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       payload.ID,
		"is_fixed": payload.IsFixed,
	})
}

// handleIssueNotes handles POST /api/issues/notes
func (r *Router) handleIssueNotes(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var payload struct {
		ID    int    `json:"id"`
		Notes string `json:"notes"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if payload.ID == 0 {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(req.Context(), `
		UPDATE raw_issues
		SET notes = ?
		WHERE id = ?
	`, payload.Notes, payload.ID)

	if err != nil {
		log.Printf("Error updating issue notes: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "issue not found")
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    payload.ID,
		"notes": payload.Notes,
	})
}

// handleGoalEvents handles /api/goal-events
func (r *Router) handleGoalEvents(w http.ResponseWriter, req *http.Request) {
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "GET":
		// List goal events with optional filters
		query := req.URL.Query()
		goalName := query.Get("goal_name")
		fromDate := query.Get("from_date")
		toDate := query.Get("to_date")
		limit := 20
		if l := query.Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}
		if limit > 100 {
			limit = 100
		}

		sql := `
			SELECT id, goal_name, confidence, page, action, detected_at
			FROM goal_events
			WHERE 1=1
		`
		args := []interface{}{}

		if goalName != "" {
			sql += " AND goal_name LIKE CONCAT('%', ?, '%')"
			args = append(args, goalName)
		}

		if fromDate != "" {
			sql += " AND detected_at >= ?"
			args = append(args, fromDate)
		}

		if toDate != "" {
			sql += " AND detected_at <= ?"
			args = append(args, toDate)
		}

		sql += " ORDER BY detected_at DESC LIMIT ?"
		args = append(args, limit)

		rows, err := db.Query(req.Context(), sql, args...)
		if err != nil {
			log.Printf("Error querying goal events: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var events []models.GoalEvent
		for rows.Next() {
			var event models.GoalEvent
			if err := rows.Scan(&event.ID, &event.GoalName, &event.Confidence, &event.Page, &event.Action, &event.DetectedAt); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}
			events = append(events, event)
		}

		// Return empty array instead of null if no results
		if events == nil {
			events = []models.GoalEvent{}
		}
		json.NewEncoder(w).Encode(events)

	case "POST":
		// Create goal event
		var payload struct {
			GoalName   string  `json:"goal_name"`
			Confidence float64 `json:"confidence"`
			Page       string  `json:"page,omitempty"`
			Action     string  `json:"action,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.GoalName == "" {
			http.Error(w, "goal_name is required", http.StatusBadRequest)
			return
		}

		// Validate confidence if provided
		confidence := payload.Confidence
		if confidence < 0 || confidence > 1 {
			confidence = 0.5 // Default to 0.5 if invalid
		}

		res, err := db.Exec(req.Context(), `
			INSERT INTO goal_events (goal_name, confidence, page, action)
			VALUES (?, ?, ?, ?)
		`, payload.GoalName, confidence, payload.Page, payload.Action)
		if err != nil {
			log.Printf("Error inserting goal event: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id := int(res.LastInsertId())
		var detectedAt time.Time
		if err := db.QueryRow(req.Context(),
			`SELECT detected_at FROM goal_events WHERE id = ?`, id,
		).Scan(&detectedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          id,
			"goal_name":   payload.GoalName,
			"confidence":  confidence,
			"page":        payload.Page,
			"action":      payload.Action,
			"detected_at": detectedAt,
		})

	case "DELETE":
		// Delete goal event by ID
		id := req.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id parameter required", http.StatusBadRequest)
			return
		}

		var intID int
		fmt.Sscanf(id, "%d", &intID)

		result, err := db.Exec(req.Context(), `DELETE FROM goal_events WHERE id = ?`, intID)
		if err != nil {
			log.Printf("Error deleting goal event: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if result.RowsAffected() == 0 {
			writeJSONError(w, http.StatusNotFound, "goal event not found")
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
