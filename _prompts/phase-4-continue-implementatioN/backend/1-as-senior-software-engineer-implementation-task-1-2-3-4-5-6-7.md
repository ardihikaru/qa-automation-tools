# AI Task Instruction

## Role
* you will act as **senior software engineer**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`
* read this bug report: `_docs/markdown-results/findings/202604/20260423-system-analysis-bugs.md`
* read this task TODO report: `_docs/markdown-results/findings/202604/20260423-todo-for-Senior-Software-Engineer.md`

## Execution Strategy

You MUST follow this 2-phase approach:

### Phase 1: Planning
- Analyze the task carefully
- Break down into clear implementation steps
- Identify risks and edge cases
- Output a structured plan BEFORE coding

### Phase 2: Execution
- Implement based on the approved plan
- Write clean, maintainable code
- Add unit tests where applicable
- Ensure code compiles and works correctly

Do NOT skip Phase 1.

## Rules
- be direct

## Execution details

* we are working on this codebase: `ai-backend`
* you can use `psql` to connect into Neon database and other related credentials
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * read this to connect database: `_docs/markdown-results/tutorial-neon-database-connection.md`
  * make sure got this health check result: `{"status":"ok","database":"connected","minimax":"configured"}`
* you will only implement following tasks
  * Task 1) Fix `POST /api/learning` existing-record update bug
  * Task 2) Enforce affected-row validation on issue update endpoints
  * Task 3) Add server-side query contract for skills list
  * Task 4) Add server-side docs query support
  * Task 5) Data cleanup + guardrails for `raw_issues`
  * Task 6) Add backend job endpoints for QA actions
  * Task 7) Improve consistency across schemas (`public`, `qa`, `ai`, `logging`)
  * use bug report above as detailed references
  * forget about other task implementation for now
* please verify your work by performing **integration test** on each updated or new APIs
  * when got bug, fix it, and re-test it again
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/backend-part-2-implementation-summary-task-1.md`
