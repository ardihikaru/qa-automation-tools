# playwright-docs — agent instructions

**Deprecated.** Flow recording now lives in [../playwright-executor](../playwright-executor/CLAUDE.md) (`docs_generate`). The Markdown summary is produced in ai-backend (`summarizeDocsRun`, via `ai.Service`, ai-manager and claude-v3). Make changes there. This package stays only for reference. The contract (frozen) is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md).

- Config comes from the root `config.yaml`, loaded by `../lib/config.ts`. Credentials come from `config/credentials.yaml`. **Never print either file.**
- `callAI` in `src/summarizer/summarizeFlow.ts` goes to ai-manager `/v1/chat` pinned to `claude-v3`. The `minimax` provider branch is **dormant**: never use it without the user's explicit approval. `callMinimax` is deprecated.
- `src/exploreSystem.ts` contains hard-coded staging credentials. Don't copy them anywhere.
- Targets HISv3 **staging** (`staging.url`). Never point it at production HISv3.
