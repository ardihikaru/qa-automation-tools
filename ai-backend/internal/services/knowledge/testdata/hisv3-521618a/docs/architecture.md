# Architecture & scaling guide

> **Catatan (HISv3):** nama service/app di dokumen ini (`user-service`, `order-service`, `catalog-gateway`, `customer-portal`, `admin-console`) adalah _contoh fiktif_ dari template awal untuk menjelaskan pola; implementasinya sudah dihapus dan `mock-server/` tidak ada lagi. Service HISv3 yang sebenarnya: [hisv3-services.md](hisv3-services.md).

How this structure grows from one product with a few services to many services, many frontends, many teams.

For a concrete, diagram-based walkthrough of the example in this repo (2 REST + 1 GraphQL + 2 frontend apps, cross-service
tests, team collaboration and step-by-step tutorials), see [multi-service-collaboration.md](multi-service-collaboration.md).

## 1. Principles

1. **Registry-driven.** `config/targets.ts` is the only place that knows which services/endpoints/apps exist. Web Playwright projects and the
   default client URLs are derived from it; fixtures are added by hand.
2. **Mirrored folders.** `src/<layer>/<name>/` (how to talk to it) ↔ `tests/<layer>/<name>/` (what we verify). The name is the
   service/app name used by the product teams, in kebab-case.
3. **One client per service, one app object per frontend.** Tests depend on these abstractions, never on URLs or selectors.
4. **Layers point downwards only:** `tests → fixtures → (page objects | API clients) → config`.
5. **Traceability.** Every test maps to a catalog case (`docs/test-catalog/`), see [test-strategy.md](test-strategy.md).
6. **Ownership follows folders** (`.github/CODEOWNERS`), so each product team reviews its own tests.

## 2. Level 1–2: one product, many services and apps (this template)

```text
config/targets.ts
  restServices      { user-service, order-service, payment-service, notification-service, … }
  graphqlEndpoints  { catalog-gateway, search-subgraph, … }
  webApps           [ customer-portal, admin-console, merchant-portal, … ]

src/api/rest/<service>/          <service>.client.ts, <service>.schemas.ts
src/api/graphql/<endpoint>/      <endpoint>.client.ts, <endpoint>.operations.ts, <endpoint>.schemas.ts
src/web/<app>/                   <app>.app.ts, pages/, components/
src/web/shared/                  BasePage + components shared by several apps (design-system widgets)
src/fixtures/                    api.fixtures.ts (one fixture per client), web.fixtures.ts (one per app)

tests/api/rest/<service>/        one spec per resource/feature
tests/api/graphql/<endpoint>/    *.query.spec.ts, *.mutation.spec.ts
tests/web/<app>/                 one spec per user journey
tests/setup/<app>.setup.ts       login once per app (and per role if needed: <app>.<role>.setup.ts)
tests/e2e-journeys/              OPTIONAL: cross-app journeys (e.g. merchant creates product → customer buys it)
```

### 2.1 The folders mirror the product repos

The template above is flat on purpose. **In this repo the folders follow the product repos**, so the test
that covers a piece of code sits in the folder that mirrors it:

| Kind       | Product repo                        | Test folder                                                                       |
| ---------- | ----------------------------------- | --------------------------------------------------------------------------------- |
| GraphQL    | `his-backend/pkg/graph/<module>/`   | `src/api/graphql/<endpoint>/<module>/` + `tests/api/graphql/<endpoint>/<module>/` |
| Web page   | `his-frontend/src/pages/<feature>/` | `src/web/<app>/pages/<feature>/` + `tests/web/<app>/<feature>/`                   |
| Web layout | `his-frontend/src/layouts/<area>/`  | `src/web/<app>/layouts/<area>/`                                                   |

So `his-backend/pkg/graph/user/` becomes `src/api/graphql/his-backend/user/`, and
`his-frontend/src/pages/auth/` becomes `src/web/his-frontend/pages/auth/` + `tests/web/his-frontend/auth/`.
A new module or page gets a new folder; nothing is added flat.

Generated Playwright projects scale automatically:

```text
api-rest · api-graphql
setup-<app>            → for each webApps entry
web-<app>-<browser>    → for each webApps entry × each browser in BROWSERS
```

Run one team's slice: `npx playwright test tests/api/rest/payment-service` or `--project=web-merchant-portal-chromium`.

### When an app has several roles

Add a setup per role and a project per role, e.g. `admin-console.admin.setup.ts` → `.auth/admin-console.admin.json` and
`admin-console.viewer.setup.ts` → `.auth/admin-console.viewer.json`. In a spec choose the session with
`test.use({ storageState: '.auth/admin-console.viewer.json' })`.

### When services sit behind one API gateway

Keep **one client per downstream service** even if they share a host (`https://api.staging.example.com/payment-service`).
`BaseApiClient.url()` joins gateway prefixes safely, so only the env value changes.

### Microfrontends / shared design system

Put shared widgets (date picker, data table, toast) in `src/web/shared/components/` and compose them in each app's pages.

## 3. Level 3: many products / many teams → npm workspaces monorepo

When products have independent release cycles or > ~5 teams contribute, split into workspaces but keep **one repo** so the core
stays consistent:

```text
automation/
├── package.json                 # "workspaces": ["packages/*", "products/*"]
├── AGENTS.md                    # global rules
├── packages/
│   ├── core/                    # BaseApiClient, GraphqlClient, expectJson, env loader, tags, uniqueId, BasePage
│   └── api-clients/             # optional: clients generated from OpenAPI / GraphQL schemas, shared by products & mobile
├── products/
│   ├── marketplace/
│   │   ├── AGENTS.md            # product-specific rules (closest AGENTS.md wins)
│   │   ├── playwright.config.ts # same pattern as this template
│   │   ├── config/  src/  tests/
│   │   └── env/*.example
│   └── payments/
│       └── …
└── mobile/                      # WebdriverIO + Appium, can import packages/api-clients for data seeding
```

Rules for the monorepo:

- `packages/core` changes need QA-platform review and a version bump (or run all products in CI).
- Products never import from each other — only from `packages/*`.
- Each product has its own CI job (path filters), report and Slack channel.

## 4. When to split into separate repositories

Only when teams need fully independent tooling versions or access control. Cost: duplicated core, drift. If you do split,
publish `core` as an internal npm package and keep this template as the starter for every new repo.
For the template → product repo → submodule workflow, see [product-setup.md](product-setup.md).

## 5. CI strategy at scale

| Pipeline          | Trigger                              | Scope                                                      |
| ----------------- | ------------------------------------ | ---------------------------------------------------------- |
| PR — changed area | pull request (path filters)          | specs under the changed `tests/<layer>/<name>/` + `@smoke` |
| Service deploy    | after deploying service X to staging | `tests/api/**/<x>/` + web specs tagged for that service    |
| Nightly           | schedule                             | full `@regression` on staging, sharded                     |
| Release gate      | before production release            | `@smoke` on staging + manual approval                      |

Use `--shard=i/n` and merge blob reports for large suites; keep `@smoke` under ~5 minutes.

## 6. Naming conventions

| Item                 | Pattern                                                                                       |
| -------------------- | --------------------------------------------------------------------------------------------- |
| Service / app folder | kebab-case, same name the product team uses (`payment-service`, `merchant-portal`)            |
| Project name         | `api-rest`, `api-graphql`, `setup-<app>`, `web-<app>-<browser>`                               |
| Env var              | `<NAME>_SERVICE_URL`, `<NAME>_GRAPHQL_URL`, `<APP>_URL`, `<ROLE>_EMAIL` / `<ROLE>_PASSWORD`   |
| Session file         | `.auth/<app>[.<role>].json`                                                                   |
| Test data prefix     | `e2e-<scope>-…` (purge job on staging can delete anything with this prefix older than N days) |
