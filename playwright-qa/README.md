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