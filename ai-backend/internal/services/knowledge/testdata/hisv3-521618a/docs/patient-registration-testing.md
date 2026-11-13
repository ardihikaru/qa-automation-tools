# Patient registration (his-backend + his-frontend)

Test doc of the epic **[Patient registration](test-catalog/his-backend/patient-registration.md)** (catalog code `PATREG`).

The cluster is the module that writes PHI: the registration form of his-frontend, the GraphQL `patient`
and `encounter` modules of his-backend, and the REST patient/encounter routers of the same service. The
stories, the scenarios and the findings come from
`his-v3-ai/services-impl/202611/backend-rest-e2e-qa-assessment/data-integration/D-HISV3-PATIENT-REGISTRATION/`
(`USER-STORIES.md` → `US-D-HISV3-PATIENT-REGISTRATION-01..17`, `USER-SCENARIOS.md` →
`SC-D-HISV3-PATIENT-REGISTRATION-01..08`, `ARCHITECTURE.md` §6 → findings `PR-01..PR-38`).

## What is covered

| Surface                                                           | Folder                                         |
| ----------------------------------------------------------------- | ---------------------------------------------- |
| GraphQL `patient` module — registration, duplicate checks, search | `tests/api/graphql/his-backend/patient/`       |
| GraphQL `encounter` module — check-in, registration number        | `tests/api/graphql/his-backend/encounter/`     |
| REST `/api/v1/patients` and `/api/v1/encounters`                  | `tests/api/rest/his-backend/`                  |
| `/administration/patient-registration`                            | `tests/web/his-frontend/patient-registration/` |

The clients live in `src/api/graphql/his-backend/patient/`, `src/api/graphql/his-backend/encounter/`,
`src/api/rest/his-backend/` and the page object in
`src/web/his-frontend/pages/patient_registration/`. The payloads come from
`src/api/graphql/his-backend/registration.builders.ts`, the DB reads from
`src/api/rest/his-backend/patient-registration.db.ts`.

## The expected failures (the point of this epic)

The findings of the cluster are documented defects, not wishes. Each of them is pinned by a test that
keeps the **desired** behaviour in the assertion and is marked as an expected failure:

```ts
test(
  'TC-PATREG-050 an anonymous caller cannot create a patient',
  {
    tag: [Tag.regression],
    annotation: { type: 'defect', description: 'DEF-803 / PR-03 — patientCreate tidak memeriksa sesi' },
  },
  async ({ hisBackendPatientAnonymous }) => {
    const result = await hisBackendPatientAnonymous.create(payload);

    test.fail(true, 'DEF-803 / PR-03 — mutasi patientCreate menjawab pemanggil anonim');
    expect(result.status).toBe(false);
  },
);
```

Playwright reports such a test as **passed** while the defect exists, and as **"unexpectedly passed"**
once the product is fixed — that is the signal the team needs. Never weaken the assertion to make the
test green: the desired behaviour is what the test states.

The defect ids are `DEF-801` (PR-01) … `DEF-838` (PR-38): the number is `800` + the finding number.
The full mapping is in the catalog file, §"Defect ids".

| Finding | Defect  | Pinned by                                                                                                        |
| ------- | ------- | ---------------------------------------------------------------------------------------------------------------- |
| PR-01   | DEF-801 | TC-PATREG-052 — anonymous `POST /api/v1/patients`                                                                |
| PR-02   | DEF-802 | TC-PATREG-053, TC-PATREG-054 — anonymous patient reads                                                           |
| PR-03   | DEF-803 | TC-PATREG-050 — anonymous `patientCreate`                                                                        |
| PR-04   | DEF-804 | TC-PATREG-023 — `patientCheckIn` panics                                                                          |
| PR-08   | DEF-808 | TC-PATREG-007 — the contract cannot carry the family data                                                        |
| PR-09   | DEF-809 | TC-PATREG-013, TC-PATREG-014, TC-PATREG-058 — duplicate check                                                    |
| PR-12   | DEF-812 | TC-PATREG-032, TC-PATREG-060 — failure reported as success                                                       |
| PR-14   | DEF-814 | TC-PATREG-064 — `regionalCode[:10]` panic                                                                        |
| PR-15   | DEF-815 | TC-PATREG-063 — `Telecom[:2]` panic                                                                              |
| PR-17   | DEF-817 | TC-PATREG-055, TC-PATREG-056 — REST `/encounters` without JWT                                                    |
| PR-20   | DEF-820 | TC-PATREG-040 — `patientMerge` is retired                                                                        |
| PR-27   | DEF-827 | TC-PATREG-066 — `patient.created_ts` is `0`                                                                      |
| PR-30   | DEF-830 | TC-PATREG-045 — `search_index` is not written on create                                                          |
| PR-36   | DEF-836 | TC-PATREG-070 — the QR link is the `abc.com` stub                                                                |
| PR-32   | DEF-832 | **not reproduced** — the deployed build keeps the NIK out of the console, so TC-PATREG-062 asserts the behaviour |

### Findings from the first live run (2026-11-09)

Four defects are **not** in the cluster documentation — they came out of running the suite against staging:

| Defect    | Finding                                                                                    |
| --------- | ------------------------------------------------------------------------------------------ |
| `DEF-839` | the GraphQL contract does not limit the identifier to 16 digits (only the FE does)         |
| `DEF-840` | the registration never returns the medical record number — `noRM` is empty                 |
| `DEF-841` | an identifier that is already registered is not refused — the old patient comes back       |
| `DEF-842` | `patientCheckEmailIsExist` rejects every valid e-mail and lets every malformed one through |
| `DEF-843` | `telecom.statusCode` and `email.statusCode` are never written                              |

## Test data

Every patient the suite creates carries the QA marker in its name (`QA-E2E …`) and a synthetic NIK of the
form `99` + 14 digits — a range no real NIK occupies, which the staging DB write guard accepts. The date
segment of `qaNikWithBirthdate()` matches `birthDttm`, so the auto-fill of the form and the payload of the
spec agree.

The rows are removed again by `deletePatientRows(patientID)` (keyed on the literal patient id — the
harness refuses anything else). The DB reads need the staging credential file
`~/.config/staging-his-v3-db/his_v3_production_20260508.json`; the DB-backed cases skip themselves
when it is missing (`skipUnlessStagingDb()`).

That file is read by the harness scripts of `his-v3-ai` (`db.py`), never by this repo. `db.py` honours
`E2E_STAGING_CRED` first and falls back to the path above; the guard mirrors that, so a credential file
outside the default location works too:

```bash
E2E_STAGING_CRED=/path/to/his_v3_production_20260508.json TEST_ENV=staging npx playwright test --grep "TC-PATREG"
```

## Running

```bash
# GraphQL patient + encounter
npx playwright test --project=api-graphql tests/api/graphql/his-backend/patient/ \
  tests/api/graphql/his-backend/encounter/encounter-registration.spec.ts

# REST patient + encounter
npx playwright test --project=api-rest tests/api/rest/his-backend/patient-rest-auth.spec.ts \
  tests/api/rest/his-backend/encounter-rest-auth.spec.ts

# the form of his-frontend
npx playwright test --project=web-his-frontend-chromium tests/web/his-frontend/patient-registration/

# the whole epic at once
npx playwright test --grep "TC-PATREG"
```

Against staging: `TEST_ENV=staging npx playwright test …`. The staging target reads `env/.env.staging`
(git-ignored, created from `env/.env.staging.example` with `npm run env:init`); it needs the GraphQL root,
the REST root, the frontend URL and the login account.

Guards: `skipUnlessRoleLogin('admission')` (the account + the RSA public key), `skipUnlessBackendRest()`
(the REST root `HIS_BACKEND_REST_URL`, the gateway root **including** `/api/v1`) and
`skipUnlessStagingDb()` (the guarded staging reads).

### The account of the suite

Every flow declares the role it acts as, at the top of its spec file:

```ts
/** The role of this flow: the admission officer of the registration clusters. */
const ROLE = 'admission';
```

- the web cases sign in per test through the real login form (`hisFrontend.signInAsRole(ROLE)`);
- the API cases log in over GraphQL with the same account (`hisBackendPatient(ROLE)`).

The account lives in `test-data/private/credentials.<TEST_ENV>.json` — one entry per role:

```json
[{ "role": "admission", "username": "qa.admission", "password": "<from the password manager>" }]
```

Without that entry the cases that need a session skip themselves with a reason. The role needs an
access group in the target database that carries the registration menus — an account without it
answers `403` instead of registering a patient.

## Known limits

- **The suite has been executed against staging (2026-11-09)**, with an impersonated session
  (`Sup3rAdm1n`): GraphQL `patient` + `encounter` → 14 green, 15 expected failures, 9 skipped; REST
  `/api/v1/patients` + `/api/v1/encounters` → 5 expected failures; the form of his-frontend → 5 green,
  3 expected failures, 1 skipped. The whole epic in one run (`--grep "TC-PATREG"`) → 20 green,
  23 expected failures, 12 skipped, **0 failed**. The 19 green rows of the epic are the `automated` ones
  of the catalog; a row that pins an open defect is `blocked` with the reason `defect:DEF-nnn`.
- **Run the projects separately against staging.** The whole epic in one run puts the GraphQL, the REST
  and the browser project on the same staging backend at the same time; the form of his-frontend then
  times out while it loads (the page stays on its loading shell) and the browser cases fail although
  they are green on their own. The npm scripts of the repo already do that: `npm run test:api:graphql`,
  `npm run test:api:rest`, `npm run test:web`.
- **PR-32 did not reproduce** on the deployed build — the identifier never reaches the browser console, so
  TC-PATREG-062 asserts the behaviour instead of expecting a failure.
- **PR-03 covers `patientCreate` only.** `patientUpdate` is not a public function of the gateway, so an
  anonymous caller is refused with `401 no token found` before the resolver runs — TC-PATREG-051 is a green
  case.
- The **master registration path** of the form needs the regional search dialog (`SearchAddress`) to fill
  the address fields. The page object does not drive it, so the cases that need a submitted master form
  (TC-PATREG-006, TC-PATREG-059) are blocked. The **emergency path** is fully automated: with
  "Daftarkan Sebagai Pasien Darurat" the schema drops the NIK, the address and the terminology fields.
- The check-in (TC-PATREG-020) needs payor, payplan, healthcare service and practitioner master data that
  this suite does not seed.
- The SATUSEHAT and WhatsApp publish (TC-PATREG-024..027, TC-PATREG-049) need an AMQP client; this repo
  has none yet (`src/messaging/core/message-broker.ts` is still a contract).
- The cases that read the staging database need its credential file; without it they skip themselves
  (`skipUnlessStagingDb()`).
- **Without that credential file the REST cases leave rows behind.** They remove the patient they created
  with `deletePatientRows(patientID)`, which is a guarded staging write; without the credential the cleanup
  cannot run and the row survives the run. The `cleanup` fixture never fails a test for this — it records a
  `cleanup-failed` annotation in the report — so the run stays green while the staging DB keeps a
  `QA-E2E …` patient. Remove them by hand (or provide the credential file).
