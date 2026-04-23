# TODO for Senior Backend Engineer - Missing `404` Contract Hardening

Date: 2026-04-23  
Owner: Senior Backend Engineer (`ai-backend`)  
Note: file path follows requested naming, but content is backend-focused.

## Objective

Standardize API behavior so write operations that target non-existing resources return `404` (not false-positive `200`), with clear error messages and integration-test coverage.

## Priority P0 (Must Complete First)

### 1) Lock `404` contract for already-fixed issue endpoints

Endpoints:
- `POST /api/issues/status`
- `POST /api/issues/notes`

TODO:
- Keep `RowsAffected()` guard mandatory after `UPDATE`.
- Return:
  - `404` when `RowsAffected() == 0`
  - `200` only when row was actually updated.
- Ensure response format consistency (`{"error":"..."}` preferred for errors).

Acceptance Criteria:
- `id=999999` returns `404` in both endpoints across 3 retries.
- Existing ID still returns `200` and persisted change is visible from `GET /api/issues`.

---

### 2) Audit all write endpoints for missing-resource behavior

Scope (minimum):
- `DELETE /api/roles?id=...`
- `DELETE /api/learning?id=...`
- `DELETE /api/goal-events?id=...`
- `DELETE /api/ai-cache?hash=...`
- `DELETE /api/embedding-cache?hash=...`
- any `POST`/`PUT` update-style handlers with `WHERE id = ...`

TODO:
- For ID/hash-scoped mutations, enforce:
  - `404` if target does not exist
  - `200/204` only if mutation affected resource.
- Avoid silent success on no-op writes.

Acceptance Criteria:
- Every targeted endpoint has explicit affected-row/delete-count check.
- Endpoint behavior documented in one matrix table (method, path, not-found status code).

---

### 3) Unify error contract for all not-found responses

TODO:
- Replace plain text errors with JSON object:
  - `{"error":"resource not found"}` or endpoint-specific equivalent.
- Keep `Content-Type: application/json` for error responses.
- Standardize wording across handlers.

Acceptance Criteria:
- Not-found responses are machine-consumable JSON in all audited endpoints.
- Existing frontend consumers remain compatible (verify key screens).

---

### 4) Add integration regression suite for `404` behavior

TODO:
- Create/extend integration tests per endpoint pair:
  - existing resource -> success
  - non-existing resource -> `404`
- Include repeated retries for previously flaky endpoints (`issues/status`, `issues/notes`).

Acceptance Criteria:
- `go test ./internal/api -run 'Integration|404|IssueUpdate' -count=1 -v` passes.
- Test names clearly describe contract (e.g. `TestXxxReturns404ForMissingTarget`).

## Priority P1 (High)

### 5) Add helper utility to reduce repeated `RowsAffected` code

TODO:
- Introduce utility/helper function for mutation result validation:
  - input: command tag/result + resource name
  - output: standardized `404` or nil.
- Refactor update/delete handlers to use helper.

Acceptance Criteria:
- Reduced duplication in router handlers.
- All refactored handlers retain same API contract.

---

### 6) Add API contract documentation for not-found semantics

TODO:
- Update backend API docs with explicit not-found behavior for each write endpoint.
- Include example request/response pairs for `404`.

Acceptance Criteria:
- Backend docs include a dedicated “Error Semantics” section.
- `404` behavior is discoverable without reading source code.

---

### 7) Improve observability for missing-target operations

TODO:
- Add structured log line when mutation returns `404`:
  - endpoint
  - target id/hash
  - request correlation id (if available)
- Track count metric for not-found writes.

Acceptance Criteria:
- Logs can answer: “Which endpoint most frequently hits missing target?”
- Metric is visible in backend monitoring/export path.

## Priority P2 (Medium)

### 8) Transactional safety and race-condition checks

TODO:
- For update-then-read patterns, ensure consistent behavior under concurrent requests.
- Evaluate whether certain endpoints should use `RETURNING` instead of separate read/write phases.

Acceptance Criteria:
- No false success in concurrent update tests.
- Documented rationale for each endpoint’s chosen SQL pattern.

---

### 9) Contract tests against real Neon data

TODO:
- Run curl-based smoke scripts against Neon-backed environment:
  - verify `404` on missing targets
  - verify success on existing targets
- Preserve report artifact under findings directory.

Acceptance Criteria:
- Stored execution evidence includes timestamps and HTTP codes.
- Reproducible command list available for QA/system analyst.

## Implementation Checklist

- [ ] Build endpoint audit matrix (write endpoints only).
- [ ] Add/confirm `RowsAffected()` or equivalent checks everywhere needed.
- [ ] Normalize error JSON response contract.
- [ ] Add missing integration tests.
- [ ] Run focused + full integration test pass.
- [ ] Run live curl verification against running server.
- [ ] Publish implementation summary with evidence.

## Suggested Deliverables

1. Code changes in `ai-backend/internal/api/router.go` (+ helper file if introduced).
2. Integration tests in `ai-backend/internal/api/*_integration_test.go`.
3. Updated API documentation section on error semantics.
4. Verification report including:
   - health result
   - `404` endpoint evidence
   - test command outputs
   - known non-scope failures (if any).

## Definition of Done

- No write endpoint returns success when target resource does not exist.
- Missing-target behavior is consistently `404` with JSON error body.
- Integration tests enforce this behavior and pass in Neon-backed run.
- Documentation and evidence report are updated.
