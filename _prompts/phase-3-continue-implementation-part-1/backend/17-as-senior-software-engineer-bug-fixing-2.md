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
* make sure to run the backend system with MiniMax AI connected
* ok now: when accessing `/api/issues?limit=20&page=1&sort_by=date_newest&is_fixed=true` it can show fixed information, which is correct
* but, we got bug when accessing `/api/issues?limit=20&page=1&sort_by=date_newest&is_fixed=false` it shows no result, but we actually have data opened now
  * same bug happens on `/api/issues?limit=20&page=1&sort_by=date_newest`
* once fixed, put the summary logs into `_docs/markdown-results/findings/202604/backend-summary-bug-task-2.md`
