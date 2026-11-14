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