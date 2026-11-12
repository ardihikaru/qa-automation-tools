---
name: qa-automation-elephant
description: Load deep project context for qa-automation-tools (the HISv3 QA automation system) before any design, brainstorm, spec drafting, architecture discussion or cross-component change. This is the EGM "Elephant" context skill. Use it at the start of /egm:egm-brainstorm or /egm:egm-draft, and whenever a task touches more than one component.
---

# qa-automation Elephant context

You are the **Elephant**: the agent that carries the long-lived context of this project. Read the sources below in order, and only as deep as the task needs. Then work from them, not from memory.

## 1. What the system is
- [OBJECTIVES.md](../../../OBJECTIVES.md): why the system exists, and its HISv3 staging target.
- [README.md](../../../README.md): the system Blueprint, with component boundaries and global invariants.
- [proposed-system-architecture.md](../../../proposed-system-architecture.md): the original architecture proposal. It is historical, and parts of it (Neon, Cloud Run, MiniMax-direct) are superseded.
- [docs/designs/SITEMAP.md](../../../docs/designs/SITEMAP.md): the approved specs. These outrank older docs.

## 2. Component contracts (EGM README files)
- [ai-backend/README.md](../../../ai-backend/README.md), with sub-contracts for `internal/api`, `auth`, `db`, `config`, `services/ai`, `services/skills` and `services/learning`.
- [dashboard/README.md](../../../dashboard/README.md)
- [playwright-qa/README.md](../../../playwright-qa/README.md)
- [playwright-docs/README.md](../../../playwright-docs/README.md)
- [this-qa-automation-system-playwright/README.md](../../../this-qa-automation-system-playwright/README.md)
- [deploy/his-v3-qa/README.md](../../../deploy/his-v3-qa/README.md): the production topology.

## 3. Key code paths
| Concern | Path |
|---|---|
| HTTP entry and wiring | `ai-backend/cmd/server/main.go` |
| Routes (one switch) | `ai-backend/internal/api/router.go`, in `dispatchAuthenticated` |
| Auth (cookie sessions) | `ai-backend/internal/auth/`, `ai-backend/internal/api/auth_handlers.go` |
| Schema | `ai-backend/internal/db/migrations.go` |
| Config and env | `ai-backend/internal/config/config.go`, `lib/config.ts` |
| LLM seam | `ai-backend/internal/services/ai/provider.go`, `aimanager.go`, `minimax.go`, `aiService.go` |
| Skills / learning | `ai-backend/internal/services/skills/`, `ai-backend/internal/services/learning/` |
| Dashboard auth | `dashboard/src/auth/` |
| HISv3 crawler | `playwright-qa/src/explorer/` |

## 4. Durable decisions (see the specs for the reasoning)
- **The LLM goes only through ai-manager, pinned to `claude-v3`.** ai-manager is the production gateway at `http://10.10.0.4:8099`, reached over WireGuard with Basic auth. Its `claude-v3` upstream is the Claude CLI REST on rnd-v3 (`10.10.0.9:8105`, `claude-opus-5`). Calls use `no_fallback`, and the client rejects any reply from a different provider. MiniMax-direct is a manual alternative (`AI_PROVIDER=minimax`). There is no automatic failover in this repo; provider failover happens inside ai-manager. Spec: [AI_MANAGER_AND_SELF_HOSTED_DEPLOY.md](../../../docs/designs/AI_MANAGER_AND_SELF_HOSTED_DEPLOY.md).
- **The DB** is MySQL 8, schema `qa_automation`, on the shared rnd-v2 master-compose MySQL. Neon/Postgres is gone.
- **Hosting** is self-hosted Docker Compose on rnd-v3-vpn, behind the clino TLS edge. Cloud Run is legacy.
- **Auth** uses cookie sessions, and is active only when the DB connects.
- **Embeddings** are placeholders: `GenerateEmbedding` is hash-based. Any similarity feature built on them is not semantically meaningful yet.

## 5. Known debt (check before proposing work)
- `router.go` is a single file of about 3k lines, with gofmt debt.
- `GenerateSuggestion` parses its LLM output with a fragile `Sscanf`.
- `ai-backend/vendor/` is stale; use `-mod=mod`.
- `playwright-docs` has a TS6059 `rootDir` error caused by importing `../lib`.

## 6. How to work here
Follow the EGM loop in the root `CLAUDE.md`. Drafts go in `.dev/docs/`. Approved specs go in `docs/designs/`, and each one gets a `SITEMAP.md` row. Whenever a Goldfish review must challenge an assumption from this context, write that assumption into the review criteria explicitly, because the reviewers never see this skill.
