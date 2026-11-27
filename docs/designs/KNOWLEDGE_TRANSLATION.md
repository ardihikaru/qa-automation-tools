# Knowledge translation (EN / ID) and draft write-back to the automation repo

Status: Draft. Author: ardihikaru (drafted by Claude). Components: ai-backend, dashboard. Date: 2026-11-27.
Depends on [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md), [CASE_AUTOMATION.md](CASE_AUTOMATION.md), [AI_CONVERSATIONS.md](AI_CONVERSATIONS.md). Code references use function names; lines are as of `main` at `4895fc0`.

## Overview
The knowledge base (`kb_*`) is English, because the automation repo's `docs/` catalog is English. This spec adds (1) an Indonesian (ID) reading view, translated by an LLM and stored only in the DB, and (2) ID authoring of drafts, translated to English on save, plus an optional, human-triggered write-back of drafts to the repo as a pull request.

## Problem
1. Indonesian-speaking QA staff read and search an English catalog (scenarios, stories, features, modules).
2. Drafts (`origin='local'`, `L-…` codes, `SaveDraft` in `knowledge/query.go`) live only in the DB. They never reach the repo, so the repo, GAOS and the exports cannot see them.

## Goals
- ID text for: `kb_parts.name`, `kb_modules.name`, `kb_submodules.name`, `kb_features.name` + `notes`, `kb_epics.title` + `area`, `kb_stories.text`, `kb_test_cases.scenario`.
- Per-user language (`en|id`), a header toggle, fallback to English, a "machine translated" marker.
- Search (`q`) matches English and Indonesian text.
- Translation runs in the background after each sync, only for new or changed text. It never blocks or fails a sync.
- A draft written in ID is stored in English in the normal columns; the ID original is kept.
- Drafts can be published to the automation repo as a PR (English), never to the default branch.

## Non-Goals
Translating the dashboard chrome (labels, buttons); other languages; translating annotations, notes, job logs or AI summaries; changing the repo or what a sync writes to `kb_*`; changing exports and the GAOS push (they keep reading English `kb_*`).

## Decisions
- **Model.** `ChatOpts` gains `Model string`. `aiManagerProvider.Chat` sends `opts.Model` when set, else `p.cfg.Model` (`ai/aimanager.go`, request built around line 102). Provider stays `claude-v3` with `no_fallback`, and the existing pin check (`aimanager.go`, "answered by … pinned to") still applies. No new upstream, no direct call to `:8105`. New env `AI_TRANSLATION_MODEL` (default `claude-haiku-5-5`; empty disables translation). The id is **unverified** (Open question 1). `ChatResult.Model` is recorded by `ai_call_log` / `ai_conversations`, so cost and the model actually used are visible.
- **Storage.** New `kb_translations`, never touched by `store.go` `apply`, so a sync cannot overwrite it and the English columns are unchanged. The key is `(product_id, entity_type, code, field, lang)`: the requested `(product_id, entity_type, code, lang)` plus `field`, because features, epics and others have two text fields, and per-field staleness avoids re-translating a scenario when only its status changed. Staleness is `src_hash` (SHA-256 of the trimmed, NFC-normalised English field text) versus the current text; the row `content_hash` is not used because it also covers status, level and links.
- **Language.** `users.language` (`en|id`, default `en`). `?lang=en|id` overrides it per request. Impersonation reads the effective user.
- **Read shape (additive).** Existing English fields never change. When the effective language is `id`, items gain `i18n: { lang, fields: { <field>: { text, state } } }`, `state` in `machine | original | stale`. The dashboard shows `i18n` text when present, else English, with the marker for `machine` and `stale`. A stale text is still shown (and searched) until re-translated.
- **Search.** `q` stays English-or-ID regardless of the display language: each `LIKE` gets `OR EXISTS (SELECT 1 FROM kb_translations x WHERE x.product_id = t.product_id AND x.entity_type = ? AND x.code = t.code AND x.lang = 'id' AND x.text LIKE ?)`. Existing `q` is in `Stories` (`query.go`), `TestCases` (`query.go`) and features (`coverage.go`). `Epics` and modules get a new `q` (code + title/area, code + name).
- **Translator** (new `knowledge/translate.go`): a `Translator` built beside the `Syncer`, started from `Syncer.OnSucceeded` (the hook documented in `knowledge/README.md`; panics are recovered by `notify`). It selects fields with no `lang='id'` row or a stale `src_hash` among active rows, batches up to 20 fields (or 6,000 characters) per call as JSON `{id, text}` in and out, and writes the rows in one transaction per batch. Backfill is the same code path on first run (nothing exists yet), plus `POST …/translations/run` to force it. One run per product at a time, at most 2 products in parallel (`TRANSLATE_CONCURRENCY`, default 1). Calls use `Feature: "knowledge-translation"`, temperature 0.1, and the default conversation kind `call`.
- **Prompt and validation.** System prompt: translate to Indonesian; keep codes (`TC-…`, `US-…`, `A01.07`), UI labels in quotes, URLs and identifiers unchanged; return JSON only. A batch is rejected if the JSON is invalid, ids differ, a field is empty, or a code token of the source is missing from the output. The batch is then split in halves and retried once; single items that still fail go to `kb_translation_failures` (attempts, `next_try_at` with backoff 1 h, 6 h, 24 h, give up at 5, `last_error` clipped like `ai_error` in `sync.go`). The next run retries due failures.
- **Draft in ID.** The draft body gains `lang` (`en|id`, default the user's language). When `id`, `SaveDraft` first translates the text fields (`scenario`, `text`) to English with the same model (synchronous, `AI_TIMEOUT_SECONDS`), stores English in `kb_test_cases` / `kb_stories`, and stores the typed ID in `kb_translations` with `state='original'`. A translation failure returns 502 and saves nothing (the form keeps the text). Editing a draft in `en` marks its ID original stale; the translator then replaces it with a machine text. A machine run never overwrites a fresh `original`.
- **Write-back, recommended: human-triggered, PR only.** Automatic publishing is rejected: drafts are unreviewed and a repo PR is hard to retract. A "Publish drafts" button (roles with `canWrite`) opens one PR per epic on the automation repo. Details below.

## Write-back design
Reads today: `ReadTree` of the bare mirror on the default branch, parsed by `testCatalog.Parse` (`testcatalog.go`): an epic is one `.md` under `docs/test-catalog/`, stories are `- **US-X-NN** ([A01.07](…)) — text` bullets under `## User stories`, cases are 10-cell rows `| ☐ | TC-X-NNN | US-X-NN | scenario | + | API | P1 | planned | | |` (cells split on `|`, no escaping, `cells()`). Writes reuse the case-automation git path.
1. **Guard.** Only the product's automation repo is ever a target (`loadAutomationRepo`, `api/case_automation.go`). Before each step call `repoWriteAllowed` with `RepoOpCreateBranch`, `RepoOpPush`, `RepoOpOpenPR` (`api/product_repos.go`), which honours `allow_non_automation_work` and the automation role. Otherwise 403 `ErrWriteNotAllowed`.
2. **Preconditions.** No running sync; the product's latest succeeded sync commit equals the default branch head after `FetchOrigin`, else 409 "sync first". Drafts that are `origin='local'`, have English text, and are not in an open publication. A draft case whose story is also a draft is published with that story or not at all.
3. **Branch.** `qa-auto/kb-<epic>-<UTC yyyymmddhhmm>` (the `qa-auto/` prefix is enforced by `Pool.Push` and `Pool.OpenPR`); `Pool.AddWorktree` from the default branch, exactly like `poolRepo.Prepare`. The default branch is never pushed, and push is never forced (`worktree.go`).
4. **Codes.** Repo codes are assigned here: next free `TC-<E>-NNN` / `US-<E>-NN`, where `<E>` is the segment used by the epic's existing codes, and NNN is the maximum over the worktree file, all `kb_*` rows (removed included) and open `kb_publications`, plus one. An epic with no existing code to copy the segment from is refused (Open question 6).
5. **Render** (new `knowledge/publish_render.go`, pure): append story bullets after the last bullet of `## User stories` (links to the feature part files, relative to the epic file) and case rows after the last row of the table, in English, with `Done` = ☐ and the draft's status. Text containing `|` or a newline is refused with a clear error, because `cells()` cannot represent it.
6. **Round-trip gate.** Before commit, run `testCatalog.Parse` over the worktree's files: it must add no new Problem, and the new stories and cases must equal the drafts field by field. Otherwise abort and remove the worktree.
7. **Commit, push, PR** with `Pool.Commit` (the one epic file), `Pool.Push`, `Pool.OpenPR`; the body lists local → repo codes and the author. Record `kb_publications` rows. The draft stays (shown "PR open") and is read-only (edit and delete return 409) while the PR is open.
8. **Reconcile** inside `OnSucceeded` before translation is queued, with `Pool.PRStatus` (`repopool/prstatus.go`, read-only). Merged and the repo code now exists as `origin='sync'`: in one transaction, move annotations, verifications (and events) and translations from the `L-` code to the repo code (an `original` ID text whose English hash no longer matches, because a reviewer edited the PR, becomes stale), mark the publication `merged`, and hard-delete the `L-` draft (as `DeleteDraft` does). Closed without merge: publication `closed`, the draft is publishable again, the reserved code is released.
Round-trip safety: `L-` codes can never collide with repo `TC-`/`US-` codes, so `store.go` (`exists as a local draft; repo entry ignored`) is never triggered and no Problem is raised; a repo entry is simply a new sync row. Two PRs racing for the same NNN conflict at merge; the second is closed and republished with fresh codes.

## Schema (appended to `migrations.go`, idempotent, skip-if pattern as `knowledge_format`)
- `ALTER TABLE users ADD COLUMN language ENUM('en','id') NOT NULL DEFAULT 'en'`.
- `kb_translations(product_id INT, entity_type ENUM('part','module','submodule','feature','epic','story','test_case'), code VARCHAR(64) utf8mb4_bin, field VARCHAR(20), lang CHAR(2), text TEXT, src_hash CHAR(64), state ENUM('machine','original'), model VARCHAR(64), translated_at DATETIME, PRIMARY KEY (product_id, entity_type, code, field, lang), FK product_id ON DELETE CASCADE)`. Search uses the PK prefix plus `LIKE` (a scan per product, like the current `LIKE` on `kb_*`).
- `kb_translation_runs(id BIGINT AI, product_id, trigger_kind ENUM('sync','manual','draft'), status ENUM('running','succeeded','failed'), total, done, failed INT, input_tokens, output_tokens INT, cost_usd DECIMAL(10,4), started_at, finished_at, error VARCHAR(400))`.
- `kb_translation_failures(product_id, entity_type, code, field, attempts TINYINT, next_try_at DATETIME, last_error VARCHAR(400), PRIMARY KEY (product_id, entity_type, code, field))`.
- `kb_publications(id BIGINT AI, product_id, entity_type ENUM('story','test_case'), local_code, repo_code, epic_code, branch, pr_url, state ENUM('open','merged','closed'), created_by CHAR(36), created_at, updated_at, UNIQUE (product_id, entity_type, local_code, state))`; `kb_translation_runs` startup cleanup mirrors `FailOrphanedSyncs`.

## API
- `GET /api/auth/me` gains `language`; `PUT /api/auth/language` `{language}`.
- Knowledge list and detail routes accept `?lang=`; items gain `i18n` (above). `q` as above.
- `GET /api/products/{id}/knowledge/translations/status` (any reader) returns `{enabled, model, lang_coverage: {total, translated, stale, missing, failed}, running, last_run}`. `POST …/translations/run` (`canWrite`) forces a run, 409 while running, 503 when `AI_TRANSLATION_MODEL` is empty.
- Draft `POST` / `PUT` bodies gain `lang`.
- `GET …/knowledge/publish/preview` returns drafts grouped by epic with the planned codes and blockers. `POST …/knowledge/publish` `{epic_code}` (`canWrite`) returns `{publications, pr_url}`, 409 / 403 / 422 for the failures above. Long git work runs in a background goroutine with state in `kb_publications`, polled by the dashboard (Open question 5: reuse `job_runs`?).

## Dashboard
- `TopBar.tsx`: EN | ID toggle; `AuthContext.tsx` holds `language`; `knowledge/api.ts` sends `lang`.
- `knowledge/ui.tsx`: a "machine translated" badge (tooltip with model and date; "outdated" for stale); used in `StoriesTab`, `TestCasesTab`, `FeaturesTab`, `ModulesTab`, `EpicsTab`, `EntityPanel`, `Drawer`, `pages/`. The English text stays reachable from the badge.
- `DraftModal.tsx`: language selector (default the user's), shows the stored English after save. `KnowledgeView`/`SyncsTab`: translation status chip and "Translate now". A Drafts panel with "Publish drafts to repo" per epic (preview of codes, PR link, state).

## Failure modes
| Failure | Behaviour |
|---|---|
| ai-manager down, model rejected, rate limit | Run `failed`, rows go to `kb_translation_failures`; sync already committed; English shown. |
| Reply broken or drops a code | Split, retry once, then a failure row. |
| Source edited mid-run | `src_hash` mismatch at write: row not written. |
| Draft translation fails | 502, nothing saved. |
| Push or PR fails after the branch exists | Publication stays absent; worktree removed; retry creates a fresh branch. |
| PR reviewer edits text | Reconcile marks the ID original stale. |
| Server restart mid-run | Orphan runs failed at startup; the next sync or manual run resumes by the pending query. |

## Cost and limits
Roughly 2.5k catalog fields on a catalog like the reference one (about 1.7k cases, 250 stories, 450 features), so about 130 calls at 20 per batch for a backfill; steady state is a handful of calls per sync. The real figure is unmeasured: the first backfill reports tokens and cost in `kb_translation_runs` and `/cost`. Limits: batch 20, concurrency 1 (max 2), temperature 0.1, 1 s pause between calls, nothing translated while `AI_TRANSLATION_MODEL` is empty. Catalog text is sent to claude-v3 (as sync summaries already do).

## Tests
Go: `ChatOpts.Model` override with and without `p.cfg.Model` (`aimanager_test.go`); hash and staleness; batch validation, split and backoff with a fake `ChatMessages`; a hook that panics or fails does not fail a sync; search `EXISTS`; `SaveDraft` with `lang=id` (fake AI, 502 saves nothing); renderer golden plus the Parse round-trip over `testdata/sample-catalog`, including `|` refusal; publish with a fake `repoOps` (as the `caseauto` fakes) for guard denial, code allocation races, closed and merged reconcile. DB tests need `QA_TEST_MYSQL_DSN`. Playwright (`this-qa-automation-system-playwright/`): toggle, marker, ID search finds an English row, draft in ID.

## Delivery (small PRs)
1 this spec. 2 `ChatOpts.Model` + env + smoke test (gate). 3 schema + translator + hook + status endpoint. 4 `i18n` read shape + search. 5 `users.language` + toggle + badges. 6 draft ID → EN. 7 renderer and round-trip gate (no git). 8 publish endpoint, reconcile, dashboard. Migrations run manually at deploy; deploy is a separate approval.

## Out of scope
Dashboard chrome i18n; translating notes, annotations, verification notes; ID in exports or GAOS; editing machine translations by hand; editing or deleting repo-synced rows; publishing anything other than stories and test cases; auto-publish; merging the PR.

## Open questions
1. Does claude-v3 accept `claude-haiku-5-5` (exact id unconfirmed)? Smoke-test through ai-manager (never `:8105`) before PR 2. The pin check compares only the provider, so a silently substituted model would pass; compare `ChatResult.Model` in the test. If unsupported: fall back to the claude-v3 default model (cost higher) or stop.
2. Add `field` to the requested key `(product_id, entity_type, code, lang)`? Chosen here; alternative is one row with a JSON of fields and per-field hashes (harder to search).
3. Is the repo text always English? A row already in Indonesian (some columns, such as "Periode pelaksanaan", suggest local usage) would be "translated" to itself. The prompt says to return Indonesian text unchanged; confirm.
4. Glossary: keep Given / When / Then and product UI labels in English, or translate? Needs a native-speaker review of the first batch.
5. Background publish state: new `kb_publications` polling versus a `job_runs` job type with the live log. `policy_propose` (`api/learning_loop.go`) is the precedent for PR-opening jobs; not read in depth here.
6. Epic code segment of `TC-<E>-NNN` is inferred from existing codes; `testcatalog.go` derives `e.Code` from the file, and whether it always equals that segment is unverified. Epics without any case are refused.
7. After merge, `job_runs`, `kb_case_results` and `case_approvals` rows that reference the `L-` code: re-key them or leave history under the old code? Not investigated.
8. Should a conversation kind `translation` be added to `ai_conversations` (130 single-call conversations of kind `call` are noisy)?
9. Assumption to challenge: the publish gate "latest sync equals head" is enough to keep code allocation correct under concurrent human PRs.

## Target Files
- `ai-backend/internal/db/migrations.go`, `ai-backend/internal/config/config.go`, `ai-backend/cmd/server/main.go` (wire `Translator`, hook)
- `ai-backend/internal/services/ai/provider.go`, `aimanager.go`, `aimanager_test.go`
- `ai-backend/internal/services/knowledge/` new `translate.go`, `translate_test.go`, `publish_render.go`, `publish.go`, `publish_test.go`; edit `query.go`, `coverage.go`, `sync.go` (hook only), `README.md`
- `ai-backend/internal/api/` `router.go`, `auth_handlers.go` (language), `knowledge.go`, new `knowledge_translate.go`, `knowledge_publish.go`, `product_repos.go` (reuse only), `README.md`
- `ai-backend/internal/auth/` (user `language` field)
- `dashboard/src/auth/AuthContext.tsx`, `api.ts`; `dashboard/src/components/TopBar.tsx`
- `dashboard/src/views/knowledge/` `api.ts`, `ui.tsx`, `DraftModal.tsx`, `KnowledgeView.tsx`, `SyncsTab.tsx`, `StoriesTab.tsx`, `TestCasesTab.tsx`, `FeaturesTab.tsx`, `ModulesTab.tsx`, `EpicsTab.tsx`, `EntityPanel.tsx`, `Drawer.tsx`, new `DraftsPanel.tsx`, `pages/*`; `dashboard/README.md`
- `this-qa-automation-system-playwright/` specs; `deploy/qa-tool/.env.prod.example` (`AI_TRANSLATION_MODEL`, `TRANSLATE_CONCURRENCY`)
