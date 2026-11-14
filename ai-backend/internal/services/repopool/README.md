# services/repopool

## Overview

Manages product repositories checked out under one **pool directory** (`REPO_POOL_DIR`, `/data/repos` in the backend container, `<repo>/repos` on app-host). It validates inputs, clones with `gh`, switches branches with `git`, and verifies that a checkout can be used by Playwright. It is the only code in ai-backend that runs `gh` or `git`. `internal/api/products.go` and `product_repos.go` are its callers.

## Interface

- `New(root) *Pool`: returns nil for an empty root (feature disabled).
- `Validate*`: `ValidateRepoURL` (`https://github.com/<owner>/<repo>[.git]` only), `ValidateRelDir` (relative, safe segments, no `..`, no segment ending in `.branches`, which is reserved for worktrees), `ValidateBranch` (conservative ref name, never starts with `-`).
- `(*Pool).ResolveDir(rel)`: absolute path guaranteed inside the root, including through symlinks.
- `(*Pool).Clone(ctx, repoURL, rel)`: `gh repo clone`, 5 min timeout. Fails if the target is not empty.
- `(*Pool).Checkout(ctx, rel, branch)`: `git fetch --prune`, `git checkout`, `git merge --ff-only origin/<branch>`. Returns the current branch.
- `(*Pool).Verify(ctx, repoURL, rel, stagingURL) Report`: checks `root_dir`, `git_repo`, `origin_matches`, `playwright` (config file or dependency, in the root or one level down) and `staging_url` (GET, 10 s, any status below 500). It never returns an error: failures are in the report.
- `(*Pool).VerifyRepo(ctx, repoURL, rel, requirePlaywright) Report` and `(*Pool).ProbeStaging(ctx, url) Check`: the per-repo checks and the staging probe, split so the probe runs once per product. `Verify` is `VerifyRepo(true)` plus the probe. `Report.Append` adds a check.
- `(*Pool).ResolveWorktreeDir(rootDir, slug)`: `<pool>/<rootDir>.branches/<slug>`, confined to the pool; an existing `.branches` or slug directory must be a real directory (no symlink). `Slug(branch)` maps `/` to `--`; `ValidateSlug` caps it at 100 characters.
- `(*Pool).SyncBranches(ctx, repoURL, rel, []BranchSpec, allowPush) ([]models.BranchResult, error)`: clone if absent, apply the push policy, `git worktree prune`, fetch with an explicit refspec, then default branch in the clone and every other listed branch in a worktree; removes worktrees of unlisted branches. Never resets, cleans or forces: dirty trees are `kept_dirty`, diverged branches `error`, absent remote branches `missing_remote`. An empty list stops after the fetch. The error return is only for whole-sync failures (input, `ErrBusy`, clone, fetch).
- `(*Pool).SetPushPolicy(ctx, rel, allowPush)`: sets `remote.origin.pushurl=DISABLED` (shared by all worktrees) or unsets it. A guard, not a sandbox: an explicit `git push <url>` or any holder of `GH_TOKEN` can still push.
- `(*Pool).ReadTree(ctx, repoID, repoURL, branch, keep, maxBytes) (sha, files, error)` (keep filters paths; nil keeps all) (`mirror.go`, PRODUCT_KNOWLEDGE_SYNC.md): fetches one branch (`--depth=1`, explicit refspec, URL from `repoURL`, never a clone's remote) into the sync-owned bare mirror `<pool>/.kb-mirrors/<repoID>.git` and returns the commit SHA and the raw bytes of every regular file under `prefix`, read with `ls-tree -z` + `cat-file --batch` (no filters, attributes or LFS). Every git call uses `--git-dir=<mirror>`, `GIT_CEILING_DIRECTORIES=<pool>`, hooks/gc/maintenance off. A mirror without `HEAD` is re-initialised; more than 20 packs trigger `gc --prune=now`. Lock key is the mirror path, so it never contends with clone/sync/verify.
- `(*Pool).SetOwner("uid:gid")` / `FixOwner(path)` (`owner.go`, `REPO_POOL_OWNER`): the backend container runs as root but playwright-executor is a host user that must write `node_modules` and reports into clones and worktrees. When set, clone, `AddWorktree` and `SyncBranches` hand the created tree to that owner (best effort, never follows symlinks, never leaves the pool root).
- `ErrBusy`: another operation is running for the same repo directory.

## Invariants

- Commands use argument slices, never a shell. Children get a minimal environment (`PATH`, `HOME`, `GH_TOKEN`, git config); `safe.directory` is scoped to the one checkout. A timeout kills the whole process group (`gh` spawns `git`). Output is truncated to 2 KB and the API returns only its first line.
- Public operations take one non-reentrant lock per clone directory and call unexported `*Locked` helpers (`cloneLocked`, `checkoutLocked`, `verifyRepoLocked`, `setPushPolicyLocked`); `SyncBranches` takes it once. `VerifyRepo`/`Verify` take the same lock; if a clone/checkout holds it, the report has `Busy` and must not be stored.
- A failed clone removes its partial target. `REPO_POOL_ALLOWED_OWNERS` (`Pool.AllowedOwners`) limits which GitHub owners can be cloned. Staging probes refuse loopback/link-local/unspecified/multicast addresses at dial time.
- Every path is resolved inside the pool root before use.
- `ReadTree` never touches a product clone, its worktrees, `.git` or remote config; `.kb-mirrors/` can be deleted at any time (the next sync recreates it). `relDirRe` forbids a leading `.` in every segment, so no `root_dir` can claim it. `runBytes` returns raw stdout (capped) and strips URL userinfo from error text.
- At most one clone/checkout per directory at a time (`TryLock`).
- **Automation worktrees** (`worktree.go`, CASE_AUTOMATION.md): `FetchOrigin` (fetch only; `SyncBranches` is not used because it removes worktrees it does not list), `AddWorktree` (branch must start with `qa-auto/`, from `origin/<base>`, replaces a leftover worktree and local branch of the same name), `OpenWorktree`, `LinkNodeModules` (relative symlink to the clone's `node_modules`), `HostPath` (maps a pool path to the executor's view), `DiffStat`, `Commit` (hooks off, fixed author, only the given repo-relative paths), `Push` (explicit non-forced refspec of one `qa-auto/` branch) and `OpenPR` (`gh pr create`, URL parsed from its output). The directory lock is taken with a wait (up to 2 min) instead of failing with `ErrBusy`. **`Push` and `OpenPR` do not check the write guard: the caller must pass `repoWriteAllowed(RepoOpPush / RepoOpOpenPR)` first** (`internal/api/case_automation.go:poolRepo`). A later `SyncBranches` removes a clean automation worktree it does not know; a worktree that holds the generated spec is dirty until its commit, so it is kept.
- `GH_TOKEN` is passed through the process environment (`gh auth git-credential` helper for `github.com`) and is never logged or returned.

## Side Effects

Writes under the pool root, runs `gh`/`git` subprocesses, makes one outbound GET during `Verify`.

## Verification

`go test -mod=mod ./internal/services/repopool/`. Worktree tests run against temporary bare origins (need only `git`). A separate test does a real clone and branch switch; enable it with `GH_TOKEN` and `REPOPOOL_IT_REPO=https://github.com/<owner>/<repo>`.
