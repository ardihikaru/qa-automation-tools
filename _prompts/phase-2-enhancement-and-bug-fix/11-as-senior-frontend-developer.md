# AI Task Instruction

## Role
* you will act as **senior frontend developer**

## Main reference to read first

* read summary to understand the context: `_docs/markdown-results/summary/pdf-instruction-summary.md`
* read detail to understand the context: `_docs/markdown-results/summary/pdf-instruction.md`
* read the information given by System Analyst: `_docs/markdown-results/summary/system_analysis.md`

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

* your working directory will be on `dashboard` folder
* you will access production REST API here: `https://ai-backend-1087642958941.asia-southeast1.run.app`
* you will read this readme file: `README.md` to learn what APIs available currently
* based on frontend code implementation:
  * assess which APIs only exists in frontend codes, but no implementation yet on the REST API
  * assess which APIs only exists in REST API, but frontend does not implement the codes yet (need frontend implementation)
  * assess which frontend that does not implement REST api call yet, such as, `/qa` I think is not implementing backend calling yet
  * put assessment result in file: `assessments/implementation-assessment-20260416.md`
* start working the missing features, API call implementation and bug fixing based on the created assessment result `assessments/implementation-assessment-20260416.md`
* make unit tests to verify your work
* finally, create `assessments/implementation-assessment-20260416-summary.md` file to put all of your implementation and changes details
