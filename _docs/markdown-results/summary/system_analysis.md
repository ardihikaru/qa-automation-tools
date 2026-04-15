# AI-Powered System Documentation & QA Intelligence Platform

## 1. Overview

This system is an **AI-driven platform** that automatically explores
applications, generates documentation, detects issues, and improves over
time using learning mechanisms.

------------------------------------------------------------------------

## 2. Core Objectives

-   Automatically explore UI (Playwright)
-   Generate structured documentation (AI)
-   Detect bugs using reusable skills
-   Learn from past executions (learning cache)
-   Optimize AI cost using caching
-   Provide analytics & dashboard insights

------------------------------------------------------------------------

## 3. High-Level Architecture

    Playwright Explorer
            ↓
       Golang Backend (API + Workers)
            ↓
       Neon Database (Postgres + pgvector)
            ↓
       Minimax AI (Summaries + Embeddings)
            ↓
       React Dashboard

------------------------------------------------------------------------

## 4. Key Components

### 4.1 Playwright Explorer

-   Executes flows (multi-role)
-   Records:
    -   Video
    -   Screenshots
    -   Logs
-   Supports:
    -   Scripted flows
    -   Autonomous exploration
    -   Guided exploration (learning-based)
    -   Multi-step planning agent

------------------------------------------------------------------------

### 4.2 Golang Backend

Responsibilities: - Ingest logs - Call AI (Minimax) - Store structured
data - Provide APIs - Manage learning & caching

Endpoints: - POST /flows - GET /flows - GET /search - GET /analytics -
GET /cost

------------------------------------------------------------------------

### 4.3 Neon Database (Postgres)

#### Core Tables

-   roles
-   flows
-   steps

#### AI Cache

-   ai_cache
-   embedding_cache

#### Learning System

-   learning_cache
-   flow_outcomes
-   action_sequences

#### QA System

-   skills
-   skill_logs
-   learned_skills
-   raw_issues

#### Analytics

-   request_logs
-   analytics
-   goal_events

------------------------------------------------------------------------

### 4.4 AI Layer (Minimax)

Used for: - Summarization - Embeddings - Intent extraction - Skill
generation

------------------------------------------------------------------------

### 4.5 Dashboard (React)

Features: - Documentation viewer - Video playback - Semantic search -
Cost analytics - QA reports - Goal confidence visualization

------------------------------------------------------------------------

## 5. Documentation Pipeline

1.  Playwright runs scenarios
2.  Capture logs & steps
3.  Send to backend
4.  AI generates:
    -   Feature description
    -   Flow explanation
5.  Save as Markdown

------------------------------------------------------------------------

## 6. Learning System

### Levels

1.  Exact cache (hash)
2.  Similarity reuse (embedding)
3.  Learning cache (context-aware)

### Key Idea

System evolves from: - Random exploration → Guided → Planning agent

------------------------------------------------------------------------

## 7. Autonomous Explorer

### Modes

-   Random
-   Guided (learning-based)
-   Planning (multi-step)

### Strategy

-   Detect actions
-   Score actions
-   Balance exploration vs exploitation

------------------------------------------------------------------------

## 8. Multi-Step Planning

Instead of: \> "What to click next?"

We use: \> "What sequence leads to goal?"

Steps: 1. Generate action tree 2. Simulate sequences 3. Score sequences
4. Execute best path

------------------------------------------------------------------------

## 9. Goal Detection

### Signals

-   UI success message
-   URL change
-   New data
-   Network response

### Output

-   Goal
-   Confidence score

------------------------------------------------------------------------

## 10. QA Skill System

### Predefined Skills

#### 1. Max Length Validation

-   Detect long input failure

#### 2. Required Field Validation

-   Detect bypass of required fields

#### 3. Data Persistence

-   Detect data loss after reload

#### 4. Multi-tab Save Issue

-   Detect unsaved tab data

------------------------------------------------------------------------

### Skill Execution Flow

-   Run after each scenario
-   Log results
-   Store in DB
-   Aggregate per scenario

------------------------------------------------------------------------

## 11. AI-Generated Skills

### Flow

1.  Capture raw issues
2.  Group similar issues
3.  Generate skill using AI
4.  Store in DB
5.  Reuse automatically

------------------------------------------------------------------------

## 12. Cost Optimization (Neon Only)

Techniques: - Hash-based caching - Input normalization - Chunking -
Embedding reuse - Similarity reuse

------------------------------------------------------------------------

## 13. Learning Cache

Stores: - Intent - Embedding - Success score - Usage count

### Decision Logic

-   Reuse if similarity high
-   Prefer high success flows
-   Apply decay for old data

------------------------------------------------------------------------

## 14. Analytics & Dashboard

### Metrics

-   Total cost
-   Cache hit rate
-   Cost per feature
-   Failed flows
-   Goal success rate

------------------------------------------------------------------------

## 15. Heatmap & Prioritization

### Heatmap

-   Identify broken features

### Prioritization

-   Based on:
    -   Frequency
    -   Severity
    -   Cost impact

------------------------------------------------------------------------

## 16. Final System Capabilities

-   Automated UI exploration
-   AI-generated documentation
-   Self-learning QA system
-   Cost-aware AI usage
-   Intelligent planning agent

------------------------------------------------------------------------

## 17. Key Insight

This system is not just testing.

It is:

**AI Reverse Engineering + QA Intelligence Platform**
