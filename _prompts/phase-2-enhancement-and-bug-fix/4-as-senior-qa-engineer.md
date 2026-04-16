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

* use `playwright` to perform testing on all available APIs. see JSON file here: `deployment-result.json`
* dont forget to update README files regarding http files usage, and also playwright docs and results
* re-read the codes under `ai-backend` folder to find out how to provide the additional authentication in the test scenario
* create `TEST-SCENARIO.md` to list the detailed scenario to test
  * file `TEST-SCENARIO.md` will be ignored from git
  * you will create template file: `TEST-SCENARIO.md.example`
* then, re-run playwright to make sure we identified and fixed issue we previously got in `TEST-RESULTS.md`
    * file `TEST-RESULTS.md` will be ignored from git
    * you will create template file: `TEST-RESULTS.md.example`
