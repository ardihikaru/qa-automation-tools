# Google Cloud Run Deployment Tutorial

> **Legacy.** Production now runs as a Docker Compose stack on app-host (see [deploy/qa-tool/README.md](deploy/qa-tool/README.md)). The database is MySQL 8 (schema `qa_automation` on db-host `10.8.0.4:5306`), configured with `DB_HOST`/`DB_PORT`/`DB_USER`/`DB_PASSWORD`/`DB_NAME` or a MySQL `DATABASE_URL` DSN.

This document describes how to deploy the QA Automation Tools (ai-backend and dashboard) to Google Cloud Run.

## Prerequisites

1. **Google Cloud SDK** - Install from https://cloud.google.com/sdk/docs/install
2. **Docker** - Install from https://docs.docker.com/get-docker/
3. **gcloud CLI** - Must be authenticated and configured

## Project Configuration

- **Project ID**: `qa-automation-tools-example-org`
- **Region**: `asia-southeast1` (Jakarta)
- **Container Registry**: `gcr.io/qa-automation-tools-example-org`

## Step-by-Step Deployment

### Step 1: Authenticate with Google Cloud

```bash
# Login to Google Cloud
gcloud auth login

# Set the active project
gcloud config set project qa-automation-tools-example-org
```

### Step 2: Enable Required APIs

```bash
# Enable Cloud Run API
gcloud services enable run.googleapis.com

# Enable Container Registry API
gcloud services enable containerregistry.googleapis.com
```

### Step 3: Configure Docker Authentication

```bash
# Configure Docker to use gcloud as credential helper
gcloud auth configure-docker
```

### Step 4: Build and Push Docker Images

#### Build ai-backend

```bash
cd ai-backend
docker build -t gcr.io/qa-automation-tools-example-org/ai-backend:latest .
docker push gcr.io/qa-automation-tools-example-org/ai-backend:latest
```

#### Build Dashboard

```bash
cd dashboard
docker build -t gcr.io/qa-automation-tools-example-org/dashboard:latest .
docker push gcr.io/qa-automation-tools-example-org/dashboard:latest
```

### Step 5: Deploy to Cloud Run

#### Deploy ai-backend

```bash
gcloud run deploy ai-backend \
  --image gcr.io/qa-automation-tools-example-org/ai-backend:latest \
  --platform managed \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --set-env-vars "DATABASE_URL=qa_automation:YOUR_PASSWORD@tcp(10.8.0.4:5306)/qa_automation?parseTime=true" \
  --set-env-vars "MINIMAX_API_KEY=YOUR_API_KEY" \
  --set-env-vars "AI_MODEL=MiniMax-M2.5" \
  --set-env-vars "AI_BASE_URL=https://api.minimax.io/v1" \
  --set-env-vars "API_SECRET_KEY=your-secret-key" \
  --set-env-vars "CORS_ENABLED=true" \
  --set-env-vars "CORS_ORIGINS=*"
```

**Important Notes:**
- `DATABASE_URL` is a go-sql-driver/mysql DSN (`user:pass@tcp(host:port)/dbname?parseTime=true`); you can set `DB_*` instead
- The `PORT` environment variable is automatically set by Cloud Run - do not set it manually
- Replace `YOUR_PASSWORD` and `YOUR_API_KEY` with actual credentials from your `.env` file

#### Deploy Dashboard

```bash
gcloud run deploy dashboard \
  --image gcr.io/qa-automation-tools-example-org/dashboard:latest \
  --platform managed \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --port=80
```

### Step 6: Verify Deployment

```bash
# Check deployed services
gcloud run services list --platform managed --region asia-southeast1

# Get service URLs
gcloud run services describe ai-backend --platform managed --region asia-southeast1 --format="value(status.url)"
gcloud run services describe dashboard --platform managed --region asia-southeast1 --format="value(status.url)"
```

## Environment Variables

### ai-backend

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | MySQL DSN (go-sql-driver/mysql format); alternative to `DB_*` | `user:pass@tcp(host:port)/qa_automation?parseTime=true` |
| `MINIMAX_API_KEY` | Minimax API key | `sk-api-...` |
| `AI_MODEL` | AI model name | `MiniMax-M2.5` |
| `AI_BASE_URL` | AI API base URL | `https://api.minimax.io/v1` |
| `API_SECRET_KEY` | Secret key for API authentication | `your-secret-key` |
| `CORS_ENABLED` | Enable CORS | `true` |
| `CORS_ORIGINS` | Allowed CORS origins | `*` |

### dashboard

The dashboard uses nginx and does not require environment variables. It proxies `/api` requests to the ai-backend service.

## Troubleshooting

### Container Failed to Start

If you see the error "container failed to start", check:

1. **Port Configuration**: Make sure the container listens on the port specified by the `PORT` environment variable (default 8080 for Go, 80 for nginx)
2. **Health Check**: Ensure the container has a health check endpoint
3. **Logs**: View logs in Google Cloud Console under Cloud Run > Logs

```bash
# View logs
gcloud logs read --resource=cloud_run_revision --filter="serviceName=dashboard"
```

### Permission Issues

If you encounter permission errors:

```bash
# Ensure Cloud Run invoker permission is granted
gcloud run services add-iam-policy-binding SERVICE_NAME \
  --member="allUsers" \
  --role="roles/run.invoker" \
  --region=asia-southeast1 \
  --platform=managed
```

### Update Deployment

To update an existing deployment, simply rebuild and redeploy:

```bash
# Rebuild
docker build -t gcr.io/qa-automation-tools-example-org/ai-backend:latest .
docker push gcr.io/qa-automation-tools-example-org/ai-backend:latest

# Redeploy (will use new image automatically)
gcloud run deploy ai-backend \
  --image gcr.io/qa-automation-tools-example-org/ai-backend:latest \
  --platform managed \
  --region asia-southeast1
```

## Service URLs

After deployment, the services will be available at:

- **ai-backend**: `https://ai-backend-000000000000.asia-southeast1.run.app`
- **Dashboard**: `https://dashboard-000000000000.asia-southeast1.run.app`

## Quick Deploy Commands

### Rebuild and Deploy ai-backend

```bash
# Build
cd ai-backend
docker build -t gcr.io/qa-automation-tools-example-org/ai-backend:latest .
docker push gcr.io/qa-automation-tools-example-org/ai-backend:latest

# Deploy (with all environment variables)
gcloud run deploy ai-backend \
  --image gcr.io/qa-automation-tools-example-org/ai-backend:latest \
  --platform managed \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --set-env-vars "DATABASE_URL=qa_automation:YOUR_PASSWORD@tcp(10.8.0.4:5306)/qa_automation?parseTime=true,MINIMAX_API_KEY=YOUR_API_KEY,AI_MODEL=MiniMax-M2.5,AI_BASE_URL=https://api.minimax.io/v1,API_SECRET_KEY=your-secret-key,CORS_ENABLED=true,CORS_ORIGINS=*"
```

### Rebuild and Deploy Dashboard

```bash
# Build
cd dashboard
docker build -t gcr.io/qa-automation-tools-example-org/dashboard:latest .
docker push gcr.io/qa-automation-tools-example-org/dashboard:latest

# Deploy
gcloud run deploy dashboard \
  --image gcr.io/qa-automation-tools-example-org/dashboard:latest \
  --platform managed \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --port=80
```

```
User → Dashboard (Cloud Run) → /api → ai-backend (Cloud Run)
                                    ↓
                              MySQL (qa_automation)
```

The dashboard proxies API requests to the ai-backend via nginx configuration. The nginx config is in `dashboard/nginx.conf`.