# PDF Instruction - Full Content

This document contains the extracted and consolidated content from all PDF chunks.

---

## Chunk 1 (Pages 1-20): Foundation - Playwright as System Explorer

### Initial Request
Can we utilize Playwright to go through my entire system and make video recording with multiple scenarios? Or is there a better idea on how to build documentation of the system from the application point of view? I can provide all possible roles with sample access for each role.

### Response: Playwright as "System Explorer + Recorder"
Playwright is great for:
- Logging in as different roles
- Running full UI flows
- Capturing: Video, Screenshots, DOM snapshots, Network logs

Example scenarios defined in YAML:
```yaml
- role: admin
  flow: create_user
- role: doctor
  flow: view_patient
- role: cashier
  flow: billing
```

Output structure:
```
/docs-output/
  admin/
    create_user.mp4
    steps.json
    screenshots/
  doctor/
    view_patient.mp4
```

### Better Approach: AI + Playwright Hybrid

**Architecture:**
1. **Step 1** — Crawl UI via Playwright: auto-discover menus, buttons, forms
2. **Step 2** — Capture Structured Data: page title, URL, visible text, actions performed, result state
3. **Step 3** — Feed into AI (Minimax/GPT): generate feature descriptions, user flows, role-based capabilities, edge cases
4. **Step 4** — Generate Docs: Feature descriptions, Role-based capabilities, User flows

---

## Chunk 2 (Pages 21-40): Architecture Redesign

### New Architecture Stack
| Layer | Technology |
|-------|------------|
| Automation | Playwright |
| Backend | Golang |
| Database | Neon (Postgres + pgvector) |
| AI | Minimax |
| Dashboard | React |

### Project Structure
```
ai-system/
├── backend/           # Golang (API + workers)
├── explorer/          # Playwright (Node)
├── ai/                # Embeddings + clustering
├── dashboard/         # React UI
├── config/            # YAML configs
└── output/            # Generated docs/videos
```

### Neon Database Design
Core Tables:
- **roles**: id, name
- **flows**: id, role_id, name, description, created_at
- **steps**: id, flow_id, action, page, created_at
- **embeddings**: semantic search using pgvector

---

## Chunk 3 (Pages 41-60): Cost Dashboard

### API Endpoints
- `GET /api/cost/overview`: Total requests, tokens, cost
- `GET /api/cost/trend`: Daily usage trends
- `GET /api/cost/efficiency`: Cache hit/miss stats
- `GET /api/cost/by-feature`: Cost per feature

### Cost Optimization Techniques
| Technique | Savings |
|-----------|---------|
| Hash cache (exact match) | 60-90% |
| Input normalization | +20-40% |
| Embedding reuse | 50-80% |
| Similarity reuse | +10-30% |
| **Combined** | **~95%** |

### Database Tables for Caching
- `ai_cache`: Summary caching
- `embedding_cache`: Embedding caching
- `learning_cache`: Context-aware learning

### React Dashboard Components
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

---

## Chunk 4 (Pages 61-80): Guided Explorer

### From Random → Guided Exploration

**Before (random):**
```typescript
const el = elements[Math.floor(Math.random() * elements.length)];
await el.click();
```

**After (guided):**
```typescript
score(action) = 
  similarity_to_known_flows
  + success_rate
  + exploration_bonus
```

### Exploration vs Exploitation
- 20% random (explore new paths)
- 80% best known (exploit)

### Guided Algorithm
1. Extract possible actions
2. Convert each action → semantic meaning
3. Generate embedding for each action
4. Query learning cache
5. Score actions
6. Pick best (or explore)

### Scoring Formula
```typescript
score = 
  match.similarity * 0.5 +
  match.success_score * 0.3 +
  Math.log(match.usage_count + 1) * 0.1 +
  explorationBonus(match) * 0.1
```

---

## Chunk 5 (Pages 81-100): Skills System + Goal Detection

### Common QA Issues to Detect
1. **Max Length Issues** — Textarea allows huge input → backend crash
2. **Required Field Bypass** — '*' marked but accepts empty
3. **Data Persistence** — Form data disappears on reload
4. **Multi-tab Save** — Tab data lost when moving between tabs

### Goal Detection Signals
| Signal | Example |
|--------|---------|
| UI text | "Success", "Created" |
| URL change | /invoice/123 |
| DOM element | .invoice-success |
| Network | 200 OK |
| Data existence | Row appears |

### Multi-Signal Confidence Scoring
```typescript
confidence = (ui_signal * 0.4) + (network_signal * 0.3) + (data_signal * 0.3)
```

### Skills System
Each skill tracks:
- `success_score` per action
- `usage_count` for ranking
- Decay logic (95% decay after 7 days)
- Feedback loop (success/failure signals)

---

## Chunk 6 (Pages 101-120): Auto-Fix Suggestions

### Multi-Suggestion System
- 1 issue → multiple suggestions
- Classify as frontend/backend
- Confidence score per suggestion

### Smart Classification
- 500 errors → backend
- JS errors → frontend
- Text containing "500" → backend

### Database Design
```sql
raw_issues (parent)
  └── issue_suggestions (child)
        - suggestion
        - confidence
        - target (frontend/backend)
        - file_path
        - diff
```

### API Endpoints
- `GET /issues?target=frontend`
- `GET /issues?target=backend`

---

## Chunk 7 (Pages 121-128): Fix Tracking + Project Separation

### Database Update
```sql
ALTER TABLE raw_issues
ADD COLUMN is_fixed BOOLEAN DEFAULT FALSE,
ADD COLUMN notes TEXT;
```

### Backend API (Golang)
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

### Three Project Separation
| Project | Responsibility |
|---------|---------------|
| docs | Product understanding |
| qa | Bug discovery |
| backend | Intelligence + storage |

### Final Project Structure
```
├── playwright-docs/     # Documentation only
├── playwright-qa/       # Full QA system
└── ai-backend/          # Golang + React
```

### Dashboard Checklist UX
- ✅ Checkbox: Mark as Fixed
- 📝 Textarea: Add QA notes
- Filter: All / Fixed / Open

---

## Key Insights

1. **This is NOT just test automation** — "What you're building is actually a System Understanding Engine"

2. **Three Project Separation Matters**
   - docs: Product understanding
   - qa: Bug discovery
   - backend: Intelligence + storage

3. **Neon is sufficient** — "You don't need Redis because Postgres + pgvector = cache + search + memory"

4. **Cost optimization works** — 95% cost reduction achievable

5. **Learning improves over time** — Day 1: random, Day 30: near-human QA tester

---

## Implementation Roadmap Summary

| Phase | What to Build | Complexity |
|-------|-------------|-----------|
| 1 | Basic recorder | Low |
| 2 | AI backend | Medium |
| 3 | Cost dashboard | Medium |
| 4 | Learning cache | High |
| 5 | Guided explorer | High |
| 6 | Skills system | High |
| 7 | Auto-fix | Very High |

---

*Document Version: 1.0*
*Extracted from: Playwright-for-System-Documentation.pdf (128 pages)*
