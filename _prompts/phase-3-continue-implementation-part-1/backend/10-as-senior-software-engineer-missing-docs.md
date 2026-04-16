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
* you can read how to create implementation summary here: `_docs/markdown-results/findings/202604/backend-implementation-summary-task-4.md`
* at this point, you have implemented all 9 backend tasks, but you are missing some implementation summary:
  * summarize your implementation summary in branch `feature/implementation-backend-task-1-critical` here: `_docs/markdown-results/findings/202604/backend-implementation-summary-task-1.md`
    * use git diff to read the changes carefully
  * summarize your implementation summary in branch `feature/implementation-backend-task-2-critical` here: `_docs/markdown-results/findings/202604/backend-implementation-summary-task-2.md`
    * use git diff to read the changes carefully
