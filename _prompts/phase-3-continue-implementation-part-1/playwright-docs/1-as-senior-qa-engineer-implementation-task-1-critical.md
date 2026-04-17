# AI Task Instruction

## Role
* you will act as **senior QA engineer**

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

* main reference is this documentation file: `_docs/markdown-results/findings/202604/todo-for-playwright-docs.md`
* we are working on this codebase: `playwright-docs`
* you can use `psql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* make sure to run the backend system with Neon database connected
* make sure to run the backend system with MiniMax AI REST API connected
  * test with "/api/search" endpoint
  ```shell
   //localhost:8080/api/issues

   # Search (requires q parameter)
   curl "http://localhost:8080/api/search?q=login"
  ```
* you will run backend/server locally, so you can test if directly
* you will only implement following tasks
  * Task 1: AI Narration Integration (P0 - Critical)
  * forget about other task implementation for now
* if there is any missing page, search feature, filter feature, you will implement it accordingly
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/playwright-docs-implementation-summary-task-1.md`
* if you found any bug on the API / backend side, expose the issue details, including which APIs, what payload used, and what's the response. store them in file: ``_docs/markdown-results/findings/202604/playwright-docs-implementation-summary-task-1-bugs.md``
  * up to 3x times retry iterations, if bugs persist, skip and go to next task / sub-tasks after filling the details in the doc
  * also add information, what's you detail tasks or sub-tasks here. in case BE do not implement it yet
