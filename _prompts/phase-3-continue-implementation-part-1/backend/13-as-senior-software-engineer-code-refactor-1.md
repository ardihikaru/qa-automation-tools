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
- be comprehensive when building documentation

## Execution details

* we are working on this codebase: `ai-backend`
* you can use `psql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* make sure to run the backend system with Neon database connected
  * you will write down how to know whether database is connected on the running server or not yet
* make sure to run the backend system with MiniMax AI connected
  * you will write down how to know whether MiniMax AI integration is connected on the running server or not yet
* the main goal here, you will make split out the scripting to database migration into separated script `ai-backend/cmd/databasemigration`
* finally you will create documentation `database-migration.md` on how it works when any developer want to add new column, alter column, add new tables, etc
  * write down comprehensive stepper on it. include when we want to deploy it into the production (Google Cloud Run)
