# TODO — Playwright-QA (Detailed Implementation Guide)

## Overview

This document provides a **deep technical implementation guide** for the Playwright-QA system.

The goal is to transform the current system from:

> ❌ Script-based automation
> into:
> ✅ Intelligent self-learning QA agent

---

## 0. System Context

Playwright-QA is part of:

* Exploration Engine
* QA Skill Executor
* Learning Agent
* Issue Detection System

It integrates with:

* AI Backend (Golang APIs)
* Neon Database (learning + issues)
* Minimax AI (optional intelligence)

---

# 🔴 P0 — CRITICAL IMPLEMENTATION

---

## 1. Guided Exploration Engine

### Objective

Replace random exploration with **learning-based decision making**

---

### Architecture

```
Explorer
  ├── Action Extractor
  ├── State Encoder
  ├── Learning Query
  └── Decision Engine
```

---

### Implementation Steps

#### Step 1 — Extract Available Actions

```ts
type Action = {
  type: "click" | "input" | "navigate"
  selector: string
  text?: string
  url?: string
}
```

Use:

```ts
await page.locator("button, a, input, [role='button']")
```

---

#### Step 2 — Encode Current State

```ts
type State = {
  url: string
  title: string
  visibleText: string[]
}
```

---

#### Step 3 — Query Learning API

```
GET /api/learning?query=<state>
```

---

#### Step 4 — Scoring Formula

```
score =
  similarity * 0.5 +
  success_score * 0.3 +
  log(usage_count) * 0.1
```

---

#### Step 5 — Decision Policy

* 80% → best action (exploit)
* 20% → random action (explore)

---

#### Step 6 — Store Result

```
POST /api/learning
```

Payload:

```json
{
  "state": "...",
  "action": "...",
  "success_score": 0.8
}
```

---

### Edge Cases

* No learning data → fallback random
* Repeated failures → penalize score
* Infinite loops → max depth = 20

---

## 2. Multi-Step Planning Engine

### Objective

Enable **lookahead decision making**

---

### Strategy

Instead of:

> act → react

We do:

> simulate → score → act

---

### Implementation

#### Step 1 — Generate Action Tree

Depth: 2–4

```ts
type Node = {
  action: Action
  children: Node[]
}
```

---

#### Step 2 — Simulate

* Clone browser context
* Execute actions virtually

---

#### Step 3 — Score Path

Criteria:

* Goal likelihood
* Success history
* Error risk

---

#### Step 4 — Execute

Only execute:
👉 FIRST step of best path

Then re-plan

---

### Risks

* Simulation cost too high → limit depth
* State mismatch → must reset context

---

## 3. Goal Detection System

### Objective

Detect when a flow **successfully completes**

---

### Signals

#### 1. UI Text

* "Success"
* "Saved"
* "Created"

#### 2. URL Change

```ts
if (newUrl !== oldUrl)
```

#### 3. DOM Changes

* New elements appear
* Modal disappears

#### 4. Network Response

* 200 / 201 responses

---

### Confidence Score

```
confidence =
  text_match * 0.3 +
  url_change * 0.2 +
  dom_change * 0.2 +
  network_success * 0.3
```

---

### API Integration

```
POST /api/goal-events
```

```json
{
  "goal": "create patient",
  "confidence": 0.87,
  "signals": {...}
}
```

---

### Edge Cases

* False positives → require threshold > 0.7
* Async delays → wait 2–3s before evaluation

---

## 4. Backend Integration (MANDATORY)

### Endpoints

#### Issues

```
POST /api/issues
```

#### Skill Logs

```
POST /api/skill-logs
```

#### Goal Events

```
POST /api/goal-events
```

---

### Implementation Pattern

```ts
await fetch("/api/issues", {
  method: "POST",
  body: JSON.stringify(issue)
})
```

---

### Retry Strategy

* Retry 3x
* Exponential backoff

---

## 5. Skill Execution System

### Objective

Run predefined QA validations

---

### Required Skills

#### 1. Max Length

* Input long string
* Detect crash / error

#### 2. Required Field

* Submit empty form

#### 3. Data Persistence

* Reload page
* Verify data

#### 4. Multi-tab Save

* Switch tabs
* Validate data consistency

---

### Interface

```ts
interface Skill {
  name: string
  execute(page: Page): Promise<Result>
}
```

---

### Logging

```
POST /api/skill-logs
```

---

---

# 🟠 P1 — HIGH PRIORITY

---

## 6. Result Submission Pipeline

### Flow

```
Explorer → Detect Issue → Submit → Backend → DB
```

---

### Issue Format

```json
{
  "page": "/patients",
  "action": "submit form",
  "error": "500 internal server error",
  "category": "backend"
}
```

---

## 7. Skill Automation Scheduler

### Strategy

* Run every X minutes
* Queue-based execution

---

---

# 🟡 P2 — MEDIUM PRIORITY

---

## 8. Auto Skill Discovery

### Flow

```
Repeated Issues → Embedding → Cluster → Generate Skill
```

---

## 9. Action Sequence Storage

```
POST /api/action-sequences
```

Store:

* intent
* steps
* success_score

---

---

# 🟢 P3 — LOW PRIORITY

---

## 10. Bug Heatmap

* Count failures per page
* Aggregate severity

---

# 🧪 Testing Strategy

## Unit Tests

* Action extraction
* Scoring engine
* Goal detection

## Integration Tests

* Full flow execution
* API communication

## Stress Testing

* 100+ flows
* Detect performance bottlenecks

---

# 🚀 Final Notes

This system is evolving into:

> **Autonomous QA Agent (not just automation)**

Key success factor:
👉 Learning quality > algorithm complexity

---

# ✅ Definition of Done

* Guided exploration working
* Goal detection reliable (>80%)
* Issues logged correctly
* Skills executed automatically
* Backend fully integrated

---

**End of Document**
