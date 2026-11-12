# Design Sitemap

An index of approved design specifications (EGM). Drafts live in `.dev/docs/` (gitignored) until they are approved.

| Link | Date | Author | Components | Description |
|---|---|---|---|---|
| [AI_MANAGER_AND_SELF_HOSTED_DEPLOY.md](AI_MANAGER_AND_SELF_HOSTED_DEPLOY.md) | 2026-11-13 | ardihikaru | ai-backend, playwright-docs, lib, deploy | Route all LLM calls through ai-manager over WireGuard; move the DB to db-host MySQL; self-host on app-host at qa.mfardiansyah.id |
| [PLAYWRIGHT_EXECUTOR.md](PLAYWRIGHT_EXECUTOR.md) | 2026-11-13 | ardihikaru | playwright-executor, ai-backend, deploy | All Playwright runs move to a systemd/mise service on app-host capped at 2.5 GiB / 60 % CPU; real `/api/jobs` dispatch; claude-v3 via ai-manager is the only LLM path (MiniMax dormant) |
| [PRODUCTS_MASTER_AND_REPO_POOL.md](PRODUCTS_MASTER_AND_REPO_POOL.md) | 2026-11-13 | ardihikaru | ai-backend, dashboard, deploy | Products master list (repo, staging URL, root dir) with a git repo pool: gh clone, branch switch, on-demand and scheduled Playwright-accessibility verify |
