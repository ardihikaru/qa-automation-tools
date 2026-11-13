# Unmapped user stories — for manual assessment

> **Generated** by `npm run catalog:report`. Do not edit here — the decision is recorded on the story line of the epic file.

User stories of the [test catalog](../test-catalog/README.md) that name **no** feature of the
[module catalogue](README.md) and say why with `(unmapped: <why>)` on the story line.

To resolve one, edit the story line in its epic file and run `npm run catalog:report`:

1. **Map it** — the behaviour belongs to an existing feature: replace `(unmapped: …)` with the id, e.g. `(D10.05)`.
2. **Add a feature** — the product has the behaviour but the catalogue misses it: add a feature row (next free id, release
   status, `Status as of`) to the module file, then map the story to it.
3. **Keep unmapped** — the behaviour is out of the product scope (e.g. purely operational): leave it, sharpen the reason.
4. **Drop the story** — only with the reviewer: set its cases to `wont` with a `not-needed:` reason first (AGENTS.md §2a).

Total: **8** stories, 49 test cases.

|   # | Story                 | Epic                                                | Cases | Why no feature (current)                                                               | Decision (fill in the epic file) |
| --: | --------------------- | --------------------------------------------------- | ----: | -------------------------------------------------------------------------------------- | -------------------------------- |
|   1 | [US-RB-01](#us-rb-01) | [RB](../test-catalog/his-backend/report-builder.md) |    16 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |
|   2 | [US-RB-02](#us-rb-02) | [RB](../test-catalog/his-backend/report-builder.md) |     6 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |
|   3 | [US-RB-03](#us-rb-03) | [RB](../test-catalog/his-backend/report-builder.md) |     9 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |
|   4 | [US-RB-04](#us-rb-04) | [RB](../test-catalog/his-backend/report-builder.md) |     7 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |
|   5 | [US-RB-05](#us-rb-05) | [RB](../test-catalog/his-backend/report-builder.md) |     4 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |
|   6 | [US-RB-06](#us-rb-06) | [RB](../test-catalog/his-backend/report-builder.md) |     2 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |
|   7 | [US-RB-07](#us-rb-07) | [RB](../test-catalog/his-backend/report-builder.md) |     2 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |
|   8 | [US-RB-08](#us-rb-08) | [RB](../test-catalog/his-backend/report-builder.md) |     3 | the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet | map / add feature / keep / drop  |

## US-RB-01

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As a report author, I want unsafe or malformed SQL and tampered encrypted submissions rejected with a stable code, so that only analyzed, operational-table queries can become reports.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (16):** 15 automated, 0 planned, 1 blocked / other — `TC-RB-001`, `TC-RB-002`, `TC-RB-003`, `TC-RB-004`, `TC-RB-005`, `TC-RB-006`, `TC-RB-007`, `TC-RB-008`, `TC-RB-009`, `TC-RB-010`, `TC-RB-011`, `TC-RB-012`, `TC-RB-013`, `TC-RB-014`, `TC-RB-015`, `TC-RB-016`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).

## US-RB-02

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As a report author, I want a preview of at most 20 rows right after verification, so that I can check the report's shape before saving it.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (6):** 5 automated, 0 planned, 1 blocked / other — `TC-RB-020`, `TC-RB-021`, `TC-RB-022`, `TC-RB-023`, `TC-RB-024`, `TC-RB-025`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).

## US-RB-03

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As a report author and a second administrator, I want create, update, run-group and two-eyes validation to behave consistently, so that nobody can run an unreviewed report.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (9):** 9 automated, 0 planned, 0 blocked / other — `TC-RB-030`, `TC-RB-031`, `TC-RB-032`, `TC-RB-033`, `TC-RB-034`, `TC-RB-035`, `TC-RB-036`, `TC-RB-037`, `TC-RB-038`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).

## US-RB-04

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As a runner in a run group, I want to generate a validated report and download it through an authenticated route, so that only the requesting user can fetch the file.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (7):** 5 automated, 0 planned, 2 blocked / other — `TC-RB-040`, `TC-RB-041`, `TC-RB-042`, `TC-RB-043`, `TC-RB-044`, `TC-RB-045`, `TC-RB-046`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).

## US-RB-05

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As an operator, I want a global execution kill switch and an audit trail that holds no SQL, parameters or row data, so that report execution can be stopped and reviewed.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (4):** 1 automated, 0 planned, 3 blocked / other — `TC-RB-050`, `TC-RB-051`, `TC-RB-052`, `TC-RB-053`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).

## US-RB-06

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As an administrator, I want a list page of report definitions, so that I can find and manage them.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (2):** 2 automated, 0 planned, 0 blocked / other — `TC-RB-060`, `TC-RB-061`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).

## US-RB-07

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As a report author, I want a builder page with an Excel-like preview, so that I can design a report in the browser.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (2):** 2 automated, 0 planned, 0 blocked / other — `TC-RB-062`, `TC-RB-063`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).

## US-RB-08

- **Epic:** [RB](../test-catalog/his-backend/report-builder.md)
- **Story:** As a runner, I want a run page that generates and downloads the report, and tells me clearly when a report is not available to me.
- **Why unmapped:** the SQL-driven report builder, HISv3-FEATURE-00243, is not in the module catalogue yet
- **Test cases (3):** 2 automated, 0 planned, 1 blocked / other — `TC-RB-064`, `TC-RB-065`, `TC-RB-066`
- **Candidate features to check:** see the module of the same repo in [the module catalogue](README.md#modules).
