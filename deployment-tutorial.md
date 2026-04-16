# Google Cloud Run Deployment Tutorial

This document describes how to deploy the QA Automation Tools (ai-backend and dashboard) to Google Cloud Run.

## Prerequisites

1. **Google Cloud SDK** - Install from https://cloud.google.com/sdk/docs/install
2. **Docker** - Install from https://docs.docker.com/get-docker/
3. **gcloud CLI** - Must be authenticated and configured

## Project Configuration

- **Project ID**: `qa-automation-tools-sismedika`
- **Region**: `asia-southeast1` (Jakarta)
- **Container Registry**: `gcr.io/qa-automation-tools-sismedika`

## Step-by-Step Deployment

### Step 1: Authenticate with Google Cloud

```bash
# Login to Google Cloud
gcloud auth login

# Set the active project
gcloud config set project qa-automation-tools-sismedika
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
docker build -t gcr.io/qa-automation-tools-sismedika/ai-backend:latest .
docker push gcr.io/qa-automation-tools-sismedika/ai-backend:latest
```

#### Build Dashboard

```bash
cd dashboard
docker build -t gcr.io/qa-automation-tools-sismedika/dashboard:latest .
docker push gcr.io/qa-automation-tools-sismedika/dashboard:latest
```

### Step 5: Deploy to Cloud Run

#### Deploy ai-backend

```bash
gcloud run deploy ai-backend \
  --image gcr.io/qa-automation-tools-sismedika/ai-backend:latest \
  --platform managed \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --set-env-vars "DATABASE_URL=postgresql://neondb_owner:YOUR_PASSWORD@ep-long-flower-a1asu9tt-pooler.ap-southeast-1.aws.neon.tech/neondb?sslmode=require" \
  --set-env-vars "MINIMAX_API_KEY=YOUR_API_KEY" \
  --set-env-vars "AI_MODEL=MiniMax-M2.5" \
  --set-env-vars "AI_BASE_URL=https://api.minimax.io/v1" \
  --set-env-vars "API_SECRET_KEY=your-secret-key" \
  --set-env-vars "CORS_ENABLED=true" \
  --set-env-vars "CORS_ORIGINS=*"
```

**Note**: The `PORT` environment variable is automatically set by Cloud Run - do not set it manually.

#### Deploy Dashboard

```bash
gcloud run deploy dashboard \
  --image gcr.io/qa-automation-tools-sismedika/dashboard:latest \
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
| `DATABASE_URL` | Neon PostgreSQL connection string | `postgresql://user:pass@host/db?sslmode=require` |
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
docker build -t gcr.io/qa-automation-tools-sismedika/ai-backend:latest .
docker push gcr.io/qa-automation-tools-sismedika/ai-backend:latest

# Redeploy (will use new image automatically)
gcloud run deploy ai-backend \
  --image gcr.io/qa-automation-tools-sismedika/ai-backend:latest \
  --platform managed \
  --region asia-southeast1
```

## Service URLs

After deployment, the services will be available at:

- **ai-backend**: `https://ai-backend-1087642958941.asia-southeast1.run.app`
- **Dashboard**: `https://dashboard-1087642958941.asia-southeast1.run.app`

## Architecture

```
User → Dashboard (Cloud Run) → /api → ai-backend (Cloud Run)
                                    ↓
                              Neon Database
```

The dashboard proxies API requests to the ai-backend via nginx configuration. The nginx config is in `dashboard/nginx.conf`.