# Dashboard (React)

## Overview

UI for exploring docs, viewing issues, tracking fixes. A Vite + React 18 single-page
app (`package.json` name `qa-dashboard`) that renders the QA Automation backend's data
behind a cookie-session login. It holds no data of its own: every view reads from the
backend through same-origin `/api/*` calls (see [ai-backend API](../ai-backend/internal/api/README.md)).

- **Dev**: `npm run dev` starts Vite (`vite.config.ts`), port `VITE_DEV_SERVER_PORT`
  (default `3000`), proxying `/api` to `VITE_API_URL` (default `http://localhost:8080`).
  Extra reverse-proxy hostnames go in `VITE_ALLOWED_HOSTS` (comma-separated).
- **Production**: `Dockerfile` builds `dist/` with `npm run build` and serves it from
  `nginx:alpine`. The current production deployment is the Docker Compose stack in
  [deploy/qa-tool](../deploy/qa-tool/README.md) on `app-host`, behind the edge-host
  nginx edge at https://qa.mfardiansyah.id. That stack builds with
  `NGINX_CONF=nginx.conf.example` and mounts `deploy/qa-tool/nginx.dashboard.conf`
  over it, which serves the SPA and proxies `/api` and `/health` to the `backend`
  service.
- **Legacy**: `cloudbuild.yaml` (Cloud Run deploy of `qa-dashboard`) and the Cloud Run
  `proxy_pass` in `nginx.conf.example` are legacy and not the current deployment path.
  `nginx.conf` is a gitignored local file.

### Tech Stack

- React 18 + React Router 6 (`react-router-dom`)
- Tailwind CSS (`tailwind.config.js`, `postcss.config.js`)
- Recharts (charts)
- lucide-react (icons)

## Interface

### Structure

| Path | Responsibility |
|------|----------------|
| `src/main.tsx` | Mounts `<BrowserRouter>` > `<AuthProvider>` > `<App />` |
| `src/App.tsx` | Route table; everything except `/login` is wrapped in `ProtectedRoute` |
| `src/auth/` | Session client (`api.ts`), `AuthProvider`/`useAuth` (`AuthContext.tsx`), `ProtectedRoute` |
| `src/components/` | `Layout` (sidebar nav + header), `UserChip` (identity, role badge, logout), `SubmitButton` (disabled + spinner while pending; use for every write action), `PageTransition` (route-change fade/slide), `icons/` (custom SVG icons) |
| `src/views/` | Page views (one per route below) |
| `src/utils/` | Mock data generators (`mockDataGenerator.ts`, `fixTrackingMockGenerator.ts`) |

### Client routes (`src/App.tsx`, nav in `src/components/Layout.tsx` `navItems`)

| Route | View | Description |
|-------|------|-------------|
| `/login` | `LoginView` | Public login form; redirects to the `from` location or `/docs` |
| `/change-password` | `ChangePasswordView` | Protected; forced when `mustChangePassword` is set |
| `/` | redirect | `Navigate` to `/docs` |
| `/docs` | `DocsView` | Browse generated docs/videos |
| `/qa` | `QAInsights` | Skills failures summary, job audit, trigger QA run / docs generation |
| `/issues` | `IssueDebugger` | Issues with suggestions + patches |
| `/cost` | `CostAnalytics` | AI usage costs |
| `/fixes` | `FixTracking` | is_fixed checkboxes + notes |
| `/skills` | `SkillsView` | QA skills, stats, and logs |
| `/navigation` | `NavigationGraphView` | Learned page-navigation graph |
| `/learning` | `LearningProgressView` | Learning cache progress |
| `/products` | `ProductsView` | Master list of products (repo link, staging URL, root dir in the repo pool, branch); Verify / Clone / Switch branch |
| `/settings` | `SettingsView` | Read-only configuration (AI gateway pinned to claude-v3, services health) and architecture diagram |

### Auth module (`src/auth/`)

- `api.ts`: `login()`, `logout()`, `me()`, `changePassword()`, `roleBadgeClass()`,
  types `AuthUser`, `LoginResponse`, class `AuthError`.
- `AuthContext.tsx`: `AuthProvider`, `useAuth()` exposing `user`, `loading`,
  `mustChangePassword`, `login`, `logout`, `refresh`, `markPasswordChanged`.
- `ProtectedRoute.tsx`: `ProtectedRoute` gate component.

### Backend API paths called (grep of `fetch(` in `src/`)

| Path | Method | Caller |
|------|--------|--------|
| `/api/auth/login` | POST | `auth/api.ts` `login` |
| `/api/auth/logout` | POST | `auth/api.ts` `logout` |
| `/api/auth/me` | GET | `auth/api.ts` `me` |
| `/api/auth/change-password` | POST | `auth/api.ts` `changePassword` |
| `/api/docs?limit&page&search&product&sort_by` | GET | `views/DocsView.tsx` |
| `/api/flows`, `/api/roles` | GET | `views/DocsView.tsx` |
| `/api/analytics`, `/api/cost/overview`, `/api/issues?...`, `/api/jobs?page&limit&sort_by` | GET | `views/QAInsights.tsx` (only when `USE_MOCK_QA_INSIGHTS` is false) |
| `/api/jobs/{jobId}` | GET | `views/QAInsights.tsx` |
| `/api/jobs/qa/run`, `/api/jobs/docs/generate` | POST | `views/QAInsights.tsx` |
| `/api/issues?limit=50&target&search` | GET | `views/IssueDebugger.tsx` |
| `/api/issues?...` | GET | `views/FixTracking.tsx` |
| `/api/issues/status`, `/api/issues/notes` | POST | `views/FixTracking.tsx` |
| `/api/cost/overview`, `/api/cost/trend`, `/api/cost/by-feature`, `/api/cost/efficiency` | GET | `views/CostAnalytics.tsx` |
| `/api/settings` | GET | `views/SettingsView.tsx` |
| `/api/products`, `/api/products/{id}` | GET, POST, PUT, DELETE | `views/ProductsView.tsx` |
| `/api/products/{id}/verify`, `/clone`, `/checkout` | POST | `views/ProductsView.tsx` |
| `/api/skills?...`, `/api/skills/stats?...`, `/api/skills/logs?...` | GET | `views/SkillsView.tsx` |
| `/api/learning?limit=100`, `/api/learning/navigation?source=` | GET | `views/NavigationGraphView.tsx` |
| `/api/learning?limit=500` | GET | `views/LearningProgressView.tsx` |

## Invariants

- **Login gating**: `ProtectedRoute` renders a "Loading session" panel while `loading`,
  redirects to `/login` (with `state.from`) when `user` is null, and redirects to
  `/change-password` whenever `mustChangePassword` is true. Every route except `/login`
  must stay inside `ProtectedRoute` (`src/App.tsx`).
- **Session source of truth**: `AuthProvider` calls `me()` (`GET /api/auth/me`) on mount;
  a 401 yields `user = null`. The session itself is an HttpOnly cookie set by the backend
  (`SESSION_COOKIE_NAME`, default `qa_session`, in `ai-backend/cmd/server/main.go`;
  `HttpOnly` in `ai-backend/internal/api/auth_helpers.go`); the
  dashboard never reads or stores tokens (no `localStorage`/`sessionStorage` use in `src/`).
- `auth/api.ts` `authFetch` always sends `credentials: 'include'`. View-level `fetch`
  calls rely on the default same-origin cookie behaviour, so all API paths must stay
  relative (`/api/...`) and be served same-origin (Vite proxy in dev, nginx `/api`
  proxy in production).
- There is **no** global 401 interceptor: the header comment in `auth/api.ts` mentions
  one in `AuthContext`, but `AuthContext.tsx` only handles 401 from `me()`. A view whose
  session expires mid-use shows its own error until the next `refresh()`/reload.
- **Role badges**: `UserChip` shows `user.role` styled by `roleBadgeClass()` from the
  `ROLE_BADGE` map (`Admin`, `Developer`, `DevOps`, `QA`, `QC`, `UI/UX Designer`);
  unknown roles fall back to the `Admin` (slate) style. Roles are display-only in the
  dashboard; no client-side route is restricted by role.
- `QAInsights` has `USE_MOCK_QA_INSIGHTS = true` hard-coded, so its summary panels
  render `generateMockQAInsightsData()` output instead of calling the backend; the job
  detail and job trigger actions still call the real API.
- `FixTracking` imports only types from `utils/fixTrackingMockGenerator.ts`;
  `installFixTrackingMockApi()` (which monkey-patches `window.fetch` for `/api/issues*`)
  is not installed anywhere in `src/`.

## Side Effects

- Network: same-origin HTTP calls to the backend paths listed above. Mutating calls are
  `POST /api/auth/login|logout|change-password`, `POST /api/jobs/qa/run`,
  `POST /api/jobs/docs/generate` (body `{ triggered_by: 'dashboard', source: 'qa_insights' }`),
  `POST /api/issues/status`, and `POST /api/issues/notes`.
- Cookies: login/logout cause the backend to set/clear the session cookie in the browser.
- `QAInsights` navigates with `window.location.href = '/issues'` (full page load).
- Container image: nginx serves `/health` (in `nginx.conf.example`) used by the
  `Dockerfile` `HEALTHCHECK`; the production compose healthcheck uses `/nginx-health`
  from `deploy/qa-tool/nginx.dashboard.conf`.

## Verification

- Build (bundle via `vite build`; does not run `tsc`): `cd dashboard && npm run build`
- Optional type-check: `cd dashboard && npx tsc --noEmit`
- Lint: `cd dashboard && npm run lint`
- Local dev against a backend on `:8080`: `cd dashboard && npm run dev`
- There is no `test` script in `package.json`.
