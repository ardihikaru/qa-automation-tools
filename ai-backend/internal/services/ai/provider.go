package ai

import (
	"context"

	"github.com/qa-automation/ai-backend/internal/models"
)

// Provider names accepted by AI_PROVIDER.
const (
	ProviderAIManager = "ai-manager"
	ProviderMiniMax   = "minimax"
)

// ChatOpts are per-call tuning knobs. Zero values mean "provider default".
type ChatOpts struct {
	Temperature float64
	MaxTokens   int
}

// ChatResult is the provider-neutral reply of a chat call.
type ChatResult struct {
	Text         string
	Provider     string // upstream provider that actually answered (e.g. antigravity, minimax)
	Model        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// ChatProvider is the single seam between the backend and any LLM gateway.
// Implementations: aiManagerProvider (default, via WireGuard) and minimaxProvider (fallback).
type ChatProvider interface {
	// Name returns the AI_PROVIDER value this implementation answers to.
	Name() string
	// Configured reports whether the credentials needed to call the provider are present.
	Configured() bool
	// Chat sends the messages and returns the first completion.
	Chat(ctx context.Context, messages []models.Message, opts ChatOpts) (ChatResult, error)
	// Health returns "ok" when the upstream is reachable, otherwise a short reason.
	Health(ctx context.Context) string
}
