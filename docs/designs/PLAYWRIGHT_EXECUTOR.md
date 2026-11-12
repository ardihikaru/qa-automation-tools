# playwright-executor: a resource-capped Playwright service, and claude-v3 as the only LLM path

*   **Author:** ardihikaru
*   **Status:** Approved (implementation-first at the user's request on 2026-11-13; a Goldfish review via `/egm:egm-review` is offered after the fact)
*   **Date:** 2026-11-13

--------------------------------------------------------------------------------

## Overview

All browser automation moves into one small service, **playwright-executor** (`playwright-executor/`). It runs on rnd-v3 as a **systemd user unit**, uses Node from **mise**, and is capped by the kernel at **2.5 GiB RAM** and **60 % of all CPUs**.

ai-backend's job endpoints, which until now were simulated, dispatch real runs to it. Every LLM call stays on one path: ai-manager pinned to **claude-v3**, the Claude CLI REST that runs on the same VM.

## The Problem

- Playwright had no runtime path. `playwright-qa` didn't compile (bad `lib` import, `run.ts` signature mismatches), and every module ran `main()` on import. `playwright-docs` was a script you ran by hand on a laptop.
- `POST /api/jobs/qa/run` and `/api/jobs/docs/generate`, which the dashboard already calls, only slept for 200 ms (`router.go` `enqueueJob`).
- Running Chromium inside the repo's tooling, or inside the backend container, would let it compete without limit for rnd-v3's 4 vCPU and 3.8 GiB, which are shared with Docker (qa-automation, gaos) and `ai-claude-cli`.
- Docs still presented MiniMax as a co-equal provider, although production only uses ai-manager → claude-v3.

## Goals

*   One process owns all Playwright execution. It has an HTTP API, a bearer token, concurrency 1, and a per-run timeout.
*   A hard resource budget: `MemoryMax=2560M`, `MemoryHigh=2304M`, `MemorySwapMax=0`, `CPUQuota = nproc × 60 %` (240 % on rnd-v3), `TasksMax=512`. These are enforced by the cgroup on node and every Chromium child.
*   The service is managed like the other rnd-v3 services: a systemd user unit (linger), its toolchain from `mise.toml`, and logs in journald.
*   The dashboard's **Run QA** button and docs generation produce real results: issues stored in `raw_issues`, and Markdown docs written by claude-v3 through `ai.Service`.
*   **claude-v3 via ai-manager is the only supported LLM path.** MiniMax code stays **dormant** (the user chose not to remove it). It is a manual emergency switch that needs explicit approval.

## Non-Goals

*   Porting guided or planning exploration modes and learned skills. `playwright-qa`'s versions were stubs. Only random exploration and built-in skills are ported.
*   Serving screenshots over HTTP. Artifacts stay on rnd-v3 disk for 7 days, and their paths are in the job result.
*   Durable queueing. Runs live in executor memory, and jobs orphaned by a backend restart are marked failed.
*   Deleting `playwright-qa/` and `playwright-docs/`. They are deprecated now and will be deleted in a later PR.
*   Removing MiniMax code (decided: keep it dormant).

## Critical User Journeys (CUJs)

1.  **Run QA from the dashboard.**
    *   *Given* the executor is healthy, *when* a user clicks Run QA (`POST /api/jobs/qa/run`), *then* the backend submits `qa_run` to the executor and polls every 3 s. The job moves `queued` → `running` → `completed`, and its result holds `summary`, `issues` and `issues_recorded`.
    *   New issues appear in `/api/issues`. Duplicates of open issues are skipped.
2.  **Generate docs.**
    *   *Given* `POST /api/jobs/docs/generate`, *when* the executor finishes recording, *then* the backend asks claude-v3 (via ai-manager) to write Markdown. The job result holds `documentation_markdown`, or `ai_error` plus the raw flows if the AI call fails.
3.  **Runaway browser.**
    *   *Given* a run that hangs, *when* `RUN_TIMEOUT_SECONDS` (900) passes, *then* the browser is killed, the run is marked `failed`, and the next queued run starts only after the old browser has closed.
    *   If memory ever exceeds 2.5 GiB, the kernel OOM-kills inside the executor's cgroup only. systemd restarts the service.
4.  **Executor down.**
    *   Backend `/health` shows `"executor":"unreachable"`. `deploy.sh` warns but doesn't fail. Jobs fail with a clear error.
    *   Clearing `PLAYWRIGHT_EXECUTOR_URL` reverts to simulated jobs (`result.simulated=true`).

## Technical Plan (Architecture)

```
browser ─TLS─▶ clino edge ─WG─▶ 10.10.0.9:8140 dashboard ──▶ backend (Docker, rnd-v3)
                                                              │  POST /v1/runs, GET /v1/runs/:id (Bearer)
                                                              ▼
                                   rnd-v3 host: playwright-executor.service (systemd --user, mise node)
                                   10.10.0.9:8141   cgroup: MemoryMax 2.5G · CPUQuota 240% · Tasks 512
                                                              │  Chromium (1 at a time)
                                                              ▼
                                                      HISv3 STAGING (UI login per role)
backend ─WG─▶ 10.10.0.4:5306 MySQL qa_automation (job_runs, raw_issues)
backend ─WG─▶ 10.10.0.4:8099 ai-manager ── provider claude-v3 (no_fallback) ─WG─▶ 10.10.0.9:8105 ai-claude-cli (rnd-v3)
```

*   **Executor** (`playwright-executor/src`): a framework-free `node:http` server (`server.ts`).
    *   `RunQueue` (`queue.ts`): concurrency 1, FIFO, `MAX_QUEUE`, timeouts through `AbortSignal`, a 24 h in-memory TTL.
    *   Run handlers: `runs/qaRun.ts` (ported from `playwright-qa` `QARunner.randomStep`, `loginAs` and built-in skills) and `runs/docsGenerate.ts` (ported from `playwright-docs` `recordFlow`/`flowLoader`, with login steps and credentials removed).
    *   Config is env only (`config.ts`). Credentials come from a mode-600 JSON file generated from `config.yaml` `credentials`.
*   **Pull model.** The backend submits and polls, and the executor never calls the backend, so it needs no backend credentials and no new auth path (`IsAuthAllowList` is unchanged).
*   **Backend.**
    *   `internal/services/executor` is the client: `Submit`, `Get`, `Wait`, and `Health` with a 30 s cache.
    *   `internal/api/jobs_executor.go` has `SetExecutor`, `runExecutorJob`, `recordExecutorIssues`, `summarizeDocsRun` and `FailOrphanedJobs`.
    *   `enqueueJob` dispatches to the executor when `PLAYWRIGHT_EXECUTOR_URL` is set, and simulates otherwise.
    *   Only whitelisted params (`roles`, `max_steps`, `interactive_skills`, `flows`) are forwarded.
    *   `/health` gains `executor: ok|degraded|unreachable|not_configured`. Existing keys are unchanged, including `minimax`.
*   **Config** (env): `PLAYWRIGHT_EXECUTOR_URL`, `PLAYWRIGHT_EXECUTOR_TOKEN`, `PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES` (30). Production values are in `deploy/his-v3-qa/.env.prod`.
*   **Deploy** (`deploy/playwright-executor/`): `install.sh` runs mise install, npm ci/build, `playwright install chromium`, `sudo install-deps`, renders the unit with `CPUQuota = nproc × 60 %`, enables and restarts it, then runs a health gate.
*   **Timeouts.** Executor run 900 s < backend wait 30 min, which covers queueing behind up to one other run. The docs AI call stays within the existing AI timeout chain (180 s client).

## Alternatives Considered

### Run Playwright in a Docker container in the compose stack
`deploy.resources.limits` could cap it as well. Rejected because:
- the user asked for systemd and mise;
- the Playwright image is about 1.5 GB;
- every backend deploy would rebuild it;
- rnd-v3 already runs its non-Docker services (`ai-claude-cli`) as systemd user units.

### Wrap the existing playwright-qa / playwright-docs packages
This would need the compile errors and the run-on-import `main()` calls fixed in place, and the hard-coded staging credentials kept. A clean port into one service was smaller and safer.

### Executor pushes results to the backend
This would need a service identity in the cookie-only auth (`auth.Require`), which is a new allow-list decision. Polling avoids that.

## Non-Functional Requirements

*   **Security.**
    *   Bearer token of at least 32 chars, compared in constant time.
    *   Bound to the WG IP only, never `0.0.0.0`.
    *   The env and credentials files are mode 600 and never printed.
    *   No target URL is taken from requests, and random clicks skip submit, logout and destructive controls.
*   **Resources.**
    *   Kernel-enforced cgroup limits, plus concurrency 1, `--renderer-process-limit=2` and a V8 heap of 512 MB.
    *   `OOMScoreAdjust=500`, so Docker and ai-claude-cli survive global memory pressure.
*   **Operability.**
    *   JSON logs in journald, `/health` with cgroup memory numbers, and `systemctl --user` controls.
    *   Rollback means checking out the previous commit and running `install.sh`.

## Detailed Implementation Plan

### Target Files to Create/Modify

*   **Created:**
    *   `playwright-executor/`: `mise.toml`, `package.json`, `package-lock.json`, `tsconfig.json`, `.env.example`, `.gitignore`, `src/**`, `test/unit.test.ts`, `README.md`, `CLAUDE.md`
    *   `deploy/playwright-executor/`: `install.sh`, `playwright-executor.service.tmpl`, `README.md`
    *   `ai-backend/internal/services/executor/client.go` and `client_test.go`
    *   `ai-backend/internal/api/jobs_executor.go` and `jobs_executor_test.go`
*   **Modified:**
    *   `ai-backend/internal/api/router.go` (executor fields, `enqueueJob` dispatch, `/health.executor`)
    *   `ai-backend/internal/config/config.go`, `ai-backend/cmd/server/main.go`
    *   `ai-backend/.env.example`, `deploy/his-v3-qa/.env.prod.example`
    *   Docs: `README.md`, `CLAUDE.md`, `OBJECTIVES.md`, `docs/designs/SITEMAP.md`, `deploy/his-v3-qa/README.md`
    *   Package READMEs: `ai-backend/internal/api/README.md`, `ai-backend/internal/config/README.md`, `ai-backend/internal/services/ai/README.md`
    *   Deprecation notices in `playwright-qa/{README,CLAUDE}.md` and `playwright-docs/{README,CLAUDE}.md`
    *   `.claude/rules/deploy-edge.md` and `.claude/skills/{qa-automation-elephant,deploy-his-v3-qa,ai-manager-gateway}/SKILL.md`

### Verification Plan

1.  **Unit tests.**
    *   `cd playwright-executor && mise exec -- npm test`: auth, validation, queue order, limits and timeout, HTTP API.
    *   `cd ai-backend && go test -mod=mod ./...`: executor client, plus job dispatch end to end against a fake executor.
2.  **rnd-v3 limits.**
    *   `systemctl --user show playwright-executor -p MemoryMax -p MemoryHigh -p CPUQuotaPerSecUSec -p TasksMax` gives `2684354560 / 2415919104 / 2.400000s / 512`.
    *   The cgroup's `cpu.max` reads `240000 100000`.
    *   `ss` shows it listening on `10.10.0.9:8141` only.
3.  **Real runs against staging.**
    *   `qa_run` and `docs_generate` both reach `completed`.
    *   Back-to-back submissions queue (concurrency 1).
    *   `MemoryPeak` stays below `MemoryMax`.
4.  **Backend.**
    *   After `deploy.sh`, `/health` shows `"executor":"ok"`.
    *   `POST /api/jobs/qa/run` with a session completes, with real `summary` and `issues_recorded`.
