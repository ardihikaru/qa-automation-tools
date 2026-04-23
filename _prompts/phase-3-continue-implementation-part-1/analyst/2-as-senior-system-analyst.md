# AI Task Instruction

## Role
* you will act as **senior system analyst**

## Main reference to read first

* read this latest proposed architecture: `proposed-system-architecture-v1-20260416.md`
* read finding: `_prompts/phase-3-continue-implementation-part-1/analyst/implementation-dashboard.md`

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

* you have full access to the backend and dashboard codebases here
  * backend: `ai-backend`
  * dashboard: `dashboard`
* in the finding markdown file, you have mapped the potential issues and un-implemented developments, based on the accessed frontend
* backend is currently running in: `http://localhost:8080`
  * the backend system is having Neon database connected
  * the backend system is having MiniMax AI connected
* frontend is currently running in: `http://localhost:3000`
* you can use `psql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* based on the database credential provided, you may verify the data on the database by yourselves 
* you will create a technical analysis by using this data as the format output: `_docs/markdown-results/by-roles/system-analyst.md`
  * put the result into this file: `_docs/markdown-results/findings/202604/new-analysis-20260423.md`
* you will also create separated file as the detailed task for backend (Senior Software Engineer) and frontend engineer (Senior Frontend Engineer):
  * todo list for Senior Software Engineer: 
    * use this format: `_docs/markdown-results/findings/202604/todo-for-backend.md`
    * put the todo list here: `_docs/markdown-results/findings/202604/20260423-todo-for-Senior-Software-Engineer.md`
  * todo list for Senior Frontend Engineer:
    * use this format: `_docs/markdown-results/findings/202604/todo-for-dashboard.md`
    * put the todo list here: `_docs/markdown-results/findings/202604/20260423-todo-for-Senior-Frontend-Engineer.md`
* summarize your implementation summary here: `_docs/markdown-results/findings/202604/20260423-system-analysis.md`
* if you found any bug on the API / backend side, expose the issue details, including which APIs, what payload used, and what's the response.
  * store them in file: `_docs/markdown-results/findings/202604/20260423-system-analysis-bugs.md`
  * up to 3x times retry iterations, if bugs persist, skip and go to next task / sub-tasks after filling the details in the doc
  * also add information, what's you detail tasks or sub-tasks here. in case BE do not implement it yet
