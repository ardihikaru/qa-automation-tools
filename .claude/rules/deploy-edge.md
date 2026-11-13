---
paths:
  - "deploy/**"
  - "docker-compose*.yml"
  - "**/Dockerfile"
---

# Deploy & shared-edge rules

- Deploying, touching clino nginx, or changing rnd-v2 MySQL users is outward-facing. Do it only when the user asks in this conversation.
- clino nginx is shared by many products. Back up the live file, install the new one, run `sudo nginx -t`, and reload only if that succeeds; otherwise restore the backup. Prefix `map` variables with `$hisv3qa_`. Never set `default_server`.
- Keep `deploy/his-v3-qa/nginx-edge.conf` byte-identical to `clino:/etc/nginx/conf.d/his-v3-qa.conf`.
- rnd-v3 ports bind to `10.10.0.9` only (Docker bypasses ufw). This applies to host services too: playwright-executor listens on `10.10.0.9:8141`, and `install.sh` refuses `0.0.0.0`.
- playwright-executor is a systemd **user** unit (`systemctl --user`, user `sismedika`, linger on), not part of the compose stack. Its budget (`MemoryMax=2560M`, `CPUQuota = nproc × 60 %`) is fixed in `deploy/playwright-executor/playwright-executor.service.tmpl`. Raising it needs the user's approval. Re-run `install.sh` after any change to `playwright-executor/`.
- Executor runs (900 s) must finish within the backend's wait (`PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES`, 30), which also covers queueing behind one other run.
- Timeouts along the AI path must be strictly ordered: ai-manager single-provider worst case (~150s) < client `AI_TIMEOUT_SECONDS` (180) < dashboard nginx `proxy_read_timeout` (300) < edge (330). Never raise `AI_TIMEOUT_SECONDS` above 290. If ai-manager falls back between providers, the call may exceed 180 s; if so, the backend returns an error and the categorisation code uses its heuristic.
- Follow the `deploy-qa-tool` skill for every deploy and rollback.
