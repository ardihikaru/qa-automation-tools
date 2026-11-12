// Command verifyai is a smoke test for the configured AI provider.
//
// It loads the same configuration as the server (env / .env / config.yaml),
// prints the provider health and sends one tiny chat request.
//
//	go run ./cmd/verifyai            # uses AI_PROVIDER (default ai-manager)
//	go run ./cmd/verifyai "ping"     # custom prompt
//
// Exit code 0 = reply received, 1 = failure. Credentials are never printed.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/services/ai"
)

func main() {
	_ = godotenv.Load()
	cfg := config.LoadConfig()

	svc := ai.NewFromConfig(cfg.Backend.AI)
	fmt.Printf("provider:   %s\n", svc.ProviderName())
	if svc.ProviderName() == ai.ProviderAIManager {
		fmt.Printf("url:        %s\n", cfg.Backend.AI.ManagerURL)
		fmt.Printf("upstream:   %q (no_fallback=%v)\n", cfg.Backend.AI.ManagerProvider, cfg.Backend.AI.ManagerNoFallback)
	}
	fmt.Printf("configured: %v\n", svc.Configured())
	fmt.Printf("health:     %s\n", svc.Health(context.Background()))
	if !svc.Configured() {
		os.Exit(1)
	}

	prompt := "Reply with exactly one word: pong"
	if len(os.Args) > 1 {
		prompt = strings.Join(os.Args[1:], " ")
	}
	start := time.Now()
	text, err := svc.Chat(prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat failed after %s: %v\n", time.Since(start).Round(time.Millisecond), err)
		os.Exit(1)
	}
	u := svc.LastUsage()
	fmt.Printf("answered by %s/%s in %s (in=%d out=%d cost=$%.6f)\n",
		u.Provider, u.Model, time.Since(start).Round(time.Millisecond), u.InputTokens, u.OutputTokens, u.CostUSD)
	fmt.Println("reply:", strings.TrimSpace(text))
}
