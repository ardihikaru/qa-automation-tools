# Multi-repo products (main, central and sibling repos, multi-branch checkouts)

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Implemented and deployed (PR #13 `feat(products): multi-repo products ...`, follow-up PR #14 strict JSON). Amended by the automation-role draft below.
*   **Date:** 2026-11-13
*   **Builds on:** [PRODUCTS_MASTER_AND_REPO_POOL.md](PRODUCTS_MASTER_AND_REPO_POOL.md) (implemented, PR #4). Everything that spec says about the pool, locks, child environment, SSRF probe and roles stays in force unless this document says otherwise. On approval that spec gets a "superseded in part by PRODUCT_REPOS.md" note (data model, routes, verify).
*   **Amended by:** [PRODUCT_REPOS_AUTOMATION_ROLE.md](PRODUCT_REPOS_AUTOMATION_ROLE.md) (Draft). "Main" stays the AI agent's base workspace; a new per-product **automation** repo role (exactly one, may equal main) now holds the Playwright scripts, is the only PR source and the write-guard target (`repoWriteAllowed`: automation OR `allow_non_automation_work`), receives the Playwright verify check, and is what the product-level and legacy columns mirror. Existing products are backfilled with automation = main. Where this spec says the main repo holds the scripts or is the PR source (Overview, Invariants 1 and 7, Verify per repo, Write guard, Legacy compatibility), the amendment wins.
*   **Repo state:** this spec and its `SITEMAP.md` row are on `main` (merged with PR #13).

## Overview
A product today has exactly one repo: `products.repo_url`, `root_dir`, `branch` and the verify state live on the `products` row (`ai-backend/internal/db/migrations.go:286-301`, `ai-backend/internal/models/models.go:207-219`). Real products span several repos. HISv3, for example, has a **central repo** (`his-v3-ai`) that manages the other repos.

A product now owns a list of repos with these roles:

*   **Main repo:** exactly one per product. It holds the automation test scripts and is the only repo PRs may come from.
*   **Central repo:** optional, at most one. Informational (it manages the siblings). May be the same repo as main.
*   **Sibling repos:** every other repo. Read context only unless the product flag `allow_non_automation_work` is on.

Each repo lists the branches to keep checked out and one default branch.

## Problem
*   One repo per product cannot describe HISv3 or any multi-repo product.
*   Only one branch can be checked out at a time (`Checkout` flips one working tree: `repopool.go:352-379`).
*   Nothing records which repo PRs may come from, and nothing stops a future PR or branch feature from touching a sibling repo.

## Goals
*   `product_settings`, `product_repos` and `product_repo_branches` tables; every legacy product migrates to one main repo; existing products keep working.
*   Exactly one main and at most one central repo per product, and at most one default branch per repo, enforced by the database.
*   Several branches per repo checked out side by side (worktrees), with path-escape and symlink guards.
*   A write guard (`repoWriteAllowed`) plus a real defence on disk (disabled push URL for non-main clones).
*   Routes under `/api/products/{id}/repos...` with fully specified contracts; a dashboard product detail panel.
*   Rollback to the previous release without data repair.

## Non-Goals
*   No auto-discovery of siblings from the central repo.
*   No PR-creation UI or API. This spec defines and tests the gate and records which repo PRs may come from.
*   The executor still uses a single `HISV3_STAGING_URL`; it does not consume per-repo paths (follow-up).
*   No changes to pool mount, `gh` auth, `REPO_POOL_ALLOWED_OWNERS` or the probe client.
*   No per-repo staging URL: `staging_url` stays on the product.
*   Deleting a checkout from disk. Deleting a registry row (or changing a repo's `root_dir`) leaves files in place.
*   Moving an existing checkout when `root_dir` is edited (the old directory becomes an orphan, see Accepted limitations).
*   Verifying each branch worktree (Decision 5).

## Decisions (owner)
1.  **Flag storage:** new table `product_settings` (`product_id` PK/FK CASCADE, `allow_non_automation_work` default 0) instead of an ALTER on `products`. A missing row means false. No settings backfill is needed; no error-1060 tolerance in `RunMigrations`.
2.  **Main repo always required.** No repo-less products by design; the main repo cannot be deleted (promote another repo first, or delete the product). (A repo-less product can still appear through a backfill conflict or a rollback window; behaviour is defined under Legacy compatibility.)
3.  **Only Admin and DevOps may toggle `allow_non_automation_work`.**
4.  **The gate covers remote writes only** (branch creation, push, PR). Clone, sync and verify stay allowed on siblings.
5.  **Verify checks the clone only** (default branch), not each worktree.
6.  **Layout:** worktrees at `<root_dir>.branches/<slug>`, one clone per repo.
7.  **Default branch** is a flag on the branch row (`is_default`), not a duplicated column; `default_branch` in JSON is derived from it.
8.  **DDL** uses flag-derived generated columns (not the FK column) for the uniqueness trick (see below), and a mandatory pre-implementation scratch run.

## Design

### Invariants
1.  Exactly one main repo per product (DB: at most one; handlers: never zero, see Legacy compatibility for the exceptions).
2.  At most one central repo per product (DB). May equal main (same row `is_main=1`, `is_central=1`).
3.  Repo `name` unique per product (DB). `repo_url` unique per product: the DB index is an exact-string `UNIQUE(product_id, repo_url)` and is **advisory only** (`https://github.com/a/b` and `https://github.com/a/b.git` differ to MySQL). Handlers compare with `repopool.NormalizeRepo` (`repopool.go:76-91`) before insert/update and return 409; the DB index only catches exact repeats.
4.  `root_dir` unique across all repos of all products (DB: `uq_product_repos_root_dir`, case-insensitive under the table collation), and no repo's footprint (`root_dir`, `root_dir.branches`) overlaps another's (handlers).
5.  A repo's branch list is either empty (legacy only) or has exactly one default: at most one default is DB-enforced, "exactly one when non-empty" is handler-enforced. Max 10 branches per repo.
6.  Branch names and slugs are unique per repo under a **binary** collation (case-sensitive, matching git refs).
7.  Product-level `playwright_accessible`, `last_verified_at`, `verify_report`, `repo_url`, `root_dir`, `branch` mean "the main repo's" (mirrored, see Legacy compatibility).

### Data model
Five separate entries appended to `migrations` in `ai-backend/internal/db/migrations.go` after the `products` statement (list currently ends at line 302; `RunMigrations` is `:304-317`). Applied entries are not edited. All are `CREATE TABLE IF NOT EXISTS` or guarded `INSERT ... SELECT`, so the existing runner (every statement, every run, first error aborts) needs no change.

```sql
-- entry 1
CREATE TABLE IF NOT EXISTS product_settings (
  product_id INT PRIMARY KEY,
  allow_non_automation_work TINYINT(1) NOT NULL DEFAULT 0,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_product_settings_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- entry 2
CREATE TABLE IF NOT EXISTS product_repos (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  name VARCHAR(100) NOT NULL,
  repo_url VARCHAR(300) NOT NULL,
  root_dir VARCHAR(200) NOT NULL,
  is_main TINYINT(1) NOT NULL DEFAULT 0,
  is_central TINYINT(1) NOT NULL DEFAULT 0,
  main_flag    TINYINT GENERATED ALWAYS AS (IF(is_main = 1, 1, NULL)) STORED,
  central_flag TINYINT GENERATED ALWAYS AS (IF(is_central = 1, 1, NULL)) STORED,
  playwright_accessible TINYINT(1) NOT NULL DEFAULT 0,
  last_verified_at DATETIME NULL,
  verify_report JSON NULL,
  sync_report JSON NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_product_repos_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_product_repos_root_dir (root_dir),
  UNIQUE KEY uq_product_repos_name (product_id, name),
  UNIQUE KEY uq_product_repos_url (product_id, repo_url),
  UNIQUE KEY uq_product_repos_main (product_id, main_flag),
  UNIQUE KEY uq_product_repos_central (product_id, central_flag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- entry 3
CREATE TABLE IF NOT EXISTS product_repo_branches (
  id INT AUTO_INCREMENT PRIMARY KEY,
  repo_id INT NOT NULL,
  branch VARCHAR(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  slug   VARCHAR(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  is_default TINYINT(1) NOT NULL DEFAULT 0,
  default_flag TINYINT GENERATED ALWAYS AS (IF(is_default = 1, 1, NULL)) STORED,
  CONSTRAINT fk_product_repo_branches_repo FOREIGN KEY (repo_id) REFERENCES product_repos(id) ON DELETE CASCADE,
  UNIQUE KEY uq_product_repo_branches (repo_id, branch),
  UNIQUE KEY uq_product_repo_branch_slug (repo_id, slug),
  UNIQUE KEY uq_product_repo_branch_default (repo_id, default_flag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

**Why the generated columns derive from the flag.** The first draft derived `main_key` from `product_id`, which is also the FK column with `ON DELETE CASCADE`. MySQL 8 rejects foreign keys with referential actions on base columns of stored generated columns (error 3104, "Cannot define foreign key with CASCADE ... on base column of stored generated column"). Here `main_flag` depends only on `is_main` and `central_flag` only on `is_central`, which are in no foreign key, and uniqueness comes from `UNIQUE(product_id, main_flag)`: unique indexes ignore rows with a NULL part, so a second row with `main_flag=1` for the same product collides and unflagged rows never do. This is believed valid on MySQL 8.0 but **has not been run** (see mandatory pre-implementation step).

**Slug column.** `slug` is stored (not recomputed) so `UNIQUE(repo_id, slug)` catches `a/b` vs `a--b`. It is `VARCHAR(200)` so a legacy branch of up to 200 characters can be backfilled; the handler caps newly registered branches at a 100-character slug (see Layout).

**Backfill** (entries 4 and 5; each statement its own entry, in this order; no settings backfill):

```sql
-- entry 4: one main repo per legacy product; skips conflicting rows instead of aborting
INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_central,
                           playwright_accessible, last_verified_at, verify_report)
SELECT p.id, 'main', p.repo_url, p.root_dir, 1, 0, p.playwright_accessible, p.last_verified_at, p.verify_report
FROM products p
WHERE NOT EXISTS (SELECT 1 FROM product_repos r WHERE r.product_id = p.id)
  AND p.root_dir NOT REGEXP '(^|/)[^/]*\\.branches(/|$)'
  AND NOT EXISTS (SELECT 1 FROM product_repos r2 WHERE r2.root_dir = p.root_dir)
ON DUPLICATE KEY UPDATE product_id = product_id;

-- entry 5: legacy branch becomes the default and sole branch, only for repos with no branch rows yet
INSERT INTO product_repo_branches (repo_id, branch, slug, is_default)
SELECT r.id, p.branch, REPLACE(p.branch, '/', '--'), 1
FROM product_repos r JOIN products p ON p.id = r.product_id
WHERE r.is_main = 1 AND p.branch <> ''
  AND NOT EXISTS (SELECT 1 FROM product_repo_branches b WHERE b.repo_id = r.id)
ON DUPLICATE KEY UPDATE product_repo_branches.branch = product_repo_branches.branch;
```
The `NOT EXISTS` guards make a re-run a no-op even after users edit branches; `ON DUPLICATE KEY UPDATE <table>.x = <table>.x` (qualified, because the SELECT side also has a `branch` column) is a no-op instead of `INSERT IGNORE` so that real errors (data truncation, FK) still abort and are visible.

**Conflicts are reported, not fatal.** A legacy product is skipped by entry 4 if its `root_dir` has a segment ending in `.branches` (any case: the MySQL regexp is case-insensitive under the `_ci` collation) or equals an existing repo's `root_dir`. Pre-API overlap rules already prevent nested `root_dir`s between products (`rootDirOverlap`, `products.go:92-99`). Operators run this **pre-flight query before migrating** (also placed in `ai-backend/internal/db/README.md`) and fix rows first:
```sql
SELECT id, name, root_dir FROM products WHERE root_dir REGEXP '(^|/)[^/]*\\.branches(/|$)';
SELECT a.id, b.id FROM products a JOIN products b ON a.id < b.id
 AND (b.root_dir LIKE CONCAT(a.root_dir,'/%') OR a.root_dir LIKE CONCAT(b.root_dir,'/%'));
```
and this **post-flight query** afterwards (expected: zero rows):
```sql
SELECT p.id, p.name FROM products p WHERE NOT EXISTS (SELECT 1 FROM product_repos r WHERE r.product_id = p.id);
```

**Deploy order.** Run `go run -mod=mod ./cmd/databasemigration` **before** starting the new binary (the old binary ignores the extra tables; the new binary needs them). After the new binary is live, run the migration once more to pick up products created by the old binary in the window (idempotent).

**Mandatory pre-implementation step (blocks the implementation PR).** (1) Run `SELECT VERSION()` on the rnd-v2 MySQL (read-only; the schema work is not done there). (2) Run entries 1 to 5 twice on a **scratch** MySQL 8 instance of the same minor version, never the shared `qa_automation` schema. (3) Record the version, the result (error numbers if any) and the final accepted DDL in this spec before implementation starts. If error 3104 or any DDL error occurs, the DDL here is amended and re-reviewed. Items previously listed as assumptions (generated-column unique index, the 3104 restriction, `ON DUPLICATE KEY UPDATE x = x`, regexp escaping in the Go raw string) are verification outcomes of this step, not beliefs.

**DDL verification result (2026-11-13).** rnd-v2 runs MySQL Community Server 8.0.33. Entries 1 to 5 were run twice on a scratch `mysql:8.0.33` container (local Docker, never `qa_automation`): no error 3104; both runs idempotent, including with legacy rows present. Confirmed: a second main or central repo per product gives 1062 on `uq_product_repos_main` / `_central`; a second default branch gives 1062 on `uq_product_repo_branch_default`; `a/b` and `a--b` collide on `uq_product_repo_branch_slug`; `Main` and `main` coexist (`utf8mb4_bin`); deleting a product cascades to its repos and branches; a legacy product with a `.branches` segment in `root_dir` is skipped by entry 4 without aborting. **One defect found and fixed:** entry 5 originally used `ON DUPLICATE KEY UPDATE branch = branch`, which fails with error 1052 (ambiguous column) because the SELECT also exposes `branch`; the DDL above now qualifies the target column. Still to verify in Go: the regexp escaping inside the migration raw string.

### Legacy compatibility and rollback
The legacy columns `products.repo_url`, `root_dir`, `branch`, `playwright_accessible`, `last_verified_at`, `verify_report` and `uq_products_root_dir` are kept (not dropped, not made nullable) for at least one release.

**One mirror helper.** `mirrorMainToLegacy(tx *sql.Tx, productID int) error`, called in the same transaction by every code path that changes the main repo's `repo_url`, `root_dir`, default branch, verify state, or which repo is main (product create, repo edit, promotion, branches PUT, verify store, legacy routes):
```sql
UPDATE products p
JOIN product_repos r ON r.product_id = p.id AND r.is_main = 1
LEFT JOIN product_repo_branches b ON b.repo_id = r.id AND b.is_default = 1
SET p.repo_url = r.repo_url, p.root_dir = r.root_dir, p.branch = COALESCE(b.branch, ''),
    p.playwright_accessible = r.playwright_accessible, p.last_verified_at = r.last_verified_at,
    p.verify_report = r.verify_report
WHERE p.id = ?
```
No other code writes those legacy columns. A **drift test** (DB-gated, see Test harness) runs a randomised sequence of operations and asserts, after each, that the legacy columns equal the main repo's values.

**Rollback.** Rolling the binary back leaves the old code a coherent single-repo view; extra tables are ignored. Edits made through the old binary during the window update only the legacy columns and are not re-imported (the backfill guards skip products that already have rows). Accepted limitation; do not edit products during a rollback window. The legacy-column drop is a later, separate migration.

**Repo-less products.** They can exist if entry 4 skipped a conflicting product, or if the old binary creates a product during a rollback window and the post-deploy migration was not rerun. Defined behaviour: `GET /api/products` lists them with `repos: []`; every repo-scoped route except `POST /repos` with `is_main: true` returns 409 `"product has no main repo"`; the legacy `/verify|clone|checkout` routes return 409 the same way; the scheduled verifier skips them; the dashboard shows a banner "No main repo: add one". The legacy columns of such a row are untouched.

**Legacy API routes** (kept for the previous dashboard build; all operate on the product's main repo):
*   `POST /{id}/verify`: verify the main repo.
*   `POST /{id}/clone`: clone the main repo, then sync its branch list (same effect as the old clone-then-checkout, `products.go:314-322`).
*   `POST /{id}/checkout` with body `{"branch": "x"}`: validates `x`, sets it as the main repo's default branch (adds it to the list if absent, subject to the 10-branch cap; moves the `is_default` flag in one tx), mirrors to `products.branch`, then syncs all branches. With no body branch it uses the current default; no default and no body branch returns 400 `"branch is required"` (as `products.go:332-334`).
*   The best-effort re-verify after clone/checkout stays (`products.go:353-356`).
*   `PUT /{id}`: legacy repo fields (`repo_url`, `root_dir`, `branch`) in the body are **ignored and the request still returns 200** (documented in the README); only `name`, `staging_url` and the flag are applied.
*   `POST /api/products` with legacy flat fields creates the single main repo (see API).

### Checkout layout: git worktrees, not separate clones
One clone per repo, one `git worktree` per extra branch, as siblings of the clone directory (not inside it):

```
<pool>/<root_dir>                      the clone; holds the default branch
<pool>/<root_dir>.branches/<slug>      linked worktree, one per additional branch
```

Why worktrees: one object store and one `git fetch` per repo (separate clones would multiply disk use of large histories on rnd-v3). Why not inside `<root_dir>`: extra directories in the default branch's tree appear as untracked content and would be walked by `HasPlaywright` (`repopool.go:391-416`) and the executor.

**Slug.** `slug = strings.ReplaceAll(branch, "/", "--")`. It is a single valid directory segment because `branchRe` (`repopool.go:31`) allows only `A-Za-z0-9._/-` with an alphanumeric first character. `ValidateBranch` already rejects `.lock` (`:68-69`), so no separate slug `.lock` rule is needed. Registered branches must have `len(slug) <= 100` (else 400 "branch name too long for a worktree"; no hash fallback, to keep names predictable). The DB `UNIQUE(repo_id, slug)` rejects `a/b` vs `a--b`.

**Limits.** `root_dir` max stays 200 (`ValidateRelDir`, `repopool.go:55`, and the column); slug max 100; joined `root_dir + ".branches/" + slug` at most 310 characters, enforced by the resolver (320 cap); max 10 branches per repo.

**Path resolution (replaces the earlier ValidateRelDir-based wording).** `ValidateRelDir` gains a case-insensitive ban on any segment ending in `.branches`, so a user-supplied `root_dir` can never claim another repo's worktree area. That rule applies **only** to user-supplied `root_dir` (the repo create/edit validators and `ResolveDir`). The worktree path does not go through it. `ResolveDir` (`repopool.go:185-214`) is refactored into two steps: `ValidateRelDir` then an unexported `resolveValidated(rel)` containing the existing symlink/escape logic. A new `ResolveWorktreeDir(rootDir, slug string) (string, error)` validates its two parts separately (`rootDir` with the same segment regexp and `.lock` rule, but without the `.branches` ban because the stored value was already vetted at write time; `slug` against `^[A-Za-z0-9][A-Za-z0-9._-]*$`, length 100, not `.`/`..`), checks the total length, builds `rootDir + ".branches/" + slug` and calls `resolveValidated`. The deepest-existing-ancestor `EvalSymlinks` check therefore covers a `<root_dir>.branches` replaced by a symlink to outside the pool. Additionally, if `<root_dir>.branches` or the slug directory exists, `Lstat` must report a real directory (not a symlink), so a symlink pointing at another in-pool checkout cannot redirect a write. Branch names reach git only after `ValidateBranch`, as arguments or as complete `origin/<branch>` refs, and never form a path (the derived slug does). `Clone` is **not** used on worktrees: worktrees are created only by `git worktree add`.

### Locking
`Pool.lock` is a non-reentrant `TryLock` keyed on the absolute clone path (`repopool.go:216-228`) and `Clone`, `Checkout` and `Verify` each take it themselves (`:331, :363, :430`). Calling them from `SyncBranches` while it holds the lock would return `ErrBusy` against itself. Resolution: split each into an unexported unlocked helper and a thin locking wrapper:

*   `cloneLocked(ctx, repoURL, abs)`, `checkoutLocked(ctx, abs, branch)`, `verifyRepoLocked(ctx, repoURL, abs, requirePlaywright)`, `setPushPolicyLocked(ctx, abs, allowPush)`.
*   Exported `Clone`, `Checkout`, `VerifyRepo`, `SetPushPolicy` validate, resolve, take `p.lock(abs)` and call the helper (behaviour unchanged for existing callers).
*   `SyncBranches` validates, resolves, takes the lock **once** and calls helpers only.
One lock per repo, so syncs of different repos run in parallel and a verify on the same repo returns busy. `ErrBusy`'s message (`repopool.go:42`, "...for this product") becomes `"another operation is already running for this repo"`; `TestLockBusy` (`repopool_test.go:143`) compares the error value, not the text, so it is unaffected (any text assertion in `products_test.go` is updated).

### Sync algorithm (`SyncBranches`)
Signature: `SyncBranches(ctx, repoURL, rel string, branches []BranchSpec, allowPush bool) ([]BranchResult, error)` where `BranchSpec{Name string; Default bool}`. The error return is only for failures that stop the whole sync (invalid input, `ErrBusy`, clone failure, fetch failure); per-branch problems go into the results and never abort the rest.

Steps, all under the one repo lock, all via `run()` with arg slices:
1.  Clone if the directory is absent (`cloneLocked`).
2.  Apply push policy (see Write guard).
3.  `git worktree prune`.
4.  `git fetch --prune origin "+refs/heads/*:refs/remotes/origin/*"` (explicit refspec: independent of how `gh repo clone` configured the remote). Failure aborts the sync (502).
5.  If the list is empty (legacy, `default_branch = ''`) stop after fetch: no checkout (the clone keeps whatever branch it has). A PUT branches that leaves the legacy empty state untouched is allowed.
6.  Read `git worktree list --porcelain` into a path-to-branch map.
7.  **Removals.** For each existing worktree under `<root_dir>.branches/` whose branch is not in the list, or equals the (new) default branch (git forbids one branch in two worktrees): if `git -C <wt> status --porcelain` is non-empty, result `kept_dirty` and keep it; otherwise `git worktree remove <wt>` (never `--force`), result `ok` ("removed").
8.  **Default branch.** If `origin/<default>` does not exist: `missing_remote`. Else if the clone is on another branch and dirty (`status --porcelain`): `kept_dirty` ("clone has local changes; default branch not switched") and continue. Else `git checkout <default> --` and `git merge --ff-only origin/<default>`; a non-fast-forward is `error` (never `reset`, never `clean`). If a dirty kept worktree still holds the new default, the default is `kept_dirty` and not switched.
9.  **Other branches.** For each non-default listed branch: `missing_remote` if `origin/<b>` is absent. Resolve the path with `ResolveWorktreeDir`. If the path exists, **verify before reuse**: `git -C <wt> rev-parse --git-common-dir` resolved with `EvalSymlinks` must equal the clone's `.git` resolved the same way, and the porcelain list must contain this exact path on `refs/heads/<b>`; otherwise result `error` ("path exists but is not the expected worktree"), left untouched. If it does not exist: `git worktree add --track -b <b> <path> origin/<b>` when no local branch `<b>` exists (`git show-ref --verify --quiet refs/heads/<b>`), else `git worktree add <path> <b>`. Then `git merge --ff-only origin/<b>` in the worktree; a diverged branch is `error` and left alone.
10.  Persist `{ "synced_at": ..., "results": [...] }` into `product_repos.sync_report` (so the UI can show the last result after a reload) in a short transaction.
Also if the default branch changes while another branch is listed, the order is: remove the new default's worktree (step 7), switch the clone (8), then add the old default's worktree (9, because it is now a non-default listed branch).

**Budget.** Existing per-step timeouts stay (clone 3 min, fetch 90 s, other git calls 10 to 20 s). The handler wraps the whole sync in `context.WithTimeout(context.WithoutCancel(req.Context()), 10*time.Minute)` (as `products.go:312` detaches from the request). Worst case: 3 min clone + 1.5 min fetch + 10 branches at about 30 s each. This can exceed the 300 s proxy timeout; the operation still finishes server-side and the UI re-reads the product, which shows `sync_report` (accepted limitation inherited from the earlier spec).

**`BranchResult`** (Go struct in `models`): `{ "name": string, "status": "ok"|"error"|"kept_dirty"|"missing_remote", "message": string }`. `message` is the first line of the git error, 200 characters at most (`firstLine`, `products.go:119-128`), never an absolute path.

### `run()` and `childEnv` signature change
`childEnv(safeDir string)` (`repopool.go:233`) becomes `childEnv(safeDirs ...string)`, emitting one `safe.directory` key per non-empty entry (`GIT_CONFIG_COUNT` accordingly). `run(ctx, dir, safeDir string, timeout, name, args...)` (`:258`) becomes `run(ctx, dir string, safeDirs []string, timeout, name, args...)`. Existing callers are updated at `:343, :369, :372, :375, :382, :452` (each passes `[]string{abs}`); worktree operations pass the clone path **and** the worktree path (a linked worktree's git dir lives under the clone's `.git`). `TestChildEnvHasNoSecretsAndScopesSafeDirectory` (`repopool_test.go:194-205`) keeps compiling with the variadic form and gets an added case for two directories; `repopool_test.go` is therefore in Target Files.

### Verify per repo
`Verify` (`repopool.go:418-473`) bundles git checks, the Playwright check and the staging probe. It is split so the probe runs once per product, not once per repo:
*   `VerifyRepo(ctx, repoURL, rel string, requirePlaywright bool) Report`: `root_dir`, `git_repo`, `origin_matches`, and the `playwright` check only when `requirePlaywright` (the main repo). Siblings report checkout facts only; `OK` is true when all included checks pass.
*   `ProbeStaging(ctx, url) Check`: the existing `checkURL` as a `Check` (`staging_url`).
*   `Verify(ctx, repoURL, rel, stagingURL)` stays as `VerifyRepo(..., true)` plus the probe appended, so existing tests (`TestVerify`, `TestVerifyBusyWhenLocked`) and the legacy flow keep their shape.
The main repo's stored report includes the `staging_url` check (so its `OK` equals today's product-level `playwright_accessible` and the mirror stays equivalent); sibling reports do not.

**Scheduled verify.** `verifyAllProducts` (`products.go:441-462`, query `SELECT productCols FROM products`, `:442`) is rewritten: per product with a main repo, probe staging once, then `VerifyRepo` each repo (main with `requirePlaywright=true`, merging the probe check into the main report), then store each result. `productCols` and `scanProduct` (`products.go:23-46`) remain for the product row; a new `repoCols`/`scanRepo` serves `product_repos`. Busy repos are skipped silently as today (`products.go:458`).

**Store (conditional, in one transaction)**, replacing the single UPDATE at `products.go:414-416` (whose `WHERE ... repo_url=? AND staging_url=? AND root_dir=?` kept stale results out):
```sql
-- tx: first statement locks the product row (see Concurrency)
SELECT staging_url FROM products WHERE id = ? FOR UPDATE;
-- inputs unchanged since verification?  (r.is_main read in the same tx)
UPDATE product_repos
SET playwright_accessible = ?, last_verified_at = UTC_TIMESTAMP(), verify_report = ?
WHERE id = ? AND repo_url = ? AND root_dir = ?;
-- mirror only if this repo is main, in the same tx
-- (mirrorMainToLegacy runs only when SELECT is_main FROM product_repos WHERE id = ? returned 1 in this tx)
```
For the main repo the staging URL that was probed is also compared with the locked `staging_url`; if it differs, the result is discarded (stale), as today. Verification is stale (state cleared) per repo when its `repo_url` or `root_dir` changes; a `staging_url` change clears the **main** repo's verify state only (siblings do not depend on it).

### Concurrency
Every repo-mutating handler (create/update/delete repo, branches PUT, promotion, flag change, verify store, legacy routes' DB writes) runs inside `db.WithTx(ctx, func(tx *sql.Tx) error)`, a new helper in `ai-backend/internal/db/connection.go` (today `r.db.Exec/Query` are `*sql.DB` calls and not tx-aware: `connection.go:46-63`; `DB.SQL` is exposed). `WithTx` begins a tx, commits on nil, rolls back otherwise, and retries the whole function up to 3 times on MySQL error 1213 (deadlock; `*mysql.MySQLError.Number`) or 1205 (lock wait timeout).

Fixed order: the first statement of every such tx is `SELECT id FROM products WHERE id = ? FOR UPDATE`, which serialises all mutations of one product; thereafter rows are touched in the order `product_repos` (and `product_repo_branches`), then `product_settings`, then `products` writes (the mirror). Because everything starts at the product row, the effective lock order is the same in every path (this is a deliberate simplification of "repos then products": the product row is only locked first as a mutex and is written last). Verify stores take the same tx shape. Handler pre-checks (overlap, normalised URL, one-main) are **advisory** and give clean 409s; the unique keys are the authority, and a 1062 is mapped by **key name** (below).

Lock for the pool (filesystem) is separate and held outside DB txs: handlers do the git work first, then open the short DB tx; no tx stays open across git or network calls.

### Write guard
**Definition.** A write-class operation creates or changes git state outside the local pool checkout: creating a branch on the remote, pushing, opening a PR, or any future automation that commits and pushes on behalf of a user. Current operations (`clone`, `sync/checkout`, `verify`) only read from GitHub and write inside the pool, so they are **not** write-class and stay allowed on every repo regardless of the flag. Local tracking branches made by `git worktree add --track -b` are local refs, not write-class.

```go
// internal/api/product_repos.go
type RepoOp string // RepoOpCreateBranch "create_branch" | RepoOpPush "push" | RepoOpOpenPR "open_pr"
var ErrWriteNotAllowed = errors.New("repo is read-only: enable allow_non_automation_work on the product")
var ErrRepoNotFound    = errors.New("repo not found")

// repoWriteAllowed loads the repo and the product flag itself, so callers cannot pass a stale or
// contradictory (repo, product) pair. One query:
//   SELECT r.is_main, COALESCE(s.allow_non_automation_work,0)
//   FROM product_repos r LEFT JOIN product_settings s ON s.product_id = r.product_id WHERE r.id = ?
func repoWriteAllowed(ctx context.Context, d *db.DB, repoID int, op RepoOp) error
// pure core, used by the table test:
func writeAllowed(isMain, flag bool, op RepoOp) error
```
Truth table: unknown `op` is denied; `isMain` allowed; `!isMain && flag` allowed; otherwise `ErrWriteNotAllowed`. No row means `ErrRepoNotFound` (deny). A missing settings row means flag false. The result is not cached. Handlers map `ErrWriteNotAllowed` to 403 with that message. A non-main central repo is a sibling.

**Real defence now (not only a contract).** `SyncBranches(allowPush)` sets, for every repo where `allowPush` is false (non-main repo and flag off), `git config remote.origin.pushurl DISABLED` (a relative path that is not a repository, so `git push origin` fails), and unsets it (`git config --unset remote.origin.pushurl`, exit code 5 for "not set" is ignored) when `allowPush` is true. `allowPush = repo.is_main || flag`. Git shares the clone's config with all its linked worktrees, so one setting covers every branch directory. The policy is re-applied on every sync, and `PUT /{id}` that changes the flag or a promotion that changes which repo is main triggers a best-effort `Pool.SetPushPolicy` for the affected repos after the DB commit (busy repos log and are fixed by the next sync).
**Bypass note.** This is a guard, not a sandbox: an explicit `git push <url>`, any process holding `GH_TOKEN` (the backend's `gh`), or anyone with a shell on rnd-v3 can still push. The control against that remains token scope (read-only, fine-grained token) and `REPO_POOL_ALLOWED_OWNERS`, both unchanged. Whether the executor process has a push-capable credential on rnd-v3 is **not verified here**.
Scope: no route performs a write-class op yet, so `repoWriteAllowed` has no production caller in this PR; any future endpoint that pushes, branches or opens a PR **must** call it first (stated as an invariant in `ai-backend/internal/api/README.md`).

### API
Everything is under `/api/products`; the router forwards the whole prefix (`router.go:652-653`) and needs no change. The path parser in `handleProducts` currently returns 404 when more than one segment follows the id (`len(parts) > 2`, `products.go:204`); it is extended to `{id}`, `{id}/{verify|clone|checkout}` and `{id}/repos[/{rid}[/{branches|verify|clone|checkout}]]`.

**Product JSON** (GET, and the body returned by every mutating route):
```json
{ "id": 1, "name": "HISv3", "staging_url": "https://...", "allow_non_automation_work": false,
  "repo_url": "...", "root_dir": "...", "branch": "main",          // legacy, mirrored from main
  "playwright_accessible": true, "last_verified_at": "2026-11-13T...Z", "verify_report": {...}, // main's
  "repos": [ <Repo>, ... ], "created_at": "...", "updated_at": "..." }
```
**Repo JSON**:
```json
{ "id": 7, "product_id": 1, "name": "his-v3-ai", "repo_url": "https://github.com/o/r", "root_dir": "o/r",
  "is_main": false, "is_central": true, "default_branch": "main", "branches": ["main", "dev"],
  "playwright_accessible": false, "last_verified_at": null, "verify_report": null,
  "sync_report": { "synced_at": "...Z", "results": [ {"name":"main","status":"ok","message":""} ] },
  "created_at": "...", "updated_at": "..." }
```
**Ordering.** `repos`: main first, then central (if not main), then by `name`. `branches`: default branch first, then alphabetical. `default_branch` is `""` when the list is empty (legacy).

| Route | Request body | Success response |
|---|---|---|
| `GET /api/products` | - | `{"products": [Product], "pool_enabled": bool}` |
| `POST /api/products` | `{"name","staging_url","repos":[{"name","repo_url","root_dir","is_main","is_central","branches":[...],"default_branch"}]}` with exactly one `is_main:true`. Legacy alternative: flat `{"name","staging_url","repo_url","root_dir","branch"}` (creates one repo named `main`, `is_main:true`; sending both forms is 400). `allow_non_automation_work` is not read here (created false; set it via PUT) | 201 Product |
| `GET /api/products/{id}` | - | 200 Product |
| `PUT /api/products/{id}` | `{"name"?, "staging_url"?, "allow_non_automation_work"?: bool}`; legacy repo fields ignored (200). The flag is a `*bool` in Go: omitted means no change, same value means no change, a different value by a role other than Admin/DevOps means 403 and **nothing** in the request is saved | 200 Product |
| `DELETE /api/products/{id}` | - | 204 (cascades repo, branch, settings rows; checkouts stay) |
| `POST /api/products/{id}/repos` | `{"name","repo_url","root_dir","is_main"?,"is_central"?,"branches"?:[...],"default_branch"?}`; `is_main:true` promotes atomically (old main becomes a sibling); `is_central:true` moves the central flag | 201 Product |
| `PUT /api/products/{id}/repos/{rid}` | partial: `{"name"?,"repo_url"?,"root_dir"?,"is_main"?,"is_central"?}`. `is_main:true` promotes; `is_main:false` on the current main is 409 (promote another first); `is_central:false` clears; branches are not editable here | 200 Product |
| `DELETE /api/products/{id}/repos/{rid}` | - | 200 Product (registry row only; deleting the central repo is allowed; the main repo is 409) |
| `PUT /api/products/{id}/repos/{rid}/branches` | `{"branches":["main","release/1.0"],"default_branch":"main"}` replaces the list and the default in one tx (clear old default flag first, then set new). Does not touch disk | 200 Product |
| `POST /api/products/{id}/repos/{rid}/verify` | none | 200 Product (that repo's report stored; main repo also mirrored) |
| `POST /api/products/{id}/repos/{rid}/clone` | none | 200 `{"product": Product, "results": [BranchResult]}` (clone, then sync) |
| `POST /api/products/{id}/repos/{rid}/checkout` | none (syncs every listed branch) | 200 `{"product": Product, "results": [BranchResult]}` |
| `POST /api/products/{id}/{verify|clone|checkout}` (legacy) | as above (`checkout` takes optional `{"branch"}`) | 200 Product (as today) |

**Errors.** Bodies are capped at 64 KiB (`MaxBytesReader`, as `products.go:171`); all errors use `writeJSONError`.

| Status | When |
|---|---|
| 400 | invalid JSON; `validateProduct`/`validateRepo` failures; empty branch list or missing/unlisted `default_branch` on a non-legacy PUT branches (a PUT that keeps an already-empty legacy list with empty default is allowed); more than 10 branches; duplicate branch; slug too long or colliding; both `repos` and flat fields on POST; no `is_main` or more than one in `repos`; `branch is required` on legacy checkout |
| 403 | role cannot manage / verify; flag changed by a role other than Admin/DevOps |
| 404 | product not found; `rid` not found **or not belonging to `{id}`** (`SELECT ... WHERE id=? AND product_id=?`, message "repo not found"); malformed path |
| 405 | method not allowed |
| 409 | `ErrBusy` ("another operation is already running for this repo"); duplicate key (messages below); `is_main:false` on the current main; delete of the main repo; root_dir overlap; repo URL repeated after `NormalizeRepo`; "product has no main repo" |
| 502 | git/gh failure (fetch, clone, checkout) |
| 503 | no DB; no repo pool (`REPO_POOL_DIR`) for pool routes |
| 500 | anything else (message not leaked: `log.Printf` then a generic text) |

`writeProductDBError` (`products.go:369-375`) is generalised into `writeDBConflict(w, err)`: it extracts the key name from MySQL error 1062 (`*mysql.MySQLError`, message `... for key 'table.index'`) and maps it:

| Key | 409 message |
|---|---|
| `uq_products_name` | a product with this name already exists |
| `uq_products_root_dir` | this root_dir is used by another product |
| `uq_product_repos_root_dir` | root_dir is already used by another repo |
| `uq_product_repos_name` | a repo with this name already exists in the product |
| `uq_product_repos_url` | this repository is already listed for the product |
| `uq_product_repos_main` | the product already has a main repo |
| `uq_product_repos_central` | the product already has a central repo |
| `uq_product_repo_branches` / `uq_product_repo_branch_slug` | duplicate or colliding branch name |
| `uq_product_repo_branch_default` | only one default branch is allowed |
| anything else | 500 generic |

Handlers: `validateProduct` is split into `validateProduct` (name, staging URL) and `validateRepo` (URL, root dir, name 1-100, branches via `repopool.ValidateBranch`, slug rules); `rootDirOverlap` is checked across all repos of all products (candidate list from `product_repos`, replacing `otherRootDirs`, `products.go:101-116`, and also checks each footprint `root_dir.branches`); `productRole` (`products.go:49-62`) gates every route and fails closed; a new `canToggleWork` (Admin, DevOps) gates the flag.

### Dashboard
`dashboard/src/views/ProductsView.tsx` (433 lines) has local `Product`/`ProductForm` types (`:18-37`), a local `api()` (`:47-57`; no shared copy exists in `dashboard/src`), and local `Modal` (`:403`) and `Field` (`:419`). **Component split:**
*   New `dashboard/src/views/products/api.ts`: moved `api()` plus the types `Product`, `Repo`, `BranchResult`, `VerifyReport`.
*   New `dashboard/src/components/Modal.tsx` and `Field.tsx`: moved verbatim from `ProductsView.tsx`; `ProductsView` imports them.
*   New `dashboard/src/views/products/ProductDetail.tsx` (the panel) and `RepoCard.tsx` (one repo: badges, branch editor, actions, sync results).
*   `ProductsView.tsx` keeps the list, add-product modal and role logic.
**State.** `ProductsView` holds `products` (with nested `repos`), `detailId: number | null`, and the existing `busy`/`error` state. `ProductDetail` takes the `Product` from the list and an `onChange(product)` callback that replaces it in the list from every response; it holds only local UI state: per-repo branch draft `{branches: string[], default: string}`, new-repo form, and a per-repo `lastResults` seeded from `repo.sync_report`. No big draft form: every action is its own call, so partial failures are visible.
**Call sequences.** Add product: `POST /api/products` with one main repo. Edit name/staging: `PUT /{id}`. Toggle flag (only Admin/DevOps see it enabled; others see it disabled with a tooltip): `PUT /{id} {allow_non_automation_work}` after a confirm showing the warning "Sibling repos become writable for branch creation and PRs. Automation scripts normally live only in the main repo." Add repo: `POST /repos`. Set main/central: `PUT /repos/{rid}` with `is_main`/`is_central`. Save branches: `PUT /repos/{rid}/branches`. Sync: `POST /repos/{rid}/checkout`. Clone: `POST /repos/{rid}/clone`. Verify: `POST /repos/{rid}/verify`. Each success calls `onChange` with the returned product.
**Errors and results.** A 4xx/5xx shows the server's `error` text in a banner at the top of the panel and leaves local drafts intact; 409 "busy" shows "Another operation is running for this repo; try again" and the buttons re-enable. Sync/clone results render as a list under the repo with status chips (ok green, kept_dirty amber, missing_remote and error red) and the message; after a reload the same list is rebuilt from `sync_report`. A proxy 504 on a long sync shows "Still running on the server; refresh in a minute" and reloads the product after a delay. A repo-less product shows the "No main repo: add one" banner and only the add-repo form.
**Switch-branch modal** (`:331-340`) is removed: branch editing and sync live in the per-repo branch list (a "Sync branches" button replaces "Switch branch"). The legacy `/checkout` route remains for old clients.
**Validation in the forms** mirrors the backend: one main, at most one central, 1 to 10 branches with exactly one default (not for legacy empty repos being left unchanged), no duplicate names or slug collisions, slug at most 100.
**README.** `dashboard/README.md` line 59 (route table row for `/products`) now says: master list with repos; product detail panel with repo badges (Main, Central), branch lists with default, the Allow non-automation work toggle (Admin/DevOps), per-repo Verify/Clone/Sync. Lines 88-89 (API table) list the new `/api/products/{id}/repos...` routes against `views/products/ProductDetail.tsx` and `RepoCard.tsx`, and state that the product views now share `views/products/api.ts`.

### Models
`ai-backend/internal/models/models.go` (`Product` at `:207-219`): `Product` gains `AllowNonAutomationWork bool` (from `product_settings`, missing row false) and `Repos []ProductRepo`. New `ProductRepo` (`ID, ProductID, Name, RepoURL, RootDir, IsMain, IsCentral, DefaultBranch, Branches []string, PlaywrightAccessible, LastVerifiedAt, VerifyReport, SyncReport, CreatedAt, UpdatedAt`) and `BranchResult`. Legacy `Product` fields stay in the JSON, populated from the main repo.

## Role matrix
| Action | Admin / DevOps | Developer | QA / QC | Other |
|---|---|---|---|---|
| Read products and repos | yes | yes | yes | yes (signed in) |
| Create, edit, delete products and repos; edit branches; clone; sync | yes | yes | no | no |
| Toggle `allow_non_automation_work` | yes | no (403) | no | no |
| Verify a repo | yes | yes | yes | no |

## Test harness
*   **Pure-Go tests** (no DB; run in CI): validators, slug rules and `ResolveWorktreeDir`, `writeAllowed` truth table, error-key mapping, `rootDirOverlap` with footprints, role gating, `childEnv`, JSON ordering, `repopool` tests against temp bare-origin repos (no MySQL).
*   **DB-gated tests** use a new helper `ai-backend/internal/testdb/testdb.go`: it reads `QA_TEST_MYSQL_DSN` and **skips** the test when it is empty. It creates a uniquely named scratch schema, runs `db.RunMigrations`, and drops the schema in cleanup; it refuses a DSN whose schema is `qa_automation` (never the shared schema). Tests that need it: unique-violation backstops (second main, second central, second default, root_dir, slug), the promotion tx, migration idempotency (run twice), the backfill (legacy rows, skipped conflicting rows, `NOT EXISTS` guards), the mirror/drift test, the `WithTx` deadlock retry, and cascade delete. CI without the variable runs everything else.
*   The previous idea of an `this-qa-automation-system-playwright` products spec is **dropped from scope**: that suite's API specs call endpoints without a session (for example `tests/api/cost.spec.ts`) and nothing in it provides login, so a products spec (products routes require auth) would need new auth plumbing. Not in this PR.

## Accepted limitations
*   **The Go guard has no production caller yet.** The disabled push URL is a guard, not a sandbox (bypass note above); token scope is the real control.
*   **Rollback window edits are lost** on roll-forward.
*   **Dirty worktrees and a dirty clone are kept, not cleaned.** Reported as `kept_dirty`.
*   **Non-fast-forward branches are skipped, not reset.**
*   **Disk growth.** Each branch adds a working tree; no quota.
*   **Edited `root_dir` orphans the old checkout** (and its worktrees) on disk.
*   **Root-owned checkouts, token breadth, untrusted code, long clones (300 s proxy), no audit trail**: unchanged from the earlier spec.
*   **Verify covers the clone only.**
*   **Distinct branch names whose slugs collide are rejected**, not renamed.
*   **`UNIQUE(product_id, repo_url)` is exact-string**; the normalised check is in handlers, so a concurrent pair of differently spelled URLs could both pass.

## Assumptions Goldfish reviewers must challenge
*   The DDL (flag-derived stored generated columns under `UNIQUE(product_id, flag)`, plus an FK `ON DELETE CASCADE` on `product_id`) is valid on the rnd-v2 MySQL version. **Unverified**; resolved only by the mandatory pre-implementation scratch run.
*   The `NOT REGEXP '(^|/)[^/]*\\.branches(/|$)'` expression in a Go raw string reaches MySQL with the intended escaping.
*   Taking `SELECT ... FROM products WHERE id=? FOR UPDATE` first in every mutating tx gives one global order and cannot deadlock with the verifier or the legacy routes.
*   `git worktree add --track -b`, `git worktree list --porcelain`, `git rev-parse --git-common-dir` and `git config remote.origin.pushurl` behave as described for a clone made by `gh repo clone`, running as root in the container; sharing config across worktrees is the default (no `extensions.worktreeConfig`). Not run here.
*   Treating `clone`, `sync` and `verify` as read-class matches the intent of "write operation (branch creation, PR)".
*   Mirroring the main repo into the legacy columns is enough for a safe rollback, and nothing else reads those columns (a grep of `ai-backend` found only `products.go` and the router; `playwright-executor` was not searched exhaustively).
*   The reserved `.branches` suffix, the two-part resolver and the `Lstat` check leave no path where a branch name or `root_dir` reaches outside the pool or into another repo's checkout.
*   A 10-branch cap, a 100-character slug cap and a 10-minute sync budget are adequate for HISv3-sized repos on rnd-v3.

## Target Files
*   Already present on the branch: `docs/designs/PRODUCT_REPOS.md`, and its row in `docs/designs/SITEMAP.md` (only the status flips later).
*   Create: `ai-backend/internal/api/product_repos.go` (+ `product_repos_test.go`: repo routes, `repoWriteAllowed`, `writeDBConflict`, `mirrorMainToLegacy`); `ai-backend/internal/services/repopool/integration_test.go` (temp bare-origin worktree tests, may be build-tagged if slow); `ai-backend/internal/testdb/testdb.go`; `dashboard/src/views/products/api.ts`, `ProductDetail.tsx`, `RepoCard.tsx`; `dashboard/src/components/Modal.tsx`, `Field.tsx`.
*   Modify: `ai-backend/internal/db/migrations.go` (entries 1 to 5), `ai-backend/internal/db/connection.go` (`WithTx`), `ai-backend/internal/models/models.go`, `ai-backend/internal/api/products.go` (path parser, create/update, verify store and iteration, legacy-route delegation), `ai-backend/internal/api/products_test.go`, `ai-backend/internal/services/repopool/repopool.go` (unlocked helpers, `SyncBranches`, `SetPushPolicy`, `ResolveWorktreeDir`, `.branches` ban, `childEnv`/`run` signatures, `VerifyRepo`/`ProbeStaging`), `ai-backend/internal/services/repopool/repopool_test.go` (including `TestChildEnvHasNoSecretsAndScopesSafeDirectory`), `dashboard/src/views/ProductsView.tsx`, READMEs: `ai-backend/internal/api/README.md` (route table lines 33-36, invariants), `ai-backend/internal/db/README.md` (new tables, pre/post-flight queries, deploy order), `ai-backend/internal/services/repopool/README.md`, `dashboard/README.md` (lines 59, 88, 89), `docs/designs/PRODUCTS_MASTER_AND_REPO_POOL.md` (add a "superseded in part by PRODUCT_REPOS.md" note), `docs/designs/SITEMAP.md` (status flip only).
*   Not changed: `router.go`, Dockerfile, compose files, `.env*`, executor, `ai-backend/README.md` (it does not list product routes: `grep products` found nothing).

## Verification Plan
1.  **Pre-implementation (blocking):** the scratch-MySQL DDL run described under Data model, with the recorded result added to this spec.
2.  `cd ai-backend && go build -mod=mod ./... && go test -mod=mod ./...` (pure-Go tests), then the same with `QA_TEST_MYSQL_DSN` pointing at a scratch server for the DB-gated tests.
3.  Migration: run twice against a scratch schema seeded with legacy products (including one with a `.branches` segment and one with empty `branch`); confirm one main row each, the skipped one reported by the post-flight query, no second-run changes, and that pre-flight queries return what the post-flight query leaves out.
4.  `repopool` tests with temp bare origins: add, fast-forward, remove, keep dirty, default swap (including dirty swap), `a/b` vs `a--b`, existing path that is not the expected worktree, missing remote branch, diverged local branch, `<root_dir>.branches` as a symlink to outside the pool, a symlink at the slug directory, branch `-x` and `a/../b`, push policy set/unset (a `git push` to a temp origin fails while disabled).
5.  Concurrency: two parallel promotions and a verify on one product leave exactly one main and a consistent mirror (DB-gated).
6.  `cd dashboard && npm run build && npx tsc --noEmit`.
7.  Manual on a scratch stack: HISv3 shape (central `his-v3-ai`, main automation repo, two siblings with several branches): sync all branches, flag off shows a disabled push URL and the guard denies, flag on enables, invariants hold from the UI; Admin/DevOps vs Developer toggle.
8.  Rollback drill on a scratch stack: deploy new, edit a product, redeploy the old binary, confirm the old UI shows the main repo coherently; roll forward and rerun the migration.
9.  After the spec PR merges later: `git worktree list` and `git branch -a` show no leftovers. No deploy without a separate approval (`deploy-his-v3-qa` skill).
