# AI Task Instruction

## Role
* you will act as **senior QA engineer**

## Main reference to read first

* read summary to understand the context: `_docs/markdown-results/summary/pdf-instruction-summary.md`
* read detail to understand the context: `_docs/markdown-results/summary/pdf-instruction.md`
* read the information given by System Analyst: `_docs/markdown-results/summary/system_analysis.md`
* read this file as this is your mail job desc: `_prompts/phase-2-enhancement-and-bug-fix/4-as-senior-qa-engineer.md`
* VERY IMPORTANT: read the how to run this playwright here: `/home/ardi/devel/projects/sismedika/qa-automation-tools/tutorial-playwright-docs.md.template`

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

* please start with `nvm use lts` command if you are in `fish shell`, or `nvm use --lts` if you are in `bash shell` to enable node by default
  * when you got `nvm: not found`, you will fix it and put your solution until you can use `nvm` into markdown file: `/home/ardi/devel/projects/sismedika/qa-automation-tools/_docs/markdown-results/summary/minimax-ai-enabling-nvm.md`. (or read this file first if existed already)
  * that tutorial will help you to start faster 
* you can read the codebase of the designated webpage: `/home/ardi/devel/projects/sismedika/his-frontend`. it is using ReactJS
* read `config.yaml` to get some credential information
  * you can read `mysql` and `backend.database` in case you need data from neon database
* you will put the resulted screenshots inside `/home/ardi/devel/projects/sismedika/qa-automation-tools/playwright-docs/output/screenshots/<designated-webpage-url>` folder
  * the value of `<designated-webpage-url>` is the value of `staging.url` inside that `config.yaml` file
  * in more details, please put screenshots each under `<designated-webpage-url>/<credentials.[role].role_code>` based on the active credentials used in `config.yaml` file
* you will take videos and put the results inside `/home/ardi/devel/projects/sismedika/qa-automation-tools/playwright-docs/output/videos/<designated-webpage-url>` folder
  * i have installed `ffmpeg`
  * the value of `<designated-webpage-url>` is the value of `staging.url` inside that `config.yaml` file
  * in more details, please put the resulted videos each under `<designated-webpage-url>/<credentials.[role].role_code>` based on the active credentials used in `config.yaml` file
  * as for the narration (text), you will utilize the `https://ai-backend-1087642958941.asia-southeast1.run.app` rest api
  * you may need to update the codes on the: `/home/ardi/devel/projects/sismedika/qa-automation-tools/playwright-docs/src/recorder`
  * when you got issue with system recording or browser issue, then, you managed to fix it, save the tutorial into file `playwright-recording-and-browser-usage.md`. if file this existed, read this file as another references / contexts
* set the record video to `true`
```shell
playwright_docs:
  output:
    video_dir: "output/videos"
    docs_dir: "output/docs"
    screenshots_dir: "output/screenshots"
    record_video: true ----> set as true
    video_format: "webm"
```
```
* you will focus only in section `staging` and `credentials` in that config files
* you may use and fix the codes here: `/home/ardi/devel/projects/sismedika/qa-automation-tools/playwright-docs/src/summarizer/summarizeFlow.ts`
  * in the end of the actions, you will updates the testing results with `callMinimax()` function or any other related actions.
* you will try using `playwright-docs` and run with `playwright`
  * first, you need to prepare the credential inside `playwright-docs/config` based on the `config.yaml` you have, by duplicating `playwright-docs/config/credentials.example.yaml` file as the real credential yaml file
* try to use all possible credentials login in `config.yaml` and executing `playwright-docs`
* you will need to read all available APIs in `README.md` as you will sometimes need to submit some data into Neon database
* as the output, I expect that you will create automation testing to documentate system based on that user login
  * you will try all possible menus, buttons, tabs, forms
  * you will fully utilize skills that's initially have been provided in Neon database, through backend API in `https://ai-backend-1087642958941.asia-southeast1.run.app`
* once done, you will make a report document, and update the issues in the Neon database through backend rest api
* additionally, please make documentation how to run this `playwright-docs` by creating `tutorial-playwright-docs.md` and fill them in
  * make sure you informed where do you start the node/nvm (e.g. in root project or in any sub-folders in this git repo)
* also, create summarized testing with named Markdown file `staging-hisv3-docs.md`. do summary of your explorations. make summary with tabular approach for each role, what pages explored. what result you got.
* you will summarize each page as each file Markdown, put them all inside `product-documentations/hisv3` folder
  * each file with summarize what is the pages accessed in this page
  * which roles have access into this page
