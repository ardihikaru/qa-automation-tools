# System Architect - Architecture & Design

## Overview

High-level architecture for the AI-Powered QA and System Documentation Platform, designed for scalability using Neon (PostgreSQL), Golang, React, and Playwright.

---

## 1. System Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AI-POWERED QA PLATFORM                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────┐     ┌─────────────────────┐     ┌──────────────┐ │
│  │  PLAYWRIGHT DOCS   │     │  PLAYWRIGHT QA    │     │   AI       │ │
│  │  (Documentation)  │     │  (QA System)   │     │  BACKEND   │ │
│  │                 │     │                │     │            │ │
│  │ • flows/       │     │ • explorer/   │     │ • API      │ │
│  │ • recorder/   │     │ • skills/    │     │ • services │ │
│  │ • video/     │     │ • runner/    │     │ • db/      │ │
│  │ • summarizer │     │ • logger/   │     │ • models/  │ │
│  │ • output/   │     │ • config/   │     │            │ │
│  └──────┬────────┘     └──────┬────────┘     └─────┬──────┘ │
│         │                    │                   │                  │         │
│         │    Logs + Videos    │        │      Neon DB         │         │
│         └──────────────────┼────────┴──────────────────┴─────────┘         │
│                        │                                              │
│                        ▼                                              │
│               ┌─────────────────────┐                                │
│               │   REACT DASHBOARD │                                │
│               │                 │                                │
│               │ • Docs View    │                                │
│               │ • QA Insights  │                                │
│               │ • AI Debugger │                                │
│               │ • Cost Analytics                                │
│               └─────────────────┘                                │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Component Breakdown

### 2.1 Playwright Documentation Generator

**Purpose**: Record flows and generate documentation

| Module | Responsibility |
|--------|-------------|
| `flows/` | Role-based flow definitions (admin.ts, doctor.ts, cashier.ts) |
| `recorder/` | Video and screenshot capture |
| `summarizer/` | Call Minimax for flow description |
| `output/videos/` | Generated MP4 files |
| `output/docs/` | Generated markdown files |

**Key Feature**: Lightweight - no issue detection, just documentation

### 2.2 Playwright QA System

**Purpose**: Autonomous exploration and bug detection

| Module | Responsibility |
|--------|-------------|
| `explorer/guided.ts` | Learning-backed action selection |
| `explorer/planner.ts` | Multi-step planning |
| `explorer/goal.ts` | Goal completion detection |
| `explorer/goal-discovery.ts` | Auto-detect goals without config |
| `skills/builtin/` | Hardcoded skills (max_length, required_field, etc.) |
| `skills/learned/` | AI-generated skills |
| `runner/run.ts` | Main execution loop |
| `logger/api.ts` | Send logs to backend |

**Key Feature**: Full intelligence - exploration, learning, skills

### 2.3 AI Backend (Golang)

**Purpose**: Processing, AI calls, database, APIs

| Service | Responsibility |
|---------|-------------|
| `api/` | HTTP endpoints |
| `services/learning/` | Learning cache management |
| `services/skills/` | Skill generation |
| `services/ai/` | Minimax integration + caching |
| `services/patch/` | Code patch generation |
| `services/ast/` | AST parsing for Go/TS |
| `db/` | Neon connection |
| `models/` | Data models |

**Key Feature**: All AI processing + storage

### 2.4 React Dashboard

**Purpose**: UI for exploring docs, viewing issues, tracking fixes

| View | Responsibility |
|------|-------------|
| `Docs View` | Browse videos/docs |
| `QA Insights` | Skills failures summary |
| `AI Debugger` | Issues with suggestions + patches |
| `Cost Analytics` | AI usage costs |
| `Fix Tracking` | is_fixed checkboxes + notes |

---

## 3. Technology Stack

| Layer | Technology | Rationale |
|-------|-----------|----------|
| **Frontend** | React + Tailwind | Fast UI, component-based |
| **Backend** | Golang | High performance, single binary |
| **Database** | Neon (Postgres + pgvector) | Free tier, vector search |
| **AI** | Minimax (abab6.5s-chat) | Cost-effective, good quality |
| **Automation** | Playwright | Cross-browser, Node.js |
| **Video** | FFmpeg | Video processing |
| **AST Parsing** | go/ast (Go), ts-morph (TS) | Code understanding |

---

## 4. Architecture Decisions

### 4.1 Staging vs Local

**Decision**: Use staging domain

| Option | Pros | Cons |
|--------|-----|------|
| Local FE+BE | Full control | Setup pain |
| **Staging (SELECTED)** | Real behavior, auth works | Need test accounts |

### 4.2 Credentials Handling

**Decision**: YAML file + gitignore

```
config/
  credentials.yaml        # REAL - ignored by git
  credentials.example.json # TEMPLATE - safe for repo
```

### 4.3 Cost Optimization Strategy

| Technique | Expected Savings |
|-----------|----------------|
| Hash-based deduplication | 60-90% |
| Input normalization | +20-40% |
| Embedding reuse | 50-80% |
| Similarity reuse | +10-30% |
| **Combined** | **~95%** |

### 4.4 Learning Progression

| Level | Capability |
|-------|------------|
| **Level 1** | Hash cache (exact match) |
| **Level 2** | Embedding reuse (similar match) |
| **Level 3** | Learning cache (context-aware) |

---

## 5. Data Flow

### 5.1 Documentation Flow

```
1. Define flows (YAML)
2. Run playwright-docs
3. Record video + screenshots
4. Call Minimax for summary
5. Store in output/docs/
6. Serve via backend API
7. Display in dashboard
```

### 5.2 QA Exploration Flow

```
1. Start at login page
2. Detect clickable elements
3. Score actions (similarity + success_score)
4. Select best action (or random 20%)
5. Execute action
6. Run QA Skills
7. Detect goal completion
8. Log to backend API
9. Backend: classify issue, generate suggestions
10. Store in Neon
```

### 5.3 Issue Resolution Flow

```
1. Issue appears in dashboard
2. Developer reviews suggestion + patch
3. QA checks "is_fixed" checkbox
4. QA adds notes (optional)
5. System learns (fix rate improves)
```

---

## 6. API Endpoints

### 6.1 Backend APIs

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/flows` | POST | Ingest logs |
| `/api/flows` | GET | List flows |
| `/api/docs` | GET | Get documentation |
| `/api/videos` | GET | List videos |
| `/api/issues` | GET | List issues |
| `/api/issues/status` | POST | Update is_fixed |
| `/api/issues/notes` | POST | Update notes |
| `/api/skills` | GET | List skills |
| `/api/search` | GET | Semantic search |
| `/api/cost` | GET | Cost analytics |
| `/api/analytics` | GET | Usage analytics |

---

## 7. Security

| Area | Implementation |
|------|---------------|
| **Credentials** | config/credentials.yaml in .gitignore |
| **API Keys** | Environment variables |
| **Access Control** | RBAC for dashboard |
| **Input Validation** | Sanitize all inputs |
| **SQL Injection** | Parameterized queries |

---

## 8. Scalability Considerations

| Area | Approach |
|------|---------|
| **Concurrent Runs** | Queue system (future) |
| **Large Embeddings** | Batch processing |
| **Long-running Jobs** | Background workers |
| **Geographic Access** | CDN for videos (future) |

---

## 9. Future Upgrades (Out of Scope v1)

- Multi-language support
- CI/CD integration
- GitHub API integration
- Real-time notifications
- Mobile app

---

## 10. Architecture Summary

| Component | Lines of Code | Complexity |
|-----------|--------------|-----------|
| playwright-docs | ~500 | Low |
| playwright-qa | ~2000 | High |
| ai-backend | ~3000 | High |
| React dashboard | ~1500 | Medium |

---

*Document Version: 1.0*
*Source: Playwright-for-System-Documentation.pdf*