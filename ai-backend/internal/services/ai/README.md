# services/ai

## Overview

Package `ai` is ai-backend's single LLM facade. `Service` sends every chat call
through a `ChatProvider`. The production ai-manager gateway, reached directly
over WireGuard and pinned to upstream **`claude-v3`** with `no_fallback`, is
the **only supported provider**. The MiniMax provider (`AI_PROVIDER=minimax`)
is **dormant**: it stays in the code, but enabling it requires the owner's
explicit approval. Callers include `internal/api/jobs_executor.go`
`summarizeDocsRun`, which turns playwright-executor recordings into Markdown.
The package also has QA-specific helpers
(issue classification, fix suggestions, skill generation) and a placeholder
embedding generator. It is constructed from
[config](../../config/README.md) `AIConfig` by `cmd/server`, `cmd/verifyai`,
and `internal/api/router.go`, and is used by `internal/services/skills`.

## Interface

- `provider.go:ChatProvider` - interface with `Name()`, `Configured()`,
  `Chat(ctx, []models.Message, ChatOpts) (ChatResult, error)`, `Health(ctx) string`, `Upstream() string`.
- `provider.go:ChatOpts` (`Temperature`, `MaxTokens`; zero = provider default)
  and `provider.go:ChatResult` (`Text`, `Provider`, `Model`, `InputTokens`,
  `OutputTokens`, `CostUSD`).
- `provider.go:ProviderAIManager` (`"ai-manager"`), `provider.go:ProviderMiniMax` (`"minimax"`).
- `aimanager.go:AIManagerConfig` (`URL`, `LoginID`, `SecretKey`, `Provider`,
  `Model`, `Caller`, `Timeout`, `NoFallback`) and `aimanager.go:NewAIManagerProvider(cfg) ChatProvider`.
- `minimax.go:ChatRequest` / `minimax.go:ChatResponse` - exported
  OpenAI-compatible wire types. The provider itself (`minimaxProvider`,
  `newMiniMaxProvider`) is unexported.
- `aiService.go:Service` - exported fields `APIKey`, `BaseURL`, `Model`,
  `ChatEndpoint` (MiniMax legacy) and methods `ProviderName`, `Configured`,
  `Health`, `LastUsage`, `Chat(prompt)`, `ChatFeature(prompt, feature)`, `ChatMessages(ctx, msgs, opts)`, `SetRecorder`,
  `GenerateEmbedding`, `GenerateEmbeddingBase64`, `ClassifyIssue`,
  `GenerateSuggestion`, `GenerateSkill`, `EstimateCost`.
- Constructors: `aiService.go:NewFromConfig(config.AIConfig)` (selects the
  provider), `aiService.go:NewServiceWithProvider(p, timeout)`, and legacy
  MiniMax-only `aiService.go:NewService` / `aiService.go:NewServiceWithConfig`.
- `aiService.go:EmbeddingRequest` / `aiService.go:EmbeddingResponse` types.

## Invariants
- **Pinned upstream.** In production `Provider` is `claude-v3`, the Claude CLI REST on rnd-v3 `10.10.0.9:8105`, reached via ai-manager, and `NoFallback` is true. `Chat` sends `no_fallback`, and returns an error if the reply has `fallback: true` or comes from any provider other than `Provider`. `Health` returns `upstream_disabled:<provider>` when ai-manager's `/v1/health` lists the pinned provider as not enabled.
- `ClassifyIssue` never fails because of the AI: a chat error falls through to the keyword heuristic.

- `aiService.go:NewFromConfig` uses MiniMax only when `Provider == "minimax"`.
  Any other value, including empty, builds an `aiManagerProvider`.
- `aimanager.go:aiManagerProvider.Chat` sends `POST {URL}/v1/chat` with JSON
  `aiManagerChatRequest` and HTTP Basic auth (`LoginID`/`SecretKey`). The API is
  ai-manager's own format, not OpenAI-compatible, and the reply text is read
  from `text`.
- `aimanager.go:NewAIManagerProvider` trims trailing `/` from `URL`, defaults
  `Timeout` to 180s and `Caller` to `qa-automation`. The URL default
  `http://10.10.0.4:8099` (WireGuard address) comes from `AI_MANAGER_URL` in config.
- `aimanager.go:aiManagerProvider.Configured` requires non-empty `URL`,
  `LoginID`, and `SecretKey`. Otherwise `Chat` errors without any network call
  and `Health` returns `not_configured`.
- `aimanager.go:aiManagerProvider.Chat` treats non-200 status (message from the
  JSON `error` field or the first 200 bytes of the body) and blank `text` as
  errors. Response bodies are capped at 4 MiB (`io.LimitReader(..., 4<<20)`).
- `aimanager.go:aiManagerProvider.Health` probes unauthenticated
  `GET {URL}/v1/health` with a 3s timeout and caches the result for
  `healthCacheTTL` (30s) under `healthMu`. It returns `ok`, `status_<code>`, or
  `unreachable`.
- `minimax.go:minimaxProvider.Chat` sends `POST {baseURL}{chatEndpoint}` with
  `Authorization: Bearer <apiKey>`, returns `choices[0]`, and reports
  `CostUSD` as 0. `Health` never calls the network and returns `configured` or
  `not_configured`.
- `minimax.go:newMiniMaxProvider` defaults: base URL `https://api.minimax.io/v1`,
  model `MiniMax-M2.5`, endpoint `/chat/completions`. `NewServiceWithConfig`
  fixes the HTTP client timeout at 30s. `NewFromConfig` can raise only
  `Service.timeout` (the context deadline), not that client timeout.
- `aiService.go:Service.ChatMessages` applies `context.WithTimeout(s.timeout)`.
  `NewServiceWithProvider` defaults `s.timeout` to 180s. Only successful calls
  update `lastUsage` (guarded by `usageMu`).
- `aiService.go:Service.Chat` always uses a single `user` message with `Temperature: 0.7`.
- `aiService.go:Service.GenerateEmbedding` does not call any embedding API. It
  returns a deterministic 1536-dimension character-sum placeholder and errors
  only when the provider is not configured.
- `aiService.go:Service.ClassifyIssue` returns `frontend`, `backend`, or
  `unknown`. When the LLM answer is neither of the first two, it falls back to
  keyword matching.
- `aiService.go:Service.EstimateCost` uses fixed rates ($0.001 per 1K input,
  $0.01 per 1K output tokens). Prefer `LastUsage().CostUSD` when ai-manager
  reports a real cost.

## Side Effects

- `aimanager.go:aiManagerProvider.Chat` / `Health` make outbound HTTP calls to
  the ai-manager gateway (`/v1/chat`, `/v1/health`).
- `minimax.go:minimaxProvider.Chat` makes outbound HTTPS calls to the MiniMax API.
- `aiService.go:Service.ClassifyIssue`, `GenerateSuggestion`, and
  `GenerateSkill` each trigger one chat call through the active provider.
- `aiService.go:Service.GenerateEmbedding` prints a warning to stdout on every call.
- `aiService.go:Service.ChatMessages` mutates in-memory `lastUsage`. The
  package does not access any database.

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/services/ai/...`
  (`aimanager_test.go` covers ai-manager success, error statuses, empty text,
  timeout, not-configured, health caching, `Service` usage recording, claude-v3
  pinning / fallback rejection / upstream-disabled health, and the `ClassifyIssue`
  heuristic fallback)

## Cost ledger
Every `ChatMessages` call (success or failure) is reported to the installed `CallRecorder`
(`recorder.go`). `SQLRecorder` inserts one row per call into `ai_call_log` with feature
(`ChatOpts.Feature`), caller, provider/model, tokens, `cost_usd`/`cost_idr`, `cost_source`,
latency and error. Usage returned by ai-manager is kept even when the call is rejected
(pinned-provider mismatch, empty text). Recording failures are logged and never affect the call.
