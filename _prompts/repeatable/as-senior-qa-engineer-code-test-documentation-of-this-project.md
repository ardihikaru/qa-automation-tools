# AI Task Instruction

## Role
* you will act as **senior QA engineer**

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
- be comprehensive when building documentation

## Execution details

* we are working on this codebase: `_docs/this-qa-automation-system-playwright`
* you will run backend REST api in `ai-backend` folder
* you will run dashboard in `dashboard` folder
* please make sure that you are capable to run the video recording to report the bugs
  * read this tutorial: `tutorial-enabling-video-recording-playwright.md`
* you can use `psql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* make sure to run the backend system with Neon database connected
  * you will write down how to know whether database is connected on the running server or not yet
* make sure to run the backend system with MiniMax AI connected
  * you will write down how to know whether MiniMax AI integration is connected on the running server or not yet
* you will read the test plan to learn how to work: `_docs/this-qa-automation-system-playwright/TEST-PLAN.md`
* once test finished, put the test summary here: `_docs/this-qa-automation-system-playwright/test-results/test-summary-<this-current-datetime>.md`
  * this will include any found issues. we expect to use this summary later for the software engineer to fix it
