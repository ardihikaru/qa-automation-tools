# GAOS inbound: the two endpoints GAOS calls on the QA tools

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Rev 1 (2026-11-14). Owner pre-authorised implementation, merge, migration and deploy. Not yet read by zero-context reviewers.
*   **Depends on:** [GAOS_INTEGRATION.md](GAOS_INTEGRATION.md) (outbound push), [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (`kb_*`).
*   **Code:** `ai-backend/internal/api/gaos_inbound.go`, `router.go:ServeHTTP`, `auth/middleware.go:IsAuthAllowList`.

## Overview
GAOS's QA tab has **Pull from QA tools** and **Push to QA tools** buttons. They call two routes on the QA tools host root (`GAOS_QA_URL`, e.g. `http://10.8.0.9:8140`, nginx already forwards all of `/api/`):

*   `GET /api/v1/gaos/results?product=<gaos id>&after=<cursor>&limit=<n>`: GAOS pulls automation facts.
*   `POST /api/v1/gaos/catalog`: GAOS pushes its planning decisions.

## Decisions
1.  **Auth: one static bearer, not the cookie.** `GAOS_INBOUND_TOKEN` (env; `gaos_` prefix, at least 37 chars, must differ from `GAOS_API_TOKEN`, else it is refused at start and the routes stay disabled). Compared in constant time (SHA-256 of both, `subtle.ConstantTimeCompare`). Unset: 503 (fail-closed). Wrong or missing: 401. The two exact paths bypass the cookie middleware in `ServeHTTP` and are added to `IsAuthAllowList`; **this is the explicit decision to widen the allow-list**, limited to these two exact paths, and the handler is the guard. (`IsAuthAllowList` is documentation only; the real bypass is `Router.ServeHTTP`.) Handlers use no session user.
2.  **Scoping.** `product` (or `product.id` in the catalog body) must be a valid GAOS id (`^p-[0-9]+$`) of a product with `gaos_integrated=1` and a matching `gaos_product_id`; anything else is 404. Bounds: 200 items, 1 MiB request body.
3.  **`results` returns script facts only.** The QA tools hold no per-case run records (see GAOS_INTEGRATION.md Non-Goals), so `result`, `runAt`, `runRef` are never sent. Item: `{caseId, implemented, spec?}`. `implemented` is true only when `kb_test_cases.automated` and the first `spec_paths` entry is a usable path (relative, printable ASCII, at most 240 bytes, no `..`, no `://`); then `spec` is set. Otherwise `implemented:false`. Only active (`removed_at IS NULL`), synced (`origin='sync'`) cases whose code matches `^TC-[A-Z]+-[0-9]{3}$`.
4.  **Cursor `"<unix>:<id>"`** over `(updated_at, id)` of `kb_test_cases`, ordered ascending; empty `after` starts at the beginning. `next` is always present (the position of the last row scanned; `after` echoed when nothing new; `0:0` initially). `more` is true when another page exists. A row edited by a sync gets a new `updated_at`, so it is re-sent. A bad cursor or limit is 400. Rows younger than 60 s are withheld (`gaosCursorLagSeconds`), so a knowledge sync still inside its transaction cannot be skipped by a cursor that moved past its later-committed rows; a sync longer than 60 s, or a same-second tie, can still miss a row until its next change, and a full pull (empty cursor) always heals it. The cursor is at most 128 bytes (a longer `after` is 400) and is always emitted in canonical `<unix>:<id>` form.
5.  **`catalog` stores decisions, never touches `kb_*`.** New table `gaos_decisions(product_id, case_code, epic, module, features, user_story, polarity, level, pri, decision, reason, cat_rev)`, PK `(product_id, case_code)`, upsert, one transaction per batch. The automation fields in the body (`status`, `implemented`, `spec`, `result`, `runAt`, `runRef`, `scenario`) are ignored. Unknown JSON fields are tolerated so that a GAOS addition does not stop a sync. A newer `cat_rev` wins: a push with an older `catRev` leaves the stored decision unchanged. Per-case refusals (HTTP 200, listed in `rejected`): `duplicate-case` (same id twice in a batch), `bad-case-id`, `bad-decision`, `bad-reason`, `bad-features`, `bad-field`, `unknown-case` (not an active case of the product). Answer `{"accepted":n,"rejected":[{caseId,reason}]}`; a batch above 200 cases is 400.
6.  **Migration.** One `CREATE TABLE IF NOT EXISTS gaos_decisions` appended to `migrations.go`; unguarded, so `TestGuardedMigrationsAreKnown` is unchanged.

## Non-goals (follow-ups)
*   Removals: a case removed from the catalog is not sent as a tombstone; GAOS keeps its last `implemented`/`spec` (same as the missing withdraw call).
*   Real per-case results (needs a run to case-id mapping).
*   Showing `gaos_decisions` in the dashboard, and converging the 79 `exists-differs` cases (the stored `module`/`features` make the diff queryable).
*   A "withdraw a case" call.

## Rollout
Migrate (`cmd/databasemigration`), set `GAOS_INBOUND_TOKEN` in app-host `.env.prod`, deploy; then on the GAOS side put the same token in `GAOS_QA_TOKEN_FILE` (`deploy/stacks/prod.api-qa-out`) and restart GAOS with `GAOS_QA_URL=http://10.8.0.9:8140`.
