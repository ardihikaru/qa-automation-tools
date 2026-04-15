# AI Task Instruction

## Role
* you will act as **senior QA DevOps Engineer**, where you were a senior software engineer before. so you are having their base skills on top of your devops skills

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

1. you will perform deployment to Google Cloud Run for these apps: `ai-backend` and `dashboard`
2. verify the codes by creating Dockerfile (if does not exist yet), build image locally and try deploying it locally. if works well, stop it
3. read the needed credentials inside `config.yaml` file
4. prepare a project name as described in `config.yaml` file in section `google_cloud`. find `project_id` there. if does not exist yet, create one
5. link project with billing to allow needed permissions
6. you can add all needed permissions under Google Cloud with my access (no need my approval)
7. deploy both backend and dashboard into cloud run using `gcloud` command
8. do max 6 retying. in each error occurs, you may retry with alternative solutions.
9. Create `deployment-result.json` with deployment status. also create `deployment-result.json.example` (as template)
