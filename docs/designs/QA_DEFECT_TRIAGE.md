# QA_DEFECT_TRIAGE: the QA tools tell GAOS which tests fail, and why

Status: implemented (PR "feat: defect triage push"). The receiving side is GAOS's own design (its section 4, "The QA tool to GAOS contract", is authoritative for the body). Builds on [CASE_RESULTS.md](CASE_RESULTS.md) and [GAOS_INTEGRATION.md](GAOS_INTEGRATION.md). All names below are invented (`acme-org/sample-service`, `DEF-101`, `TC-SAMPLE-008`).

## 1. Goal

GAOS can plan, triage and (for a backend case) propose a fix for a defect, but only if it hears of it with enough context. The QA tools already ingest a Playwright JSON report per run; this change reads the **defect** side of that report, remembers the last state of each case, and pushes **changes** to `POST /api/v1/products/{gaosProductId}/qa/defects` through the existing outbox, sweeper and write slot of `gaossync`.

## 2. What a report says

A test is read for a `defect` annotation (any test, not only skipped ones). `defectId` is the first `DEF-n` (1 to 6 digits) in its description; `title` is the description without that id (at most 200 characters).

| Playwright | Annotation | Kind | Notes |
|---|---|---|---|
| `status: expected`, `expectedStatus: failed` | has `DEF-n` | `xfail` | a pinned failure that failed as expected; `excerpt` is the annotation's text |
| `status: unexpected`, `expectedStatus: failed` | has `DEF-n` | `xpass` | the test passed although pinned to fail: the fix landed; `excerpt` is Playwright's "Expected to fail, but passed" |
| `status: unexpected`, `expectedStatus: passed` | any | `fail` | an unexpected failure; `defectId` empty; `title` the spec title; `excerpt` the failure message |
| anything else | | none | including a `test.fail()` with no `DEF-n`: there is nothing to name |

The result values of the results API do not change: an `xfail` is a pass there, a failure a fail, and an `xpass` stays what Playwright calls it (`unexpected`, so fail). The defect view is separate. Several tests of one case give the most serious observation (`fail`, then `xpass`, then `xfail`).

## 3. Derived per case

* `automationType`: `be` for level `API` or `GQL`, `e2e` for `WEB` or `E2E`.
* `backend`: `graphql` when the spec path contains `tests/api/graphql/`, `rest` for `tests/api/rest/`, empty otherwise.
* `targetRepos` (at most 8, `owner/repo`): the service or app directory under the spec path's tests root (`tests/api/graphql/<dir>/`, `tests/api/rest/<dir>/`, `tests/web/<dir>/`), matched by name (case-insensitive) to the product's `product_repos`; the slug is read from the repository URL. An `e2e` case adds the repositories the product-level mapping gives that directory (section 6). A directory that resolves to nothing gives an empty list; GAOS then blocks the fix, it never trusts a name from here.
* `operations` (best effort, at most 5): GraphQL operation names (`query|mutation|subscription Name`) and REST route names (`.get|post|put|patch|delete('/path')`, scheme, host and query dropped, `${...}` becomes `_id`, no leading slash) found in the spec and in the files it imports from the same automation repo (at most 8 files of 256 KiB, read from the repo pool without leaving the repo). Each must match `^[A-Za-z_][A-Za-z0-9_.\-/]{0,80}$`; empty is allowed.

## 4. Decisions

1. **State, not events.** A table `qa_defect_state` (one row per product and case) holds the last observation and the hash of what GAOS must know (case, kind, defect id, title). A push is owed while `hash` differs from `pushed_hash` and `rejected_hash`. The excerpt and the run are not part of the hash, so a changing failure message does not re-push.
2. **Clearing.** A case that passes with no defect annotation loses its row, so its next failure is new. A report older than the stored state is ignored.
3. **One push pass.** The defects go in the same pass as cases and results (`gaossync.pass`), after them, so the case exists in GAOS first. The sweeper's level-triggered safety net also owes a push to a product with pending states (and no active backoff), so a crash between the ingest and the marker loses nothing. An ingest that changes a state fires the same `OnResultsStored` hook as one that stores a result.
4. **Answers.** `200`: items in `rejected` get `rejected_hash = hash` and a reason, and are not sent again until they change; the rest get `pushed_hash`. A rejection `defect-limit` (GAOS's daily cap on new keys) is not a verdict: the items stay pending and the defect push waits an hour. `404` and `405` (an older GAOS without the route): not a failure of the product, nothing marked, the defect push waits 30 minutes (`qa_defect_push.backoff_until`), cases and results still go. `413`: the batch is halved; a single item that is still too large is rejected locally (`too-large`). Any other failure fails the pass like the cases do, and the outbox retries it.
5. **Limits.** At most 50 items and about 240 KiB per request (GAOS reads 256 KiB); a larger set is split into several requests. Only products with the GAOS link on are pushed. `be` and `e2e` items are both sent; GAOS decides what it fixes.
6. **Untrusted text.** `title` and `excerpt` leave this server only after a scrub (private keys, JWTs, AWS-style keys, bearer or basic tokens, `password=`/`token=` pairs, GitHub and GAOS tokens) and the case automation redactor (known secrets, tokens, URL credentials); the excerpt is cut to 4 KiB on a character boundary. The scrub runs when the state is stored, so a secret is never in the table.
7. **A state the push cannot describe** (no usable spec path, an unknown level) is not sent; it is marked rejected locally with the reason (`no-usable-spec`, `bad-level`) and logged by case code.
8. **A defect case must exist in GAOS (found live, 2026-11-26).** The team's catalog marks a case that pins a defect `blocked`, and the case push skips blocked cases, so every such defect was rejected `unknown-case` and, by item 4, never re-sent. The case push now also sends a `blocked` case that has a `qa_defect_state` row and a usable spec (as `planned`, implemented, with its spec). Rejected rows are not retried on their own: after the case reaches GAOS, delete the row (or let the case's next observation change its hash) so the defect is pushed. Test: `TestBuildPlanSendsABlockedCaseThatCarriesADefect`.
9. **Operation names follow tsconfig aliases (found live, 2026-11-26).** The walk from a spec to the GraphQL or REST calls it makes followed only relative imports, but the automation repo imports its helpers through `compilerOptions.paths` (`@api/*`, `@fixtures`), so DEF-922 reported no operation, GAOS found no source file in the application repo and its triager had to downgrade a clear `easy` defect to `medium`. The walk now reads the repo's `tsconfig.json` (line comments tolerated) and resolves an alias import to its repo-relative path; a package import still resolves to nothing. Test: `TestOperationsFollowTsconfigAliases`.

## 5. The body

`POST /api/v1/products/p-6/qa/defects`, bearer `GAOS_API_TOKEN`:

```json
{"source":"push","items":[{
  "caseId":"TC-SAMPLE-008","defectId":"DEF-101","kind":"xfail","level":"GQL",
  "automationType":"be","backend":"graphql",
  "targetRepos":["acme-org/sample-service"],"operations":["editSampleRecord"],
  "spec":"tests/api/graphql/sample-service/records/edit.spec.ts",
  "title":"a negative quantity is accepted and stored",
  "excerpt":"DEF-101 a negative quantity is accepted and stored",
  "runRef":"run-1","runAt":1795600800}]}
```

`defectId` is omitted for `fail`; `runRef` is omitted when it is empty or not printable ASCII.

## 6. The service-to-repository mapping

`product_service_repos (product_id, service_dir, repo_name)`, set through `GET|PUT /api/products/{id}/service-repos` (no dashboard page). `PUT {"service_dir":"sample-app","repos":["sample-service","sample-billing"]}` replaces that directory's repositories (an empty list removes it); every name must be a repository of the product, distinct, at most 8; the role must be able to manage products. `GET` lists the mapping.

## 7. Target files

`internal/services/caseresults/` (`parse.go`, `defects.go`, `defects_store.go`), `internal/services/gaossync/` (`defects.go`, `sync.go`, `sweeper.go`), `internal/api/` (`case_results.go`, `case_automation.go`, `service_repos.go`, `products.go`), `internal/db/migrations.go` (`qa_defect_state`, `qa_defect_push`, `product_service_repos`), `cmd/server/main.go`, the READMEs of those packages and `docs/GAOS_COLLABORATION.md`.

## 8. Review

Self-reviewed against the repo checklist: schema appended idempotently, no new env, no real names, the READMEs of the changed directories updated, the closed GAOS vocabularies (`kind`, `level`, `automationType`, `backend`) used as written, every limit of GAOS's section 4.1 enforced on this side too. Assumptions to confirm on the GAOS side: `rejected[].caseId` identifies the item (one state per case), and `defect-limit` is the reason of the daily cap.
