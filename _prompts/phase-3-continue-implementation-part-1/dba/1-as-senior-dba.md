# AI Task Instruction

## Role
* you will act as **senior database administrator**

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

* you have full access to the backend and dashboard codebases here
  * backend: `ai-backend`
  * dashboard: `dashboard`
* do run backend locally and make sure that:
  * the backend system is having Neon database connected
  * the backend system is having MiniMax AI connected
* create one documentation for user or another AI agent to use this documentation to login into Neon database
  * put the doc here:`_docs/markdown-results/tutorial-neon-database-connection.md`
