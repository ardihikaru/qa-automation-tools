# AI Task Instruction

## Role
* you will act as **senior software engineer**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`

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
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* make sure to run the backend system with Neon database connected
* you are verifying backend implementation to support tasks mentioned in this document: `_docs/markdown-results/findings/202604/todo-for-dashboard.md`
  * you will only implement following tasks
    * Task 2: Fix Tracking View Enhancement (P0 - Critical)
    * forget about other task implementation for now
Current reports say that we need to:
  - Has basic filter by fixed/all/open
  - Has toggle fixed status
  - Has basic notes editing
* What's need to done in backend for Task 2 Requirements:
  1. Filter by category (All / Frontend / Backend / Unknown)
  2. Filter by search (page, action, error text)
  3. Sort options (date newest/oldest, severity high/low)
  4. Issue detail panel with full error text and suggestions
  5. Pagination UI
  6. Handle API response with pagination inf
* please make sure that backend has supported that requirements
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/backend-implementation-summary-task-11.md`
