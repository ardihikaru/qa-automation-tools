# NAVIGATION_MAP: a route and page map of the product, with test coverage and the next cases to automate

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Rev 3 (2026-11-21), implemented (section 12 lists what changed during implementation). Rev 2 was approved for implementation after one EGM round (design, implementation readiness, codebase details; resolutions in section 11). The product owner asked for a deep analysis and an implementation of `/navigation` now that case automation works end to end. Operator approval is delegated to the orchestrating agent under the product owner's standing instruction ("work among your agents, never as my confirmation"). Not deployed.
*   **Depends on:** [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (the sync, its mirror and `kb_*`), [CASE_RESULTS.md](CASE_RESULTS.md) (`kb_case_results`), [CASE_AUTOMATION.md](CASE_AUTOMATION.md) (`case_run` / `case_create`, the executor), [EPIC_AUTOMATION.md](EPIC_AUTOMATION.md) (eligibility).
*   **Contract:** nothing sent to GAOS changes, and no extra GAOS push is caused (the catalog sync's skip logic and `parserVersion` are untouched). The executor's `case_run` result gains one optional field, `nav`.

## 1. Problem

`/navigation` ("Navigation Graph", `dashboard/src/views/NavigationGraphView.tsx`) draws `learning_cache` rows from `GET /api/learning?limit=100`. They were written by the exploratory crawler (`qa_run`), removed in `2de1bdb refactor(playwright-executor): drop qa_run and docs_generate, keep case_run`. Today no production client writes `learning_cache` or `navigation_edges`: the only writers are the generic handlers `POST /api/learning` (`router.go:1312`, called only by the E2E spec `tests/api/learning.spec.ts`) and `POST /api/learning/navigation` (no caller); `learning.AddToCache` and `AddNavigationEdge` have no caller in the backend. Production `GET /api/learning` returns `[]`, so the page is empty and stays empty.

Meanwhile the QA tools hold real, maintained sources of the product's surface:

*   the knowledge base: 1,700 SUT cases, each with a scenario that usually names the route it calls first (`GET /api/v1/service-status`, `GET /kiosk-sut-v2/health`), its spec files (`spec_paths`), its story and through the story its features; features whose "Repo / entry point" (`kb_features.repo_entry`) and name hold routes (``sut-vendor-backend `/api/v1/swagger/*` ``, ``… (`/master-patient`)``);
*   the automation repo, read by the sync's mirror: page objects (`protected readonly path = '/report-builder/list'`), the app object that holds them (`SutFrontendApp.loginPage`), fixtures, REST clients (``/** `POST /omnichannel/booking` … */``, `this.get('service-status')`) and the specs that use them;
*   per-case results (`kb_case_results`) and case jobs (`job_runs`);
*   the runs: every `case_run` / `case_create` run goes through playwright-executor, where a reporter sees each `page.goto` and each API request.

The 2026-11-20 run picked VSYS and TC-VKAUTH-007 by hand as "trivial next cases". That choice — a planned case on a route that already has a passing spec — can be computed.

## 2. What "navigation" means now (decision)

**A route map of the product, coloured by test coverage, with a drill-down and a "next to automate" list.**

1.  **Map.** Every frontend page (`surface = web`, per frontend app) and every REST route (`surface = api`, per backend service) that the catalog, the automation repo or a real run names, shown as a path tree (`/api/v1` → `kiosk-sut-v2` → `health`).
2.  **Coverage overlay.** Per route: its cases (automated, planned, other), the latest result of each automated case, the last case job, whether a run visited it; a status colour; gaps (routes with no case, routes with only planned cases). Cases with no route (GraphQL, message-queue, data-driven chains) are counted as `unmapped`, so the map never claims more than it knows.
3.  **Drill-down.** Route → features → cases → latest result / last job → the job page.
4.  **Next to automate.** Eligible planned cases on a route that already has a passing automated case, with that case and its spec as the example to copy.

**Out of scope (stated as gaps on the page):** GraphQL operations (`tests/api/graphql/**`, e.g. `/user/query`), streams, message-queue consumers, and the data-driven vendor specs that call the client through `src/data/vendor-cases/` (their cases still get routes from their scenarios).

**Retired:** the crawler graph on `/navigation`. It has no data source, and a page-to-page edge graph with success rates cannot be rebuilt from the new sources (specs say which pages a test opens, not in which order a user moves between them). The new page does not call `/api/learning*`. The `learning_*`, `navigation_edges`, `action_sequences` tables and `/api/learning*` are **not** touched: they belong to the `/learning` redesign (`feat/learning-loop`). The URL `/navigation` stays (bookmarks keep working); the sidebar label becomes "Navigation map".

Not chosen: crawling the live app (credentials, the staging budget, pages without coverage); parsing sut-frontend's own router (not a repo of the pool; the module catalogue already names the frontend routes of features).

## 3. Sources

All extraction is in a new pure package `internal/services/navmap` (no DB, clock or I/O; it does not import `knowledge`): `Build(Input) Inventory`, deterministic (outputs sorted).

```go
type Feature struct{ Code, Name, RepoEntry string }
type Epic    struct{ Code, Service string }
type Story   struct{ Code string; FeatureCodes []string }
type Case    struct{ Code, EpicCode, StoryCode, Scenario, Polarity string; SpecPaths []string }
type Input struct {
    Features []Feature; Epics []Epic; Stories []Story; Cases []Case
    Files map[string][]byte // repo files (3.3); others ignored
}
type Route struct {
    Key, Surface, App, Path string
    Methods, Sources, PageObjects []string // sorted, unique
}
type Link struct{ RouteKey, RefType, Ref, Source, Method string } // RefType feature|case|spec
type Inventory struct{ Routes []Route; Links []Link; Problems []string }
```

`knowledge` converts its `*Snapshot` to `Input`.

### 3.1 Route identity and `NormPath`

A route is `(surface, app, path)`; `Key = hex(sha1(surface + "\x00" + app + "\x00" + path))` (40 chars). `NormPath(p)`, in order:

1.  drop `scheme://host[:port]` (and a bare `host:port/` prefix), the query and the fragment;
2.  `<name>` → `:name`, `{name}` → `:name`, `\$\{[^}]*\}` → `:param`; existing `:name` and a final `*` are kept; no percent-decoding;
3.  collapse `//`; strip one trailing `/` (not on `/`);
4.  a segment that is all digits, a UUID, or 16+ hex characters → `:id`;
5.  reject (return `""`) when the result contains `..`, whitespace, a control character, a backtick or a quote, does not start with `/`, is `/:x` or `/*` alone, or is longer than 300 bytes.

A path in text ends at a backtick, whitespace, `)`, `,`, `;`, `—` or the end. `Match(pattern, path)`: same segment count (or a final `*` that matches one or more rest segments), and each pattern segment equals the path segment or is `:x`.

### 3.2 Catalog

*   **Feature routes.** If `repo_entry` contains a token ending in `-frontend`, each backticked span of `name` or `repo_entry` that starts with `/` and not `/api/` is a **web** route of the first such app. Otherwise the first token of `repo_entry` (`^[a-z0-9-]+`) is the **api** app and each backticked span of `repo_entry` and `name` that, after an optional method word, starts with `/` is an api route of it. Link `feature → route`, source `catalog`.
*   **Case routes (scenario).** Only the **first** `\b(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+` followed by an optional backtick and a `/…` path in the case's scenario. App: the case's epic's `service` mapped to a known app (itself, else `his-<service>`, else itself); known apps are the apps of feature routes and the service folders of 3.3. Link `case → route` with that method, source `scenario`.
*   **Negative probes are dropped.** A route whose only sources are scenario links of cases with polarity `-` (no positive case, no feature, no repo, no observation) is removed with its links (`/api/v1/nonexistent`, `/api/v1/thisisnotaroute`, `/kiosk-sut-v2/nonexistent`).
*   **Case via feature.** For each feature of the case's story, a `case → route` link per feature route, source `feature`. Context only (3.6).

### 3.3 Automation repo

`KeepForSync` also keeps `*.ts` files under `src/web/`, `src/api/rest/`, `src/fixtures/`, `tests/web/`, `tests/api/rest/` (about 400 KiB for SUT today; the 32 MiB tree cap is shared with `docs/`). `navmap` skips a file over 256 KiB with a problem. The catalog parser reads only `docs/`.

*   **Page objects** (any `.ts` under `src/web/<app>/`): each `^\s*(export\s+)?(abstract\s+)?class\s+(\w+)` starts a class; `(?:protected|public|private)?\s*(?:override\s+)?readonly\s+path(?:\s*:\s*string)?\s*=\s*['"\x60]([^'"\x60]+)['"\x60]` belongs to the nearest preceding class → **web** route of `<app>`, page object = class name. A class without a `path` has no route.
*   **Members** (any `.ts` under `src/web/<app>/`): `readonly\s+(\w+)\s*:\s*(\w+)\s*;` where the type is a page-object class → member name means that class (`hisFrontend.loginPage` → `LoginPage`).
*   **Fixtures** (`src/fixtures/*.ts`, multi-line): `^\s{2,}(\w+)\s*:\s*async\s*\(` followed, within the next 400 bytes, by `use\(\s*new\s+(\w+)\(` → fixture name means class.
*   **Literal navigations** in page objects and web specs: `goto\(\s*['"\x60]([^'"\x60]+)['"\x60]` → web route of the file's app (templates normalised by `NormPath`).
*   **Web specs** (`tests/web/<app>/**/*.spec.ts`). A spec is split into **test blocks** at each `\btest(\.only|\.skip|\.fixme)?\(\s*['"\x60]`; the text before the first block plus `test.beforeEach` bodies is **shared**. A block uses page class C when the block or the shared text contains `new C(`, `\.m\b` for a member `m` of C, a destructured fixture name of C (`async\s*\(\s*\{([^}]*)\}`), or a literal navigation. Its codes are `caseresults.CaseCodes(title)` (`TC-[A-Z]+-[0-9]{3}`, the same regex the result parser uses).
*   **REST clients** (`src/api/rest/<service>/**/*.ts`). A method starts at `^\s{2}(?:(?:public|private|protected|static|async|readonly)\s+)*(\w+)\s*\(` at two-space indentation (not `constructor`, `super`, `if`, `for`, `while`, `switch`, `return`); its body runs to the next method start. A `private`/`protected` method has no routes. Routes of a public method: the `` `METHOD[|METHOD…] /path` `` spans of the `/** … */` block that ends on the line just above it; if there are none, the paths of `this\.(get|post|put|patch|delete|url)\(\s*['"\x60]([^'"\x60]+)` in its body (relative paths get a leading `/`; method from the verb, `GET` for `url`). → **api** routes of `<service>`.
*   **API specs** (`tests/api/rest/<service>/**/*.spec.ts`), split into blocks as above. A block uses client method `m` of `<service>` when it contains `\.m\(` and `m` is a public method of that service's clients (`get post put patch delete fetch url call request` ignored); literal `\$\{env\.\w+\}(/[^\x60'"\s?]*)` URLs are api routes of `<service>`.
*   **Links.** `spec → route` for every route any block or the shared text of the spec uses (source `repo`). `case → route` (source `spec`): for a case whose `spec_paths` hold the spec, the routes of the blocks whose title names the case plus the shared routes; if no block names it, all routes of the spec.

### 3.4 Mount prefix (api)

Per api app, the **mount** is the most frequent `/api/v<N>` two-segment prefix among its routes from all sources (ties: lexically smallest); none, no mount. The mount is prepended to every route of that app that does not start with `/api/` (client-relative paths, kiosk scenarios, `/kiosk-sut-v2` features), then routes merge by key. For sut-vendor-backend this gives `/api/v1/service-status` (GET, POST, DELETE), `/api/v1/swagger/*`, `/api/v1/swagger/:relativePath` and `/api/v1/kiosk-sut-v2/…`. Observed paths already contain the mount (`SUT_VENDOR_BACKEND_URL` ends in `/api/v1`).

### 3.5 Real runs (observed)

**Reporter.** `playwright-executor/reporters/nav-reporter.cjs` (CommonJS, no dependency, empty constructor). `onStepEnd(test, result, step)`: only `step.category === 'pw:api'`; title `Navigate` → web; title in `GET POST PUT PATCH DELETE HEAD OPTIONS {method}` → api with method `step.params?.method || title` (`{method}` → `GET`); URL `step.params?.url ?? step.subtitle`, resolved with `new URL(url, test.parent.project()?.use?.baseURL)`; a relative URL with no base, and any non-`http(s)` URL, is dropped. Kept per entry: `title` (the test title), `file` (relative to the repo, from `test.location.file`), `project`, `kind`, `method`, `host`, `path` (pathname only; the query is never kept). Aggregated by all of these with a `hits` count; at most 500 entries, then `truncated: true`. `onEnd` writes `{ "version": 1, "truncated": bool, "entries": [...] }` to `process.env.QA_NAV_OUTPUT`. Every hook is wrapped in `try/catch`. `step.params` and `step.subtitle` are not part of Playwright's documented reporter API; both were checked on 1.64.1 (the SUT repo) and a test runs the reporter in a real Playwright run of the executor's own pinned version.

**Executor.** `caseRun.ts` adds the reporter (`--reporter=json,list,<abs path>`) and `QA_NAV_OUTPUT=<runDir>/nav.json` only when the reporter file exists, `require()` of it succeeded at executor start, and `EXECUTOR_NAV_REPORTER` is not `0`. It reads `nav.json` with the same `within`/`isFile` guards as `results.json`, cap 256 KiB, and returns it as `result.nav` (omitted when absent or invalid; a warn event says why).

**Backend.** `navObservations(nav, pid)`: drop entries whose title has no case code or whose project starts with `setup-` or `vendor-e2e-`; one candidate per code in the title. App: `tests/web/<app>/` → web app, `tests/api/rest/<service>/` → api app, else `""`. The candidate's path is `NormPath`ed and matched (`Match`) to the product's `nav_routes` of the same surface (same app when known; else any app; the longest literal match wins, ties by key). **Unmatched candidates are counted, never stored**, so a raw path with a name or a booking number never reaches the database; what is stored is the route key. Rows: `nav_observations(product_id, route_key, job_id, run_id, case_code, method, hits, observed_at)`, upsert on `(job_id, run_id, case_code, route_key, method)` with `hits = VALUES(hits)` (a resumed job re-stores the same run idempotently; a `case_create` repair is a new executor run id).

*   `finishCaseRun` reads and deletes `result["nav"]` **first** (before any early return) and sets `result["nav_count"] = {stored, unmatched}`.
*   `executorRunner.RunCase` stores `final.Result["nav"]` with `e.jobID`, `e.pid` and `final.ID`, before computing the verdict.

Observations older than 180 days are deleted by the same store call (`DELETE … WHERE product_id = ? AND observed_at < UTC_TIMESTAMP() - INTERVAL 180 DAY`).

### 3.6 Evidence levels

Status colour and the "next" list use only **direct** case links: `scenario`, `spec`, and observations. `feature` links are shown as "related through a feature". A case that has no direct link to any route counts as `unmapped`.

## 4. Data (`migrations.go`, appended: four entries, each one Go string with a Go comment, unguarded `CREATE TABLE IF NOT EXISTS`)

```sql
CREATE TABLE IF NOT EXISTS nav_routes (
  product_id INT NOT NULL,
  route_key CHAR(40) NOT NULL,
  surface VARCHAR(8) NOT NULL,
  app VARCHAR(100) NOT NULL,
  path VARCHAR(300) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  methods JSON NOT NULL,
  sources JSON NOT NULL,
  page_objects JSON NOT NULL,
  PRIMARY KEY (product_id, route_key),
  CONSTRAINT fk_nav_routes_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS nav_links (
  product_id INT NOT NULL,
  route_key CHAR(40) NOT NULL,
  ref_type VARCHAR(8) NOT NULL,
  ref VARCHAR(300) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  source VARCHAR(10) NOT NULL,
  method VARCHAR(8) NOT NULL DEFAULT '',
  PRIMARY KEY (product_id, route_key, ref_type, ref, source, method),
  INDEX idx_nav_links_ref (product_id, ref_type, ref),
  CONSTRAINT fk_nav_links_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS nav_builds (
  product_id INT NOT NULL PRIMARY KEY,
  sync_id INT NULL,
  commit_sha CHAR(40) NULL,
  nav_version INT NOT NULL,
  status VARCHAR(10) NOT NULL,          -- ok | failed
  error VARCHAR(500) NOT NULL DEFAULT '',
  routes INT NOT NULL DEFAULT 0,
  links INT NOT NULL DEFAULT 0,
  problems JSON NULL,                   -- at most 50 strings
  built_at DATETIME NOT NULL,
  CONSTRAINT fk_nav_builds_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS nav_observations (
  product_id INT NOT NULL,
  route_key CHAR(40) NOT NULL,
  job_id VARCHAR(64) NOT NULL,
  run_id VARCHAR(64) NOT NULL DEFAULT '',
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  method VARCHAR(8) NOT NULL DEFAULT '',
  hits INT NOT NULL DEFAULT 1,
  observed_at DATETIME NOT NULL,
  PRIMARY KEY (job_id, run_id, case_code, route_key, method),
  INDEX idx_nav_obs_route (product_id, route_key, observed_at),
  CONSTRAINT fk_nav_obs_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

Rollback: the tables are inert; the old binary ignores them. If `feat/run-history` (`pw_run_cases`) is on `main` when this is implemented, the drill-down shows the case's last `pw_run_cases.duration_ms`; otherwise the last case job's `job_runs.duration_ms`.

## 5. When the inventory is built

**Outside the catalog transaction.** In `Syncer.run`:

*   after a **succeeded** sync's commit: `s.buildNav(syncID, t, sha, snap, files)`;
*   on the **skipped** path (scheduled sync, unchanged commit): if `nav_builds` has no row for the product or its `nav_version <> navmap.Version`, parse the snapshot from the files already read (`t.format.Parse`, pure) and `buildNav` with the previous succeeded sync's id; the sync still ends `skipped` and `OnUnchanged` is called as today (no catalog write, no `OnSucceeded`, no GAOS push).

`buildNav`: `navmap.Build` under `recover()` (a panic is a failed build), then one transaction with its own 1-minute context: `DELETE FROM nav_links WHERE product_id = ?`, `DELETE FROM nav_routes WHERE product_id = ?`, batched inserts (300 rows per statement), and an upsert of `nav_builds` (`status='ok'`, counts, the first 50 problems). Any error rolls the inventory back (the previous one stays) and upserts `nav_builds` with `status='failed'` and the error. **A nav failure never changes the sync's status**; it is logged as `knowledge: sync <id> (product <pid>): navigation map: <err>`. So the map fills itself at the first scheduled sync after the deploy, or at once with **Sync now**.

## 6. API

Under the product router (`products.go`: `case parts[1] == "navigation"`), cookie auth, any authenticated role (read-only, like `knowledge/summary`). Errors via `writeJSONError`.

*   `GET /api/products/{id}/navigation?surface=&app=&status=&q=` → 200
    ```json
    {"product_id":3,
     "build":{"status":"ok","sync_id":42,"commit":"…","built_at":"…","error":"","problems":["…"]} | null,
     "apps":[{"surface":"api","app":"sut-vendor-backend","routes":31}],
     "totals":{"routes":0,"passing":0,"failing":0,"automated":0,"planned":0,"untested":0,"observed":0,"unmapped_cases":0},
     "routes":[{"key":"…","surface":"api","app":"…","path":"…","methods":[],"sources":[],"page_objects":[],
                "status":"passing","cases":0,"automated":0,"planned":0,"pass":0,"fail":0,"features":0,"specs":0,
                "observed_hits":0,"last_observed_at":null,"last_result_at":null}],
     "next":[{"case_code":"…","priority":"P1","epic":"…","route_key":"…","path":"…","app":"…","method":"GET",
              "tier":1,"reason":"…","example_case":"…","example_spec":"…"}]}
    ```
    *   **Case sets.** A route's cases are the active (`removed_at IS NULL`) cases with a direct link to it; a wildcard route `X/*` also has the cases of every route under `X/` of the same app, and so has a route `X` (not `/`) with no case of its own (a catalog prefix such as `/api/v1/kiosk-sut-v2`).
    *   **Status:** `failing` (an automated case's latest `kb_case_results.result` is `fail`), else `passing` (≥ 1 `pass`), else `automated` (automated cases, no result), else `planned` (cases, none automated), else `untested`. `automated` = `kb_test_cases.automated = 1`; `planned` = `status = 'planned'`.
    *   **Filters:** `surface` (`web|api`), `app` (exact), `status` (one of the five), `q` (case-insensitive substring of the path, a page object, or a case code of the route). They filter `routes`; `totals` count the filtered routes (`unmapped_cases` ignores filters); `next` honours `surface` and `app`. Unknown `surface`/`status` → 400.
    *   **`next`** (at most 25, one entry per case). Candidates: the product's eligible cases — the predicate of `eligibleCodes` (`status = 'planned'`, `reason_category = ''`, `JSON_LENGTH(spec_paths) = 0`, active, no non-`planned` `gaos_decisions` row) shared as one SQL fragment `eligibleCaseWhere` by both, minus `jobExclusions` — that have a `scenario` link to a route R. **Tier 1:** R has a passing automated case whose direct link carries the **same method** (web: any). **Tier 2:** R has a passing automated case with another method. No other tiers. A case on several routes keeps its best tier, then the lexically smallest path. Order: tier, priority (`P0..P3`, empty last), code. `example_case`/`example_spec`: the passing case (smallest code) and its first spec; `reason`: "GET /api/v1/kiosk-sut-v2/health has a passing GET case (TC-…)".
    *   `build` is `null` when the product has never been built: the page says "Run a knowledge sync".
*   `GET /api/products/{id}/navigation/routes/{key}` (`key` = 40 hex) → the route object above plus `features:[{code,name,release_status}]`, `cases:[{code,scenario,status,automated,priority,epic,link_sources:[],result,run_at,run_ref,last_job:{id,type,status,completed_at,duration_ms}|null}]` (direct links), `related_cases:[{code,status,feature}]` (`feature` links, at most 100), `specs:[path]`, `observations:[{job_id,case_code,method,hits,observed_at}]` (latest 50). 404 on an unknown key. `last_job` is the case's newest `case_run`/`case_create` row in `job_runs` (one grouped query over the route's cases).

## 7. Dashboard (`/navigation`)

`views/navigation/NavigationMapView.tsx` replaces `views/NavigationGraphView.tsx` (deleted); `App.tsx` routes `/navigation` to it; `Layout.tsx` label "Navigation map". Fetches use `api`/`ApiError` from `views/products/api.ts` and `useLoad` from `views/knowledge/hooks.ts`; UI bits reuse `views/knowledge/ui.tsx` (`Loading`, `ErrorNote`, `Empty`, `RelTime`, `CaseStatusChip`).

*   **Product:** `?product=<id>`; without it, the first product (by id) whose `knowledge_format` is set; a select lists all products.
*   **Filters in the URL:** `surface` (All / Pages / API), `app`, `status`, `q` (debounced).
*   **Header strip:** counters per status (each a filter toggle), unmapped cases with the gap note (GraphQL, MQ, data-driven), the build's sync, commit and time; "Run a knowledge sync" (link to `/products/:id/knowledge`) when `build` is null; the build error when `failed`.
*   **Tree:** one section per `(surface, app)`; rows are path segments, indented, collapsible; a route row has a status dot, `automated/cases`, pass/fail, and an "observed" mark; a segment row shows the worst status of its subtree and its counts. All rows open when `q` is set. Selecting a route opens the drill-down; the selection is in the URL (`route=<key>`).
*   **Drill-down** (right panel ≥ lg, below on smaller screens): methods, sources, page objects; features (link `/products/:id/knowledge/features/:code`); cases (link to the case page, status chip, latest result and time, last job link `/jobs/:id`); related cases; specs; observations (job links).
*   **Next to automate:** the `next` list with tier, reason, priority and the example; each case links to its case page (where **Create automation** is).
*   Each block has loading, empty and error states. The page root fills the content width (no `max-w-*` on the root).

## 8. Safety

Read-only for users; no LLM call. The reporter sends nothing: the executor reads a file it wrote. Query strings are never kept; raw observed paths never reach the database (only matched route keys). Per-file and per-run caps bound the scan and the observations; observations expire after 180 days. No change to auth allow-lists, GAOS payloads or `kb_*` rows.

## 9. Target files

*   `ai-backend/internal/services/navmap/` (new): `navmap.go` (types, `Build`, `Version`), `path.go` (`NormPath`, `Match`), `catalog.go`, `repo.go`, `coverage.go` (status, wildcard roll-up, tiers; pure), `README.md`, tests and `testdata/` (trimmed copies of the SUT files the tests need).
*   `ai-backend/internal/services/caseresults/parse.go` (export `CaseCodes`).
*   `ai-backend/internal/services/knowledge/`: `assistants.go` (`KeepForSync`), `sync.go` (the two `buildNav` call sites), `nav_store.go` (new), tests, `README.md`.
*   `ai-backend/internal/db/migrations.go` (four entries, appended).
*   `ai-backend/internal/api/`: `navigation.go` (new), `navigation_test.go` (new), `products.go` (route), `case_automation.go` (store `nav`), `epic_batches.go` (`eligibleCaseWhere`), `README.md`.
*   `playwright-executor/`: `reporters/nav-reporter.cjs` (new), `src/runs/caseRun.ts`, `src/config.ts` (`EXECUTOR_NAV_REPORTER`), tests, `README.md`, `.env.example`.
*   `dashboard/src/`: `views/navigation/` (new), `App.tsx`, `components/Layout.tsx`; delete `views/NavigationGraphView.tsx`.
*   `this-qa-automation-system-playwright/tests/api/navigation.spec.ts`, `tests/dashboard/navigation-map.spec.ts` (new, read-only against prod, skipped without env); `tests/dashboard/layout-width.spec.ts` keeps visiting `/navigation` (unchanged).
*   `docs/designs/SITEMAP.md`, `README.md` (API table).

## 10. Verification

*   `go test -mod=mod ./internal/services/navmap/`: `NormPath` table (ids, `<x>`, templates, `..`, query, host, single-param); `Match`; catalog extraction (VSYS → `/api/v1/service-status` GET/POST/DELETE, `/api/v1/swagger/*`; kiosk → `/api/v1/kiosk-sut-v2/health`; negative probes dropped; first-path-only); repo extraction on `testdata` (page objects with two classes in one file; `hisFrontend.loginPage` → `/auth/login`; fixtures across lines; client JSDoc vs private `call`); mount inference; status and wildcard roll-up; tiers; determinism (shuffled input).
*   With `QA_TEST_MYSQL_DSN`: a sync over a fake tree builds the inventory and a second sync replaces it; a skipped sync builds a missing inventory and does not call `OnSucceeded`; a nav failure leaves the sync `succeeded`; observation storage (match, unmatched not stored, idempotent upsert); both handlers (filters, 400, 404, `next` excludes a case with an active job and a non-planned GAOS decision).
*   Executor: `caseRun` passes the reporter only when allowed; reads `nav.json` with the guards; the reporter in a real Playwright run (one `request.get` against a local server).
*   Dashboard: `npx tsc --noEmit`, `npm run lint`, `npm run build`; a local run against a DB synced from the local SUT automation repo (desktop and phone screenshots).
*   After deploy (separate approval): `GET /api/products/3/navigation?surface=api&app=sut-vendor-backend` lists `/api/v1/service-status`, `/api/v1/swagger/*`, `/api/v1/kiosk-sut-v2/…`; `?surface=web` the sut-frontend pages of the web specs; the E2E specs above.

## 11. Review resolutions (Rev 1 → Rev 2)

| Finding | Resolution |
|---|---|
| Build inside the sync transaction can fail or slow the catalog | Built after the commit in its own transaction; a nav failure never changes the sync status (section 5) |
| `parserVersion` bump re-syncs every product and pushes to GAOS | Not bumped; `nav_builds.nav_version` triggers a nav-only build on the skipped path (section 5) |
| Observed raw paths can hold names, birth dates, booking numbers | Only matched route keys are stored; unmatched paths are counted (3.5) |
| Web specs reach pages through `hisFrontend.<member>` | Member map from `readonly m: C` (3.3) |
| Setup/teardown visits credited to the case | Entries without a code in the title and `setup-*`/`vendor-e2e-*` projects dropped (3.5) |
| App from a hyphenated project name is ambiguous | App from the spec folder (3.5) |
| `RunCase` has no result map; repair runs overwrite hits | Stored in `RunCase` keyed by executor run id (3.5) |
| Scenario regex picks secondary mentions and negative probes | First method+path only; negative-only routes dropped (3.2) |
| `<x>` placeholders, multi-line calls and fixtures, two classes per file, private transport | Defined in 3.1 and 3.3 |
| Multi-page specs link every case to every page | Per-test blocks with case codes in titles; whole-spec only as fallback (3.3) |
| "Next" tier 2 "same parent" is too broad; missing `gaos_decisions` exclusion | Tiers only on the same route (method match first); shared eligibility fragment (section 6) |
| Per-file cap not possible in `KeepForSync`; 32 MiB risk | Cap applied in `navmap`; measured ~400 KiB extra (3.3) |
| Reporter load error aborts the run; API not public | `require()` check at start, flag, file-exists check; real-run test (3.5) |
| GraphQL/MQ/data-driven not covered | Declared out of scope; `unmapped_cases` shown (section 2) |
| Observation growth | 180-day expiry; only matched rows (3.5) |
| `learning_cache` claim | Corrected (section 1) |
| Status colours not in `styles.ts` | Reuse `ui.tsx` (section 7) |
| Migration layout | Four Go string entries with Go comments (section 4) |

## 12. Implementation notes (Rev 3)

1.  **Reporter titles differ by Playwright version.** The executor's own Playwright (1.59.1) names request steps `GET "<url>"` and navigations `Navigate to "<url>"`, with no `params`/`subtitle`; the SUT repo's 1.64.1 uses `GET`/`Navigate` with the URL in `step.params.url`. The reporter reads both; `test/navReporter.test.ts` runs it in a real 1.59.1 run, and it was checked by hand on 1.64.1 (requests and a `page.goto`). Spec files are reported relative to the config file's folder (Playwright's `rootDir` is the testDir).
2.  **Prefix roll-up.** A route with no case of its own stands for the routes under it (section 6), so the catalog's module prefixes (`/kiosk-sut-v2`, `/omnichannel`, `/vtt`) show their area's status instead of "no test case".
3.  **The OnSucceeded hook runs before the navigation build**, so the GAOS push is not delayed by it.
4.  **Route detail shape.** In `GET …/routes/{key}`, `cases`, `features` and `specs` are lists (they replace the counts of the map's route object). `feat/run-history` was on `main` at implementation time, so each case also has `last_run: {run_id, result, duration_ms, started_at, source} | null` from `pw_run_cases`/`pw_runs`, and the panel links to `/history?tab=runs&product=<id>&case=<code>`. `feat/learning-loop` also landed: it kept `/api/learning*` as the legacy crawler API; this page no longer reads it.
5.  **Mount from all sources** (3.4), not only features and scenarios: a REST client's JSDoc that writes `/api/v1/…` is evidence too.
6.  **Local check.** `TestNavLocalRepo` (`NAV_LOCAL_REPO`) builds the map from a local SUT checkout: 231 routes (sut-frontend 36 pages, sut-vendor-backend 87, sut-data-integration 99, sut-backend 9), 1,392 of 1,809 cases with a direct route. `TestNavDevSeed` (`NAV_DEV_DSN`) fills a local database for the dashboard.
