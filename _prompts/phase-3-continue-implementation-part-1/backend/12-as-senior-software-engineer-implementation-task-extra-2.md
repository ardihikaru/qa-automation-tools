# AI Task Instruction

## Role
* you will act as **senior software engineer**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`

## Codex / Minimax AI CLI extra command can be used
```shell
+ rg
+ fd
+ ast-grep
```

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
* we got report with this results:
```shell
Scan error: can't scan into dest[8]: cannot scan NULL into *string
```
* due to this, we suggest to re-run all existing APIs: `ai-backend/internal/api/router.go`
  * deepen the testing with all possible scenarios; `IMPLEMENTATION-PLAN.md`. we will need to update this document accordingly
  * run playwright to test all possible scenarios
  * we will add extra test scenarios in our unit tests
  * we will perform integration tests with real backend running including Neon database dan MiniMax AI rest api connected (verify this as well)
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/backend-implementation-summary-extra-task-2.md`
* for playwright result, put here: `TEST-RESULTS.md`
