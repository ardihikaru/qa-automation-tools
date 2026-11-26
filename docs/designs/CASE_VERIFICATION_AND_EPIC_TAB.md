# Case verification, run history, Epics tab and system timezone

Status: Draft. Author: ardihikaru. Components: ai-backend, dashboard.

## Problem
1. `/jobs/:id` shows no start, end or elapsed time and no other runs of the same test case (TC).
2. A TC page shows no history of the jobs that ran it.
3. The knowledge base has no list of epics with their coverage.
4. Nothing records that a human checked a TC, so machine results and drafts cannot be trusted or filtered.

## Decisions
- **Run history** reuses `GET /api/products/{pid}/knowledge/test-cases/{code}/jobs` (`listCaseJobs`). It gains `?limit=` (default 10, max 50) and a `summary {total, success, failed}`. A job counts as *success* when it is `completed` with outcome not `fail`, and as *failed* when it is `failed` or its verdict is `fail`. Queued and running jobs are listed but not counted.
- **Job detail**: `GET /api/jobs/{id}` adds `case_code`, `product_id` (from `payload`) and `elapsed_ms` (`completed_at` or now, minus `started_at`; `created_at` when never started). Existing fields are unchanged.
- **Verification** is stored in a new table `kb_case_verifications`, not on `kb_test_cases`, so a knowledge sync can never overwrite it. States: unverified (no row), `valid`, `invalid`. Re-verifying overwrites the row (latest verifier wins); `verdict: null` deletes it. Roles: those with `canWrite` in `handleKnowledge` (`canVerify`). Impersonated calls record the target user and follow the existing impersonation audit.
- **Verification history** is an append-only table `kb_case_verification_events`. Every successful verification PUT (valid, invalid, or a clear, recorded as `cleared`) inserts one event in the same transaction as the `kb_case_verifications` upsert or delete. The latest-row table stays the source of the list filters.
- **Epics tab** is a new tab in the product knowledge page. Coverage per epic reuses `Syncer.CoverageBy(..., "epic")` (automated, planned, blocked, manual-only, wont, pct) and adds `verified_valid`, `verified_invalid`, `last_pass`, `last_fail` from one grouped query joining `kb_test_cases`, `kb_case_verifications` and `kb_case_results`.
- **System timezone** is a row in new `app_settings` (`timezone`, default `Asia/Jakarta`, shown as WIB). It is returned by `GET /api/settings` and changed by `PUT /api/settings/timezone` (Admin, DevOps), validated with `time.LoadLocation`. The database session stays UTC and the API keeps UTC timestamps. Only the dashboard formats times, for verification, job start/end and run history, in the system zone.

## Interfaces
- `PUT /api/products/{id}/knowledge/test-cases/{code}/verification` body `{ "verdict": "valid" | "invalid" | null, "note": "<=500 chars, optional" }`. 200 returns the item `verification`. 403 for roles without `canVerify`; 404 for unknown or removed TC; 400 for other verdicts or unknown fields.
- Test case list and detail items gain `verification: { verdict, verified_by_name, verified_at, note } | null`. The list gains two filters that combine with the existing ones: `?verified=any|valid|invalid|none` (`any` = verified by someone, valid or invalid; `none` = not verified by any human yet) and `?verified_by=<user id>` (verified by that person; implies `any`). `GET /api/products/{id}/knowledge/test-cases/verifiers` returns the distinct verifiers `[{id, name, count}]` of the product to fill the person selector.
- `GET /api/products/{id}/knowledge/test-cases/{code}/verification/history?limit=50` (default 50, max 200, any reader) returns `{history: [{id, verdict: valid|invalid|cleared, note, verified_by, verified_by_name, verified_at}], total}`, newest first. The test case detail item also has `verification_count` (number of events).
- `GET /api/settings` gains `timezone`. `PUT /api/settings/timezone` body `{ "timezone": "Asia/Jakarta" }`.

## Schema (appended to `migrations.go`, idempotent)
- `kb_case_verifications(product_id, case_code utf8mb4_bin, verdict ENUM('valid','invalid'), note VARCHAR(500), verified_by CHAR(36), verified_by_name VARCHAR(191), verified_at DATETIME, PRIMARY KEY (product_id, case_code))`.
- `kb_case_verification_events(id BIGINT AUTO_INCREMENT PK, product_id, case_code utf8mb4_bin, verdict ENUM('valid','invalid','cleared'), note VARCHAR(500), verified_by CHAR(36), verified_by_name VARCHAR(191), verified_at DATETIME, INDEX (product_id, case_code, id))`.
- `app_settings(k VARCHAR(64) PRIMARY KEY, v TEXT, updated_by CHAR(36) NULL, updated_at DATETIME)` plus `INSERT IGNORE` of `timezone = Asia/Jakarta`.

## Dashboard
- Job page: started, ended, elapsed (ticks while running) and an "Other runs of this TC" list.
- TC page: "Verification history" (events, newest first) and "Run history" (`✓ N · ✗ M`, compact rows with status icon, datetime, job link) and a verification control.
- TC tab: columns Verified (✓ / ✗ / –), Verified by (name, datetime in system zone), Action (Mark valid, Mark invalid, Clear; hidden without write rights) and filters: Verification (All, Verified by anyone, Valid, Invalid, Not verified yet) and Verified by (person selector, from the `verifiers` endpoint; disabled for Not verified yet). Both filters are kept in the URL like the existing ones.
- Epics tab: code, title, TC count, coverage bar, verified valid/invalid/total, last run pass/fail.
- Settings page: timezone selector for Admin and DevOps.

## Out of scope
Epic-level verification; notifications; GAOS push of verification (nothing sent to GAOS changes).

## Delivery
PR 1 this spec. PR 2 job detail and run history. PR 3 timezone setting. PR 4 verification. PR 5 Epics tab. Migrations run manually at deploy; deploy is a separate approval.

## Target Files
- `ai-backend/internal/db/migrations.go`
- `ai-backend/internal/api/router.go`, `case_automation.go`, `knowledge.go`, `settings.go` and their tests
- `ai-backend/internal/services/knowledge/query.go`, `coverage.go`, new `verification.go`, `products.go` (path depth)
- `ai-backend/README.md`, `ai-backend/internal/api/README.md` (contracts)
- `dashboard/src/views/JobView.tsx`, `SettingsView.tsx`
- `dashboard/src/views/knowledge/KnowledgeView.tsx`, `TestCasesTab.tsx`, new `EpicsTab.tsx`, new `JobHistory.tsx`, `api.ts`, `pages/TestCasePage.tsx`
- new `dashboard/src/lib/time.ts`; `dashboard/README.md`
- `this-qa-automation-system-playwright/` specs
