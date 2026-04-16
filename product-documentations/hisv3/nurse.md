# HIS V3 Staging Documentation - Nurse

## Role: Nurse
**Credential**: KG2024003

## Exploration Summary

| Page | URL Path | Status | Notes |
|------|----------|--------|-------|
| Login | /auth/login | ✅ Success | Successfully logged in |
| Nurse Dashboard | /nurse/queue | ✅ Working | Queue management dashboard |
| Patient Vitals | /nurse/vitals | ✅ Working | Vital signs recording interface |

## Key Findings

### 1. Login Flow
- **Status**: Successful
- **Process**: 
  1. Navigate to /auth/login
  2. Enter username (KG2024003)
  3. Enter password (Bismillah1!)
  4. Click submit button
  5. Successfully redirected to nurse dashboard

### 2. Nurse Dashboard
- **Status**: Working
- **Features visible**:
  - Patient queue management
  - Quick patient access
  - Nursing tasks
  - Vital signs monitoring

### 3. Patient Vitals
- **Status**: Working
- **Features**:
  - Vital signs recording
  - Patient vital history
  - Recording interface

## Screenshots Location
`playwright-docs/output/screenshots/stag-app-hisv3.sismedika.online/nurse/`

## Test Date
2025-04-16

## Notes
- Nurse role has access to queue and vitals management
- Dashboard shows patient queue information
- Vitals recording interface is functional