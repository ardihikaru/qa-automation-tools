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

* we are working on this codebase: `ai-backend`
* I found that there are two endpoint that has problems
```shell
APIs Not working:
  - /api/search - returns 404 (need to check)
  - /api/cost/overview - NULL scan error
```
* you can use `pdf_extract.py` to extract and read PDF file: `MiniMax-API-Endpoint.pdf` to add knowledge regarding usage of MiniMax AI's rest api
* you may read the config file: `config.yaml` to get minimax AI credential
* that endpoints maybe related with minimax AI integration problems, so we will go deeper in this integration issue first:
  * in `Minimax AI` section, I updated `api_key` value
  * I don't know about what value to use for `model` and `embedding_model`. you know better than me
  * as for `base_url`, please help me to make sure whether that will be needed or not. and verify it with the real testing scenario
  * please make a small script to verify usage of minimal AI with that. put in here: `ai-backend/cmd` (fix it, if it is existed)
* then, the code implementation of APIs `/api/search` and `/api/cost/overview` according to the technical documentation you read under `Main reference to read first` above
* finally, perform an integration test by running it locally, and re-test those two endpoints
