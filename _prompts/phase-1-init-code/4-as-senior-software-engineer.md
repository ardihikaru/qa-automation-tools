# AI Task Instruction

## Role
* you will act as **senior software engineer**

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

1. read the detailed task you are given to work on: `_docs/markdown-results/by-roles/senior-software-engineer.md`
2. follow detailed instruction given to you
3. you will assess the wrote codes in `ai-backend` folder according to your tasks
4. add unit tests
5. codes are planned to be deployed into Google Cloud run. you have my full access to `gcloud` command. you will make sure that the codes are compatible for that purpose
   * create a new project called `qa-automation-tools-sismedika` if does not exist yet
   * link billing
   * do other related action with permissions
