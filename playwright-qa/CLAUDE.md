# playwright-qa — agent instructions

This is the explorer (random, guided, planner), logger, runner and skills engine that crawls HISv3 staging and reports to the backend at `API_URL`. The contract is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md).

- The backend is behind cookie auth. A run against https://his-v3.qa.sismedika.com must log in first via `POST /api/auth/login` and reuse the `qa_session` cookie.
- Only crawl HISv3 **staging**. Write-actions against production HISv3 are forbidden.
- Never write learned skills or issues straight to MySQL. Always go through the backend API.
- Type check: `npx tsc --noEmit`.
