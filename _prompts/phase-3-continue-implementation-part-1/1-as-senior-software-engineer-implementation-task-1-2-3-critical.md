# AI Task Instruction

## Role
* you will act as **senior software engineer**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`
* read this summarized finding: `_docs/markdown-results/findings/202604/20260416.md`

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
* you are granted the access of the `config.yaml` file. you can use `mysql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
* make sure to run the backend system with Neon database connected
* you will only implement following tasks
  * Task 1: Learning Cache API Implementation (P0 - Critical)
  * Task 2: Skill Logs API Implementation (P0 - Critical)
  * Task 3: Goal Events API Implementation (P0 - Critical)
  * forget about other task implementation for now
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/backend-implementation-summary-task-1.md`
