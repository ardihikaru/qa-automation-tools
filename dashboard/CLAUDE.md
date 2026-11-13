# dashboard — agent instructions

This is a React 18, Vite 5, TypeScript and Tailwind SPA. The contract is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md).

## Commands
- Dev: `npm run dev`. Vite proxies `/api` to `VITE_API_URL`, which defaults to `http://localhost:8080`.
- Build: `npm run build` (plain `vite build`, **no** type check). Type check: `npx tsc --noEmit`. Lint: `npm run lint`.

## Conventions
- **Use same-origin `/api` only.** In production, nginx (`deploy/his-v3-qa/nginx.dashboard.conf`) proxies `/api` to the backend. Never hard-code a backend host.
- **Auth:** every fetch that needs a session goes through `src/auth/api.ts` helpers, or passes `credentials: 'include'`. Pages sit behind `ProtectedRoute`. A 401 means: go back to `/login`.
- **Roles:** badge colours live in `ROLE_BADGE` (`src/auth/api.ts`). Backend role names come from `normalizeRole` in `ai-backend/internal/seed/seed_users.go` **and** from `upsertServiceUser` (the only writer of the `Service` role; `normalizeRole` deliberately refuses it). Keep `ROLE_BADGE` in sync with both; a missing role falls back to the Admin colours. `IMPERSONATOR_ROLES` in the same file mirrors the backend's `auth.ImpersonatorRoles`.
- **Identity changes** (impersonate, stop) go through `useAuth()` and end in a full reload (`window.location.assign('/')`). Never swap the user in place.
- **Docker:** `nginx.conf` is gitignored. Images built from a clean checkout use `--build-arg NGINX_CONF=nginx.conf.example`, and production mounts its own config.
