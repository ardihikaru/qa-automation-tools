# AI Task Instruction

## Role
* you will act as **senior software engineer**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`
* read this task TODO report: `_docs/markdown-results/findings/202604/20260423-todo-for-Senior-Frontend-Engineer-missing-404.md`

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
  * work on the documentated task report as referenced
  * forget about other task implementation for now
* please verify your work by performing **integration test** on each updated or new APIs
  * when got bug, fix it, and re-test it again
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/backend-part-2-implementation-summary-404-report.md`
