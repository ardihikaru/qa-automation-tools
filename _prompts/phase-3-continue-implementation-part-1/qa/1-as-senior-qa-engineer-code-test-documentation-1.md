# AI Task Instruction

## Role
* you will act as **senior QA engineer**

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
- be comprehensive when building documentation

## Execution details

* we are working on this codebase: `_docs/this-qa-automation-system-playwright`
* you will run backend REST api in `ai-backend` folder
* you will run dashboard in `dashboard` folder
* you can use `psql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* make sure to run the backend system with Neon database connected
  * you will write down how to know whether database is connected on the running server or not yet
* make sure to run the backend system with MiniMax AI connected
  * you will write down how to know whether MiniMax AI integration is connected on the running server or not yet
* the main goal here, you will write down playwright codes, including its all possible scenarios and flows into `_docs/this-qa-automation-system-playwright` directory
  * you may read this codebase as reference: `https://github.com/developersismedika/qa-automation-tools/blob/5ea6a46a0fe3fc0419563718f45bcf2a3dc96bf9/playwright-docs`
* we plan to use its own system to verify the usage of this QA automation system by using REST API + dashboard as the target of the "how this system works"
* put the README file there. as we will need to summarize the QA automation coverage summary in that readme file
