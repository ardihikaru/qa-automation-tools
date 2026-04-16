# QA Automation Tools

A comprehensive QA automation system with AI-powered features for testing and monitoring healthcare web applications.

## Table of Contents

- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Running Backend Locally](#running-backend-locally)
- [Running Dashboard Locally](#running-dashboard-locally)
- [Running Both Services](#running-both-services)
- [Environment Variables](#environment-variables)
- [API Documentation](#api-documentation)
- [Deployment](#deployment)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Architecture                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   User                                                           │
│    │                                                             │
│    ▼                                                             │
│   ┌──────────────────┐      ┌──────────────────┐               │
│   │   Dashboard     │─────▶│   ai-backend      │               │
│   │   (React/UI)   │      │   (Go API)        │               │
│   │   :3000        │◀─────│   :8080          │               │
│   └──────────────────┘      └────────┬────────┘               │
│                                         │                        │
│                                         ▼                        │
│                                  ┌──────────────────┐            │
│                                  │   Neon Database │            │
│                                  │   (PostgreSQL)  │            │
│                                  └──────────────────┘            │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Prerequisites

### Required Software

| Software | Version | Description |
|----------|---------|-------------|
| [Go](https://go.dev/dl/) | 1.23+ | Backend runtime |
| [Node.js](https://nodejs.org/) | 18+ | Dashboard runtime |
| [Docker](https://docs.docker.com/get-docker/) | Latest | Container runtime |
| [PostgreSQL Client](https://www.postgresql.org/download/) | 15+ | Database client (optional) |

### Required Accounts

- [Neon](https://neon.tech/) - PostgreSQL database
- [MiniMax](https://minimax.io/) - AI service (for some features)
- [Google Cloud Platform](https://cloud.google.com/) - Cloud deployment

---

## Quick Start

### Option 1: Docker Compose (Recommended)

```bash
# Start all services
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f backend
docker-compose logs -f dashboard
```

### Option 2: Local Development

```bash
# Terminal 1 - Start backend
cd ai-backend
cp .env.example .env
# Edit .env with your credentials
go run cmd/server/main.go

# Terminal 2 - Start dashboard
cd dashboard
npm install
npm run dev
```

---

## Running Backend Locally

### Step 1: Configure Environment

```bash
cd ai-backend

# Copy example environment file
cp .env.example .env
```

### Step 2: Edit .env File

Open `.env` and configure the following:

```bash
# ------------------------------------------------------------
# DATABASE (PostgreSQL - Neon)
# ------------------------------------------------------------
# Get from your Neon console: https://console.neon.tech
DATABASE_URL=postgresql://username:password@host/neondb?sslmode=require

# ------------------------------------------------------------
# MINIMAX AI (Optional - for AI features)
# ------------------------------------------------------------
# Get from https://platform.minimax.io/
MINIMAX_API_KEY=sk-api-your-key-here

# ------------------------------------------------------------
# SERVER CONFIG
# ------------------------------------------------------------
PORT=8080
BACKEND_HOST=0.0.0.0
```

### Step 3: Install Dependencies

```bash
cd ai-backend
go mod download
```

### Step 4: Run the Backend

```bash
# Option 1: Using Go run
go run cmd/server/main.go

# Option 2: Build and run
go build -o server ./cmd/server
./server

# Option 3: Using Air (live reload)
go install github.com/cosmtrek/air@latest
air
```

### Step 5: Verify Backend is Running

```bash
# Health check
curl http://localhost:8080/health

# Expected response:
# {"status": "ok", "database": "connected"}
```

---

## Running Dashboard Locally

### Step 1: Install Dependencies

```bash
cd dashboard
npm install
```

### Step 2: Configure Environment

```bash
cd dashboard

# Copy example environment file (if needed)
# The .env file already has sensible defaults
```

### Step 3: Run the Dashboard

```bash
# Development mode (with hot reload)
npm run dev

# Or build and preview
npm run build
npm run preview
```

### Step 4: Access the Dashboard

Open your browser to:
- **Dashboard UI**: http://localhost:3000
- **Backend API**: http://localhost:8080

---

## Running Both Services

### Option 1: Using Concurrently

```bash
# Terminal 1 - Backend
cd ai-backend
go run cmd/server/main.go

# Terminal 2 - Dashboard
cd dashboard
npm run dev
```

### Option 2: Using Docker Compose

```bash
# Create docker-compose.yml in project root
cat > docker-compose.yml << 'EOF'
version: '3.8'

services:
  backend:
    build: ./ai-backend
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=${DATABASE_URL}
      - MINIMAX_API_KEY=${MINIMAX_API_KEY}
    volumes:
      - ./ai-backend/.env:/app/.env

  dashboard:
    build: ./dashboard
    ports:
      - "3000:80"
    depends_on:
      - backend

EOF

# Run services
docker-compose up -d
```

---

## Environment Variables

### Backend (.env)

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `DATABASE_URL` | ✅ | Neon PostgreSQL connection string | `postgresql://user:pass@host/db?sslmode=require` |
| `MINIMAX_API_KEY` | ❌ | MiniMax AI API key | `sk-api-...` |
| `AI_MODEL` | ❌ | AI model name | `MiniMax-M2.5` |
| `AI_BASE_URL` | ❌ | AI API base URL | `https://api.minimax.io/v1` |
| `PORT` | ❌ | Server port | `8080` |
| `API_SECRET_KEY` | ❌ | API authentication | `your-secret-key` |
| `CORS_ENABLED` | ❌ | Enable CORS | `true` |
| `CORS_ORIGINS` | ❌ | Allowed origins | `*` |

### Dashboard (.env)

| Variable | Required | Description | Default |
|----------|----------|-------------|---------|
| `VITE_API_URL` | ✅ | Backend API URL | `http://localhost:8080` |
| `VITE_DEV_SERVER_PORT` | ❌ | Dev server port | `3000` |
| `VITE_APP_TITLE` | ❌ | Application title | `QA Automation Dashboard` |
| `VITE_APP_DEBUG` | ❌ | Debug mode | `false` |

---

## API Documentation

### Base URL

| Environment | URL |
|-------------|-----|
| Local | `http://localhost:8080` |
| Production | `https://ai-backend-1087642958941.asia-southeast1.run.app` |

### Endpoints

| Endpoint | Method | Description | Auth |
|----------|--------|-------------|------|
| `/health` | GET | Health check | ❌ |
| `/api/flows` | GET | List flows | ❌ |
| `/api/flows` | POST | Create flow | ❌ |
| `/api/issues` | GET | List issues | ❌ |
| `/api/issues` | POST | Create issue | ❌ |
| `/api/skills` | GET | List skills | ❌ |
| `/api/search` | GET | Semantic search | ❌ |
| `/api/cost/overview` | GET | Cost overview | ❌ |
| `/api/cost/trend` | GET | Cost trend | ❌ |
| `/api/cost/efficiency` | GET | Cost efficiency | ❌ |
| `/api/cost/by-feature` | GET | Cost by feature | ❌ |
| `/api/analytics` | GET | Usage analytics | ❌ |
| `/api/learning` | GET | Learning records | ❌ |

### Example API Calls

```bash
# Health check
curl http://localhost:8080/health

# Get all flows
curl http://localhost:8080/api/flows

# Create a flow
curl -X POST http://localhost:8080/api/flows \
  -H "Content-Type: application/json" \
  -d '{"role_id":1,"name":"Test Flow","description":"A test flow"}'

# Get issues
curl http://localhost:8080/api/issues

# Search (requires q parameter)
curl "http://localhost:8080/api/search?q=login"
```

---

## Deployment

### Deploy to Google Cloud Run

#### 1. Build and Push Docker Image

```bash
# Build backend
cd ai-backend
docker build -t gcr.io/qa-automation-tools-sismedika/ai-backend:latest .
docker push gcr.io/qa-automation-tools-sismedika/ai-backend:latest

# Build dashboard
cd dashboard
docker build -t gcr.io/qa-automation-tools-sismedika/dashboard:latest .
docker push gcr.io/qa-automation-tools-sismedika/dashboard:latest
```

#### 2. Deploy to Cloud Run

```bash
# Deploy backend
gcloud run deploy ai-backend \
  --image gcr.io/qa-automation-tools-sismedika/ai-backend:latest \
  --platform managed \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --set-env-vars "DATABASE_URL=postgresql://user:pass@host/db?sslmode=require"

# Deploy dashboard
gcloud run deploy dashboard \
  --image gcr.io/qa-automation-tools-sismedika/dashboard:latest \
  --platform managed \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --port=80
```

#### 3. Get Service URLs

```bash
gcloud run services describe ai-backend --region asia-southeast1 --format="value(status.url)"
gcloud run services describe dashboard --region asia-southeast1 --format="value(status.url)"
```

---

## Testing

### API Testing

```bash
# Test all endpoints
curl http://localhost:8080/health
curl http://localhost:8080/api/flows
curl http://localhost:8080/api/issues
curl http://localhost:8080/api/skills

# Test with jq for pretty output
curl -s http://localhost:8080/api/flows | jq .
```

### Playwright Testing

```bash
# Install Playwright
cd playwright-qa
npm install

# Run tests
npm run test

# Run with UI
npm run test:ui
```

---

## Troubleshooting

### Backend Issues

#### Database Connection Failed

```bash
# Check DATABASE_URL format
echo $DATABASE_URL

# Test database connection
psql "$DATABASE_URL" -c "SELECT 1"
```

#### Port Already in Use

```bash
# Find process using port 8080
lsof -i :8080

# Kill the process
kill -9 <PID>
```

#### Missing Dependencies

```bash
cd ai-backend
go mod tidy
go mod download
```

### Dashboard Issues

#### CORS Errors

Make sure the backend has CORS enabled:

```bash
# In .env file
CORS_ENABLED=true
CORS_ORIGINS=*
```

#### Cannot Connect to Backend

```bash
# Check backend is running
curl http://localhost:8080/health

# Check VITE_API_URL in dashboard/.env
cat dashboard/.env | grep VITE_API_URL
```

### Docker Issues

#### Container Won't Start

```bash
# View logs
docker logs <container-name>

# Check port mapping
docker ps

# Rebuild without cache
docker build --no-cache -t myimage .
```

---

## File Structure

```
qa-automation-tools/
├── ai-backend/               # Go backend
│   ├── cmd/server/           # Entry point
│   ├── internal/
│   │   ├── api/              # HTTP handlers
│   │   ├── config/          # Configuration
│   │   ├── db/              # Database
│   │   ├── models/          # Data models
│   │   └── services/       # Business logic
│   ├── .env                 # Environment variables
│   └── Dockerfile
│
├── dashboard/               # React frontend
│   ├── src/                # Source code
│   ├── nginx.conf          # Nginx config
│   ├── .env               # Environment variables
│   └── Dockerfile
│
├── playwright-qa/          # Playwright tests
├── config.yaml             # Main configuration
├── docker-compose.yml      # Docker compose
└── README.md             # This file
```

---

## Additional Resources

- [Backend API Documentation](./ai-backend/README.md)
- [Dashboard Documentation](./dashboard/README.md)
- [Deployment Tutorial](./deployment-tutorial.md)
- [MiniMax API Documentation](./MiniMax-API-Endpoint.pdf)
- [Playwright Documentation](./Playwright-for-System-Documentation.pdf)

---

## License

Copyright © 2025 Sismedika. All rights reserved.