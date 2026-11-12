---
paths:
  - "deploy/**"
  - "docker-compose*.yml"
  - "**/Dockerfile"
---

# Deploy & shared-edge rules

- Deploying, touching edge-host nginx, or changing db-host MySQL users is outward-facing. Do it only when the user asks in this conversation.
- edge-host nginx is shared by many products. Back up the live file, install the new one, run `sudo nginx -t`, and reload only if that succeeds; otherwise restore the backup. Prefix `map` variables with `$qatool_`. Never set `default_server`.
- Keep `deploy/qa-tool/nginx-edge.conf` byte-identical to `edge-host:/etc/nginx/conf.d/qa-tool.conf`.
- app-host ports bind to `10.8.0.9` only (Docker bypasses ufw).
- Timeouts along the AI path must be strictly ordered: ai-manager single-provider worst case (~150s) < client `AI_TIMEOUT_SECONDS` (180) < dashboard nginx `proxy_read_timeout` (300) < edge (330). Never raise `AI_TIMEOUT_SECONDS` above 290. If ai-manager falls back between providers, the call may exceed 180 s; if so, the backend returns an error and the categorisation code uses its heuristic.
- Follow the `deploy-qa-tool` skill for every deploy and rollback.
