# Report Builder (his-backend `reportbuilder` + his-frontend `/report-builder`)

**Goal:** An Administrator can build SQL-driven reports from verified-safe, aggregate-only queries,
validate them with a second author, and let designated runner groups generate and download xlsx files,
while the system keeps patient-identifying data out of every result and keeps a full audit trail.

**E2E suite prefix:** `tests/api/graphql/his-backend/reportbuilder/` and `tests/web/his-frontend/report-builder/`
**Last updated:** 2026-11-12

---

## User stories

- **US-RB-01** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As a report author, I want unsafe or malformed SQL and tampered encrypted submissions rejected with a stable code, so that only analyzed, operational-table queries can become reports.
- **US-RB-02** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As a report author, I want a preview of at most 20 rows right after verification, so that I can check the report's shape before saving it.
- **US-RB-03** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As a report author and a second administrator, I want create, update, run-group and two-eyes validation to behave consistently, so that nobody can run an unreviewed report.
- **US-RB-04** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As a runner in a run group, I want to generate a validated report and download it through an authenticated route, so that only the requesting user can fetch the file.
- **US-RB-05** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As an operator, I want a global execution kill switch and an audit trail that holds no SQL, parameters or row data, so that report execution can be stopped and reviewed.
- **US-RB-06** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As an administrator, I want a list page of report definitions, so that I can find and manage them.
- **US-RB-07** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As a report author, I want a builder page with an Excel-like preview, so that I can design a report in the browser.
- **US-RB-08** (unmapped: the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet) — As a runner, I want a run page that generates and downloads the report, and tells me clearly when a report is not available to me.
- **US-RB-09** (unmapped: HISv3-FEATURE-00243 sample report with enum param, not in module catalogue yet) — As a runner, I want a sample aggregate report with an enum parameter (bed status) so that I can filter the bed recap by status and trust that invalid parameter values are rejected before execution.

---

## Test cases

### verify-guard.spec.ts (TC-RB-001–016)

| ☑ | TC-RB-001 | US-RB-01 | empty SQL → SQL_REJECTED + SQL_EMPTY | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-002 | US-RB-01 | SQL comment → SQL_REJECTED + SQL_COMMENT | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-003 | US-RB-01 | multi-statement → SQL_REJECTED + SQL_MULTI_STATEMENT | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-004 | US-RB-01 | non-SELECT statement → SQL_REJECTED + SQL_NON_SELECT | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-005 | US-RB-01 | SELECT * → SQL_REJECTED + SQL_SELECT_STAR | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-006 | US-RB-01 | blocked table (user) → SQL_TABLE_BLOCKED or SQL_DENIED_KEYWORD | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-007 | US-RB-01 | report_definition prefix → SQL_TABLE_BLOCKED | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-008 | US-RB-01 | hex literal → SQL_HEX_LITERAL | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-009 | US-RB-01 | INTO clause → SQL_INTO_CLAUSE | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-010 | US-RB-01 | tampered cipher byte → TOKEN_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-011 | US-RB-01 | foreign key (fingerprint mismatch) → TOKEN_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-012 | US-RB-01 | spliced chunk from different SQL → TOKEN_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-013 | US-RB-01 | wrong label prefix → TOKEN_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-014 | US-RB-01 | stale nonce → TOKEN_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☑ | TC-RB-015 | US-RB-01 | valid operational SQL → OK + verificationToken | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/verify-guard.spec.ts` | |
| ☐ | TC-RB-016 | US-RB-01 | patient table → SQL_TABLE_UNCLASSIFIED / PATIENT_DATA_DISABLED | - | GQL | P2 | blocked | | deferred:patient table not in TABLE-CLASSES.md yet |

### preview.spec.ts (TC-RB-020–025)

| ☑ | TC-RB-020 | US-RB-02 | author previews valid SQL → OK + sampleRows | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/preview.spec.ts` | |
| ☑ | TC-RB-021 | US-RB-02 | preview without token → PARAM_INVALID or TOKEN_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/preview.spec.ts` | |
| ☑ | TC-RB-022 | US-RB-02 | fabricated token → TOKEN_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/preview.spec.ts` | |
| ☑ | TC-RB-023 | US-RB-02 | cross-user token replay → TOKEN_INVALID or FORBIDDEN | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/preview.spec.ts` | |
| ☐ | TC-RB-024 | US-RB-02 | execution disabled → EXECUTION_DISABLED | - | GQL | P2 | blocked | | deferred:kill-switch toggle unsafe on shared staging |
| ☑ | TC-RB-025 | US-RB-02 | preview result has valid column metadata | + | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/preview.spec.ts` | |

### definition.spec.ts (TC-RB-030–038)

| ☑ | TC-RB-030 | US-RB-03 | author creates report → reportID present, unvalidated | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-031 | US-RB-03 | author2 validates → validationStatus becomes validated | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-032 | US-RB-03 | author cannot self-validate → FORBIDDEN | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-033 | US-RB-03 | stale version on update → VERSION_CONFLICT | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-034 | US-RB-03 | outsider cannot delete → FORBIDDEN or NOT_FOUND | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-035 | US-RB-03 | reportGetSqlForEdit for owner → OK + sql + version | + | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-036 | US-RB-03 | outsider cannot read SQL for edit → error | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-037 | US-RB-03 | validated report can be enabled by its author | + | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |
| ☑ | TC-RB-038 | US-RB-03 | setRunGroups stores acg_id correctly | + | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/definition.spec.ts` | |

### generate-download.spec.ts (TC-RB-040–046)

| ☑ | TC-RB-040 | US-RB-04 | runner initializes validated enabled report with run group | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/generate-download.spec.ts` | |
| ☑ | TC-RB-041 | US-RB-04 | generation reaches ready state with rowCount >= 0 | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/generate-download.spec.ts` | |
| ☑ | TC-RB-042 | US-RB-04 | download returns 200 with spreadsheet content-type | + | E2E | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/generate-download.spec.ts` | |
| ☑ | TC-RB-043 | US-RB-04 | outsider cannot download → 401/403/404 | - | E2E | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/generate-download.spec.ts` | |
| ☐ | TC-RB-044 | US-RB-04 | generated file expires after 24 h → 503 ErrDownloadUnavailable | - | GQL | P2 | blocked | | deferred:24h Redis TTL not testable in automated suite |
| ☑ | TC-RB-045 | US-RB-04 | disabled report cannot be initialized by runner → error | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/generate-download.spec.ts` | |
| ☐ | TC-RB-046 | US-RB-04 | RESULT_TOO_LARGE when row count exceeds cap | - | GQL | P2 | blocked | | blocked:needs REPORTBUILDER_MAX_ROWS low-cap environment config |

### kill-switch-audit.spec.ts (TC-RB-050–053)

| ☐ | TC-RB-050 | US-RB-05 | SuperAdmin disables execution globally → EXECUTION_DISABLED | - | GQL | P2 | blocked | | blocked:needs SuperAdmin role and isolated non-shared environment |
| ☐ | TC-RB-051 | US-RB-05 | SuperAdmin re-enables execution → preview succeeds again | + | GQL | P2 | blocked | | blocked:needs SuperAdmin role and isolated non-shared environment |
| ☑ | TC-RB-052 | US-RB-05 | audit row is written for create action | + | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/kill-switch-audit.spec.ts` | |
| ☐ | TC-RB-053 | US-RB-05 | kill-switch on shared staging disrupts all users | - | GQL | P2 | wont | | unsafe:toggling kill-switch on shared staging would disrupt all report users |

### list.spec.ts (TC-RB-060–061)

| ☑ | TC-RB-060 | US-RB-06 | list page loads and shows report-builder-list container | + | WEB | P1 | automated | `tests/web/his-frontend/report-builder/list.spec.ts` | |
| ☑ | TC-RB-061 | US-RB-06 | search input filters the list | + | WEB | P1 | automated | `tests/web/his-frontend/report-builder/list.spec.ts` | |

### builder-preview.spec.ts (TC-RB-062–063)

| ☑ | TC-RB-062 | US-RB-07 | builder page loads with verify panel visible | + | WEB | P1 | automated | `tests/web/his-frontend/report-builder/builder-preview.spec.ts` | |
| ☑ | TC-RB-063 | US-RB-07 | edit page loads for an existing report (run-group picker visible) | + | WEB | P1 | automated | `tests/web/his-frontend/report-builder/builder-preview.spec.ts` | |

### sample-report.spec.ts (TC-RB-070–073)

| ☑ | TC-RB-070 | US-RB-09 | sample report preview returns 4 columns, ≤20 rows, layout header and subtitle | + | GQL | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/sample-report.spec.ts` | |
| ☑ | TC-RB-071 | US-RB-09 | full flow with status=normal: create → validate → initialize → download; header rows, data row count, and TOTAL row match DB oracle | + | E2E | P0 | automated | `tests/api/graphql/his-backend/reportbuilder/sample-report.spec.ts` | |
| ☑ | TC-RB-072 | US-RB-09 | status=disable produces a different bed total than normal and matches the DB | - | E2E | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/sample-report.spec.ts` | |
| ☑ | TC-RB-073 | US-RB-09 | enum param value outside the declared set is rejected with PARAM_INVALID | - | GQL | P1 | automated | `tests/api/graphql/his-backend/reportbuilder/sample-report.spec.ts` | |

### run.spec.ts (TC-RB-064–066)

| ☑ | TC-RB-064 | US-RB-08 | run page loads the form for an enabled report | + | WEB | P1 | automated | `tests/web/his-frontend/report-builder/run.spec.ts` | |
| ☑ | TC-RB-065 | US-RB-08 | a report the runner may not run (unvalidated, no run group) shows not-found on the run page | - | WEB | P1 | automated | `tests/web/his-frontend/report-builder/run.spec.ts` | |
| ☐ | TC-RB-066 | US-RB-08 | outsider sees access-denied on run page | - | WEB | P2 | blocked | | deferred:FE routing for outsider access-denied not yet implemented |

---

## Frontend locator contract

Page objects use `getByTestId` locators. These `data-testid` attributes must be added to his-frontend before the web specs can run against a real backend:

| Attribute                 | Page / component                     |
| ------------------------- | ------------------------------------ |
| `report-builder-list`     | ReportBuilderListPage root container |
| `report-builder-search`   | Search input field                   |
| `report-verify-panel`     | ReportBuilderPage verify panel       |
| `report-run-group-picker` | ReportBuilderPage run-group picker   |
| `report-run-form`         | ReportRunPage run parameter form     |

---

## Deferred / blocked cases

| TC            | Blocked reason                                 | Unblock condition                                     |
| ------------- | ---------------------------------------------- | ----------------------------------------------------- |
| TC-RB-016     | patient table not in TABLE-CLASSES.md          | Fill TABLE-CLASSES.md patient classification          |
| TC-RB-024     | Kill-switch toggle unsafe on shared staging    | Isolated non-shared environment                       |
| TC-RB-044     | 24 h Redis TTL not testable in automated suite | Long-running integration harness                      |
| TC-RB-046     | Needs low-cap maxRows env                      | Local backend with REPORTBUILDER_MAX_ROWS set low     |
| TC-RB-050/051 | Needs SuperAdmin + isolated env                | REPORTBUILDER_KILLSWITCH_OVERRIDE=true + isolated env |
| TC-RB-053     | Wont:unsafe on shared staging                  | Intentionally not automated                           |
| TC-RB-066     | FE routing for outsider not yet implemented    | Add FE access-denied route for /report-builder/run    |

---

## Running these specs

```bash
# API specs only (no browser)
npx playwright test tests/api/graphql/his-backend/reportbuilder/ --project=api-graphql

# Web specs (requires his-frontend URL in env)
npx playwright test tests/web/his-frontend/report-builder/ --project="web-his-frontend-chromium"

# Offline contract guard — validates all 15 operations against the SDL; no backend needed.
# Default SDL: ../../_worktrees/his-backend-report-builder/pkg/graph/reportbuilder/graphqls/reportbuilder.graphqls
node scripts/check-reportbuilder-contract.mjs

# Custom SDL path:
REPORTBUILDER_SDL=/path/to/reportbuilder.graphqls node scripts/check-reportbuilder-contract.mjs
```

The contract guard reads `src/api/graphql/his-backend/reportbuilder/reportbuilder.operations.ts` with a
plain regex (no TypeScript compilation needed) and validates every operation document string against the
SDL using `graphql` (already a devDependency). Run it after any SDL change or any edit to operations.ts.

### Request budgets (per user) and the local stack

The backend limits every author per minute and per day. Verify: 20/min and 200/day. Preview: 10/min and 100/day.
Nonce issuance: 30/min. Set-execution (kill switch): 5/min. A limit answers `RATE_LIMITED`:

- **per minute**: the kit (`ReportKit.untilNotRateLimited`) waits it out with `expect.poll` (90 s budget).
- **per day**: the message says "batas harian"; the kit fails **at once** with instructions instead of polling,
  because waiting does not help. Repeated full runs on one author consume the daily budget quickly
  (a full API run is about 50 verify calls). On the **local isolated Redis only**, delete the
  `reportbuilder:rl:*` keys; never on a shared Redis.

A full run needs the isolated local stack (backend graphql + restapi, consumer, RabbitMQ, Redis, MinIO) because
generation is asynchronous and the download route is served by the REST API: set `HIS_BACKEND_GRAPHQL_URL`
(`.../api/v1`) **and** `HIS_BACKEND_REST_URL` (`.../api/v1` of the REST port). Download specs skip without the latter.
Setup and teardown: `services-impl/202611/report-builder/STEP-E2E-AUTOMATION.md` (his-v3-ai).

### Role accounts

`author` is `HIS_AUTH_*` (SuperAdmin/Administrator). `author2` is a second admin (the two-eyes validator).
`runner` is a **non-admin** account that belongs to the group in `REPORTBUILDER_RUNNER_ACG_ID` (used as the run group).
`outsider` is a **non-admin** account; specs that need a user who is neither the owner nor an author use it, and
TC-043 uses `author2` as the non-owner downloader because `runner` is the generator.
