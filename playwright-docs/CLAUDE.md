# playwright-docs — agent instructions

This tool records SUT flows with Playwright and summarises them into Markdown. The contract is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md).

- Config comes from the root `config.yaml`, loaded by `../lib/config.ts`. Credentials come from `config/credentials.yaml`. **Never print either file.**
- AI calls use `callAI` in `src/summarizer/summarizeFlow.ts`, which goes to ai-manager `/v1/chat` with Basic auth (`AI_MANAGER_*` env). Use `provider: minimax` only as a fallback. `callMinimax` is deprecated.
- The ai-manager URL `http://10.8.0.4:8099` only works from a WireGuard peer.
- Type check: `npx tsc --noEmit`. The TS6059 `rootDir` error is pre-existing and comes from importing `../lib`.
- Targets SUT **staging** (`staging.url`). Never point it at production SUT.
