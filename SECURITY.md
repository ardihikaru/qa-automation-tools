# Security policy

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately through
[GitHub Security Advisories](https://github.com/ardihikaru/qa-automation-tools/security/advisories/new).
Include the affected component, a description, steps to reproduce, and the impact you expect.

You should get an acknowledgement within 7 days. Fixes are released on `main`. Advisories are
published once a fix is available.

## Supported versions

Only the latest `main` is supported.

## Hardening notes for operators

- Authentication is enabled only while the database is connected. If `/health` reports
  `"database": "disconnected"`, the API is open: treat that as an incident.
- Never publish container or executor ports on `0.0.0.0`. Bind to a private or WireGuard address
  and terminate TLS at an edge proxy.
- Keep `.env*`, `config.yaml` and the executor credential files out of git. The repo `.gitignore`
  already covers them.
- The Playwright executor must only target staging environments. Restrict it with
  `EXECUTOR_ALLOWED_HOSTS`.
