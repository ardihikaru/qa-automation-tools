package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qa-automation/ai-backend/internal/models"
)

// Service handles learning cache operations
type Service struct {
	db *pgxpool.Pool
}

// NewService creates a new learning service
func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// AddToCache adds a record to the learning cache
func (s *Service) AddToCache(ctx context.Context, intent, action, page string, success bool) error {
	score := 0.5
	if success {
		score = 1.0
	}

	// Check if record exists
	var existingID int
	err := s.db.QueryRow(ctx, `
		SELECT id FROM learning_cache 
		WHERE intent = $1 AND action = $2 AND page = $3
	`, intent, action, page).Scan(&existingID)

	if err == nil {
		// Update existing record
		_, err = s.db.Exec(ctx, `
			UPDATE learning_cache
			SET success_score = CASE 
				WHEN $4 THEN LEAST(success_score + 0.1, 1.0)
				ELSE GREATEST(success_score - 0.1, 0.0)
			END,
			usage_count = usage_count + 1,
			last_success = CASE WHEN $4 THEN NOW() ELSE last_success END,
			updated_at = NOW()
			WHERE id = $5
		`, success, existingID)
		return err
	}

	// Insert new record
	_, err = s.db.Exec(ctx, `
		INSERT INTO learning_cache (intent, action, page, success_score, usage_count, last_success)
		VALUES ($1, $2, $3, $4, 1, CASE WHEN $4 THEN NOW() ELSE NULL END)
	`, intent, action, page, score)

	return err
}

// Search searches the learning cache
func (s *Service) Search(ctx context.Context, intent string, limit int) ([]models.LearningRecord, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, intent, action, page, success_score, usage_count, last_success, decay_factor, updated_at
		FROM learning_cache
		WHERE intent ILIKE '%' || $1 || '%'
		ORDER BY success_score DESC, usage_count DESC
		LIMIT $2
	`, intent, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.LearningRecord
	for rows.Next() {
		var record models.LearningRecord
		if err := rows.Scan(&record.ID, &record.Intent, &record.Action, &record.Page, 
			&record.SuccessScore, &record.UsageCount, &record.LastSuccess, &record.DecayFactor, &record.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, record)
	}

	return results, nil
}

// GetBestAction gets the best action for a given intent and page
func (s *Service) GetBestAction(ctx context.Context, intent, page string) (*models.LearningRecord, error) {
	var record models.LearningRecord
	err := s.db.QueryRow(ctx, `
		SELECT id, intent, action, page, success_score, usage_count, last_success, decay_factor, updated_at
		FROM learning_cache
		WHERE page = $1
		ORDER BY success_score DESC, usage_count DESC
		LIMIT 1
	`, page).Scan(&record.ID, &record.Intent, &record.Action, &record.Page, 
		&record.SuccessScore, &record.UsageCount, &record.LastSuccess, &record.DecayFactor, &record.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return &record, nil
}

// ApplyDecay applies decay to learning records
func (s *Service) ApplyDecay(ctx context.Context) error {
	// Apply 5% decay to records older than 7 days
	_, err := s.db.Exec(ctx, `
		UPDATE learning_cache
		SET success_score = success_score * 0.95
		WHERE updated_at < NOW() - INTERVAL '7 days'
		AND success_score > 0.1
	`)

	return err
}

// GetStats gets learning statistics
func (s *Service) GetStats(ctx context.Context) (map[string]interface{}, error) {
	var total, avgScore, avgUsage int
	var highSuccess int

	err := s.db.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			AVG(success_score)::int,
			AVG(usage_count)::int,
			COUNT(*) FILTER (WHERE success_score > 0.8)
		FROM learning_cache
	`).Scan(&total, &avgScore, &avgUsage, &highSuccess)

	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_records":    total,
		"avg_success":     avgScore,
		"avg_usage":      avgUsage,
		"high_success":   highSuccess,
	}, nil
}

// ClearOldRecords clears records older than specified days
func (s *Service) ClearOldRecords(ctx context.Context, days int) error {
	if days <= 0 {
		days = 30
	}

	_, err := s.db.Exec(ctx, `
		DELETE FROM learning_cache
		WHERE updated_at < NOW() - INTERVAL '1 day' * $1
		AND usage_count < 3
	`, days)

	return err
}

// ----- Navigation Edge Methods -----

// AddNavigationEdge adds or updates a navigation edge
func (s *Service) AddNavigationEdge(ctx context.Context, sourcePage, targetPage, action string, success bool, durationMs int) error {
	// Check if edge exists
	var existingID int
	err := s.db.QueryRow(ctx, `
		SELECT id FROM navigation_edges 
		WHERE source_page = $1 AND target_page = $2 AND action = $3
	`, sourcePage, targetPage, action).Scan(&existingID)

	if err == nil {
		// Update existing edge
		_, err = s.db.Exec(ctx, `
			UPDATE navigation_edges
			SET success_count = success_count + $4,
				failure_count = failure_count + $5,
				success_rate = success_count::decimal / (success_count + failure_count)::decimal,
				avg_duration_ms = (avg_duration_ms * (success_count + failure_count - 1) + $6) / (success_count + failure_count),
				last_used = NOW(),
				updated_at = NOW()
			WHERE id = $7
		`, map[bool]int{true: 1, false: 0}[success], map[bool]int{true: 0, false: 1}[success], durationMs, existingID)
		return err
	}

	// Insert new edge
	successCount := 0
	failureCount := 0
	if success {
		successCount = 1
	} else {
		failureCount = 1
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO navigation_edges (source_page, target_page, action, success_count, failure_count, success_rate, avg_duration_ms, last_used)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`, sourcePage, targetPage, action, successCount, failureCount, 0.5, durationMs)

	return err
}

// GetNavigationEdges gets navigation edges from a source page
func (s *Service) GetNavigationEdges(ctx context.Context, sourcePage string) ([]models.NavigationEdge, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, source_page, target_page, action, success_count, failure_count, 
		       success_rate, avg_duration_ms, last_used, created_at, updated_at
		FROM navigation_edges
		WHERE source_page = $1
		ORDER BY success_rate DESC, success_count DESC
	`, sourcePage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []models.NavigationEdge
	for rows.Next() {
		var edge models.NavigationEdge
		if err := rows.Scan(&edge.ID, &edge.SourcePage, &edge.TargetPage, &edge.Action,
			&edge.SuccessCount, &edge.FailureCount, &edge.SuccessRate, &edge.AvgDuration,
			&edge.LastUsed, &edge.CreatedAt, &edge.UpdatedAt); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}

	if edges == nil {
		edges = []models.NavigationEdge{}
	}
	return edges, nil
}

// GetBestNextPage gets the best next page for exploration
func (s *Service) GetBestNextPage(ctx context.Context, currentPage string) (*models.NavigationEdge, error) {
	var edge models.NavigationEdge
	err := s.db.QueryRow(ctx, `
		SELECT id, source_page, target_page, action, success_count, failure_count, 
		       success_rate, avg_duration_ms, last_used, created_at, updated_at
		FROM navigation_edges
		WHERE source_page = $1
		ORDER BY success_rate DESC, success_count DESC
		LIMIT 1
	`, currentPage).Scan(&edge.ID, &edge.SourcePage, &edge.TargetPage, &edge.Action,
		&edge.SuccessCount, &edge.FailureCount, &edge.SuccessRate, &edge.AvgDuration,
		&edge.LastUsed, &edge.CreatedAt, &edge.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return &edge, nil
}

// ----- Action Sequence Methods -----

// AddActionSequence adds or updates an action sequence
func (s *Service) AddActionSequence(ctx context.Context, goal string, steps string, success bool, durationMs int) error {
	var existingID int
	err := s.db.QueryRow(ctx, `
		SELECT id FROM action_sequences 
		WHERE goal = $1 AND steps = $2
	`, goal, steps).Scan(&existingID)

	if err == nil {
		// Update existing sequence
		_, err = s.db.Exec(ctx, `
			UPDATE action_sequences
			SET success_count = success_count + $3,
				failure_count = failure_count + $4,
				avg_duration_ms = (avg_duration_ms * (success_count + failure_count - 1) + $5) / (success_count + failure_count),
				last_used = NOW(),
				is_valid = $6,
				updated_at = NOW()
			WHERE id = $7
		`, map[bool]int{true: 1, false: 0}[success], map[bool]int{true: 0, false: 1}[success], durationMs, success, existingID)
		return err
	}

	// Insert new sequence
	successCount := 0
	failureCount := 0
	if success {
		successCount = 1
	} else {
		failureCount = 1
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO action_sequences (goal, steps, success_count, failure_count, avg_duration_ms, last_used)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, goal, steps, successCount, failureCount, durationMs)

	return err
}

// GetActionSequences gets action sequences for a goal
func (s *Service) GetActionSequences(ctx context.Context, goal string) ([]models.ActionSequence, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, goal, steps, success_count, failure_count, 
		       avg_duration_ms, last_used, is_valid, created_at, updated_at
		FROM action_sequences
		WHERE goal = $1 AND is_valid = true
		ORDER BY success_count DESC, avg_duration_ms ASC
	`, goal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sequences []models.ActionSequence
	for rows.Next() {
		var seq models.ActionSequence
		if err := rows.Scan(&seq.ID, &seq.Goal, &seq.Steps, &seq.SuccessCount, &seq.FailureCount,
			&seq.AvgDuration, &seq.LastUsed, &seq.IsValid, &seq.CreatedAt, &seq.UpdatedAt); err != nil {
			return nil, err
		}
		sequences = append(sequences, seq)
	}

	if sequences == nil {
		sequences = []models.ActionSequence{}
	}
	return sequences, nil
}

// GetBestActionSequence gets the best action sequence for a goal
func (s *Service) GetBestActionSequence(ctx context.Context, goal string) (*models.ActionSequence, error) {
	var seq models.ActionSequence
	err := s.db.QueryRow(ctx, `
		SELECT id, goal, steps, success_count, failure_count, 
		       avg_duration_ms, last_used, is_valid, created_at, updated_at
		FROM action_sequences
		WHERE goal = $1 AND is_valid = true
		ORDER BY success_count DESC, avg_duration_ms ASC
		LIMIT 1
	`, goal).Scan(&seq.ID, &seq.Goal, &seq.Steps, &seq.SuccessCount, &seq.FailureCount,
		&seq.AvgDuration, &seq.LastUsed, &seq.IsValid, &seq.CreatedAt, &seq.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return &seq, nil
}

// ----- Context Cache Methods -----

// AddContextRecord adds or updates a context record
func (s *Service) AddContextRecord(ctx context.Context, contextKey, contextValue, bestAction string, success bool) error {
	var existingID int
	err := s.db.QueryRow(ctx, `
		SELECT id FROM context_cache 
		WHERE context_key = $1 AND context_value = $2
	`, contextKey, contextValue).Scan(&existingID)

	if err == nil {
		// Update existing record
		_, err = s.db.Exec(ctx, `
			UPDATE context_cache
			SET success_score = CASE 
				WHEN $3 THEN LEAST(success_score + 0.1, 1.0)
				ELSE GREATEST(success_score - 0.1, 0.0)
			END,
			usage_count = usage_count + 1,
			last_success = CASE WHEN $3 THEN NOW() ELSE last_success END,
			updated_at = NOW()
			WHERE id = $4
		`, success, existingID)
		return err
	}

	// Insert new record
	score := 0.5
	if success {
		score = 1.0
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO context_cache (context_key, context_value, best_action, success_score, usage_count, last_success)
		VALUES ($1, $2, $3, $4, 1, CASE WHEN $5 THEN NOW() ELSE NULL END)
	`, contextKey, contextValue, bestAction, score, success)

	return err
}

// GetContextRecord gets a context record
func (s *Service) GetContextRecord(ctx context.Context, contextKey, contextValue string) (*models.ContextRecord, error) {
	var record models.ContextRecord
	err := s.db.QueryRow(ctx, `
		SELECT id, context_key, context_value, best_action, success_score, 
		       usage_count, last_success, created_at, updated_at
		FROM context_cache
		WHERE context_key = $1 AND context_value = $2
		ORDER BY success_score DESC, usage_count DESC
		LIMIT 1
	`, contextKey, contextValue).Scan(&record.ID, &record.ContextKey, &record.ContextValue, &record.BestAction,
		&record.SuccessScore, &record.UsageCount, &record.LastSuccess, &record.CreatedAt, &record.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return &record, nil
}

// GetLearningStats gets overall learning statistics
func (s *Service) GetLearningStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Learning cache stats
	var cacheTotal, cacheAvgScore, cacheAvgUsage int
	err := s.db.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			AVG(success_score)::int,
			AVG(usage_count)::int
		FROM learning_cache
	`).Scan(&cacheTotal, &cacheAvgScore, &cacheAvgUsage)
	if err == nil {
		stats["learning_cache"] = map[string]interface{}{
			"total_records": cacheTotal,
			"avg_success":   cacheAvgScore,
			"avg_usage":    cacheAvgUsage,
		}
	}

	// Navigation edges stats
	var edgesTotal, edgesAvgSuccess int
	err = s.db.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			AVG(success_rate)::int
		FROM navigation_edges
	`).Scan(&edgesTotal, &edgesAvgSuccess)
	if err == nil {
		stats["navigation_edges"] = map[string]interface{}{
			"total_edges":  edgesTotal,
			"avg_success":  edgesAvgSuccess,
		}
	}

	// Action sequences stats
	var seqTotal, seqAvgSuccess int
	err = s.db.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			AVG(success_count)::int
		FROM action_sequences
	`).Scan(&seqTotal, &seqAvgSuccess)
	if err == nil {
		stats["action_sequences"] = map[string]interface{}{
			"total_sequences": seqTotal,
			"avg_success":   seqAvgSuccess,
		}
	}

	return stats, nil
}

// HashInput creates a hash for input deduplication
func HashInput(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// HashToJSON creates a hash from a struct
func HashToJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return HashInput(string(data))
}

// TimeTrack tracks execution time
func TimeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s took %s\n", name, elapsed)
}