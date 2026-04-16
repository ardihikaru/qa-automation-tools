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

1. I found issue that once I deployed it, the connection to the APIs does not work yet. it is due to the in the dependency injection, it fails to connect with the real database
2. I prepared the needed credentials in `config.yaml` file. 
3. what we expect here:
   * system will still be able to run even without database connection. since we will need it running and utilize `/health` endpoint to check deployment status (when no real database connection prepared in env variables)
   * in the real deployment (in Google Cloud run), the codes need to make sure that it is running with database connection ready and is connected properly with Neon Database
4. We also need to add unit test into each endpoint's main handler. to make sure that it is capable to response it as expected
5. try validating the fixed locally first, with real database connection applied, then test calling each endpoint with `CURL` action
   * it is STRONGLY suggested to create http files. please create `resources` folder. inside, it contains mainly `*.http` files
   * you can read this files inside this folder as reference: `/home/ardi/devel/projects/sismedika/his-vendor-backend/resources`. thought this is an external project, you can still the code structure here
   * dont forget to add `http-client.env.json` (exclude from git) and its template `http-client.env.json.example` inside `resources` folder. it is config file used my GoLand code editor to run http files.
