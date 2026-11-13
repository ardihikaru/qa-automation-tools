# AI access to the qa-tool app (https://qa-tool.sismedika.com)

- AI agents use **only** the seeded service account `ai-service` (role `Service`) through the `qa-tool-session` skill and its `qa-tool.sh` helper, or the `qa-tool-operator` agent. Never use, ask for, or reset a human's password. To see the app as someone else, **impersonate** them.
- The service account can impersonate any active user, Admins included, with their full rights. **Writes (any non-GET) need an explicit request from the user** for that specific action. Every impersonated write is logged and audited server-side.
- Always end the session: `stop`, then `logout` (use a `trap`). One impersonation at a time.
- `~/.config/qa-automation/ai-service.env` and `AI_SERVICE_PASSWORD` in rnd-v3 `.env.prod` are secrets. Never `cat`, `Read`, grep with values, echo, or paste them. Load them into env only (`.claude/rules/secrets.md`).
- If `/health` shows `database` not `connected`, auth is off. Stop and report it as an incident.
