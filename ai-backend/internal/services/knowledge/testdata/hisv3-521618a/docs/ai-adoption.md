# Adopting AI coding tools in this repository

> **ai-manager:** tooling AI repo ini memakai `ai-manager` dengan urutan provider MiniMax-M3 (minimax-cli) → claude-sonnet-5-5 → gemini-3.8-flash-medium (antigravity); lihat `src/ai/ai-manager.client.ts`. Hanya untuk authoring katalog/test, tidak pernah dijalankan saat test berjalan.

Audience: QA engineers, SDETs and leads. The rules the AI must follow are in [`../AGENTS.md`](../AGENTS.md);
this document explains **how the team sets up and works with** AI tools on top of those rules.

## 1. Which tool reads which file

| Tool                                                | Reads                                            | Set up in this repo                                                |
| --------------------------------------------------- | ------------------------------------------------ | ------------------------------------------------------------------ |
| Claude Code                                         | `CLAUDE.md` (does not read `AGENTS.md` natively) | `CLAUDE.md` contains `@AGENTS.md` (import) + Claude-specific notes |
| GitHub Copilot (IDE, code review, cloud agent, CLI) | `.github/copilot-instructions.md`, `AGENTS.md`   | Both present; Copilot file points to AGENTS.md                     |
| OpenAI Codex, Cursor, Jules, Aider, RooCode, Zed, … | `AGENTS.md`                                      | Nothing extra needed                                               |

One source of truth: **edit `AGENTS.md`**, not the tool-specific files. Nested `AGENTS.md` files are allowed (e.g. `mobile/AGENTS.md`)
for folder-specific rules; the closest one to the edited file takes precedence in tools that support nesting.

Personal preferences stay local: `CLAUDE.local.md` and `.claude/settings.local.json` are git-ignored.

## 2. Playwright-specific AI tooling

### 2.1 Playwright Test Agents (planner → generator → healer)

Built into Playwright since 1.56:

- **planner** explores the running app and writes a Markdown test plan
- **generator** turns the plan into Playwright test files
- **healer** runs failing tests and proposes fixes

```bash
npx playwright init-agents --loop=claude    # Claude Code  → .claude/agents/ + .mcp.json
npx playwright init-agents --loop=vscode    # VS Code / Copilot agent mode
npx playwright init-agents --loop=copilot   # GitHub Copilot
```

Team rules for the agents:

1. Run `init-agents` on a branch and **review the diff** — it can overwrite `.mcp.json`. Regenerate after every Playwright upgrade.
2. Point the planner at `TEST_ENV=local` (mock or local services), never at staging with real QA accounts in the prompt.
3. Generator output must be **refactored into this structure** before merge: page objects in `src/web/**`, clients in `src/api/**`,
   `test` imported from `@fixtures`, factories + cleanup. Raw generated specs are drafts, not deliverables.
4. Healer suggestions follow AGENTS.md §6.5: a healed locator is fine; a changed **expectation** needs a human decision (app bug vs test bug).

### 2.1b Repository agents

`.claude/agents/` also holds `test-case-planner`, `e2e-test-engineer`, `integration-test-advisor` and `unit-test-advisor`
(see [test-strategy.md](test-strategy.md) §5). Their names differ from the `playwright-test-*` agents, so `init-agents` does not overwrite them.

### 2.2 Playwright MCP

The Playwright MCP server lets an agent drive a real browser (accessibility snapshots, clicks, navigation) to discover locators
and flows. Use it for exploration and locator discovery; the resulting code still has to follow AGENTS.md.

## 3. Recommended workflow (human in the loop)

```text
1. Describe   → ticket / acceptance criteria / API spec (OpenAPI, GraphQL schema) given to the agent
2. Plan       → agent proposes test cases + files to touch            ← human approves the plan
3. Generate   → agent writes code inside the structure
4. Verify     → agent runs `npm run check` + the specs 3×             ← must be real output, not claimed
5. Review     → human reviews the PR with the checklist below          ← label `ai-assisted`
6. Learn      → if the agent repeated a mistake, add one line to AGENTS.md
```

## 4. Prompt templates

**New REST endpoint tests**

```text
Read AGENTS.md. Add backend E2E tests for POST /api/v1/orders/{id}/refund in order-service.
Acceptance criteria: <paste>. Follow the "Add tests for an existing REST endpoint" recipe.
Cover happy path, 401, 403, 400, 404, 409 (already refunded). Use factories and cleanup.
Run npm run check and the new spec with --repeat-each=3, then report files changed and real results.
```

**New GraphQL operation**

```text
Read AGENTS.md. In catalog-gateway add the `searchProducts(term: String!, first: Int)` query.
Schema excerpt: <paste SDL>. Add the operation, a zod-validated client method and specs
(happy path, empty result, validation error via execute()). Do not change src/api/core.
```

**Frontend flow connected to backend**

```text
Read AGENTS.md. For customer-portal, cover "customer cancels an order from My orders".
Arrange the order via orderApi, act in the UI, assert UI status and the order status via orderApi.
Create/extend page objects; propose data-testid additions if no accessible locator exists.
```

**Fix a failing test**

```text
Read AGENTS.md §6.5. tests/web/admin-console/products.spec.ts fails on staging (trace attached).
Find the root cause. If the app is wrong, do not change the expectation — mark test.fail() with
the ticket and explain. Do not add retries, sleeps or force.
```

## 5. Review checklist for AI-generated tests

- [ ] Does each test assert the **behaviour from the ticket**, not just "page loads"? (No assertion-free or tautological tests.)
- [ ] Imports from `@fixtures`, clients and page objects used — no raw selectors/requests in specs
- [ ] Locators are user-facing (`getByRole`/`getByLabel`), no XPath, no `nth()` guessing
- [ ] No `waitForTimeout`, `networkidle`, `force: true`, inflated timeouts, extra retries
- [ ] Data via factories, unique, cleaned up; no dependence on existing data
- [ ] Negative/authorization cases present for API changes
- [ ] No secrets, real emails/phones/IDs, production URLs
- [ ] No unrelated changes, no new dependencies, no config edits without approval
- [ ] CI is green, and the author ran the tests (not only the agent's claim)

## 6. Governance

- Use only company-approved AI tools and accounts. Do not paste secrets, `.env` files, session files, customer data or
  proprietary source outside approved tools.
- AI-assisted PRs get the `ai-assisted` label (PR template checkbox). The human who opens the PR owns the code.
- `CODEOWNERS` still applies: framework code (`config/`, `src/fixtures/`, `src/api/core/`) requires QA-platform review.
- Track simple metrics per quarter: % AI-assisted PRs, review rework, flaky-test rate. Adjust AGENTS.md based on what goes wrong.

## 7. Keeping AGENTS.md useful

- Short, concrete, verifiable rules beat long essays. If a rule can be enforced by ESLint/TypeScript, enforce it there and keep one line here.
- Update it in the same PR that changes a convention.
- When an agent makes the same mistake twice, add a rule (that is the cheapest fix).
