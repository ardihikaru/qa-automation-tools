# learnloop

## Overview
The learning loop of [LEARNING_LOOP.md](../../../../docs/designs/LEARNING_LOOP.md): findings observed in real automation work become AI-drafted policies that a human approves and the tool publishes as **one new `.claude/rules/learned-<slug>.md` file per policy** on the product's automation repo, tracked until the PR is merged. A merged file is read back into every later `case_create` prompt (`caseauto.learnedBlock`), which closes the loop.

The HTTP routes, the job runner, the publisher (repo pool) and the driver tick are in `internal/api/learning_loop.go`; this package holds the decisions, the SQL of the two tables and the proposal machine.

## Interface
- **Text** (`normalize.go`): `Normalize`, `Fingerprint` (sha256 of the normalised text; exact fingerprints are the only dedup), `Signature` (the failure text of a job message, `ok=false` for generic verdicts), `IsHumanOutcome` (`epicauto.HumanOutcome` plus `stopped while waiting for approval`).
- **Harvest** (`harvest.go`): `Harvest(ctx, db, now, redactor)` scans the last 30 days of `case_approvals` (`changes` / `rejected` with text), `job_events` (`the reply was refused:`, `the spec did not pass:`, `the spec passed`) and failed `job_runs`; `ApprovalCandidate`, `RepairCandidates`, `RepeatedCandidates` (a signature in ≥ 3 distinct jobs) are pure; `Ingest` stores one candidate (skip a known `source_key`; else one transaction: join the open finding with the same fingerprint or create one, insert the evidence).
- **Findings** (`store.go`): `ListFindings` (filters product / status / kind / q, counts per status), `GetFinding` (+ ≤ 50 evidence rows), `SetFindingStatus` (dismiss, reopen), `Reserve` / `Release` (a proposal takes `new` findings of one product, all or nothing), `Briefs`, `Reconcile` (decision 5: covered → `covered`, merged → `learned`, rejected / failed / closed → `new`), `LearnFindings`.
- **States** (`state.go`): finding statuses, kinds, `ProposalState(JobFacts)` (decision 4), `JobType = "policy_propose"`.
- **Drafts** (`draft.go`): `Prompt` (findings, the synced `AGENTS.md` golden rules and headings, the synced rule files, a reviewer's change text, all as fenced data), `ParseDraft` / `Draft.Validate` (slug, title, 40–1,200-byte policy, the injection screen: no H1, no `---` / `@` / `<` line starts, no HTML comments), `RenderFile`, `NormalizeMarkdown`, `FreeSlug`, `LearnedPath`, `IsLearnedPath`, `PRTitle`, `PRBody` (the automation repo's Bahasa template headings).
- **Proposer** (`proposer.go`): `Proposer.Run(ctx, ProposalJob)`: prepare → generate (one corrective turn) → `covered` ends it → approval (`caseauto.AwaitApproval`, external id `qa-<job id>-pr[N]`, GAOS gets the whole policy) → *Needs changes* redrafts (≤ 2 rounds) → `Publisher.Publish` → PR. `Resume` continues an open or approved approval with the stored draft (never regenerates).

## Invariants
- The committed file names finding kinds, case codes and job ids only; excerpts stay in the database (golden rule 5 of the automation repo: no PII).
- Nothing is pushed before a human approved the exact file text shown on `/jobs/:id` (and in GAOS); there is no in-place edit.
- Job errors that mean "a human said no" start with `rejected`, `approval expired` or `changes were requested too many times` (the same texts `case_create` uses).
- Harvest and reconcile are idempotent: a re-run changes nothing.

## Side Effects
MySQL reads of `case_approvals`, `job_events`, `job_runs`, `kb_assistants`; writes of `learning_findings` and `learning_evidence`. The proposer's side effects go through its interfaces (LLM, approvals, events, job payload, GAOS Inbox, publisher).

## Verification
`go test -mod=mod ./internal/services/learnloop/` (pure functions and the proposer with fakes). The SQL, the job, the publisher (temporary bare origin, fake `gh` on `PATH`), the GAOS webhook and the restart path: `QA_TEST_MYSQL_DSN=… go test -mod=mod -run 'Learning|Harvest|Proposal|ManualFindings|ResumeProposals' ./internal/api/`.
