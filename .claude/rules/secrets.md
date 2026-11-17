---
paths:
  - "**/.env*"
  - "**/config.yaml"
  - "**/credentials.yaml"
  - "_data/**"
  - "deploy/**"
---

# Secrets handling

- Never `cat`, `Read`, `grep` (with values), echo, log, commit, or paste into chat the contents of `.env*`, `config.yaml`, `_data/**`, `~/.config/ai-stack/basic-auth.json`, `~/.config/qa-automation/*` (including `ai-service.env`, the AI service account credentials), `~/.config/minio/*` (the staging MinIO credentials file of HISv3; `MINIO_ACCESS_KEY`/`MINIO_SECRET_KEY` are secrets too), `deploy/his-v3-qa/.env.prod`, or the rnd-v2 `infra-prod/.env`.
- To use a secret, load it into an env var or pipe it over stdin (`MYSQL_PWD=…`, `ssh … 'read -r P; …'`). Print only key **names**, counts or file modes (`grep -c '^KEY=.\+'`, `ls -l`).
- Generated secrets go to the session scratchpad with `umask 077` and are deleted after use.
- `*.example` / `*.template` files hold names and safe defaults only. Never put real values in them.
- If a secret leaks into output or a commit, say so right away and propose rotating it (`qa-db-ops` shows how to rotate the DB password).
