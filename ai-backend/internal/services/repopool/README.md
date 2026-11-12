# services/repopool

## Overview

Manages product repositories checked out under one **pool directory** (`REPO_POOL_DIR`, `/data/repos` in the backend container, `<repo>/repos` on rnd-v3). It validates inputs, clones with `gh`, switches branches with `git`, and verifies that a checkout can be used by Playwright. It is the only code in ai-backend that runs `gh` or `git`. `internal/api/products.go` is its caller.

## Interface

- `New(root) *Pool`: returns nil for an empty root (feature disabled).
- `Validate*`: `ValidateRepoURL` (`https://github.com/<owner>/<repo>[.git]` only), `ValidateRelDir` (relative, safe segments, no `..`), `ValidateBranch` (conservative ref name, never starts with `-`).
- `(*Pool).ResolveDir(rel)`: absolute path guaranteed inside the root, including through symlinks.
- `(*Pool).Clone(ctx, repoURL, rel)`: `gh repo clone`, 5 min timeout. Fails if the target is not empty.
- `(*Pool).Checkout(ctx, rel, branch)`: `git fetch --prune`, `git checkout`, `git merge --ff-only origin/<branch>`. Returns the current branch.
- `(*Pool).Verify(ctx, repoURL, rel, stagingURL) Report`: checks `root_dir`, `git_repo`, `origin_matches`, `playwright` (config file or dependency, in the root or one level down) and `staging_url` (GET, 10 s, any status below 500). It never returns an error: failures are in the report.
- `ErrBusy`: another clone/checkout is running for the same directory.

## Invariants

- Commands use argument slices, never a shell. Output is truncated to 2 KB.
- Every path is resolved inside the pool root before use.
- At most one clone/checkout per directory at a time (`TryLock`).
- `GH_TOKEN` is passed through the process environment (`gh auth git-credential` helper for `github.com`) and is never logged or returned.

## Side Effects

Writes under the pool root, runs `gh`/`git` subprocesses, makes one outbound GET during `Verify`.

## Verification

`go test -mod=mod ./internal/services/repopool/`. The integration test does a real clone and branch switch; enable it with `GH_TOKEN` and `REPOPOOL_IT_REPO=https://github.com/<owner>/<repo>`.
