# caseauto

## Overview
The "create automation" state machine of [CASE_AUTOMATION.md](../../../../docs/designs/CASE_AUTOMATION.md): for one test case without a spec it asks claude-v3 for a Playwright spec, runs it through playwright-executor, repairs it up to 3 times, waits for a human approval and then commits, pushes and opens a pull request on the product's automation repo. Every side effect sits behind an interface, so the machine is unit-tested with fakes.

## Interface
- `Machine.Run(ctx, Job) (*Outcome, error)`: `prepare -> generate -> write -> run -> repair (<= MaxRepairs, default 3) -> approval -> push -> pr -> done`. `Machine.Resume` skips to the approval step for a job whose approval was open when the backend restarted.
- Interfaces: `RepoOps` (git; the api adapter guards push and PR with `repoWriteAllowed`), `LLM` (satisfied by `*ai.Service`, the only LLM path: ai-manager pinned to claude-v3), `Executor` (`RunCase`), `Approvals`, `Events`, `State` (job payload), `Inbox` (satisfied by `*gaossync.Syncer`; nil = dashboard door only).
- `ParseReply` + `Validate`: the LLM reply must hold exactly one `FILE: <path>` line and one fenced block. The file must be at `tests/<web|api/rest>/<module>/<CODE>.spec.ts`, at most 20 KiB, UTF-8, mention the code, have every `test(...)` title start with the code, and contain no `.only/.skip/.fixme/.fail/.slow`, no `process.env` dump or dynamic read, no process or code execution, no dynamic import/require, no import other than `@playwright/test`, relative paths inside the repo and bare modules the example specs already import, and no literal URL whose host is not the product's staging host.
- `Redactor.Redact`: replaces literal secrets given at start, `Bearer` tokens, `gh*_` / `github_pat_` / `gaos_` tokens and URL userinfo by `***`. `Machine.ev` applies it (and a 1000 byte clip) to every event; the prompts and the PR body pass through it too.
- `VerdictFromResult`: reads the executor's case_run result (`report` through `caseresults.Parse`). Also used by the api's `case_run` job.
- `DecisionFromAnswer`: maps a GAOS Inbox answer (option label, else choice) to `approved|rejected|changes`.
- `EventStore` (`job_events`, seq allocated by `INSERT ... SELECT MAX(seq)+1`, retried on duplicate key) and `ApprovalStore` (`case_approvals`; `Answer` is `UPDATE ... WHERE status = 'open'`, so the first answer wins). `EventStore.Progress` reports a job's last step and whether a `push`/`pr` event exists; the api's orphan resume uses it to refuse re-running a job that may have a PR.

Every `ChatMessages` call names the job's conversation (`ai.ConvRef{job, <job id>, product, case code}`) and labels its messages (`Message.Kind`: `prompt`, `reply`, `refusal`, `run_result`, `feedback`) for the stored conversation (AI_CONVERSATIONS.md); what is sent to the model is unchanged.

## Invariants
- No push, commit-to-remote or PR happens before an `approved` answer; a reject or an expiry (72 h) ends the job `failed`. `changes` loops back to the LLM at most `MaxChangeRounds` (2) times, each round with a new approval `qa-<job>-prN`.
- The first approval's external id is `qa-<job id>-pr`. The GAOS door is best effort: an outage is logged and the dashboard door always works. The machine polls the approval row every 2 s and GAOS (`GetInboxItem`) every 30 s as the fallback for a missed webhook. An answer given on the job page stays local, so its GAOS item stays open until it expires; an answer given on the run history is relayed to GAOS by the api (HISTORY_ANSWER.md). Every answer records its source (`answer_source.go`: `gaos`, `qa-tool:history`, `qa-tool:job-page`, `qa-tool:learning`, `system` for any expiry), actor and GAOS's source object: `ApprovalStore.AnswerSourced`, the optional `SourcedAnswerer` the poll uses, and `ApprovalStore.Claim` (re-labels an answer GAOS delivered first once GAOS confirmed it was the QA tools' relay). `Answer` keeps its signature and derives the source from the door. `Approval.Who()` is the "<actor> (<source>)" text used in PR bodies and events.
- A refused LLM reply (parse or validation) never reaches the disk or the executor; it costs one repair round and the reason is sent back to the model.
- The machine never runs a shell and never executes LLM output: only the validated file is written.

## Side Effects
Events and approvals in MySQL; LLM calls; through the adapters: git worktree, executor runs, push and PR.

## Verification
`go test -mod=mod ./internal/services/caseauto/`: table-driven validation and redaction, and the machine against fakes (happy path, repair loop, repair budget, refused replies, reject, expiry, changes rounds, GAOS door and outage, push/PR failures, resume). `store_integration_test.go` needs `QA_TEST_MYSQL_DSN`.


## Generation guards (added after the first live runs)

- The prompt carries the repo's fixture declarations (`src/fixtures/api.fixtures.ts`, `web.fixtures.ts`); `Validate` rejects a test callback that destructures a fixture not declared there (`ValidateOptions.Fixtures`, `FixtureNames`), so the model cannot guess one from a naming pattern.
- A run that died while loading the automation repo's configuration (`Invalid configuration for TEST_ENV`, missing module) fails the job at once (`envFailure`) instead of spending repair rounds: an LLM rewrite cannot fix the host's environment.
- The prompt also carries the head and method names of every client class the fixture files import from `@api/*.client` (`clientDocs`), so the model only calls methods that exist.
- **Learned policies** (LEARNING_LOOP.md decision 8, `learned.go`): `RepoOps.ReadLearned` returns the worktree's `.claude/rules/learned-*.md`; `initialConvo` adds a `LEARNED POLICIES` block after the repo guide with each file's title and policy part (`LearnedPolicyPart`: no provenance comment, no Why / Evidence), ≤ 1,500 bytes each, 16,000 in total; a file over the budget is left out with a `warn` event. The `AGENTS.md` clip (`GuideClip`, 12,000 bytes) now writes a `warn` event when it cuts. The `run` warn `the spec did not pass: …` carries the first error line of the output tail (`FailureExcerpt`) so the learning loop's findings name the real failure.
- `AwaitApproval` (`approval.go`) is the approval wait (record, GAOS item, polls, 72 h expiry); `Machine.approve` and the learning loop's proposer both call it.
- Operational: the claude-v3 upstream (`ai-claude-cli` on rnd-v3) had `MemoryMax=512M`; large generate/repair prompts got its `claude` subprocess OOM-killed (symptom: ai-manager 502 `connection refused` on :8105). It is now 1024M.
