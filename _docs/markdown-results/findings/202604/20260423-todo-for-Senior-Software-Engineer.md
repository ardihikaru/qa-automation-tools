# TODO for Senior Software Engineer (Backend)

Date: 2026-04-23  
Owner: Senior Software Engineer (`ai-backend`)

## P0 - Must Fix First

### 1) Fix `POST /api/learning` existing-record update bug
- File: `ai-backend/internal/api/router.go` (`handleLearning`, update branch)
- Problem:
  - Existing record updates fail with `500` (`SQLSTATE 42P18`) due wrong SQL placeholders/args.
- Tasks:
  - Correct SQL placeholder usage and argument binding.
  - Add regression tests:
    1. Create learning record.
    2. Re-post same payload (should update, not fail).
    3. Re-post with `success=false` (score should decrease).
- Acceptance:
  - 3 consecutive retries on same payload return `200` (no `500`).

### 2) Enforce affected-row validation on issue update endpoints
- Files: `ai-backend/internal/api/router.go`
  - `handleIssueStatus`
  - `handleIssueNotes`
- Problem:
  - Non-existing `id` still returns `200` and success JSON.
- Tasks:
  - Check `RowsAffected()` after `UPDATE`.
  - Return `404` with clear error when `id` not found.
  - Add tests for:
    - existing ID -> `200`
    - non-existing ID -> `404`
- Acceptance:
  - `id=999999` consistently returns `404` for both endpoints.

### 3) Add server-side query contract for skills list
- Files:
  - `ai-backend/internal/api/router.go` (`handleSkills`)
- Problem:
  - `GET /api/skills?limit=1&page=1` still returns all rows.
- Tasks:
  - Support `search`, `severity`, `is_active`, `page`, `limit`, `sort_by`.
  - Return pagination metadata like `/api/issues`.
  - Update integration tests.
- Acceptance:
  - `limit=1` returns exactly 1 row and valid pagination object.

## P1 - High

### 4) Add server-side docs query support
- File: `ai-backend/internal/api/router.go` (`handleDocs`)
- Tasks:
  - Add query params: `search`, `product`, `page`, `limit`, `sort_by`.
  - Avoid returning full dataset by default for large docs set.
- Acceptance:
  - Frontend can fetch docs/flows incrementally with page navigation.

### 5) Data cleanup + guardrails for `raw_issues`
- DB evidence:
  - `8` rows with blank `page/action` and blank `severity`.
- Tasks:
  - Create migration or one-off cleanup script for existing invalid rows.
  - Add stricter validation path for all issue ingestion entry points.
  - Optional: check constraints for trimmed non-empty text where appropriate.
- Acceptance:
  - No new blank `page/action/severity` rows can be inserted through API.

### 6) Add backend job endpoints for QA actions
- Needed by frontend quick actions:
  - `POST /api/jobs/qa/run`
  - `POST /api/jobs/docs/generate`
  - `GET /api/jobs/:id`
- Tasks:
  - Define minimal async job model + status polling contract.
- Acceptance:
  - QAInsights buttons can trigger real backend workflow and poll status.

## P2 - Medium

### 7) Improve consistency across schemas (`public`, `qa`, `ai`, `logging`)
- Tasks:
  - Decide canonical schema strategy.
  - Make SQL schema explicit where needed to avoid ambiguity.
- Acceptance:
  - Same endpoint behavior across environments regardless of `search_path`.
