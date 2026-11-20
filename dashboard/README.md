# Dashboard (React)

## Overview

UI for products, repos, product knowledge, case automation jobs, skills, agents, learning and cost. A Vite + React 18 single-page
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
| `src/components/` | `Layout` (sidebar nav + top bar; owns page width: views fill the content area and must not set `max-w-*`/`mx-auto` on their root), `TopBar` + `pageHeader.ts` (page title bar: crumbs, title, page actions, `UserChip`; pages call `usePageHeader({ title, crumbs, actions })`, otherwise the title is the longest-prefix `navItems` label; also sets `document.title`), `UserChip` (identity, role badge, "Log in as another user", logout), `ImpersonationBanner` (amber bar above the header while impersonating, with Return), `ImpersonateDialog` (user picker for impersonation), `Modal` (shared centred dialog: Escape, backdrop click, aria), `SubmitButton` (disabled + spinner while pending; use for every write action), `PageTransition` (route-change fade/slide), `icons/` (custom SVG icons) |
| `src/views/` | Page views (one per route below) |
| `src/utils/` | Mock data generators (`mockDataGenerator.ts`, `fixTrackingMockGenerator.ts`) |

### Client routes (`src/App.tsx`, nav in `src/components/Layout.tsx` `navItems`)

| Route | View | Description |
|-------|------|-------------|
| `/login` | `LoginView` | Public login form; redirects to the `from` location or `/products` |
| `/change-password` | `ChangePasswordView` | Protected; forced when `mustChangePassword` is set |
| `/` | redirect | `Navigate` to `/products` |
| `/cost` | `CostAnalytics` | AI usage, spend, errors, latency, recent calls and the ai-manager gateway view |
| `/skills` | `SkillsView` | Switch in `?view=`: QA checks (default; skills table, stats and logs; each check shows Global or its product) or `repo` (Claude skills synced from automation repos, `kind=skill`). Product filter in `?product=` (`global` = global QA checks only; ignored on stats and logs) |
| `/agents` | `assistants/AgentsView` | Agents synced from each product's automation repo (`.claude/agents`): product filter `?product=`, search `?q=`, `?page=`; rows show model, tools and path |
| `/rules` | `assistants/RulesView` | Rules, commands and instructions (`CLAUDE.md`/`AGENTS.md`); segmented filter `?kind=rule\|command\|instructions` (empty = all three, counts from `kinds`), same product filter and search; rules show their `paths` globs |
| `/assistants/:productId/:code` | `assistants/AssistantDetailView` | One synced agent, skill, rule, command or instructions file: frontmatter, body (safe markdown renderer `assistants/Markdown.tsx`, or source), View in repo, removed banner, notes and tags for Admin, DevOps, Developer, QA, QC |
| `/navigation` | `NavigationGraphView` | Learned page-navigation graph |
| `/learning` | `LearningProgressView` (+ `views/learning/`) | The learning loop (LEARNING_LOOP.md): timeline, findings, policy proposals; product filter (URL `?product=`, default the first product with an automation repo), status/kind/state filters |
| `/products` | `ProductsView` | Master list with repos; product detail panel with repo badges (Main, Automation, Central), Make main and Make automation actions, a Verify now prompt on an unverified automation repo, branch lists with default, the Allow non-automation work toggle (Admin/DevOps), per-repo Verify/Clone/Sync, and the Knowledge base card (`views/knowledge/KnowledgeCard.tsx`: format select for Admin/DevOps, Sync now, latest sync, counts, What changed). The list has a Knowledge base column (`views/products/KnowledgeCell.tsx`): automated share (text + bar) and sync freshness from each configured product's `summary` (loaded per row after the table renders, errors inline per row), a Sync icon button for Admin, DevOps, Developer, QA, QC, and a link to the knowledge base; unconfigured products show "Not set up" (Admin/DevOps get a link to the details). `?product=<id>` opens that product's detail; the top bar shows `Products › <name>` |
| `/history` | `history/HistoryView` | **Run history** ([spec](../docs/designs/RUN_HISTORY.md)), nav entry after Products; successor of the removed QA Insights page, real data only. Tabs in `?tab=` (`builds`, `runs`, `syncs`); filters and page in the URL (`?product`, `?from`/`?to` UTC, `?page`, `?page_size`, plus per tab `type, status, case, epic` / `source, outcome, case, epic, job` / `status`). Builds: queue, machine time, time waiting for approval (amber) and total per job, a stacked step bar, a row that expands to the step list; links to `/jobs/:id` and the case page. Playwright runs: every report received (CI, upload, Run case, each build run) with duration and pass/fail/flaky/skipped; a row expands to its per-case durations (`/api/history/runs/{id}`). Knowledge syncs: duration per sync. AI conversations (`?tab=conversations`, filters `kind`, `q`): every non-archived conversation with Archive. Builds and syncs link to their conversation. Refresh button in the top bar; no polling |
| `/qa` | redirect | `Navigate` to `/history` (old QA Insights bookmarks) |
| `/conversations/:id` | `conversations/ConversationView` | One stored AI conversation ([spec](../docs/designs/AI_CONVERSATIONS.md)): header (kind, product, job or sync link, size, model), the thread (`components/ConversationThread.tsx`: replies and reviewer feedback as markdown through `assistants/Markdown.tsx` with `idPrefix` and `showLinkHost`; prompts, refusals and run results as text in a `<pre>`, collapsed when long), Archive / Unarchive. Messages need a `canVerify` role |
| `/archive` | `conversations/ArchiveView` | Nav entry **Archive**: archived conversations (open, Unarchive), totals, **Prune all archived** (enabled for an Admin acting as itself: `PruneDialog` shows the preview counts and needs the word `prune` typed), and the prune log for Admins |
| `/products/:id/knowledge` | `knowledge/KnowledgeView` | Product knowledge base ([spec](../docs/designs/PRODUCT_KNOWLEDGE_SYNC.md)); no nav item, reached from the card or the products list. Top bar: `Products › <product>`, title Knowledge base, compact Sync action. Tabs in `?tab=` (`modules`, `features`, `stories`, `test-cases`, `syncs`); every row links to its detail page, and a quick-peek button opens the drawer (`?e=<type>:<code>`, with an Open page link). Modules shows per-part/module/feature coverage bars; Features filters by module, release and coverage state and sorts tested first (`sort=tested`, default) or by code. Every tab pages (`views/knowledge/paging.ts`, `Pagination` in `ui.tsx`): `?size=` (5, 10, 25, 50; default 5) is shared, `?page=` belongs to the open tab (other tabs keep theirs in memory), and a filter change goes back to page 1. Modules pages by module, grouped under part headers that show part totals, in one column; tested modules and features come first, "Hide untested" drops the rest (with the hidden count), and runs of more than 3 untested features fold into "+ N features without test cases". Syncs pages the latest 50 client-side. Local story/test case drafts for Admin, DevOps, Developer, QA, QC. On the Modules tab every role sees a Download checklist button (`ChecklistDialog.tsx`: PDF/Excel/CSV, last update in the Status column (checked by default), Force regenerate; the `X-Export-Cache` header becomes a note after the download) ([spec](../docs/designs/MODULE_CHECKLIST_EXPORT.md)) |
| `/products/:id/knowledge/:type/:code` | `knowledge/pages/KnowledgeEntityPage` | Detail page of one knowledge entity; `:type` is `parts`, `modules`, `submodules`, `features`, `epics`, `stories` or `test-cases` (one component per type in `views/knowledge/pages/`, shared `EntityShell`). Top bar crumbs `Products › <product> › Knowledge base › …parents`, titled with the code (the full trail repeats in the page below `lg`), removed banner, coverage sentence, the case grid (every active test case as a square coloured by status, grouped by story; one tab stop, arrow keys move), children with a coverage filter, "Hide untested" and segmented status bars (tested children first; more than 3 untested fold into one line), notes and tags. Renders progressively: a skeleton with the type and code from the URL until the entity arrives, then the case grid with its own skeleton. An entity without test cases shows one panel instead of empty sections (`pages/Untested.tsx`): what is missing, what to do (link to the automation repo's `docs/test-catalog`, then sync), the release status for features, the parent's coverage and siblings that do have test cases (`pages/context.ts`). Long top-bar trails keep the first crumb and the two nearest parents and fold the rest into a "…" menu (`components/TopBar.tsx`). No nav item |
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
| `/api/cost/{overview,trend,by-feature,by-model,efficiency,filters}?from&to&feature&provider&model&status`, `/api/cost/recent?...&limit&offset`, `/api/cost/gateway?month=YYYY-MM|all` | GET | `views/CostAnalytics.tsx` (types in `utils/costTypes.ts`) |
| `/api/settings` | GET | `views/SettingsView.tsx` |
| `/api/history/{builds,runs,syncs}`, `/api/history/runs/{id}` | GET | `views/history/` (`api.ts`; `formatMs` is shared from `views/knowledge/api.ts`) |
| `/api/conversations[...]`, `/api/jobs/{id}/conversation` | GET, POST | `views/conversations/api.ts`; the job page's AI conversation card (`JobConversationCard.tsx`, polls `since_seq` every 2 s while the job runs) |
| `/api/jobs/{id}`, `/api/jobs/{id}/events`, `/api/jobs/{id}/approval` | GET; GET; POST | `views/JobView.tsx`, `views/knowledge/api.ts` (case automation job log and approval) |
| `/api/products`, `/api/products/{id}` | GET, POST, PUT, DELETE | `views/ProductsView.tsx`, `views/products/ProductDetail.tsx` (PUT: name, staging URL, `allow_non_automation_work`) |
| `/api/products/{id}/repos`, `/api/products/{id}/repos/{rid}` | POST; PUT, DELETE | `views/products/ProductDetail.tsx` (add, edit, set main/automation/central, remove repo) |
| `/api/products/{id}/repos/{rid}/branches` | PUT | `views/products/ProductDetail.tsx` / `RepoCard.tsx` (save branch list and default) |
| `/api/products/{id}/repos/{rid}/verify`, `/clone`, `/checkout` | POST | `views/products/ProductDetail.tsx` / `RepoCard.tsx` (Verify, Clone, Sync branches) |
| `/api/products/{id}` | PUT `{gaos_integrated, gaos_product_id}` | `views/products/GaosCard.tsx` (Admin/DevOps; the list's `gaos_configured` says whether the server can push) |
| `/api/products/{id}/gaos/sync` | POST (200 result, 202 still running, 409 busy) | `views/products/GaosCard.tsx` (Sync to GAOS now) |
| `/api/products/{id}` | PUT `{knowledge_format}` | `views/knowledge/KnowledgeCard.tsx` (Admin/DevOps set the catalog format) |
| `/api/products/{id}/knowledge/summary`, `/tree`, `/syncs`, `/syncs/{sid}`, `/epics`, `/stories`, `/test-cases`, `/features` (`sort=tested\|code`), `/modules`, `/coverage?by=`, `/{parts\|modules\|submodules\|features\|epics\|stories\|test-cases}/{code}` | GET | `views/knowledge/api.ts` (all knowledge views; the case grid pages through `test-cases?{part\|module\|submodule\|epic}=…&page_size=200`) |
| `/api/products/{id}/knowledge/exports/modules-checklist?format=pdf\|xlsx\|csv&last_update=0\|1&force=0\|1` | GET (file download; reads `Content-Disposition` and `X-Export-Cache` when same-origin) | `views/knowledge/ChecklistDialog.tsx`, `downloadChecklist` in `views/knowledge/api.ts` |
| `/api/products/{id}/knowledge/sync` | POST (202 `{sync_id}`; 409 with `sync_id` is followed, not shown as an error) | `views/knowledge/KnowledgeCard.tsx`, `views/knowledge/useKnowledgeSync.ts` (products list rows, knowledge base top bar) |
| `/api/products/{id}/knowledge/annotations/{type}/{code}` | PUT, DELETE | `views/knowledge/EntityPanel.tsx` (notes and tags; also on the detail pages via `pages/EntityShell.tsx`) |
| `/api/products/{id}/knowledge/stories`, `/test-cases` (and `/{code}`) | POST; PUT, DELETE | `views/knowledge/DraftModal.tsx`, `EntityPanel.tsx` (local drafts, `^L-[A-Z0-9-]{1,30}$`) |

The product views share `views/products/api.ts` (fetch helper, types, form validation); `components/Modal.tsx` and `components/Field.tsx` were moved out of `ProductsView.tsx`. The legacy `/api/products/{id}/verify|clone|checkout` routes remain on the server for old clients but are no longer called, and the Switch-branch modal is gone.
| `/api/skills?...&product_id=<id>\|global`, `/api/skills/stats?...`, `/api/skills/logs?...` | GET | `views/SkillsView.tsx` |
| `/api/assistants?product_id&kind&q&page&page_size` | GET | `views/assistants/api.ts` (`AssistantList`: agents, rules, repo skills) |
| `/api/products/{id}/knowledge/assistants/{code}` | GET | `views/assistants/api.ts` (`AssistantDetailView`) |
| `/api/products/{id}/knowledge/annotations/assistant/{code}` | PUT, DELETE | `views/assistants/AnnotationEditor.tsx` |
| `/api/products` | GET | `views/assistants/api.ts` `useProducts` (product filter; loaded once per page load) |
| `/api/learning?limit=100`, `/api/learning/navigation?source=` | GET | `views/NavigationGraphView.tsx` |
| `/api/learning/findings`, `/api/learning/proposals`, `/api/learning/timeline` (GET); `/api/learning/findings` (POST), `…/findings/{id}/dismiss\|reopen`, `/api/learning/proposals` (POST), `…/proposals/{id}/check`, `/api/jobs/{id}/approval` (POST) | GET, POST | `views/LearningProgressView.tsx`, `views/learning/` |

## Invariants

- **Assistant files** (`src/views/assistants/`): synced repo content is untrusted. `Markdown.tsx`
  builds React elements only (no `dangerouslySetInnerHTML`); raw HTML shows as text, and links
  are limited to http(s), mailto, in-page anchors and repo-relative paths resolved against the
  file's `source_url`. The list is read-only; the only write is the note/tag annotation
  (`SubmitButton`, one request in flight).
- **Knowledge base** (`src/views/knowledge/`): the sync card polls `GET syncs/{sid}` every
  1.5 s while `status='running'` or `ai_status='pending'`, stops after 10 minutes or on
  unmount, and resumes when the page opens on a running sync (`useSyncFollower.ts`). Synced
  rows are read-only (lock note); only `origin='local'` drafts get Edit and Delete. Lists use
  `useLoad` (aborts stale requests). `Drawer.tsx` is always mounted (interruptible CSS
  transitions in `index.css` `.kb-drawer*`), traps focus, closes on Escape, and must be
  rendered outside `space-y-*` containers (their child margins offset fixed elements).
  Role checks (`canWriteKnowledge`, `canSetFormat`) are UI hints; the server enforces them.
  `useKnowledgeSync` guards Sync synchronously (a double click sends one POST) and follows the
  sync with the same poller; the icon only spins when motion is allowed, and the row or
  button text says "Syncing…".
- **Top bar** (`components/TopBar.tsx`, `pageHeader.ts`): the page title lives in the top bar
  (`h1`); views must not repeat it as an in-page heading. Sub-views (details, URL tabs) set
  crumbs and a more specific title. Several components may claim the header; the latest
  mounted claim wins and unmounting releases it.

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

## Side Effects

- Network: same-origin HTTP calls to the backend paths listed above. Mutating calls are
  `POST /api/auth/login|logout|change-password|impersonate|impersonate/stop`, plus the product, repo, knowledge and case-automation calls listed above.
- Cookies: login/logout cause the backend to set/clear the session cookie in the browser.
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
