# Products master list and repo pool

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Implemented on the owner's decisions of 2026-11-13 (see Decisions). Goldfish review run on 2026-11-13 (design + codebase-details reviewers); fixes are in PR #4 (see Review outcome).
*   **Date:** 2026-11-13

## Overview
A "Products" master list in the dashboard. Each product records its repo link, main (staging) URL, whether Playwright can reach it, and the root dir of its checkout on app-host. A **Verify** button checks all of that. A repo pool dir in this repo lets the system `gh repo clone` repos and switch branches.

## Problem
No place defines which products exist, where their repos live, or whether the app-host VM can run Playwright against them. `playwright-executor` (merged, PR #3) targets one env-configured staging URL (`TARGET_STAGING_URL`); there is no per-product registry.

## Goals
*   `products` table + CRUD API + dashboard page.
*   Verify: root dir exists and is a git repo whose `origin` matches the repo link; staging URL reachable (non-5xx); Playwright present (config or package in root dir).
*   Clone and branch switch from the UI via `gh` / `git`, confined to a pool dir.

## Non-Goals
*   Making the executor consume per-product URLs (follow-up; today it uses `TARGET_STAGING_URL`).
*   Running tests in the cloned repos.

## Technical Plan
*   **Pool dir:** `repos/` in this repo, containing only `.gitkeep`; `.gitignore` has `repos/*` and `!repos/.gitkeep`. `root_dir` is stored relative to it.
*   **Mount:** backend runs in Docker; playwright-executor runs on the host. Bind-mount host `<deploy checkout>/repos` into the backend container **read-write** at `/data/repos` (only that dir). Clone and checkout need write access; a read-only mount would allow Verify only. The host path is the same one the executor reads, so "accessible by Playwright" is true when the host dir exists.
*   **Migration:** append `products(id, name UNIQUE, repo_url, staging_url, root_dir, branch, playwright_accessible, last_verified_at, verify_report JSON, created_at, updated_at)`.
*   **API:** `/api/products` (GET, POST), `/api/products/{id}` (PUT, DELETE), `POST /api/products/{id}/verify|clone|checkout`.
*   **Safety:** `repo_url` must match `^https://github\.com/[\w.-]+/[\w.-]+(\.git)?$`; branch matches a strict ref regex and never starts with `-`; `exec.Command` with an arg slice (no shell); resolved path must stay inside the pool (reject `..`, symlink escape); per-product lock; timeouts (clone 5 min, verify 15 s).
*   **gh auth:** `gh` and `git` added to the backend image; `GH_TOKEN` supplied via `.env.prod` (key name only in `.env.prod.example`). Token scope: read access to the product repos.
*   **UI:** `ProductsView` (table, add/edit modal, status chip, Verify/Clone/Switch branch, all via `SubmitButton`).

## Decisions (owner, 2026-11-13)
1.  GitHub token: taken from the owner's `~/.config/github/token.txt` locally (checked valid with `gh`; value never printed). Production needs `GH_TOKEN` set in `.env.prod` on app-host, which is a separate step.
2.  The pool mount is read-write, for this one directory only.
3.  Verify runs both on demand and on a schedule (`PRODUCTS_VERIFY_INTERVAL_MINUTES`, default 360).

## Writes by role
Admin, DevOps and Developer may create, edit, delete, clone and switch branch. QA and QC may also Verify. Everyone signed in can read.

## Former open questions
1.  Is there a GitHub token with read access to the product repos, and where is it stored?
2.  Read-write pool mount (enables clone/branch) vs read-only (verify only, clone by hand)?
3.  Should Verify also run on a schedule, or only on demand?

## Assumptions Goldfish reviewers must challenge
*   A writable bind mount plus `gh` in the backend container is acceptable security-wise (backend can now run git/gh and write to one dir).
*   `staging_url` reachability from the backend container equals reachability from the executor on the host.
*   Storing `root_dir` relative to the pool is sufficient (no products outside it).

## Target Files
*   Create: `repos/.gitkeep`, `ai-backend/internal/api/products.go` (+ test), `ai-backend/internal/services/repopool/` (+ test), `dashboard/src/views/ProductsView.tsx`
*   Modify: `ai-backend/internal/db/migrations.go`, `ai-backend/internal/models/models.go`, `ai-backend/internal/api/router.go`, `ai-backend/Dockerfile`, `deploy/qa-tool/docker-compose*.yml`, `deploy/qa-tool/.env.prod.example`, `.gitignore`, `dashboard/src/App.tsx`, `dashboard/src/components/Layout.tsx`, READMEs of touched packages, `docs/designs/SITEMAP.md`

## Verification Plan
Go table tests for URL/branch/path validation and verify checks (temp git dirs); `go test -mod=mod ./...`; dashboard build + tsc; manual verify against a real repo and a bogus path.


## Review outcome (2026-11-13)

Fixed in PR #4: role check now fails closed; children of gh/git get a minimal environment with `safe.directory` scoped to the checkout (was `*` with the full backend env); timeout kills the process group; failed clone cleans up; clone/checkout run detached from the request; verify takes the directory lock and its stored result is conditional on unchanged inputs; overlapping `root_dir` rejected; staging probe refuses loopback/link-local/metadata addresses, credentials in URLs, and returns coarse errors; origin shown without userinfo; `REPO_POOL_ALLOWED_OWNERS` allowlist; stricter repo URL/branch rules; Playwright detection parses dependencies and ignores symlinks; role-aware buttons in the UI.

Accepted limitations (follow-ups):
*   **Root-owned checkouts.** The container runs as root, so cloned files are root-owned on the host; the host executor can read but not modify them. Running the backend as a non-root uid matching the host user is the fix.
*   **Token breadth.** The token in use is the owner's personal token. Use a fine-grained token limited to the product repos and set `REPO_POOL_ALLOWED_OWNERS`.
*   **Reachability is from the backend container,** not the host executor; `playwright_accessible` is a heuristic. A probe performed by the executor would be exact.
*   **Untrusted code.** Cloned repos and any branch of them become code the executor may run; only trusted owners should be allowed.
*   **Long clones.** A clone plus checkout can exceed the 300 s proxy timeout; the operation still completes server-side and the page can be refreshed.
*   **No audit trail** of who cloned or changed what, and the lock map is unbounded (small).
*   Branch edits do not switch the checkout; use Switch branch.
