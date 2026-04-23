# AI Task Instruction

## Role
* you will act as **senior frontend engineer**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`
* read this bug report: `_docs/markdown-results/findings/202604/20260423-system-analysis-bugs.md`
* read this task TODO report: `_docs/markdown-results/findings/202604/20260423-todo-for-Senior-Frontend-Engineer.md`

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

* we are working on this codebase: `dashboard`
* you will run backend locally based on codebase `ai-backend`
* you can use `psql` to connect into Neon database and other related credentials
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * read this to connect database: `_docs/markdown-results/tutorial-neon-database-connection.md`
  * make sure got this health check result: `{"status":"ok","database":"connected","minimax":"configured"}`
* you will only implement following tasks
  * Task 1) Implement server-side search/filter/pagination for `/docs`
  * Task 2) Wire `/qa` quick action buttons to backend jobs
  * Task 3) Complete `/skills` server-driven table behavior
  * Task 4) Normalize `/issues` search trigger behavior
  * Task 5) Replace full-page spinner with skeleton-first rendering
  * Task 6) Improve data quality fallback rendering
  * Task 7) Add action audit UI for quick actions
  * use bug report above as detailed references
  * forget about other task implementation for now
* you can read following report and documentation to find backend implementation
  * read new development report #1: `_docs/markdown-results/findings/202604/backend-part-2-implementation-summary-task-1.md`
  * read new development report #2: `_docs/markdown-results/findings/202604/backend-part-2-implementation-summary-task-all.md`
  * most of the implemented APIs and its usage are stored in http files here: `ai-backend/resources`
* please verify your work by performing **integration test** on each updated or new APIs
  * when got bug, fix it, and re-test it again
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/dashboard-part-2-implementation-summary-task-all.md`
  * if any missing APIs, inform expected API (GET/POST/PUT/DELETE) and its routing, e.g. `GET /api/<some-routing-here>`. expected payload and expected response
