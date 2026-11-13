---
paths:
  - "dashboard/**"
---

# Dashboard rules

- Call the API through same-origin `/api/...` only. Never hard-code a backend host or read `VITE_API_URL` in app code (it is a dev-server proxy setting).
- Session-bound requests need `credentials: 'include'` (use `src/auth/api.ts` helpers).
- New pages go behind `ProtectedRoute` unless a spec says otherwise.
- `npm run build` and `npx tsc --noEmit` must pass (`vite build` alone does not type-check).
- `nginx.conf` is gitignored. For prod nginx changes, edit `deploy/his-v3-qa/nginx.dashboard.conf`.
- **Width belongs to `Layout`.** Page roots fill the content area; never put `max-w-*` or `mx-auto` on a view's root element. Constrain only inner pieces that need it (modals, text blocks), never the page. `this-qa-automation-system-playwright/tests/dashboard/layout-width.spec.ts` enforces this.
