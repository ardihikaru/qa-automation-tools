# System Architect

## Architecture Overview

```mermaid
flowchart LR
A[Playwright] --> B[Golang API]
B --> C[Neon DB]
B --> D[AI Layer]
C --> E[Dashboard]
```

## Components
- Explorer (Node)
- Backend (Go)
- DB (Neon + pgvector)
- Dashboard (React)

## Design Principles
- Decoupled services
- AI-first pipeline
- Learning-based system
