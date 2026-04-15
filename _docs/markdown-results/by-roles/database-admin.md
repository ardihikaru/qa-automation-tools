# Database Administrator - Neon Schema Design

## Overview

Database schema design for Neon (PostgreSQL) to support the AI-Powered QA Platform with caching, embeddings, issues tracking, and fix status management.

---

## 1. Schema Overview

### 1.1 Database: aisystem

| Schema | Tables | Purpose |
|--------|--------|----------|
| **public** | Core data | Roles, flows, steps, analytics |
| **ai** | AI cache | Summaries, embeddings |
| **qa** | Issues | Raw issues, skills, suggestions |

---

## 2. Core Tables (public schema)

### 2.1 roles

```sql
CREATE TABLE roles (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

**Description**: System roles (admin, doctor, cashier)

**Sample Data**:
```sql
INSERT INTO roles (name, description) VALUES
('admin', 'Administrator with full access'),
('doctor', 'Medical professional'),
('cashier', 'Billing operations');
```

### 2.2 flows

```sql
CREATE TABLE flows (
    id SERIAL PRIMARY KEY,
    role_id INT REFERENCES roles(id),
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

**Description**: User flows per role (create_user, view_patient, billing)

### 2.3 steps

```sql
CREATE TABLE steps (
    id SERIAL PRIMARY KEY,
    flow_id INT REFERENCES flows(id),
    step_order INT NOT NULL,
    action TEXT NOT NULL,
    page_url TEXT,
    details JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);
```

**Description**: Individual steps in each flow

### 2.4 analytics

```sql
CREATE TABLE analytics (
    id SERIAL PRIMARY KEY,
    feature TEXT NOT NULL,
    usage_count INT DEFAULT 0,
    last_used TIMESTAMP DEFAULT NOW()
);
```

**Description**: Feature usage tracking

---

## 3. AI Schema (ai schema)

### 3.1 ai_cache

```sql
CREATE TABLE ai_cache (
    id SERIAL PRIMARY KEY,
    cache_key TEXT UNIQUE NOT NULL,
    input_hash TEXT NOT NULL,
    output TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_ai_cache_hash ON ai_cache(input_hash);
```

**Purpose**: Hash-based deduplication for AI summaries

**Usage**: 60-90% cost reduction via exact match

### 3.2 embedding_cache

```sql
CREATE TABLE embedding_cache (
    id SERIAL PRIMARY KEY,
    input_text TEXT UNIQUE NOT NULL,
    embedding VECTOR(1536) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Enable pgvector extension first
-- CREATE EXTENSION IF NOT EXISTS vector;
```

**Purpose**: Store embeddings for semantic search

**Usage**: 50-80% embedding cost reduction

### 3.3 learning_cache

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

**Purpose**: Context-aware learning from past interactions

**Key Feature**: success_score evolves over time

### 3.4 action_sequences

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

**Purpose**: Store successful multi-step plans

### 3.5 navigation_edges

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

**Purpose**: Learn navigation graph

---

## 4. QA Schema (qa schema)

### 4.1 raw_issues

```sql
CREATE TABLE raw_issues (
    id SERIAL PRIMARY KEY,
    scenario_id TEXT,
    page TEXT,
    action TEXT,
    error_text TEXT,
    dom_snapshot TEXT,
    category TEXT, -- 'frontend' | 'backend' | 'unknown'
    is_fixed BOOLEAN DEFAULT FALSE,
    notes TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_issues_created ON raw_issues(created_at);
CREATE INDEX idx_issues_category ON raw_issues(category);
CREATE INDEX idx_issues_fixed ON raw_issues(is_fixed);
```

**Purpose**: Capture detected issues with fix tracking

**Key Fields**:
- `is_fixed`: Checkbox to mark as resolved
- `notes`: QA notes for context

### 4.2 issue_suggestions (Child Table)

```sql
CREATE TABLE issue_suggestions (
    id SERIAL PRIMARY KEY,
    issue_id INT REFERENCES raw_issues(id) ON DELETE CASCADE,
    suggestion TEXT NOT NULL,
    confidence FLOAT,
    target TEXT, -- 'frontend' | 'backend'
    file_path TEXT,
    diff TEXT,
    function_name TEXT,
    test_coverage BOOLEAN DEFAULT FALSE,
    priority_score FLOAT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_suggestions_issue ON issue_suggestions(issue_id);
```

**Purpose**: Multiple suggestions per issue

**Key Fields**:
- `target`: Frontend/backend classification
- `diff`: Generated patch
- `priority_score`: Impact-based ranking

### 4.3 skills

```sql
CREATE TABLE skills (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    pattern JSONB, -- how to detect
    severity TEXT, -- 'critical' | 'high' | 'medium' | 'low'
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_skills_name ON skills(name);
```

**Purpose**: QA Skills (detection patterns)

### 4.4 skill_logs

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

**Purpose**: Track skill execution results

### 4.5 goal_events

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

**Purpose**: Goal completion tracking

### 4.6 raw_issues (captured issues)

```sql
CREATE TABLE raw_issues (
    id SERIAL PRIMARY KEY,
    scenario_id TEXT,
    page TEXT,
    action TEXT,
    error_text TEXT,
    dom_snapshot TEXT,
    category TEXT, -- 'frontend' | 'backend' | 'unknown'
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_raw_issues_created ON raw_issues(created_at);
```

**Purpose**: Issues captured from Playwright runs

---

## 5. Logging Tables (logging schema)

### 5.1 request_logs

```sql
CREATE TABLE request_logs (
    id SERIAL PRIMARY KEY,
    type TEXT NOT NULL, -- 'summary' | 'embedding'
    feature TEXT,
    tokens INT,
    cost_estimate FLOAT,
    cache_hit BOOLEAN,
    flow_id INT,
    success BOOLEAN,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_request_logs_created ON request_logs(created_at);
CREATE INDEX idx_request_logs_type ON request_logs(type);
CREATE INDEX idx_request_logs_feature ON request_logs(feature);
```

**Purpose**: Cost analytics tracking

**Key Metrics**:
- Total requests
- Total tokens
- Cost estimate
- Cache hit rate
- Success/failure

---

## 6. Views for Dashboard

### 6.1 Overview KPIs

```sql
CREATE VIEW cost_overview AS
SELECT
    COUNT(*) as total_requests,
    SUM(cost_estimate) as total_cost,
    AVG(cost_estimate) as avg_cost,
    COUNT(*) FILTER (WHERE cache_hit = true) * 1.0 / COUNT(*) as cache_hit_rate
FROM request_logs;
```

### 6.2 Fix Rate

```sql
CREATE VIEW fix_rate AS
SELECT
    COUNT(*) FILTER (WHERE is_fixed = true) as fixed,
    COUNT(*) FILTER (WHERE is_fixed = false) as open,
    COUNT(*) FILTER (WHERE is_fixed = true) * 1.0 / COUNT(*) as fix_rate
FROM raw_issues;
```

### 6.3 Issues by Target

```sql
CREATE VIEW issues_by_target AS
SELECT
    category,
    COUNT(*) as total
FROM raw_issues
GROUP BY category;
```

### 6.4 Skills Failure Summary

```sql
CREATE VIEW skills_summary AS
SELECT
    s.name,
    COUNT(*) FILTER (WHERE sl.passed = false) as fail_count,
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE sl.passed = false) * 1.0 / COUNT(*) as fail_rate
FROM skill_logs sl
JOIN skills s ON sl.skill_id = s.id
GROUP BY s.name;
```

---

## 7. Migrations

### 7.1 Initial Schema

```sql
-- Run once on new Neon project

-- Enable pgvector
CREATE EXTENSION IF NOT EXISTS vector;

-- Create schemas
CREATE SCHEMA IF NOT EXISTS ai;
CREATE SCHEMA IF NOT EXISTS qa;
CREATE SCHEMA IF NOT EXISTS logging;

-- Run table creates (see sections above)
```

### 7.2 Add is_fixed + notes (v1.1)

```sql
ALTER TABLE raw_issues
ADD COLUMN is_fixed BOOLEAN DEFAULT FALSE,
ADD COLUMN notes TEXT;
```

---

## 8. Backup & Restore

### 8.1 Backup

```bash
pg_dump "neon-connection-string" > backup.sql
```

### 8.2 Point-in-time Recovery

Neon provides automatic point-in-time recovery. Configure in Neon console.

---

## 9. Performance Tuning

### 9.1 Indexes (Already Added)

| Table | Index | Query Pattern |
|-------|-------|---------------|
| ai_cache | input_hash | Exact match lookup |
| embedding_cache | input_text | Similarity search |
| learning_cache | intent | Intent matching |
| navigation_edges | from_page | Graph traversal |
| raw_issues | is_fixed | Filter fixed/open |
| request_logs | created_at | Time filtering |

### 9.2 Partitioning (Future)

For large request_logs:

```sql
CREATE TABLE request_logs (
    LIKE logging.request_logs INCLUDING ALL
) PARTITION BY RANGE (created_at);
```

---

## 10. Connection String

Get from Neon console:

```
postgresql://user:password@ep-xxx.us-east-1.aws.neon.tech/aisystem?sslmode=require
```

Store in environment variable, not in code.

---

## 11. Schema Summary

| Schema | Tables | Purpose |
|--------|--------|----------|
| public | 4 | Core + analytics |
| ai | 5 | AI caching + learning |
| qa | 6 | Issues + skills |
| logging | 1 | Cost tracking |
| **Total** | **16** | Full system |

---

*Document Version: 1.0*
*Source: Playwright-for-System-Documentation.pdf*