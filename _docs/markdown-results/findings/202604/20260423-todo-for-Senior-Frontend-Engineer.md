# TODO for Senior Frontend Engineer (Dashboard)

Date: 2026-04-23  
Owner: Senior Frontend Engineer (`dashboard`)

## P0 - Must Fix First

### 1) Implement server-side search/filter/pagination for `/docs`
- File: `dashboard/src/views/DocsView.tsx`
- Current gap:
  - Fetch-all + client-side filtering; triggers on every input change.
- Tasks:
  - Add query-state model (`search`, `role`, `sort`, `page`, `limit`).
  - Trigger fetch on Enter (and optional explicit Search button).
  - Render pagination controls based on backend metadata.
- Acceptance:
  - Large docs dataset remains responsive; no full re-filter in client.

### 2) Wire `/qa` quick action buttons to backend jobs
- File: `dashboard/src/views/QAInsights.tsx`
- Current gap:
  - `Run QA Test` and `Generate Docs` are visual-only.
- Tasks:
  - Connect to job endpoints (`/api/jobs/qa/run`, `/api/jobs/docs/generate`).
  - Show request state (queued/running/success/error).
  - Show last run result metadata.
- Acceptance:
  - Clicking each button starts a real backend process.

### 3) Complete `/skills` server-driven table behavior
- File: `dashboard/src/views/SkillsView.tsx`
- Current gap:
  - Search/filter only in memory; no pagination.
- Tasks:
  - Query backend with `search`, `severity`, `passed`, `page`, `limit`.
  - Add page navigation and total count.
  - Keep tab-specific query params in URL for shareable state.
- Acceptance:
  - `skills`, `stats`, and `logs` tabs support scalable server-side filtering.

## P1 - High

### 4) Normalize `/issues` search trigger behavior
- Files:
  - `dashboard/src/views/FixTracking.tsx` (already debounced)
  - `dashboard/src/views/IssueDebugger.tsx` (still per-keystroke fetch)
- Tasks:
  - Apply same strategy across both pages:
    - Enter-to-search or debounce + explicit apply.
  - Avoid uncontrolled request spikes.
- Acceptance:
  - No endpoint called on every keystroke without delay/control.

### 5) Replace full-page spinner with skeleton-first rendering
- Target views:
  - `DocsView`, `QAInsights`, `IssueDebugger`, `SkillsView`, `LearningProgressView`, `CostAnalytics`, `NavigationGraphView`
- Tasks:
  - Render page layout immediately.
  - Show local skeleton placeholders for cards/table/chart sections.
- Acceptance:
  - No blank/single-spinner screen while loading data.

### 6) Improve data quality fallback rendering
- Target views: `FixTracking`, `IssueDebugger`
- Evidence:
  - API currently returns rows with blank `page/action/severity`.
- Tasks:
  - Render meaningful fallback (`Unknown page`, `Unknown action`, `Unknown severity`).
  - Add badge for malformed/legacy issue row.
- Acceptance:
  - UI remains clear even when backend data is partially malformed.

## P2 - Medium

### 7) Add action audit UI for quick actions
- File: `dashboard/src/views/QAInsights.tsx`
- Tasks:
  - Display latest job executions (type, status, duration, started_at).
  - Add retry button for failed jobs.
- Acceptance:
  - Users can trace and re-run QA/docs actions from dashboard.
