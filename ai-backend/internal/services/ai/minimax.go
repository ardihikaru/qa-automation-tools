package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

// minimaxProvider calls MiniMax's OpenAI-compatible /chat/completions directly.
// Kept as a fallback for AI_PROVIDER=minimax; the default is ai-manager.
type minimaxProvider struct {
	apiKey       string
	baseURL      string
	model        string
	chatEndpoint string
	httpClient   *http.Client
}

func newMiniMaxProvider(apiKey, baseURL, model, chatEndpoint string, timeout time.Duration) *minimaxProvider {
	if baseURL == "" {
		baseURL = "https://api.minimax.io/v1"
	}
	if model == "" {
		model = "MiniMax-M2.5"
	}
	if chatEndpoint == "" {
		chatEndpoint = "/chat/completions"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &minimaxProvider{
		apiKey:       apiKey,
		baseURL:      baseURL,
		model:        model,
		chatEndpoint: chatEndpoint,
		httpClient:   &http.Client{Timeout: timeout},
	}
}

// ChatRequest is the MiniMax (OpenAI-compatible) request body.
type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []models.Message `json:"messages"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
}

// ChatResponse is the MiniMax (OpenAI-compatible) response body.
type ChatResponse struct {
	Choices []struct {
		Message models.Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *minimaxProvider) Name() string     { return ProviderMiniMax }
func (p *minimaxProvider) Upstream() string { return p.model }
func (p *minimaxProvider) Configured() bool { return p.apiKey != "" }

func (p *minimaxProvider) Health(ctx context.Context) string {
	if !p.Configured() {
		return "not_configured"
	}
	return "configured"
}

func (p *minimaxProvider) Chat(ctx context.Context, messages []models.Message, opts ChatOpts) (ChatResult, error) {
	if !p.Configured() {
		return ChatResult{}, fmt.Errorf("MINIMAX_API_KEY not set")
	}
	body, err := json.Marshal(ChatRequest{
		Model:       p.model,
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	})
	if err != nil {
		return ChatResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+p.chatEndpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ChatResult{}, fmt.Errorf("minimax: API returned status %d", resp.StatusCode)
	}

	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChatResult{}, err
	}
	if len(out.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("minimax: no response from AI")
	}
	return ChatResult{
		Text:         out.Choices[0].Message.Content,
		Provider:     ProviderMiniMax,
		Model:        p.model,
		InputTokens:  out.Usage.PromptTokens,
		OutputTokens: out.Usage.CompletionTokens,
	}, nil
}
