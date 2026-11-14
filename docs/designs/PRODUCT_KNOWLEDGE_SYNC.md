# Product knowledge base: sync from the automation repo

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Approved rev 3 (2026-11-13). Rev 1 was reviewed by two Goldfish reviewers (design/readiness and codebase accuracy). Rev 2 was reviewed for implementation readiness and had no blockers. Every finding is resolved, as the "Review resolution" tables show. The owner pre-authorised implementation, merge and deploy.
*   **Date:** 2026-11-13
*   **Depends on:** [PRODUCT_REPOS.md](PRODUCT_REPOS.md) and [PRODUCT_REPOS_AUTOMATION_ROLE.md](PRODUCT_REPOS_AUTOMATION_ROLE.md), both implemented (`main` at `449ae18`).
*   **Code references** use function names. Line numbers are approximate.

## Overview
A product's **automation repo** (`product_repos.is_automation = 1`) can hold a structured catalog of the product. SUT's automation repo, `example-org/automation-test-sut`, keeps one under `docs/`. The table below shows what it contains; the counts are taken at commit `521618a`.

| Entity | Where it is defined | Example | SUT count |
|---|---|---|---|
| Part | `docs/modules/[A-D]-<name>.md`, one top-level file per part | `A` | 4 |
| Module | a `### A01 — Name` heading inside a part file | `A01` | 39 |
| Sub-module | the `Sub-module` text cell of a feature row (it has no id) | `Master Pasien` | about 200 |
| Feature | a table row with 12 cells under a module heading | `A01.07` | 444 |
| Epic | every `.md` file under `docs/test-catalog/<service>/` (any depth; `_*` and `README.md` are excluded) | `OUTREG` | 46 |
| User story | `- **US-OUTREG-05** ([A01.07](…), …) — text` or `(unmapped: why)` | `US-OUTREG-05` | 246 |
| Test case (the scenario is inline) | a table row with 10 cells that starts with `\| ☐ \|` or `\| ☑ \|` | `TC-OUTREG-001` | 1659 (456 automated) |

The format is defined and checked by that repo's `scripts/test-catalog.mjs`. This spec adds four things:
*   a **Sync** action that imports the catalog into `qa_automation` as the product's knowledge base;
*   a schedule that re-syncs when the branch head moves;
*   read APIs and dashboard pages to browse it;
*   local annotations and local draft records.

The sync is a deterministic Go parser. claude-v3, reached through ai-manager, only writes an optional summary of the changes.

## Problem
*   The qa tool doesn't know what a product contains (its modules and features) or how well it is covered (its stories and test cases).
*   The catalog changes every day in git, so a copy made by hand goes stale.

## Goals
1.  On demand (a button) and on a schedule, read the automation repo's catalog at the head of its default branch. Store it as the current knowledge base, together with the commit SHA it came from.
2.  **Idempotent.** A second sync of the same commit and parser version writes no entity rows and reports an empty diff. Entities that disappear are soft-deleted. Entities that come back are restored.
3.  Every sync stores its diff and its parse problems. The AI writes a natural-language summary of the diff. If the AI step fails, the sync still counts as succeeded.
4.  Pages to browse the Part → Module → Sub-module → Feature tree, the stories and the test cases. Every entity shows its trace chain: feature, then stories, then test cases, then spec paths.
5.  Local notes and tags on any entity, and they survive re-syncs. Local draft stories and test cases, clearly marked, that can never collide with synced ids.
6.  The format is pluggable per product (`knowledge_format`). The SUT format is the first one shipped, and the only one for now.

## Non-Goals
*   **Writing back to the automation repo.** Synced records are read-only in the app. The sync never commits or pushes. It never touches the product's clone, its worktrees, its `.git` or its remote configuration (see "Reading the repo").
*   **AI extraction of entities.** It is not repeatable. Each new product format gets its own Go adapter.
*   **Reading the `SC-*` and long-form `US-*` source documents in `sut-ai`.**
*   **Linking test cases to executor runs.** That is future work. `spec_paths` makes it possible later.

## Decisions (user, 2026-11-13)
1.  A deterministic code parser does the sync. The AI only writes the change summary.
2.  Synced rows are read-only. Local annotations and local drafts are allowed.
3.  The scheduled sync runs every 6 h and only when something changed.
4.  The work follows the full EGM flow. The user pre-authorised the PRs, the merges, the migration and the deploy for this feature.

## Design

### Reading the repo: a bare mirror that only the sync uses
The product's clone in the pool is shared with AI agents and humans, so the sync never runs git inside it. Concurrent fetches there would hit "cannot lock ref", and `git archive` would run filters and attributes from its config. The sync keeps its own **bare mirror** instead, one per repo:

*   **Location:** `<pool root>/.kb-mirrors/<repo_id>.git`, resolved through `resolveValidated` (the symlink guard). `relDirRe` already requires every segment to start with a letter or digit, so no product `root_dir` can claim a path that starts with `.`.
*   **Every git command** runs as `git --git-dir=<mirrorAbs> …` with its working directory set to `mirrorAbs`, `GIT_CEILING_DIRECTORIES=<pool root>`, and `safe.directory=<mirrorAbs>` through `childEnv`. A half-deleted mirror then fails; it never falls through to another repo. The minimum git version is 2.29, and the backend image's Alpine git is newer.
*   **New method:** `func (p *Pool) ReadTree(ctx context.Context, repoID int, repoURL, branch, prefix string, maxBytes int64) (sha string, files map[string][]byte, err error)` in `repopool.go`. It runs these steps:
    1.  Validate `repoURL` (`ValidateRepoURL`) and check `OwnerAllowed`. Validate `branch` (`ValidateBranch`). `prefix` must match `^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$` with no `..` segment.
    2.  Take `p.lock(mirrorAbs)`. This key is never used by clone, sync or verify, so it cannot contend with them. `ErrBusy` can only come from another sync of the same repo, and the running flag already prevents that.
    3.  If `<mirrorAbs>/HEAD` is missing, remove `mirrorAbs` and run `git init --bare -q <mirrorAbs>`.
    4.  `git -c core.hooksPath=/dev/null -c gc.auto=0 -c maintenance.auto=false -c fetch.writeFetchHEAD=false fetch --no-tags --depth=1 <https URL derived from NormalizeRepo(repoURL)> +refs/heads/<branch>:refs/heads/<branch>`, with a 120 s timeout. The URL comes from `product_repos.repo_url`, not from any remote configured in a clone. Credentials come from `childEnv` (the gh credential helper) as they do for clone.
    5.  `git rev-parse --verify refs/heads/<branch>^{commit}` gives `sha`, which must be 40 hex characters.
    6.  `git ls-tree -r -z --full-tree <sha> -- <prefix>`. Keep only the `100644` and `100755` blob entries.
    7.  `git cat-file --batch` with one `<blob-sha>` per line on stdin. The raw object bytes are read from the batch framing (`<sha> blob <size>\n<bytes>\n`). Reading raw objects means no filters, no smudge or LFS, and no attributes. The total is capped at `maxBytes`, with an error past it.
    8.  Housekeeping: when `objects/pack` holds more than 20 `.pack` files, run `git --git-dir=… -c core.hooksPath=/dev/null gc --prune=now --quiet` (60 s). A failure here is only logged.
*   **The `run()` helper truncates output at 2000 characters.** A new helper `runBytes(ctx, dir string, safeDirs []string, extraEnv []string, stdin io.Reader, timeout time.Duration, maxBytes int64, name string, args ...string) ([]byte, error)` returns stdout as raw bytes. It uses the same `childEnv`, process-group kill and `WaitDelay`. Stderr is captured separately, truncated, and used only in the error text.
*   **Mirrors are owned by the sync.** Deleting a mirror is always safe, because the next sync recreates it.

The sync calls `ReadTree` with `prefix = "docs"` and `maxBytes = 32 MiB`. SUT's `docs/` holds about 1 MB of `.md` files plus one CSV.

### Format adapters: package `ai-backend/internal/services/knowledge`
*   **`catalog.go`**:
    *   `type Format interface { Name() string; Version() int; Parse(files map[string][]byte) (*Snapshot, []Problem, error) }`.
    *   `var formats = map[string]Format{…}`, with `Lookup(name)` and `Names()`.
    *   The `Snapshot` and `Problem{File, Line, Msg}` types, plus `Enums()` (see below).
*   **`sut.go`** is named `test-catalog-v1` and has `Version() = 1`. Raise the version on any change that alters parse output. It ports the parse half of `scripts/test-catalog.mjs` at `521618a`. Where that script uses `Map.set`, duplicates resolve as **last wins**, which matches the script, and every duplicate is also recorded as a Problem. Lines are split on `\n`, and a trailing `\r` is stripped from each line.
    *   **Parts and modules.**
        *   Files: `^docs/modules/[A-D]-[^/]+\.md$`, top level only.
        *   Part: `{code: first letter, name: text of the first "# " line, or the base name if there is none}`.
        *   Module: a line that matches `^### ([A-D]\d{2}) — (.+)$`.
    *   **Features.**
        *   Candidate rows are lines that start with `|`. Split on `|`, drop the first and last piece, and trim each cell.
        *   Cell 0 must match `^(?:<a id="[^"]*"></a>)?([A-D]\d{2}\.\d{2,3}(?:\.\d{2})?)$`.
        *   Any count other than 12 cells is a Problem, and the row is skipped.
        *   Cell mapping:

            | Cell | Field | Notes |
            |---|---|---|
            | 1 | sub-module name | |
            | 2 | name | |
            | 3 | repo entry | |
            | 4 | release status | the backticked token, one of `PROD DEV-DONE IN-PROGRESS NOT-STARTED UNKNOWN`; anything else is a Problem and is stored as `UNKNOWN` |
            | 5 | status_as_of | the raw text, with `—` stored as empty |
            | 6 | evidence | |
            | 7 | notes | |
            | 8–11 | — | generated by the script, so ignored |
        *   A feature whose id does not start with `<current module>.` is a Problem. It is **still stored**, as mjs does. Its `module_code` is the id prefix (`A01.07` → `A01`).
        *   A duplicate feature id is a Problem, and the last one wins.
    *   **Sub-modules.**
        *   Key: `(module_code, lower(trim(name)))`. An empty cell means no sub-module.
        *   Code: `<module>~<slug>-<hash>`. The slug is the lowercased name with every run of characters outside `[a-z0-9]` collapsed to `-` and trimmed, then cut to 33 characters. The hash is the first 6 hex characters of the sha1 of the lowercased name.
        *   The code is always at most 44 characters and contains no `/`.
        *   The sub-module's name comes from its first occurrence.
    *   **Epics.**
        *   Files: every path under `docs/test-catalog/` that ends in `.md`, where the base name does not start with `_` and the path is not `docs/test-catalog/README.md`.
        *   A file directly in `docs/test-catalog/` is a Problem, and it is still parsed.
        *   Service: the first path segment. Area: any middle segments, joined with `/`.
        *   Title: the first `# ` line, or the file name if there is none.
        *   Code: the code part of the file's first story id (`US-OUTREG-01` → `OUTREG`). With no stories, the code is the path relative to `docs/test-catalog/` without `.md`, run through the slug rule, uppercased and cut to 64 characters. This matches mjs's path fallback.
        *   A duplicate epic code from two files is a Problem, and the first file wins. The second file's stories and cases still point at that code.
    *   **Stories.**
        *   Lines matching `^- \*\*(US-[A-Z]+-\d{2})\*\*(.*?)\s+—\s+(.*)$`.
        *   If the middle part matches `\(unmapped:\s*([^)]+)\)`, the captured text becomes `unmapped_reason` and there are no features. Otherwise the features are the distinct matches of `[A-D]\d{2}\.\d{2,3}(?:\.\d{2})?` in the middle part.
        *   A story with neither is a Problem, and it is still stored.
        *   A duplicate story id is a Problem, and the last one wins.
        *   A feature id that isn't in the snapshot is a Problem, and that one link is dropped.
        *   The story's epic is the code of its id.
    *   **Test cases.**
        *   Lines matching `^\|\s*[☐☑]\s*\|`. The cells are `[box, id, story, scenario, polarity, level, priority, status, spec, reason]`. A row without exactly 10 cells is a Problem and is skipped.
        *   The id must match `^TC-[A-Z]+-\d{3}$`. Otherwise it is a Problem and the row is skipped.
        *   An unknown `status` is a Problem and the row is **skipped**, as in mjs.
        *   A duplicate id is a Problem, and the last one wins.
        *   If the story doesn't match `^US-[A-Z]+-\d{2}$`, or isn't a known story, it is a Problem. The case is still stored, with `story_code = NULL`.
        *   An unknown polarity, level or priority is a Problem. The raw value is stored, truncated to the column size.
        *   `automated` is true when `status == "automated"`. A checkbox that disagrees is a Problem.
        *   `spec_paths` is every backticked substring of the spec cell.
        *   `reason` is split at the first `:` into `reason_category` and a trimmed `reason_detail`. A category outside the reason enum is a Problem, and it is still stored.
        *   The epic is the containing file's epic code.
        *   `scenario` is the raw cell text, with its markdown kept.
    *   **Not ported:** the check that spec files exist, which needs a working tree.
    *   `Parse` returns an error only when it finds zero features **and** zero test cases. That means the wrong format or a wrong path. The sync then fails instead of soft-deleting everything.
*   **Enums**, exported by `Enums()` and also served by the API:

    | Enum | Values |
    |---|---|
    | release | `PROD DEV-DONE IN-PROGRESS NOT-STARTED UNKNOWN` |
    | polarity | `+ -` |
    | level | `API GQL WEB E2E` |
    | priority | `P0 P1 P2 P3` |
    | status | `planned automated blocked manual-only wont` |
    | reason category | `defect not-needed blocked unsafe manual-only deferred other` |

    Statuses `blocked`, `manual-only` and `wont` need a reason. An `automated` case needs at least one spec path. These two rules are enforced for local drafts (400) and are only Problems for synced rows.
*   **Tests:**
    *   `sut_test.go` has a table-driven test for every rule above, including CRLF input and each duplicate and last-wins case.
    *   It also has a **golden test** over `testdata/sut-521618a/` (the `.md` files of `docs/` copied verbatim). It asserts 4 parts, 39 modules, 444 features, 46 epics, 246 stories (9 of them unmapped), and 1659 test cases (456 automated). The sub-module count is recorded at copy time.
    *   `testdata/README.md` records the source SHA and how to refresh the copy.

### Schema (appended to `ai-backend/internal/db/migrations.go`)
*   All tables are InnoDB, `utf8mb4_general_ci`. **Every column that holds a code** (`code`, `module_code`, `submodule_code`, `epic_code`, `story_code`, `feature_code`, `entity_code`) is `VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`.
*   JSON columns have no literal default. They are `NULL` or always written by the code.
*   User references are `CHAR(36) NULL`, because `users.id` is `CHAR(36)`. They have no FK, so the history stays if a user is deleted.

**`kb_syncs`**: one row per sync attempt.
*   Identity and source:
    *   `id INT AUTO_INCREMENT`, `product_id INT NOT NULL` (FK `products` ON DELETE CASCADE);
    *   `repo_id INT NULL` (no FK, so history survives repo deletion), `repo_name VARCHAR(100)`, `repo_url VARCHAR(300)`, `branch VARCHAR(200)`;
    *   `format VARCHAR(50)`, `parser_version INT NOT NULL DEFAULT 0`, `commit_sha CHAR(40) NULL`.
*   Run state:
    *   `status ENUM('running','succeeded','failed','skipped') NOT NULL`;
    *   `trigger_kind ENUM('manual','schedule') NOT NULL`, `triggered_by CHAR(36) NULL`;
    *   `started_at DATETIME NOT NULL`, `finished_at DATETIME NULL`.
*   Results: `counts JSON NULL`, `diff JSON NULL`, `problems JSON NULL`.
*   AI summary: `ai_status ENUM('none','pending','done','failed') NOT NULL DEFAULT 'none'`, `ai_summary MEDIUMTEXT NULL`, `ai_error VARCHAR(500) NULL`.
*   `error VARCHAR(1000) NULL`.
*   One running sync per product: `running_flag TINYINT GENERATED ALWAYS AS (IF(status='running',1,NULL)) STORED`, with `UNIQUE KEY uq_kb_syncs_running (product_id, running_flag)`. It is generated from `status`, not from the FK column, so MySQL error 3104 does not apply.
*   `INDEX idx_kb_syncs_product (product_id, id)`.

**Entity tables.** Every entity table has these common columns:
*   `id INT AUTO_INCREMENT`, `product_id INT NOT NULL` (FK products CASCADE), `code` with `UNIQUE KEY (product_id, code)`;
*   `origin ENUM('sync','local') NOT NULL DEFAULT 'sync'`;
*   `content_hash CHAR(64) NOT NULL`, the sha256 of the canonical JSON of the parsed fields **without** `source_file` and `source_line`. Local rows hash their own fields the same way.
*   `source_file VARCHAR(300) NULL`, and `source_line INT NULL` where the entity has a line. Both are NULL for local rows. When only the location changed, the sync updates these two columns quietly: no diff entry, and `changed_sync_id` stays as it was.
*   `first_sync_id INT NULL`, `changed_sync_id INT NULL` (the last sync that inserted, changed or restored the row; unchanged rows are not written);
*   `removed_at DATETIME NULL`, `removed_sync_id INT NULL`;
*   `created_by CHAR(36) NULL` (local rows only), `created_at`, `updated_at`.

| Table | Specific columns |
|---|---|
| `kb_parts` | `name VARCHAR(300)` |
| `kb_modules` | `part_code`, `name VARCHAR(300)` |
| `kb_submodules` | `module_code`, `name VARCHAR(300)` |
| `kb_features` | `module_code`, `submodule_code NULL`, `name TEXT`, `repo_entry VARCHAR(300)`, `release_status VARCHAR(20)`, `status_as_of VARCHAR(20)`, `evidence VARCHAR(500)`, `notes TEXT`, `INDEX (product_id, module_code)` |
| `kb_epics` | `title VARCHAR(500)`, `service VARCHAR(100)`, `area VARCHAR(200)` |
| `kb_stories` | `epic_code`, `text TEXT`, `unmapped_reason VARCHAR(500) NULL`, `INDEX (product_id, epic_code)` |
| `kb_test_cases` | `epic_code`, `story_code NULL`, `scenario TEXT`, `polarity VARCHAR(4)`, `level VARCHAR(8)`, `priority VARCHAR(4)`, `status VARCHAR(20)`, `automated TINYINT(1)`, `spec_paths JSON NOT NULL`, `reason_category VARCHAR(30)`, `reason_detail VARCHAR(1000)`, plus indexes on `(product_id, epic_code)`, `(product_id, story_code)` and `(product_id, status)` |

`part_code` is `CHAR(1)` with the bin collation. `created_at` and `updated_at` are `DATETIME DEFAULT CURRENT_TIMESTAMP` (`ON UPDATE` for `updated_at`).

**Every bounded string is truncated** rune-safely to its column size before it is written, and the parser records a Problem `<code> <field> truncated to <n>` for each. This applies to `name` (300), `title` (500), `repo_entry` (300), `evidence` (500), `status_as_of` (20), `release_status` (20), `service` (100), `area` (200), `unmapped_reason` (500), `polarity` (4), `level` (8), `priority` (4), `status` (20), `reason_category` (30), `reason_detail` (1000) and `source_file` (300). TEXT fields (`notes`, feature `name`, story `text`, `scenario`) are cut at 60 000 bytes rune-safely, with a Problem. Exceeding a column can therefore never fail a sync with MySQL error 1406.

**Links are kept by code**, not by surrogate id. Then a local draft can point at a synced code, and a parent that is removed and later restored keeps its links without any remapping. Every join is on `(product_id, code)`, which the unique key indexes.

**`kb_story_features`** has columns `product_id` (FK products CASCADE), `story_code`, `feature_code`, `origin ENUM('sync','local')` and `PRIMARY KEY (product_id, story_code, feature_code)`, plus `INDEX (product_id, feature_code)`.
*   It is the **only** way to look up a story's features in either direction.
*   On each sync, the stored `origin='sync'` link set is compared with the snapshot's set. Only the difference is deleted or inserted, so a sync with no changes writes nothing. Links of removed stories go too, because those stories are absent from the snapshot. A restored story gets its links back from the snapshot that restores it.
*   A local draft write replaces its own `origin='local'` rows.

**`kb_annotations`** has columns `product_id`, `entity_type ENUM('part','module','submodule','feature','epic','story','test_case')`, `entity_code`, `note TEXT NULL`, `tags JSON NULL`, `updated_by CHAR(36) NULL`, `updated_at`, and `PRIMARY KEY (product_id, entity_type, entity_code)`, plus the FK products CASCADE. It is keyed independently of the entity rows, so annotations survive re-syncs, removals and restores.

**The format setting** works in two parts, and the order is the point:
*   It is added by a guarded ALTER, `-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'product_settings' AND column_name = 'knowledge_format'`, then `ALTER TABLE product_settings ADD COLUMN knowledge_format VARCHAR(50) NULL DEFAULT NULL`.
    *   `NULL` means it was never set.
    *   `''` means sync is deliberately turned off.
*   The one-time seed: `-- skip-if: SELECT COUNT(*) FROM product_settings WHERE knowledge_format IS NOT NULL`, then:

    ```sql
    INSERT INTO product_settings (product_id, knowledge_format)
    SELECT id, 'test-catalog-v1' FROM products WHERE name = 'SUT'
    ON DUPLICATE KEY UPDATE product_settings.knowledge_format = COALESCE(product_settings.knowledge_format, 'test-catalog-v1')
    ```

    Once any product has a value, including an admin's `''`, the guard skips the seed forever. While no product has a value, the seed runs on every migration and does nothing unless a product named `SUT` exists. That is intended: a SUT product created later still gets the default.

The `kb_*` tables come before the ALTER and the seed, in the order `kb_syncs`, `kb_parts`, `kb_modules`, `kb_submodules`, `kb_features`, `kb_epics`, `kb_stories`, `kb_test_cases`, `kb_story_features`, `kb_annotations`.

### JSON shapes
*   **`counts`:** `{"parts":n,"modules":n,"submodules":n,"features":n,"features_with_stories":n,"epics":n,"stories":n,"stories_unmapped":n,"test_cases":n,"automated":n}`. These are the active rows only (not removed), and `origin='sync'` only.
*   **`diff`:** `{"unchanged":bool,"total":{"added":n,"updated":n,"removed":n,"restored":n},"by_type":{"<type>":{"added":[codes],"updated":[codes],"removed":[codes],"restored":[codes]}},"truncated":bool}`.
    *   The type keys are `part`, `module`, `submodule`, `feature`, `epic`, `story` and `test_case`.
    *   Each code list keeps at most 200 codes, and `truncated` is true when any list was cut.
    *   The diff is computed in Go from the existing rows loaded before the upsert. It **never** relies on `RowsAffected`.
*   **`problems`:** `{"total":n,"by_message":{"<message template>":n},"items":[{"file":"docs/…","line":n,"msg":"…"}]}`, with `items` capped at 500. A message template is the message with its codes and numbers replaced by `#`.
*   **`diff.unchanged`:** `true` when `commit_sha` and `parser_version` equal those of the previous succeeded sync. The UI then shows "Already up to date (abc1234)".

### Sync engine (`knowledge/sync.go`)
`type Syncer struct { DB *db.DB; Pool *repopool.Pool; AI Summarizer }`. `Summarizer` is `interface{ Configured() bool; ChatMessages(context.Context, []models.Message, ai.ChatOpts) (string, error) }`. `*ai.Service` satisfies it, and tests use a fake.

**`Start(ctx, productID int, trigger string, userID string) (int64, error)`** is a thin wrapper around the internal `start(…) (int64, <-chan struct{}, error)`. The channel closes when `run` returns, and the scheduler waits on it.
1.  It loads the following and returns the listed errors:

    | Load | Error when it fails |
    |---|---|
    | product name | `ErrProductNotFound` |
    | `knowledge_format` | `ErrNotConfigured` when it is `NULL` or `''`; `ErrUnknownFormat` when the value is not registered |
    | automation repo (`is_automation = 1`) | `ErrNoAutomation` |
    | default branch: `product_repo_branches.is_default`, falling back to `products.branch`, then to `main` | — |

    A missing pool or DB returns `ErrUnavailable`.
2.  It inserts the `kb_syncs` row with `status='running'`, `started_at=UTC_TIMESTAMP()` and `ai_status='none'`. If that hits duplicate key error 1062 on `uq_kb_syncs_running`, it returns `ErrRunning{SyncID}`. The id comes from a SELECT of the running row.
3.  `go s.run(syncID, …)` and return the id. `run` builds its context from `context.Background()` with a 10-minute timeout. It never uses the request context.

**`run`**
*   A `defer` at the top recovers a panic and calls `finish` for any path that has not reached a terminal status.
*   `finish` writes `status`, `error` and `finished_at` with a **fresh** `context.WithTimeout(context.Background(), 10*time.Second)`. An expired sync context therefore cannot leave the row in `running`.

1.  `ReadTree(ctx, repo.ID, repo.URL, branch, "docs", 32<<20)`. On an error, the status becomes `failed`. The error text is the coarse message from the `repopool` errors; it never contains a URL with credentials.
2.  If the trigger is **`schedule`**: when the latest `succeeded` sync of the product has the same `repo_id`, `branch`, `format`, `parser_version` and `commit_sha`, the status becomes `skipped` with no diff, and `run` returns. Manual syncs always continue. A failed sync never counts as "latest succeeded", so the next tick retries it.
3.  `Parse`. If it returns an error, the status becomes `failed`.
4.  **Then one transaction (`db.WithTx`):**
    1.  It locks the product row: `SELECT … FOR UPDATE`, the same as `lockProduct`.
    2.  For each entity type, in the order parts, modules, submodules, features, epics, stories, test_cases:
        1.  Load the existing rows of the product: `code → origin, content_hash, removed_at`.
        2.  Handle each parsed entity:

            | Existing row | Action | Diff |
            |---|---|---|
            | none | insert with `first_sync_id = changed_sync_id = id` | `added` |
            | `origin='local'` (impossible: local codes start with `L-`, which no parser regex produces) | write nothing, record a Problem | — |
            | `removed_at` not NULL | update and clear `removed_at` | `restored` |
            | hash differs | update | `updated` |
            | otherwise | write nothing | — |
        3.  Rows with `origin='sync'` and `removed_at IS NULL` that were not seen get `removed_at = UTC_TIMESTAMP()` and `removed_sync_id = id`, and go into the diff as `removed`.
        4.  Writes are multi-row `INSERT … ON DUPLICATE KEY UPDATE` statements of at most 200 rows each.
    3.  Apply the difference between the snapshot's links and the stored `origin='sync'` links in `kb_story_features`.
    4.  Compute `counts` with `SELECT COUNT…` inside the transaction.
    5.  **In the same transaction**, update `kb_syncs`: `status='succeeded'`, `finished_at`, `commit_sha`, `parser_version`, `counts`, `diff`, `problems`, and `ai_status = 'pending'` when the diff is not empty and the AI is configured, otherwise `'none'`.

    If the backend crashes before the commit, nothing is applied and the row stays `running` until `FailOrphanedSyncs` marks it failed. After the commit, the data and its status agree.
5.  **AI summary.** Only when `ai_status='pending'`, and with its own `context.WithTimeout(context.Background(), 3*time.Minute)`.
    *   The prompt is built from:
        *   the product name;
        *   the previous and new short SHA;
        *   the diff totals;
        *   for up to 100 changed codes, `code: name/text`, with each text cut to 200 runes;
        *   the problem count;
        *   all clipped to 60 000 runes by `clipRunes`, a new rune-safe helper in the knowledge package. The byte-based `clip` in `api/jobs_executor.go` stays as it is.
    *   The system message is: "You summarise changes to a product test catalog for QA engineers. Reply with 3–8 markdown bullets in English. No preamble."
    *   The call is `ChatMessages(ctx, msgs, ai.ChatOpts{Temperature: 0.3})`.
    *   On success, set `ai_status='done'` and `ai_summary`. On error, set `ai_status='failed'` and `ai_error` (the error text cut to 400 runes). Neither outcome changes `status`.

**`FailOrphanedSyncs(ctx)`** runs at server start, next to `FailOrphanedJobs`, as two statements:
*   `UPDATE kb_syncs SET status='failed', error='backend restarted while the sync was in progress', finished_at=UTC_TIMESTAMP() WHERE status='running'`
*   `UPDATE kb_syncs SET ai_status='failed', ai_error='backend restarted before the summary finished' WHERE ai_status='pending'`

The backend is one process, so every `running` or `pending` row found at startup is an orphan.

**`StartScheduler(ctx, interval)`**
*   `interval <= 0`, a nil pool or a nil DB disables it.
*   A `time.Ticker` fires first after one full interval, so a restart does not trigger a fetch.
*   On every tick, it goes through the products where `knowledge_format` is a registered name, **one at a time**. For each it calls `Start` with `trigger='schedule'` and waits for `run` to finish. `Start` returns an id, and the scheduler waits on a `done` channel.
*   `ErrRunning` and `ErrNoAutomation` are logged at debug level and skipped.
*   It logs once when it starts, as `StartProductVerifier` does.
*   The mirror lock is separate from the verifier's lock, so the two schedulers firing together do not contend.

**Config:** `KB_SYNC_INTERVAL_MINUTES` uses `getEnvInt`, defaults to `360`, and `0` turns it off. It goes into `ProductsConfig.KnowledgeSyncIntervalMinutes` in `internal/config/config.go`, and is documented in `ai-backend/.env.example` and `deploy/qa-tool/.env.prod.example`.

**Wiring in `cmd/server/main.go`:** after `router.SetRepoPool(pool)`:
*   `syncer := knowledge.NewSyncer(database, pool, aiService)` (nil when `database == nil`);
*   `router.SetKnowledge(syncer)`;
*   `syncer.FailOrphanedSyncs(ctx)`;
*   `syncer.StartScheduler(ctx, …)`.

### API (`ai-backend/internal/api/knowledge.go`)
**Routing:** `handleProducts` adds `case parts[1] == "knowledge": r.handleKnowledge(w, req, d, id, parts[2:], canManage, canVerify)`. `router.go` gets only the `knowledge *knowledge.Syncer` field and `SetKnowledge`. The existing `len(parts) > 5` limit holds: the longest path is `{id}/knowledge/annotations/{type}/{code}`, which has 5 parts, and codes never contain `/`.

**Shared rules:**
*   If the product does not exist, every route returns 404.
*   Path codes must match `^[A-Za-z0-9.~_-]{1,64}$`, otherwise 404.
*   Errors use the existing `writeJSONError` body `{"error": "..."}`.
*   Bodies go through `decodeStrictJSON` (64 KiB limit), so an unknown field returns 400.
*   **Writes are allowed for `canVerify`** (Admin, DevOps, Developer, QA, QC). Others get 403. Reads are open to any authenticated user.
*   A list hides removed rows unless `include_removed=1`.
*   Lists are ordered by code.
*   `q` is a `LIKE` search with `%`, `_` and `\` escaped, run against code, name or text, and scenario.
*   `page` starts at 1. `page_size` defaults to 50, and the maximum is 200.
*   **Every entity object** contains: `type`, `code`, `origin`, `removed_at`, `removed_sync_id`, `changed_sync_id`, `source_file`, `source_line`, `source_url`, `annotation: {note, tags, updated_at} | null`.
    *   `source_url` is computed on the server as `https://github.com/<NormalizeRepo(repo_url)>/blob/<sha>/<source_file>#L<line>`, where `sha` is the latest succeeded sync's `commit_sha`.
    *   It is null for removed rows, local rows, or when no sync has succeeded yet.

| Method | Path | Success | Errors |
|---|---|---|---|
| POST | `knowledge/sync` | 202 `{sync_id}` | 400 `not configured` / `unknown format`; 409 `{error:"a sync is already running", sync_id}`; 409 `no automation repo`; 503 when pool or DB missing; 403 |
| GET | `knowledge/syncs?limit=20` (max 100) | `{syncs:[row without diff.by_type and problems.items]}` | |
| GET | `knowledge/syncs/{sid}` | the full row | 404 when `sid` is not an integer or belongs to another product |
| GET | `knowledge/summary` | `{format, formats:[names], configured, enums, repo:{id,name,url,branch}\|null, latest_sync (any status, including running), last_success, counts, coverage:{test_cases, automated, pct}}` | |
| GET | `knowledge/tree?include_removed=0` | `{parts:[{code,name,modules:[{code,name,submodules:[{code,name,features:[F]}], features_without_submodule:[F]}]}]}`, where `F = {code,name,release_status,status_as_of,origin,removed_at,stories,test_cases,automated,coverage_pct,has_annotation}` | |
| GET | `knowledge/{parts\|modules\|submodules\|features\|epics}/{code}` | the entity, its parent and child codes and names, its annotation; a feature also includes `stories:[S with test_cases:[T]]` | 404 |
| GET | `knowledge/epics` | `{epics:[E + case counts per status]}` | |
| GET | `knowledge/stories?epic=&feature=&origin=&tag=&q=&include_removed=&page=&page_size=` | `{items:[S], total}` | |
| GET | `knowledge/stories/{code}` | S + `features:[{code,name,removed_at}]`, `test_cases:[T]` | 404 |
| GET | `knowledge/test-cases?epic=&story=&status=&level=&priority=&automated=&origin=&tag=&q=&include_removed=&page=&page_size=` | `{items:[T], total}` | 400 when an enum filter value is invalid |
| GET | `knowledge/test-cases/{code}` | T + `story`, `features` | 404 |
| PUT | `knowledge/annotations/{type}/{code}` with body `{note, tags}` | 200 annotation | 400 for a bad type, a note over 10 000 runes, or more than 20 tags (each tag must be 1–40 runes); 404 when no entity has that code (removed entities count as existing) |
| DELETE | `knowledge/annotations/{type}/{code}` | 204 | |
| POST | `knowledge/stories` with body `{code, epic_code, text, feature_codes[]}` | 201 S | 400 when `code` does not match `^L-[A-Z0-9-]{1,30}$`, or `epic_code` / a feature code is not an active entity of the product; 409 when the code exists |
| POST | `knowledge/test-cases` with body `{code (^L-[A-Z0-9-]{1,30}$), epic_code, story_code?, scenario, polarity, level, priority, status, spec_paths[], reason_category?, reason_detail?}` | 201 T | 400 under the enum and need-reason rules, or when the epic or story is not an active entity; 409 when the code exists |
| PUT | `knowledge/stories/{code}`, `knowledge/test-cases/{code}` (same body without `code`) | 200 | **409 `managed by repo <repo name>; edit it there, then Sync`** for a synced row; 404 |
| DELETE | `knowledge/stories/{code}`, `knowledge/test-cases/{code}` | 204, a hard delete that also removes its annotation and its local `kb_story_features` rows | 409 for a synced row; 409 `local test cases still reference this story` |

`counts` and `coverage` count only synced rows. Local drafts are counted separately as `local:{stories, test_cases}`. Coverage means `automated ÷ distinct test cases`; with zero test cases it is `0`. When a draft links to a parent that was later removed, the link is shown with the parent's `removed_at`.

**Product format:** `PUT /api/products/{id}` accepts an optional `knowledge_format`. The value must be `""` or a registered name, and only `canToggleWork` (Admin, DevOps) may set it; anyone else gets 403. `loadProducts` returns `knowledge_format` as a string, with `NULL` returned as `""`, in `models.Product.KnowledgeFormat`.

### Dashboard
The dashboard is built by the `frontend-worker` agent, which applies Emil Kowalski's design-engineering rules (https://emilkowal.ski/skill).
*   **`views/knowledge/api.ts`** holds the types and fetchers, and reuses `api<T>()` and `ApiError`.
*   **`views/knowledge/KnowledgeCard.tsx`**, shown in `ProductDetail.tsx` below the repo cards:
    *   **When the product is not configured:** one line, "No catalog format set". Admins and DevOps also get a `<select>` of `formats`.
    *   **Sync button:** it is fixed-width, and its RefreshCw icon rotates while a sync runs. Next to it, the latest sync's relative time (with the absolute time as a tooltip), the short SHA linked to the commit, and a status chip.
    *   **Tiles:** Modules, Features, Stories, Test cases and Automated %.
    *   **"What changed" disclosure:**
        *   the AI summary, with `- ` lines rendered as a list and no markdown dependency;
        *   a "Summarising…" placeholder while `ai_status='pending'`;
        *   the +/~/−/↺ totals;
        *   "Already up to date" when `diff.unchanged` is set;
        *   the problems, as `by_message` counts with the items expandable.
    *   **"Open knowledge base →"** link.
    *   **Polling:** `GET syncs/{sid}` every 1.5 s while `status='running'` or `ai_status='pending'`. It stops after 10 minutes or on unmount. On load, if `latest_sync` is running, polling resumes.
*   **The `/products/:id/knowledge` route** (`App.tsx`, inside `ProtectedRoute`/`Layout`) shows `KnowledgeView.tsx`, with the active tab in `?tab=`:
    *   **Modules:** a collapsible tree from parts to modules to sub-modules to features. Each feature shows a release chip, story and test-case counts, and a coverage bar.
    *   **Stories:** a list with search and filters for epic, feature and tag.
    *   **Test cases:** a paginated table with filters for status, level, priority, epic, automated and tag.
    *   **Syncs:** the history.
*   **The drawer** (`Drawer.tsx`) opens for any row:
    *   the entity, its trace chain, `source_url` as "View in repo", and the note and tag editor;
    *   for local drafts, edit and delete actions;
    *   for synced rows, a lock icon with "Managed by repo — edit there, then Sync".

    Rows are marked by state:
    *   Removed rows are struck through and show "removed in sync #n".
    *   Local rows carry a "Local" badge.
*   **"New draft" story or test case:** canVerify roles get this action, which opens a `Modal` form.
*   **Motion:**
    *   The drawer and tab changes use transform and opacity only, ease-out, 150–200 ms.
    *   Filters, search and pagination are not animated.
    *   `prefers-reduced-motion` turns motion off.
    *   Buttons press to `scale(0.97)`.
    *   Hit targets are at least 32 px.
    *   The drawer traps focus and closes on Escape.
    *   Icons come from lucide-react only.

### Amendment 1 (2026-11-14): coverage and detail pages
Every entity gets a `Coverage` object: `{test_cases, automated, planned, blocked, manual_only, wont, pct, state}`. `state` is one of:
*   `untested`: no test cases;
*   `none`: test cases, but none automated;
*   `partial`: some automated;
*   `full`: all automated.

For a part, module, sub-module or feature, coverage counts the distinct active test cases of the active stories linked to its active features. For an epic or a story, it counts that epic's or story's active test cases.

New and changed API:
*   **New:** `GET knowledge/coverage?by=part|module|submodule|feature|epic|story[&part=&module=&submodule=&epic=]`, which returns `{by, coverage:{code: Coverage}}`. An unknown `by` is 400.
*   **New:** `GET knowledge/modules`, which returns modules with `coverage`, `features` and `submodules` counts.
*   **New:** `GET knowledge/features?part=&module=&submodule=&release=&coverage=<states,…>&q=&page=`, paginated, with coverage on every item.
*   **Changed:** `stories` and `test-cases` accept `part`, `module` and `submodule` filters, which work through the linked features. `stories` also accepts `coverage=`.
*   **Changed:** detail endpoints return the entity's `coverage` and the coverage of the children they list.

Dashboard: one page per entity at `/products/:id/knowledge/:type/:code`. Each page has:
*   a breadcrumb trail;
*   a one-sentence coverage summary;
*   a case grid: one square per test case, colored by status;
*   child lists filtered by automation state.

Later addition: `features` accepts `sort=tested|code`. The default, `tested`, lists features that have test cases first. The feature detail loads all its stories' test cases in one query.

### Amendment 2 (2026-11-14): assistant files
The same sync, with the same button, schedule and history, also reads these files from the automation repo:

| Kind | Files |
|---|---|
| `agent` | `.claude/agents/**/*.md` |
| `skill` | `.claude/skills/<name>/SKILL.md` |
| `rule` | `.claude/rules/**/*.md` |
| `command` | `.claude/commands/**/*.md` |
| `instructions` | every `CLAUDE.md` / `AGENTS.md` outside `node_modules`, `vendor`, `.git`, `dist` and `build` |

`ReadTree` now takes a path filter (`keep func(path) bool`) instead of a prefix, and the sync passes `KeepForSync`.

Parsing:
*   **Frontmatter:** the YAML is parsed with `yaml.v3` and stored as JSON. Invalid YAML is a Problem, and the file is still stored.
*   **Name:** frontmatter `name`; otherwise the skill's folder name, the instructions file's path, or the file name.
*   **Description:** frontmatter `description`; otherwise the first plain paragraph line, otherwise the first heading.
*   **Body:** capped at 1 MiB.

Storage:
*   Files go into `kb_assistants`, keyed by `code = <kind>~<slug(path,33)>-<6 hex sha1(path)>`. The table has the same soft-delete, diff and annotation semantics as the other entity tables.
*   `kb_annotations.entity_type` gains `assistant` (guarded ALTER).
*   `parser_version` is now `format.Version()*1000 + AssistantsVersion`, so a parser upgrade re-syncs an unchanged commit.
*   `counts` gains `agents`, `skills`, `rules`, `commands` and `instructions`.

API:
*   `GET knowledge/assistants?kind=&q=&page=` lists one product's files. Bodies are left out of the list, and the response includes `kinds` counts.
*   `GET knowledge/assistants/{code}` returns the full file, including its body.
*   `GET /api/assistants?product_id=&kind=&q=&page=` lists the files of every product. Each item has `product_id`, `product_name` and `source_url`.

QA check skills (the `skills` table) gain `product_id INT NULL` (guarded ALTER; `NULL` means global). The filter `GET /api/skills?product_id=N` returns the global skills plus product N's; `product_id=global` returns only the global ones.

Dashboard:
*   `/skills` gets a product filter and a switch between "QA checks" and "Repo skills".
*   New `/agents` and `/rules` pages, where "rules" covers rules, commands and instructions, with the same product filter.

### Rollback
Everything here is additive. The previous binary ignores the new tables and the new column. Rolling back is a redeploy of the previous image, with no data repair needed. The mirrors under `.kb-mirrors/` can stay where they are.

## Accepted limitations
*   **Sub-module renames.** Sub-modules have no id in the repo, so renaming one counts as one removal plus one addition. Its annotation stays on the removed code.
*   **Epic codes can change once.** An epic file that has no stories gets a code from its path. When its first story is added, the code changes to the story's epic code. Its old epic row is then removed and its annotation stays on the old code.
*   **Scenarios.** Only the G/W/T cell of each test case is stored. `SC-*` prose and `docs/*-scenarios.md` are not parsed.
*   **Scheduling.** The scheduler runs products one at a time.
*   **Parser drift.** The parser is a port of `test-catalog.mjs`. If that script's format changes, the Go port needs a matching change and a `Version()` bump. The golden test and the `problems.by_message` counts surface any drift.
*   **No incremental fetch.** The mirror is fetched with `--depth=1`, so every fetch transfers the `docs/` objects again. That costs about 1–2 MB per changed sync.

## Assumptions Goldfish reviewers must challenge
1.  The parser rules match `test-catalog.mjs` at `521618a`, apart from the differences listed above.
2.  **The sync never writes to the product clone, its worktrees or its remote.** It only reads and writes its own bare mirror, with hooks, gc and maintenance disabled and object bytes read raw.
3.  Three things together prevent a stuck sync: the `running_flag` UNIQUE key, a terminal write with a fresh context, and `FailOrphanedSyncs`.
4.  Keeping links by code is correct for soft delete and restore.
5.  The two-guard seed runs once, and it never overrides an admin's decision.
6.  The AI summary is the only LLM call. It goes through `*ai.Service` (ai-manager → claude-v3), and its failure never fails a sync.

## Review resolution (rev 1 → rev 2)

| Finding | Resolution |
|---|---|
| Design B1 and codebase 9: the seed overrode an admin's choice | The column is NULL by default, and the seed is guarded |
| Design B2: enums | Enums table, and `enums`/`formats` returned in `summary` |
| Design M1: skipping on the SHA alone | The skip tuple now includes `parser_version`, `format`, `repo_id` and `branch`; manual syncs report `unchanged` |
| Design M2: the status could disagree with the data | The status is written in the same transaction as the data |
| Design M3: the AI summary had no state | `ai_status` added; the UI keeps polling while it is `pending`; the summary uses its own timeout; orphaned `pending` rows are reconciled |
| Design M4: local drafts | The promoted path is removed; deletes are hard; 409 when a draft is still referenced; epic and story must exist |
| Design M5: link ownership | `kb_story_features.origin` added |
| Design M6: epic codes | Duplicates: first file wins; fallback code is the slugged path; listed under Accepted limitations |
| Design M7: endpoints the UI needed | Detail endpoint for every type; annotation returned on rows; `source_url`; `removed_sync_id`; tag filter; coverage defined; `latest_sync` includes a running sync |
| Design M8: shapes and errors | JSON shapes section; error column in the API table |
| Codebase 1: `users.id` type | Changed to `CHAR(36)` |
| Codebase 2, 3, 5 and 15: the sync could touch the shared clone | Bare mirror per repo with raw objects, hooks, gc and maintenance off, URL from the DB, explicit refspec |
| Codebase 4: busy and colliding with the verifier | The mirror has its own lock; the scheduler runs serially and waits for each sync |
| Codebase 6: a sync could stay running | The terminal write uses a fresh context |
| Codebase 7: collations | Every code column uses `utf8mb4_bin` |
| Codebase 8: sub-module codes | `~` separator, cut to 33 characters, a 6-hex hash always added (at most 44 characters) |
| Codebase 10: drift from mjs | Last wins, the out-of-heading feature is stored, a row with unknown status is skipped, the fallback is the path |
| Codebase 11 and 12: file regex and CRLF | `[^/]+`; trailing `\r` stripped |
| Codebase 13: `clip` | New rune-safe `clipRunes` |
| Codebase 14: config naming | `KB_SYNC_INTERVAL_MINUTES` under `ProductsConfig` |
| Codebase 16 and design minor: "changes nothing" was false | Unchanged rows are not written; `changed_sync_id` replaces `last_sync_id` |
| Codebase 17: smaller items | JSON columns have no defaults; `NewRouter` is wired after the pool; the real dispatch signature is used |

### Review resolution (rev 2 → rev 3, implementation readiness)

| Finding | Resolution |
|---|---|
| M1: the mirror was not pinned | Every command uses `--git-dir`, `GIT_CEILING_DIRECTORIES` and `safeDirs`; the mirror is resolved through `resolveValidated`; a mirror without `HEAD` is re-initialised |
| M2: hash vs line number | The location is left out of the hash and updated quietly |
| M3: column overflow | Every bounded string is truncated to its column size, with a Problem |
| M4: `unchanged` | It is now `diff.unchanged` |
| M5: idempotency of links | Only the link-set difference is applied |
| Minor findings | Local TC code regex; FK on links; full guard text; qualified seed column; seed re-run is intended; pack housekeeping; internal `start` returns a done channel; `source_*` nullable on every entity; `.kb-mirrors` ban dropped (`relDirRe` already blocks a leading `.`); git ≥ 2.29; `--prune` removed |

## Target Files
*   **Spec PR:** `docs/designs/PRODUCT_KNOWLEDGE_SYNC.md`, `docs/designs/SITEMAP.md` (this row added; `PRODUCT_REPOS_AUTOMATION_ROLE.md` flipped to Implemented, PR #17).
*   **Backend:**
    *   `ai-backend/internal/services/repopool/repopool.go` (`ReadTree`, `runBytes`) and `repopool_test.go`, `README.md`
    *   new `ai-backend/internal/services/knowledge/` (`catalog.go`, `sut.go`, `sync.go`, `store.go`, `query.go`, `text.go`, `*_test.go`, `testdata/**`, `README.md`)
    *   `ai-backend/internal/db/migrations.go`, `ai-backend/internal/db/README.md`
    *   new `ai-backend/internal/api/knowledge.go`, `knowledge_test.go`
    *   `ai-backend/internal/api/products.go`, `ai-backend/internal/api/router.go` (the field and setter only), `ai-backend/internal/api/README.md`
    *   `ai-backend/internal/models/models.go`
    *   `ai-backend/internal/config/config.go`
    *   `ai-backend/cmd/server/main.go`
    *   `ai-backend/.env.example`, `deploy/qa-tool/.env.prod.example`
*   **Dashboard:** `dashboard/src/App.tsx`, `dashboard/src/views/products/ProductDetail.tsx`, `dashboard/src/views/products/api.ts`, new `dashboard/src/views/knowledge/*`, `dashboard/src/index.css` (drawer and reduced-motion only), `dashboard/README.md`.
*   **E2E:** new `this-qa-automation-system-playwright/tests/knowledge.spec.ts`, following that directory's conventions.
*   **Not changed:** the executor, Dockerfile, compose, nginx and `lib/`.

## Verification Plan
1.  `cd ai-backend && gofmt -l` on the touched files, then `go build -mod=mod ./... && go vet -mod=mod ./... && go test -mod=mod ./...`.
    *   The golden parser test runs.
    *   The `ReadTree` test uses a local bare "remote" made in `t.TempDir()` and served as a file URL through a test-only seam. It asserts that the product clone's `.git` mtime and refs are unchanged.
    *   The sync tests use a fake `Summarizer`. The DB tests run when `QA_TEST_MYSQL_DSN` is set, and cover:
        *   idempotency (no row writes and an empty diff);
        *   remove and restore;
        *   annotation survival;
        *   409 when a sync is running;
        *   the schedule skip;
        *   the orphan reconcile.
2.  Migration: run `go run -mod=mod ./cmd/databasemigration` against prod `qa_automation` before the binary is deployed, then run it a second time and confirm it does nothing.
3.  `cd dashboard && npm run build`.
4.  After deploy, through `qa-tool-session` as `ai-service` impersonating an Admin (the user requested this write):
    1.  Call `POST /api/products/<SUT>/knowledge/sync` and poll until it succeeds.
    2.  Call `GET summary`. The counts must match the current HEAD.
    3.  Sync again. The diff must be empty and `unchanged` must be true.
