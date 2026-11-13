---
name: qa-tool-session
description: Log in to the production qa-automation app (https://qa-tool.mfardiansyah.id) as the AI service account and act as any user or role through impersonation. Use when an agent must call the qa-tool API, check what a given user/role sees or may do, smoke-test prod after a deploy, or reproduce a role-specific bug — never with a human's password.
---

# qa-tool session (AI service account + impersonation)

**Spec:** [docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md](../../../docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md). The rule is `.claude/rules/qa-tool-ai-account.md`.

## Account
- **Username:** `ai-service`, role **`Service`**. On its own it has no product rights. It may impersonate **any** active user, Admins included, with that user's full rights (the owner's decision). Treat it as an admin credential.
- **Credentials** live in `~/.config/qa-automation/ai-service.env` on the workstation (mode 600) with the keys `QA_TOOL_URL`, `QA_TOOL_USERNAME`, `QA_TOOL_PASSWORD`. Production has the same values in `.env.prod` on app-host (`AI_SERVICE_USERNAME`, `AI_SERVICE_PASSWORD`).
- **Never** `cat`, `Read`, grep or echo the credential file or the password. The helper loads it with `set -a; . file` and passes the password to curl through stdin only.

## Helper: `.claude/skills/qa-tool-session/qa-tool.sh`
Use a cookie jar in the session scratchpad: `export QA_TOOL_JAR=<scratchpad>/qa-tool.jar`.

```bash
S=.claude/skills/qa-tool-session/qa-tool.sh
$S login                       # {"user":"ai-service","role":"Service","impersonator":null}
$S users                       # username  role  active|inactive
$S as <username>               # start impersonating; now acting as that user
$S call GET /api/products      # any API call: prints status line, then body
$S call POST /api/x '{"k":1}'  # JSON body (Content-Type is always application/json)
$S stop                        # back to ai-service
$S logout                      # always finish with this (also deletes the jar)
$S smoke <qa-username>         # full post-deploy check (below)
```

To act as a **role** rather than a named person, run `$S users`, pick an active user with that role, and run `$S as <them>`.

## Rules of use
1. **Read freely. Write only when the user explicitly asked for that write.** That covers anything other than GET, and above all deleting or creating products or running clone, checkout or jobs. Every write made while impersonating is logged server-side (`impersonated-write actor=… target=…`) and stored in `impersonation_audit`.
2. **Always clean up.** Run `stop`, then `logout`, even after a failure; wrap the work in a `trap '$S logout' EXIT`.
3. **One impersonation per session.** A second `as` while impersonating returns 403. Run `stop` first.
4. **Never change passwords or users.** `change-password` returns 403 while impersonating anyway. Rotating the service password itself is an owner task (see the deploy README).
5. **Browser automation** of this app isn't covered here. Playwright runs only in playwright-executor against SUT staging.

## Post-deploy smoke (`$S smoke <qa-username>`)
The steps and the status each should return:
1. `/health`: 200, with `database: connected`. A disconnected DB means auth is off, which is an incident (CLAUDE.md hard rule 4).
2. Login: 200, and `/api/auth/me` shows role `Service`.
3. `/api/users`: 200.
4. Impersonate the QA user: 200, with `user.impersonator.role` = `Service`.
5. `GET /api/products`: 200.
6. `POST /api/products`: 403, because QA cannot manage products. The request is refused before anything is written.
7. Stop: 200, with role back to `Service`.
8. Logout: 204.

## Errors
| Status | Meaning |
|---|---|
| 401 on login | Wrong credentials, or the service user isn't seeded (`AI_SERVICE_*` unset, the password is shorter than 32 characters, or the username collides with a human account; check the backend log for `seed:`) |
| 401 later | The session expired (12 h) or someone logged it out. Log in again |
| 403 `forbidden` | The real role isn't Admin/Service, or you're already impersonating |
| 400 / 404 / 409 on `as` | Self / unknown user / inactive user |
| 415 | The request isn't `Content-Type: application/json` (the helper always sends it) |
| 400 `not impersonating` on `stop` | Nothing to stop. Harmless |

An impersonation lasts at most 1 hour. When it expires, the session silently becomes `ai-service` again, so check `$S me` if the results look like the wrong role.

## Setup (owner, once)
```bash
umask 077; mkdir -p ~/.config/qa-automation
# write QA_TOOL_URL=https://qa-tool.mfardiansyah.id, QA_TOOL_USERNAME=ai-service and
# QA_TOOL_PASSWORD=<same value as AI_SERVICE_PASSWORD in app-host .env.prod> without echoing it
chmod 600 ~/.config/qa-automation/ai-service.env
```
