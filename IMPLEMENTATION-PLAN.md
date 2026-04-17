# Implementation Plan: Local API Testing with Playwright

## Phase 1: Analysis & Planning

### API Endpoints Identified (from router.go):
1. `/health` - Health check (no auth required)
2. `/api/flows` - CRUD for flows (GET/POST)
3. `/api/docs` - Documentation (GET)
4. `/api/videos` - Videos (GET)
5. `/api/issues` - Issues (GET/POST)
6. `/api/skills` - Skills (GET)
7. `/api/skills/logs` - Skill logs (GET/POST)
8. `/api/skills/stats` - Skill stats (GET)
9. `/api/search` - Search (GET)
10. `/api/cost/overview` - Cost overview (GET)
11. `/api/cost/trend` - Cost trend (GET)
12. `/api/cost/efficiency` - Cost efficiency (GET)
13. `/api/cost/by-feature` - Cost by feature (GET)
14. `/api/analytics` - Analytics (GET)
15. `/api/learning` - Learning (GET/POST)
16. `/api/learning/navigation` - Learning navigation (GET/POST)
17. `/api/learning/sequences` - Learning sequences (GET/POST)
18. `/api/learning/context` - Learning context (GET/POST)
19. `/api/learning/stats` - Learning stats (GET)
20. `/api/ai-cache` - AI cache (GET/POST/DELETE)
21. `/api/embedding-cache` - Embedding cache (GET/POST/DELETE)
22. `/api/goal-events` - Goal events (GET/POST/DELETE)

### Authentication Mechanism:
- API_SECRET_KEY is defined in config but NOT enforced in router.go
- CORS enabled for all origins
- Bearer token authentication available in config but not implemented

### Risks & Edge Cases:
- Database may not be connected (returns 503)
- AI service may not be available (returns 503)
- Search endpoint needs valid `q` parameter
- NULL scanning issues in endpoints with LEFT JOIN

## Phase 2: Execution Plan

### Step 1: Start ai-backend server in background
- Build the server
- Run it locally on port 8080
- Verify it's accessible

### Step 2: Create TEST-SCENARIO.md.example
- List all API endpoints to test
- Define test scenarios (happy path, edge cases, error cases)
- Include authentication tests

### Step 3: Create TEST-RESULTS.md.example
- Template for capturing test results
- Include status codes, response times, error details

### Step 4: Run Playwright tests
- Test all endpoints
- Verify responses
- Compare with previous issues in TEST-RESULTS.md

### Step 5: Update README files
- Document HTTP file usage
- Document Playwright test results
- Update API documentation

---
**Status**: Phase 1 Complete - Ready for Phase 2 Execution

## Extra Task 2: NULL Scanning Issue Fix (2026-04-17)

### Issue Reported
```
Scan error: can't scan into dest[8]: cannot scan NULL into *string
```

### Root Cause
The `/api/skills/logs` endpoint had a LEFT JOIN query that could return NULL for skill_name when no matching skill was found.

### Solution Applied
- Added COALESCE to the SQL query in `router.go` line ~644
- Changed: `s.name as skill_name`
- To: `COALESCE(s.name, 'Unknown') as skill_name`

### Testing Completed
- All 20+ API endpoints tested successfully
- NULL scanning issue resolved
- Database connection working properly

---
**Status**: ✅ COMPLETED
