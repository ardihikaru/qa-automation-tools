package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		if r.db != nil {
			fmt.Fprint(w, `{"status": "ok", "database": "connected"}`)
		} else {
			fmt.Fprint(w, `{"status": "ok", "database": "disconnected"}`)
		}
		return
	}

	// Route request
	switch req.URL.Path {
	// Flow endpoints
	case "/api/flows":
		r.handleFlows(w, req)

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

// handleIssues handles /api/issues
func (r *Router) handleIssues(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		// List issues with optional filters
		w.Header().Set("Content-Type", "application/json")
		
		query := req.URL.Query()
		filterFixed := query.Get("is_fixed")
		target := query.Get("target")

		sql := `
			SELECT i.id, i.page, i.action, i.error_type, i.error_text, 
			       i.category, i.severity, i.is_fixed, i.notes, i.created_at,
			       s.id, s.suggestion, s.confidence, s.target, s.file_path, s.diff
			FROM raw_issues i
			LEFT JOIN issue_suggestions s ON i.id = s.issue_id
			WHERE 1=1
		`
		args := []interface{}{}
		argNum := 1

		if filterFixed != "" {
			sql += fmt.Sprintf(" AND i.is_fixed = $%d", argNum)
			args = append(args, filterFixed == "true")
			argNum++
		}

		if target != "" {
			sql += fmt.Sprintf(" AND s.target = $%d", argNum)
			args = append(args, target)
			argNum++
		}

		sql += " ORDER BY i.created_at DESC"

		rows, err := r.db.Query(req.Context(), sql, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type IssueWithSuggestions struct {
			models.RawIssue
			Suggestions []models.IssueSuggestion `json:"suggestions"`
		}

		issuesMap := make(map[int]*IssueWithSuggestions)

		for rows.Next() {
			var issue models.RawIssue
			var sug models.IssueSuggestion
			var sugID, sugIssueID *int

			err := rows.Scan(
				&issue.ID, &issue.Page, &issue.Action, &issue.ErrorType, &issue.ErrorText,
				&issue.Category, &issue.Severity, &issue.IsFixed, &issue.Notes, &issue.CreatedAt,
				&sugID, &sug.Suggestion, &sug.Confidence, &sug.Target, &sug.FilePath, &sug.Diff,
			)
			if err != nil {
				log.Println("Scan error:", err)
				continue
			}

			if _, ok := issuesMap[issue.ID]; !ok {
				issuesMap[issue.ID] = &IssueWithSuggestions{
					RawIssue:   issue,
					Suggestions: []models.IssueSuggestion{},
				}
			}

			if sugID != nil {
				if sugIssueID != nil && *sugIssueID == issue.ID {
					sug.IssueID = issue.ID
					issuesMap[issue.ID].Suggestions = append(issuesMap[issue.ID].Suggestions, sug)
				}
			}
		}

		var result []IssueWithSuggestions
		for _, issue := range issuesMap {
			result = append(result, *issue)
		}

		// Return empty array instead of null if no results
		if result == nil {
			result = []IssueWithSuggestions{}
		}

		json.NewEncoder(w).Encode(result)

	case "POST":
		// Create issue
		var issue models.RawIssue
		if err := json.NewDecoder(req.Body).Decode(&issue); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err := r.db.QueryRow(req.Context(), `
			INSERT INTO raw_issues (page, action, error_type, error_text, category, severity)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at
		`, issue.Page, issue.Action, issue.ErrorType, issue.ErrorText, issue.Category, issue.Severity).Scan(&issue.ID, &issue.CreatedAt)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Generate suggestions using AI
		if issue.Category == "" && issue.ErrorText != "" {
			category, _ := r.aiService.ClassifyIssue(issue.ErrorText)
			issue.Category = category
			r.db.Exec(req.Context(), `UPDATE raw_issues SET category = $1 WHERE id = $2`, category, issue.ID)
		}

		// Set proper headers and status before encoding
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(issue)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSkills handles /api/skills
func (r *Router) handleSkills(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := r.db.Query(req.Context(), `
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

	// Check if AI service is available
	if r.aiService == nil {
		http.Error(w, "AI service not available", http.StatusServiceUnavailable)
		return
	}

	// Generate embedding for query
	embedding, err := r.aiService.GenerateEmbedding(query)
	if err != nil {
		log.Printf("Search: GenerateEmbedding error: %v", err)
		http.Error(w, fmt.Sprintf("AI service error: %v", err), http.StatusInternalServerError)
		return
	}

	// Check if database is available
	db := r.getDB()
	if db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	// Search in learning cache
	rows, err := db.Query(req.Context(), `
		SELECT intent, action, page, success_score, usage_count
		FROM learning_cache
		ORDER BY embedding <-> $1::vector
		LIMIT 10
	`, embedding)
	if err != nil {
		log.Printf("Search: Database query error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type SearchResult struct {
		Intent      string  `json:"intent"`
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

	err := r.db.QueryRow(req.Context(), `
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

	rows, err := r.db.Query(req.Context(), `
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
		Hit int `json:"hit"`
		Miss int `json:"miss"`
	}

	err := r.db.QueryRow(req.Context(), `
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

	rows, err := r.db.Query(req.Context(), `
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
	// Simplified - return list of docs
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"docs": []}`)
}

// handleVideos handles /api/videos
func (r *Router) handleVideos(w http.ResponseWriter, req *http.Request) {
	// Simplified - return list of videos
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"videos": []}`)
}

// handleAnalytics handles /api/analytics
func (r *Router) handleAnalytics(w http.ResponseWriter, req *http.Request) {
	// Return basic stats
	w.Header().Set("Content-Type", "application/json")

	type AnalyticsResponse struct {
		Flows  int `json:"flows"`
		Issues int `json:"issues"`
	}

	response := AnalyticsResponse{
		Flows:  0,
		Issues: 0,
	}

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
			Intent      string  `json:"intent"`
			Action     string  `json:"action"`
			Page       string  `json:"page"`
			Success    bool    `json:"success"`
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
			"id":     intID,
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
				"cached":   true,
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
			TextHash string    `json:"text_hash"`
			Text     string    `json:"text"`
			Model    string    `json:"model"`
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
			Model     string   `json:"model,omitempty"`
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
				"cached":   true,
				"id":      existingID,
				"hash":    hash,
				"message": "Cache hit",
			})
			return
		}

		// Convert embedding to string for storage (vector type)
		var embeddingStr string
		if len(payload.Embedding) > 0 {
			embeddingStr = fmt.Sprintf("[%s]", strings.Join(func() []string {
				s := make([]string, len(payload.Embedding))
				for i, v := range payload.Embedding {
					s[i] = fmt.Sprintf("%.6f", v)
				}
				return s
			}(), ","))
		}

		model := payload.Model
		if model == "" {
			model = "emb-embedding-01"
		}

		var id int
		var createdAt time.Time
		err = db.QueryRow(req.Context(), `
			INSERT INTO embedding_cache (text_hash, text, embedding, model)
			VALUES ($1, $2, $3::vector, $4)
			RETURNING id, created_at
		`, hash, payload.Text, embeddingStr, model).Scan(&id, &createdAt)

		if err != nil {
			log.Printf("Embedding cache insert error: %v", err)
			// Fallback: insert without vector
			err = db.QueryRow(req.Context(), `
				INSERT INTO embedding_cache (text_hash, text, model)
				VALUES ($1, $2, $3)
				RETURNING id, created_at
			`, hash, payload.Text, model).Scan(&id, &createdAt)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
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
			Action    string `json:"action"`
			Success  bool   `json:"success"`
			DurationMs int  `json:"duration_ms"`
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
			"added": true,
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
			Goal      string `json:"goal"`
			Steps     string `json:"steps"`
			Success  bool   `json:"success"`
			DurationMs int  `json:"duration_ms"`
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
			"goal": payload.Goal,
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
			Success     bool   `json:"success"`
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
			"added": true,
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

	w.Header().Set("Content-Type", "application/json")

	var payload struct {
		ID        int  `json:"id"`
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

	_, err := r.db.Exec(req.Context(), `
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
		"id":        payload.ID,
		"is_fixed": payload.IsFixed,
	})
}

// handleIssueNotes handles POST /api/issues/notes
func (r *Router) handleIssueNotes(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	_, err := r.db.Exec(req.Context(), `
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