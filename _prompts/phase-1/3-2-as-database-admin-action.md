# AI Task Instruction

## Role
* you will act as **senior database administrator**

## Main reference to read first

* read summary to understand the context: `_docs/markdown-results/summary/pdf-instruction-summary.md`
* read detail to understand the context: `_docs/markdown-results/summary/pdf-instruction.md`
* read the information given by System Analyst: `_docs/markdown-results/summary/system_analysis.md`
* read the information regarding your role (you have done this): `_docs/markdown-results/by-roles/database-admin.md`

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

1. you will complete the `config.yaml` information as you will be given access to the staging database
2. you may only READ the data in the given database access (with SELECT action only)
3. in config file, section `STAGING ENVIRONMENT`, I added database connection string `mysql`. you will use `mysql` command to connect into the database
4. please go to `access_group` and find `acg_code` for the `role_code` you will enrich in section `credentials` later
5. build query to find active list of user (status_code = 'normal'). and simply take one login user per role. you can join these three tables: `access_group`, `user`, and `user_access_group`
6. make sure you queried and added user with username has prefix `KG%`. one role with one sample user added into `config.yaml` file 
7. summarize your actions and results inside `_docs/markdown-results/summary/configure-role-per-user.md`
