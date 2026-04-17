package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/learning"
	"github.com/qa-automation/ai-backend/internal/services/skills"
)

// Router handles HTTP requests
type Router struct {
	db              *db.DB
	aiService       *ai.Service
	learningService *learning.Service
	skillsService   *skills.Service
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
	}
}

// getDB returns the database pool, or nil if not available
func (r *Router) getDB() *pgxpool.Pool {
	if r.db == nil {
		return nil
	}
	return r.db.Pool
}

// ServeHTTP handles HTTP requests
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle OPTIONS
	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Health check - always available
	if req.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		minimaxStatus := "not_configured"
		if r.aiService != nil && r.aiService.APIKey != "" {
			minimaxStatus = "configured"
		}

		if r.db != nil {
			fmt.Fprintf(w, `{"status":"ok","database":"connected","minimax":"%s"}`, minimaxStatus)
		} else {
			fmt.Fprintf(w, `{"status":"ok","database":"disconnected","minimax":"%s"}`, minimaxStatus)
		}
		return
	}

	// Route request
	switch req.URL.Path {
	// Flow endpoints
	case "/api/flows":
		r.handleFlows(w, req)

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

		err := db.QueryRow(req.Context(), `
			INSERT INTO flows (role_id, name, description)
			VALUES ($1, $2, $3)
			RETURNING id, created_at, updated_at
		`, flow.RoleID, flow.Name, flow.Description).Scan(&flow.ID, &flow.CreatedAt, &flow.UpdatedAt)

		if err != nil {
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

		err := db.QueryRow(req.Context(), `
			INSERT INTO roles (name, description)
			VALUES ($1, $2)
			RETURNING id, created_at
		`, role.Name, role.Description).Scan(&role.ID, &role.CreatedAt)

		if err != nil {
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

		_, err := db.Exec(req.Context(), `DELETE FROM roles WHERE id = $1`, intID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

		// Base SQL query
		sql := `SELECT i.id, i.page, i.action, i.error_type, i.error_text, 
			       i.category, i.severity, i.is_fixed, i.notes, i.created_at
			FROM raw_issues i
			WHERE 1=1`
		args := []interface{}{}
		argNum := 1

		// Filter by fixed status
		if filterFixed != "" {
			sql += fmt.Sprintf(" AND i.is_fixed = $%d", argNum)
			args = append(args, filterFixed == "true")
			argNum++
		}

		// Filter by category
		if category != "" && category != "all" {
			sql += fmt.Sprintf(" AND i.category = $%d", argNum)
			args = append(args, category)
			argNum++
		}

		// Filter by target (frontend/backend via suggestions)
		if target != "" {
			sql += fmt.Sprintf(" AND i.id IN (SELECT issue_id FROM issue_suggestions WHERE target = $%d)", argNum)
			args = append(args, target)
			argNum++
		}

		// Search by page, action, or error text
		if search != "" {
			sql += fmt.Sprintf(" AND (i.page ILIKE '%%' || $%d || '%%' OR i.action ILIKE '%%' || $%d || '%%' OR i.error_text ILIKE '%%' || $%d || '%%')", argNum, argNum, argNum)
			args = append(args, search)
			argNum++
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
		sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argNum, argNum+1)
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
			if err := rows.Scan(
				&issue.ID, &issue.Page, &issue.Action, &issue.ErrorType, &issue.ErrorText,
				&issue.Category, &issue.Severity, &issue.IsFixed, &issue.Notes, &issue.CreatedAt,
			); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}

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
		countArgNum := 1

		if filterFixed != "" {
			countSql += fmt.Sprintf(" AND is_fixed = $%d", countArgNum)
			countArgs = append(countArgs, filterFixed == "true")
			countArgNum++
		}
		if category != "" && category != "all" {
			countSql += fmt.Sprintf(" AND category = $%d", countArgNum)
			countArgs = append(countArgs, category)
			countArgNum++
		}
		if search != "" {
			countSql += fmt.Sprintf(" AND (page ILIKE '%%' || $%d || '%%' OR action ILIKE '%%' || $%d || '%%' OR error_text ILIKE '%%' || $%d || '%%')", countArgNum, countArgNum, countArgNum)
			countArgs = append(countArgs, search)
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

		// Validate required fields - page and action are required
		if issue.Page == "" {
			http.Error(w, "page is required", http.StatusBadRequest)
			return
		}
		if issue.Action == "" {
			http.Error(w, "action is required", http.StatusBadRequest)
			return
		}

		err := db.QueryRow(req.Context(), `
			INSERT INTO raw_issues (page, action, error_type, error_text, category, severity)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at
		`, issue.Page, issue.Action, issue.ErrorType, issue.ErrorText, issue.Category, issue.Severity).Scan(&issue.ID, &issue.CreatedAt)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Generate category using AI only when service is available
		if issue.Category == "" && issue.ErrorText != "" && r.aiService != nil {
			category, _ := r.aiService.ClassifyIssue(issue.ErrorText)
			issue.Category = category
			db.Exec(req.Context(), `UPDATE raw_issues SET category = $1 WHERE id = $2`, category, issue.ID)
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
		WHERE issue_id = $1
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

	rows, err := db.Query(req.Context(), `
		SELECT id, name, description, check_type, check_query, severity, is_builtin, is_active, created_at
		FROM skills
		WHERE is_active = true
		ORDER BY is_builtin DESC, name
	`)
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

	json.NewEncoder(w).Encode(skills)
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
		// Query skill logs with filters
		query := req.URL.Query()
		skillID := query.Get("skill_id")
		page := query.Get("page")
		passed := query.Get("passed")
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
			SELECT sl.id, sl.skill_id, sl.page, sl.action, sl.result, sl.is_passed, sl.error_message, sl.created_at,
			       COALESCE(s.name, 'Unknown') as skill_name
			FROM skill_logs sl
			LEFT JOIN skills s ON sl.skill_id = s.id
			WHERE 1=1
		`
		args := []interface{}{}
		argNum := 1

		if skillID != "" {
			sql += fmt.Sprintf(" AND sl.skill_id = $%d", argNum)
			args = append(args, skillID)
			argNum++
		}

		if page != "" {
			sql += fmt.Sprintf(" AND sl.page = $%d", argNum)
			args = append(args, page)
			argNum++
		}

		if passed != "" {
			sql += fmt.Sprintf(" AND sl.is_passed = $%d", argNum)
			args = append(args, passed == "true")
			argNum++
		}

		if fromDate != "" {
			sql += fmt.Sprintf(" AND sl.created_at >= $%d", argNum)
			args = append(args, fromDate)
			argNum++
		}

		if toDate != "" {
			sql += fmt.Sprintf(" AND sl.created_at <= $%d", argNum)
			args = append(args, toDate)
			argNum++
		}

		sql += fmt.Sprintf(" ORDER BY sl.created_at DESC LIMIT $%d", argNum)
		args = append(args, limit)

		rows, err := db.Query(req.Context(), sql, args...)
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
			if err := rows.Scan(&entry.ID, &entry.SkillID, &entry.Page, &entry.Action, &entry.Result,
				&entry.IsPassed, &entry.ErrorMsg, &entry.CreatedAt, &entry.SkillName); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}
			skillLogs = append(skillLogs, entry)
		}

		if skillLogs == nil {
			skillLogs = []SkillLogWithName{}
		}
		json.NewEncoder(w).Encode(skillLogs)

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

		var id int
		var createdAt time.Time
		err := db.QueryRow(req.Context(), `
			INSERT INTO skill_logs (skill_id, page, action, result, is_passed, error_message)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at
		`, payload.SkillID, payload.Page, payload.Action, payload.Result, payload.IsPassed, payload.ErrorMsg).Scan(&id, &createdAt)

		if err != nil {
			log.Printf("Error inserting skill log: %v", err)
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

	// Get aggregated stats for skills
	type SkillStats struct {
		SkillID    int        `json:"skill_id"`
		SkillName  string     `json:"skill_name"`
		TotalRuns  int        `json:"total_runs"`
		PassedRuns int        `json:"passed_runs"`
		FailedRuns int        `json:"failed_runs"`
		PassRate   float64    `json:"pass_rate"`
		LastRun    *time.Time `json:"last_run,omitempty"`
	}

	var stats []SkillStats

	rows, err := db.Query(req.Context(), `
		SELECT sl.skill_id, 
		       COALESCE(s.name, 'Unknown') as skill_name,
		       COUNT(*) as total_runs,
		       COUNT(*) FILTER (WHERE sl.is_passed = true) as passed_runs,
		       COUNT(*) FILTER (WHERE sl.is_passed = false) as failed_runs,
		       CASE WHEN COUNT(*) > 0 THEN COUNT(*) FILTER (WHERE sl.is_passed = true) * 1.0 / COUNT(*) ELSE 0 END as pass_rate,
		       MAX(sl.created_at) as last_run
		FROM skill_logs sl
		LEFT JOIN skills s ON sl.skill_id = s.id
		GROUP BY sl.skill_id, s.name
		ORDER BY total_runs DESC
	`)
	if err != nil {
		log.Printf("Error querying skill stats: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

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
	json.NewEncoder(w).Encode(stats)
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

	// Search in learning cache - use text matching since vector extension not available in Neon
	rows, err := db.Query(req.Context(), `
		SELECT intent, action, page, success_score, usage_count
		FROM learning_cache
		WHERE intent ILIKE '%' || $1 || '%'
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
			COUNT(*) FILTER (WHERE cache_hit = true),
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
		SELECT DATE(created_at) as date, SUM(cost_estimate) as cost
		FROM request_logs
		WHERE created_at > NOW() - INTERVAL '30 days'
		GROUP BY DATE(created_at)
		ORDER BY date
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
			COUNT(*) FILTER (WHERE cache_hit = true),
			COUNT(*) FILTER (WHERE cache_hit = false)
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

	var docs []DocInfo

	// Walk through product-documentations directory
	productDir := "../product-documentations"
	entries, err := os.ReadDir(productDir)
	if err != nil {
		log.Printf("Error reading product-documentations: %v", err)
		json.NewEncoder(w).Encode([]DocInfo{})
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

	if docs == nil {
		docs = []DocInfo{}
	}
	json.NewEncoder(w).Encode(docs)
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
		argNum := 1

		if intent != "" {
			sql += fmt.Sprintf(" AND intent ILIKE '%%' || $%d || '%%'", argNum)
			args = append(args, intent)
			argNum++
		}

		if page != "" {
			sql += fmt.Sprintf(" AND page = $%d", argNum)
			args = append(args, page)
			argNum++
		}

		sql += fmt.Sprintf(" ORDER BY success_score DESC, usage_count DESC LIMIT $%d", argNum)
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
			WHERE intent = $1 AND action = $2 AND page = $3
		`, payload.Intent, payload.Action, payload.Page).Scan(&existingID)

		if err == nil {
			// Update existing record
			successBool := payload.Success
			_, err = db.Exec(req.Context(), `
				UPDATE learning_cache
				SET success_score = CASE 
					WHEN $4::bool THEN LEAST(success_score + 0.1, 1.0)
					ELSE GREATEST(success_score - 0.1, 0.0)
				END,
				usage_count = usage_count + 1,
				last_success = CASE WHEN $4::bool THEN NOW() ELSE last_success END,
				updated_at = NOW()
				WHERE id = $5
			`, successBool, existingID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      existingID,
				"updated": true,
			})
		} else {
			// Insert new record
			score := 0.5
			if payload.Success {
				score = 1.0
			}
			var id int
			var createdAt, updatedAt time.Time
			err := db.QueryRow(req.Context(), `
				INSERT INTO learning_cache (intent, action, page, success_score, usage_count, last_success)
				VALUES ($1, $2, $3, $4, 1, CASE WHEN $5 THEN NOW() ELSE NULL END)
				RETURNING id, created_at, updated_at
			`, payload.Intent, payload.Action, payload.Page, score, payload.Success).Scan(&id, &createdAt, &updatedAt)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         id,
				"intent":     payload.Intent,
				"action":     payload.Action,
				"page":       payload.Page,
				"success":    payload.Success,
				"created_at": createdAt,
			})
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

		_, err := db.Exec(req.Context(), `DELETE FROM learning_cache WHERE id = $1`, intID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			WHERE input_hash = $1
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
		err := db.QueryRow(req.Context(), `SELECT id FROM ai_cache WHERE input_hash = $1`, hash).Scan(&existingID)

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

		var id int
		var createdAt time.Time
		err = db.QueryRow(req.Context(), `
			INSERT INTO ai_cache (input_hash, input_text, output_text, model, cost_estimate)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at
		`, hash, payload.InputText, payload.OutputText, model, payload.Cost).Scan(&id, &createdAt)

		if err != nil {
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
			_, err := db.Exec(req.Context(), `DELETE FROM ai_cache WHERE input_hash = $1`, hash)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
			WHERE text_hash = $1
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
		err := db.QueryRow(req.Context(), `SELECT id FROM embedding_cache WHERE text_hash = $1`, hash).Scan(&existingID)

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

		var id int
		var createdAt time.Time
		// Store embedding as JSONB since vector extension is not available in Neon
		err = db.QueryRow(req.Context(), `
			INSERT INTO embedding_cache (text_hash, text, embedding, model)
			VALUES ($1, $2, $3, $4)
			RETURNING id, created_at
		`, hash, payload.Text, payload.Embedding, model).Scan(&id, &createdAt)

		if err != nil {
			log.Printf("Embedding cache insert error: %v", err)
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
			_, err := db.Exec(req.Context(), `DELETE FROM embedding_cache WHERE text_hash = $1`, hash)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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

	_, err := db.Exec(req.Context(), `
		UPDATE raw_issues
		SET is_fixed = $1
		WHERE id = $2
	`, payload.IsFixed, payload.ID)

	if err != nil {
		log.Printf("Error updating issue status: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

	_, err := db.Exec(req.Context(), `
		UPDATE raw_issues
		SET notes = $1
		WHERE id = $2
	`, payload.Notes, payload.ID)

	if err != nil {
		log.Printf("Error updating issue notes: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		argNum := 1

		if goalName != "" {
			sql += fmt.Sprintf(" AND goal_name ILIKE '%%' || $%d || '%%'", argNum)
			args = append(args, goalName)
			argNum++
		}

		if fromDate != "" {
			sql += fmt.Sprintf(" AND detected_at >= $%d", argNum)
			args = append(args, fromDate)
			argNum++
		}

		if toDate != "" {
			sql += fmt.Sprintf(" AND detected_at <= $%d", argNum)
			args = append(args, toDate)
			argNum++
		}

		sql += fmt.Sprintf(" ORDER BY detected_at DESC LIMIT $%d", argNum)
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

		var id int
		var detectedAt time.Time
		err := db.QueryRow(req.Context(), `
			INSERT INTO goal_events (goal_name, confidence, page, action)
			VALUES ($1, $2, $3, $4)
			RETURNING id, detected_at
		`, payload.GoalName, confidence, payload.Page, payload.Action).Scan(&id, &detectedAt)

		if err != nil {
			log.Printf("Error inserting goal event: %v", err)
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

		_, err := db.Exec(req.Context(), `DELETE FROM goal_events WHERE id = $1`, intID)
		if err != nil {
			log.Printf("Error deleting goal event: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
