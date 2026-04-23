# Backend Part 2 Implementation Summary - Missing `404` Contract Report

Date: 2026-04-23  
Role: Senior Software Engineer (`ai-backend`)

## Scope Reference

Implemented based on:
- `proposed-system-architecture-v1-20260416.md`
- `_docs/markdown-results/findings/202604/20260423-todo-for-Senior-Frontend-Engineer-missing-404.md`

## Phase 1 - Plan

1. Audit all mutation endpoints that target ID/hash-scoped resources.
2. Enforce `404` on no-op mutations (`RowsAffected() == 0`).
3. Keep existing issue update fixes and normalize missing-target errors to JSON.
4. Add integration regression tests for all newly enforced `404` paths.
5. Verify with Neon-backed integration tests and live curl checks including health endpoint.

## Phase 2 - Execution

## 1) Code Changes

### A. Added shared JSON error writer
- File: `ai-backend/internal/api/router.go`
- Added helper:
  - `writeJSONError(w, status, message)`
- Purpose:
  - standardize missing-target responses as JSON (`{"error":"..."}`)

### B. Enforced `404` for missing targets on scoped deletes
- File: `ai-backend/internal/api/router.go`
- Updated endpoints:
  - `DELETE /api/roles?id=...`
  - `DELETE /api/learning?id=...`
  - `DELETE /api/goal-events?id=...`
  - `DELETE /api/ai-cache?hash=...`
  - `DELETE /api/embedding-cache?hash=...`
- Behavior:
  - use `RowsAffected()` after `DELETE`
  - return `404` JSON error when zero rows affected

### C. Kept and normalized issue update `404` responses
- File: `ai-backend/internal/api/router.go`
- Endpoints:
  - `POST /api/issues/status`
  - `POST /api/issues/notes`
- Behavior:
  - still `RowsAffected()` guarded
  - now returns JSON error body for `404`

### D. Added integration regression tests
- File: `ai-backend/internal/api/crud_integration_test.go`
- Added test:
  - `TestDeleteEndpointsReturn404ForMissingTargetsIntegration`
- Covered cases:
  - missing role id
  - missing learning id
  - missing goal-event id
  - missing ai-cache hash
  - missing embedding-cache hash
- Assertion includes status `404` and JSON `error` field.

## 2) Endpoint Contract Matrix (Not-Found Semantics)

| Method | Endpoint | Missing Target Behavior |
|---|---|---|
| POST | `/api/issues/status` | `404` + `{"error":"issue not found"}` |
| POST | `/api/issues/notes` | `404` + `{"error":"issue not found"}` |
| DELETE | `/api/roles?id={id}` | `404` + `{"error":"role not found"}` |
| DELETE | `/api/learning?id={id}` | `404` + `{"error":"learning record not found"}` |
| DELETE | `/api/goal-events?id={id}` | `404` + `{"error":"goal event not found"}` |
| DELETE | `/api/ai-cache?hash={hash}` | `404` + `{"error":"ai cache entry not found"}` |
| DELETE | `/api/embedding-cache?hash={hash}` | `404` + `{"error":"embedding cache entry not found"}` |

Note:
- Non-scoped “clear all” operations (`DELETE /api/ai-cache` and `DELETE /api/embedding-cache` without hash) remain success operations.

## 3) Verification Results

### A. Focused Neon-backed integration tests (updated endpoints)
Command:
- `GOCACHE=/tmp/go-build-cache go test ./internal/api -run 'Test(DeleteEndpointsReturn404ForMissingTargetsIntegration|IssueUpdateReturns404ForMissingIDIntegration|RolesCRUDIntegration|LearningCRUDIntegration|GoalEventsCRUDIntegration|AICacheCRUDIntegration)' -count=1 -v`

Result:
- PASS

### B. Live API checks
Command run with server on `:18080` + curl checks.

Health:
- `GET /health` => `{"status":"ok","database":"connected","minimax":"configured"}`

Missing-target checks:
- `POST /api/issues/status` (`id=999999`) retry x3 => all `404`
- `POST /api/issues/notes` (`id=999999`) retry x3 => all `404`
- `DELETE /api/roles?id=99999999` => `404`
- `DELETE /api/learning?id=99999999` => `404`
- `DELETE /api/goal-events?id=99999999` => `404`
- `DELETE /api/ai-cache?hash=missing-hash-404` => `404`
- `DELETE /api/embedding-cache?hash=missing-hash-404` => `404`

All returned JSON error body.

### C. Full package test status (informational)
Command:
- `GOCACHE=/tmp/go-build-cache go test ./internal/api -count=1`

Result:
- FAIL due to pre-existing unrelated test:
  - `TestEmbeddingCacheCRUDIntegration`
  - error: `invalid input syntax for type vector`

This failure is outside the missing-`404` scope implemented in this report.

## Files Modified

- `ai-backend/internal/api/router.go`
- `ai-backend/internal/api/crud_integration_test.go`

