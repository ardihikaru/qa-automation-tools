# AI Layer

## Functions
- Summarization
- Embeddings
- Clustering

## Pipeline
```mermaid
flowchart LR
A[Logs] --> B[Normalize]
B --> C[Embedding]
C --> D[Store]
```

## Optimization
- Cache
- Deduplication
