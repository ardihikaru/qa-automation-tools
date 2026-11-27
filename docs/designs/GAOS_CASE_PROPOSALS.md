# GAOS_CASE_PROPOSALS: GAOS proposes the test cases a merged bundle of pull requests needs

Status: implemented on branch `feat/gaos-case-proposals`. Builds on [GAOS_INBOUND.md](GAOS_INBOUND.md) (the guard) and [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (local drafts). The GAOS side is GAOS's design `gaos-external-tasks-and-pr-bundles.md` §7.5 and §7.7 (D-100).

## 1. Goal

When every pull request of a bundle has merged (one task-manager code across repositories), GAOS's test planner reads the pull requests' "what QA must test" notes and proposes test cases. This route takes those proposals. A proposed new case becomes a **local draft** a QA person reviews and promotes into the automation repository, as any draft. An existing case is tied to the bundle's pull request URLs. Nothing is written to the automation repository and no job is started.

## 2. Contract

Guard: the GAOS inbound guard (bearer `GAOS_INBOUND_TOKEN`, constant-time, `503` when unset, `401` wrong or missing, `405` wrong method, `503` no database).

### `POST /api/v1/gaos/case-proposals`

```json
{"product":"p-6","bundleKey":"taskmgr:ABC-12","taskCode":"ABC-12",
 "prUrls":["https://github.com/acme/backend/pull/42","https://github.com/acme/web/pull/7"],
 "cases":[{"action":"create","epic":"OUTREG","title":"…","scenario":"…","polarity":"+","level":"E2E","priority":"P2"},
          {"action":"link","caseCode":"TC-OUTREG-004","note":"…"}]}
```

| Field | Rule |
|---|---|
| `product` | a GAOS product id of a product flagged `gaos_integrated` (`gaosProductLocalID`); otherwise `404` |
| `bundleKey` | `taskmgr:<code>` or `pr:<owner>/<repo>#<n>` |
| `taskCode` | optional, `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$` |
| `prUrls` | 1 to 20 `https://github.com/<owner>/<repo>/pull/<n>` |
| `cases` | 1 to 20 items |

The body is strictly decoded (unknown fields refused), at most 256 KiB. A malformed envelope is `400` and nothing is written.

`200 {"results":[{"index":0,"outcome":"created","caseCode":"L-G3F9A01C2-0"},{"index":1,"outcome":"linked","caseCode":"TC-OUTREG-004"},{"index":2,"outcome":"refused","reason":"…"}]}`

* **`create`**: the local draft `L-G` + the first 8 hex (upper case) of `sha256(product + "|" + bundleKey)` + `-` + index, through `Syncer.SaveDraft`: status `planned`, the named epic (it must be an active epic), the scenario `title` + blank line + `scenario`, `reason_detail` "Proposed by GAOS for <bundleKey> (task <code>): <pr urls>" (capped 1000). The first call answers `created`; a call with the same code updates the draft and answers `updated`. The code is positional, so a retried plan overwrites its drafts. Title 1–200 characters, scenario at most 2000; the enums are the knowledge service's own.
* **`link`**: one `kb_case_pr_links` row per pull request URL for a live case (`INSERT IGNORE` on the primary key `(product_id, case_code, pr_url)`); a replay answers `linked` and adds nothing. An unknown or removed case is refused.
* Any other `action`, or an input the knowledge service refuses, refuses that item only (`outcome: refused`, `reason`); the other items are still applied.

`GET /api/products/{id}/knowledge/test-cases/{code}` gains `pr_links: [{pr_url, bundle_key, note, created_at}]`.

## 3. Decisions

1. **Drafts, not the repository.** Synced cases are managed by the automation repository and read-only here; the draft path already exists, is reviewed by a person and is promoted by them. GAOS writes nothing a person did not see.
2. **Positional codes.** Retrying GAOS's test planner reruns a model, which rephrases titles; a content hash would add a second set of drafts. A shorter retried plan leaves the higher-index drafts in place (accepted).
3. **Text is stored plain.** The dashboard renders through React, which escapes; no field is rendered as HTML.
4. **No session user.** Drafts are created with `created_by` NULL.

## 4. Target files

`ai-backend/internal/api/gaos_case_proposals.go` (new), `gaos_case_proposals_test.go` (new), `gaos_inbound.go` (allowlist and switch), `knowledge.go` (`pr_links`), `ai-backend/internal/db/migrations.go` (`kb_case_pr_links`), `docs/designs/GAOS_INBOUND.md`, `docs/designs/SITEMAP.md`, `docs/GAOS_COLLABORATION.md`.
