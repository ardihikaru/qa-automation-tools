package skills

import (
	"context"
	"fmt"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/ai"
)

// Service handles QA skills operations against MySQL. Uses *db.DB (the
// context-first wrapper around *sql.DB).
type Service struct {
	db        *db.DB
	aiService *ai.Service
}

// NewService creates a new skills service.
func NewService(database *db.DB, aiService *ai.Service) *Service {
	return &Service{
		db:        database,
		aiService: aiService,
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// BuiltinSkills returns the built-in QA skills seeded on startup.
func BuiltinSkills() []models.Skill {
	return []models.Skill{
		{
			Name:        "max_length",
			Description: "Check if textarea has proper maxlength attribute",
			CheckType:   "validation",
			CheckQuery:  "textarea[maxlength], input[maxlength]",
			Severity:    "high",
			IsBuiltin:   true,
			IsActive:    true,
		},
		{
			Name:        "required_field",
			Description: "Check if required fields can be submitted empty",
			CheckType:   "validation",
			CheckQuery:  "input[required], textarea[required], select[required]",
			Severity:    "high",
			IsBuiltin:   true,
			IsActive:    true,
		},
		{
			Name:        "data_persistence",
			Description: "Check if form data persists after page reload",
			CheckType:   "data",
			CheckQuery:  "localStorage, sessionStorage",
			Severity:    "medium",
			IsBuiltin:   true,
			IsActive:    true,
		},
		{
			Name:        "multi_tab_save",
			Description: "Check if tab data is lost when switching without save",
			CheckType:   "validation",
			CheckQuery:  ".tab-content, [data-tab]",
			Severity:    "medium",
			IsBuiltin:   true,
			IsActive:    true,
		},
	}
}

// EnsureBuiltinSkills makes sure all builtin skills exist in the database.
// It uses INSERT IGNORE, so existing rows (matched on the unique name) are kept.
func (s *Service) EnsureBuiltinSkills(ctx context.Context) error {
	skills := BuiltinSkills()

	for _, skill := range skills {
		_, err := s.db.Exec(ctx, `
			INSERT IGNORE INTO skills (name, description, check_type, check_query, severity, is_builtin, is_active)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, skill.Name, skill.Description, skill.CheckType, skill.CheckQuery, skill.Severity, boolToInt(skill.IsBuiltin), boolToInt(skill.IsActive))

		if err != nil {
			return fmt.Errorf("failed to insert skill %s: %w", skill.Name, err)
		}
	}

	return nil
}

// GetSkills returns all active skills.
func (s *Service) GetSkills(ctx context.Context) ([]models.Skill, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, description, check_type, check_query, severity, is_builtin, is_active, created_at
		FROM skills
		WHERE is_active = 1
		ORDER BY is_builtin DESC, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []models.Skill
	for rows.Next() {
		var skill models.Skill
		if err := rows.Scan(&skill.ID, &skill.Name, &skill.Description, &skill.CheckType, &skill.CheckQuery,
			&skill.Severity, &skill.IsBuiltin, &skill.IsActive, &skill.CreatedAt); err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}

	return skills, nil
}

// GetSkillByName returns a skill by name.
func (s *Service) GetSkillByName(ctx context.Context, name string) (*models.Skill, error) {
	var skill models.Skill
	err := s.db.QueryRow(ctx, `
		SELECT id, name, description, check_type, check_query, severity, is_builtin, is_active, created_at
		FROM skills
		WHERE name = ?
	`, name).Scan(&skill.ID, &skill.Name, &skill.Description, &skill.CheckType, &skill.CheckQuery,
		&skill.Severity, &skill.IsBuiltin, &skill.IsActive, &skill.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &skill, nil
}

// LogSkillResult logs a skill execution result.
func (s *Service) LogSkillResult(ctx context.Context, log models.SkillLog) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO skill_logs (skill_id, page, action, result, is_passed, error_message)
		VALUES (?, ?, ?, ?, ?, ?)
	`, log.SkillID, log.Page, log.Action, log.Result, boolToInt(log.IsPassed), log.ErrorMsg)

	return err
}

// GetSkillLogs returns recent skill execution logs.
func (s *Service) GetSkillLogs(ctx context.Context, skillID int, limit int) ([]models.SkillLog, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, skill_id, page, action, result, is_passed, error_message, created_at
		FROM skill_logs
		WHERE skill_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, skillID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.SkillLog
	for rows.Next() {
		var log models.SkillLog
		if err := rows.Scan(&log.ID, &log.SkillID, &log.Page, &log.Action, &log.Result,
			&log.IsPassed, &log.ErrorMsg, &log.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetSkillStats returns aggregate skill execution stats.
func (s *Service) GetSkillStats(ctx context.Context) (map[string]interface{}, error) {
	var totalRuns, passed, failed int

	err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN is_passed = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN is_passed = 0 THEN 1 ELSE 0 END), 0)
		FROM skill_logs
	`).Scan(&totalRuns, &passed, &failed)

	if err != nil {
		return nil, err
	}

	passRate := 0.0
	if totalRuns > 0 {
		passRate = float64(passed) / float64(totalRuns)
	}

	return map[string]interface{}{
		"total_runs": totalRuns,
		"passed":     passed,
		"failed":     failed,
		"pass_rate":  passRate,
	}, nil
}

// GenerateSkill uses AI to generate a new skill from issues.
func (s *Service) GenerateSkill(ctx context.Context, issues []models.RawIssue) (*models.Skill, error) {
	if len(issues) == 0 {
		return nil, fmt.Errorf("no issues provided")
	}

	// Call AI to generate skill
	skillJSON, err := s.aiService.GenerateSkill(issues)
	if err != nil {
		return nil, err
	}
	_ = skillJSON // Use in production for parsing

	// Parse and insert skill (placeholder - in production we'd parse skillJSON).
	var skill models.Skill
	skill.Name = "generated_" + fmt.Sprint(len(issues))
	skill.Description = "Auto-generated from " + fmt.Sprint(len(issues)) + " issues"
	skill.CheckType = "validation"
	skill.CheckQuery = ".generated"
	skill.Severity = "medium"
	skill.IsBuiltin = false
	skill.IsActive = true

	_, err = s.db.Exec(ctx, `
		INSERT INTO skills (name, description, check_type, check_query, severity, is_builtin, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, skill.Name, skill.Description, skill.CheckType, skill.CheckQuery, skill.Severity, boolToInt(skill.IsBuiltin), boolToInt(skill.IsActive))

	if err != nil {
		return nil, err
	}

	return &skill, nil
}

// DeactivateSkill deactivates a skill by name.
func (s *Service) DeactivateSkill(ctx context.Context, name string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE skills SET is_active = 0 WHERE name = ?
	`, name)

	return err
}

// ActivateSkill activates a skill by name.
func (s *Service) ActivateSkill(ctx context.Context, name string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE skills SET is_active = 1 WHERE name = ?
	`, name)

	return err
}
