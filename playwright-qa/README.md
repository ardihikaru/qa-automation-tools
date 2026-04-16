# Playwright QA System

Autonomous exploration and bug detection.

## Structure

| Module | Responsibility |
|--------|-------------|
| `explorer/` | Guided exploration, planning, goal detection |
| `skills/builtin/` | Hardcoded skills (max_length, required_field, etc.) |
| `skills/learned/` | AI-generated skills |
| `runner/` | Main execution loop |
| `logger/` | Send logs to backend |
| `config/` | Configuration |

## Key Feature

Full intelligence - exploration, learning, skills, goal detection.

## Exploration Modes

| Mode | Description |
|------|------------|
| `random` | Random element click |
| `guided` | Learning-based action selection |
| `planning` | Multi-step planning agent |

## Built-in QA Skills

1. **Max Length** - Detect textarea overflow
2. **Required Field** - Detect empty required input bypass
3. **Data Persistence** - Detect data loss after reload
4. **Multi-tab Save** - Detect unsaved tab data

## API Testing

### Production API

Production API endpoint: https://ai-backend-000000000000.asia-southeast1.run.app

### Testing Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/flows` | GET/POST | CRUD flows |
| `/api/issues` | GET/POST | Issue tracking |
| `/api/skills` | GET | QA skills |
| `/api/search` | GET | Search learning cache |
| `/api/cost/*` | GET | Cost analytics |

### Manual Testing

```bash
# Base URL
BASE_URL="https://ai-backend-000000000000.asia-southeast1.run.app"

# Test health
curl $BASE_URL/health

# Test flows
curl $BASE_URL/api/flows

# Test search
curl "$BASE_URL/api/search?q=login"
```

### Test Results

See `TEST-RESULTS.md.example` for detailed test results including:
- Status codes for all endpoints
- Known issues (NULL values, status code mismatches)
- Error handling validation