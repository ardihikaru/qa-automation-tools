# TODO for Dashboard Developer - Senior Frontend Developer

## Role
**Senior Frontend Developer** - React Dashboard

## Project
**AI-Powered QA and System Documentation Platform** - Dashboard (React + Vite + Tailwind)

## Context

You will be implementing the React Dashboard for visualizing QA automation activities. The dashboard uses React with Vite, Tailwind CSS, and connects to the AI-Backend APIs.

## Current Status
- **Overall Progress**: 30% complete
- **Completed**: FixTracking.tsx (partial), basic routing
- **Not Implemented**: CostAnalytics.tsx, QAInsights.tsx, IssueDebugger.tsx, DocsView.tsx

## Reference Files
- `proposed-system-architecture-v1-20260416.md` - Full architecture
- `dashboard/src/views/` - Existing views
- `dashboard/src/App.tsx` - Main app component
- `ai-backend/internal/api/router.go` - API endpoints

---

## Task 1: Cost Analytics View Implementation (P0 - Critical)

### Description
Complete implementation of CostAnalytics.tsx view to display AI cost analytics.

### API Endpoints Used
- `GET /api/cost/overview` - Total requests, tokens, cost, cache hit rate
- `GET /api/cost/trend` - Daily cost for last 30 days
- `GET /api/cost/efficiency` - Cache hit/miss stats
- `GET /api/cost/by-feature` - Cost breakdown by feature

### Implementation Requirements

**1. Overview Cards**
- Total Requests (number)
- Total Tokens (number)
- Total Cost (currency)
- Cache Hit Rate (percentage with color indicator)

**2. Cost Trend Chart**
- Line chart showing daily cost over 30 days
- X-axis: Date
- Y-axis: Cost in currency
- Use a charting library (recharts or chart.js)

**3. Efficiency Metrics**
- Visual bar/pie chart showing hits vs misses
- Percentage display

**4. Cost by Feature**
- Bar chart showing top 10 features by cost
- Table view with feature name and cost

### Component Structure

```tsx
// src/views/CostAnalytics.tsx
import { useEffect, useState } from 'react';

interface CostOverview {
  total_requests: number;
  total_tokens: number;
  total_cost: number;
  cache_hit_rate: number;
}

interface CostTrend {
  date: string;
  cost: number;
}

interface CostByFeature {
  feature: string;
  cost: number;
}

export default function CostAnalytics() {
  const [overview, setOverview] = useState<CostOverview | null>(null);
  const [trends, setTrends] = useState<CostTrend[]>([]);
  const [efficiency, setEfficiency] = useState({ hit: 0, miss: 0 });
  const [byFeature, setByFeature] = useState<CostByFeature[]>([]);
  
  // Fetch data from APIs
  // Render components
  
  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold mb-6">Cost Analytics</h1>
      
      {/* Overview Cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <CostCard title="Total Requests" value={overview?.total_requests} />
        <CostCard title="Total Tokens" value={overview?.total_tokens} />
        <CostCard title="Total Cost" value={`$${overview?.total_cost?.toFixed(4)}`} />
        <CacheHitRate value={overview?.cache_hit_rate} />
      </div>
      
      {/* Charts */}
      <div className="grid grid-cols-2 gap-6">
        <CostTrendChart data={trends} />
        <EfficiencyChart data={efficiency} />
      </div>
      
      {/* Cost by Feature */}
      <div className="mt-6">
        <CostByFeatureTable data={byFeature} />
      </div>
    </div>
  );
}
```

### Dependencies to Install
```bash
npm install recharts
# or
npm install chart.js react-chartjs-2
```

### Expected Time: 2 days

---

## Task 2: Fix Tracking View Enhancement (P0 - Critical)

### Description
Enhance FixTracking.tsx with full functionality for tracking issue fixes.

### API Endpoints Used
- `GET /api/issues` - List all issues with filters
- `POST /api/issues/status` - Mark issue as fixed/unfixed
- `POST /api/issues/notes` - Add notes to issue

### Implementation Requirements

**1. Filter Bar**
- Filter by: All / Fixed / Open
- Filter by category: All / Frontend / Backend / Unknown
- Sort by: Date (newest/oldest), Severity (high/low)
- Search by: page, action, error text

**2. Issue List Table**
- Columns: ID, Page, Action, Error Type, Category, Severity, Fixed, Created
- Row click to expand details
- Checkbox to mark as fixed
- Notes button to view/add notes

**3. Issue Detail Panel**
- Full error text
- DOM snapshot (if available)
- Suggestions list
- Fix status toggle
- Notes textarea

**4. Pagination**
- 20 items per page
- Page navigation

### Component Structure

```tsx
// src/views/FixTracking.tsx
import { useState, useEffect } from 'react';

interface Issue {
  id: number;
  page: string;
  action: string;
  error_type: string;
  error_text: string;
  category: string;
  severity: string;
  is_fixed: boolean;
  notes: string;
  created_at: string;
  suggestions: Suggestion[];
}

export default function FixTracking() {
  const [issues, setIssues] = useState<Issue[]>([]);
  const [filter, setFilter] = useState({ fixed: 'all', category: 'all' });
  const [selectedIssue, setSelectedIssue] = useState<Issue | null>(null);
  
  // Fetch issues
  // Handle filter changes
  // Handle status update
  // Handle notes update
  
  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold mb-6">Fix Tracking</h1>
      
      {/* Filter Bar */}
      <FilterBar filter={filter} onFilterChange={setFilter} />
      
      {/* Issues Table */}
      <IssueTable 
        issues={issues} 
        onSelect={setSelectedIssue}
        onStatusChange={updateStatus}
      />
      
      {/* Issue Detail */}
      {selectedIssue && (
        <IssueDetail 
          issue={selectedIssue}
          onClose={() => setSelectedIssue(null)}
          onNotesUpdate={updateNotes}
        />
      )}
    </div>
  );
}
```

### Expected Time: 2 days

---

## Task 3: QA Insights View Implementation (P1 - High)

### Description
Implement QAInsights.tsx as the main overview dashboard.

### API Endpoints Used
- `GET /api/analytics` - Overall analytics
- `GET /api/skills` - List of skills
- `GET /api/cost/overview` - Cost summary

### Implementation Requirements

**1. Key Metrics Cards**
- Total Flows (from analytics)
- Open Issues (from issues count)
- Active Skills (from skills)
- Total Cost (from cost overview)

**2. Recent Activity Section**
- Recent flows created
- Recent issues detected
- Recent fixes applied

**3. Skills Summary**
- List of all skills
- Pass/fail count per skill
- Last execution time

**4. Quick Actions**
- Run new documentation
- Run new QA test
- View all issues

### Component Structure

```tsx
// src/views/QAInsights.tsx
export default function QAInsights() {
  const [metrics, setMetrics] = useState({
    flows: 0,
    issues: { total: 0, open: 0, fixed: 0 },
    skills: 0,
    cost: 0
  });
  
  // Fetch metrics
  // Display dashboard
  
  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold mb-6">QA Insights</h1>
      
      {/* Metrics Cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <MetricCard title="Total Flows" value={metrics.flows} />
        <MetricCard title="Open Issues" value={metrics.issues.open} />
        <MetricCard title="Active Skills" value={metrics.skills} />
        <MetricCard title="Total Cost" value={`$${metrics.cost.toFixed(2)}`} />
      </div>
      
      {/* Activity & Skills */}
      <div className="grid grid-cols-2 gap-6">
        <RecentActivity />
        <SkillsSummary />
      </div>
      
      {/* Quick Actions */}
      <QuickActions />
    </div>
  );
}
```

### Expected Time: 2 days

---

## Task 4: Issue Debugger View Implementation (P1 - High)

### Description
Implement IssueDebugger.tsx for detailed issue analysis.

### API Endpoints Used
- `GET /api/issues` - Get issue details
- `GET /api/issues?target=frontend` - Filter by frontend
- `GET /api/issues?target=backend` - Filter by backend

### Implementation Requirements

**1. Issue Detail View**
- Full page URL
- Action performed
- Complete error text
- Error type and category
- DOM snapshot (if available)
- Timestamp

**2. Suggestions Panel**
- List of fix suggestions
- Confidence score for each
- Target (frontend/backend)
- File path (if available)
- Diff preview (if available)

**3. Classification Info**
- How issue was classified
- Confidence level
- Evidence (error code, stack trace)

### Component Structure

```tsx
// src/views/IssueDebugger.tsx
export default function IssueDebugger() {
  const [issue, setIssue] = useState<Issue | null>(null);
  
  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold mb-6">Issue Debugger</h1>
      
      {issue ? (
        <div className="grid grid-cols-2 gap-6">
          <IssueDetails issue={issue} />
          <SuggestionsPanel issue={issue} />
        </div>
      ) : (
        <IssueList onSelect={setIssue} />
      )}
    </div>
  );
}
```

### Expected Time: 2 days

---

## Task 5: Documentation View Implementation (P2 - Medium)

### Description
Implement DocsView.tsx for viewing generated documentation.

### API Endpoints Used
- `GET /api/docs` - List documentation
- `GET /api/flows` - Get flow details

### Implementation Requirements

**1. Documentation List**
- Search documentation
- Filter by role
- Filter by date
- Sort by name/date

**2. Documentation Viewer**
- Flow title and description
- Steps with screenshots
- Video player (if available)
- Markdown content

### Expected Time: 2 days

---

## Task 6: Skill Execution UI (P2 - Medium)

### Description
Add ability to run skills manually from dashboard.

### API Endpoints Used
- `GET /api/skills` - List skills
- `GET /api/skill-logs` - View execution results

### Implementation Requirements

**1. Skills List**
- Show all skills with status
- Enable/disable toggle
- Run button

**2. Execution Results**
- View past executions
- Filter by skill, date
- Success/failure stats

### Expected Time: 1 day

---

## Task 7: Navigation Graph Visualization (P3 - Low)

### Description
Add navigation graph visualization.

### API Endpoints Used
- `GET /api/search` - Query navigation edges

### Implementation Requirements
- Visual graph showing page connections
- Interactive nodes
- Path highlighting

### Expected Time: 2 days

---

## Task 8: Learning Progress Visualization (P3 - Low)

### Description
Show learning cache progress in dashboard.

### API Endpoints Used
- `GET /api/learning` - Query learning records

### Implementation Requirements
- Success scores over time
- Usage statistics
- Top learned actions

### Expected Time: 1 day

---

## Implementation Guidelines

### Styling
- Use Tailwind CSS classes
- Follow existing component patterns
- Maintain responsive design
- Use consistent color scheme

### API Calls
- Use fetch or axios
- Handle loading states
- Handle error states
- Use proper TypeScript types

### Component Structure
```
dashboard/src/
├── views/
│   ├── CostAnalytics.tsx     # Implement
│   ├── FixTracking.tsx        # Enhance
│   ├── QAInsights.tsx         # Implement
│   ├── IssueDebugger.tsx      # Implement
│   └── DocsView.tsx          # Implement
├── components/
│   ├── CostCard.tsx          # New
│   ├── FilterBar.tsx          # New
│   ├── IssueTable.tsx         # New
│   └── ...
└── hooks/
    └── useApi.ts              # New - API wrapper
```

### State Management
- Use React hooks (useState, useEffect)
- Consider useContext for global state
- Keep components focused

---

## Dependencies to Install

```bash
# For charts
npm install recharts

# For date handling (if needed)
npm install date-fns

# For HTTP requests
# Already using fetch, no need to install
```

---

## Notes

1. All components should be responsive
2. Use proper loading indicators
3. Handle API errors gracefully
4. Follow accessibility guidelines
5. Test on multiple screen sizes

---

*Document Version: 1.0*
*Created: 2026-04-17*
*Assigned To: Senior Frontend Developer*