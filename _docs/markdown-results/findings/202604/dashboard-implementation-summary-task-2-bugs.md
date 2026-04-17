# Dashboard Task 2 - Backend/API Bug and Blocking Issues

## Date
- 2026-04-17

## Context
Task 2 frontend implementation depends on backend APIs:
- `GET /api/issues`
- `POST /api/issues/status`
- `POST /api/issues/notes`
- `GET /api/search` (required check in instruction)

In this environment, backend service at `http://localhost:8080` is not reachable.

## Blocking Issue 1

### API
- `GET http://localhost:8080/api/issues?limit=2`

### Payload / Query
- Query string: `limit=2`

### Retry Attempts (up to 3x)
1. attempt=1 -> `curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server`
2. attempt=2 -> `curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server`
3. attempt=3 -> `curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server`

### Response
- No HTTP response received (connection refused)

### Impact
- Cannot validate live behavior for:
  - list data rendering with real backend data
  - pagination metadata contract at runtime
  - status/notes mutations against backend

## Blocking Issue 2

### API
- `GET http://localhost:8080/api/search?q=login`

### Payload / Query
- Query string: `q=login`

### Retry Attempts (up to 3x)
1. attempt=1 -> `curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server`
2. attempt=2 -> `curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server`
3. attempt=3 -> `curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server`

### Response
- No HTTP response received (connection refused)

### Impact
- Cannot verify required search endpoint connectivity from current environment.

## Additional Connectivity Check

### API
- `GET http://localhost:8080/health`

### Result
- `curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server`

## Frontend Subtasks Completed Despite Blocker

- Implemented all Task 2 frontend requirements in `FixTracking.tsx`:
  - fixed/open/all filter
  - category filter
  - debounced search filter
  - sort options
  - issue detail panel with full error text + suggestions
  - pagination UI + API pagination handling
  - status and notes mutation handlers

## Pending Due to Backend Unavailable

- End-to-end runtime verification with live backend data
- Confirmation of `/api/issues/status` and `/api/issues/notes` mutation responses in this environment
