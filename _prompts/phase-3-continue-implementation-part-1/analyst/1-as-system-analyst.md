# AI Task Instruction

## Role
* you will act as **senior system analyst** with 20 years of experiences

## Main reference to read first

* read this database's tables: `_docs/markdown-results/by-roles/database-admin.md`
* read your last finding: `_docs/markdown-results/findings/202604/20260416.md`
* read system architecture: `proposed-system-architecture-v1-20260416.md`
* read all available APIs: `rest-api-usage-documentation.md`
* read API usage: `ai-backend/resources`

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

* based on your last finding, please re-assess the implementation of the `ai-backend` and `dashboard` part
* you will have access to the real credentials inside `config.yaml`. you can login into database with `pgsql` command
  * when I checked on the database, it has four schemas: (1) `ai`, (2) `logging`, (3) `public`, and (4) `qa`.
  * please explain what each schema is used for
* you can use `psql` to connect into Neon database and other related credentials
  * IMPORTANT: database we are talking about in this project is only **Neon database**
  * you can also found Neon related config here: `_docs/markdown-results/by-roles/.env`
  * you may find other related config here: `config.yaml`
* make sure to run the backend system with Neon database connected
  * you will write down how to know whether database is connected on the running server or not yet
* make sure to run the backend system with MiniMax AI connected
  * you will write down how to know whether MiniMax AI integration is connected on the running server or not yet
* you may run both backend and frontend locally if you need
* we are looking to any possible missing implementation
* you will go through all the codes in `dashboard` to verify that each available button can already trigger calling API call request into backend REST API
* you will be creating assessment result to `_docs/markdown-results/findings/202604/20260418-system-assessment-results.md`
