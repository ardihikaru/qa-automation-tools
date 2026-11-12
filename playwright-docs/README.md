# Playwright Documentation Generator

Record browser flows and generate documentation automatically using Playwright.

## Overview

`playwright-docs` drives a Chromium browser through predefined HIS (Hospital
Information System) frontend flows, one role at a time, and captures a
screenshot after every step (plus optional video). It also contains an AI
summarizer that turns a flow's steps into a Markdown document, and a standalone
route explorer.

- `src/index.ts` (`main`) is the entry point. It loads the shared `config.yaml`
  through the repository-level config loader ([../lib/README.md](../lib/README.md),
  `lib/config.ts`), launches the browser, maps each configured credential role
  to a flow role, and calls `recordFlow` for every matching flow.
- `src/flows/flowLoader.ts` holds the flow definitions in code (there are no
  YAML/JSON flow files; the top-level `flows/` directory is empty).
- `src/recorder/recordFlow.ts` executes the steps and writes screenshots.
- `src/summarizer/summarizeFlow.ts` calls the AI gateway. The default provider
  is the production **ai-manager** (reached over WireGuard). Calling MiniMax
  directly is a fallback you select with `provider: "minimax"`. The same
  provider model is used by the Go backend ([../ai-backend/README.md](../ai-backend/README.md)).
- `src/exploreSystem.ts` is a separate script that visits a fixed list of HIS
  routes and saves full-page screenshots.

> Note: the summarizer is **not wired into `main`**. `index.ts` imports only
> `loadFlows` and `recordFlow`, so `npm run dev` produces screenshots/videos
> but no Markdown in `output/docs/`. `generateDocumentation` has to be called
> explicitly.

## Requirements

- **Node.js**: >= 18.0.0 (LTS recommended; `package.json` `engines`). Node 18+
  is also required for the global `fetch` / `AbortSignal.timeout` used by the summarizer.
- **npm**: >= 9.0.0
- **Google Chrome** at `/usr/bin/google-chrome`, or set `CHROME_PATH`
  (`index.ts` passes `executablePath: process.env.CHROME_PATH || '/usr/bin/google-chrome'`).

## Installation

```bash
# Install dependencies
npm install

# Install Playwright browsers (only needed if you point CHROME_PATH at a Playwright build)
npx playwright install chromium
```

> Note: This project uses a centralized configuration from the parent directory's `config.yaml` file. Ensure the parent directory has the required node_modules (yaml, dotenv).

## Running

```bash
# Run using npx ts-node (recommended)
npx ts-node src/index.ts

# Or use the npm script
npm run dev

# Standalone route explorer (hardcoded target, see Invariants)
npx ts-node src/exploreSystem.ts
```

Known-broken `package.json` scripts (do not rely on them):

- `npm run record` points to `src/recorder.ts`, which does not exist (the recorder is `src/recorder/recordFlow.ts` and has no CLI entry).
- `npm test` runs `playwright test`, but the package has no spec files or `playwright.config`.
- `npm run build` (`tsc`) fails with TS6059 because `tsconfig.json` sets `rootDir: ./src` but includes `../lib/**/*`. `npm start` depends on that build.

## Configuration

The project uses the centralized `config.yaml` from the parent directory. Update the following sections:

- **staging.url** / **staging.timeout**: Base URL and navigation timeout (`getStaging`)
- **credentials**: Role-based login credentials keyed by role name (`getCredentials`, `getAllCredentials`)
- **playwright_docs.browser**: Browser settings (`headless`, `args`, `viewport`)
- **playwright_docs.output**: `docs_dir` (logged only), `video_dir`, `record_video`
- **playwright_docs.flows.roles**: Roles to generate docs for
- **playwright_docs.flows.continue_on_error**: Keep running a role's remaining flows after a failure
- **backend.ai**: AI provider settings (see below)

`lib/config.ts` `getConfigPath` searches, in order: `./config.yaml`,
`../config.yaml`, `<lib>/../../config.yaml`, then `~/.config/qa-automation/config.yaml`.

### AI provider (summarizer)

`getAI()` in `lib/config.ts` resolves the AI settings. **Environment variables
take precedence over `config.yaml`:**

| Env var | `AIConfig` field | Default |
|---------|------------------|---------|
| `AI_PROVIDER` | `provider` | `ai-manager` |
| `AI_MANAGER_URL` | `manager_url` | `http://10.10.0.4:8099` |
| `AI_MANAGER_LOGIN_ID` | `manager_login_id` | empty (required for ai-manager) |
| `AI_MANAGER_SECRET_KEY` | `manager_secret_key` | empty (required for ai-manager) |
| `AI_MANAGER_PROVIDER` | `manager_provider` | `claude-v3` (Claude CLI REST on rnd-v3, via ai-manager) |
| `AI_MANAGER_NO_FALLBACK` | `manager_no_fallback` | `true` |
| `AI_MANAGER_MODEL` | `manager_model` | empty (`claude-opus-5`) |
| `AI_TIMEOUT_SECONDS` | `timeout_seconds` | `180` |
| `MINIMAX_API_KEY` | `api_key` | empty (only used when `provider = minimax`) |

Keep the ai-manager credentials in the environment, not in `config.yaml`.

`config/credentials.example.yaml` is a local example file. No code in this
package reads `config/credentials.yaml`; credentials come from `config.yaml`.

## Interface

- `src/index.ts`
  - `main(): Promise<void>` runs on import (`main().catch(console.error)`) and is also exported.
- `src/flows/flowLoader.ts`
  - `interface FlowStep { action: 'click' | 'type' | 'select' | 'wait' | 'navigate'; selector?; value?; wait? }`
  - `interface FlowDefinition { id; name; role; description; steps: FlowStep[] }`. `role` is one of `admin | doctor | cashier | super_admin | nurse | admission | pharmacy | laboratory`.
  - `loadFlows(): FlowDefinition[]` concatenates the super_admin, admin, doctor, nurse, admission, cashier, pharmacy, laboratory and radiographer flow arrays.
  - `getFlowsByRole(role: string): FlowDefinition[]`
- `src/recorder/recordFlow.ts`
  - `interface Credential { username; password; role_code }`
  - `interface FlowConfig { staging: { url; timeout }; api: { backend_url; minimax_api_key (deprecated); ai?: AIConfig } }`
  - `recordFlow(page, flow, credentials, config, options = defaultOptions): Promise<void>`
  - `RecorderOptions` (type export: `outputDir`, `videoDir`, `screenshotDir`, `defaultWait`)
- `src/summarizer/summarizeFlow.ts`
  - `callAI(request: SummarizationRequest, config: SummarizerConfig): Promise<SummarizationResponse>` is the provider-aware entry point.
  - `generateDocumentation(flowName, flowDescription, role, steps, config, outputDir): Promise<string>` returns the written `.md` path.
  - `callMinimax(request, config: MinimaxConfig)` is **deprecated**. It forces `provider: 'minimax'`.
  - Types: `SummarizationRequest`, `SummarizationResponse`, `SummarizerConfig` (`Partial<AIConfig> & { api_key?; base_url? }`), `MinimaxConfig` (deprecated).
- `src/exploreSystem.ts` is a script with no exports. It runs `main()` on load.
- Shared config: `loadConfig`, `getStaging`, `getCredentials`, `getAllCredentials`, `getPlaywrightDocs`, `getBackendURL`, `getAI` and `AIConfig` come from `lib/config.ts` ([../lib/README.md](../lib/README.md)).

## Invariants

- **The default AI provider is ai-manager.** `callAI` uses `config.provider || 'ai-manager'`, and `getAI()` defaults `provider` to `ai-manager`. Only `provider === 'minimax'` routes to `chatViaMiniMax`.
- `chatViaAIManager` throws when `manager_login_id` or `manager_secret_key` is empty. It sends HTTP Basic auth, `caller: 'qa-automation-playwright-docs'`, `temperature: 0.7`, `provider` (`manager_provider`, default `claude-v3`), optional `model`, and `no_fallback` (default `true`). It reads the reply from the `text` field, and throws if `text` is empty or the reply comes from a provider other than the pinned one.
- `chatViaMiniMax` defaults to `https://api.minimax.io/v1/chat/completions` and model `MiniMax-M2.5`. It reads `choices[0].message.content`.
- Both AI calls time out after `timeout_seconds` (default 180 s).
- `callAI` never throws. On any provider error it logs the error and returns a fallback `SummarizationResponse` (`flow_description`, the joined steps, and `'Error generating ...'` placeholders).
- Response parsing is heuristic. `feature_description` is the first non-empty line, `edge_cases` are lines starting with `-` (at most 5), `key_points` are lines starting with `*` (at most 5), and `user_flow` is always the input steps.
- In `index.ts`, `roleMapping` maps credential role names (for example `doctor_gp`, `nurse_icu`, `radiographer`, `finance`) to flow roles. Unmapped names map to themselves. A flow runs for a role when `flow.role === flowRole`, or for every flow when `roleName === 'all'`. Roles with no credentials are skipped.
- If a flow fails, the remaining flows for that role are skipped unless `playwright_docs.flows.continue_on_error` is true.
- Flow step `value`s may use the placeholders `{{username}}`, `{{password}}` and `{{role_code}}`. `replacePlaceholders` replaces only the first occurrence of each.
- `click` and `type` steps accept comma-separated fallback selectors (`tryClickWithFallback` / `tryFillWithFallback`). A miss is logged, not thrown.
- `navigate` steps treat `selector` as a path appended to `staging.url`.
- `recordFlow` ignores `playwright_docs.output.docs_dir`. The screenshot root is the `options.outputDir` default `output`, relative to the process cwd.
- `exploreSystem.ts` does not use `config.yaml`. Its target URL, login credentials, route list and `output/exploration` directory are hardcoded in its `config` object. Do not run it against other environments without editing the script. The embedded credentials are a known hygiene issue.

## Side Effects

- **Browser automation**: launches Chrome/Chromium (`chromium.launch`) and logs into the HIS staging app with the configured role credentials.
- **Disk writes** (paths are relative to the cwd):
  - `recordFlow` writes `output/screenshots/<staging-host>/<role_code>/<flow.id>-step-N.png`, `<flow.id>-final.png`, and `<flow.id>-error-step-N.png` on step failure. It creates directories with `mkdirSync({ recursive: true })`.
  - Videos go to `<playwright_docs.output.video_dir>/<staging-host>/` when `record_video` is true.
  - `generateDocumentation` writes `<outputDir>/<flow-name-kebab>.md` (`fs.writeFileSync`).
  - `exploreSystem.ts` writes `output/exploration/route-*.png` and `error-*.png`.
- **Network (AI)**: `callAI` sends `POST {manager_url}/v1/chat` (ai-manager, default) or `POST` to the MiniMax chat-completions URL (fallback). The prompt includes the flow name, role, description and steps.
- **Process exit**: `main` calls `process.exit(1)` on a top-level error.
- **Console logging** of progress. `recordFlow` logs the typed values, including the substituted password.

## Output

Generated artifacts are saved to:

- `output/screenshots/`: Screenshot images (per staging host and role code)
- `output/videos/`: Recorded videos (if enabled)
- `output/docs/`: Documentation files (only when `generateDocumentation` is called; see the Overview note)
- `output/exploration/`: Route explorer screenshots

## Project Structure

```
playwright-docs/
├── src/
│   ├── index.ts              # Main entry point (main)
│   ├── exploreSystem.ts      # Standalone HIS route explorer
│   ├── flows/
│   │   └── flowLoader.ts     # Flow definitions (in code)
│   ├── recorder/
│   │   └── recordFlow.ts     # Flow recorder
│   └── summarizer/
│       └── summarizeFlow.ts  # AI summarizer (ai-manager default, MiniMax fallback)
├── config/                   # Local config (credentials.example.yaml)
├── flows/                    # Empty (flows live in src/flows/flowLoader.ts)
├── recorder/                 # Empty placeholder
├── summarizer/               # Empty placeholder
├── output/                   # Generated artifacts (ignored)
├── package.json
└── tsconfig.json             # Includes ../lib/**/* (shared lib/config.ts)
```

## Dependencies

- **playwright**: Browser automation
- **yaml**: YAML parsing
- **dotenv**: Environment variables
- **@types/node**: Node.js types
- **@types/yaml**: YAML types
- **ts-node**: TypeScript execution
- **typescript**: TypeScript compiler

## Verification

- Type-check (works around the `rootDir` issue that breaks `npm run build`):
  `cd playwright-docs && npx tsc --noEmit --rootDir ..`
- Smoke run against staging (needs `config.yaml`, Chrome, and network access):
  `cd playwright-docs && npm run dev`
- There is no automated test suite. `npm test` finds no specs.
