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
* I found that there are two endpoint that has problems - re-assess if current codes have fixed issues in following APIs
```shell:
  - /api/search
  - /api/cost/overview
```
* you can use `pdf_extract.py` to extract and read PDF file: `MiniMax-API-Endpoint-part-2.pdf` to add knowledge regarding usage of MiniMax AI's rest api
* you may read the config file: `config.yaml` to get minimax AI credential
* that endpoints maybe related with minimax AI integration problems, so we will go deeper in this integration issue first:
  * in `Minimax AI` section, I updated `api_key` value
  * for `model` and `embedding_model`. you can adjust it according to the PDF file referenced above. maybe we don't need `embedding_model` ?
  * as for `base_url`, you can adjust it according to the PDF file referenced above. We obviously need to update it
* please make a small script to verify usage of minimal AI with that. put in here: `ai-backend/cmd` -> `ai-backend/cmd/verifyminimax`
* Common current model names include:
  * MiniMax-M2.7 
  * MiniMax-M2.7-highspeed 
  * MiniMax-M2.5
* please try all possible models and create `result.json` to store the testing result
* if any error occurs. do retry with other possible objectives, but max are 4x retry times
