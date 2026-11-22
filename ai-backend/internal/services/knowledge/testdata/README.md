# testdata

`sample-catalog/` is a small **synthetic** `test-catalog-v1` catalog for a fictional web app, "Acme Clinic". It is hand-written; no real product, company or person is described. Only `example.com` / `example.test` domains appear in it.

Layout (the same as a real automation repo's `docs/`):

- `docs/modules/{A,B,C}-*.md`: one file per part. `### <code> — <name>` headings open modules; 12-column feature rows hold the ID (with an `<a id>` anchor), sub-module, name, repo, release status, status-as-of, evidence and notes. `README.md`, `traceability.md` and `unmapped-stories.md` are there for realism and are not parsed.
- `docs/test-catalog/<service>/[<area>/]<epic>.md`: one epic per file, with a `## User stories` list and a 10-column test-case table. Services are `clinic-backend/` and `vendor-api/`; `vendor-api/partners/claims.md` is the nested area. `README.md` and `_template.md` contain story/case lines on purpose: the parser must skip them.

What it covers: 2- and 3-digit feature ids and a `.NN` sub-feature (`B01.003.01`), every release status, an as-of of `—`, rows without a sub-module, sub-module names that differ only in case (`Patient Master` / `patient master` share one code), stories linking one or several features, two unmapped stories, every case status, automated cases with spec paths (plain and inside a link), and reasons of several categories.

The fixture is valid: `TestSUTGolden` expects 0 problems and these numbers:

| parts | modules | features | sub-modules | epics | stories | unmapped | features with stories | test cases | automated |
|---|---|---|---|---|---|---|---|---|---|
| 3 | 7 | 26 | 14 | 5 | 17 | 2 | 13 | 31 | 14 |

`TestKnowledgeAPI` (`internal/api/knowledge_test.go`, needs `QA_TEST_MYSQL_DSN`) uses the same fixture and also expects module `A01` to have 8 features, feature `A01.07` to have 3 stories, and 13 features with no test case (`coverage=untested`). It looks up `US-APPT-05`, `TC-APPT-001` and epic `APPT`.

When you change the fixture:

1. Keep it valid (`go test -mod=mod -run TestSUTGolden ./internal/services/knowledge/` must report 0 problems) and keep it fictional.
2. Update the counts and the specific codes asserted in `TestSUTGolden` (`testcatalog_test.go`, including `goldenSubmodules`), in `TestKnowledgeAPI`, and in this table.
3. `TestGoldenHashes` (`export_checklist_test.go`) pins the checklist export of this fixture, so it fails too. It prints the new hashes; copy them in. A fixture-only change does not need a `TemplateVersion` bump.
