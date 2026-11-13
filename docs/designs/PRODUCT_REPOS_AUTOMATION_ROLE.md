# Automation repo role (separate from the main repo)

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Draft rev 2 (after three Goldfish reviews: design, codebase accuracy, implementation readiness). Human decisions recorded below. Awaiting re-review and owner approval. Not implemented.
*   **Date:** 2026-11-13
*   **Depends on:** [PRODUCT_REPOS.md](PRODUCT_REPOS.md) (implemented and deployed: PR #13, strict-JSON follow-up PR #14, merge commit `8da8f6f`). Everything that spec says stays in force unless this document says otherwise. It amends that spec's Overview ("Main repo" bullet), Invariants 1 and 7, "Verify per repo", "Write guard" and "Legacy compatibility and rollback".
*   **Code references** cite function names; line numbers are approximate (read on 2026-11-13, `main` at `16f409f`).

## Overview
Today **main** means two things: the AI agent's base workspace, and the repo that holds the Playwright e2e scripts and is the only PR source. The code expresses that as `writeAllowed(isMain, ...)`, the Playwright check only for `repo.IsMain` (`verifyAndStore`, `verifyAllProducts` in `ai-backend/internal/api/products.go`), and the legacy mirror from the main row (`mirrorMainToLegacy`, `ai-backend/internal/api/product_repos.go`). For HISv3 they differ: the AI base workspace is `his-v3-ai`, the Playwright automation lives in `automation-test-hisv3`.

Roles per product after this change:

| Role | Count | Meaning |
|---|---|---|
| `main` | exactly one | The AI agent's base workspace. A label/pointer only (see Decision 10); it cannot be deleted. |
| `automation` | exactly one; may be the same repo as main | Holds the Playwright scripts; the only PR source; the repo the Playwright check and the staging probe apply to; the repo the product-level and legacy columns mirror. |
| `central` | zero or one | Unchanged. |

The first release is a **no-op for existing products**: the backfill sets automation = current main. Moving HISv3's roles is a separate, explicit post-deploy step (Decision 9).

## Problem
*   A product whose scripts live outside its AI workspace cannot be modeled: either main is the workspace (and the Playwright check fails, looking in the wrong repo) or main is the scripts repo (and the workspace is just a sibling).
*   The write guard and push policy equate "main" with "may push", which is wrong for a workspace that is not the PR source.

## Goals
*   An `automation` role, exactly one per product (DB: at most one; handlers: never zero).
*   `repoWriteAllowed` allows a repo iff it is automation or the product flag `allow_non_automation_work` is on.
*   The Playwright check, staging probe, product-level `playwright_accessible` and the legacy mirror follow the automation repo.
*   Migration appended after the existing entries, idempotent, applied before the binary; backfill; rollback without data repair.

## Non-Goals
*   No PR-creation UI or API (no route performs a write-class op yet; the gate is tested, not called).
*   No change to central, branches, worktrees, locking, sync or the pool; no per-repo staging URL; the executor still uses one `HISV3_STAGING_URL`.
*   No DB-level protection against deleting the automation repo (handler-enforced, like main).
*   No auto-verify after a role change (Decision 8).

## Decisions
Made by the human (first round):
1.  **Migration guard = option A, a `-- skip-if:` runner guard.** Rejected alternatives, recorded for the record: **B** a `product_repo_roles` table (departs from the flag-column pattern, adds a join to every repo read, and makes the previous binary's repo `DELETE` fail with 1451 during a rollback window); **C** tolerating errors 1060/1061 in the runner (already rejected in `PRODUCT_REPOS.md` Decision 1; a half-applied multi-clause ALTER would be misread as applied).
2.  **A main repo that is not automation is read-only on the remote** (push URL disabled, DB guard denies) while `allow_non_automation_work` is off. Owner decision; nothing changes for backfilled products.
3.  **A create with no `is_automation` repo makes the main repo the automation repo.** Not a 400.
4.  The stale status and repo-state lines of `PRODUCT_REPOS.md` are corrected in this PR (done), and the merge-conflict block in `ai-backend/internal/db/README.md` is resolved in this PR.

Made by the coordinator (second round, not re-asked):
5.  **Degraded states are one precedence table** (below). Check order everywhere: `hasMain` first (existing `errNoMain`), then `hasAutomation` (new `errNoAutomation`).
6.  **Legacy-column mirror follows the automation repo** (`mirrorMainToLegacy` is renamed `mirrorAutomationToLegacy`). The legacy columns (`repo_url`, `root_dir`, `branch`, `playwright_accessible`, `last_verified_at`, `verify_report`) are one record written from one row. The pre-PRODUCT_REPOS binary treats that single repo as "the repo with the tests" (clone, branch checkout, Playwright verify: `products.go` at `453ccec`, the clone branch step `if opErr == nil && p.Branch != ""`), so mirroring the automation row keeps that rollback view meaningful and keeps `products.playwright_accessible` equal to the automation repo's. The immediately previous release reads `product_repos`, not these columns. Splitting columns across two rows is rejected. Legacy `/verify|/clone|/checkout` routes also target the automation repo.
7.  **Promotion semantics identical to main**: `is_automation:true` moves the flag atomically (clear old, set new, one tx). `is_automation:false` on the current automation repo is 409; deleting it is 409. A promotion clears the verify state of **both** the demoted and the promoted repo (their report shapes change).
8.  **Promotion does not auto-verify.** The promoted repo shows "Unverified" until someone runs Verify (or the scheduled verifier does); the dashboard prompts "Verify now".
9.  **HISv3 transition is an explicit post-deploy step** (section below).
10. **Main has no consumer beyond being a label and an AI-base-workspace pointer.** Evidence (grep, 2026-11-13): `grep -rnE 'FROM products|JOIN products|UPDATE products|INTO products|product_repos|PlaywrightAccessible|\.Branch\b' ai-backend/cmd ai-backend/internal --include=*.go --exclude=*_test.go`, minus `internal/api/product_repos.go`, `internal/api/products.go` and `internal/db/migrations.go`, finds only the `models.go` struct fields and an unrelated `rep.Branch` in `repopool.go`. `grep -rnE 'playwright_accessible|product_repos|FROM products|/api/products' playwright-executor/src playwright-executor/tests lib deploy playwright-qa playwright-docs this-qa-automation-system-playwright --exclude-dir=node_modules` finds one line: `this-qa-automation-system-playwright/tests/api/impersonation.spec.ts:69` (a POST that expects 403; unrelated). So nothing reads the legacy columns or `is_main` outside the two API files and the dashboard.

## Open Questions
None. (An assumption, not a question: Decision 9's steps do not depend on what HISv3's production roles are today; they are stated as "ensure ... then promote".)

## Design

### Invariants (replacing PRODUCT_REPOS.md 1 and 7)
1.  Exactly one main and exactly one automation repo per product (DB: at most one each via `UNIQUE(product_id, flag)`; handlers: never zero except in the degraded states below). They may be the same row; a repo may hold any combination of main, automation, central.
2.  Product-level `playwright_accessible`, `last_verified_at`, `verify_report`, `repo_url`, `root_dir`, `branch` mean "the **automation** repo's", mirrored.
3.  A repo's remote-write right is `is_automation OR allow_non_automation_work`. `is_main` alone grants nothing.

### Data model
Two entries **appended after `impersonation_audit`** (the end of the `migrations` slice; `migrations.go`), in this order, named by purpose in a leading comment (`product_repos_add_automation_role`, `product_repos_backfill_automation_role`). Applied entries are not edited. `product_repos` is created by the existing entry without the new columns, so fresh and existing databases take the same path.

```sql
-- product_repos_add_automation_role
-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'product_repos' AND column_name = 'automation_flag'
ALTER TABLE product_repos ALGORITHM=COPY, LOCK=SHARED,
  ADD COLUMN is_automation TINYINT(1) NOT NULL DEFAULT 0 AFTER is_central,
  ADD COLUMN automation_flag TINYINT GENERATED ALWAYS AS (IF(is_automation = 1, 1, NULL)) STORED AFTER central_flag,
  ADD UNIQUE KEY uq_product_repos_automation (product_id, automation_flag);

-- product_repos_backfill_automation_role (set-based, no Go; only products that have no automation repo yet)
UPDATE product_repos m
LEFT JOIN product_repos a ON a.product_id = m.product_id AND a.is_automation = 1
SET m.is_automation = 1
WHERE m.is_main = 1 AND a.id IS NULL;
```
*   The generated column depends on `is_automation` only, not on `product_id` (the FK column), so the 3104 restriction in `PRODUCT_REPOS.md` ("Why the generated columns derive from the flag") does not apply. Same pattern as `main_flag`/`central_flag` (`migrations.go`, `product_repos` entry).
*   `ALGORITHM=COPY, LOCK=SHARED` is written explicitly: a STORED generated column cannot be added in place, and naming it makes MySQL fail loudly instead of silently choosing a different lock. Reads continue, writes wait for the copy of a tiny table. One ALTER means all-or-nothing, so the presence of `automation_flag` proves the whole change.
*   **Must be proven in the scratch run, not assumed:** a generated column that references a column added in the same ALTER; `LOCK=SHARED` accepted with `ALGORITHM=COPY`; the leading `-- skip-if:` comment reaching MySQL without error.
*   The column is `NOT NULL DEFAULT 0`, so the previous binary's `INSERT INTO product_repos (...)` (it names its columns) keeps working and its rows are `is_automation=0`. No NULL handling is needed.
*   **Race:** the skip-if check and the ALTER are not atomic; two concurrent runners would make the second fail with 1060 and abort with a clear error, nothing corrupt. The runner is a manual command (`go run -mod=mod ./cmd/databasemigration`), accepted.
*   **Backfill re-run after the new binary is live** (to catch products created by the previous binary in the window): run it in a **quiet window** (no dashboard edits; announce), as the single statement above. A handler promotion racing it cannot create two automation repos (the unique key); the loser gets 1062, which the handler maps by key name (`uq_product_repos_automation`) to 409 "the product already has an automation repo" and the user retries. InnoDB blocks the UPDATE on rows a promotion tx has locked, so it does not read half a swap.

### Runner change: the `-- skip-if:` guard
`RunMigrations` runs every entry on every run with no guard and aborts at the first error (`migrations.go`, `RunMigrations`). Exact grammar and behaviour:
*   `func parseSkipIf(sql string) (query string, ok bool)`: strip leading whitespace, newlines and a UTF-8 BOM; take the first line; `ok` is true iff it starts with `-- skip-if:` (case-sensitive, one space after `--`); `query` is the rest of that line, trimmed. A directive with an empty query returns `("", true)` and the runner aborts with "empty skip-if guard". Pure function, unit-tested.
*   For `ok` entries the runner runs `query` on the `*sql.DB` (outside any transaction), requires exactly one row and one column scanned into `sql.NullInt64`; zero rows, more than one row, NULL or a query error abort the run with `migration <i> skip-if guard failed: ...`. A value `> 0` skips the entry and logs `Migration i/N skipped (guard > 0)`; `0` runs it.
*   The **whole entry text, including the comment line, is still sent to MySQL**, so the statement is valid when pasted by hand. For an `ok` entry the runner pins one `*sql.Conn` and runs `SET SESSION lock_wait_timeout = 30` before the ALTER (the pool connections used by other entries are not touched).
*   `summarize()` skips the directive line, so an ALTER failure reports `ALTER TABLE product_repos ...`, not the comment.
*   No existing entry starts with `-- skip-if:`, so existing entries behave unchanged.
*   `ai-backend/internal/db/README.md` invariants (the bullet "Every statement in `migrations.go:migrations` must stay idempotent ..." and the `RunMigrations` bullet) are reworded: idempotent either by `IF NOT EXISTS`/guarded DML or by a leading `-- skip-if:` guard with the grammar above; `summarize` skips the directive.

**Deploy order.** Run the migration command **before** starting the new binary (the new binary selects `is_automation`; without it every repo read is a 500). The previous binary ignores the column. Re-run the migration after the new binary is live.

**Mandatory pre-implementation step (blocks the implementation PR).** On a scratch `mysql:8.0.33` container (rnd-v2's version, per `PRODUCT_REPOS.md` "DDL verification result"), never `qa_automation`: run the **full real `migrations` slice twice** via `RunMigrations` (not just the new entries), seeded with legacy-style rows, a rollback-window row (`is_main=1, is_automation=0`) and a row with automation != main. Record here the version, results, error numbers and the three "must be proven" items above, plus: a second `is_automation=1` row per product gives 1062 on `uq_product_repos_automation`; the second run changes nothing and leaves automation != main alone. **DDL verification result (run 2026-11-13, scratch `mysql:8.0.33` docker container on 127.0.0.1, never `qa_automation`; the real `RunMigrations` runner through `internal/testdb`, plus `cmd/databasemigration` against a scratch schema `step0`):** PASSED, no MySQL error on any step.
*   Full `migrations` slice (33 entries) applied on an empty schema, then again (the testdb harness runs it once, the tests run it again, `TestMigrationsIdempotentAndBackfill` three times in total). The second run logs `Migration 32/33 skipped (guard > 0)` and nothing else changes.
*   Proven: a generated column referencing `is_automation` added in the same ALTER (`automation_flag ... STORED AFTER central_flag`) works; `ALGORITHM=COPY, LOCK=SHARED` is accepted; the leading `-- skip-if:` comment line reaches MySQL without error (the whole entry text is executed).
*   Seeded legacy-style rows, a rollback-window row (`is_main=1, is_automation=0`), a row with automation != main, and a main plus sibling product: the backfill heals the first and the last (main becomes automation), leaves automation != main alone, and the second run changes nothing (`TestMigrationsAutomationHealAndRerun`).
*   A second `is_automation=1` row for one product fails with error 1062 on `product_repos.uq_product_repos_automation` (message key `product_repos.uq_product_repos_automation`, mapped by `conflictMessage`); another product may have its own automation repo.
*   Post-flight queries (roles; legacy drift) and the roll-forward `UPDATE` ran on the scratch schema: the roles query returned zero rows after the heal; the drift query listed the product whose automation repo (no default branch, branches `zeta`, `alpha`) differed from the legacy columns before the `UPDATE` and zero rows after, with `products.branch = 'alpha'` (the `MIN` fallback) and `''` for a repo with no branches.
*   Runner guard paths (`TestRunnerSkipIfGuard`): `> 0` skips, `0` runs; NULL, no row, two rows, two columns, a query error and an empty query each abort with `skip-if guard failed` and the entry does not execute; a failing guarded statement reports the ALTER line, not the directive.
*   No DDL change was needed: the spec DDL is used verbatim.

### Degraded states and the precedence table
A product normally always has main and automation. Degraded states arise from a rollback window (the previous binary creates or changes repos without knowing `is_automation`) or the existing backfill-conflict case. **S0** normal; **S1** no main repo (repos may or may not exist; "both missing" with repos is S1 and also has no automation); **S2** main exists, no automation. Checks run in this order: `hasMain` then `hasAutomation`. New `hasAutomation(repos)` and `errNoAutomation = conflict("product has no automation repo")` are twins of `hasMain`/`errNoMain` (`products.go`, `product_repos.go`).

| Route | S0 | S1 (no main) | S2 (main, no automation) |
|---|---|---|---|
| `GET` product(s), `PUT /{id}` (name, staging, flag), `DELETE /{id}` | 200/204 | same | same (no repo check today) |
| `POST /{id}/repos` | normal rules | 409 `errNoMain` unless body `is_main:true`; then the automation rule below | body must have `is_automation:true` (or the product has no repos at all); else 409 `errNoAutomation` |
| `PUT /{id}/repos/{rid}` | normal rules | 409 `errNoMain` (repair is via POST only, as today) | allowed only if body has `is_automation:true` (it may carry other fields, applied atomically); else 409 `errNoAutomation` |
| `DELETE /repos/{rid}`, `PUT /repos/{rid}/branches`, `POST /repos/{rid}/verify|clone|checkout` (`loadRepoForRoute`) | normal | 409 `errNoMain` | 409 `errNoAutomation` |
| Legacy `POST /{id}/verify|clone|checkout` (`legacyRepoAction`) | on the automation repo | 409 `errNoMain` | 409 `errNoAutomation` |
| `verifyAllProducts` | runs | skipped | skipped |

**Automation rule for `POST /repos`** (applies after the main check): if the body has `is_automation:true`, it promotes (old flag cleared in the same tx); else if the product has **no repos at all**, the new repo becomes automation (implied; it must be main, enforced by the main check); else if the product has an automation repo, the new repo is not automation; else (repos exist, none automation) 409 `errNoAutomation`. A `PUT` that sets `is_automation:true` on the repo that already is automation is idempotent 200. `PUT is_automation:false` on the automation repo is 409 "this is the automation repo: promote another repo first" (after the state checks). Re-running the migration heals S2 (main becomes automation).

### Checklist of every main use (non-test, `grep -rnE 'IsMain|is_main|isMain|hasMain|errNoMain' ai-backend --include=*.go --exclude=*_test.go --exclude-dir=vendor`)
KEEP = stays main-specific; SWITCH = becomes automation; ADD = automation twin added.

| Where | Use | Action |
|---|---|---|
| `models.go` `ProductRepo.IsMain`; comments on `Product.Repos` ("legacy fields mirror the main repo") and `ProductRepo` ("main, central or sibling") | field and docs | KEEP field; ADD `IsAutomation bool json:"is_automation"`; reword both comments |
| `products.go` `repoCols`, `scanRepo` | column list/scan | ADD `r.is_automation` |
| `products.go` `sortRepos` | ordering (main, central, name) | KEEP + ADD automation rank: main, automation, central, name |
| `products.go` `errNoMain` | error | KEEP + ADD `errNoAutomation` |
| `products.go` `createProduct`: flat body `repoBody{... IsMain: true}`; loop `if in.IsMain` choosing the legacy INSERT source | create | flat body: ADD `IsAutomation: true`; INSERT source: SWITCH to the automation row (the mirror overwrites it in the same tx anyway) |
| `products.go` `updateProduct` staging-change clear `WHERE product_id=? AND is_main=1` | stale verify | SWITCH to `is_automation=1` |
| `products.go` `applyPushPolicies` `allow := rp.IsMain \|\| flag` | push URL | SWITCH |
| `products.go` `verifyAndStore` `if repo.IsMain` (Playwright + probe) | verify | SWITCH |
| `products.go` `storeVerify` `SELECT is_main`, stale compare, mirror trigger | store | SWITCH to `is_automation` |
| `products.go` `verifyAllProducts` `hasMain` skip and `rp.IsMain` branch | scheduler | KEEP main skip + ADD automation skip; SWITCH the Playwright/probe branch |
| `product_repos.go` `writeAllowed`, `repoWriteAllowed` (SQL selects `r.is_main`) | guard | SWITCH |
| `product_repos.go` `repoBody`, `repoPatch` | request | KEEP `is_main`; ADD `is_automation` |
| `product_repos.go` `validateRepoSet` | create validation | KEEP main count; ADD defaulting (returns the defaulted set) |
| `product_repos.go` `mirrorMainToLegacy` | mirror | SWITCH (rename; see mirror SQL) |
| `product_repos.go` `insertRepo` | INSERT | ADD `is_automation` |
| `product_repos.go` `hasMain` | helper | KEEP + ADD `hasAutomation` |
| `product_repos.go` `loadRepoForRoute` | route load | KEEP + ADD twin |
| `product_repos.go` `addRepo`: `!hasMain && !in.IsMain`; promotion `SET is_main=0`; `if in.IsMain` -> `applyPushPolicies` | add | KEEP the main parts; ADD automation rule, flag clear, and push-policy trigger on automation change |
| `product_repos.go` `updateRepo`: `!hasMain`; `!*patch.IsMain && cur.IsMain` (409); `promote`; `isMain, isCentral := cur.IsMain \|\| promote`; `UPDATE ... is_main=?` ; clear-verify `repoURL/rootDir changed \|\| promote`; `mainChanged = promote` -> `applyPushPolicies` | update | KEEP main handling; ADD automation twins (409 on unset, promotion, write `is_automation`); SWITCH clear-verify condition to `automationPromoted \|\| automationDemoted` (main promotion alone no longer clears); SWITCH the push-policy trigger to automation change |
| `product_repos.go` `deleteRepo`: `!hasMain`; `cur.IsMain` (409) | delete | KEEP + ADD twins |
| `product_repos.go` `putBranches`: `!hasMain`; mirror trigger `if cur.IsMain` | branches | KEEP check + ADD twin; SWITCH mirror trigger to `cur.IsAutomation` |
| `product_repos.go` `runSync` `SyncBranches(..., repo.IsMain \|\| flag)` | push policy | SWITCH |
| `product_repos.go` `legacyRepoAction`: main selection loop, `errNoMain`, checkout load `r.is_main = 1`, its `errNoMain` | legacy routes | SWITCH selection and load to automation; KEEP `errNoMain` first + ADD twin |
| `migrations.go` `main_flag`, backfill 1 and 2 (`WHERE r.is_main = 1 AND p.branch <> ''`) | applied entries | KEEP (not edited). Backfill 2 seeded branches only for main repos, which is why the mirror needs a fallback (below) |
| Dashboard `is_main` / `mainRepo` uses | see Dashboard | per Dashboard section |

### Legacy mirror
`mirrorAutomationToLegacy(tx, productID)`:
```sql
UPDATE products p
JOIN product_repos r ON r.product_id = p.id AND r.is_automation = 1
LEFT JOIN product_repo_branches b ON b.repo_id = r.id AND b.is_default = 1
SET p.repo_url = r.repo_url, p.root_dir = r.root_dir,
    p.branch = COALESCE(b.branch, (SELECT MIN(x.branch) FROM product_repo_branches x WHERE x.repo_id = r.id), ''),
    p.playwright_accessible = r.playwright_accessible, p.last_verified_at = r.last_verified_at,
    p.verify_report = r.verify_report
WHERE p.id = ?
```
When automation != main the automation repo may have no default branch (backfill entry 5 only seeded main repos). The branch is then its default, else its first branch (alphabetical, via `MIN`), else `''`. A `''` branch is safe for the pre-PRODUCT_REPOS binary: it only checks out `if p.Branch != ""` after clone, and its `/checkout` without a body branch returns 400 "branch is required" (`products.go` at `453ccec`). Under the PRODUCT_REPOS invariants a non-empty branch list always has a default, so the `MIN` fallback is defensive; the dashboard shows a hint on an automation repo with no branches. It runs in the same tx as every change to the automation repo's url, dir, branches/default, verify state, or which repo is automation. `uq_products_root_dir` cannot collide: repo `root_dir`s are globally unique (`uq_product_repos_root_dir`) and repo-less products' legacy `root_dir` is covered by `otherRootDirs`.

### Verify and staleness
*   Callers pass `requirePlaywright = repo.IsAutomation` (`VerifyRepo` already takes the parameter, `repopool.go`; no change there). The staging probe merges into the **automation** report only. A main repo that is not automation gets the git checks only (`root_dir`, `git_repo`, `origin_matches`).
*   `storeVerify` compares `is_automation`, and for the automation repo also `staging_url`; mirrors when automation.
*   A `staging_url` change clears the automation repo's state and re-mirrors.
*   `HasPlaywright` scans the root and immediate subdirectories (`repopool.go`). **Layout check (read-only, 2026-11-13):** `gh api repos/developersismedika/automation-test-hisv3/contents --jq '.[]|.type+" "+.name'` lists `playwright.config.ts` and `package.json` at the repo root (plus `tests/`, `src/`, `config/`, ...), so the check passes at the root.
*   **Main health indicator.** A broken main must not hide behind a green product chip. Reports are stored per repo, including non-automation ones (`storeVerify` writes `rep.OK` to that row's `playwright_accessible`, which for a non-automation repo means "checks passed"). The product list shows two chips when automation != main: "Automation: Accessible/Failing/Unverified" and "Main: OK/Failing/Unverified", the latter from the main repo's `last_verified_at` and `verify_report.ok`. No new API field.

### Write guard and push policy
```go
func writeAllowed(isAutomation, flag bool, op RepoOp) error   // was isMain
// repoWriteAllowed SQL: SELECT r.is_automation, COALESCE(s.allow_non_automation_work,0) FROM product_repos r LEFT JOIN product_settings s ...
```
Truth table: unknown op denied; `isAutomation` allowed; `!isAutomation && flag` allowed; otherwise `ErrWriteNotAllowed`; no row `ErrRepoNotFound`. `ErrWriteNotAllowed` text becomes `"repo is read-only: it is not the automation repo and allow_non_automation_work is off"` (no test compares the text; the 403 mapping is unchanged).

**The DB guard is authoritative; the disabled push URL is a courtesy.** `allowPush = repo.IsAutomation || flag` in `runSync` and `applyPushPolicies`. `SyncBranches` re-asserts the policy on every run (`setPushPolicyLocked` is called after the clone step, `repopool.go` around `SyncBranches`), using the `allowPush` the handler computes from the product it loaded just before. `applyPushPolicies` (after any automation promotion and after a flag change) now retries `Pool.SetPushPolicy` on `ErrBusy`, bounded to 3 attempts with 1 s between them; if still busy it logs and the next sync fixes it. **TOCTOU, accepted:** between loading the product and applying the policy a promotion or flag change can commit, so the push URL can lag by one sync; the DB guard evaluated at call time is what future write routes depend on. Decision 2 (main read-only on the remote) is an owner decision.

### API
Repo JSON gains `"is_automation": bool`; `repos` order is main, automation (if not main), central (if neither), then `name`.

```go
type repoBody struct { ...; IsAutomation bool  `json:"is_automation"` }   // create; absent == false
type repoPatch struct { ...; IsAutomation *bool `json:"is_automation"` }  // nil == no change
func validateRepoSet(repos []repoBody) ([]repoBody, string) // returns the defaulted copy
```
`validateRepoSet`: first the existing "exactly one `is_main`" check; then count `is_automation`: 0 -> the main row gets `IsAutomation=true` in the returned copy; more than 1 -> 400 `"at most one repo can be the automation repo"`. (Zero after defaulting cannot happen.) Request bodies are decoded strictly (`decodeStrictJSON`), so `is_automation` is a declared field; the previous binary would reject it with 400 (only matters if a new dashboard talks to a rolled-back binary).

```json
POST /api/products/{id}/repos
{ "name": "automation-test-hisv3", "repo_url": "https://github.com/o/automation-test-hisv3", "root_dir": "o/automation-test-hisv3",
  "is_automation": true, "branches": ["main"], "default_branch": "main" }
```

| Route | Change |
|---|---|
| `POST /api/products` | `repos[]` accept `is_automation`; defaulting as above; flat form: one repo that is main and automation |
| `POST /{id}/repos` | automation rule (precedence table); `is_automation:true` promotes atomically |
| `PUT /{id}/repos/{rid}` | `is_automation:true` promotes (idempotent if already automation), may carry other fields; `false` on the automation repo is 409 |
| `DELETE /{id}/repos/{rid}` | the automation repo is 409 (main still 409) |
| per-repo verify; legacy routes | per Verify and the precedence table |

**Error table additions.**

| Status | When / message |
|---|---|
| 400 | `"at most one repo can be the automation repo"` (create) |
| 409 | `"this is the automation repo: promote another repo first"` (PUT `is_automation:false`) |
| 409 | `"the automation repo cannot be deleted: promote another repo first, or delete the product"` |
| 409 | `"product has no automation repo"` (`errNoAutomation`, per the table) |
| 409 (key map) | `duplicateKeyMessages` in `products.go` gains `"uq_product_repos_automation": "the product already has an automation repo"` (`conflictMessage` maps 1062 by key name) |

Role gating is unchanged (`is_automation` moves like main; it is not an Admin/DevOps-only flag).

### Role matrix (per repo)

| Capability | main | automation | central | other |
|---|---|---|---|---|
| AI base workspace | yes | no | no | no |
| Playwright check, staging probe | only if also automation | yes | no | no |
| Product-level and legacy mirror | only if also automation | yes | no | no |
| Remote write (`repoWriteAllowed`), push URL enabled | only if also automation or flag | yes | only if flag | only if flag |
| Delete / unset without promoting another | no | no | yes | yes |

### HISv3 transition (post-deploy, explicit, human-run via the dashboard)
1.  Deploy: migration, then binary. Backfill makes automation = main on every product: **no behaviour change**.
2.  For HISv3: ensure both `his-v3-ai` and `automation-test-hisv3` are registered repos (Add repo if missing; sync branches).
3.  Make `his-v3-ai` main (Make main), then make `automation-test-hisv3` automation (Make automation), in that order or either; they are independent.
4.  Expected transient state: the promoted automation repo and the demoted repo are **Unverified** (their reports are cleared; Decision 8) and the product chip shows Unverified; `his-v3-ai` is now read-only on the remote while the flag is off.
5.  Click Verify on `automation-test-hisv3` (Playwright check at its root, staging probe) and on `his-v3-ai` (git checks). Chip returns to Accessible.

### Dashboard
Files and exact edits (names, not line numbers, where they drift):
*   `views/products/api.ts`: `Repo.is_automation: boolean` (next to `is_main`); new `automationRepo(p)` beside `mainRepo`; the comment "Legacy fields, mirrored from the main repo" (`Product`) becomes "...automation repo". `repoFieldProblem` and `branchProblem` need **no rule change** (the role is not a text field).
*   `views/products/RepoCard.tsx`: `RepoAction` union adds `'automation'` (`:126`); badge **Automation** next to Main/Central (`:161-162`); the "Read only" badge condition `!repo.is_main && !allowWork` becomes `!repo.is_automation && !allowWork` (`:163`) with the title "Not the automation repo: read-only on the remote"; a **Make automation** button beside **Make main** (`:234-244`), shown when `!repo.is_automation`, no "Unset" button; the delete button also disabled for the automation repo with tooltip "Promote another repo to automation first" (`:223-224`); the Verify tooltip (`:187`) becomes "Check the checkout and origin; the automation repo also gets the Playwright and staging URL checks"; when the automation repo has no `last_verified_at` show an amber "Verify now" link; when it has no branches show "Set a default branch (the legacy mirror needs one)".
*   `views/products/ProductDetail.tsx`: `FLAG_WARNING` (`:19-20`) ends "...Automation scripts normally live only in the automation repo."; the flag description (`:273`) says "only the automation repo may create branches, push or open PRs"; the no-main banner (`:244-246`) keeps "No main repo: add one." and drops "The main repo holds the automation scripts."; new banner "No automation repo: promote one (Make automation) or add one with Automation repo ticked." shown when `hasMain && !hasAutomation` (`hasAutomation` next to `hasMain`, `:78`); `NewRepoForm` and `emptyNewRepo` (`:37-46`) gain `is_automation` (default `product.repos.length === 0`); the create payload (`:190`) adds `is_automation: f.is_automation`; the Add-repo form (`:339-365`) adds an **Automation repo** checkbox beside "Main repo", with "The current automation repo becomes a sibling" (like the existing main warning, `:360`); `hasMain`-gated pieces (`:79`, `:306`, `:341`, `:365`) are unchanged for main and get the automation twin where they gate the add form (`showAddForm` also shows when `!hasAutomation`); `onRepoAction` handles `'automation'` -> `PUT {is_automation:true}` (beside `:145`). **Warning when automation != main** (amber panel note): "Automation scripts live in `<automation>`, not in the main repo `<main>`: Playwright verify, PR source and the product status apply to `<automation>`. `<main>` (the AI base workspace) is read-only on the remote unless Allow non-automation work is on."
*   `views/ProductsView.tsx`: `StatusChip` (`:31-34`) reads `automationRepo(p)` ("No automation repo" badge if missing) and, when automation != main, adds the "Main: ..." chip (see Verify); the table column header "Main repo / branch" (`:192`) and the cell (`:212`, `:231`) show the automation repo/branch, with the main name in a muted second line when different; the add-product payload (`:118`) adds `is_automation: true`; the helper text (`:268`) says "The first repo is both the main (AI workspace) and the automation repo. Change roles in the product detail."
*   `dashboard/README.md`: `/products` row (`:59`) badge list "Main, Automation, Central" and the Make main/automation actions; the `/repos` row (`:117`) "set main/automation/central".

### Tests
**Existing tests that change** (all in `ai-backend/internal`):
*   `db/migrations_test.go` `TestMigrationsIdempotentAndBackfill`: after the double run, assert every legacy-backfilled repo has `is_automation=1` (the 2 repos, equal to the main ids); insert a product with an `is_main=1, is_automation=0` repo and one with automation != main, run migrations twice, assert the first is healed and the second untouched.
*   `db/migrations_test.go` `TestUniqueKeyBackstopsAndCascade`: add a second-automation 1062 on `uq_product_repos_automation`.
*   `api/product_repos_db_test.go` `assertMirror`: find the **automation** row (and assert exactly one main and one automation), compare the legacy columns to it (branch per the fallback rule).
*   `TestProductsAPI_RepoLifecycleAndInvariants`: the promotion expectation (`find(...).IsMain ... p.RootDir != "o/his-v3-ai" || p.Branch != "dev"`) changes: promoting a repo to **main** no longer moves the mirror (root_dir stays the automation repo's, branch unchanged); add an automation promotion step where the mirror moves; add unset-automation and delete-automation 409 cases.
*   `TestProductsAPI_RepoLessProduct`: the four 409 cases keep the `no main repo` text (main is checked first); the final repair (`POST /repos` `is_main:true`, no `is_automation`, product has no repos) must also yield `IsAutomation`; assert via `assertMirror`.
*   `TestMirrorDrift`: add `automate` (`PUT is_automation:true`) as an operation and `is_automation` on random adds; delete/unset of main or automation repos is 409 and must leave the invariants intact.
*   `TestParallelPromotions`: keep, plus a variant promoting automation in parallel with `storeVerify`; assert exactly one automation and a consistent mirror.
*   `api/product_repos_test.go`: `TestWriteAllowedTruthTable` (first parameter is automation; a main-only repo is denied with the flag off), `TestConflictMessage` (key `uq_product_repos_automation`), `TestProductJSONShape` (`is_automation` key present), `TestOrdering` (main, automation, central, name), `TestValidateRepoAndSet` (returns the defaulted set; zero -> main; two -> 400; automation != main accepted).
**New tests:**
*   Pure: `parseSkipIf` (BOM, leading blank lines, case, no space, empty query, plain entries unchanged); `summarize` skips the directive.
*   DB-gated (`QA_TEST_MYSQL_DSN` via `testdb.New`): runner skip-if (`>0` skips, `0` runs, NULL, two rows, query error each abort/skip as specified); the guarded ALTER twice; the precedence table (S1, S2, repo-less, both missing) for every row, including `PUT is_automation:true` with other fields; verify: automation report has `playwright` and `staging_url`, non-automation main has neither, product `playwright_accessible` equals the automation repo's; split-role `createSplit` helper (automation != main) through create, mirror, staging change clearing only the automation repo; push-policy helper retries on `ErrBusy` (pure test with an injected function).
*   Dashboard: `cd dashboard && npm run build && npx tsc --noEmit`.

### Rollback
*   **To the previous release (PRODUCT_REPOS):** the column and key are ignored (its inserts and selects name columns). Its promotion of main does not touch `is_automation`, so automation may diverge from main; its mirror writes main into the legacy columns; **it can also delete the repo that is automation (a sibling, if main differs), and the heal then makes main the automation repo silently.** Before rolling back, save `SELECT product_id, id, name FROM product_repos WHERE is_automation = 1`; after roll-forward compare it with the same query to see silent changes.
*   **Roll-forward procedure** (quiet window): (1) run the migration command (heals products with no automation repo: main becomes automation); (2) re-point the legacy columns, runnable as is:
```sql
UPDATE products p
JOIN product_repos r ON r.product_id = p.id AND r.is_automation = 1
LEFT JOIN product_repo_branches b ON b.repo_id = r.id AND b.is_default = 1
SET p.repo_url = r.repo_url, p.root_dir = r.root_dir,
    p.branch = COALESCE(b.branch, (SELECT MIN(x.branch) FROM product_repo_branches x WHERE x.repo_id = r.id), ''),
    p.playwright_accessible = r.playwright_accessible, p.last_verified_at = r.last_verified_at,
    p.verify_report = r.verify_report;
```
(3) run the post-flight queries. Do not edit products during the window (inherited limitation).
*   **To the pre-PRODUCT_REPOS binary:** the legacy columns describe the automation repo (Decision 6).
*   **Post-flight** (zero rows each; recorded in `ai-backend/internal/db/README.md`):
```sql
-- roles: every product with repos has exactly one main and exactly one automation repo
SELECT p.id, SUM(r.is_main) AS mains, SUM(r.is_automation) AS automations
FROM products p JOIN product_repos r ON r.product_id = p.id
GROUP BY p.id HAVING mains <> 1 OR automations <> 1;
-- legacy drift: all six legacy columns equal the automation repo's
SELECT p.id FROM products p
JOIN product_repos r ON r.product_id = p.id AND r.is_automation = 1
LEFT JOIN product_repo_branches b ON b.repo_id = r.id AND b.is_default = 1
WHERE NOT (p.repo_url <=> r.repo_url AND p.root_dir <=> r.root_dir
  AND p.branch <=> COALESCE(b.branch, (SELECT MIN(x.branch) FROM product_repo_branches x WHERE x.repo_id = r.id), '')
  AND p.playwright_accessible <=> r.playwright_accessible AND p.last_verified_at <=> r.last_verified_at
  AND p.verify_report <=> r.verify_report);
```
(Both queries are run on the scratch server in the mandatory step before they are documented.)

## Accepted limitations
*   Rollback-window edits are lost on roll-forward; a silent heal can change the automation repo (mitigated by the pre/post snapshot).
*   The push URL can lag one sync behind a role/flag change; the DB guard is the authority.
*   The skip-if check and the ALTER are not atomic across two runners (manual command).
*   No DB protection against deleting the automation repo; handlers only.

## Assumptions Goldfish reviewers must challenge
*   The ALTER (generated column referencing a column added in the same statement, `ALGORITHM=COPY, LOCK=SHARED`, leading comment line) works on MySQL 8.0.33. Unverified until the scratch run; the scratch run must use the full slice, twice.
*   The self-join `UPDATE` avoids error 1093 and is a no-op on re-run and on products with automation != main.
*   The `-- skip-if:` grammar cannot misfire on an existing entry and a guard failure always aborts.
*   Mirroring the automation row (not main) is the right rollback choice, and nothing else reads the legacy columns or `is_main` (grep result in Decision 10; `playwright-executor` was searched, other consumers outside the repo are unknown).
*   A `''` mirror branch is harmless to the pre-PRODUCT_REPOS binary (verified in its `products.go` at `453ccec`, the clone step and `/checkout`).
*   Clearing verify on both demoted and promoted automation repos, and not auto-verifying, is acceptable.
*   Making a non-automation main read-only on the remote is acceptable for the AI workspace.
*   The precedence table is complete for every route that today calls `hasMain`/`errNoMain`.

## Target Files
*   Already in this PR: `docs/designs/PRODUCT_REPOS_AUTOMATION_ROLE.md`; `docs/designs/PRODUCT_REPOS.md` (Status, "Amended by", "Repo state"); `docs/designs/SITEMAP.md` (see below); `ai-backend/internal/db/README.md` (the merge-conflict block, with its HEAD / `=======` / `origin/main` markers, at lines 38-46 of `origin/main`'s copy from an earlier merge, is resolved here by keeping both table lists; no other file on `main` contains conflict markers).
*   Implementation (after approval): `ai-backend/internal/db/migrations.go` (two appended entries, `parseSkipIf`, runner and `summarize` changes), `ai-backend/internal/db/migrations_test.go`, `ai-backend/internal/models/models.go`, `ai-backend/internal/api/product_repos.go`, `ai-backend/internal/api/products.go`, `ai-backend/internal/api/product_repos_test.go`, `ai-backend/internal/api/product_repos_db_test.go`, `ai-backend/internal/api/products_test.go` (only if it asserts changed text), `dashboard/src/views/products/api.ts`, `RepoCard.tsx`, `ProductDetail.tsx`, `dashboard/src/views/ProductsView.tsx`; READMEs: `ai-backend/internal/api/README.md` (product route rows, the Verify row, and the invariants about mirror, write guard, repo-less products), `ai-backend/internal/db/README.md` (new columns and entries, runner guard wording, post-flight and roll-forward SQL), `dashboard/README.md`.
*   `docs/designs/SITEMAP.md` status changes: (1) the `PRODUCT_REPOS.md` row: Draft -> Implemented (in this PR); (2) this spec's row added as Draft (in this PR); (3) this spec's row Draft -> Implemented when the implementation PR merges.
*   Not changed: `router.go`, `repopool.go` and its README (`VerifyRepo(requirePlaywright)` already exists), executor, Dockerfile, compose, `.env*`.

## Verification Plan
1.  **Blocking:** scratch `mysql:8.0.33` run of the real full `migrations` slice twice (see Data model); record the result in this spec; also run the two post-flight queries and the roll-forward UPDATE there.
2.  `cd ai-backend && go build -mod=mod ./... && go test -mod=mod ./...`, then again with `QA_TEST_MYSQL_DSN` on the scratch server.
3.  `cd dashboard && npm run build && npx tsc --noEmit`.
4.  Scratch stack: split-role product (main `his-v3-ai`, automation `automation-test-hisv3`): Playwright check only on the automation repo; flag off: automation push URL enabled, main disabled; promote automation moves the badge and re-mirrors; unset/delete automation gets 409; the degraded-state rows of the table by editing `is_automation` in SQL.
5.  Rollback drill (scratch): deploy new, split roles, redeploy the previous binary (it lists repos), delete the automation sibling with it, roll forward, run the roll-forward procedure and compare snapshots.
6.  Deploy only with a separate approval (`deploy-qa-tool`); migration before binary.
