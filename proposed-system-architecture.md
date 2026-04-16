# Proposed System Architecture

## Executive Summary

This document proposes the technical architecture for the AI-Powered QA and System Documentation Platform. The system is designed to automatically explore web applications, generate documentation, detect QA issues, and improve over time using learning mechanisms.

---

## 1. High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           CLIENTS                                       │
├─────────────────────┬─────────────────────┬───────────────────────────┤
│   Playwright-Docs   │    Playwright-QA    │      Dashboard            │
│   (Documentation)   │    (QA Testing)      │     (Visualization)       │
└─────────┬───────────┴─────────┬───────────┴───────────┬─────────────┘
          │                     │                       │
          ▼                     ▼                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         AI-BACKEND (Golang)                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │
│  │   Flows    │  │   Issues    │  │   Skills    │  │   Cost      │   │
│  │   API      │  │    API      │  │    API      │  │   API       │   │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │
│  │   Search    │  │  Learning   │  │   Docs      │  │  Videos     │   │
│  │    API      │  │    API      │  │    API      │  │    API      │   │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
          │                     │                       │
          │                     │                       │
          ▼                     ▼                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     NEON DATABASE (PostgreSQL)                          │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────────┐              │
│  │ public  │  │    ai   │  │    qa   │  │  logging    │              │
│  │ schema  │  │  schema │  │  schema │  │   schema    │              │
│  └─────────┘  └─────────┘  └─────────┘  └─────────────┘              │
└─────────────────────────────────────────────────────────────────────────┘
          │                     │                       │
          └─────────────────────┴───────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    MINIMAX AI (External Service)                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                    │
│  │ Summarize   │  │  Embeddings │  │   Skill     │                    │
│  │             │  │             │  │  Generation │                    │
│  └─────────────┘  └─────────────┘  └─────────────┘                    │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Component Details

### 2.1 Playwright-Docs (Documentation Generator)

**Purpose**: Generate automated documentation including screenshots and videos.

**Technology Stack**:
- Language: TypeScript / Node.js
- Framework: Playwright
- AI Integration: Minimax API

**Core Modules**:
```
playwright-docs/
├── src/
│   ├── index.ts              # Main entry point
│   ├── exploreSystem.ts     # System route explorer
│   ├── flows/
│   │   └── flowLoader.ts    # Flow definitions per role
│   ├── recorder/
│   │   └── recordFlow.ts   # Video/screenshot capture
│   └── summarizer/
│       └── summarizeFlow.ts # AI summarization
├── config/
│   └── credentials.yaml     # Role-based credentials
└── output/                  # Generated docs/videos
```

**Key Features**:
- Multi-role credential management
- Automated login flows
- Screenshot capture per step
- Video recording
- Role-based route mapping

### 2.2 Playwright-QA (QA Automation)

**Purpose**: Self-documented periodical QA automation with learning capabilities.

**Technology Stack**:
- Language: TypeScript / Node.js
- Framework: Playwright
- AI Integration: Minimax API

**Core Modules**:
```
playwright-qa/
├── explorer/        # System exploration
├── skills/          # QA skill definitions
├── runner/          # Test runner
├── logger/          # Result logging
└── config/          # Configuration
```

**Key Features**:
- Random exploration
- Guided exploration (learning-based)
- Multi-step planning agent
- Goal detection
- Skill execution
- Result submission to backend

### 2.3 AI-Backend (Golang API)

**Purpose**: Central API for data ingestion, AI communication, and data storage.

**Technology Stack**:
- Language: Go
- Framework: Standard library / Gorilla Mux
- Database: pgx (PostgreSQL driver)
- AI: Minimax API client

**Core Modules**:
```
ai-backend/
├── cmd/
│   └── server/
│       └── main.go          # Entry point
├── internal/
│   ├── api/
│   │   └── router.go        # HTTP handlers
│   ├── models/
│   │   └── models.go        # Data models
│   ├── services/
│   │   ├── ai/              # AI service
│   │   ├── learning/        # Learning service
│   │   └── skills/          # Skills service
│   ├── db/
│   │   └── db.go            # Database connection
│   └── config/
│       └── config.go        # Configuration
└── resources/               # Static resources
```

**API Endpoints**:
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/flows` | GET/POST | Flow management |
| `/api/issues` | GET/POST | Issue management |
| `/api/issues/status` | POST | Mark issue as fixed |
| `/api/issues/notes` | POST | Add notes to issue |
| `/api/skills` | GET | List QA skills |
| `/api/search` | GET | Semantic search |
| `/api/cost/overview` | GET | Cost overview |
| `/api/cost/trend` | GET | Cost trend |
| `/api/cost/efficiency` | GET | Cache efficiency |
| `/api/cost/by-feature` | GET | Cost by feature |

### 2.4 Dashboard (React UI)

**Purpose**: Visualize all QA automation activities.

**Technology Stack**:
- Language: TypeScript
- Framework: React + Vite
- Styling: Tailwind CSS

**Core Views**:
```
dashboard/src/views/
├── QAInsights.tsx      # QA overview
├── FixTracking.tsx    # Issue fix tracking
├── IssueDebugger.tsx  # Issue debugging
├── DocsView.tsx       # Documentation viewer
└── CostAnalytics.tsx  # Cost analytics
```

---

## 3. Database Architecture

### 3.1 Schema Overview

| Schema | Tables | Purpose |
|--------|--------|---------|
| **public** | 4 | Core data - roles, flows, steps, analytics |
| **ai** | 5 | AI cache - summaries, embeddings |
| **qa** | 6 | Issues - skills, suggestions |
| **logging** | 1 | Cost tracking |

### 3.2 Core Tables (Public Schema)

```
┌─────────────┐
│    roles    │  System roles (admin, doctor, cashier)
├─────────────┤
│    flows    │  User flows per role
├─────────────┤
│    steps    │  Individual steps in each flow
├─────────────┤
│  analytics  │  Feature usage tracking
└─────────────┘
```

### 3.3 AI Tables (AI Schema)

```
┌─────────────────┐
│    ai_cache     │  Hash-based AI summary caching
├─────────────────┤
│ embedding_cache │  Embeddings for semantic search
├─────────────────┤
│  learning_cache │  Context-aware learning
├─────────────────┤
│ action_sequences│  Successful multi-step plans
├─────────────────┤
│ navigation_edges│  Navigation graph
└─────────────────┘
```

### 3.4 QA Tables (QA Schema)

```
┌───────────────────┐
│    raw_issues     │  Detected issues with fix tracking
├───────────────────┤
│issue_suggestions  │  Fix suggestions per issue
├───────────────────┤
│      skills       │  QA skill definitions
├───────────────────┤
│    skill_logs     │  Skill execution logs
├───────────────────┤
│   goal_events     │  Goal completion tracking
└───────────────────┘
```

### 3.5 Logging Tables

```
┌───────────────┐
│ request_logs  │  Cost analytics tracking
└───────────────┘
```

---

## 4. API Communication Flow

### 4.1 Documentation Generation Flow

```
1. Playwright-Docs
   │
   ▼ (Run flows)
2. Capture screenshots + videos
   │
   ▼ (Submit flow data)
3. AI-Backend: POST /api/flows
   │
   ▼ (Store)
4. Neon Database: flows, steps tables
   │
   ▼ (Optional: Generate summary)
5. AI-Backend → Minimax API
   │
   ▼ (Return summary)
6. Save to ai_cache
```

### 4.2 QA Testing Flow

```
1. Playwright-QA
   │
   ▼ (Explore system)
2. Execute skills (max_length, required_field, etc.)
   │
   ▼ (Detect issues)
3. Submit issues: POST /api/issues
   │
   ▼ (Store)
4. Neon Database: raw_issues
   │
   ▼ (Generate suggestions)
5. AI-Backend → Minimax API (classify)
   │
   ▼ (Update category)
6. Issue suggestions: POST /api/issues
   │
   ▼ (Store suggestions)
7. Neon Database: issue_suggestions
```

### 4.3 Learning Flow

```
1. Playwright-QA (Exploration)
   │
   ▼ (Action results)
2. Record success/failure
   │
   ▼ (Submit learning)
3. POST /api/learning
   │
   ▼ (Store)
4. Neon Database: learning_cache
   │
   ▼ (Query for next action)
5. GET /api/search (with embedding)
   │
   ▼ (Return best actions)
6. Use learning for guided exploration
```

---

## 5. Cost Optimization Strategy

### 5.1 Caching Layers

```
┌────────────────────────────────────┐
│        Level 1: Hash Cache        │
│   Exact match → Return cached      │
│   (60-90% savings)                 │
└────────────────────────────────────┘
              │
              ▼
┌────────────────────────────────────┐
│      Level 2: Embedding Cache      │
│   Similar inputs → Reuse embed     │
│   (50-80% savings)                │
└────────────────────────────────────┘
              │
              ▼
┌────────────────────────────────────┐
│      Level 3: Learning Cache       │
│   Context-aware → Best action      │
│   (10-30% additional savings)     │
└────────────────────────────────────┘
```

### 5.2 Cost Tracking

Every API call logs:
- Request type (summary/embedding)
- Feature/endpoint
- Token count
- Cost estimate
- Cache hit/miss
- Success/failure

Dashboard displays:
- Total requests
- Total tokens
- Total cost
- Cache hit rate
- Cost by feature
- Daily trends

---

## 6. Skill System Architecture

### 6.1 Built-in Skills

| Skill | Purpose | Detection Method |
|-------|---------|------------------|
| Max Length | Detect backend crash from long input | Submit long text, check error |
| Required Field | Detect bypass of required fields | Submit empty required field |
| Data Persistence | Detect form data loss after reload | Fill form, reload, check |
| Multi-tab Save | Detect tab data loss | Fill tab, switch, check |

### 6.2 Skill Evolution

```
1. Observe repeated issues
        │
        ▼
2. Group by similarity (pgvector)
        │
        ▼
3. Call Minimax → Generate skill
        │
        ▼
4. Store in database (skills table)
        │
        ▼
5. Use in next QA run
```

### 6.3 Issue Auto-Fix System

```
1. Raw issue detected
        │
        ▼
2. Classify (frontend/backend)
   - 500 errors → backend
   - JS errors → frontend
        │
        ▼
3. Generate multiple suggestions
        │
        ▼
4. Store in issue_suggestions
        │
        ▼
5. User reviews in dashboard
        │
        ▼
6. Mark as fixed + add notes
```

---

## 7. Security Considerations

### 7.1 API Security
- API Secret Key authentication
- CORS enabled (configurable origins)
- Input validation and sanitization

### 7.2 Database Security
- SSL/TLS encrypted connections
- Credential rotation
- Principle of least privilege

### 7.3 Credential Management
- Credentials stored in config.yaml (gitignored)
- Separate credentials per environment
- Role-based access control

---

## 8. Deployment Architecture

### 8.1 Current Deployment (Staging)

```
┌─────────────────────────────────────────────────────────────┐
│                         GCP Cloud Run                        │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐    ┌─────────────────┐                │
│  │   AI-Backend    │    │    Dashboard    │                │
│  │   (Golang)      │    │    (React)      │                │
│  └─────────────────┘    └─────────────────┘                │
└─────────────────────────────────────────────────────────────┘
              │                         │
              │                         │
              ▼                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    External Services                         │
├─────────────────────────────────────────────────────────────┤
│  Neon Database   │   Minimax API   │   SUT Staging     │
│  (PostgreSQL)    │   (AI)          │   (Target App)       │
└─────────────────────────────────────────────────────────────┘
```

### 8.2 URLs

| Service | URL |
|---------|-----|
| AI Backend | https://ai-backend-000000000000.asia-southeast1.run.app |
| Dashboard | (to be determined) |
| SUT Staging | https://stag-app.mfardiansyah.id/ |

---

## 9. Future Enhancements

### 9.1 Phase 1 - Core (Current)
- [x] Basic flow execution
- [x] Screenshot capture
- [x] Video recording
- [x] Issue tracking
- [x] Fix status tracking

### 9.2 Phase 2 - Learning
- [ ] Learning cache API
- [ ] Guided exploration
- [ ] Navigation graph
- [ ] Action sequences

### 9.3 Phase 3 - AI Enhancement
- [ ] AI narration for videos
- [ ] Auto menu discovery
- [ ] Goal detection
- [ ] Auto skill generation

### 9.4 Phase 4 - Advanced
- [ ] Auto-fix suggestions
- [ ] Patch generation
- [ ] Heatmap visualization
- [ ] Advanced analytics

---

## 10. Conclusion

This architecture provides a comprehensive solution for:
1. **Automated documentation** of web applications
2. **Self-learning QA system** that improves over time
3. **Cost-optimized AI** usage through caching
4. **Visualization dashboard** for monitoring and tracking

The system is modular and scalable, allowing individual components to be enhanced independently.

---

*Document Version: 1.0*
*Created: 2026-04-16*
*Author: Senior System Analyst*
