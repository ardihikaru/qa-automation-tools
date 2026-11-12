# lib

## Overview

`lib/config.ts` is the shared TypeScript config loader for the Node projects. It finds and parses a `config.yaml` file using the `yaml` package, applies environment overrides, caches the result in memory, and exposes typed getters for each section.

Who imports it (checked by grep across `dashboard`, `playwright-qa`, `playwright-docs`, and `this-qa-automation-system-playwright`, excluding `node_modules`):

- **playwright-docs:**
  - [`src/index.ts`](../playwright-docs/src/index.ts) imports `loadConfig`, `getStaging`, `getCredentials`, `getPlaywrightDocs`, `getAllCredentials`, `getBackendURL`, and `getAI`.
  - [`src/recorder/recordFlow.ts`](../playwright-docs/src/recorder/recordFlow.ts) and [`src/summarizer/summarizeFlow.ts`](../playwright-docs/src/summarizer/summarizeFlow.ts) import only the `AIConfig` type.
- **playwright-qa:**
  - [`src/explorer/random.ts`](../playwright-qa/src/explorer/random.ts) imports from `'../../lib/config'`. From `playwright-qa/src/explorer/` that path resolves to `playwright-qa/lib/config`, which does not exist, so the import does not reach this file.
- **dashboard, this-qa-automation-system-playwright:** no imports found.
- **ai-backend:** no imports. It is Go and has its own config package, even though the file header comment lists `ai-backend` and `dashboard` as users.

## Interface

- **Loader:**
  - `config.ts:loadConfig(): Config`
  - `config.ts:clearCache(): void`
  - `config.ts:validateConfig(): { valid, errors }`
- **Section getters:**
  - `config.ts:getStaging`, `getCredentials(role)`, `getAllCredentials`, `getBackend`, `getDatabase`
  - `config.ts:getAI`, `getPlaywrightDocs`, `getPlaywrightQA`, `getLogger`, `getDashboard`, `getCostOptimization`, `getMonitoring`
- **Derived values:**
  - `config.ts:getDatabaseURL(): string`
  - `config.ts:getBackendURL(path?): string`
  - `config.ts:getAPIKey(): string`
- **Default export:** an object containing all of the functions above.
- **Exported types:**
  - Top level: `Config`.
  - Sections: `StagingConfig`, `Credential`, `CredentialsConfig`, `BackendConfig`, `DatabaseConfig`, `AIConfig`, `SecurityConfig`.
  - Playwright: `BrowserConfig`, `OutputConfig`, `FlowsConfig`, `PlaywrightDocsConfig`, `ExplorationConfig`, `LearningConfig`, `GoalConfig`, `GoalsConfig`, `SkillsConfig`, `PlaywrightQAConfig`.
  - Other sections: `LoggerConfig`, `DashboardConfig`, `CostOptimizationConfig`, `MonitoringConfig`, `EnvOverrides`.
- **Config file read:** `config.yaml`. Key names are listed below; the key layout is documented in [`config.yaml.template`](../config.yaml.template).
  - Top-level keys (`config.ts:Config`): `staging`, `credentials`, `backend`, `playwright_docs`, `playwright_qa`, `logger`, `dashboard`, `cost_optimization`, `monitoring`, plus optional `development` and `production`.
  - `backend` keys: `host`, `port`, `database`, `ai`, `security`.
  - `backend.database` keys: `host`, `port`, `username`, `password`, `name`, `connection_string`.
  - `backend.ai` keys: `provider`, `manager_url`, `manager_login_id`, `manager_secret_key`, `manager_provider`, `manager_model`, `manager_no_fallback`, `timeout_seconds`, `api_key`, `model`, `embedding_model`, `base_url`.
  - `backend.security` keys: `api_secret_key`, `cors_enabled`, `cors_origins`.
  - `playwright_qa` keys: `browser`, `exploration`, `learning`, `goals`, `skills`.
  - `playwright_docs` keys: `browser`, `output`, `flows`.

## Invariants

- **Config file lookup** (`config.ts:getConfigPath`) returns the first `config.yaml` that exists, in this order:
  1. `$CWD/config.yaml`
  2. `$CWD/../config.yaml`
  3. `<__dirname>/../../config.yaml`
  4. `$HOME/.config/qa-automation/config.yaml`

  If none exists, `config.ts:loadConfig` throws `Config file not found`.
- **Caching:** `config.ts:loadConfig` parses once and caches in the module-level `cachedConfig`. Later edits to the file are not seen until `config.ts:clearCache` runs.
- **Environment overrides:** `config.ts:loadConfig` reads `NODE_ENV` (default `development`) and uses `Object.assign` to merge the matching `development` or `production` block over the top-level config. The merge is shallow, so an override section replaces the whole section.
- **AI settings:** in `config.ts:getAI`, environment variables take precedence over `config.yaml`:

  | Variable | Fallback |
  | --- | --- |
  | `AI_PROVIDER` | `ai-manager` |
  | `AI_MANAGER_URL` | an in-code default URL |
  | `AI_MANAGER_LOGIN_ID` | empty string |
  | `AI_MANAGER_SECRET_KEY` | empty string |
  | `AI_MANAGER_PROVIDER` | `claude-v3` |
  | `AI_MANAGER_NO_FALLBACK` | `true` (any value other than `false`) |
  | `AI_MANAGER_MODEL` | empty string |
  | `AI_TIMEOUT_SECONDS` | `180` |
  | `MINIMAX_API_KEY` | empty string |

  This matches the backend's provider choice: ai-manager by default, MiniMax as the fallback.
- **Validation:** `config.ts:validateConfig` requires `staging.url`, at least one entry in `credentials`, and `backend.database.host`. It also requires `api_key` when `provider = minimax`; otherwise it requires both ai-manager credentials.
- **Database URL:** if `connection_string` is unset, `config.ts:getDatabaseURL` builds a `postgres://...?sslmode=require` URL. That is the legacy Postgres format, not MySQL. Prefer `connection_string`.
- **Backend URL:** `config.ts:getBackendURL` always uses `http://`.

## Side Effects

- Filesystem reads only: `fs.existsSync` and `fs.readFileSync` in `config.ts:getConfigPath` and `config.ts:loadConfig`.
- Reads `process.env`: `NODE_ENV`, `HOME`, and the AI variables listed in `config.ts:getAI`.
- Mutates module state through `cachedConfig` (`config.ts:loadConfig`, `config.ts:clearCache`).
- No network calls.

## Verification

- Type-check: run `npx tsc --noEmit -p tsconfig.json` from the repo root. The root [`tsconfig.json`](../tsconfig.json) has `include: ["lib/**/*"]`.
  - The repo root has no `package.json` and no local TypeScript install. Use the compiler installed in `playwright-docs`: `playwright-docs/node_modules/.bin/tsc --noEmit -p tsconfig.json`. This passes cleanly.
  - `cd playwright-docs && npx tsc --noEmit` currently fails with TS6059 (`lib/config.ts` is not under `rootDir`) because that project's `rootDir` is `src`.
- No unit tests exist for this file.
