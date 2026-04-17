# AI Backend (Golang)

Processing, AI calls, database, APIs.

## Structure

| Service | Responsibility |
|---------|-------------|
| `cmd/server/` | Main entry point |
| `cmd/databasemigration/` | Run Neon database migrations |
| `internal/api/` | HTTP endpoints |
| `internal/services/learning/` | Learning cache management |
| `internal/services/skills/` | Skill generation |
| `internal/services/ai/` | Minimax integration + caching |
| `internal/services/patch/` | Code patch generation |
| `internal/services/ast/` | AST parsing for Go/TS |
| `internal/db/` | Neon connection |
| `internal/models/` | Data models |
| `internal/db/migrations.go` | Database migrations |

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/flows` | POST | Ingest logs |
| `/api/flows` | GET | List flows |
| `/api/docs` | GET | Get documentation |
| `/api/videos` | GET | List videos |
| `/api/issues` | GET | List issues |
| `/api/issues/status` | POST | Update is_fixed |
| `/api/issues/notes` | POST | Update notes |
| `/api/skills` | GET | List skills |
| `/api/search` | GET | Semantic search |
| `/api/cost` | GET | Cost analytics |
| `/api/analytics` | GET | Usage analytics |

## Tech Stack

- Golang (single binary)
- Neon (Postgres + pgvector)
- Minimax AI (abab6.5s-chat)
