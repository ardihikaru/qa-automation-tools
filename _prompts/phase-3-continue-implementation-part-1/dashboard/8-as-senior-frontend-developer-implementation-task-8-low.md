# AI Task Instruction

## Role
* you will act as **senior frontend developer**

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

* main reference is this documentation file: `_docs/markdown-results/findings/202604/todo-for-dashboard.md`
* we are working on this codebase: `dashboard`
* you may read this http files to understand what REST APIs that have been implemented so far: `ai-backend/resources`
* you can use `psql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* make sure to run the backend system with Neon database connected
* you will run backend/server locally, and frontend (dashboard) locally, so you can test if directly
* you will only implement following tasks
  * Task 8: Learning Progress Visualization (P3 - Low)
  * forget about other task implementation for now
* if there is any missing page, search feature, filter feature, you will implement it accordingly
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/dashboard-implementation-summary-task-6.md`
* if you found any bug on the API / backend side, expose the issue details, including which APIs, what payload used, and what's the response.
  * store them in file: ``_docs/markdown-results/findings/202604/dashboard-implementation-summary-task-6-bugs.md``
  * up to 3x times retry iterations, if bugs persist, skip and go to next task / sub-tasks after filling the details in the doc
  * also add information, what's you detail tasks or sub-tasks here. in case BE do not implement it yet
