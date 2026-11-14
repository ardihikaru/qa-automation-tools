# playwright-executor — agent instructions

This is the only place Playwright runs for qa-automation. It is a Node/TS HTTP service that runs on app-host as a systemd user unit, with its toolchain from mise. The contract is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md). Deploy and ops are in [../deploy/playwright-executor/README.md](../deploy/playwright-executor/README.md).

- **Toolchain.** Always run through mise (`mise exec -- npm ...`). Node is pinned in `mise.toml`, and `playwright` is pinned to an exact version in `package.json`. Bumping Playwright means re-running `install.sh` so the matching Chromium is downloaded.
- **Commands.**
  - Type check: `mise exec -- npm run typecheck`
  - Tests: `mise exec -- npm test`
  - Build: `mise exec -- npm run build`
- **No AI and no backend calls here.** LLM work belongs in ai-backend (`ai.Service`). The backend polls `/v1/runs/:id`, and the executor never pushes.
- **Staging only.** Never accept a target URL from a request. Never add hard-coded credentials; case env files live in `EXECUTOR_ENV_DIR` and you must never print them.
- **Resource budget.** Keep concurrency at 1. Don't raise `MemoryMax=2560M` or `CPUQuota` (60 % of CPUs) in the unit template without the user's approval.
- **New run type.** Add it to `RUN_TYPES` in `src/types.ts`, write a handler in `src/runs/`, validate its params in `src/validate.ts`, and dispatch it in `server.ts` `makeHandler` (today only `case_run` exists). Then add the mapping in ai-backend `internal/api/jobs_executor.go` `executorRunTypes`.
