package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

// AIManagerConfig configures the client for the production ai-manager gateway.
// In production it is reached directly over WireGuard (http://10.10.0.4:8099),
// never through the public TLS edge.
type AIManagerConfig struct {
	URL       string // base URL, e.g. http://10.10.0.4:8099
	LoginID   string // HTTP Basic login (shared ai-stack credentials)
	SecretKey string // HTTP Basic secret
	Provider  string // upstream provider inside ai-manager; this repo pins "claude-v3" (rnd-v3 Claude CLI)
	Model     string // optional upstream model
	Caller    string // reported to ai-manager for usage attribution
	// NoFallback forbids ai-manager from answering with a different provider.
	// Enforced twice: sent as no_fallback, and a reply flagged fallback=true is rejected.
	NoFallback bool
	Timeout    time.Duration // CLI-backed providers can take ~150s; default 180s
}

// aiManagerProvider calls POST {URL}/v1/chat. ai-manager's API is custom
// (not OpenAI-compatible): request takes messages/prompt, reply carries `text`.
type aiManagerProvider struct {
	cfg        AIManagerConfig
	httpClient *http.Client

	healthMu     sync.Mutex
	healthAt     time.Time
	healthStatus string
}

// aiManagerChatRequest mirrors ai-manager internal/server/handlers_chat.go.
type aiManagerChatRequest struct {
	Messages    []models.Message `json:"messages"`
	Provider    string           `json:"provider,omitempty"`
	Model       string           `json:"model,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Caller      string           `json:"caller,omitempty"`
	NoFallback  bool             `json:"no_fallback,omitempty"`
}

// aiManagerChatResponse mirrors ai-manager ChatManagerResponse (subset).
type aiManagerChatResponse struct {
	Text         string  `json:"text"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Fallback     bool    `json:"fallback"`
	Error        string  `json:"error"`
}

const healthCacheTTL = 30 * time.Second

// NewAIManagerProvider builds an ai-manager client. Exported for cmd/verifyai.
func NewAIManagerProvider(cfg AIManagerConfig) ChatProvider {
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = 180 * time.Second
	}
	if cfg.Caller == "" {
		cfg.Caller = "qa-automation"
	}
	return &aiManagerProvider{cfg: cfg, httpClient: &http.Client{Timeout: cfg.Timeout}}
}

func (p *aiManagerProvider) Name() string     { return ProviderAIManager }
func (p *aiManagerProvider) Upstream() string { return p.cfg.Provider }

func (p *aiManagerProvider) Configured() bool {
	return p.cfg.URL != "" && p.cfg.LoginID != "" && p.cfg.SecretKey != ""
}

func (p *aiManagerProvider) Chat(ctx context.Context, messages []models.Message, opts ChatOpts) (ChatResult, error) {
	if !p.Configured() {
		return ChatResult{}, fmt.Errorf("ai-manager: AI_MANAGER_URL/AI_MANAGER_LOGIN_ID/AI_MANAGER_SECRET_KEY not set")
	}
	body, err := json.Marshal(aiManagerChatRequest{
		Messages:    messages,
		Provider:    p.cfg.Provider,
		Model:       p.cfg.Model,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Caller:      p.cfg.Caller,
		NoFallback:  p.cfg.NoFallback,
	})
	if err != nil {
		return ChatResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.URL+"/v1/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.cfg.LoginID, p.cfg.SecretKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("ai-manager: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ChatResult{}, fmt.Errorf("ai-manager: read body: %w", err)
	}

	var out aiManagerChatResponse
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != http.StatusOK {
		msg := out.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
			if len(msg) > 200 {
				msg = msg[:200]
			}
		}
		return ChatResult{}, fmt.Errorf("ai-manager: status %d: %s", resp.StatusCode, msg)
	}
	if p.cfg.NoFallback && p.cfg.Provider != "" && (out.Fallback || (out.Provider != "" && out.Provider != p.cfg.Provider)) {
		return ChatResult{}, fmt.Errorf("ai-manager: answered by %q but provider is pinned to %q", out.Provider, p.cfg.Provider)
	}
	if strings.TrimSpace(out.Text) == "" {
		return ChatResult{}, fmt.Errorf("ai-manager: empty response text")
	}
	return ChatResult{
		Text:         out.Text,
		Provider:     out.Provider,
		Model:        out.Model,
		InputTokens:  out.InputTokens,
		OutputTokens: out.OutputTokens,
		CostUSD:      out.CostUSD,
	}, nil
}

// Health probes GET /v1/health (unauthenticated) and caches the result for 30s,
// so the backend /health endpoint never hammers or blocks on the gateway.
// When an upstream provider is pinned, it must also be listed as enabled
// (otherwise "upstream_disabled:<name>").
func (p *aiManagerProvider) Health(ctx context.Context) string {
	if !p.Configured() {
		return "not_configured"
	}
	p.healthMu.Lock()
	defer p.healthMu.Unlock()
	if time.Since(p.healthAt) < healthCacheTTL && p.healthStatus != "" {
		return p.healthStatus
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	status := "unreachable"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.URL+"/v1/health", nil)
	if err == nil {
		if resp, err := p.httpClient.Do(req); err == nil {
			var h struct {
				Providers map[string]bool `json:"providers"`
			}
			_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&h)
			resp.Body.Close()
			switch {
			case resp.StatusCode != http.StatusOK:
				status = fmt.Sprintf("status_%d", resp.StatusCode)
			case p.cfg.Provider != "" && h.Providers != nil && !h.Providers[p.cfg.Provider]:
				status = "upstream_disabled:" + p.cfg.Provider
			default:
				status = "ok"
			}
		}
	}
	p.healthAt, p.healthStatus = time.Now(), status
	return status
}
