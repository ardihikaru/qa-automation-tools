# AI Task Instruction

## Role
* you will act as **senior system analyst** with 20 years of experiences

## Main reference to read first

* read summary to understand the context: `_docs/markdown-results/summary/pdf-instruction-summary.md`
* read detail to understand the context: `_docs/markdown-results/summary/pdf-instruction.md`
* read the information given by System Analyst: `_docs/markdown-results/summary/system_analysis.md`
* read this database's tables: `_docs/markdown-results/by-roles/database-admin.md`

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
- to make a very comprehensive documentations

## Execution details

* if I can summarize on my point of view based on the references above, the main objectives of this project is:
  1. Able to read all available roles in the system (current case: HIS v3; e.g. `https://stag-app-hisv3.sismedika.online/`)
  2. Automatically generate full documentation in video format, including narration. This covers all menus and system flows within the targeted application. 
  3. the prepared `playwright-docs` is meant to create QA automation to generate: (1) self-exploration of system menus dan capability, (2) screenshots, (4) recorded videos with narratives supported by MiniMax AI. we will need to prepare a proper prompt(s) in the end
  4. the prepared `playwright-qa` is meant to create self-documented periodical QA automation to perform testing and submit the results into the backend. we will need to prepare a proper prompt(s) in the end
  5. the prepared `ai-backend` is meant to support the communication with every data logging
  6. the prepared `dashboard` is meant to visualize everything related with the QA automation activities
  7. Automate testing and generate detailed logs of the testing results. The system will automatically query AI to analyze potential bugs—whether they originate from the frontend (FE), backend (BE), or both. It will also provide an estimated commit hash where the issue was introduced.
  8. Each testing iteration will be logged. For problematic parts (bugs), there will be an action checklist. Users accessing the dashboard can mark whether an issue has been fixed or not.
  9. The AI automation system has a set of basic skills, defining mandatory checks. As it is used more frequently, these skills will evolve. Each finding will be analyzed and added to the skill database, allowing continuous improvement over time.
* you will have access to the real credentials inside `config.yaml`. you can login into database with `mysql` command
  * when I checked on the database, it has four schemas: (1) `ai`, (2) `logging`, (3) `public`, and (4) `qa`.
  * please explain what each schema is used for
* you will assess all possible implementation. what's have been implemented and what's not implemented yet.
* create an objective file called `OBJECTIVES.md` to summarize all tables we have, and what's the usage of each table for.
* with that objectives, you will assess the backend implementation in `ai-backend` folder. read the entire codebase
  * update the objectives to verify each API is related with which table(s)
  * update the objectives which table is not implemented yet
  * update the objectives to add TODOs what features we need to implement and will be related to which tables
* with that objectives, you will assess the frontend implementation in `dashboard` folder. read the entire codebase
  * update the objectives to verify each page is related with which table(s) and which API(s)
  * update the objectives which page is not implemented yet, such as, (1) still reading tables with mock data, or (2) has fields and filters but still has no implementation yet 
  * update the objectives to add TODOs what features we need to implement and will be related to which table(s) and which API(s); we may talks about future API(s) that's not ready yet as well here
* with that objectives, you will assess the frontend implementation in `playwright-docs` folder. read the entire codebase
  * update the objectives to verify what part that's not implemented yet, so we will have TODOs listed for next development iteration
* with that objectives, you will assess the frontend implementation in `playwright-qa` folder. read the entire codebase
  * update the objectives to verify what part that's not implemented yet, so we will have TODOs listed for next development iteration
* you will be creating technical architecture documentations accordingly once the objectives clearer. save them to `proposed-system-architecture.md`
