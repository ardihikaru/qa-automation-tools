# epicauto

## Overview
The pure decisions of [EPIC_AUTOMATION.md](../../../../docs/designs/EPIC_AUTOMATION.md): what state a case of an "Automate epic" batch is in, and what the driver does next. No database, clock or network: `internal/api/epic_batches.go` loads the facts (jobs that carry the batch id, eligibility) and applies the decision.

## Interface
- `Classify(Case) State`: `pending | active | awaiting_approval | pr | declined | failed | needs_person | skipped`, from the newest `case_create` job of the batch for the case (none: `pending` if the case is still eligible, else `skipped`).
- `HumanOutcome(err) bool`: an error that is the operator's decision (`rejected…`, `approval expired…`, `changes were requested too many times…`, an orphan that had passed the approval step).
- `Plan([]Case) Decision`: in a fixed order: a case that needs a person stops the batch; three consecutive technical failures (newest first; `declined` neither counts nor resets, a `pr` resets) stop it; nothing pending or active completes it; otherwise enqueue pending cases in snapshot order until `MaxOpen` (2) are in flight.

## Invariants
- A batch never writes item rows: case state is derived from `job_runs`, so there is nothing to drift.
- `Plan` never enqueues for a batch it decided to stop or complete.

## Side Effects
None.

## Verification
`go test -mod=mod -race ./internal/services/epicauto/`; the SQL, the one-running-batch key and the cancel race are tested in `internal/api/epic_batches_test.go` on a scratch MySQL (`QA_TEST_MYSQL_DSN`).
