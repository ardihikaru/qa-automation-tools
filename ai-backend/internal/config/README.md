# config

## Overview

Package `config` builds the process-wide `*Config` for ai-backend. Values come
from environment variables (optionally loaded from a `.env` file through
`godotenv`), with non-secret overrides read from an optional `config.yaml`.
The result is cached in a package-level singleton. Consumers include
`cmd/server`, `cmd/databasemigration`, `cmd/verifyai`, [db](../db/README.md)
(`GetDatabaseURL`) and [services/ai](../services/ai/README.md) (`AIConfig`).

## Interface

- `config.go:Config` - root struct (`Staging`, `Credentials`, `Backend`,
  `PlaywrightDocs`, `PlaywrightQA`, `Logger`, `Dashboard`, `CostOptimization`,
  `Monitoring`, `Development`, `Production`), plus nested types
  `BackendConfig`, `DatabaseConfig`, `AIConfig`, `SecurityConfig`,
  `StagingConfig`, `CredentialsConfig`, `EnvOverrides`, and the
  Playwright/logger/dashboard/cost/monitoring config structs.
- `config.go:LoadConfig() *Config` - returns the cached config, building it on
  first call.
- `config.go:Config.GetDatabaseURL() string` - MySQL DSN for go-sql-driver/mysql.
- `config.go:Config.GetBackendAddress() string` - `host:port` for the HTTP server.
- `config.go:ClearCache()` - resets the singleton (for tests).
- Environment variables read in `config.go:LoadConfig` (default shown only when
  it is not a secret):

| Variable | Field | Default |
| --- | --- | --- |
| `BACKEND_HOST` | `Backend.Host` | `0.0.0.0` |
| `PORT` | `Backend.Port` | `8080` |
| `DB_HOST` | `Backend.Database.Host` | hard-coded legacy host in `config.go` (set `DB_HOST` explicitly) |
| `DB_PORT` | `Backend.Database.Port` | hard-coded legacy port in `config.go` (set `DB_PORT` explicitly) |
| `DB_USER` | `Backend.Database.Username` | non-empty built-in default |
| `DB_PASSWORD` | `Backend.Database.Password` | empty (secret) |
| `DB_NAME` | `Backend.Database.Name` | `qa_automation` |
| `DATABASE_URL` | `Backend.Database.ConnectionString` | unset (secret-bearing) |
| `AI_PROVIDER` | `Backend.AI.Provider` | `ai-manager` |
| `AI_TIMEOUT_SECONDS` | `Backend.AI.TimeoutSeconds` | `180` |
| `AI_MANAGER_URL` | `Backend.AI.ManagerURL` | `http://10.8.0.4:8099` |
| `AI_MANAGER_LOGIN_ID` | `Backend.AI.ManagerLoginID` | empty (secret) |
| `AI_MANAGER_SECRET_KEY` | `Backend.AI.ManagerSecretKey` | empty (secret) |
| `AI_MANAGER_PROVIDER` | `Backend.AI.ManagerProvider` | `claude-v3` |
| `AI_MANAGER_NO_FALLBACK` | `Backend.AI.ManagerNoFallback` | `true` |
| `AI_MANAGER_MODEL` | `Backend.AI.ManagerModel` | empty |
| `PLAYWRIGHT_EXECUTOR_URL` | `Backend.Executor.URL` | empty (simulated jobs). Prod: `http://10.8.0.9:8141` |
| `PLAYWRIGHT_EXECUTOR_TOKEN` | `Backend.Executor.Token` | empty (secret) |
| `PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES` | `Backend.Executor.TimeoutMinutes` | `30` |
| `REPO_POOL_DIR` | `Backend.Products.PoolDir` | empty (clone/verify disabled). Prod: `/data/repos` |
| `PRODUCTS_VERIFY_INTERVAL_MINUTES` | `Backend.Products.VerifyIntervalMinutes` | `360` (`0` disables the scheduled verify) |
| `GH_TOKEN` | not in `Config` | read by `gh`/`git` directly from the process env (secret, never logged) |
| `MINIMAX_API_KEY` (dormant) | `Backend.AI.APIKey` | empty (secret) |
| `AI_MODEL` | `Backend.AI.Model` | `MiniMax-M2.5` |
| `AI_BASE_URL` | `Backend.AI.BaseURL` | `https://api.minimax.io/v1` |
| `AI_CHAT_ENDPOINT` | `Backend.AI.ChatEndpoint` | `/chat/completions` |
| `API_SECRET_KEY` | `Backend.Security.APISecretKey` | empty (secret) |
| `CORS_ENABLED` | `Backend.Security.CORSEnabled` | `true` |
| `CORS_ORIGINS` | `Backend.Security.CORSOrigins` | `*` |
| `STAGING_URL` | `Staging.URL` | empty |
| `GRAPH_API_URL` | `Staging.GraphAPIURL` | empty |
| `REST_API_URL` | `Staging.RestAPIURL` | empty |
| `STAGING_TIMEOUT` | `Staging.Timeout` | `30000` |
| `NAVIGATION_TIMEOUT` | `Staging.NavigationTimeout` | `30000` |
| `ACTION_TIMEOUT` | `Staging.ActionTimeout` | `5000` |
| `NODE_ENV` | selects `Development` / `Production` overrides | unset |

## Invariants

- `config.go:LoadConfig` is a memoized singleton (`config.go:cfg`). Later env
  changes are ignored until `ClearCache()` is called.
- Precedence in `LoadConfig`: env/defaults first, then
  `config.go:loadYAMLConfig`, then `DATABASE_URL`, then `applyOverrides` for
  `NODE_ENV=development|production`.
- `config.go:loadYAMLConfig` tries `config.yaml`, then `../config.yaml`, then
  `./config.yaml`, and stops at the first one that reads successfully. A YAML
  value only overrides a field whose env var is unset. Secrets
  (`ManagerLoginID`, `ManagerSecretKey` are tagged `yaml:"-"`; DB password,
  API keys) are never taken from YAML. `CORSEnabled` from YAML can only turn
  CORS on.
- `config.go:Config.GetDatabaseURL` priority: (1) `Backend.Database.ConnectionString`
  verbatim (set from `DATABASE_URL` by `LoadConfig`); (2) `DATABASE_URL`
  verbatim unless it starts with `postgres` (legacy Neon URLs are ignored);
  (3) DSN built from `DB_*` as
  `user:pass@tcp(host:port)/dbname?parseTime=true&loc=Local&charset=utf8mb4&collation=utf8mb4_general_ci&multiStatements=true`.
  It returns `""` when host, user, or name is empty.
- Caveat: `LoadConfig` copies any non-empty `DATABASE_URL` into
  `ConnectionString` without the `postgres` check, so with a config from
  `LoadConfig` step (1) wins and the legacy-URL filter in step (2) does not
  apply. `loadYAMLConfig` never copies `connection_string` from YAML, despite
  the doc comment on `GetDatabaseURL`.
- `getEnvInt` falls back to the default on unparsable input. `getEnvBool`
  treats only `true`/`1`/`yes` (case-insensitive) as true.
- `Backend.AI.Provider` values other than `minimax` fall through to the
  ai-manager branch (both in the `LoadConfig` log switch and in
  `services/ai.NewFromConfig`).

## Side Effects

- `config.go:LoadConfig` calls `godotenv.Load()`, which reads `.env` from the
  working directory into the process environment if present.
- `config.go:loadYAMLConfig` reads `config.yaml` from disk.
- `config.go:LoadConfig` logs the server port, DB host, and selected AI
  provider/URL (and whether credentials are missing) through `log`. It does
  not log secret values.

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/config/...` (the package
  has no test files)
