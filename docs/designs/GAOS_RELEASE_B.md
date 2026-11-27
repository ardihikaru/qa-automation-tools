# GAOS_RELEASE_B: repository kinds, the approved commit, and a pinned auto-merge of an approved spec

Status: proposed, rev 2 (two EGM reviews: design, implementation readiness; triage in §9). Builds on [GAOS_AUTOMATION.md](GAOS_AUTOMATION.md) and [CASE_AUTOMATION.md](CASE_AUTOMATION.md). The GAOS side is GAOS decision D-103 (GAOS v0.65.0, design `docs/designs/release-b-e2e-kinds-merge-unit-tests.md` in the GAOS repository); this document fixes the QA tool's side of that contract.

## 1. Goal

GAOS now knows, per product repository, whether it is **backend only** (`be`) or **frontend and backend** (`fullstack`), and may ask the QA tool to let an approved spec merge by itself. Three things are missing here:

1. A way for GAOS to learn what this build supports, so it never sends a field an older build would misread.
2. The `kind` of a request, applied to what runs and to what is created.
3. The commit pushed after a person approved a spec, and an **opt-in** GitHub auto-merge of that pull request, behind checks this tool verifies itself.

GAOS never merges and holds no merge token. This tool turns auto-merge on only for a spec a person approved **in the QA tool**, only for a commit that holds exactly the approved file, and only when every check in §4 holds.

## 2. Non-goals

- Merging anything itself: GitHub merges, when the repository's required checks pass.
- Generating GraphQL specs (still refused by `validate.go`).
- Unit tests (a later design).
- Any change to the session-authenticated routes or the dashboard.
- Turning off auto-merge on already-armed pull requests when a switch is turned off (§9, D1).

## 3. Contract

### 3.1 `GET /api/v1/gaos/capabilities`

Added to `IsGAOSInboundPath` (`gaos_inbound.go`), so it has the guard of every GAOS route: bearer `GAOS_INBOUND_TOKEN`, 503 without a token or a database, 401 for a wrong token, 405 for a method other than GET. Answer `200 {"capabilities":[...]}`:

| Capability | Listed when |
|---|---|
| `kind` | always |
| `auto-merge` | `QA_SPEC_AUTO_MERGE=on` (read from the environment at each call; default off) |

An older build answers **401** (the path falls through to session auth), which GAOS treats as "nothing listed": it then sends today's body.

### 3.2 `POST /api/v1/gaos/automation`: two new optional fields

```json
{"product":"p-6","caseCode":"TC-API-001","mode":"create","kind":"be","auto_merge":true}
```

| Field | Values | Effect |
|---|---|---|
| `kind` | absent, `""`, `be`, `fullstack` | Any other value: **422** `kind must be be or fullstack`, on every mode, checked right after the mode. `run` of `*`: `be` runs only `API` and `GQL` cases; `fullstack` and absent run every automated case; one active run-all per product **and kind**. `run` of one case: stored, no effect. `create`: `be` requires level `API`, `fullstack` requires `WEB` or `E2E`, else **422** `kind does not match the case level` (checked after the case is found); the generated file must then lie under `tests/api/rest/` (`be`) or `tests/web/` (`fullstack`) — `Validate` refuses another folder, which feeds the repair loop. Stored on the job payload as `kind`. |
| `auto_merge` | absent, `true`, `false` | `create` only; stored as `auto_merge_requested` (boolean, absent = `false`). A non-boolean is the existing **422** `invalid JSON body`. Reusing an active job (200) keeps that job's own request. |

GAOS never sends `fullstack` for an `API` case: its create cards take the kind of the case's level (GAOS D-103 §3.1).

### 3.3 `GET /api/v1/gaos/jobs/{id}`: two new fields

| Field | When present | Value |
|---|---|---|
| `pushed_sha` | a create job has pushed | `git rev-parse HEAD` of the worktree right after the push (40 hex); cleared before each commit, so a retried job never shows an earlier attempt's |
| `auto_merge` | a create job has opened its pull request | `enabled`, `off`, `pending` (gh was called; a cancel or restart froze the record), `refused:<token>` (§4), or `refused:restarted` (the job ended between opening the pull request and the step) |

GAOS compares `pushed_sha` with the merged pull request's **head** (`head.sha`), not with the squash commit.

## 4. The auto-merge step

`caseauto.Machine` gains an optional `AutoMerge AutoMerger` (nil in tests and when no repo pool is configured) and an optional `HeadReader` on its repo (`poolRepo.HeadSHA`):

```go
type HeadReader interface{ HeadSHA(ctx context.Context, ws Workspace) (string, error) }
type AutoMerger interface{ AutoMerge(ctx context.Context, in AutoMergeIn) string }
type AutoMergeIn struct {
    Job Job; Workspace Workspace; PRURL, PushedSHA string
    ApprovalSource string        // the approving answer's Source
    SpecPath, SpecContent string // the file and the exact bytes approved
}
```

In `publish`: clear `pushed_sha` and `auto_merge`; commit; push; read `HeadSHA` (a failure or a non-40-hex value records nothing and leads to `refused:no-sha`); open the pull request; call `AutoMerge`; record its answer as `auto_merge` and an event (`info` enabled, `warn` refused; none for `off`). The adapter (`api/spec_auto_merge.go`) reads `auto_merge_requested` from the job record and decides in this order; the first failure answers its token and calls nothing further:

| # | Check | Token |
|---|---|---|
| 0 | `auto_merge_requested` is true | `off` (not asked; not a refusal) |
| 1 | `QA_SPEC_AUTO_MERGE=on` | `refused:disabled` |
| 2 | the product is GAOS-integrated and GAOS answers `GET /products/{id}/spec-auto-merge` `{"allowed":true}` **now** (strict: `allowed` must be a boolean; any other status, body or transport error is unreachable; no retry) | `refused:gaos-denied`, `refused:gaos-unreachable` |
| 3 | `pushed_sha` was recorded | `refused:no-sha` |
| 4 | the approving answer came from a person in the QA tool (`Source` starts with `qa-tool:`); an answer relayed by GAOS, or by anything holding GAOS's token, never auto-merges | `refused:not-approved-in-qa-tool` |
| 5 | `git diff --name-only --no-renames origin/<base>...HEAD` is exactly the spec path, and `HEAD:<path>` is the git blob id of the approved bytes, hashed in Go | `refused:content-mismatch`, `refused:content-unverified` |
| 6 | the pull request URL matches whole (`PRRepo`) | `refused:bad-pr-url` |
| 7 | `gh api repos/{o}/{r} --jq` says neither `admin` nor `maintain`; a missing `permissions` object (an App token) refuses | `refused:admin-identity`, `refused:github-unreadable` |
| 8 | `gh api --paginate repos/{o}/{r}/rules/branches/<qa-auto branch>` lists `update` (without `update_allows_fetch_and_merge`) **and** `deletion` | `refused:no-branch-ruleset` |
| 9 | `gh api --paginate .../rules/branches/<the pull request's base, ws.Base>` lists at least one required status check, and every one names an `integration_id` | `refused:no-fixed-check` |
| 10 | `repoWriteAllowed(…, RepoOpAutoMerge)`: a new op, allowed on an automation repository only | `refused:write-not-allowed` |
| 11 | `auto_merge` is recorded `pending`; then `gh pr merge <url> --auto --squash --match-head-commit <pushed_sha>` exits 0 | `refused:merge-failed` |

Then `enabled`. A refusal never fails the job: the pull request stays open for a person, as today. Every `gh api` call reduces its answer with `--jq` to a short value (the repo pool's `run` caps output at 2000 bytes); paginated answers are joined line by line, and anything unparsable refuses.

### 4.1 Cancel

`POST /api/v1/gaos/jobs/{id}/cancel` on a job whose `auto_merge` is `enabled` or `pending` also runs `gh pr merge <url> --disable-auto` (in the pool root; logged on failure; the cancel's answer is unchanged).

### 4.2 What the operator sets up on the automation repository

- A ruleset on `refs/heads/qa-auto/*` with **Restrict updates** (fetch-and-merge off) and **Restrict deletions**, the QA tool's identity its only bypass actor.
- A ruleset on the base branch with **Require status checks to pass**, each check bound to its source, running lint and typecheck, a secret scan, and a dry run with no repository secrets; workflow token permissions read-only.
- **Allow auto-merge** on; the QA tool's identity a user token without admin or maintain.

### 4.3 What remains, recorded in GAOS D-103

- This tool cannot read bypass actors (that needs admin, which check 7 forbids): another bypass actor of the `qa-auto/*` ruleset could push after the approval. `--match-head-commit` is checked when auto-merge is turned on; GAOS runs the case after the merge only when the merged head equals `pushed_sha`, and notes any other.
- The required check's content (that it scans for secrets and runs without them) is the operator's.
- Classic branch protection is invisible to `rules/branches`: such a repository always answers `refused:no-fixed-check`.

## 5. Change list (Target Files)

| File | Change |
|---|---|
| `ai-backend/internal/api/gaos_automation.go` | `kind`, `auto_merge`; validation; `caseLevel`; run-all kind filter and dedup; `pushed_sha`, `auto_merge`, `refused:restarted` on the status; cancel disarms |
| `ai-backend/internal/api/gaos_capabilities.go` (new) | the route and `specAutoMergeOn` |
| `ai-backend/internal/api/gaos_inbound.go` | the path in `IsGAOSInboundPath` and the switch |
| `ai-backend/internal/api/case_automation.go` | `caseEnqueueIn.Kind`, `.AutoMerge` into the payload; `Job.Kind` from it; `poolRepo.HeadSHA`; `newMachine` sets the adapter |
| `ai-backend/internal/api/product_repos.go` | `RepoOpAutoMerge` |
| `ai-backend/internal/api/spec_auto_merge.go` (new) | §4's adapter |
| `ai-backend/internal/services/caseauto/machine.go`, `validate.go` | the hooks; `Job.Kind`; `ValidateOptions.Kind` |
| `ai-backend/internal/services/repopool/automerge.go` (new) | `HeadSHA`, `CommittedExactly`, `GitBlobSHA`, `PRRepo`, `RepoAdmin`, `BranchRuleTypes`, `FixedRequiredChecks`, `EnableAutoMerge`, `DisableAutoMerge` |
| `ai-backend/internal/services/gaossync/automerge.go` (new) | `SpecAutoMergeAllowed` |
| tests next to each | §6 |
| `docs/designs/SITEMAP.md`, `docs/designs/GAOS_AUTOMATION.md`, `docs/GAOS_COLLABORATION.md`, `deploy/qa-tool/.env.prod.example` | the contract and the switch |

## 6. Tests

- `api/gaos_release_b_test.go` (DB): capabilities behind the guard (401, 200, 405); `kind` 422 on create and run; level mismatch 422; a fitting kind passes; the `be` level filter; a `be` run-all with no backend case 409; a run-all of another kind is not reused; status `pushed_sha`, `auto_merge`, `refused:restarted`; `RepoOpAutoMerge` only on automation repositories.
- `api/spec_auto_merge_test.go`: each of checks 0–11 refuses with its token and calls `gh pr merge` only when all pass, with `--match-head-commit <sha>` and the base read from `ws.Base`; a GAOS-relayed approval refuses; capabilities follow the switch; the kind rules.
- `caseauto/release_b_test.go`: `pushed_sha` recorded from `HeadSHA`; the adapter gets the approved path and bytes; its outcome is recorded with an event; without hooks nothing is recorded; a malformed HEAD is never recorded.
- `repopool/automerge_test.go`: `GitBlobSHA` equals `git hash-object --no-filters`; `PRRepo` refuses anything but a whole pull request URL; paginated answers join and a truncated one refuses.

## 7. Phases

1. Capabilities and `kind` (§3.1, §3.2): GAOS sends `kind`; nothing merges.
2. `HeadSHA` and `pushed_sha` (§3.3): GAOS records the approved commit.
3. The adapter behind `QA_SPEC_AUTO_MERGE` (§4): off until the operator turns it on.

All three land in one pull request; each builds and tests on its own.

## 8. Rollout

1. Merge and deploy with `QA_SPEC_AUTO_MERGE` unset: only `kind` is listed.
2. Set up §4.2 on the automation repository, then `QA_SPEC_AUTO_MERGE=on` and the product's GAOS switch. A deploy of either is a separate approval.

## 9. Review triage (rev 1 → rev 2)

| Finding | Severity | Disposition |
|---|---|---|
| Cancel never disarms; a frozen record hides an armed merge (design B1) | Blocker | PROPOSE FIX: cancel runs `--disable-auto`; `pending` recorded before gh (§4.1, check 11) |
| Switching off does not disarm armed pull requests (design B1) | Blocker | DECLINE (D1): an armed one merges only a commit a person approved, behind the ruleset; turning a switch off stops new ones. The operator runs `gh pr merge --disable-auto` on open `qa-auto/*` pull requests if needed; written in §2 |
| GAOS's token can approve, ask and allow (readiness S1) | Blocker | PROPOSE FIX: check 4, an approval in the QA tool only |
| The pin is a commit, not the approved content (readiness S2) | Blocker | PROPOSE FIX: check 5 |
| Interfaces and data flow undefined (readiness R1) | Blocker | PROPOSE FIX: §4 signatures and order |
| "Pinned" overstated (design M1) | Major | PROPOSE FIX: §4.3 wording; GAOS compares the merged head |
| Bypass inference wrong; fetch-and-merge; deletion (design M2, readiness S6) | Major | PROPOSE FIX: check 8 |
| Output truncation; pagination (design M3, readiness R5) | Major | PROPOSE FIX: `--jq`, `--paginate`, joined pages |
| Squash vs head (design M4) | Major | PROPOSE FIX: §3.3 last line |
| Restart after the pull request (design M5) | Major | PROPOSE FIX: `refused:restarted` |
| Kind not tied to the folder; fullstack and API (design M6) | Major | PROPOSE FIX: `ValidateOptions.Kind`; GAOS sends `be` for API cases (§3.2) |
| Other bypass identities (design M7, readiness S4) | Major | PROPOSE FIX: admin or maintain; missing permissions refuse; residual §4.3 |
| Wrong base branch (readiness S3) | Major | PROPOSE FIX: check 9 reads `ws.Base` |
| Kind filter code; dedup across kinds (readiness R2) | Major | PROPOSE FIX: §3.2, §5 |
| Kind validation contradiction (readiness R3) | Major | PROPOSE FIX: 422 on every mode |
| Unreachable route; older build answers 401 (readiness R4) | Major | PROPOSE FIX: §3.1 |
| GAOS client strictness (readiness R6) | Major | PROPOSE FIX: check 2 |
| No phases (readiness R10) | Major | PROPOSE FIX: §7 |
| Own write op (design m1) | Minor | PROPOSE FIX: `RepoOpAutoMerge` |
| Stale `pushed_sha` (readiness R7) | Minor | PROPOSE FIX: cleared before commit |
| Classic protection; Actions-bound checks; gh pinning; waiting forever; App-bound own check (design m2/m3/m6/m7, readiness S7) | Minor | ESCALATE: §4.2/§4.3 for the operator; gh pinning is a Dockerfile chore |
| Capability cache on rollback (design m5) | Minor | DECLINE: a rollback within ten minutes sends `kind` to a build that ignores it; that runs every case, today's behaviour |
| URL anchoring (readiness S5) | Major | CONTEST: `PRRepo` requires `prURLRe.FindString(u) == u`, a whole match (tested) |
