package models

import (
	"encoding/json"
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
	Steps       []Step    `json:"steps,omitempty"`
}

// Step represents a flow step
type Step struct {
	ID        int    `json:"id"`
	FlowID    int    `json:"flow_id"`
	StepOrder int    `json:"step_order"`
	Action    string `json:"action"`
	Selector  string `json:"selector,omitempty"`
	Value     string `json:"value,omitempty"`
}

// Skill represents a QA skill
type Skill struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CheckType   string    `json:"check_type"`
	CheckQuery  string    `json:"check_query"`
	Severity    string    `json:"severity"`
	IsBuiltin   bool      `json:"is_builtin"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	// ProductID ties a QA check skill to one product; nil means global (every product).
	ProductID *int `json:"product_id"`
}

// RawIssue represents a detected issue
type RawIssue struct {
	ID        int       `json:"id"`
	Page      string    `json:"page"`
	Action    string    `json:"action"`
	ErrorType string    `json:"error_type,omitempty"`
	ErrorText string    `json:"error_text,omitempty"`
	Category  string    `json:"category,omitempty"`
	Severity  string    `json:"severity"`
	IsFixed   bool      `json:"is_fixed"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IssueSuggestion represents a suggestion for fixing an issue
type IssueSuggestion struct {
	ID         int       `json:"id"`
	IssueID    int       `json:"issue_id"`
	Suggestion string    `json:"suggestion"`
	Confidence float64   `json:"confidence"`
	Target     string    `json:"target,omitempty"`
	FilePath   string    `json:"file_path,omitempty"`
	Diff       string    `json:"diff,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AIRequest represents an AI API request
type AIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Kind labels the message for the stored conversation (AI_CONVERSATIONS.md: prompt, reply,
	// feedback, refusal, run_result, ...). Never serialised: what a provider receives is unchanged.
	Kind string `json:"-"`
}

// AIResponse represents an AI API response
type AIResponse struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
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
	Intent       string    `json:"intent"`
	Action       string    `json:"action"`
	Page         string    `json:"page"`
	SuccessScore float64   `json:"success_score"`
	UsageCount   int       `json:"usage_count"`
	LastSuccess  time.Time `json:"last_success"`
	DecayFactor  float64   `json:"decay_factor"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SkillLog represents a skill execution log
type SkillLog struct {
	ID        int       `json:"id"`
	SkillID   int       `json:"skill_id"`
	Page      string    `json:"page"`
	Action    string    `json:"action"`
	Result    string    `json:"result"`
	IsPassed  bool      `json:"is_passed"`
	ErrorMsg  string    `json:"error_message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// NavigationEdge represents a navigation graph edge
type NavigationEdge struct {
	ID           int       `json:"id"`
	SourcePage   string    `json:"source_page"`
	TargetPage   string    `json:"target_page"`
	Action       string    `json:"action"`
	SuccessCount int       `json:"success_count"`
	FailureCount int       `json:"failure_count"`
	SuccessRate  float64   `json:"success_rate"`
	AvgDuration  int       `json:"avg_duration_ms"`
	LastUsed     time.Time `json:"last_used,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ActionSequence represents a multi-step plan
type ActionSequence struct {
	ID           int       `json:"id"`
	Goal         string    `json:"goal"`
	Steps        string    `json:"steps"` // JSON string
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
	ID           int       `json:"id"`
	ContextKey   string    `json:"context_key"`
	ContextValue string    `json:"context_value"`
	BestAction   string    `json:"best_action"`
	SuccessScore float64   `json:"success_score"`
	UsageCount   int       `json:"usage_count"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GoalEvent represents a goal completion tracking event
type GoalEvent struct {
	ID         int       `json:"id"`
	GoalName   string    `json:"goal_name"`
	Confidence float64   `json:"confidence"`
	Page       string    `json:"page,omitempty"`
	Action     string    `json:"action,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
}

// Product is a product under test: where its repo lives and how to reach it.
type Product struct {
	ID                   int             `json:"id"`
	Name                 string          `json:"name"`
	RepoURL              string          `json:"repo_url"`
	StagingURL           string          `json:"staging_url"`
	RootDir              string          `json:"root_dir"`
	Branch               string          `json:"branch"`
	PlaywrightAccessible bool            `json:"playwright_accessible"`
	LastVerifiedAt       *time.Time      `json:"last_verified_at"`
	VerifyReport         json.RawMessage `json:"verify_report"`
	// AllowNonAutomationWork comes from product_settings (a missing row is false).
	AllowNonAutomationWork bool `json:"allow_non_automation_work"`
	// KnowledgeFormat is product_settings.knowledge_format ("" = not set or sync off; PRODUCT_KNOWLEDGE_SYNC.md).
	KnowledgeFormat string `json:"knowledge_format"`
	// GAOS is the product_settings GAOS integration (GAOS_INTEGRATION.md).
	GAOS GAOSSettings `json:"gaos"`
	// Repos lists the product's repos (main first). The legacy fields above mirror the automation repo.
	Repos     []ProductRepo `json:"repos"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// GAOSSettings is a product's GAOS integration state (the server-wide "configured" flag is
// gaos_configured on the products list).
type GAOSSettings struct {
	Integrated bool       `json:"integrated"`
	ProductID  string     `json:"product_id"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	LastStatus string     `json:"last_status"`
	LastNote   string     `json:"last_note"`
}

// ProductRepo is one repository of a product (main, automation, central or sibling; see PRODUCT_REPOS.md and PRODUCT_REPOS_AUTOMATION_ROLE.md).
type ProductRepo struct {
	ID                   int             `json:"id"`
	ProductID            int             `json:"product_id"`
	Name                 string          `json:"name"`
	RepoURL              string          `json:"repo_url"`
	RootDir              string          `json:"root_dir"`
	IsMain               bool            `json:"is_main"`
	IsAutomation         bool            `json:"is_automation"`
	IsCentral            bool            `json:"is_central"`
	DefaultBranch        string          `json:"default_branch"`
	Branches             []string        `json:"branches"`
	PlaywrightAccessible bool            `json:"playwright_accessible"`
	LastVerifiedAt       *time.Time      `json:"last_verified_at"`
	VerifyReport         json.RawMessage `json:"verify_report"`
	SyncReport           json.RawMessage `json:"sync_report"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// BranchResult is the outcome of syncing one branch checkout.
// Status is one of "ok", "error", "kept_dirty", "missing_remote".
type BranchResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
