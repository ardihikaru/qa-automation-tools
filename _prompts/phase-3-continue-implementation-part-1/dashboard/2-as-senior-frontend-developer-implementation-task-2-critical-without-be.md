# AI Task Instruction

## Role
* you will act as **senior frontend developer**

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
* BE docs for this task has been prepared here: `_docs/markdown-results/findings/202604/backend-implementation-summary-task-11.md`
* we are working on this codebase: `dashboard`
* you can access backend in: `http://localhost:8080`
  * make sure to run the backend system with Neon database connected
  * make sure to run the backend system with MiniMax AI REST API connected
    * test with "/api/search" endpoint
    ```shell
     http://localhost:8080/api/issues

     # Search (requires q parameter)
     curl "http://localhost:8080/api/search?q=login"
    ```
* you will only implement following tasks
  * Task 2: Fix Tracking View Enhancement (P0 - Critical)
  * forget about other task implementation for now
* if there is any missing page, search feature, filter feature, you will implement it accordingly
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/dashboard-implementation-summary-task-2.md`
* if you found any bug on the API / backend side, expose the issue details, including which APIs, what payload used, and what's the response. store them in file: ``_docs/markdown-results/findings/202604/dashboard-implementation-summary-task-2-bugs.md``
  * up to 3x times retry iterations, if bugs persist, skip and go to next task / sub-tasks after filling the details in the doc
  * also add information, what's you detail tasks or sub-tasks here. in case BE do not implement it yet
* Current State of `FixTracking.tsx`:
  - Has basic filter by fixed/all/open
  - Has toggle fixed status
  - Has basic notes editing
* What's Missing (Task 2 Requirements):
  1. Filter by category (All / Frontend / Backend / Unknown)
  2. Filter by search (page, action, error text)
  3. Sort options (date newest/oldest, severity high/low)
  4. Issue detail panel with full error text and suggestions
  5. Pagination UI
  6. Handle API response with pagination inf
* the implementation plan:
  1. **Add state for filters**: category, search, sortBy, pagination
  2. **Update API call** to include all filter parameters
  3. **Add filter bar** with category dropdown, search input, sort dropdown
  4. **Add issue detail panel** that shows when clicking on an issue
  5. **Add pagination UI** at the bottom
  6. **Handle the new response format** with data and pagination
