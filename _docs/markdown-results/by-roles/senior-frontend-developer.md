# Senior Frontend Developer - Extracted Content

> Extracted from PDF chunks in `_docs/reference-pdf/`
> Role: Senior Frontend Developer + QA

---

## Role Context

This document contains content extracted from the AI-Powered QA and System Documentation Platform PDF, filtered for relevance to Senior Frontend Developer responsibilities.

---

## 1. Project Architecture (Frontend)

### Final Tech Stack

| Layer | Technology |
|-------|------------|
| Dashboard | React |
| Backend | Golang |
| Database | Neon (Postgres + pgvector) |
| Automation | Playwright |
| AI | Minimax |

### Project Structure

```
ai-system/
├── backend/                  # Golang
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── api/
│   │   ├── db/
│   │   ├── ai/
│   │   ├── explorer/
│   │   └── analytics/
│   └── migrations/
│
├── explorer/                 # Playwright (Node)
│   ├── flows/
│   ├── autonomous/
│   ├── runner.ts
│   └── utils.ts
│
├── dashboard/                # React
│   ├── src/
│   └── package.json
│
├── config/
│   ├── credentials.yaml
│   └── scenarios.yaml
│
├── output/
└── docs/
```

---

## 2. React Dashboard - Cost Overview

### Installation

```bash
npm install recharts
```

### Cost Overview Cards Component

```typescript
function Overview({ data }) {
  return (
    <div className="grid grid-cols-3 gap-4">
      <div>Total Requests: {data.total_requests}</div>
      <div>Total Tokens: {data.total_tokens}</div>
      <div>Total Cost: ${data.total_cost.toFixed(4)}</div>
    </div>
  );
}
```

### Daily Cost Chart

```typescript
import { LineChart, Line, XAxis, YAxis, Tooltip } from "recharts";

function DailyChart({ data }) {
  return (
    <LineChart width={500} height={300} data={data}>
      <XAxis dataKey="date" />
      <YAxis />
      <Tooltip />
      <Line dataKey="cost" />
    </LineChart>
  );
}
```

### Cache Stats Component

```typescript
function CacheStats({ data }) {
  return (
    <div>
      <p>Cache Hit: {data.hit}</p>
      <p>Cache Miss: {data.miss}</p>
    </div>
  );
}
```

---

## 3. Issue Tracking Dashboard

### Issue Card Component

```typescript
function IssueCard({ issue }) {
  const [checked, setChecked] = useState(issue.is_fixed);
  const [notes, setNotes] = useState(issue.notes || "");

  const toggleFix = async () => {
    const newVal = !checked;
    setChecked(newVal);
    await fetch("/api/issues/status", {
      method: "POST",
      body: JSON.stringify({
        id: issue.id,
        is_fixed: newVal
      })
    });
  };

  const saveNotes = async () => {
    await fetch("/api/issues/notes", {
      method: "POST",
      body: JSON.stringify({
        id: issue.id,
        notes
      })
    });
  };

  return (
    <div className="border p-4 mb-4 rounded-xl shadow">
      <h3>{issue.page}</h3>
      <p>Action: {issue.action}</p>
      <p>Category: {issue.category}</p>
      <div>
        {issue.suggestions.map((s: any) => (
          <div key={s.id} className="mt-2 p-2 bg-gray-100">
            <p>{s.suggestion}</p>
            <small>
              {s.target} | confidence: {s.confidence}
            </small>
          </div>
        ))}
      </div>
    </div>
  );
}
```

### Filter Toggle Component

```typescript
<select onChange={(e) => setTarget(e.target.value)}>
  <option value="">All</option>
  <option value="frontend">Frontend</option>
  <option value="backend">Backend</option>
</select>
```

---

## 4. Frontend QA Skills (Issues to Detect)

The system automatically detects these frontend issues:

### Skill 1: Max Length Validation

- **Issue**: Textarea allows huge input → backend crash
- **Detection**: Check if textarea has `maxLength` attribute
- **Suggestion**: Add client-side max length attribute

### Skill 2: Required Field Validation

- **Issue**: '*' marked but accepts empty string
- **Detection**: Verify form validates required fields properly
- **Suggestion**: Add frontend validation for required fields

### Skill 3: Data Persistence

- **Issue**: Form data disappears on reload (localStorage not used)
- **Detection**: Check if form uses localStorage/sessionStorage
- **Suggestion**: Implement data persistence

### Skill 4: Multi-tab Save

- **Issue**: Tab data lost when moving between tabs without saving
- **Detection**: Check if multi-tab forms save each tab individually
- **Suggestion**: Auto-save or warn user before leaving tab

---

## 5. Smart Classification Logic (Frontend/Backend)

```typescript
if (strings.Contains(issue.ErrorText, "500") {
  category = "backend"
}
// JavaScript errors → frontend
if (strings.Contains(issue.ErrorText, "TypeError") {
  category = "frontend"
}
```

---

## 6. API Endpoints for Frontend

### Get Issues with Suggestions

```go
func GetIssues(w http.ResponseWriter, r *http.Request) {
  rows, _ := db.Query(`
    SELECT 
      i.id, i.page, i.action, i.category,
      s.id, s.suggestion, s.confidence, s.target
    FROM raw_issues i
    LEFT JOIN issue_suggestions s ON i.id = s.issue_id
    ORDER BY i.created_at DESC
  `)
}
```

### Update Issue Status

```go
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
```

### Update Issue Notes

```go
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

---

## 7. Database Schema (Frontend Relevant)

### Raw Issues Table

```sql
ALTER TABLE raw_issues
ADD COLUMN is_fixed BOOLEAN DEFAULT FALSE,
ADD COLUMN notes TEXT;
```

### Issue Suggestions Table

```sql
CREATE TABLE issue_suggestions (
  id SERIAL PRIMARY KEY,
  issue_id INT,
  suggestion TEXT,
  confidence FLOAT,
  target VARCHAR(50),  -- 'frontend' or 'backend'
  file_path TEXT,
  diff TEXT
);
```

---

## 8. Goal Detection (Frontend Signals)

### UI Signals to Detect

| Signal | Example |
|--------|---------|
| Toast/Snackbar | "Success", "Created" |
| Modal | Modal appears/disappears |
| New Element | Table row, badge appears |
| Form Reset | Form clears after submit |

### Confidence Scoring

```typescript
confidence = (ui_signal * 0.4) + (network_signal * 0.3) + (data_signal * 0.3)
```

---

## 9. Guided Explorer - Frontend Integration

### Scoring Algorithm

```typescript
score = (
  similarity * 0.5 +
  success_score * 0.3 +
  Math.log(usage_count + 1) * 0.1 +
  explorationBonus * 0.1
)
```

### Exploration vs Exploitation

- **20%** - Explore new paths (random)
- **80%** - Exploit best known path

---

## 10. Key Insights for Frontend Developer

### What NOT to Build

- ❌ Video recordings alone (not searchable, not updateable)
- ❌ Hardcoded flows (not adaptive)

### What to Build

- ✅ React Dashboard with real-time issue tracking
- ✅ Form validation (max length, required fields)
- ✅ LocalStorage persistence for forms
- ✅ Multi-tab form auto-save
- ✅ Cost monitoring charts
- ✅ Issue filtering (frontend/backend)

### Final System Capabilities

- 🤖 Autonomous QA detection
- 🔍 AI root cause analysis
- 💡 Auto-fix suggestions
- ✅ QA checklist for marking fixes

---

*Document Version: 1.0*
*Extracted from: Playwright-for-System-Documentation.pdf (128 pages)*
*Role: Senior Frontend Developer + Senior QA*