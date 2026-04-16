package ai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

// Service handles AI interactions with Minimax
type Service struct {
	APIKey        string
	BaseURL       string
	Model         string
	ChatEndpoint  string
	httpClient    *http.Client
}

// NewService creates a new AI service
func NewService(apiKey string) *Service {
	// Default values - these will be overridden by config
	return &Service{
		APIKey:       apiKey,
		BaseURL:      "https://api.minimax.io/v1",
		Model:        "MiniMax-M2.5",
		ChatEndpoint: "/chat/completions",
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NewServiceWithConfig creates a new AI service with full configuration
func NewServiceWithConfig(apiKey, baseURL, model, chatEndpoint string) *Service {
	return &Service{
		APIKey:       apiKey,
		BaseURL:      baseURL,
		Model:        model,
		ChatEndpoint: chatEndpoint,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// ChatRequest represents a chat request
type ChatRequest struct {
	Model      string        `json:"model"`
	Messages   []models.Message `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
}

// ChatResponse represents a chat response
type ChatResponse struct {
	Choices []struct {
		Message models.Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Chat generates a chat completion
func (s *Service) Chat(prompt string) (string, error) {
	if s.APIKey == "" {
		return "", fmt.Errorf("MINIMAX_API_KEY not set")
	}

	req := ChatRequest{
		Model: s.Model,
		Messages: []models.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", s.BaseURL+s.ChatEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.APIKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI")
	}

	return chatResp.Choices[0].Message.Content, nil
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
	if s.APIKey == "" {
		return nil, fmt.Errorf("MINIMAX_API_KEY not set")
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

	fmt.Printf("⚠️  Using placeholder embedding (MiniMax doesn't have public embedding API)\n")
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

	result, err := s.Chat(prompt)
	if err != nil {
		return "", err
	}

	if result == "frontend" || result == "backend" {
		return result, nil
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

	result, err := s.Chat(prompt)
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

	result, err := s.Chat(prompt)
	if err != nil {
		return "", err
	}

	return result, nil
}

// EstimateCost estimates the cost of an API call
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

func init() {
	// Set defaults from environment
	if os.Getenv("MINIMAX_API_KEY") == "" {
		fmt.Println("⚠️  MINIMAX_API_KEY not set - AI features will be disabled")
	}
}