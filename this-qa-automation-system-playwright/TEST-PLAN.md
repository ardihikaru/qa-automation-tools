# QA Automation System - Test Plan

## Overview
This document outlines the test plan for verifying the QA Automation System itself using Playwright.

## System Under Test

### Backend (REST API)
- **Base URL**: http://localhost:8080
- **Health Check**: /health

### Dashboard (Web UI)
- **Base URL**: http://localhost:3000
- **Routes**: /docs, /qa, /issues, /cost, /fixes, /skills

## Test Coverage

### 1. API Tests

#### 1.1 Health & Status
- [ ] GET /health - Returns status, database connection, minimax config
- [ ] GET /api/analytics - Returns system analytics

#### 1.2 Flows API
- [ ] GET /api/flows - List all flows
- [ ] POST /api/flows - Create new flow

#### 1.3 Roles API
- [ ] GET /api/roles - List all roles
- [ ] POST /api/roles - Create new role

#### 1.4 Issues API
- [ ] GET /api/issues - List issues with pagination
- [ ] POST /api/issues - Create new issue
- [ ] POST /api/issues/status - Update issue fix status
- [ ] POST /api/issues/notes - Add notes to issue

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

#### 1.8 Search API
- [ ] GET /api/search - Semantic search

#### 1.9 Cache APIs
- [ ] GET /api/ai-cache - Get AI cache
- [ ] POST /api/ai-cache - Store AI response
- [ ] GET /api/embedding-cache - Get embedding cache

### 2. Dashboard Tests

#### 2.1 Navigation
- [ ] Home page redirects to /docs
- [ ] All navigation links work correctly

#### 2.2 Docs View (/docs)
- [ ] Page loads without errors
- [ ] Documentation list displays

#### 2.3 QA Insights View (/qa)
- [ ] Page loads without errors
- [ ] Analytics cards display

#### 2.4 Issues View (/issues)
- [ ] Page loads without errors
- [ ] Issue list displays

#### 2.5 Cost Analytics View (/cost)
- [ ] Page loads without errors
- [ ] Cost overview cards display

#### 2.6 Fix Tracking View (/fixes)
- [ ] Page loads without errors
- [ ] Issues table displays

#### 2.7 Skills View (/skills)
- [ ] Page loads without errors
- [ ] Skills list displays

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
- Neon database connected
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