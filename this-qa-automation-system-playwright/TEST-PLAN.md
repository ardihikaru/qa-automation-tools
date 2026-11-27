# QA Automation System - Test Plan

## Overview
This document outlines the test plan for verifying the QA Automation System itself using Playwright.

## System Under Test

### Backend (REST API)
- **Base URL**: http://localhost:8080
- **Health Check**: /health

### Dashboard (Web UI)
- **Base URL**: http://localhost:3000
- **Routes**: /products, /cost, /skills, /navigation, /learning

## Test Coverage

### 1. API Tests

#### 1.1 Health & Status
- [ ] GET /health - Returns status, database connection, minimax config

#### 1.5 Skills API
- [ ] GET /api/skills - List active skills
- [ ] GET /api/skills/stats - Get skill performance statistics
- [ ] GET /api/skills/logs - Get skill execution logs

#### 1.6 Cost Analytics API
- [ ] GET /api/cost/overview - Get total cost overview
- [ ] GET /api/cost/trend - Get cost trend (30 days)
- [ ] GET /api/cost/efficiency - Get cache hit/miss stats
- [ ] GET /api/cost/by-feature - Get cost by feature

#### 1.7 Learning API
- [ ] GET /api/learning - List learning records
- [ ] POST /api/learning - Add learning record
- [ ] DELETE /api/learning - Delete learning record

### 2. Dashboard Tests

#### 2.1 Navigation
- [ ] Home page redirects to /products
- [ ] All navigation links work correctly

#### 2.5 Cost Analytics View (/cost)
- [ ] Page loads without errors
- [ ] Cost overview cards display

#### 2.7 Skills View (/skills)
- [ ] Page loads without errors
- [ ] Skills list displays

## Planned coverage added 2026-11-27 (status: planned, no spec yet)

Every item below is `planned`: it has a spec path in "Spec (to write)" but the file does not exist yet.
Design specs: `docs/designs/CASE_VERIFICATION_AND_EPIC_TAB.md`, `docs/designs/KNOWLEDGE_TRANSLATION.md`.
Tests that write data (verification, publish, timezone, language) must use a throw-away product or user and clean up
(production shares the `qa_automation` DB). Roles: Admin/DevOps/Developer/QA/QC can verify; Admin/DevOps change the timezone.

### 1.8 Job detail and per-case run history API (`tests/api/jobs.spec.ts`, `tests/api/knowledge-verification.spec.ts`)
- [ ] P-JOB-01 `GET /api/jobs/{id}` of a `case_run` job has `case_code`, `product_id` and `elapsed_ms` (>= 0; counts from `started_at`, or `created_at` if never started)
- [ ] P-JOB-02 `elapsed_ms` of a finished job ends at `completed_at`, of a running job ends at now
- [ ] P-JOB-03 `GET .../test-cases/{code}/jobs` returns `summary {total, success, failed}`; `total = success + failed`; queued and running jobs are listed but not counted
- [ ] P-JOB-04 `?limit=` defaults to 10, caps at 50; the summary covers all jobs of the case, not only the page

### 1.9 Test case verification API (`tests/api/knowledge-verification.spec.ts`)
- [ ] P-VER-01 `PUT .../test-cases/{code}/verification {verdict:'valid'}` returns 200 with `verified_by`, `verified_by_name`, `verified_at` (UTC), `note`
- [ ] P-VER-02 re-verify as `invalid` overwrites the current row; `verdict:null` clears it (response `null`)
- [ ] P-VER-03 missing `verdict` key, other verdicts, unknown fields, note over 500 chars give 400; unknown or removed case gives 404
- [ ] P-VER-04 a role without write rights (e.g. Viewer) gets 403
- [ ] P-VER-05 list and detail items carry `verification`; a sync does not wipe it
- [ ] P-VER-06 filter `verified=any|valid|invalid|none`; `verified_by=<user id>` implies `any`; `verified=none` with `verified_by` gives 400; bad value gives 400; combines with epic/status filters and paging totals
- [ ] P-VER-07 `GET .../test-cases/verifiers` lists id, name and count of current verifications of live cases
- [ ] P-VER-08 every verify, re-verify and clear appends one event: `GET .../verification/history` is newest first with verdict `valid|invalid|cleared`, `total` is the full count, `limit` default 50 max 200
- [ ] P-VER-09 `verification_count` on the detail item equals the number of events (can be above 0 while `verification` is null)
- [ ] P-VER-10 edit or delete of a draft that is `queued` or `running` for publish returns 409

### 1.10 Epics API (`tests/api/knowledge-epics.spec.ts`)
- [ ] P-EPC-01 `GET .../knowledge/epics` keeps existing fields and adds `verified_valid`, `verified_invalid`, `last_pass`, `last_fail` (integers)
- [ ] P-EPC-02 counts equal the numbers from the test case list filtered by epic and `verified`

### 1.11 System timezone API (`tests/api/settings-timezone.spec.ts`)
- [ ] P-TZ-01 `GET /api/settings` has `timezone` (default `Asia/Jakarta`)
- [ ] P-TZ-02 `PUT /api/settings/timezone` as Admin/DevOps with a valid IANA zone returns 200 and persists; restore the old value afterwards
- [ ] P-TZ-03 invalid zone gives 400; other roles get 403; no database gives 503

### 1.12 Knowledge translation and language API (`tests/api/knowledge-translation.spec.ts`)
- [ ] P-TRN-01 `GET /api/auth/me` has `user.language`; `PUT /api/auth/language {language:'id'}` returns 200, bad value 400, non-JSON 415, while impersonating 403
- [ ] P-TRN-02 `?lang=id` on knowledge lists and details adds `i18n {lang, fields {text, state}}`; `?lang=en` has no `i18n`; bad value gives 400
- [ ] P-TRN-03 `state` is `machine`, `original` or `stale`; a stale text is still returned and searched
- [ ] P-TRN-04 `q` matches English and Indonesian text on stories, test cases, features, epics and modules
- [ ] P-TRN-05 `GET .../translations/status` returns `enabled`, `model`, `running`, `lang_coverage {total, translated, stale, missing, failed}` with `translated + stale + missing = total`
- [ ] P-TRN-06 `POST .../translations/run` returns 202 with `run_id`; 409 while running; 503 when translation is disabled (read-only role gets 403)
- [ ] P-TRN-07 a knowledge sync never changes the English columns or removes translations

### 1.13 Draft translation and Publish drafts API (`tests/api/knowledge-publish.spec.ts`)
- [ ] P-PUB-01 saving a draft with `lang:'id'` stores English in the normal columns and the Indonesian original as a translation; failure of the translator gives 502 and nothing is saved; translation disabled gives 503
- [ ] P-PUB-02 `POST .../knowledge/publish {epic_code}` returns 202 with `job_id`; unknown epic 400; no automation repo 409; nothing publishable 422 with `blockers`; read-only role 403
- [ ] P-PUB-03 one active publish per product: a second request returns 409 with the running `job_id`
- [ ] P-PUB-04 `GET .../knowledge/publications` lists `id, epic_code, state (queued|running|pr_open|merged|failed), pr_url, branch, job_id, codes` newest first
- [ ] P-PUB-05 drafts with `|` or newlines are accepted and published as `\|` and `<br>`; control characters are refused
- [ ] P-PUB-06 after a merged PR and a sync, results, verifications, annotations and translations move to the repo code, the `L-` draft is removed and its path answers 404 with `moved_to` (run only against a throw-away automation repo)
- [ ] P-PUB-07 the job never pushes to the default branch; the write guard (`allow_non_automation_work`, automation role) is respected

### 1.14 Cost per vendor and model API (`tests/api/cost.spec.ts`)
- [ ] P-CST-01 `GET /api/cost/by-vendor` returns vendors (`Anthropic`, `Other`) with models, calls, errors, tokens, `cost`, `cost_idr`, averages and `share_pct`; sorted by cost; empty range gives `vendors: []`
- [ ] P-CST-02 each model has `label` (for example `claude-haiku-5-5` -> `Haiku 5.5`) and a per-feature list that includes `translation`
- [ ] P-CST-03 honours `from`, `to`, `feature`, `provider`, `model`, `status` filters like the other cost endpoints; 401 without a session
- [ ] P-CST-04 `by-model` rows have `vendor` and `model_label` and keep the old fields
- [ ] P-CST-05 the recorded model is `observed_model` when ai-manager returns it, else `model`

### 2.8 Job page (`tests/dashboard/job-detail.spec.ts`)
- [ ] P-UI-JOB-01 header shows Started, Ended and Elapsed in the system timezone with its abbreviation (for example WIB); elapsed ticks while running
- [ ] P-UI-JOB-02 "Other runs of this TC" lists the other jobs with status icon, time and a link, without the current one
- [ ] P-UI-JOB-03 a `drafts_publish` job page shows the live log and the code mapping (`was L-...`)

### 2.9 Test case page (`tests/dashboard/test-case-page.spec.ts`)
- [ ] P-UI-TC-01 "Run history" shows `check N, cross M`, compact rows with status icon, date-time in the system timezone and a job link
- [ ] P-UI-TC-02 verification control: Mark valid, Mark invalid, Clear with optional note; hidden without write rights; the badge updates without a reload
- [ ] P-UI-TC-03 "Verification history (N)" is collapsible, newest first, with name, date-time, note and a cleared entry; reloads after each action
- [ ] P-UI-TC-04 the scenario and story text areas: Shift+Enter adds a line, Enter and Ctrl/Cmd+Enter submit, the height grows to a maximum then scrolls; helper text "Press Shift+Enter for a new line"

### 2.10 Test cases tab and Epics tab (`tests/dashboard/knowledge-verification.spec.ts`)
- [ ] P-UI-KB-01 test cases tab has Verified (check, cross, dash), Verified by and Action columns; the action is hidden without write rights
- [ ] P-UI-KB-02 filters "Verification" (All, Verified by anyone, Valid, Invalid, Not verified yet) and "Verified by" (person) are kept in the URL (`?verified=`, `?verified_by=`); "Verified by" is disabled for "Not verified yet"
- [ ] P-UI-KB-03 Epics tab (`?tab=epics`) lists code, title, TC count, coverage bar, verified valid/invalid/total and last run pass/fail, with pagination
- [ ] P-UI-KB-04 tab choice and `?size=` behave like the other tabs (arrow keys, page reset)

### 2.11 Language, translation and publish UI (`tests/dashboard/knowledge-language.spec.ts`)
- [ ] P-UI-LNG-01 the EN|ID toggle in the header saves the choice, reloads the data, rolls back with an error toast on failure and ignores double clicks
- [ ] P-UI-LNG-02 in ID, lists, detail pages and the drawer show Indonesian text; the "MT" marker appears for machine translations ("MT (outdated)" for stale) and nowhere for original or missing; missing falls back to English
- [ ] P-UI-LNG-03 searching an Indonesian word and its English source word finds the same test case
- [ ] P-UI-LNG-04 the Admin-only translation status chip polls while a run is queued or running and hides itself on error
- [ ] P-UI-LNG-05 the drafts panel lists local drafts per epic; "Publish drafts" cannot be double clicked; badges Publishing, PR open (with link), Merged ("TC-... (was L-...)") and Failed (with job link)
- [ ] P-UI-LNG-06 the UI chrome stays English in both languages

### 2.12 Settings and cost page (`tests/dashboard/settings-timezone.spec.ts`, `tests/dashboard/cost.spec.ts`)
- [ ] P-UI-SET-01 Settings has a Timezone card: Admin/DevOps get a selector (Asia/Jakarta, Asia/Makassar, Asia/Jayapura, Asia/Singapore, UTC) and Save; other roles see it read-only
- [ ] P-UI-SET-02 after changing the zone, dashboard times (verification, jobs, run history) use the new abbreviation; restore WIB afterwards
- [ ] P-UI-CST-01 "By vendor & model": vendor rows expand into models with calls, tokens, USD and IDR cost, average per call and a share bar
- [ ] P-UI-CST-02 expanding a model shows its per-feature split ("Translation (knowledge, ID to EN)") and a small daily-cost trend
- [ ] P-UI-CST-03 the provider/model chart uses the vendor and model labels

### Lint and build gates (not specs)
- [ ] P-GATE-01 `cd dashboard && npm run lint && npx tsc --noEmit && npm run build` pass in CI once CI is re-enabled
- [ ] P-GATE-02 `cd ai-backend && go build -mod=mod ./... && go vet -mod=mod ./... && go test -mod=mod ./...` (DB-backed tests need `QA_TEST_MYSQL_DSN`)

## Test Strategy

### API Tests
- Use Playwright's request API for HTTP testing
- Verify response status codes
- Verify response body structure
- Test query parameters and filters
- Test error handling (404, 400, 500)

### Dashboard Tests
- Use Playwright browser automation
- Verify page loads
- Verify no console errors
- Test navigation between pages
- Verify UI elements are present

## Test Environment

### Prerequisites
- Backend running on port 8080
- Dashboard running on port 3000
- MySQL database connected (`qa_automation`)
- MiniMax AI configured

### Verification Commands
```bash
# Check backend health
curl http://localhost:8080/health

# Check dashboard
curl http://localhost:3000
```

## Risk Assessment

1. **API Dependency**: Tests depend on backend being running
2. **Data State**: Some tests may depend on existing data in database
3. **Timing**: Dashboard tests may need wait conditions for API calls

## Exit Criteria

- All critical API endpoints return 200
- All dashboard routes load without errors
- No console errors in dashboard
- Response times within acceptable limits