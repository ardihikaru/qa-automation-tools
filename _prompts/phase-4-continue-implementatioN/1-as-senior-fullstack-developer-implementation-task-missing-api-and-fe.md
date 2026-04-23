# AI Task Instruction

## Role
* you will act as **senior fullstack developer**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`
* read this task TODO report: `_docs/markdown-results/findings/202604/dashboard-part-2-implementation-summary-task-all.md`

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

* we are working on this codebase: `ai-backend` for the backend API
* we are working on this codebase: `dashboard` for the frontend development
* you can use `psql` to connect into Neon database and other related credentials
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * read this to connect database: `_docs/markdown-results/tutorial-neon-database-connection.md`
  * make sure got this health check result: `{"status":"ok","database":"connected","minimax":"configured"}`
* on the **TODO report**, please focus ONLY in "Missing APIs for full server-driven completion" section.
  * use other section as refences only!
  * forget about other task implementation for now
* you will look up into the database first, to locate with tables we will work on it, for each API implementation
  * make a concrete plan for this
  * make mermaidjs to show the table relationship for each new API implementation
* you will implement backend API first
  * verify implementation with integration test
* then, you implement on the dashboard part and do integration with the implemented APIs
  * when got bug, fix it, and re-test it again
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/backend-part-2-implementation-summary-fullstack-1.md`
