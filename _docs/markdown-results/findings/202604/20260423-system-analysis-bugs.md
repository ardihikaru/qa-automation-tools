# System Analysis Bug Report (2026-04-23)

Role: Senior System Analyst  
Environment: Local backend on `http://localhost:18080` with Neon connected

---

## Bug 1: `POST /api/learning` fails on update path for existing record

### Endpoint
- `POST /api/learning`

### Payload
```json
{
  "intent": "bug-repro-20260423",
  "action": "click",
  "page": "/qa",
  "success": true
}
```

### Reproduction
1. First call creates record successfully.
2. Re-send same payload to trigger update branch.

### Responses
- Initial create:
  - HTTP `201`
  - body: `{"action":"click","created_at":"2026-04-23T05:08:49.694705Z","id":13,"intent":"bug-repro-20260423","page":"/qa","success":true}`
- Retry #1:
  - HTTP `500`
  - body: `ERROR: could not determine data type of parameter $1 (SQLSTATE 42P18)`
- Retry #2:
  - HTTP `500`
  - body: `FATAL: prepared statement name is already in use (SQLSTATE 08P01)`
- Retry #3:
  - HTTP `500`
  - body: `ERROR: could not determine data type of parameter $1 (SQLSTATE 42P18)`

### Status
- Persisted after 3 retries.

### Suspected root cause
- Wrong SQL placeholder numbering / argument mismatch in update query inside `handleLearning`.

### Detail task for BE
- Fix SQL binding, add regression tests for create->update->update flow.

---

## Bug 2: `POST /api/issues/status` returns success for non-existing issue ID

### Endpoint
- `POST /api/issues/status`

### Payload
```json
{
  "id": 999999,
  "is_fixed": true
}
```

### Responses (3 retries)
- Retry #1: HTTP `200`, body `{"id":999999,"is_fixed":true}`
- Retry #2: HTTP `200`, body `{"id":999999,"is_fixed":true}`
- Retry #3: HTTP `200`, body `{"id":999999,"is_fixed":true}`

### Expected
- `404` when target issue does not exist.

### Status
- Persisted after 3 retries.

### Suspected root cause
- No `RowsAffected()` validation after `UPDATE`.

### Detail task for BE
- Return `404` when no updated row; add tests for non-existing ID.

---

## Bug 3: `POST /api/issues/notes` returns success for non-existing issue ID

### Endpoint
- `POST /api/issues/notes`

### Payload
```json
{
  "id": 999999,
  "notes": "retry-note"
}
```

### Responses (3 retries)
- Retry #1: HTTP `200`, body `{"id":999999,"notes":"retry-note"}`
- Retry #2: HTTP `200`, body `{"id":999999,"notes":"retry-note"}`
- Retry #3: HTTP `200`, body `{"id":999999,"notes":"retry-note"}`

### Expected
- `404` when target issue does not exist.

### Status
- Persisted after 3 retries.

### Suspected root cause
- No `RowsAffected()` validation after `UPDATE`.

### Detail task for BE
- Return `404` when no updated row; add tests for non-existing ID.
