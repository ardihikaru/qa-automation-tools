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
  [deploy/his-v3-qa](../deploy/his-v3-qa/README.md) on `rnd-v3-vpn`, behind the clino
  nginx edge at https://qa-tool.sismedika.com. That stack builds with
  `NGINX_CONF=nginx.conf.example` and mounts `deploy/his-v3-qa/nginx.dashboard.conf`
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
| `src/components/` | `Layout` (sidebar nav + header; owns page width: views fill the content area and must not set `max-w-*`/`mx-auto` on their root), `UserChip` (identity, role badge, "Log in as another user", logout), `ImpersonationBanner` (amber bar above the header while impersonating, with Return), `ImpersonateDialog` (user picker for impersonation), `Modal` (shared centred dialog: Escape, backdrop click, aria), `SubmitButton` (disabled + spinner while pending; use for every write action), `PageTransition` (route-change fade/slide), `icons/` (custom SVG icons) |
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
| `/cost` | `CostAnalytics` | AI usage, spend, errors, latency, recent calls and the ai-manager gateway view |
| `/fixes` | `FixTracking` | is_fixed checkboxes + notes |
| `/skills` | `SkillsView` | QA skills, stats, and logs |
| `/navigation` | `NavigationGraphView` | Learned page-navigation graph |
| `/learning` | `LearningProgressView` | Learning cache progress |
| `/products` | `ProductsView` | Master list with repos; product detail panel with repo badges (Main, Automation, Central), Make main and Make automation actions, a Verify now prompt on an unverified automation repo, branch lists with default, the Allow non-automation work toggle (Admin/DevOps), per-repo Verify/Clone/Sync, and the Knowledge base card (`views/knowledge/KnowledgeCard.tsx`: format select for Admin/DevOps, Sync now, latest sync, counts, What changed). `?product=<id>` opens that product's detail |
| `/products/:id/knowledge` | `knowledge/KnowledgeView` | Product knowledge base ([spec](../docs/designs/PRODUCT_KNOWLEDGE_SYNC.md)); no nav item, reached from the card. Tabs in `?tab=` (`modules`, `features`, `stories`, `test-cases`, `syncs`); every row links to its detail page, and a quick-peek button opens the drawer (`?e=<type>:<code>`, with an Open page link). Modules shows per-part/module/feature coverage bars; Features filters by module, release and coverage state. Local story/test case drafts for Admin, DevOps, Developer, QA, QC |
| `/products/:id/knowledge/:type/:code` | `knowledge/pages/KnowledgeEntityPage` | Detail page of one knowledge entity; `:type` is `parts`, `modules`, `submodules`, `features`, `epics`, `stories` or `test-cases` (one component per type in `views/knowledge/pages/`, shared `EntityShell`). Breadcrumbs, removed banner, coverage sentence, the case grid (every active test case as a square coloured by status, grouped by story; one tab stop, arrow keys move), children with a coverage filter and segmented status bars, notes and tags. No nav item |
| `/settings` | `SettingsView` | Read-only configuration (AI gateway pinned to claude-v3, services health) and architecture diagram |

### Auth module (`src/auth/`)

- `api.ts`: `login()`, `logout()`, `me()`, `changePassword()`, `listUsers()`,
  `impersonate(username)`, `stopImpersonation()`, `roleBadgeClass()`,
  `IMPERSONATOR_ROLES` (`Admin`, `Service`; UI hint only), types `AuthUser`
  (with optional `impersonator`), `Impersonator`, `UserSummary`, `LoginResponse`
  (the shared `{user, mustChangePassword}` shape), class `AuthError` (carries the
  backend's `{error}` text and the HTTP status).
- `AuthContext.tsx`: `AuthProvider`, `useAuth()` exposing `user`, `loading`,
  `mustChangePassword`, `login`, `logout`, `refresh`, `markPasswordChanged`,
  `impersonate`, `stopImpersonation`.
- `ProtectedRoute.tsx`: `ProtectedRoute` gate component.

### Impersonation flow ([spec](../docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md) §6)

1. `UserChip` shows a `UserCog` button (`aria-label="Log in as another user"`) when
   `user.role` is `Admin` or `Service` and `user.impersonator` is absent.
2. `ImpersonateDialog` loads `GET /api/users`, filters by username or full name
   (case-insensitive), shows role badges, hides the caller (by `id`), disables
   inactive users, and shows the backend `{error}` inline. Rows are locked while a
   request is in flight.
3. `useAuth().impersonate(username)` posts `/api/auth/impersonate`; on 200 it does
   `window.location.assign('/')` (full reload, so no page state from the previous
   identity survives).
4. While `user.impersonator` is set, `Layout` renders `ImpersonationBanner`
   (`role="status"`): "Viewing as X (role). Signed in as Y. [Return to Y]". The
   banner calls `refresh()` on window focus, so an expired impersonation (the
   backend falls back to the real user) clears it.
5. Return calls `useAuth().stopImpersonation()` (`POST /api/auth/impersonate/stop`),
   then reloads to `/`. The button is disabled while in flight; errors show inline.
6. Logout while impersonating confirms with "This signs out <real name> completely."
   and ends both the impersonation and the real session.

### Backend API paths called (grep of `fetch(` in `src/`)

| Path | Method | Caller |
|------|--------|--------|
| `/api/auth/login` | POST | `auth/api.ts` `login` |
| `/api/auth/logout` | POST | `auth/api.ts` `logout` |
| `/api/auth/me` | GET | `auth/api.ts` `me` |
| `/api/auth/change-password` | POST | `auth/api.ts` `changePassword` |
| `/api/users` | GET | `auth/api.ts` `listUsers` (Admin/Service only) |
| `/api/auth/impersonate` | POST | `auth/api.ts` `impersonate` |
| `/api/auth/impersonate/stop` | POST | `auth/api.ts` `stopImpersonation` |
| `/api/docs?limit&page&search&product&sort_by` | GET | `views/DocsView.tsx` |
| `/api/flows`, `/api/roles` | GET | `views/DocsView.tsx` |
| `/api/analytics`, `/api/cost/overview`, `/api/issues?...`, `/api/jobs?page&limit&sort_by` | GET | `views/QAInsights.tsx` (only when `USE_MOCK_QA_INSIGHTS` is false) |
| `/api/jobs/{jobId}` | GET | `views/QAInsights.tsx` |
| `/api/jobs/qa/run`, `/api/jobs/docs/generate` | POST | `views/QAInsights.tsx` |
| `/api/issues?limit=50&target&search` | GET | `views/IssueDebugger.tsx` |
| `/api/issues?...` | GET | `views/FixTracking.tsx` |
| `/api/issues/status`, `/api/issues/notes` | POST | `views/FixTracking.tsx` |
| `/api/cost/{overview,trend,by-feature,by-model,efficiency,filters}?from&to&feature&provider&model&status`, `/api/cost/recent?...&limit&offset`, `/api/cost/gateway?month=YYYY-MM|all` | GET | `views/CostAnalytics.tsx` (types in `utils/costTypes.ts`) |
| `/api/settings` | GET | `views/SettingsView.tsx` |
| `/api/products`, `/api/products/{id}` | GET, POST, PUT, DELETE | `views/ProductsView.tsx`, `views/products/ProductDetail.tsx` (PUT: name, staging URL, `allow_non_automation_work`) |
| `/api/products/{id}/repos`, `/api/products/{id}/repos/{rid}` | POST; PUT, DELETE | `views/products/ProductDetail.tsx` (add, edit, set main/automation/central, remove repo) |
| `/api/products/{id}/repos/{rid}/branches` | PUT | `views/products/ProductDetail.tsx` / `RepoCard.tsx` (save branch list and default) |
| `/api/products/{id}/repos/{rid}/verify`, `/clone`, `/checkout` | POST | `views/products/ProductDetail.tsx` / `RepoCard.tsx` (Verify, Clone, Sync branches) |
| `/api/products/{id}` | PUT `{knowledge_format}` | `views/knowledge/KnowledgeCard.tsx` (Admin/DevOps set the catalog format) |
| `/api/products/{id}/knowledge/summary`, `/tree`, `/syncs`, `/syncs/{sid}`, `/epics`, `/stories`, `/test-cases`, `/features`, `/modules`, `/coverage?by=`, `/{parts\|modules\|submodules\|features\|epics\|stories\|test-cases}/{code}` | GET | `views/knowledge/api.ts` (all knowledge views; the case grid pages through `test-cases?{part\|module\|submodule\|epic}=…&page_size=200`) |
| `/api/products/{id}/knowledge/sync` | POST (202 `{sync_id}`; 409 with `sync_id` is followed, not shown as an error) | `views/knowledge/KnowledgeCard.tsx` |
| `/api/products/{id}/knowledge/annotations/{type}/{code}` | PUT, DELETE | `views/knowledge/EntityPanel.tsx` (notes and tags; also on the detail pages via `pages/EntityShell.tsx`) |
| `/api/products/{id}/knowledge/stories`, `/test-cases` (and `/{code}`) | POST; PUT, DELETE | `views/knowledge/DraftModal.tsx`, `EntityPanel.tsx` (local drafts, `^L-[A-Z0-9-]{1,30}$`) |

The product views share `views/products/api.ts` (fetch helper, types, form validation); `components/Modal.tsx` and `components/Field.tsx` were moved out of `ProductsView.tsx`. The legacy `/api/products/{id}/verify|clone|checkout` routes remain on the server for old clients but are no longer called, and the Switch-branch modal is gone.
| `/api/skills?...`, `/api/skills/stats?...`, `/api/skills/logs?...` | GET | `views/SkillsView.tsx` |
| `/api/learning?limit=100`, `/api/learning/navigation?source=` | GET | `views/NavigationGraphView.tsx` |
| `/api/learning?limit=500` | GET | `views/LearningProgressView.tsx` |

## Invariants

- **Knowledge base** (`src/views/knowledge/`): the sync card polls `GET syncs/{sid}` every
  1.5 s while `status='running'` or `ai_status='pending'`, stops after 10 minutes or on
  unmount, and resumes when the page opens on a running sync (`useSyncFollower.ts`). Synced
  rows are read-only (lock note); only `origin='local'` drafts get Edit and Delete. Lists use
  `useLoad` (aborts stale requests). `Drawer.tsx` is always mounted (interruptible CSS
  transitions in `index.css` `.kb-drawer*`), traps focus, closes on Escape, and must be
  rendered outside `space-y-*` containers (their child margins offset fixed elements).
  Role checks (`canWriteKnowledge`, `canSetFormat`) are UI hints; the server enforces them.

- **Login gating**: `ProtectedRoute` renders a "Loading session" panel while `loading`,
  redirects to `/login` (with `state.from`) when `user` is null, and redirects to
  `/change-password` whenever `mustChangePassword` is true. While `user.impersonator`
  is set it redirects `/change-password` to `/` (the backend returns 403 there). Every
  route except `/login` must stay inside `ProtectedRoute` (`src/App.tsx`).
- **Session source of truth**: `AuthProvider` calls `me()` (`GET /api/auth/me`) on mount;
  a 401 yields `user = null`. A network or 5xx error from a later `refresh()` keeps the
  current identity instead of signing the user out. The session itself is an HttpOnly cookie set by the backend
  (`SESSION_COOKIE_NAME`, default `qa_session`, in `ai-backend/cmd/server/main.go`;
  `HttpOnly` in `ai-backend/internal/api/auth_helpers.go`); the
  dashboard never reads or stores tokens (no `localStorage`/`sessionStorage` use in `src/`).
- `auth/api.ts` `authFetch` always sends `credentials: 'include'`. View-level `fetch`
  calls rely on the default same-origin cookie behaviour, so all API paths must stay
  relative (`/api/...`) and be served same-origin (Vite proxy in dev, nginx `/api`
  proxy in production).
- There is **no** global 401 interceptor. `AuthContext.tsx` handles a 401 only from
  `me()` (user becomes null) and from `impersonate()`/`stopImpersonation()` (user
  becomes null and the app navigates to `/login`). A view whose session expires
  mid-use shows its own error until the next `refresh()`/reload.
- **Identity changes reload the app**: after a successful impersonate or stop, the
  dashboard calls `window.location.assign('/')`; it never swaps identity in place.
- **Role badges**: `UserChip` shows `user.role` styled by `roleBadgeClass()` from the
  `ROLE_BADGE` map (`Admin`, `Developer`, `DevOps`, `QA`, `QC`, `UI/UX Designer`,
  `Service` in cyan); unknown roles fall back to the `Admin` (slate) style. Roles are
  display-only in the dashboard, except that the impersonation button is shown only to
  `Admin`/`Service`; the backend enforces every permission.
- `QAInsights` has `USE_MOCK_QA_INSIGHTS = true` hard-coded, so its summary panels
  render `generateMockQAInsightsData()` output instead of calling the backend; the job
  detail and job trigger actions still call the real API.
- `FixTracking` imports only types from `utils/fixTrackingMockGenerator.ts`;
  `installFixTrackingMockApi()` (which monkey-patches `window.fetch` for `/api/issues*`)
  is not installed anywhere in `src/`.

## Side Effects

- Network: same-origin HTTP calls to the backend paths listed above. Mutating calls are
  `POST /api/auth/login|logout|change-password|impersonate|impersonate/stop`, `POST /api/jobs/qa/run`,
  `POST /api/jobs/docs/generate` (body `{ triggered_by: 'dashboard', source: 'qa_insights' }`),
  `POST /api/issues/status`, and `POST /api/issues/notes`.
- Cookies: login/logout cause the backend to set/clear the session cookie in the browser.
- `QAInsights` navigates with `window.location.href = '/issues'` (full page load).
- `AuthContext` `impersonate`/`stopImpersonation` reload the app with
  `window.location.assign('/')`.
- `ImpersonationBanner` adds a window `focus` listener (only while impersonating) that
  calls `GET /api/auth/me`.
- Container image: nginx serves `/health` (in `nginx.conf.example`) used by the
  `Dockerfile` `HEALTHCHECK`; the production compose healthcheck uses `/nginx-health`
  from `deploy/his-v3-qa/nginx.dashboard.conf`.

## Verification

- Build (bundle via `vite build`; does not run `tsc`): `cd dashboard && npm run build`
- Optional type-check: `cd dashboard && npx tsc --noEmit`
- Lint: `cd dashboard && npm run lint`
- Local dev against a backend on `:8080`: `cd dashboard && npm run dev`
- There is no `test` script in `package.json`.
