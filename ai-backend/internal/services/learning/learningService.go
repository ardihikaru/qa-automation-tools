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