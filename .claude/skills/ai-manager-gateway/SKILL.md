---
name: ai-manager-gateway
description: How qa-automation talks to the production ai-manager LLM gateway (POST http://10.8.0.4:8099/v1/chat over WireGuard, HTTP Basic), always pinned to upstream claude-v3 (Claude CLI REST on app-host 10.8.0.9:8105). Use when adding or changing any LLM call, choosing an upstream provider/model, debugging AI errors/timeouts (401/502/503), smoke-testing AI, or touching ai-backend/internal/services/ai or playwright-docs summarizer.
---

# ai-manager gateway

ai-manager is the Go gateway in the personal-workspace repo (`apps/ai-manager`). It runs on **db-host** (`10.8.0.4:8099`) as a systemd user unit. Its public URL is `https://ai-manager.mfardiansyah.id`, but **this repo calls it directly over WireGuard**. That skips the public TLS edge and its 330-second limit.

## Contract (custom, not OpenAI-compatible)
- `POST /v1/chat` takes `{messages:[{role,content}] | prompt, provider?, model?, temperature?, max_tokens?, caller?, no_fallback?, session_id?...}`.
- On 200 it returns `{text, provider, model, input_tokens, output_tokens, cost_usd, cost_idr, fallback, ...}`.
- Errors come back as `{"error": "..."}`:
  - 400: bad body, or a model the provider doesn't offer
  - 401: authentication failed
  - 502: every provider failed
  - 503: unknown or disabled provider
- `GET /v1/health` needs no auth and lists the providers and the default. `GET /v1/providers` needs auth.
- Async mode: send an `X-Request-ID` header to get a 202, then poll `/v1/requests/{id}/result`. This repo doesn't use it yet.
- No streaming. The request body limit is 256 KB.
- Upstream providers: `antigravity` (the gateway default), `claude`, **`claude-v3`**, `minimax-cli`, `minimax`.
- **This repo always uses `claude-v3`.** That is the second `ai-claude-cli` instance, on app-host at `10.8.0.9:8105` (default model `claude-opus-5`; public name `claude-api.ai.mfardiansyah.id`). In ai-manager it is *explicit-only*: it is only reached with `"provider":"claude-v3"`, it is never a fallback target, and it has no sessions or workspaces. ai-manager enables it through `CLAUDE_V3_CHAT_URL` (in `apps/ai-manager/README.md`, "claude-v3").

## In this repo
- Go: `ai-backend/internal/services/ai/aimanager.go` (`aiManagerProvider`) sits behind `ChatProvider`. The factory is `ai.NewFromConfig`, and callers only use `*ai.Service`.
- TS: `callAI` in `playwright-docs/src/summarizer/summarizeFlow.ts`.
- Env: `AI_PROVIDER=ai-manager`, `AI_MANAGER_URL=http://10.8.0.4:8099`, `AI_MANAGER_LOGIN_ID`, `AI_MANAGER_SECRET_KEY`, `AI_MANAGER_PROVIDER=claude-v3`, `AI_MANAGER_NO_FALLBACK=true`, `AI_MANAGER_MODEL` (empty = `claude-opus-5`), `AI_TIMEOUT_SECONDS=180`.
- Pinning is enforced twice. Requests send `provider: claude-v3` and `no_fallback: true`, and the client **rejects** any reply whose `provider` isn't `claude-v3` or that is flagged `fallback: true`. `/health` shows `ai_upstream: claude-v3`, and `ai` becomes `upstream_disabled:claude-v3` if ai-manager stops listing it as enabled.
- Credentials: the shared ai-stack Basic credentials are in `~/.config/ai-stack/basic-auth.json` (`login_id`, `secret_key`) on db-host and app-host. **Never print them.**
- Every request sets `caller` to `qa-automation` or `qa-automation-playwright-docs`, so usage shows up under that name in ai-manager's `/admin/usage/recent`.

## Smoke test
```bash
cd ai-backend
eval "$(ssh app-host 'grep -E "^AI_MANAGER_(LOGIN_ID|SECRET_KEY)=" ~/devel/projects/qa-automation-tools/deploy/qa-tool/.env.prod' | sed 's/^/export /')"
AI_PROVIDER=ai-manager AI_MANAGER_URL=http://10.8.0.4:8099 AI_MANAGER_PROVIDER=claude-v3 go run -mod=mod ./cmd/verifyai "Reply with one word: pong"   # explicit, so a local config.yaml can't switch providers
curl -s http://10.8.0.4:8099/v1/health   # no auth; providers["claude-v3"] must be true
curl -s http://10.8.0.9:8105/health      # ai-claude-cli itself (no auth); actual chat goes via ai-manager only
```
You need to be on a WireGuard peer. A 401 means the credentials are wrong. A 502 means every upstream failed; check ai-manager's logs on db-host with `journalctl --user -u ai-manager`.

## Rules
- Never call a provider directly from new code, `:8105` included. Every call goes through ai-manager with `claude-v3`. Changing the upstream needs the user's explicit approval.
- If claude-v3 is down, calls fail. They do **not** silently move to another model. Callers must degrade gracefully (for example, `ClassifyIssue` falls back to a heuristic). Diagnose in two places: ai-claude-cli on app-host (`curl 10.8.0.9:8105/health`), and ai-manager's `CLAUDE_V3_CHAT_URL` on db-host.
- Timeout ordering: ai-manager single-provider worst case (~150 s) < `AI_TIMEOUT_SECONDS` (180) < dashboard nginx 300 s < edge 330 s. Keep `AI_TIMEOUT_SECONDS` between 180 and 290. A gateway-internal fallback chain can exceed 180 s; that surfaces as a timeout error, and callers must degrade gracefully (for example, `ClassifyIssue` falls back to a heuristic).
- Planned move: ai-manager may move to db2-host at `10.8.0.5:8099`. Only `AI_MANAGER_URL` would change.
