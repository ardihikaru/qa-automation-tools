# Contributing

Thanks for your interest! Bug reports, docs fixes and pull requests are all welcome.

## Project layout

| Path | What | Build / test |
|---|---|---|
| `ai-backend/` | Go 1.23 REST API, MySQL 8, LLM facade | `cd ai-backend && go build -mod=mod ./... && go test -mod=mod ./...` |
| `dashboard/` | React 18 + Vite + Tailwind SPA | `cd dashboard && npm ci && npm run build` |
| `playwright-executor/` | Node service, the only Playwright runtime | `cd playwright-executor && npm ci && npm test` |
| `this-qa-automation-system-playwright/` | E2E tests of this system (needs a running stack) | `npx playwright test` |
| `deploy/` | Reference Docker Compose stack and systemd unit | see each README |

Every directory has a `README.md` that works as its **contract** (interface, invariants, side
effects). If your change alters one of those, update that README in the same PR.

## Workflow

1. Fork the repo, or create a branch off `main`. Maintainers use git worktrees:
   `git worktree add ../qa-automation-tools.worktrees/<branch> -b <branch> main`.
2. Use [Conventional Commits](https://www.conventionalcommits.org/): `feat(dashboard): …`, `fix(executor): …`, `docs: …`.
3. Keep PRs small and focused. Fill in the PR template. CI must pass.
4. Bigger features start as a design doc in `docs/designs/` (see `docs/designs/SITEMAP.md`).

## Ground rules

- **Go:** always pass `-mod=mod`. `ai-backend/vendor/` is stale and ignored.
- **Schema changes** are appended to `ai-backend/internal/db/migrations.go` as idempotent statements,
  and applied with `go run -mod=mod ./cmd/databasemigration`. The server never migrates on its own.
- **LLM calls** go only through `ai.Service` / `ChatProvider` in `ai-backend/internal/services/ai/`.
  Don't add direct provider SDK calls elsewhere.
- **Playwright** runs only inside `playwright-executor`, and only against staging targets.
- **No secrets** in commits: no `.env*`, `config.yaml`, tokens or passwords. Use the `.example` templates.
- **No real infrastructure names.** Docs use generic roles (`edge-host`, `app-host`, `db-host`),
  documentation IP ranges (`10.8.0.x`, `203.0.113.x`), and `example.com` / `example-org` placeholders.

## Private maintainer overlay

The maintainer's real deployment details (host names, domains, IPs and infra runbooks) are kept out
of this public repo, in two gitignored places at the repo root:

- `.private/`: `README.md` (overview, sync and new-VM checklist), `name-map.md` (generic → real
  names), `infra.md`, and `originals/` (a pre-publication snapshot of the docs).
- `CLAUDE.local.md`: loaded automatically by Claude Code. It tells AI agents to read `.private/` first.

If you maintain a deployment, keep both on every machine that has a checkout (workstation and
deployed VMs), and copy them when you set up a new VM. Contributors without them can ignore this
section: everything builds and tests without them.

## Code of Conduct

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
