# Senior QA Engineer - Extracted Content

> Extracted from PDF chunks in `_docs/reference-pdf/`
> Role: Senior QA Engineer + Senior QA

---

## Role Context

This document contains content extracted from the AI-Powered QA and System Documentation Platform PDF, filtered for relevance to Senior QA Engineer responsibilities.

---

## 1. Core QA Skills (Repetitive Issues to Detect)

The system automatically detects these 4 core QA issues:

### Skill 1: Max Length Validation

| Aspect | Detail |
|--------|--------|
| **Issue** | Textarea allows huge input → backend crash |
| **Detection** | Check if textarea has `maxLength` attribute |
| **Suggestion** | Add client-side max length attribute |
| **Root Cause** | QA forgot to verify max char it can work |
| **Impact** | Error on the backend side when text size is too big |

### Skill 2: Required Field Validation

| Aspect | Detail |
|--------|--------|
| **Issue** | '*' marked but accepts empty string |
| **Detection** | Verify form validates required fields properly |
| **Suggestion** | Add frontend validation for required fields |
| **Root Cause** | Form field marked with '*' but validation not implemented |
| **Impact** | Invalid data enters the system |

### Skill 3: Data Persistence

| Aspect | Detail |
|--------|--------|
| **Issue** | Form data disappears on reload (localStorage not used) |
| **Detection** | Check if form uses localStorage/sessionStorage |
| **Suggestion** | Implement data persistence |
| **Scenario** | After creating form, user wants to view - data is gone (empty form), but exists in DB |
| **Note** | Sometimes it's a backend bug, sometimes frontend persistence issue |

### Skill 4: Multi-tab Save

| Aspect | Detail |
|--------|--------|
| **Issue** | Tab data lost when moving between tabs without saving |
| **Detection** | Check if multi-tab forms save each tab individually |
| **Suggestion** | Auto-save or warn user before leaving tab |
| **Scenario** | User fills 3 tabs, saves only after last tab → data in first and second tab not saved |

---

## 2. Skill Tracking System

### Database Schema for Skills

```sql
-- Track repetitive issues
CREATE TABLE skill_issues (
  id SERIAL PRIMARY KEY,
  skill_id VARCHAR(50),      -- 'max_length', 'required_field', 'persistence', 'multi_tab_save'
  page VARCHAR(255),
  action TEXT,
  occurrence_count INT DEFAULT 1,
  created_at TIMESTAMP DEFAULT NOW()
);
```

### Skill Counter Logic

- Log every issue detected during Playwright runs
- Quantify by scenario ID
- Track how many repetitive issues happen
- Store in Neon database

---

## 3. Goal Detection System

### Multi-Signal Detection

| Signal Type | Example | Weight |
|-------------|---------|--------|
| UI text | "Success", "Created" | 0.4 |
| URL change | /invoice/123 | 0.3 |
| DOM element | .invoice-success | 0.3 |
| Network | 200 OK | 0.3 |
| Data existence | Row appears in table | 0.3 |

### Confidence Scoring

```typescript
confidence = (ui_signal * 0.4) + (network_signal * 0.3) + (data_signal * 0.3)
```

### Goal Detection Signals

**UI Signals:**
- Success toast/snackbar
- Modal appears/disappears
- New element appears (table row, badge)
- Form reset

**Navigation Signals:**
- URL changes
- Query params update

**Network Signals:**
- HTTP 200 OK
- API response success

### Goal Success Rate Tracking

| Goal | Success Rate |
|------|-------------|
| create_invoice | 87% |
| create_user | 92% |

---

## 4. Issue Management System

### Database Schema

```sql
-- Main issues table
CREATE TABLE raw_issues (
  id SERIAL PRIMARY KEY,
  page VARCHAR(255),
  action TEXT,
  category VARCHAR(50),      -- 'frontend' or 'backend'
  error_text TEXT,
  is_fixed BOOLEAN DEFAULT FALSE,
  notes TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);

-- Issue suggestions (child table)
CREATE TABLE issue_suggestions (
  id SERIAL PRIMARY KEY,
  issue_id INT,
  suggestion TEXT,
  confidence FLOAT,           -- 0.0 to 1.0
  target VARCHAR(50),         -- 'frontend' or 'backend'
  file_path TEXT,
  diff TEXT
);
```

### Smart Classification (Override AI)

Before calling AI for classification, use heuristics:

```typescript
if (strings.Contains(issue.ErrorText, "500") {
  category = "backend"
}
if (strings.Contains(issue.ErrorText, "TypeError") {
  category = "frontend"
}
```

**Rule:**
- 500 errors → backend
- JavaScript errors (TypeError, ReferenceError) → frontend

---

## 5. Auto-Fix Suggestions System

### Multi-Suggestion Approach

- 1 issue → multiple suggestions
- Each suggestion has confidence score
- Classify as frontend/backend
- Provide file path and diff where applicable

### Suggestion Example

```json
{
  "suggestions": [
    {
      "text": "Add server-side validation for input length",
      "target": "backend",
      "confidence": 0.9
    },
    {
      "text": "Add client-side max length attribute",
      "target": "frontend",
      "confidence": 0.7
    }
  ]
}
```

---

## 6. Fix Tracking Workflow

### QA Dashboard Features

| Feature | Description |
|---------|-------------|
| ✅ Checkbox | Mark issue as Fixed |
| 📝 Textarea | Add QA notes |
| 🎯 Filter | All / Fixed / Open |

### API Endpoints

```go
// Update Fix Status
func UpdateIssueStatus(w http.ResponseWriter, r *http.Request) {
  var payload struct {
    ID      int  `json:"id"`
    IsFixed bool `json:"is_fixed"`
  }
  json.NewDecoder(r.Body).Decode(&payload)
  db.Exec(`
    UPDATE raw_issues
    SET is_fixed = $1
    WHERE id = $2
  `, payload.IsFixed, payload.ID)
}

// Update Notes
func UpdateIssueNotes(w http.ResponseWriter, r *http.Request) {
  var payload struct {
    ID    int    `json:"id"`
    Notes string `json:"notes"`
  }
  json.NewDecoder(r.Body).Decode(&payload)
  db.Exec(`
    UPDATE raw_issues
    SET notes = $1
    WHERE id = $2
  `, payload.Notes, payload.ID)
}
```

### Query Issues

```sql
SELECT 
  id,
  page,
  action,
  category,
  is_fixed,
  notes
FROM raw_issues
ORDER BY created_at DESC;
```

---

## 7. Learning Cache for QA

### 3-Level Learning Progression

| Level | Capability | Benefit |
|-------|------------|---------|
| Level 1 | Hash cache (exact match) | 60-90% savings |
| Level 2 | Embedding reuse (similar match) | 50-80% savings |
| Level 3 | Learning cache (context-aware) | 95% total savings |

### What QA Needs from Learning Cache

| Field | Purpose |
|-------|---------|
| `intent` | What the user was trying to do |
| `embedding` | Semantic representation |
| `success_score` | How often this action succeeds |
| `usage_count` | How often this is used |

### Success Score Tracking

```sql
-- Track success rate per action
ALTER TABLE learning_cache 
ADD COLUMN success_score FLOAT DEFAULT 0.5,
ADD COLUMN usage_count INT DEFAULT 0;
```

### Decay Logic

- 95% decay after 7 days of inactivity
- Recent successes weighted higher
- Old failures gradually forgotten

---

## 8. Cost Dashboard for QA

### Metrics QA Should Track

| Metric | Description |
|--------|-------------|
| Total Requests | Total AI requests made |
| Total Tokens | Token usage |
| Total Cost | Estimated cost in USD |
| Cache Hit Rate | Percentage of cached responses |
| Cost per Feature | Cost breakdown by feature |
| Daily Trends | Cost over time |

### API Endpoints

```
GET /api/cost/overview    - Total requests, tokens, cost
GET /api/cost/trend       - Daily cost trends
GET /api/cost/efficiency  - Cache hit rate
GET /api/cost/by-feature  - Cost per feature
```

---

## 9. QA Workflow Summary

### Human-in-the-Loop Process

1. **Playwright runs** → Explores system autonomously
2. **Issues detected** → Skills identify problems
3. **Suggestions generated** → AI provides fix recommendations
4. **QA reviews** → Human validates suggestions
5. **Mark as Fixed** → QA checks the checkbox
6. **Add Notes** → QA documents the fix
7. **Filter** → View All / Fixed / Open issues

### Final System Capability

> "You built: 🤖 Autonomous QA → AI Root Cause Analyzer → AI Code Assistant"

> "Human-in-the-loop: QA marks fixed + adds notes"

---

## 10. Project Separation for QA Focus

| Project | Responsibility | QA Focus |
|---------|---------------|----------|
| `playwright-docs/` | Product understanding | Documentation |
| `playwright-qa/` | Bug discovery | Issue detection |
| `ai-backend/` | Intelligence + storage | Learning system |

---

## Key Insights for QA Engineer

### What This System Does

1. **Autonomous Exploration** - Playwright explores like a real user
2. **Skill-Based Testing** - Detects 4 core repetitive issues
3. **Goal Detection** - Identifies if user succeeded or failed
4. **Learning** - Improves over time (Day 1: random → Day 30: near-human)
5. **Suggestions** - AI provides fix recommendations
6. **Tracking** - QA marks issues as fixed with notes

### QA Checklist

- [ ] Review detected issues
- [ ] Classify issues (frontend/backend)
- [ ] Validate AI suggestions
- [ ] Mark issues as fixed
- [ ] Add QA notes
- [ ] Filter and track progress
- [ ] Analyze trends (cost, success rate)

---

*Document Version: 1.0*
*Extracted from: Playwright-for-System-Documentation.pdf (128 pages)*
*Role: Senior QA Engineer + Senior QA*