# OBJECTIVES.md - QA Automation System Objectives

## Overview

This document outlines the objectives of the AI-Powered QA and System Documentation Platform. It maps database tables to API implementations, identifies gaps, and defines future development goals.

---

## 1. System Objectives Summary

Based on the project references, the main objectives are:

1. **Read all available roles** in the SUT system (e.g., https://stag-app.mfardiansyah.id/)
2. **Automatically generate full documentation** with AI narration. As of 2026-11-13, every LLM call goes through ai-manager pinned to **claude-v3**; MiniMax is dormant.
3. **Documentation runs** (`docs_generate`, previously playwright-docs): self-exploration of system menus, screenshots and narratives
4. **QA runs** (`qa_run`, previously playwright-qa): periodic QA automation that tests and submits results to the backend.
   Both now run in **playwright-executor**, a systemd/mise service on app-host capped at 2.5 GiB / 60 % CPU ([spec](docs/designs/PLAYWRIGHT_EXECUTOR.md)).
5. **ai-backend**: Support communication with every data logging
6. **dashboard**: Visualize everything related to QA automation activities
7. **Automated testing** with detailed logs - query AI to analyze potential bugs (FE/BE), estimate commit hash where issue was introduced
8. **Each testing iteration logged** - issues have action checklist, users can mark fixed/not fixed
9. **AI automation skills** - basic mandatory checks that evolve over time, findings analyzed and added to skill database

---

## 2. Database Schemas

Based on the design in `database-admin.md`, there are 4 schemas:

| Schema | Tables | Purpose |
|--------|--------|---------|
| **public** | 4 | Core data - roles, flows, steps, analytics |
| **ai** | 5 | AI cache - summaries, embeddings |
| **qa** | 6 | Issues - skills, suggestions |
| **logging** | 1 | Cost tracking |

**Total: 16 tables**

---

## 3. Database Tables and API Mapping

### 3.1 Public Schema Tables

| Table | Purpose | API Endpoint | Status |
|-------|---------|---------------|--------|
| `roles` | System roles (admin, doctor, cashier) | - | 🔶 Not implemented |
| `flows` | User flows per role | `/api/flows` | ✅ Implemented (GET, POST) |
| `steps` | Individual steps in each flow | `/api/flows` (nested) | 🔶 Partial (via flows) |
| `analytics` | Feature usage tracking | `/api/analytics` | 🔶 Mock response |

### 3.2 AI Schema Tables

| Table | Purpose | API Endpoint | Status |
|-------|---------|---------------|--------|
| `ai_cache` | Hash-based deduplication for AI summaries | - | 🔶 Not directly exposed |
| `embedding_cache` | Store embeddings for semantic search | `/api/search` | ✅ Implemented (via search) |
| `learning_cache` | Context-aware learning from past interactions | `/api/learning` | 🔶 Mock response |
| `action_sequences` | Store successful multi-step plans | - | 🔶 Not implemented |
| `navigation_edges` | Learn navigation graph | - | 🔶 Not implemented |

### 3.3 QA Schema Tables

| Table | Purpose | API Endpoint | Status |
|-------|---------|---------------|--------|
| `raw_issues` | Capture detected issues with fix tracking | `/api/issues` | ✅ Implemented (GET, POST) |
| `issue_suggestions` | Multiple suggestions per issue | `/api/issues` (nested) | ✅ Implemented |
| `skills` | QA Skills (detection patterns) | `/api/skills` | ✅ Implemented (GET) |
| `skill_logs` | Track skill execution results | - | 🔶 Not implemented |
| `goal_events` | Goal completion tracking | - | 🔶 Not implemented |
| `raw_issues` (captured) | Issues captured from Playwright runs | `/api/issues` (POST) | ✅ Implemented |

### 3.4 Logging Schema Tables

| Table | Purpose | API Endpoint | Status |
|-------|---------|---------------|--------|
| `request_logs` | Cost analytics tracking | `/api/cost/*` | ✅ Implemented (overview, trend, efficiency, by-feature) |

---

## 4. API Endpoints Assessment

### 4.1 Implemented Endpoints

| Endpoint | Method | Status | Table(s) |
|----------|--------|--------|----------|
| `/health` | GET | ✅ Working | - |
| `/api/flows` | GET | ✅ Working | flows |
| `/api/flows` | POST | ✅ Working | flows, steps |
| `/api/issues` | GET | ✅ Working | raw_issues, issue_suggestions |
| `/api/issues` | POST | ✅ Working | raw_issues |
| `/api/issues/status` | POST | ✅ Working | raw_issues (is_fixed) |
| `/api/issues/notes` | POST | ✅ Working | raw_issues (notes) |
| `/api/skills` | GET | ✅ Working | skills |
| `/api/search` | GET | ✅ Working | learning_cache (embedding) |
| `/api/cost/overview` | GET | ✅ Working | request_logs |
| `/api/cost/trend` | GET | ✅ Working | request_logs |
| `/api/cost/efficiency` | GET | ✅ Working | request_logs |
| `/api/cost/by-feature` | GET | ✅ Working | request_logs |
| `/api/docs` | GET | 🔶 Mock | - |
| `/api/videos` | GET | 🔶 Mock | - |
| `/api/analytics` | GET | 🔶 Mock | - |
| `/api/learning` | GET | 🔶 Mock | - |

### 4.2 Not Implemented Endpoints (TODO)

| Endpoint | Description | Priority | Table(s) |
|----------|-------------|----------|----------|
| `POST /api/skills` | Create new skill | Medium | skills |
| `POST /api/learning` | Add learning record | High | learning_cache |
| `POST /api/skill-logs` | Log skill execution | Medium | skill_logs |
| `GET /api/navigation` | Get navigation graph | Low | navigation_edges |
| `POST /api/action-sequences` | Save action sequence | Medium | action_sequences |
| `GET /api/goal-events` | Get goal events | Low | goal_events |
| `POST /api/goal-events` | Record goal event | Low | goal_events |
| `GET /api/roles` | List roles | Medium | roles |
| `POST /api/roles` | Create role | Low | roles |

---

## 5. Dashboard Assessment

### 5.1 Implemented Views

| View | File | Description | API(s) Used | Status |
|------|------|-------------|-------------|--------|
| QA Insights | `QAInsights.tsx` | QA overview and insights | - | 🔶 Needs implementation |
| Fix Tracking | `FixTracking.tsx` | Track fixed/open issues | `/api/issues` | ✅ Implemented |
| Issue Debugger | `IssueDebugger.tsx` | Debug issues | `/api/issues` | 🔶 Needs implementation |
| Docs View | `DocsView.tsx` | View documentation | - | 🔶 Needs implementation |
| Cost Analytics | `CostAnalytics.tsx` | Cost overview | `/api/cost/*` | 🔶 Needs implementation |

### 5.2 Dashboard TODOs

| Feature | Description | Priority | Related Tables |
|---------|-------------|----------|----------------|
| Flow Explorer UI | Interactive flow diagram | Low | flows, steps |
| Role Management UI | Manage roles and permissions | Low | roles |
| Learning Visualization | Show learning progress | Medium | learning_cache |
| Navigation Graph | Visualize page navigation | Low | navigation_edges |
| Skill Execution UI | Run skills manually | Medium | skills, skill_logs |
| Goal Tracking UI | Track goal completion | Low | goal_events |
| Video Playback | Play recorded videos | Medium | - |
| Screenshot Gallery | View captured screenshots | Medium | - |

---

## 5b. playwright-executor (2026-11-13)

Sections 6 and 7 below describe the deprecated packages. Their run paths now live in `playwright-executor/`:

| Feature | Where | Status |
|---------|-------|--------|
| Run service (HTTP API, bearer auth, concurrency 1, timeouts) | `playwright-executor/src/server.ts`, `queue.ts` | ✅ Deployed on app-host :8141 |
| QA run: login, random exploration, built-in skills, JS errors, HTTP 5xx | `src/runs/qaRun.ts`, `src/skills/builtin.ts` | ✅ |
| Docs run: per-role flows + screenshots | `src/runs/docsGenerate.ts`, `src/flows/flows.ts` | ✅ |
| Backend dispatch (`/api/jobs/qa/run`, `/api/jobs/docs/generate`) | `ai-backend/internal/api/jobs_executor.go` | ✅ Issues → `raw_issues`, docs → Markdown via claude-v3 |
| Resource cap (2.5 GiB, 60 % CPU) via systemd + mise | `deploy/playwright-executor/` | ✅ |
| Guided / planning exploration, learned skills, video, screenshot serving | — | ⬜ TODO |

## 6. Playwright-Docs Assessment (deprecated package)

### 6.1 Implemented Features

| Feature | File | Status |
|---------|------|--------|
| Multi-role credential management | `config/credentials.yaml` | ✅ Complete |
| Login flow | `flows/flowLoader.ts` | ✅ Complete |
| Screenshot capture | `recorder/recordFlow.ts` | ✅ Complete |
| Video recording | `index.ts` | ✅ Complete |
| Role-based flow mapping | `index.ts` | ✅ Complete |
| Summarizer (ai-manager/claude-v3; MiniMax dormant) | `summarizer/summarizeFlow.ts` | ⛔ Superseded by backend `summarizeDocsRun` |

### 6.2 Playwright-Docs TODOs

| Feature | Description | Priority |
|---------|------------|----------|
| Auto-discovery of menus | Dynamically discover available menus | High |
| AI-generated narration | Narration through ai-manager/claude-v3 | High |
| Documentation generation | ✅ Done in executor + backend (`documentation_markdown`) | — |
| Submit to backend | ✅ Done (backend polls the executor) | — |
| Comprehensive route coverage | Test all possible routes per role | High |

---

## 7. Playwright-QA Assessment (deprecated package)

### 7.1 Implemented Features

| Feature | Directory | Status |
|---------|-----------|--------|
| Explorer | `explorer/` | 🔶 In development |
| Skills system | `skills/` | 🔶 In development |
| Runner | `runner/` | 🔶 In development |
| Logger | `logger/` | 🔶 In development |
| Config | `config/` | 🔶 In development |

### 7.2 Playwright-QA TODOs

| Feature | Description | Priority |
|---------|------------|----------|
| Guided exploration | Implement learning-based exploration | High |
| Multi-step planning | Plan complex flows | High |
| Goal detection | Detect goal completion | High |
| Auto-skill discovery | Auto-detect and create skills | High |
| Submit results to backend | Send QA results to ai-backend | High |
| Skill execution automation | Run skills periodically | Medium |

---

## 8. Implementation Gap Summary

### 8.1 Backend Gaps

| Gap | Priority | Impact |
|-----|----------|--------|
| Learning cache API incomplete | High | Can't store learning data |
| Navigation edges not tracked | Medium | Can't learn navigation |
| Goal events not recorded | Low | Can't track goal completion |
| Skill logs not captured | Medium | Can't track skill execution |

### 8.2 Dashboard Gaps

| Gap | Priority | Impact |
|-----|----------|--------|
| QA Insights view incomplete | Medium | Can't see overview |
| Cost Analytics view incomplete | Medium | Can't see cost data |
| Docs View incomplete | Medium | Can't view docs |
| Issue Debugger incomplete | Medium | Can't debug issues |

### 8.3 Playwright-Docs Gaps

| Gap | Priority | Impact |
|-----|----------|--------|
| AI narration not integrated | High | Videos lack narration |
| Auto menu discovery | High | Manual flow definition |
| Documentation generation | Medium | Manual doc creation |

### 8.4 Playwright-QA Gaps

| Gap | Priority | Impact |
|-----|----------|--------|
| Guided exploration | High | Random exploration only |
| Multi-step planning | High | Single step only |
| Goal detection | High | Can't detect success |
| Auto skill discovery | Medium | Manual skill definition |

---

## 9. Future Development Priorities

### Phase 1 - Core Functionality
- [ ] Fix Tracking in Dashboard (partial done)
- [ ] Cost Analytics in Dashboard
- [ ] Submit QA results to backend

### Phase 2 - Learning System
- [ ] Learning cache API
- [ ] Guided exploration in Playwright-QA
- [ ] Navigation graph tracking

### Phase 3 - AI Enhancement
- [ ] AI narration for videos
- [ ] Auto menu discovery
- [ ] Goal detection
- [ ] Auto skill discovery

### Phase 4 - Advanced Features
- [ ] Auto-fix suggestions
- [ ] Patch generation
- [ ] Heatmap visualization
- [ ] Skill evolution

---

## 10. Related Documentation

- `pdf-instruction-summary.md` - Full system design from PDF
- `system_analysis.md` - System analysis document
- `database-admin.md` - Database schema design
- `tutorial-playwright-docs.md` - How to run Playwright Docs

---

*Last Updated: 2025-04-16*
*Document Version: 1.0*