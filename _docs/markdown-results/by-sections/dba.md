# Database Design (DBA)

## Core Tables
- flows
- steps
- roles

## AI Cache
- ai_cache
- embedding_cache

## Learning Cache
- learning_cache

## Diagram
```mermaid
erDiagram
flows ||--o{ steps : has
flows ||--o{ learning_cache : improves
```

## Optimization
- Index embeddings
- Use pgvector
