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
```
* I have tested the minimax configuration with this script: `ai-backend/cmd/verifyminimax/main.go`
  * based on the result `ai-backend/cmd/verifyminimax/result.json`, we got this
    ```shell
    "summary": {
        "best_base_url": "https://api.minimax.io/v1",
        "best_endpoint": "/chat/completions",
        "best_model": "MiniMax-M2.5",
        "failed": 13,
        "successful": 3,
        "total_tests": 16
      }
    ```
    * so, we will remove usage of `embedding_model`
    * change `base_url` into "https://api.minimax.io/v1"
    * change `model` into "MiniMax-M2.5"
    * add `chat_endpoint` with value of "/chat/completions" in the `config.yaml` file.
    * we will remove `embedding_model` key
* you may read the config file: `config.yaml` to get minimax AI credential
* please refactor codes in `ai-backend` based on the information above.
* refactor codes under `/api/search` as it uses MiniMax AI to response the request with utilizing the MiniMax AI. You can read the Minimax API usage here: `ai-backend/cmd/verifyminimax/main.go`
