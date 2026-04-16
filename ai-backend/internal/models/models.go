package models

import (
	"time"
)

// Role represents a user role in the system
type Role struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Flow represents a user flow
type Flow struct {
	ID          int       `json:"id"`
	RoleID      int       `json:"role_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Steps      []Step    `json:"steps,omitempty"`
}

// Step represents a flow step
type Step struct {
	ID        int       `json:"id"`
	FlowID    int       `json:"flow_id"`
	StepOrder int       `json:"step_order"`
	Action    string    `json:"action"`
	Selector string    `json:"selector,omitempty"`
	Value    string    `json:"value,omitempty"`
}

// Skill represents a QA skill
type Skill struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CheckType   string    `json:"check_type"`
	CheckQuery string    `json:"check_query"`
	Severity   string    `json:"severity"`
	IsBuiltin  bool      `json:"is_builtin"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

// RawIssue represents a detected issue
type RawIssue struct {
	ID         int       `json:"id"`
	Page      string    `json:"page"`
	Action    string    `json:"action"`
	ErrorType string    `json:"error_type,omitempty"`
	ErrorText string    `json:"error_text,omitempty"`
	Category string    `json:"category,omitempty"`
	Severity string    `json:"severity"`
	IsFixed  bool      `json:"is_fixed"`
	Notes    string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IssueSuggestion represents a suggestion for fixing an issue
type IssueSuggestion struct {
	ID         int       `json:"id"`
	IssueID    int       `json:"issue_id"`
	Suggestion string    `json:"suggestion"`
	Confidence float64  `json:"confidence"`
	Target    string    `json:"target,omitempty"`
	FilePath  string    `json:"file_path,omitempty"`
	Diff      string    `json:"diff,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AIRequest represents an AI API request
type AIRequest struct {
	Model      string    `json:"model"`
	Messages   []Message `json:"messages"`
	Temperature float64  `json:"temperature,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIResponse represents an AI API response
type AIResponse struct {
	Choices []Choice `json:"choices"`
	Usage  Usage   `json:"usage"`
}

// Choice represents a response choice
type Choice struct {
	Message Message `json:"message"`
}

// Usage represents token usage
type Usage struct {
	InputTokens  int `json:"prompt_tokens"`
	OutputTokens int `json:"completion_tokens"`
}

// LearningRecord represents a learning cache entry
type LearningRecord struct {
	ID           int       `json:"id"`
	Intent      string    `json:"intent"`
	Action      string    `json:"action"`
	Page        string    `json:"page"`
	SuccessScore float64 `json:"success_score"`
	UsageCount  int       `json:"usage_count"`
	LastSuccess time.Time `json:"last_success"`
	DecayFactor float64 `json:"decay_factor"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CostOverview represents cost analytics overview
type CostOverview struct {
	TotalRequests   int     `json:"total_requests"`
	TotalTokens   int     `json:"total_tokens"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	TotalCost    float64 `json:"total_cost"`
	CacheHitRate float64 `json:"cache_hit_rate"`
}

// CostTrend represents daily cost trend
type CostTrend struct {
	Date  string  `json:"date"`
	Cost float64 `json:"cost"`
}

// CostByFeature represents cost breakdown by feature
type CostByFeature struct {
	Feature string  `json:"feature"`
	Cost    float64 `json:"cost"`
}

// SkillLog represents a skill execution log
type SkillLog struct {
	ID          int       `json:"id"`
	SkillID    int       `json:"skill_id"`
	Page       string    `json:"page"`
	Action     string    `json:"action"`
	Result     string    `json:"result"`
	IsPassed   bool      `json:"is_passed"`
	ErrorMsg   string    `json:"error_message,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// NavigationEdge represents a navigation graph edge
type NavigationEdge struct {
	ID            int       `json:"id"`
	SourcePage   string    `json:"source_page"`
	TargetPage   string    `json:"target_page"`
	Action       string    `json:"action"`
	SuccessCount int       `json:"success_count"`
	FailureCount int       `json:"failure_count"`
	SuccessRate float64   `json:"success_rate"`
	AvgDuration int       `json:"avg_duration_ms"`
	LastUsed    time.Time `json:"last_used,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ActionSequence represents a multi-step plan
type ActionSequence struct {
	ID            int       `json:"id"`
	Goal          string    `json:"goal"`
	Steps         string    `json:"steps"` // JSON string
	SuccessCount int       `json:"success_count"`
	FailureCount int       `json:"failure_count"`
	AvgDuration  int       `json:"avg_duration_ms,omitempty"`
	LastUsed     time.Time `json:"last_used,omitempty"`
	IsValid      bool      `json:"is_valid"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ContextRecord represents context-aware learning entry
type ContextRecord struct {
	ID            int       `json:"id"`
	ContextKey   string    `json:"context_key"`
	ContextValue string   `json:"context_value"`
	BestAction  string   `json:"best_action"`
	SuccessScore float64  `json:"success_score"`
	UsageCount   int       `json:"usage_count"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GoalEvent represents a goal completion tracking event
type GoalEvent struct {
	ID         int       `json:"id"`
	GoalName   string    `json:"goal_name"`
	Confidence float64  `json:"confidence"`
	Page       string    `json:"page,omitempty"`
	Action     string    `json:"action,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
}