package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/models"
)

// Service is the backend's AI facade. All LLM traffic goes through a ChatProvider
// (default: ai-manager over WireGuard; fallback: MiniMax direct).
type Service struct {
	// APIKey/BaseURL/Model/ChatEndpoint describe the MiniMax fallback and are kept
	// for backward compatibility with existing callers and tests.
	APIKey       string
	BaseURL      string
	Model        string
	ChatEndpoint string

	provider ChatProvider
	timeout  time.Duration

	usageMu   sync.Mutex
	lastUsage ChatResult
	recorder  CallRecorder
	convStore ConversationStore
}

// SetConversationStore installs the store that keeps every exchange (AI_CONVERSATIONS.md).
// nil = nothing is stored.
func (s *Service) SetConversationStore(cs ConversationStore) {
	s.usageMu.Lock()
	s.convStore = cs
	s.usageMu.Unlock()
}

// SetRecorder installs the ledger that receives one record per AI call (success or failure).
func (s *Service) SetRecorder(r CallRecorder) {
	s.usageMu.Lock()
	s.recorder = r
	s.usageMu.Unlock()
}

// NewService creates a MiniMax-backed AI service (legacy constructor).
func NewService(apiKey string) *Service {
	return NewServiceWithConfig(apiKey, "", "", "")
}

// NewServiceWithConfig creates a MiniMax-backed AI service (legacy constructor).
func NewServiceWithConfig(apiKey, baseURL, model, chatEndpoint string) *Service {
	return newMiniMaxService(apiKey, baseURL, model, chatEndpoint, 30*time.Second)
}

func newMiniMaxService(apiKey, baseURL, model, chatEndpoint string, timeout time.Duration) *Service {
	p := newMiniMaxProvider(apiKey, baseURL, model, chatEndpoint, timeout)
	return &Service{
		APIKey:       apiKey,
		BaseURL:      p.baseURL,
		Model:        p.model,
		ChatEndpoint: p.chatEndpoint,
		provider:     p,
		timeout:      p.httpClient.Timeout,
	}
}

// NewServiceWithProvider creates a service around any ChatProvider.
func NewServiceWithProvider(p ChatProvider, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	return &Service{provider: p, timeout: timeout}
}

// ProviderName returns the configured AI_PROVIDER value.
func (s *Service) ProviderName() string {
	if s == nil || s.provider == nil {
		return ""
	}
	return s.provider.Name()
}

// Upstream returns the pinned upstream provider/model (e.g. "claude-v3").
func (s *Service) Upstream() string {
	if s == nil || s.provider == nil {
		return ""
	}
	return s.provider.Upstream()
}

// Configured reports whether the active provider has the credentials it needs.
func (s *Service) Configured() bool {
	return s != nil && s.provider != nil && s.provider.Configured()
}

// Health returns the active provider's health ("ok", "configured", "not_configured", ...).
func (s *Service) Health(ctx context.Context) string {
	if s == nil || s.provider == nil {
		return "not_configured"
	}
	return s.provider.Health(ctx)
}

// LastUsage returns token/cost data of the most recent successful chat call.
func (s *Service) LastUsage() ChatResult {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	return s.lastUsage
}

// Chat sends a single user prompt and returns the reply text.
func (s *Service) Chat(prompt string) (string, error) {
	return s.ChatFeature(prompt, "")
}

// ChatFeature is Chat with a feature label for the cost ledger.
func (s *Service) ChatFeature(prompt, feature string) (string, error) {
	return s.ChatMessages(context.Background(), []models.Message{{Role: "user", Content: prompt}}, ChatOpts{Temperature: 0.7, Feature: feature})
}

// ChatMessages sends a full message list through the active provider.
func (s *Service) ChatMessages(ctx context.Context, messages []models.Message, opts ChatOpts) (string, error) {
	if s == nil || s.provider == nil {
		return "", fmt.Errorf("AI service not initialised")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	start := time.Now()
	res, err := s.provider.Chat(ctx, messages, opts)
	latency := time.Since(start)
	s.usageMu.Lock()
	rec := s.recorder
	cs := s.convStore
	if err == nil {
		s.lastUsage = res
	}
	s.usageMu.Unlock()
	if rec != nil {
		cr := CallRecord{
			At: start, Feature: opts.Feature, Caller: opts.Caller,
			Provider: res.Provider, Model: res.Model, RequestedProvider: res.RequestedProvider,
			Status: "ok", Fallback: res.Fallback,
			InputTokens: res.InputTokens, OutputTokens: res.OutputTokens,
			CostUSD: res.CostUSD, CostIDR: res.CostIDR, CostSource: res.CostSource,
			LatencyMS: int(latency.Milliseconds()),
		}
		if cr.Provider == "" {
			cr.Provider = s.provider.Name()
		}
		if err != nil {
			cr.Status, cr.Error = "error", err.Error()
		}
		rec.RecordCall(cr)
	}
	if cs != nil {
		saveExchange(ctx, cs, messages, opts, res, err)
	}
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// EmbeddingRequest represents an embedding request
type EmbeddingRequest struct {
	Text  string `json:"text"`
	Model string `json:"model"`
}

// EmbeddingResponse represents an embedding response
type EmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

// GenerateEmbedding generates an embedding for text
// Note: MiniMax doesn't have public embedding API, so we return a placeholder
// In production, consider using OpenAI text-embedding-3-small or another provider
func (s *Service) GenerateEmbedding(text string) ([]float64, error) {
	if !s.Configured() {
		return nil, fmt.Errorf("AI provider not configured")
	}

	// Per MiniMax-API-Endpoint.pdf: embedding API not available publicly
	// Return a simple hash-based placeholder embedding for demo purposes
	// In production, integrate with OpenAI or other embedding provider
	placeholder := make([]float64, 1536)
	hash := 0
	for i, c := range text {
		placeholder[i%1536] += float64(c)
		hash ^= int(c) << ((i % 8) * 4)
	}
	// Normalize
	for i := range placeholder {
		placeholder[i] = placeholder[i] / float64(len(text)+1)
		if placeholder[i] > 1 {
			placeholder[i] = 1
		}
	}

	fmt.Printf("⚠️  Using placeholder embedding (no embedding API behind the AI provider)\n")
	return placeholder, nil
}

// GenerateEmbeddingBase64 generates an embedding and returns as base64
func (s *Service) GenerateEmbeddingBase64(text string) (string, error) {
	emb, err := s.GenerateEmbedding(text)
	if err != nil {
		return "", err
	}

	embJSON, err := json.Marshal(emb)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(embJSON), nil
}

// ClassifyIssue classifies an issue as frontend or backend
func (s *Service) ClassifyIssue(errorText string) (string, error) {
	prompt := fmt.Sprintf(`
Classify this error as either "frontend" or "backend":

Error: %s

Return just one word: frontend or backend
`, errorText)

	// An AI failure (gateway down, 401/502, timeout) is not fatal: fall through to the
	// keyword heuristic so the issue still gets a category.
	result, err := s.ChatFeature(prompt, "classify-issue")
	if err == nil {
		result = strings.ToLower(strings.Trim(strings.TrimSpace(result), ".\"'`"))
		if result == "frontend" || result == "backend" {
			return result, nil
		}
	}

	// Fallback to simple classification
	if contains(errorText, []string{"500", "Internal Server Error", "database", "SQL"}) {
		return "backend", nil
	}
	if contains(errorText, []string{"JS", "JavaScript", "TypeError", "ReferenceError"}) {
		return "frontend", nil
	}

	return "unknown", nil
}

// GenerateSuggestion generates a fix suggestion for an issue
func (s *Service) GenerateSuggestion(issue models.RawIssue) (string, float64, error) {
	prompt := fmt.Sprintf(`
Generate a fix suggestion for this issue:

Page: %s
Action: %s
Error: %s
Category: %s

Provide a brief suggestion and confidence score (0-1). Format:
Suggestion: <your suggestion>
Confidence: <score>
`, issue.Page, issue.Action, issue.ErrorText, issue.Category)

	result, err := s.ChatFeature(prompt, "issue-suggestion")
	if err != nil {
		return "", 0, err
	}

	// Parse result
	var suggestion string
	var confidence float64
	fmt.Sscanf(result, "Suggestion: %s\nConfidence: %f", &suggestion, &confidence)

	if suggestion == "" {
		suggestion = result
		confidence = 0.5
	}

	return suggestion, confidence, nil
}

// GenerateSkill generates a new QA skill from observed issues
func (s *Service) GenerateSkill(issues []models.RawIssue) (string, error) {
	if len(issues) == 0 {
		return "", fmt.Errorf("no issues provided")
	}

	prompt := fmt.Sprintf(`
Generate a QA skill to detect these issues:

%s

For each issue, provide:
1. Skill name
2. Check type (selector, validation, data)
3. Check query (CSS selector or validation logic)
4. Severity (low, medium, high)

Format as JSON array:
[{"name": "...", "check_type": "...", "check_query": "...", "severity": "..."}]
`, formatIssues(issues))

	result, err := s.ChatFeature(prompt, "skill-generation")
	if err != nil {
		return "", err
	}

	return result, nil
}

// EstimateCost estimates the cost of an API call
// When the last call reported a real cost (ai-manager does), prefer LastUsage().CostUSD.
func (s *Service) EstimateCost(inputTokens, outputTokens int) float64 {
	inputCost := float64(inputTokens) / 1000.0 * 0.001  // $0.001 per 1K input
	outputCost := float64(outputTokens) / 1000.0 * 0.01 // $0.01 per 1K output
	return inputCost + outputCost
}

// Helper functions

func contains(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) && containsAt(s, sub) {
			return true
		}
	}
	return false
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func formatIssues(issues []models.RawIssue) string {
	var result string
	for i, issue := range issues {
		result += fmt.Sprintf("%d. %s - %s (%s)\n", i+1, issue.Page, issue.Action, issue.ErrorText)
	}
	return result
}

// NewFromConfig builds the AI service selected by AI_PROVIDER.
func NewFromConfig(c config.AIConfig) *Service {
	timeout := time.Duration(c.TimeoutSeconds) * time.Second
	if c.Provider == ProviderMiniMax {
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		return newMiniMaxService(c.APIKey, c.BaseURL, c.Model, c.ChatEndpoint, timeout)
	}
	return NewServiceWithProvider(NewAIManagerProvider(AIManagerConfig{
		URL:        c.ManagerURL,
		LoginID:    c.ManagerLoginID,
		SecretKey:  c.ManagerSecretKey,
		Provider:   c.ManagerProvider,
		Model:      c.ManagerModel,
		Timeout:    timeout,
		NoFallback: c.ManagerNoFallback,
	}), timeout)
}
