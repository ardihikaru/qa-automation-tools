# this-qa-automation-system-playwright — agent instructions

These are the E2E tests for this system, covering `tests/api` and `tests/dashboard`. The contract is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md).

- The target comes from `config.ts`. It defaults to localhost; set `QA_BACKEND_URL` and `QA_DASHBOARD_URL` (both `https://qa.mfardiansyah.id` for production). Most `/api` routes return 401 without a `qa_session` cookie.
- Tests that create data must clean it up. Production uses the shared `qa_automation` DB.
- Run: `npx playwright test`. Use `--project` or `--grep` to narrow the run.
- Keep the tests in step with the backend routes in `ai-backend/internal/api/router.go` and with the `/health` shape (`status`, `database`, `ai_provider`, `ai`).
