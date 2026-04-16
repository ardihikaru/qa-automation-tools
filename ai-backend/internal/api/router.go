package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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
	// Simplified - return basic stats
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"flows": 0, "issues": 0}`)
}

// handleLearning handles /api/learning
func (r *Router) handleLearning(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"learning_records": 0}`)
}