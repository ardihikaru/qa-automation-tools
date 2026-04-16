# AI Task Instruction

## Role
* you will act as **senior QA engineer**

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

* read `IMPLEMENTATION-PLAN.md` to see all possible APIs.
* read codebase under `ai-backend/internal/api/router.go` to get more understanding to the APIs
* use `playwright` to perform testing on all available APIs into production API: `https://ai-backend-1087642958941.asia-southeast1.run.app`
* don't forget to update README files regarding http files usage, and also playwright docs and results
* create `TEST-SCENARIO.md` to list the detailed scenario to test
  * file `TEST-SCENARIO.md` will be ignored from git
  * you will create template file: `TEST-SCENARIO.md.example`
* then, re-run playwright to make sure we identified and fixed issue we previously got in `TEST-RESULTS.md`
    * file `TEST-RESULTS.md` will be ignored from git
    * you will inform which API got issues
    * you will create template file: `TEST-RESULTS.md.example`
* we will not acknowledge NULL value on the response, if you found one. it will be backend bugs to solve. put them into `TEST-RESULTS.md` so will fix it later
* in case any error happens. do retry with alternative approach (you may act as senior software engineer when fixing the found bugs), max 4x retries per scenario
