# deploy/playwright-executor

## Overview

This directory installs [playwright-executor](../../playwright-executor/README.md) on **rnd-v3-vpn** (`10.10.0.9`) as a **systemd user service** for the user `sismedika`, following the same pattern as `ai-claude-cli.service`.

It runs on the host rather than in Docker because:
- the kernel cgroup limits apply directly to node and every Chromium child;
- the toolchain comes from mise, like the other rnd-v3 services;
- the Docker images stay small (the backend doesn't need a browser).

| Item | Value |
|---|---|
| Unit | `~/.config/systemd/user/playwright-executor.service`, rendered from `playwright-executor.service.tmpl`. Linger is on, so it starts at boot. |
| Listen | `10.10.0.9:8141` (WireGuard IP only). The ai-backend container reaches it there. |
| Runtime | `mise exec -- node dist/src/server.js` in `~/devel/projects/qa-automation-tools/playwright-executor`. Node comes from that directory's `mise.toml`. |
| Env file | `~/.config/qa-automation/playwright-executor.env` (mode 600) |
| Credentials | `~/.config/qa-automation/hisv3-credentials.json` (mode 600) |
| Artifacts | `~/qa-executor-data/runs/<run_id>/`, kept for 7 days |
| Logs | `journalctl --user -u playwright-executor` |

### Resource budget

rnd-v3 has 4 vCPU and 3.8 GiB RAM, and also runs Docker (qa-automation, gaos) and `ai-claude-cli` (MemoryMax 512M).

| Limit | Value | Effect |
|---|---|---|
| `MemoryMax` | `2560M` | Hard cap (2.5 GiB) on node plus all Chromium processes. The kernel OOM-kills inside the cgroup only. |
| `MemoryHigh` | `2304M` | Reclaim/throttle starts before the hard cap. |
| `MemorySwapMax` | `0` | No swap. |
| `CPUQuota` | `nproc × 60 %` | 240 % on rnd-v3, i.e. 2.4 of 4 CPUs. |
| `TasksMax` | `512` | |
| `OOMScoreAdjust` | `500` | Under global memory pressure, the executor dies before Docker or ai-claude-cli. |
| App level | concurrency 1 | One Chromium at a time, `--renderer-process-limit=2`, V8 heap 512 MB per renderer. |

The `sismedika` user manager delegates the `cpu memory pids` controllers (`/sys/fs/cgroup/user.slice/user-<uid>.slice/user@<uid>.service/cgroup.controllers`), so these limits are enforced even though this is a user unit.

## Interface

### Install / upgrade

The code must already be on rnd-v3 at `~/devel/projects/qa-automation-tools`. It ships with the same git bundle the `deploy-his-v3-qa` skill uses.

```bash
ssh rnd-v3-vpn 'cd ~/devel/projects/qa-automation-tools && deploy/playwright-executor/install.sh'
# later upgrades, apt deps already present:
ssh rnd-v3-vpn 'cd ~/devel/projects/qa-automation-tools && SKIP_DEPS=1 deploy/playwright-executor/install.sh'
```

`install.sh` does the following, and is idempotent:
1. `mise trust` and `mise install`.
2. `npm ci`, build, and prune dev dependencies.
3. `playwright install chromium`, then `sudo ... install-deps chromium` (apt).
4. Creates a placeholder env file if one is missing.
5. Renders the unit with `CPUQuota = nproc × 60 %`.
6. `daemon-reload`, then enable and restart.
7. Polls `/health` for up to 30 s.

It refuses `EXECUTOR_BIND=0.0.0.0`.

### Secrets (one-time, or when rotating)

None of these values are printed. They are generated in a local scratchpad with `umask 077` and piped over ssh stdin:
- **HISv3 staging URL and credentials:** taken from the local `config.yaml` (`staging.url`, `credentials.*`) and written as JSON to `hisv3-credentials.json`.
- **`EXECUTOR_TOKEN`:** `openssl rand -hex 32`. The same value goes into `deploy/his-v3-qa/.env.prod` as `PLAYWRIGHT_EXECUTOR_TOKEN`, together with `PLAYWRIGHT_EXECUTOR_URL=http://10.10.0.9:8141`.

### Operate

```bash
systemctl --user status playwright-executor
systemctl --user restart playwright-executor
journalctl --user -u playwright-executor -f
systemctl --user show playwright-executor -p MemoryMax -p MemoryHigh -p CPUQuotaPerSecUSec -p MemoryPeak -p MemoryCurrent
systemd-cgtop --user   # live CPU and memory per unit
```

**Rollback:** check out the previous commit in the rnd-v3 checkout and re-run `SKIP_DEPS=1 install.sh`. To take the executor out completely, run `systemctl --user disable --now playwright-executor` and clear `PLAYWRIGHT_EXECUTOR_URL` in `.env.prod`. The backend then falls back to simulated jobs.

## Invariants

- Bound to the WireGuard IP only. Docker bypasses ufw, but this is a host process, and it must still never listen on `0.0.0.0`.
- Don't raise the limits in the template without the user's approval. If jobs need more memory, reduce `max_steps` or the number of roles instead.
- The env file and credentials file stay at mode 600 and are never printed or committed.

## Verification

Run on rnd-v3:

```bash
systemctl --user show playwright-executor -p MemoryMax -p MemoryHigh -p CPUQuotaPerSecUSec -p TasksMax
#   MemoryMax=2684354560  MemoryHigh=2415919104  CPUQuotaPerSecUSec=2.400000s  TasksMax=512
cat /sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/app.slice/playwright-executor.service/cpu.max
#   240000 100000
ss -ltnp | grep 8141                     # 10.10.0.9:8141 only
curl -s http://10.10.0.9:8141/health     # status ok, chromium installed
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://10.10.0.9:8141/v1/runs   # 401
```

For an end-to-end check, trigger **Run QA** in the dashboard (or `POST /api/jobs/qa/run` with a session cookie) and poll `/api/jobs/<id>` until it reaches `completed`. The result should contain `executor_run_id`, `summary` and `issues_recorded`. Backend `/health` should report `"executor":"ok"`.
