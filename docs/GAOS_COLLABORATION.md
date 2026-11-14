# How the QA tools and GAOS work together

GAOS is the platform that plans and tracks work (a kanban board per product, a design gate, agents). The QA tools hold a
product's test catalog and run its scripts. For a product flagged **integrated with GAOS** the two exchange the product's
test cases in both directions. This page is the operator's and developer's guide from the QA tools' side. The same story
from GAOS's side is `product/qa-collaboration.html` on the GAOS docs site (`https://gaos.mfardiansyah.id/docs/`, behind its
login); the exact request shapes of GAOS's API are on its page *The QA sync API*.

Specs in this repository: [GAOS_INTEGRATION.md](designs/GAOS_INTEGRATION.md) (QA tools to GAOS) and
[GAOS_INBOUND.md](designs/GAOS_INBOUND.md) (GAOS to QA tools). Code: `ai-backend/internal/services/gaossync/` (the push),
`ai-backend/internal/api/gaos_inbound.go` (the two endpoints), `ai-backend/internal/api/products_gaos.go` (the product
settings and the manual sync), `dashboard/src/views/products/GaosCard.tsx` (the card on a product's page).

## Who owns what

Every test case has two halves and each half has one writer, so the two flows never write the same field.

| Half | Fields | Owner |
|---|---|---|
| Planning | case id, epic, module, features, user story, scenario, polarity, level, priority, **decision** (planned, blocked, manual-only, wont) and reason | **GAOS**, once the case exists. The QA tools may *create* a case there, never change one. |
| Automation | `implemented`, `spec` (script path), and, when they exist, the last result, its time and a run reference | **The QA tools.** `implemented` and `spec` come from the synced catalog; `result`, `runAt` and `runRef` from the stored Playwright runs (`kb_case_results`, CASE_RESULTS.md). |

In the QA tools the synced test cases (`kb_test_cases`, read from the automation repository) stay **read-only**. What GAOS
decides is kept apart, in the table `gaos_decisions`; nothing GAOS sends ever writes a `kb_*` row.

## Direction 1: QA tools to GAOS (automatic)

1. A test author changes the product's automation repository (a new case row, a new script).
2. The knowledge sync reads the repository (scheduled, or started by hand) and updates `kb_test_cases`.
3. If the product is flagged integrated and that sync **succeeded**, a push to GAOS starts in the background. It never
   blocks or fails the knowledge sync. A scheduled sync that was skipped because the commit did not change re-pushes only
   when the last push was not `ok`.
4. The push sends to the GAOS product named in the flag (`gaos_product_id`, for SUT `p-6`), sequentially, 200 cases per
   request:
   * `POST {GAOS_API_URL}/products/{id}/qa/cases`: create-only. A new case is created `planned` in GAOS, with
     `implemented` and `spec`. A case that exists in GAOS with different planning fields comes back `exists-differs` and
     is **not changed** (informational, not a failure). A case in an epic GAOS has not seen creates the epic and one new
     **Blocked** card on the product's board (the design gate holds it until an operator releases it).
   * `POST {GAOS_API_URL}/products/{id}/qa/results`: the script facts (`implemented`, `spec`) of every case the push sent,
     and, for a case with a stored run, `result` (`pass`, `fail`, `skipped`), `runAt` (unix **seconds**, an integer) and
     `runRef` (only when printable ASCII and at most 120 bytes; otherwise left out). A case with no stored run carries no
     run fields. GAOS answers `stale` for a run older than the one it holds: that is counted (`results_stale`, and the
     note says so) and is **not** a failure. Any other refusal (`future`, `bad-result`, `bad-run-ref`, `run-incomplete`,
     `unknown-case`...) makes the push `partial`.
   * **An upload starts a push.** When a results upload (the CI route or the dashboard card) stores at least one result and
     the product is flagged, the same push starts in the background, coalesced with any push already running. An upload that
     stored nothing (the same run sent again) starts none, and an upload never waits for or fails on the push. So a CI run
     reaches GAOS without anyone pressing "Pull".
5. **What is sent:** cases whose status is `planned` or `automated` with a code like `TC-AUTH-001`. **What is skipped
   and counted:** `blocked`, `manual-only` and `wont` cases (the QA side may not park or retire a case in GAOS) and local
   drafts (codes starting `L-`).
6. GAOS answers one write at a time per token. A `429` is waited out (the `Retry-After`, at most 10 seconds) and retried up
   to three times. `401`, `403`, `404`, `503` and any other failure stop the push and are recorded as `failed`.
7. The outcome (`ok`, `partial`, `failed`, plus a note such as *sent 1146 cases (created 0, unchanged 1067,
   exists-differs 79), skipped 554*) is stored on the product and shown in its GAOS card. Notes never contain a URL, a token
   or a response body.

A push also runs when the flag is switched on, and on demand: `POST /api/products/{id}/gaos/sync` (Admin or DevOps), the
**Sync to GAOS now** button. It waits up to 280 seconds and answers `200` with the result, or `202` if the push is still
running; a second request while one runs answers `409`.

## Direction 2: GAOS to QA tools (two endpoints)

GAOS's **Pull from QA tools** and **Push to QA tools** buttons call two routes on this application's host root
(`GAOS_QA_URL`, for SUT `http://10.8.0.9:8140`; the dashboard's nginx already forwards `/api`). Both need
`Authorization: Bearer <GAOS_INBOUND_TOKEN>` and no cookie. `401` wrong or missing token, `503` token not configured
(closed), `405` wrong method, `404` unknown product or one that is not flagged with that GAOS id.

### `GET /api/v1/gaos/results?product=<gaos id>&after=<cursor>&limit=<n>`

GAOS pulls script facts. Answer: `{"product":"p-6","items":[{"caseId","implemented","spec?"}],"next":"<unix>:<id>","more":bool}`.

* `limit` is capped at 200; a bad `limit` or `after` is `400`. Only active synced cases whose code matches
  `^TC-[A-Z]+-[0-9]{3}$`.
* `implemented:true` plus `spec` only when the case is automated and the path is usable (relative, ASCII, at most 240
  bytes); otherwise `implemented:false` and no `spec`. A case removed from the repository is a **tombstone**: it is sent as
  `implemented:false` with no script (GAOS clears the script fact; the case itself stays there).
* `result`, `runAt` (unix seconds) and `runRef` are added for a case with a stored run (CASE_RESULTS.md). This pull remains the way GAOS fetches on its own schedule; the push above delivers the same facts without waiting for it.
* The cursor `next` is `<unix>:<id>` over the case row's `(updated_at, id)`, always present (`0:0` at the start), so a case
  changed by a knowledge sync is sent again. Rows younger than 60 seconds are held back so a sync in progress is not
  skipped. An empty `after` starts over and heals anything missed.

### `POST /api/v1/gaos/catalog`

GAOS pushes its planning data (decisions). Body: `{"product":{"id","name","repo"},"cases":[...],"catHead":n}`, at most 200
cases and 1 MiB (more is `400`). Unknown JSON fields are tolerated; the automation fields in the body (`status`,
`implemented`, `spec`, `result`, `runAt`, `runRef`, `scenario`) are **ignored**. Each valid case is upserted into
`gaos_decisions` (key: product and case id). A newer `catRev` wins; an older push leaves the stored row alone. Answer
`200`: `{"accepted":n,"rejected":[{"caseId","reason"}]}` with reasons `bad-case-id`, `bad-decision`, `bad-reason`,
`bad-features`, `bad-field`, `duplicate-case`, `unknown-case`.

### Seeing what GAOS decided

`GET /api/products/{id}/gaos/decisions?diff=0|1&page=&page_size=` (any signed-in user, read-only) lists the rows of
`gaos_decisions` and compares each with what this side would send for the same case (fields `epic`, `module`, `features`,
`user_story`, `polarity`, `level`, `pri`). The product's GAOS card shows it as **GAOS decisions**; filtering to the
differing ones is how the `exists-differs` cases are found.

## Turning it on for a product

1. **GAOS side first** (the operator): the product exists in GAOS and its id is in `GAOS_API_QA_PRODUCTS`; a QA token file
   holds the value you will use as `GAOS_API_TOKEN`; a separate outbound token file holds the value you will use as
   `GAOS_INBOUND_TOKEN`. The three GAOS tokens must differ.
2. **Here:** run the migration (`cmd/databasemigration`, see the `qa-db-ops` skill) before the new image starts. Set in
   `deploy/qa-tool/.env.prod` on app-host (never print them):

   | Variable | Meaning |
   |---|---|
   | `GAOS_API_URL` | GAOS API base, `https://...` or `http://` to loopback or `10.8.0.0/24` only; no credentials, query or fragment. SUT on app-host: `http://10.8.0.9:18301/api/v1` |
   | `GAOS_API_TOKEN` | the QA bearer GAOS issued (`gaos_...`, at least 37 characters) |
   | `GAOS_INBOUND_TOKEN` | the bearer GAOS must present to this side (`gaos_...`); must differ from `GAOS_API_TOKEN`, else the routes stay disabled |

   Without `GAOS_API_URL` and `GAOS_API_TOKEN` the flag cannot be turned on (`400`) and nothing is ever sent.
3. Deploy (`deploy-qa-tool` skill).
4. In `/products`, open the product: in its **GAOS integration** card turn the switch on and enter the GAOS product id
   (Admin or DevOps). The first full push starts at once. The product list shows a **GAOS** badge.

## Checking it

* The product's GAOS card: last push time, status (`ok`, `partial`, `failed`) and note. A healthy push says `rejected 0`.
* `/health` is unchanged; a failed push never affects it.
* From GAOS: the product's **QA** tab (totals and the two last-sync lines).

## When it does not work

| Symptom | Cause | Action |
|---|---|---|
| The switch is disabled, "not configured" | `GAOS_API_URL` or `GAOS_API_TOKEN` is empty or invalid | Set both in `.env.prod`, deploy |
| Push `failed`, "HTTP 401" or "403" | wrong or wrongly scoped `GAOS_API_TOKEN` | Use the GAOS QA token (`prod.api-qa`), not the admin or outbound one |
| Push `failed`, "HTTP 404" | the GAOS id is wrong, or the product is not in GAOS's `GAOS_API_QA_PRODUCTS` | Fix the id, or ask the GAOS operator to list it |
| Push `failed`, "HTTP 503" | GAOS has no token configured | GAOS operator |
| "answered 200 with an unreadable body" | the two sides disagree on a response shape | Compare the live response with GAOS's *The QA sync API*; this happened once with `cardsDeferred` (a string in GAOS) |
| `partial`, a rejected sample | GAOS refused cases for a reason other than `exists-differs` | Read the codes in the sample (`bad-spec`, `epic-limit`, ...) |
| `exists-differs` on many cases | GAOS holds the case with different planning fields (GAOS wins) | Informational; use the GAOS decisions view to see which fields |
| GAOS's buttons fail with 401 / 503 here | `GAOS_INBOUND_TOKEN` missing or different from GAOS's `prod.api-qa-out` | Make them the same value |

## Security in short

Bearer tokens only on the two routes; constant-time comparison; a missing token closes them. No token appears in a log, a
note or a response. Everything received is bounded (200 items, 1 MiB) and checked; text from GAOS is never written into
`kb_*`. The URL to GAOS is configuration, never request input, and redirects are refused. Pushes for two products never
overlap (GAOS allows one write at a time per token).

## Not built yet

A "withdraw a
case" call (a case is retired in GAOS with decision `wont`). Converging the `exists-differs` cases is a follow-up that starts
from the GAOS decisions view.

*Status on 2026-11-14:* the push and the two endpoints are deployed; tombstones in `results`, the decisions view and the
module-derivation fix are merged to `main` and go live with the next deploy.
