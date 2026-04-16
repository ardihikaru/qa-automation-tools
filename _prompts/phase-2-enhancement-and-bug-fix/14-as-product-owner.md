# AI Task Instruction

## Role
* you will act as **senior product owner** with 20 years of experiences

## Main reference to read first

* read summary to understand the context: `_docs/markdown-results/summary/pdf-instruction-summary.md`
* read detail to understand the context: `_docs/markdown-results/summary/pdf-instruction.md`
* read the information given by System Analyst: `_docs/markdown-results/summary/system_analysis.md`
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
- to make a very comprehensive documentations

## Execution details

* you assessment will mainly based on the **proposed architecture** and **summarized finding**
* you will create what tasks todo for the team into `_docs/markdown-results/findings/202604/todo-20260417.md` file, then technically split them into separated detailed instruction:
  * (1) senior software engineer to work on `ai-backend` - into file: `_docs/markdown-results/findings/202604/todo-for-backend.md` file
  * (2) senior frontend developer to work on `dashboard` - into file: `_docs/markdown-results/findings/202604/todo-for-dashboard.md` file
  * (3) senior QA engineer to work on `playwright-docs` and `playwright-qa` - into file: `_docs/markdown-results/findings/202604/todo-for-playwright-dos.md` and `_docs/markdown-results/findings/202604/todo-for-playwright-qa.md` files
