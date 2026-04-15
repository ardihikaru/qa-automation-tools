# QA Automation Tool - Implementation Guide

Based on the PDF document, here is the complete implementation plan:

## 1. Recommended Tool: Playwright

**Why Playwright?**
- Fast and modern
- Excellent for authenticated/internal web apps
- Handles complex SPAs (React/Vue/Angular)
- Built-in screenshots, video recording, tracing
- Parallel execution, retries, network interception
- Auto-waiting reduces flaky tests
- Great for CI/CD

## 2. Architecture Overview

```
User Trigger (HTTP/Scheduler)
        ↓
Go Backend (App Engine)
        ├── Run Playwright Audit
        ├── Collect Audit Payload
        ├── Send to LLM (AI Analysis)
        ├── Parse Structured JSON
        ├── Build Google Chat Card
        └── Publish to Google Chat Space
```

## 3. Project Structure

```
qa-auditor/
├── main.go              # Entry point
├── audit_runner.go      # Orchestrates audit flow
├── playwright_runner.go # Browser automation
├── ai_client.go         # LLM integration
├── google_chat.go       # Google Chat API client
├── card_builder.go     # Card JSON builder
├── prompt_builder.go   # AI prompt templates
├── config.go           # Configuration
├── types.go            # Data structures
│
├── playwright/
│   ├── audit.js        # Playwright script
│   └── package.json
│
├── app.yaml            # App Engine config
└── go.mod
```

## 4. Go Data Structures (types.go)

```go
package main

type AuditSeverity string
const (
    SeverityHigh   AuditSeverity = "HIGH"
    SeverityMedium AuditSeverity = "MEDIUM"
    SeverityLow    AuditSeverity = "LOW"
)

type AuditStatus string
const (
    StatusPass     AuditStatus = "PASS"
    StatusNeedsFix AuditStatus = "NEEDS_FIX"
    StatusBlocked  AuditStatus = "BLOCKED"
)

type FrontendAuditResult struct {
    PageName    string        `json:"pageName"`
    Module      string        `json:"module"`
    Environment string        `json:"environment"`
    URL         string        `json:"url"`
    AuditTime   string        `json:"auditTime"`
    Status      AuditStatus   `json:"status"`
    Summary     AuditSummary  `json:"summary"`
    Findings    []AuditFinding `json:"findings"`
}

type AuditSummary struct {
    High   int `json:"high"`
    Medium int `json:"medium"`
    Low    int `json:"low"`
}

type AuditFinding struct {
    Severity       AuditSeverity `json:"severity"`
    Category       string        `json:"category"`
    Title          string        `json:"title"`
    Description    string        `json:"description"`
    Steps          []string      `json:"steps"`
    ExpectedResult string        `json:"expectedResult"`
    ActualResult   string        `json:"actualResult"`
    Recommendation string        `json:"recommendation"`
    Evidence       *AuditEvidence `json:"evidence,omitempty"`
}

type AuditEvidence struct {
    ScreenshotURL string `json:"screenshotUrl,omitempty"`
    VideoURL      string `json:"videoUrl,omitempty"`
    ConsoleLogURL string `json:"consoleLogUrl,omitempty"`
}
```

## 5. Playwright Data Collector Format

```json
{
  "pageContext": {
    "pageName": "Master Package",
    "module": "Master Tariff",
    "environment": "Staging",
    "url": "https://stag-app.mfardiansyah.id/master/master-tariff/master-package",
    "auditTime": "2026-04-15T09:00:00+07:00",
    "browser": "Chromium",
    "viewport": "1440x900"
  },
  "domSummary": {
    "title": "Master Package",
    "headings": ["Master Package"],
    "buttons": ["Add Package", "Export", "Refresh"],
    "inputs": ["Search Package"],
    "tables": [{
      "name": "Master Package Table",
      "columns": ["Code", "Package Name", "Status", "Action"],
      "rowCount": 10
    }],
    "modals": [],
    "alerts": []
  },
  "visibleText": ["Master Package", "Search Package", "Add Package"],
  "interactiveElements": [{
    "type": "button",
    "label": "Add Package",
    "selector": "button[data-testid='add-package']",
    "enabled": true,
    "visible": true
  }],
  "consoleLogs": [{
    "type": "error",
    "message": "Cannot read properties of undefined"
  }],
  "networkFailures": [{
    "method": "GET",
    "url": "/api/master-package?page=2",
    "status": 500
  }],
  "interactionTrace": [
    "Navigate to page",
    "Wait for table render",
    "Click Add Package",
    "Fill form",
    "Click Save"
  ],
  "screenshotMetadata": {
    "path": "/screenshots/master-package.png",
    "description": "Modal visible, save button disabled unexpectedly"
  }
}
```

## 6. AI Prompt Template (prompt_builder.go)

### System Prompt
```
You are a Senior Frontend QA Auditor specializing in healthcare web applications.
Your task is to inspect a target webpage and produce a structured frontend QA audit report.

# Objective
Evaluate the page for:
- Functional issues
- UI / UX inconsistencies
- Validation problems
- Layout / responsiveness issues
- Accessibility concerns
- Browser console errors
- Network/API failures
- Broken interactions

# Severity Rules
- HIGH: Critical workflow blocked, data cannot be saved/edited/deleted, broken page rendering
- MEDIUM: Feature partially broken, major UX issue with workaround
- LOW: Cosmetic issue, minor alignment inconsistency

# Important Rules
- DO NOT invent issues
- Only report findings supported by observable evidence
- Return STRICT JSON only
```

### User Prompt Template
```
Audit this webpage using the provided inspection data.

PAGE CONTEXT:
- Page Name: {pageName}
- Module: {module}
- Environment: {environment}
- URL: {url}

OBSERVED UI STATE:
{domSummary}

CONSOLE LOGS:
{consoleLogs}

NETWORK FAILURES:
{networkFailures}

Generate structured QA audit JSON.
```

## 7. Google Chat Card Structure

```json
{
  "cardsV2": [{
    "cardId": "qa-audit-master-package",
    "card": {
      "header": {
        "title": "Frontend QA Audit Result",
        "subtitle": "Master Package • Staging"
      },
      "sections": [{
        "widgets": [{
          "decoratedText": {
            "topLabel": "Target URL",
            "text": "Master Package Page"
          }
        }]
      }, {
        "widgets": [{
          "columns": {
            "columnItems": [
              { "widgets": [{ "decoratedText": { "text": "🔴 High: 1" } }] },
              { "widgets": [{ "decoratedText": { "text": "🟠 Medium: 2" } }] },
              { "widgets": [{ "decoratedText": { "text": "🟢 Low: 1" } }] }
            ]
          }
        }]
      }]
    }
  }]
}
```

## 8. App Engine Configuration (app.yaml)

```yaml
runtime: go122
env: standard

instance_class: F2

automatic_scaling:
  min_instances: 1
  max_instances: 3

env_variables:
  GOOGLE_CHAT_WEBHOOK_URL: "https://chat.google.com/..."
  LLM_API_URL: "https://api.llm.provider/v1/..."
  LLM_API_KEY: "your-api-key"

handlers:
- url: /.*
  script: auto
  secure: always
```

## 9. Frontend QA Audit Report Template (Markdown)

# Frontend QA Audit Report

## Audit Metadata
| Field | Value |
|-------|-------|
| Project | SUT |
| Environment | Staging |
| Base URL | https://stag-app.mfardiansyah.id |
| Module | Master Tariff |
| Page | Master Package |
| Target URL | https://stag-app.mfardiansyah.id/master/master-tariff/master-package |

## Audit Scenario
### Objective
Assess frontend quality, usability, and functional issues on the Master Package page.

### Scope
- Page rendering
- Form behavior
- Table/grid behavior
- Search/filter/pagination
- Validation messages
- Console/network errors

## Findings Summary
| Severity | Count |
|----------|-------|
| High | 0 |
| Medium | 0 |
| Low | 0 |

## Detailed Findings
### Finding #1
| Field | Value |
|-------|-------|
| Severity | HIGH/MEDIUM/LOW |
| Category | Functional/UI/UX/Cosmetic |
| Title | [Issue title] |
| Steps to Reproduce | 1. ... 2. ... 3. ... |
| Expected Result | [What should happen] |
| Actual Result | [What actually happened] |
| Evidence | Screenshot/Video URL |
| Recommendation | [Actionable fix suggestion] |

## Severity Definition
| Severity | Definition |
|----------|------------|
| High | Blocks critical workflow / causes data loss / major broken feature |
| Medium | Feature partially broken / significant UX issue / workaround exists |
| Low | Cosmetic / minor inconsistency / no workflow impact |

## Summary
- Status: NEEDS_FIX / PASS / BLOCKED
- Overall assessment notes
- Recommended retest date
