# TODO for Backend Developer - Senior Software Engineer

## Role
**Senior Software Engineer** - Backend Developer

## Project
**AI-Powered QA and System Documentation Platform** - ai-backend (Golang)

## Context

You will be implementing backend APIs for the QA Automation System. The system uses Golang with pgx (PostgreSQL driver) and communicates with Neon database and Minimax AI.

## Current Status
- **Overall Progress**: 70% complete
- **Completed**: Flow API, Issues API, Skills API, Search API, Cost APIs
- **Not Implemented**: Learning cache, skill logs, goal events, navigation edges, action sequences, roles

## Reference Files
- `proposed-system-architecture-v1-20260416.md` - Full architecture
- `OBJECTIVES.md` - Table to API mapping
- `ai-backend/internal/api/router.go` - Current API implementation
- `ai-backend/internal/models/models.go` - Data models
- `ai-backend/internal/db/db.go` - Database connection

---

## Task 1: Learning Cache API Implementation (P0 - Critical)

### Description
Implement `/api/learning` endpoints for storing and retrieving learning records from the learning_cache table.

### Database Table: learning_cache

```sql
CREATE TABLE learning_cache (
    id SERIAL PRIMARY KEY,
    intent TEXT NOT NULL,
    normalized_input TEXT NOT NULL,
    flow_id INT,
    embedding VECTOR(1536),
    success_score FLOAT DEFAULT 1.0,
    usage_count INT DEFAULT 1,
    last_used TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_learning_intent ON learning_cache(intent);
```

### API Endpoints to Implement

**GET /api/learning**
- Query parameters: `q` (search query), `limit`
- Returns: List of learning records with similarity search
- Uses: Embedding similarity search (`<->` operator)

**POST /api/learning**
- Request body:
  ```json
  {
    "intent": "view patient list",
    "action": "click",
    "page": "/doctor/patient",
    "success_score": 0.9,
    "usage_count": 5
  }
  ```
- Generates embedding for intent
- Stores in database with default values

### Implementation Steps

1. Add LearningRecord model to `models.go`
2. Add learning handler to `router.go`
3. Implement GET endpoint with embedding search
4. Implement POST endpoint with embedding generation
5. Implement decay logic (apply 0.95 decay after 7 days)
6. Write unit tests

### Dependencies
- AI service for embedding generation
- Database with pgvector extension

---

## Task 2: Skill Logs API Implementation (P0 - Critical)

### Description
Implement endpoint for tracking skill execution results in `skill_logs` table.

### Database Table: skill_logs

```sql
CREATE TABLE skill_logs (
    id SERIAL PRIMARY KEY,
    scenario_id TEXT,
    skill_id INT REFERENCES skills(id),
    passed BOOLEAN,
    details JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_skill_logs_scenario ON skill_logs(scenario_id);
CREATE INDEX idx_skill_logs_skill ON skill_logs(skill_id);
```

### API Endpoints to Implement

**GET /api/skill-logs**
- Query parameters: `skill_id`, `scenario_id`, `passed`, `from_date`, `to_date`
- Returns: List of skill execution logs with pagination

**POST /api/skill-logs**
- Request body:
  ```json
  {
    "scenario_id": "run-001",
    "skill_id": 1,
    "passed": false,
    "details": {
      "page": "/doctor/prescription",
      "action": "submit",
      "error": "required field missing"
    }
  }
  ```

### Implementation Steps

1. Add SkillLog model to `models.go`
2. Add skill-logs handler to `router.go`
3. Implement GET with filtering and pagination
4. Implement POST to create log entry
5. Write unit tests

---

## Task 3: Goal Events API Implementation (P0 - Critical)

### Description
Implement endpoints for tracking goal completion events.

### Database Table: goal_events

```sql
CREATE TABLE goal_events (
    id SERIAL PRIMARY KEY,
    scenario_id TEXT,
    goal TEXT,
    confidence FLOAT,
    signals JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_goal_scenario ON goal_events(scenario_id);
```

### API Endpoints to Implement

**GET /api/goal-events**
- Query parameters: `scenario_id`, `goal`, `from_date`, `to_date`
- Returns: List of goal events

**POST /api/goal-events**
- Request body:
  ```json
  {
    "scenario_id": "run-001",
    "goal": "create_prescription",
    "confidence": 0.85,
    "signals": {
      "ui_text": "Prescription created",
      "url_change": "/doctor/prescription/123",
      "network_status": 200
    }
  }
  ```

### Implementation Steps

1. Add GoalEvent model to `models.go`
2. Add goal-events handler to `router.go`
3. Implement GET with filtering
4. Implement POST to record goal completion
5. Write unit tests

---

## Task 4: Navigation Edges API (P1 - High)

### Description
Implement endpoints to track and query navigation graph.

### Database Table: navigation_edges

```sql
CREATE TABLE navigation_edges (
    id SERIAL PRIMARY KEY,
    from_page TEXT NOT NULL,
    to_page TEXT NOT NULL,
    action TEXT NOT NULL,
    success_rate FLOAT DEFAULT 0.5
);

CREATE INDEX idx_nav_from ON navigation_edges(from_page);
```

### API Endpoints to Implement

**GET /api/navigation**
- Query parameters: `from_page`, `to_page`
- Returns: Navigation edges matching criteria

**POST /api/navigation**
- Request body:
  ```json
  {
    "from_page": "/doctor/queue",
    "to_page": "/doctor/patient",
    "action": "click",
    "success_rate": 0.95
  }
  ```

### Implementation Steps

1. Add NavigationEdge model to `models.go`
2. Add navigation handler to `router.go`
3. Implement GET for path finding
4. Implement POST to record navigation edge
5. Implement path finding algorithm (BFS/DFS)

---

## Task 5: Action Sequences API (P1 - High)

### Description
Implement endpoints for storing multi-step action sequences.

### Database Table: action_sequences

```sql
CREATE TABLE action_sequences (
    id SERIAL PRIMARY KEY,
    intent TEXT NOT NULL,
    steps TEXT[] NOT NULL,
    success_score FLOAT,
    usage_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);
```

### API Endpoints to Implement

**GET /api/action-sequences**
- Query parameters: `intent` (search)
- Returns: Matching action sequences

**POST /api/action-sequences**
- Request body:
  ```json
  {
    "intent": "register patient",
    "steps": ["/auth/login", "/admission/patient/new", "/admission/patient/save"],
    "success_score": 0.85
  }
  ```

### Implementation Steps

1. Add ActionSequence model to `models.go`
2. Add action-sequences handler to `router.go`
3. Implement GET with intent search
4. Implement POST to save sequence
5. Update usage_count on successful use

---

## Task 6: Roles API (P1 - High)

### Description
Implement endpoints for role management.

### Database Table: roles

```sql
CREATE TABLE roles (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

### API Endpoints to Implement

**GET /api/roles**
- Returns: List of all roles

**POST /api/roles**
- Request body:
  ```json
  {
    "name": "doctor_sp",
    "description": "Doctor Specialist"
  }
  ```

### Implementation Steps

1. Add Role model to `models.go`
2. Add roles handler to `router.go`
3. Implement GET (list all)
4. Implement POST (create new)
5. Write unit tests

---

## Task 7: Analytics Enhancement (P2 - Medium)

### Description
Enhance `/api/analytics` to return real data instead of mock response.

### Implementation Steps

1. Query flows table: COUNT(*) 
2. Query raw_issues table: COUNT(*), COUNT(*) WHERE is_fixed
3. Query skills table: COUNT(*)
4. Query skill_logs: success/failure rate
5. Return comprehensive analytics object
6. Add caching for performance

### Expected Response
```json
{
  "flows": {
    "total": 45,
    "by_role": {
      "doctor": 20,
      "nurse": 15,
      "admission": 10
    }
  },
  "issues": {
    "total": 123,
    "open": 45,
    "fixed": 78,
    "by_category": {
      "frontend": 60,
      "backend": 45,
      "unknown": 18
    }
  },
  "skills": {
    "total": 5,
    "executions": 500,
    "failures": 45
  },
  "learning": {
    "records": 1200,
    "success_rate": 0.75
  }
}
```

---

## Task 8: Docs Endpoint Enhancement (P2 - Medium)

### Description
Implement `/api/docs` to return actual documentation data.

### Implementation Steps

1. Create table or use existing flows/steps
2. Store markdown documentation
3. Link to screenshots/videos
4. Implement GET with filters
5. Add search functionality

### Expected Response
```json
{
  "docs": [
    {
      "id": 1,
      "title": "Doctor SP Login Flow",
      "role": "doctor_sp",
      "steps": [...],
      "screenshots": ["/screenshots/doctor_sp/login-1.png"],
      "video": "/videos/doctor_sp/login.webm",
      "created_at": "2026-04-16T10:00:00Z"
    }
  ]
}
```

---

## Task 9: Videos Endpoint Enhancement (P2 - Medium)

### Description
Implement `/api/videos` to return video metadata.

### Implementation Steps

1. Create videos table or use file system metadata
2. Store video URL, duration, associated flow
3. Implement GET with filters
4. Add pagination

### Expected Response
```json
{
  "videos": [
    {
      "id": 1,
      "filename": "doctor-sp-login.webm",
      "url": "https://storage.googleapis.com/.../doctor-sp-login.webm",
      "duration": 45.5,
      "flow_id": 1,
      "created_at": "2026-04-16T10:00:00Z"
    }
  ]
}
```

---

## Implementation Guidelines

### Code Structure
```
ai-backend/
├── internal/
│   ├── api/
│   │   └── router.go      # Add new handlers here
│   ├── models/
│   │   └── models.go      # Add new models here
│   ├── services/
│   │   ├── ai/            # AI service (embedding, summarization)
│   │   ├── learning/      # Learning service
│   │   └── skills/        # Skills service
│   └── db/
│       └── db.go          # Database connection
```

### Database Migration
Create SQL migration files in `ai-backend/internal/migrations/`:
- `001_create_learning_cache.sql`
- `001_create_skill_logs.sql`
- `001_create_goal_events.sql`
- `001_create_navigation_edges.sql`
- `001_create_action_sequences.sql`

### Testing
- Write unit tests for each handler
- Use existing test patterns from `router_test.go`
- Test edge cases (empty results, invalid input, etc.)

### Error Handling
- Return proper HTTP status codes
- Include error messages in response
- Log errors for debugging

---

## Notes

1. All endpoints should follow RESTful conventions
2. Use proper JSON response format
3. Implement proper validation for request bodies
4. Add pagination for list endpoints (limit, offset)
5. Use environment variables for configuration

---

*Document Version: 1.0*
*Created: 2026-04-17*
*Assigned To: Senior Software Engineer (Backend)*
