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
5. **What is sent:** cases whose status is `planned` or `automated` with a code like `TC-AUTH-001`. A `blocked` case that carries a defect state (`qa_defect_state`) and a usable spec is sent too, as an implemented case: its spec runs as an expected failure, and GAOS refuses a defect for a case it does not know (`unknown-case`). **What is skipped
   and counted:** `blocked`, `manual-only` and `wont` cases (the QA side may not park or retire a case in GAOS) and local
   drafts (codes starting `L-`).
6. GAOS answers one write at a time per token. A `429` is waited out (the `Retry-After`, at most 10 seconds) and retried up
   to three times. `401`, `403`, `404`, `503` and any other failure stop the push and are recorded as `failed`.
7. The outcome (`ok`, `partial`, `failed`, plus a note such as *sent 1146 cases (created 0, unchanged 1067,
   exists-differs 79), skipped 554*) is stored on the product and shown in its GAOS card. Notes never contain a URL, a token
   or a response body.

**A push that is owed survives a restart and a GAOS outage.** Every automatic trigger (an upload, a catalog sync, switching
the flag on) first records "a push is owed" on the product in the database, and the record is cleared only when a push
ends `ok` or `partial`. If the QA tool restarts in between, or GAOS is down or answers `401`/`5xx` so the push `failed`, a
sweeper (5 seconds after start, then every minute) sends it again, waiting longer after each failure (30 seconds per failed
attempt, at most 10 minutes). Sending again is harmless: GAOS answers `unchanged` and `stale` for what it already holds.
A product that is switched off stops being retried. The record is written before an upload's answer is returned, and even
if the QA tool is killed at the worst instant (after the results are stored and before the record exists), the sweeper
notices on the next start that the product's results or cases are newer than its last push and sends it.

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

## Direction 3: GAOS starts an automation (two more endpoints)

GAOS can now ask the QA tools to write an automation for a case, or to run the one the case already has, and to watch
the job. The two routes sit in the same family as the endpoints above and use the **same** `GAOS_INBOUND_TOKEN`: no
cookie, no session, `401` a wrong or missing token, `503` token not configured (closed), `405` a wrong method.

### `POST /api/v1/gaos/automation`

Body: `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"create"}`. `mode` is `create` (generate and write a new
automation for the case) or `run` (run the spec the case already has). Answer: `202 {"job_id":"job-..."}`.

* `product` is the GAOS product id and must name a product this side has flagged as integrated (`gaos_integrated` and
  `gaos_product_id`); an unknown id, and an id of a product that is not flagged, are both `404 unknown product`.
* `caseCode` is the case of that product; an unknown (or deleted) case is `404 unknown case`.
* A bad body, and a mode that is neither `create` nor `run`, are `422`.
* The job is the ordinary case automation job (`case_create` or `case_run`): it is on the job page, its log is the
  usual one, and a `create` still waits for a human to approve the spec before the pull request is opened. **GAOS does
  not get the approval gate.**
* A job that is already active for that case is not started twice: the call answers `200 {"job_id"}` with the active
  job's id. A case that already has a spec (`create`), a case that has none (`run`), and a product without an
  automation repository are `409`.
* `caseCode` `"*"` with `mode` `run` runs every automated case of the product (one `case_run` per case, one after
  the other, through the same path) as one job of type `gaos_run_all`; `202 {"job_id"}`, or `200` with the active
  run-all of that product, or `409` when the product has no automated case. `"*"` with `create` is `422`. A restart
  marks a run-all job `failed` (it is not resumed). Its `report` is the sum of the cases' counts; a case that cannot be
  started counts as `skipped`.
* An instance that cannot run case automation (no playwright-executor, no repository pool, no `REPO_POOL_HOST_DIR`, or
  no AI for `create`) answers `503`; nothing is started.

### `GET /api/v1/gaos/jobs/{id}`

The state of one job: `{"job_id","status","phase","job_type","error","report":{"passed","failed","skipped"},"pr_url"?,"job_url"?}`.

* `status` is `queued`, `running`, `awaiting_approval` (a running `create` whose spec waits for a human; the answer adds `waiting_s`), `succeeded` or `failed` (the job page's `completed` is `succeeded` here; failing
  tests are in `report`, not in `status`); `phase` is the step the job is in (`prepare`, `generate`, `write`, `run`,
  `repair`, `approval`, `push`, `pr`, `done`; a run-all shows `run <case> (n/total)`); `job_type` is `create` or
  `run`; `error` is empty until the job fails.
* `report` holds the counts of the last run of the spec (`0`, `0`, `0` before the first run).
* `pr_url` is the pull request the job opened (from the job payload while it runs, from its result after); `job_url` is the job's page (`DASHBOARD_ORIGIN` + `/jobs/<id>`; absent when the origin is unset or `*`). Both are absent when unknown. GAOS lists the pull request on its Pull requests page, and accepts `job_url` as a link only on the host it is configured to trust (`GAOS_QA_ORIGIN`).
* An unknown job id is `404`.

The job id is the one the `202` answered. The numbers, the phase and the error are the ones the job page shows, so a
GAOS card and the dashboard never disagree.

### `POST /api/v1/gaos/jobs/{id}/cancel`

GAOS calls it when it fails a card (timeout, flapping cap). A job still queued, running or awaiting approval has its open
approval rejected ("canceled by GAOS"), is stopped and marked `failed`; the answer is
`{"job_id","status":"failed","error":"canceled by GAOS","canceled":true}`. A job that already ended is returned as it is
with `"canceled":false`. An unknown job is `404`. After a cancel no pull request can be opened for that job.

### Defects (QA tools to GAOS)

The Playwright ingest also reads the `defect` annotations of a report (a pinned `test.fail()` that failed as expected is
an `xfail`, one that now passes is an `xpass`, an unexpected failure is a `fail`). The last state of each case is
kept, and **only a change** is pushed to `POST /api/v1/products/{id}/qa/defects` with the other pushes, in batches of at
most 50 items and 240 KiB. Each item carries the case, the defect id, the kind, the level, `automationType` (`be` for
API/GQL, `e2e` for WEB/E2E), `backend` (`graphql` or `rest` from the spec path), `targetRepos` (the spec's service
directory resolved to the product's repositories, plus the mapped backends for an e2e case), up to five `operations`,
the spec, a title and an excerpt of at most 4 KiB with secrets removed. A GAOS without the route (404 or 405) is waited
out for 30 minutes and nothing is lost. The mapping of an app directory to its backend repositories is set with
`PUT /api/products/{id}/service-repos`. See [QA_DEFECT_TRIAGE.md](designs/QA_DEFECT_TRIAGE.md).

## Learned policies (Inbox approvals)

The `/learning` page of the QA tools turns findings from case automation (a reviewer's *Needs changes*, a repair that
fixed a failure, a failure repeated in three jobs, or a note typed by a QA engineer) into a proposed rule file for the
product's automation repo (`.claude/rules/learned-<slug>.md`). For a GAOS-integrated product the approval is also
asked in the GAOS Inbox, exactly like a generated spec: title "Approve learned policy for <product>", the whole policy
text in the context, options **Approve**, **Reject**, **Needs changes** (reply with the change). The external id has the
same `qa-<job id>-pr[N]` shape, so the existing answer webhook handles it; nothing changes on the GAOS side. After
approval the QA tools open a pull request on the automation repo; a person merges it there. See
[LEARNING_LOOP.md](designs/LEARNING_LOOP.md).

## Answering an approval from the QA tools' run history (QA tools to GAOS)

Spec: [HISTORY_ANSWER.md](designs/HISTORY_ANSWER.md). Every approval waiting now (a generated spec of `case_create`, a
learned policy of `policy_propose`) is listed on `/history` under **Waiting for approval**, with an **Answer** button
(roles Admin, DevOps, Developer, QA, QC). The dialog offers the GAOS item's options in the same order: **1 Approve**,
**2 Reject**, **3 Needs changes** (a comment is required). When the item exists in GAOS (`gaos_created`) and the product
is integrated, the answer is **relayed to GAOS first**:

```
POST {GAOS_API_URL}/products/{pid}/inbox/{externalId}/answer      Authorization: Bearer <GAOS_API_TOKEN>
{"choice": 1, "text": "lgtm", "actor": {"username": "ardi", "display": "ai-service as ardi"}}
```

`actor.username` is the signed-in user (the impersonated one when impersonating); `display` is "<real user> as <target>"
when impersonating, else the full name. GAOS works out the client and its domain from the token and logs the source.

| GAOS answers | The QA tools do |
|---|---|
| 200 | the only answer recorded as ours: apply the submitted answer here at once (source `qa-tool:history`, GAOS's `source` object from the 200 stored); the job continues without waiting for the webhook or the poll. GAOS's webhook (it carries no `source`) is a no-op when it comes later; if it lands *before* the 200 is applied, the row is claimed back as `qa-tool:history` with the 200's `source`. |
| 409 `already_answered` | someone answered first in GAOS (`reason` names it): adopt **that** answer here (source `gaos`; a GAOS back-office answer shows as "operator (GAOS)") and tell the user. An own-words answer the gate cannot use falls back to the local answer. |
| 410 `expired` | adopt the expiry: the approval closes as `expired` (source `system`), the user is told. |
| 429 | wait `Retry-After` (at most 5 s) and try once more; then as 5xx. |
| timeout / transport error | read the item once (`GET …/inbox/{externalId}`); `answered` or `expired` is adopted as above, else fall back. |
| 404 (also an older GAOS without the endpoint), 400 `bad-actor`/`bad-json`/`secret-in-body`, 401/403, 422 `bad-choice`/`bad-text`, 5xx | **fall back**: the answer is applied in the QA tools only, with a note in the dialog and a warning in the job log; the GAOS item stays open until it expires or is answered there (ignored). No durable retry. 401/403/422 are logged as configuration errors. |

A username GAOS would refuse (outside `A-Z a-z 0-9 . _ @ + -`, longer than 64, or `operator`) is never sent: the answer is
local, with a note. GAOS validates `actor.display` but does not store it, so the real actor behind an impersonation
("ai-service as ardi") is kept only on the QA side (`answer_actor`).

The QA tools deploy can go **before or after** the GAOS release that adds the endpoint: until then every relay gets 404
and falls back to the local answer. Nothing else sent to GAOS changes.

**Where every answer came from** (`case_approvals.answer_source`, shown as "answered via … by …" on the job page and in
the Builds row of `/history`): `gaos` (webhook, poll, or a 409), `qa-tool:history`, `qa-tool:job-page`,
`qa-tool:learning`, `system` (any expiry). `answer_actor` is who answered; `answer_source_detail` holds GAOS's `source`
object (from the 200/409, the webhook when GAOS sends `source`, or the poll). `answered_by` keeps its old door value.

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

The answer relay sends the signed-in username (and, when impersonating, the real user's name) to GAOS; the comment is
redacted first, as before. Bearer tokens only on the inbound routes (the five GAOS routes and the CI results route);
constant-time comparison; a missing token closes them. No token appears in a log, a
note or a response. Everything received is bounded (200 items, 1 MiB) and checked; text from GAOS is never written into
`kb_*`. The URL to GAOS is configuration, never request input, and redirects are refused. Pushes for two products never
overlap (GAOS allows one write at a time per token).

## Not built yet

A "withdraw a
case" call (a case is retired in GAOS with decision `wont`). Converging the `exists-differs` cases is a follow-up that starts
from the GAOS decisions view.

*Status on 2026-11-14:* the push and the two endpoints are deployed; tombstones in `results`, the decisions view and the
module-derivation fix are merged to `main` and go live with the next deploy.

*Status on 2026-11-25:* the two automation routes (Direction 3) are implemented and go live with the next deploy.
