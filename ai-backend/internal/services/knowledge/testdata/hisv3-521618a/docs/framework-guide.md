# Panduan framework (Playwright)

> **Catatan (HISv3):** nama service/app di dokumen ini (`user-service`, `order-service`, `catalog-gateway`, `customer-portal`, `admin-console`) adalah _contoh fiktif_ dari template awal untuk menjelaskan pola; implementasinya sudah dihapus dan `mock-server/` tidak ada lagi. Service HISv3 yang sebenarnya: [hisv3-services.md](hisv3-services.md). Panduan ini sebelumnya README template; README repo sekarang ada di `../README.md`. Path `mock-server/`, `START_MOCK_SERVER` dan `npm run mock:start` sudah tidak berlaku.

A standard, ready-to-clone structure for **end-to-end test automation with Playwright + TypeScript**.

| Covers                                    | How                                                                                                           |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| **Backend E2E — REST**                    | Typed service clients + zod contract checks (`tests/api/rest/**`)                                             |
| **Backend E2E — GraphQL**                 | Typed operations + GraphQL client (`tests/api/graphql/**`)                                                    |
| **Frontend E2E connected to the backend** | Page objects, login once via `storageState`, seed via API → act in UI → verify via API (`tests/web/**`)       |
| **Targets: local or staging**             | `TEST_ENV=local \| staging`, validated config, production guard                                               |
| **Many services / many frontends**        | Registry-driven projects (`config/targets.ts`), mirrored folder layout                                        |
| **Native Android / iOS**                  | Separate WebdriverIO + Appium 3 project in `mobile/`                                                          |
| **AI-assisted coding**                    | [`AGENTS.md`](AGENTS.md) (rules for agents) + [`docs/ai-adoption.md`](docs/ai-adoption.md) (guide for humans) |

It runs **out of the box**: a small bundled mock backend + two demo web apps (`mock-server/`) are started automatically for the
`local` target, so `npm test` is green on a fresh clone. Replace the mock with your real services when you adopt the template.

---

## Table of contents

1. [Quick start](#1-quick-start)
2. [Repository structure](#2-repository-structure)
3. [Running tests](#3-running-tests)
4. [Environments: local vs staging](#4-environments-local-vs-staging)
5. [Backend E2E — REST API](#5-backend-e2e--rest-api)
6. [Backend E2E — GraphQL](#6-backend-e2e--graphql)
7. [Frontend E2E connected to the backend](#7-frontend-e2e-connected-to-the-backend)
8. [Git-ignored files & folders](#8-git-ignored-files--folders)
9. [Scaling: many REST services, GraphQL endpoints and frontend apps](#9-scaling-many-rest-services-graphql-endpoints-and-frontend-apps)
10. [Mobile: Android & iOS](#10-mobile-android--ios)
11. [AI-assisted development](#11-ai-assisted-development)
12. [CI/CD](#12-cicd)
13. [Conventions cheat sheet](#13-conventions-cheat-sheet)
14. [Troubleshooting](#14-troubleshooting)
15. [References / sources](#15-references--sources)

Starting a repo for a real product from this template? See [docs/product-setup.md](docs/product-setup.md).
What is tested and what is still missing: [docs/test-strategy.md](docs/test-strategy.md) and the [test catalog](docs/test-catalog/README.md).

---

## 1. Quick start

Prerequisites: **Node.js ≥ 22.19** (see `.nvmrc`), Git, plus **one** of npm ≥ 10 or **Bun ≥ 1.3**.

Playwright itself runs on Node — Bun is a package manager + task runner, not a replacement for the Node
runtime, so both toolchains share the exact same scripts in `package.json` (none of them hardcode `bun`).

| Toolchain | Install       | Run a script     | Playwright CLI      |
| --------- | ------------- | ---------------- | ------------------- |
| npm       | `npm ci`      | `npm run <name>` | `npx playwright …`  |
| Bun       | `bun install` | `bun run <name>` | `bunx playwright …` |

```bash
bun install                               # install dependencies (bun.lock is committed)
bunx playwright install --with-deps chromium
bun run env:init                         # creates git-ignored env files from *.example templates
bun run test                              # API + web tests against the local mock target
bun run report                           # open the HTML report
```

Same flow with npm: `npm ci` → `npx playwright install --with-deps chromium` → `npm run env:init` → `npm test`.

> Changing dependencies means updating **both** lockfiles (`package-lock.json` and `bun.lock`, `mobile/`
> included): `npm ci` and `bun install --frozen-lockfile` both reject a stale lockfile, and CI runs one lane
> per toolchain.

Useful next commands: `bun run test:ui` (interactive UI mode), `bun run test:smoke`, `bun run check` (typecheck incl. mobile + lint + format).

---

## 2. Repository structure

```text
.
├── AGENTS.md                      # Rules for AI coding agents (standards, policy, recipes)  ← single source of truth
├── CLAUDE.md                      # Claude Code entry → imports AGENTS.md
├── README.md                      # You are here
├── playwright.config.ts           # Projects generated from config/targets.ts
├── config/
│   ├── env.ts                     # Loads env/.env.<TEST_ENV>, validates with zod, production guard
│   └── targets.ts                 # Registry: REST services, GraphQL endpoints, web apps, test accounts
├── env/
│   ├── .env.local.example         # ✅ committed template   → env/.env.local   (❌ ignored)
│   └── .env.staging.example       # ✅ committed template   → env/.env.staging (❌ ignored)
├── src/
│   ├── api/
│   │   ├── core/                  # BaseApiClient, GraphqlClient, expectJson(), shared schemas
│   │   ├── rest/
│   │   │   ├── user-service/      # <service>.client.ts + <service>.schemas.ts
│   │   │   └── order-service/
│   │   └── graphql/
│   │       └── catalog-gateway/   # <endpoint>/ then <module>/ → <module>.client.ts + .operations.ts + .schemas.ts
│   ├── web/
│   │   ├── shared/base.page.ts
│   │   ├── customer-portal/       # <app>.app.ts + pages/<feature>/ + layouts/<area>/ + components/
│   │   └── admin-console/
│   ├── fixtures/                  # api.fixtures.ts, web.fixtures.ts → index.ts (the only `test` to import)
│   ├── data/factories/            # buildUser(), buildProduct() — valid, unique payloads
│   └── utils/                     # tags, unique ids, guards
├── tests/                         # Mirrors src/: one folder per service / endpoint / app
│   ├── api/rest/<service>/*.spec.ts
│   ├── api/graphql/<endpoint>/<module>/*.spec.ts
│   ├── web/<app>/<feature>/*.spec.ts
│   └── setup/<app>.setup.ts       # Logs in once per app → .auth/<app>.json
├── test-data/
│   ├── static/                    # ✅ committed, non-sensitive data for data-driven tests
│   └── private/                   # ❌ ignored (except README + *.example.json)
├── .auth/                         # ❌ ignored session files (except README + .gitkeep)
├── mobile/                        # Native Android/iOS: WebdriverIO + Appium (own package.json)
├── mock-server/                   # Demo backend + frontends for the local target (delete when adopting)
├── docs/
│   ├── architecture.md            # Scaling the structure (multi-service, multi-app, multi-team)
│   ├── multi-service-collaboration.md  # 2 REST + 1 GraphQL + 2 apps: diagrams, cross-service tests, tutorials (Bahasa Indonesia)
│   └── ai-adoption.md             # How the team adopts AI coding tools on this repo
├── scripts/init-env.mjs           # Copies *.example → real (ignored) files
└── .github/                       # CI workflow, CODEOWNERS, PR template, Copilot instructions
```

**Layering rule** — dependencies only point downwards:

```text
tests/**  →  src/fixtures  →  src/web/** (page objects)  →  config/
                           →  src/api/** (clients)       →  config/
```

Specs never call `request.get(...)` or `page.locator(...)` with raw selectors directly — they go through a client or a page object.

---

## 3. Running tests

| Goal                          | Command                                                                                                      |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------ |
| Everything (local target)     | `npm test`                                                                                                   |
| Backend only (REST + GraphQL) | `npm run test:api`                                                                                           |
| REST only / GraphQL only      | `npm run test:api:rest` / `npm run test:api:graphql`                                                         |
| Frontend only (all apps)      | `npm run test:web`                                                                                           |
| One app only                  | `npx playwright test --project=web-customer-portal-chromium`                                                 |
| One file / one test           | `npx playwright test tests/api/rest/user-service/users.spec.ts -g "duplicate"`                               |
| By tag                        | `npm run test:smoke` · `npm run test:regression` · `npx playwright test --grep-invert @destructive`          |
| Against staging               | `npm run test:staging` · `npm run test:staging:smoke`                                                        |
| Extra browsers                | `BROWSERS=chromium,firefox,webkit npm run test:web`                                                          |
| Mobile-viewport web           | `BROWSERS=mobile-chrome,mobile-safari npm run test:web`                                                      |
| Debug                         | `npm run test:ui` · `npm run test:debug` · `npx playwright show-trace .runs/<run>/test-results/**/trace.zip` |
| Record a new flow             | `npm run codegen -- http://localhost:3000`                                                                   |

Generated Playwright projects (see `npm run test:list`):

```text
api-rest, api-graphql                         # no browser
setup-customer-portal, setup-admin-console    # login once, save .auth/<app>.json
web-customer-portal-<browser>                 # depends on setup-customer-portal
web-admin-console-<browser>                   # depends on setup-admin-console
```

---

## 4. Environments: local vs staging

```text
TEST_ENV (default: local)
   │
   ▼
env/.env.<TEST_ENV>  ──►  process.env  ──►  config/env.ts (zod validation)  ──►  config/targets.ts  ──►  projects / clients
   ▲                         ▲
   │ git-ignored file         └── real env vars (CI secrets, shell exports) ALWAYS win over the file
```

|                     | `local`                                                                   | `staging`                                       |
| ------------------- | ------------------------------------------------------------------------- | ----------------------------------------------- |
| Command             | `npm test`                                                                | `npm run test:staging`                          |
| Env file            | `env/.env.local`                                                          | `env/.env.staging` (or CI secrets — no file)    |
| Backend             | bundled mock (`START_MOCK_SERVER=true`) **or** your services on localhost | shared staging services                         |
| Accounts            | demo accounts                                                             | dedicated QA accounts from the password manager |
| `ALLOW_DESTRUCTIVE` | `true`                                                                    | `false` (default)                               |

Pointing **local** at your real locally-running services: set the URLs in `env/.env.local` and `START_MOCK_SERVER=false`.

Safety rails built into `config/env.ts`:

- Only `local` and `staging` are accepted — **production is not a target**.
- Any `*_URL` (or any other setting whose value is an http(s) URL) whose host is in `PROTECTED_HOSTS` or contains a `prod` / `production` label aborts the run.
- On `staging`, an empty `PROTECTED_HOSTS` aborts the run: the label heuristic alone does not recognise most production hostnames.
- Missing or malformed variables fail fast with a readable list, before any test starts.
- Tests tagged `@destructive` must call `skipUnlessDestructiveAllowed()`.

**Adding a variable:** add it to both `env/*.example` files and to the schema in `config/env.ts`. Never read `process.env` elsewhere.

---

## 5. Backend E2E — REST API

One **client per service** (`src/api/rest/<service>/<service>.client.ts`). Methods map 1:1 to endpoints and return the raw
`APIResponse`; the spec decides what to assert. Contracts live next to the client as zod schemas.

```ts
// tests/api/rest/user-service/users.spec.ts
import { test, expect } from '@fixtures';
import { expectJson } from '@api/core/assertions';
import { UserSchema } from '@api/rest/user-service/user-service.schemas';
import { buildUser } from '@data/factories/user.factory';
import { Tag } from '@utils/tags';

test('admin creates a user and reads it back', { tag: [Tag.smoke] }, async ({ userApi, cleanup }) => {
  const payload = buildUser();

  const created = await expectJson(await userApi.createUser(payload), UserSchema, 201); // status + contract
  cleanup.add(() => userApi.deleteUser(created.id)); // always runs, even on failure

  expect(created).toMatchObject(payload);
});
```

Building blocks:

| Piece                             | Where                             | Purpose                                                                             |
| --------------------------------- | --------------------------------- | ----------------------------------------------------------------------------------- |
| `BaseApiClient`                   | `src/api/core/base-api-client.ts` | URL joining (works with origins _and_ gateway prefixes), verbs                      |
| `expectJson(res, Schema, status)` | `src/api/core/assertions.ts`      | Status check with body in the error + zod contract validation → typed result        |
| `apiContextFor(role)`             | fixture                           | `APIRequestContext` as `admin` / `customer` / `anonymous` (token cached per worker) |
| `userApi`, `orderApi`             | fixtures                          | Ready-to-use clients with the right role                                            |
| `cleanup.add(fn)`                 | fixture                           | LIFO teardown; failures are reported as annotations, never hide the real result     |

Always cover per endpoint: happy path + contract, auth (401/403), validation (400), not found (404), conflict/business rule (409/422).

---

## 6. Backend E2E — GraphQL

One folder per GraphQL endpoint (gateway or subgraph), one folder per module inside it:
`src/api/graphql/<endpoint>/<module>/`. The `<module>` name is the one of the product repo
(`his-backend/pkg/graph/<module>`).

```ts
// src/api/graphql/catalog-gateway/catalog.operations.ts
export const ProductsQuery = defineOperation<{ products: Product[] }, { first?: number; category?: string }>(
  'Products',
  /* GraphQL */ `
    query Products($first: Int, $category: String) {
      products(first: $first, category: $category) {
        id
        name
        price
        category
      }
    }
  `,
);
```

| Method                                        | Use for                                                           |
| --------------------------------------------- | ----------------------------------------------------------------- |
| `client.query(Operation, vars)`               | Happy path — asserts HTTP 200 + no `errors`, returns typed `data` |
| `client.execute(Operation, vars)`             | Negative tests — returns `{ response, body }` untouched           |
| `catalogApi.products()`, `.createProduct()` … | Convenience wrappers that also validate the result with zod       |

```ts
test('customer cannot create products (FORBIDDEN)', async ({ apiContextFor }) => {
  const client = new CatalogGraphqlClient(await apiContextFor('customer'));
  const { body } = await client.execute(CreateProductMutation, { input: buildProduct() });
  expect(body.errors?.[0]?.extensions?.code).toBe('FORBIDDEN');
});
```

GraphQL specifics to test: field-level authorization, `errors[].extensions.code`, partial data (`data` + `errors`), nullability,
input validation, pagination arguments. Note that HTTP status codes for GraphQL errors differ per server (see GraphQL-over-HTTP in
the references) — assert on the GraphQL error, not only on the status. When the schema is available, switch hand-written
operations to **GraphQL Code Generator** (TypedDocumentNode) — the client API stays the same.

---

## 7. Frontend E2E connected to the backend

```text
setup-<app> project  ──login via real UI──►  .auth/<app>.json (storageState, git-ignored)
                                                   │ reused by every test of that app (no login per test)
web-<app>-<browser>  ──────────────────────────────┘

Inside a test:   ARRANGE via API  ──►  ACT in the UI  ──►  ASSERT in the UI  +  ASSERT in the backend (API)
```

```ts
// tests/web/customer-portal/dashboard.spec.ts
test('buying in the UI creates an order in order-service', async ({
  customerPortal,
  catalogApi,
  orderApi,
  cleanup,
}) => {
  const product = await catalogApi.createProduct(buildProduct({ price: 7.5 })); // arrange via GraphQL
  cleanup.add(() => catalogApi.deleteProduct(product.id));

  await customerPortal.dashboard.goto(); // act in the UI
  await customerPortal.dashboard.buy(product.name);
  await expect(customerPortal.dashboard.toast).toHaveText(/^Order .+ placed$/); // assert UI

  await expect.poll(findOrder).toBeDefined(); // assert backend (eventual consistency → poll)
});
```

Rules of thumb:

- Locators: `getByRole` → `getByLabel` → `getByText` → `getByTestId`. No XPath, no CSS chains, no `nth()` without a reason.
- Never `page.waitForTimeout()`; use web-first assertions (`await expect(locator).toBeVisible()`) and `expect.poll`.
- Tests that exercise login itself start logged out: `test.use({ storageState: { cookies: [], origins: [] } })`.
- Same tests run against **local** or **staging** — only the env changes. Data is unique per test, so parallel runs on a shared staging are safe.

---

## 8. Git-ignored files & folders

### 8.1 What is ignored, why, and what to commit instead

| Path                                                          | Why ignored                                                              | Committed sample / template                                | How to create the real file                           |
| ------------------------------------------------------------- | ------------------------------------------------------------------------ | ---------------------------------------------------------- | ----------------------------------------------------- |
| `env/.env.local`, `env/.env.staging`                          | URLs + credentials per developer/environment                             | `env/.env.local.example`, `env/.env.staging.example`       | `npm run env:init`, then fill secrets                 |
| `.auth/*.json`                                                | Live session cookies/tokens                                              | `.auth/README.md` (shows the file shape), `.auth/.gitkeep` | Generated by `tests/setup/*.setup.ts` each run        |
| `test-data/private/*`                                         | Real QA accounts, masked PII, large data                                 | `test-data/private/*.example.json`, `README.md`            | Copy example, fill from vault                         |
| `.runs/<run id>/`                                             | Per-run archive: payloads, responses, results, report (may hold QA data) | —                                                          | Produced by `npx playwright test`; never auto-deleted |
| `test-results/`, `playwright-report/`, `blob-report/`         | Legacy default output folders                                            | —                                                          | Not used by this repo's config                        |
| `mobile/apps/*` (`*.apk`, `*.ipa`, `*.app/`)                  | Big binaries, signed builds                                              | `mobile/apps/README.md`, `.gitkeep`                        | Download CI artifact of the app repo                  |
| `mobile/env/.env.android`, `.env.ios`                         | Device + app config                                                      | `mobile/env/*.example`                                     | `npm run env:init`                                    |
| `node_modules/`, `mobile/logs/`, `mobile/reports/`            | Installed / generated                                                    | —                                                          | `npm ci`, test runs                                   |
| `CLAUDE.local.md`, `.claude/settings.local.json`              | Personal AI-tool settings                                                | `CLAUDE.md`, `AGENTS.md` (shared, committed)               | Create locally if needed                              |
| `.DS_Store`, `.idea/`, `.vscode/*` (except `extensions.json`) | Editor/OS noise                                                          | `.vscode/extensions.json`                                  | —                                                     |

### 8.2 How to store the ignored values safely

1. **Secrets never live in Git** — not even in a private repo or an old commit. The source of truth is the team password
   manager / vault (1Password, Bitwarden, HashiCorp Vault, AWS/GCP Secret Manager…) and **CI secrets** for pipelines.
2. **Commit a template, ignore the real file.** Every ignored config file has a `*.example` sibling with the same keys and fake
   values (`__FROM_PASSWORD_MANAGER__`). New joiners run `npm run env:init` and fill the blanks.
3. **CI does not write env files.** Pass the same variable names as CI secrets/variables; real env vars override the file.
4. **Use dedicated QA accounts** with least privilege — never personal accounts or real customer data.
5. **Keep empty folders with `.gitkeep`** (or a `README.md` that explains the folder) so the structure exists after cloning.

### 8.3 `.gitignore` patterns used in this repo

The annotated patterns live in the header and sections of [`.gitignore`](.gitignore) itself — that file is the single source.

- `dir/` ignores the folder itself → nothing inside can be re-included. Use `dir/*` when you need `!dir/keep-me`.
- Later rules override earlier ones; `!` re-includes.
- Ignore rules only affect **untracked** files.

### 8.4 Handy commands

```bash
git check-ignore -v env/.env.staging          # which rule ignores this file?
git status --ignored                          # list ignored files
git ls-files --others --ignored --exclude-standard   # same, scriptable

# Committed a secret by accident?
git rm --cached env/.env.staging && git commit -m "chore: stop tracking env file"
# …then ROTATE the secret immediately — it is still in history (rewrite history only with your security team's process).

# Personal ignores that should not be shared with the team:
echo "my-scratch/" >> .git/info/exclude             # this clone only
git config --global core.excludesFile ~/.gitignore_global   # all your repos
```

### 8.5 Samples of ignored files

`env/.env.staging` (real file, **not committed** — template: `env/.env.staging.example`):

```dotenv
START_MOCK_SERVER=false
USER_SERVICE_URL=https://api.staging.example.com/user-service
CATALOG_GRAPHQL_URL=https://graphql.staging.example.com/graphql
CUSTOMER_PORTAL_URL=https://portal.staging.example.com
CUSTOMER_EMAIL=qa.customer@example.com
CUSTOMER_PASSWORD=<from password manager>
ALLOW_DESTRUCTIVE=false
```

`.auth/customer-portal.json` (generated, **not committed**):

```json
{
  "cookies": [],
  "origins": [{ "origin": "http://localhost:3000", "localStorage": [{ "name": "cp.token", "value": "tok_…" }] }]
}
```

`test-data/private/staging-accounts.json` (**not committed** — template: `staging-accounts.example.json`):

```json
{ "accounts": [{ "label": "customer-with-orders", "email": "qa.orders@example.com", "password": "<from vault>" }] }
```

`test-data/private/credentials.<env>.json` (**not committed** — template: `credentials.<env>.example.json`):
the dedicated accounts, one per role, so a spec does not have to run as the superadmin:

```json
[{ "role": "nurse", "username": "qa.nurse", "password": "<from vault>" }]
```

The file is chosen by `TEST_ENV` (`credentials.local.json` / `credentials.staging.json`) and read at
runtime, never imported. See `docs/his-frontend-testing.md` → "Dedicated accounts per role".

---

## 9. Scaling: many REST services, GraphQL endpoints and frontend apps

The structure is **registry-driven and mirrored**: one registry entry + one folder in `src/` + the same folder in `tests/`.

> **Walkthrough with diagrams:** [docs/multi-service-collaboration.md](docs/multi-service-collaboration.md) explains the
> example product in this repo (2 REST services, 1 GraphQL endpoint, 2 frontend apps): the folder mapping, how one test
> collaborates across services (sequence diagram), the spec × service matrix, team ownership, and step-by-step tutorials for
> adding a third service and a third frontend app.

```text
config/targets.ts           src/                                   tests/
─────────────────           ────                                   ──────
restServices                api/rest/<service>/                    api/rest/<service>/
  user-service                user-service.client.ts                 auth.spec.ts, users.spec.ts
  order-service               user-service.schemas.ts
  payment-service  ← new      payment-service.client.ts  ← new       payment-service/*.spec.ts ← new
graphqlEndpoints            api/graphql/<endpoint>/                api/graphql/<endpoint>/
  catalog-gateway             catalog.client.ts / .operations.ts
  search-subgraph  ← new      search.client.ts ...       ← new
webApps                     web/<app>/                             web/<app>/   +  setup/<app>.setup.ts
  customer-portal             customer-portal.app.ts, pages/, components/
  admin-console
  merchant-portal  ← new      merchant-portal.app.ts ... ← new       → projects setup-merchant-portal + web-merchant-portal-<browser> are generated
```

**Add a REST service:** env var in `config/env.ts` + both `env/*.example` → entry in `restServices` → `src/api/rest/<service>/`
(client + schemas) → fixture in `src/fixtures/api.fixtures.ts` → specs in `tests/api/rest/<service>/` → CODEOWNERS line.

**Add a GraphQL endpoint:** same steps under `graphqlEndpoints` and `src/api/graphql/<endpoint>/` (client + operations + schemas).
Inside the endpoint, one folder per module: `src/api/graphql/<endpoint>/<module>/` + specs in `tests/api/graphql/<endpoint>/<module>/`.

**Add a frontend app:** URL env var → entry in `webApps` → `src/web/<app>/` (`<app>.app.ts`, `pages/`, `layouts/`, `components/`) → fixture
in `src/fixtures/web.fixtures.ts` → `tests/setup/<app>.setup.ts` → specs in `tests/web/<app>/<feature>/`. Projects are generated automatically.

**Using this template for a real product** (create the product repo, customize it, embed it as a submodule in the
service/app repos): **[docs/product-setup.md](docs/product-setup.md)**.

Multiple products or teams with separate release cycles → npm workspaces monorepo (`packages/core` shared, `products/<product>`
each with its own `playwright.config.ts`). Full guide, naming conventions and CI strategy: **[docs/architecture.md](docs/architecture.md)**.

---

## 10. Mobile: Android & iOS

Playwright automates browsers (including mobile **viewports** via device emulation, used here by `BROWSERS=mobile-chrome,mobile-safari`)
and has only _experimental_ Android support for Chrome/WebView — no native apps, no iOS. Native app tests therefore live in a separate
**WebdriverIO 10 + Appium 3** project in `mobile/` that shares the same conventions (screen objects ≈ page objects, env templates,
same backend and QA accounts).

```text
mobile/
├── package.json                 # own toolchain: npm ci inside mobile/
├── wdio.shared.conf.ts          # runner, mocha, appium service, screenshots on failure
├── wdio.android.conf.ts         # UiAutomator2 capabilities
├── wdio.ios.conf.ts             # XCUITest capabilities (macOS + Xcode)
├── env/.env.android.example     # ✅ committed  → .env.android (❌ ignored)
├── env/.env.ios.example
├── apps/                        # ❌ ignored binaries (.apk/.ipa/.app) — README explains where to get them
├── src/screens/                 # base.screen.ts, login.screen.ts, home.screen.ts
├── src/helpers/                 # selectors (accessibility id), app restart, env
└── tests/specs/login.spec.ts
```

```bash
cd mobile && npm ci && npm run appium:drivers     # once
npm run test:android                              # emulator running + app in mobile/apps/
npm run test:ios                                  # macOS + simulator
```

Details (prerequisites, selector strategy, device clouds, CI): **[mobile/README.md](mobile/README.md)**.

---

## 11. AI-assisted development

| File                                                                 | Audience                                                                  | Content                                                                                            |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| [`AGENTS.md`](AGENTS.md)                                             | AI coding agents (Codex, Copilot, Cursor, Claude Code via `CLAUDE.md`, …) | Project map, commands, coding standards, coding policy, recipes, definition of done                |
| [`CLAUDE.md`](CLAUDE.md)                                             | Claude Code                                                               | `@AGENTS.md` import + Claude-specific notes                                                        |
| [`.github/copilot-instructions.md`](.github/copilot-instructions.md) | GitHub Copilot                                                            | Pointer to AGENTS.md + non-negotiables                                                             |
| [`docs/ai-adoption.md`](docs/ai-adoption.md)                         | Humans                                                                    | Tool setup, Playwright Test Agents & MCP, workflow, prompt templates, review checklist, governance |

The ESLint config enforces the most important rules mechanically (missing `await`, `waitForTimeout`, `force: true`, wrong `test`
import, relative imports in specs), so AI-generated code that breaks them fails `npm run check`.

---

## 12. CI/CD

`.github/workflows/e2e.yml`:

| Trigger                       | Target                     | Scope                                                   |
| ----------------------------- | -------------------------- | ------------------------------------------------------- |
| Pull request / push to `main` | `local` (mock)             | `check` (npm) + `check-bun` (Bun) lanes, then all tests |
| Nightly (06:00 WIB)           | `staging`                  | `@smoke` + `@regression`                                |
| Manual (`workflow_dispatch`)  | choose `local` / `staging` | optional `--grep`                                       |

The `check-bun` lane installs with `bun install --frozen-lockfile` and runs `bun run check`, so the Bun
toolchain stays verified instead of documented-only.

Staging values come from **repository variables** (`STAGING_*_URL`, `STAGING_*_EMAIL`) and **secrets**
(`STAGING_CUSTOMER_PASSWORD`, `STAGING_ADMIN_PASSWORD`). Reports and traces are uploaded as artifacts for 14 days.

---

## 13. Conventions cheat sheet

| Thing       | Convention                                        | Example                                   |
| ----------- | ------------------------------------------------- | ----------------------------------------- |
| Spec file   | `<feature>.spec.ts`, kebab-case                   | `users.spec.ts`, `products.query.spec.ts` |
| Setup file  | `<app>.setup.ts`                                  | `customer-portal.setup.ts`                |
| REST client | `<service>.client.ts` → `PascalCaseServiceClient` | `UserServiceClient`                       |
| GraphQL op  | `<OperationName>Query` / `Mutation` constant      | `CreateProductMutation`                   |
| Page object | `<page>.page.ts` → `XxxPage`                      | `DashboardPage`                           |
| Component   | `<name>.component.ts`                             | `CustomerHeader`                          |
| Fixture     | camelCase noun                                    | `userApi`, `customerPortal`, `cleanup`    |
| Test title  | behaviour, present tense                          | `'customer cannot list users (403)'`      |
| `describe`  | `<service or app> · <area>`                       | `'user-service · users'`                  |
| Tags        | constants from `src/utils/tags.ts`                | `{ tag: [Tag.smoke] }`                    |
| Test data   | factory + `uniqueId()` prefix `e2e-`              | `buildUser({ role: 'admin' })`            |
| Commits     | Conventional Commits                              | `test(order-service): cover cancel flow`  |

---

## 14. Troubleshooting

| Symptom                                | Fix                                                                                                        |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `Invalid configuration for TEST_ENV=…` | Run `npm run env:init`; fill missing keys listed in the error.                                             |
| `Executable doesn't exist …`           | `npx playwright install --with-deps chromium` (or set `BROWSER_CHANNEL=chrome` to use installed Chrome).   |
| Port 3000/4001 already in use          | Stop the other process, or change the URLs in `env/.env.local` (mock reads the same vars).                 |
| Web tests start logged out / 401       | Delete `.auth/*.json` and rerun — the setup project recreates them.                                        |
| Test passes alone, fails in parallel   | Shared data. Use factories (`uniqueId`) and never depend on global counts or order.                        |
| Flaky on staging only                  | Look at the trace (`npx playwright show-trace`). Replace sleeps with `expect.poll` / web-first assertions. |

---

## 15. References / sources

The structure and rules in this template are based on the following official documentation and widely used conventions.

**Playwright (official docs)**

- Getting started — https://playwright.dev/docs/intro
- Best practices (user-facing locators, isolation, web-first assertions) — https://playwright.dev/docs/best-practices
- API testing (`APIRequestContext`, request fixture) — https://playwright.dev/docs/api-testing
- Authentication (setup project + `storageState`) — https://playwright.dev/docs/auth
- Page Object Models — https://playwright.dev/docs/pom
- Fixtures (`test.extend`, worker fixtures, `mergeTests`) — https://playwright.dev/docs/test-fixtures
- Projects & project dependencies — https://playwright.dev/docs/test-projects
- Parameterize tests, environment variables & `.env` files — https://playwright.dev/docs/test-parameterize
- Web server (`webServer` config) — https://playwright.dev/docs/test-webserver
- Annotations & tags — https://playwright.dev/docs/test-annotations
- Locators — https://playwright.dev/docs/locators
- Emulation (mobile viewports) — https://playwright.dev/docs/emulation
- Sharding — https://playwright.dev/docs/test-sharding
- Continuous integration — https://playwright.dev/docs/ci
- Android (experimental: Chrome for Android & WebView) — https://playwright.dev/docs/api/class-android
- Playwright Test Agents (planner, generator, healer) — https://playwright.dev/docs/test-agents
- Playwright MCP server — https://github.com/microsoft/playwright-mcp
- Release notes — https://playwright.dev/docs/release-notes
- eslint-plugin-playwright — https://github.com/playwright-community/eslint-plugin-playwright

**GraphQL**

- Serving over HTTP — https://graphql.org/learn/serving-over-http/
- GraphQL over HTTP specification (draft) — https://graphql.github.io/graphql-over-http/draft/
- GraphQL Code Generator — https://the-guild.dev/graphql/codegen

**Git & ignored files**

- `gitignore` reference (pattern format, negation, `dir/` vs `dir/*`) — https://git-scm.com/docs/gitignore
- `git check-ignore` — https://git-scm.com/docs/git-check-ignore
- GitHub's collection of `.gitignore` templates — https://github.com/github/gitignore

**Configuration & secrets**

- The Twelve-Factor App — Config (config in the environment) — https://12factor.net/config
- OWASP Secrets Management Cheat Sheet — https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html
- GitHub Actions — using secrets — https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets
- dotenv — https://github.com/motdotla/dotenv
- zod (schema validation) — https://zod.dev

**AI-assisted coding**

- AGENTS.md open format — https://agents.md
- Claude Code memory (`CLAUDE.md`, `@AGENTS.md` import) — https://code.claude.com/docs/en/memory
- GitHub Copilot repository custom instructions (`copilot-instructions.md`, `AGENTS.md`) — https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/add-custom-instructions/add-repository-instructions

**Mobile**

- Appium documentation — https://appium.io/docs/en/latest/
- WebdriverIO getting started — https://webdriver.io/docs/gettingstarted
- WebdriverIO Appium service (WebdriverIO 10 ↔ Appium 3) — https://webdriver.io/docs/appium-service
- Appium UiAutomator2 driver (Android) — https://github.com/appium/appium-uiautomator2-driver
- Appium XCUITest driver (iOS) — https://appium.github.io/appium-xcuitest-driver/latest/

**Test strategy & repo conventions**

- Martin Fowler — The Practical Test Pyramid — https://martinfowler.com/articles/practical-test-pyramid.html
- GitHub CODEOWNERS — https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners
- npm workspaces — https://docs.npmjs.com/cli/using-npm/workspaces
- Conventional Commits — https://www.conventionalcommits.org/en/v1.0.0/

> Versions verified when this template was created (Nov 2026): Playwright 1.64, TypeScript 6.0 (typescript-eslint does not yet
> support TS 7), zod 4, ESLint 10, WebdriverIO 10, Appium 3, Node.js 22 LTS.
