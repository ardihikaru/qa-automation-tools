# Outpatient registration (his-backend + his-frontend)

Test doc of the epic **[Outpatient registration](test-catalog/his-backend/outpatient-registration.md)** (catalog
code `OUTREG`).

The cluster is the outpatient registration of HISv3: the encounter form of his-frontend, the GraphQL
`encounter` module of his-backend and the REST encounter router of the same service. The stories, the
scenarios and the findings come from
`his-v3-ai/services-impl/202611/backend-rest-e2e-qa-assessment/data-integration/D-HISV3-OUTPATIENT-REGISTRATION/`
(`USER-STORIES.md` → `US-D-HISV3-OUTPATIENT-REGISTRATION-01..20`, `USER-SCENARIOS.md` →
`SC-D-HISV3-OUTPATIENT-REGISTRATION-01..06`, `ARCHITECTURE.md` §6 → findings `OR-01..OR-22`).

The journey the epic covers is _a patient is registered → the outpatient encounter is created_. The
master patient itself belongs to the `D-HISV3-PATIENT-REGISTRATION` epic, so a spec that needs a
complete encounter registers a patient over the API first.

## What is covered

| Surface                                                       | Folder                                     |
| ------------------------------------------------------------- | ------------------------------------------ |
| GraphQL `encounter` module — `encounterCreate` and its guards | `tests/api/graphql/his-backend/encounter/` |
| REST `/api/v1/encounters` — the unauthenticated write surface | `tests/api/rest/his-backend/`              |
| `/administration/encounter/outpatient` and its dashboard      | `tests/web/his-frontend/encounter/`        |

The clients live in `src/api/graphql/his-backend/encounter/` and `src/api/rest/his-backend/`, the
page objects in `src/web/his-frontend/pages/encounter/` and
`src/web/his-frontend/pages/encounter-dashboard/`. The payloads come from
`src/api/graphql/his-backend/encounter/outpatient.builders.ts`, the preconditions from
`outpatient.kit.ts`, the master data and the row reads from
`src/api/rest/his-backend/outpatient-master-data.db.ts` and `outpatient-encounter.db.ts`.

## The expected failures (the point of this epic)

The findings of the cluster are documented defects, not wishes. Each of them is pinned by a test that
keeps the **desired** behaviour in the assertion and is marked as an expected failure:

```ts
test(
  'TC-OUTREG-010 an outpatient encounter without a payment bearer is refused instead of crashing',
  {
    tag: [Tag.regression],
    annotation: { type: 'defect', description: 'DEF-850 / OR-01 — paymentBearer di-dereference tanpa penjaga' },
  },
  async ({ hisBackendEncounter, hisBackendPatient, cleanup }) => {
    // … arrange …
    const { response, body } = await api.execute(EncounterCreateOperation, { data: payload });

    // Assert — the desired behaviour: an answer, not a crash
    test.fail(true, 'DEF-850 / OR-01 — resolver dereference paymentBearer yang nil');
    expect(response.status()).toBeLessThan(500);
    expect(body.errors ?? []).toHaveLength(0);
    expect(body.data?.encounterCreate?.status).toBe(false);
  },
);
```

The defects this epic pins:

| Defect    | Finding                                                                                 | Case                 |
| --------- | --------------------------------------------------------------------------------------- | -------------------- |
| `DEF-850` | the resolver dereferences `paymentBearer`, its `address` and `referral` without a guard | `TC-OUTREG-010..012` |
| `DEF-853` | a regular outpatient encounter gets its registration number only at check-in            | `TC-OUTREG-013`      |
| `DEF-854` | `POST /api/v1/encounters` answers an anonymous caller                                   | `TC-OUTREG-015..016` |
| `DEF-858` | a patient may own a second active outpatient encounter                                  | `TC-OUTREG-014`      |
| `DEF-872` | the barcode check-in crashes instead of answering                                       | `TC-OUTREG-021`      |

The defect ids follow the numbering of the epic: `DEF-850` = OR-01, `DEF-851` = OR-02, … `DEF-871`
= OR-22 (the number is `849` + the finding number). `DEF-872` is not part of the cluster
documentation — it came out of the reading of the current source and took the next free id.

## Test data

Nothing of this epic is seeded: the master data it needs belongs to the hospital, not to the suite.

- **The patient** is created over the `patient` GraphQL module (`patientCreateFullWithSourceInfo`)
  and removed again by its literal patient id. The name and the phone number carry the `QA-E2E`
  marker.
- **The pay plan** is discovered from the staging schema: a `payplan` of the class `general` whose
  contract carries the administrative tariff of the `outpatient` patient class
  (`outpatient-master-data.db.ts`). A hospital without such a contract cannot register a walk-in
  patient at all, so the case skips with that reason.
- **The poli slot** is discovered the same way: a `healthcare_service_schedule` that runs on the
  weekday of the visit, with its practitioner, its healthcare service and its slot.
- **The rows of the encounter** are read back through the guarded staging SELECT of the harness
  (`outpatient-encounter.db.ts`) and removed by the literal encounter id.

## Running

```bash
# GraphQL encounter (outpatient registration)
TEST_ENV=staging npx playwright test tests/api/graphql/his-backend/encounter/outpatient-registration.spec.ts \
  --project=api-graphql

# REST encounter (the unauthenticated write surface)
TEST_ENV=staging npx playwright test tests/api/rest/his-backend/encounter-rest-outpatient.spec.ts \
  --project=api-rest

# the form of his-frontend
TEST_ENV=staging npx playwright test tests/web/his-frontend/encounter/outpatient-form.spec.ts \
  --project="web-*"

# the whole epic at once
TEST_ENV=staging npx playwright test --grep TC-OUTREG
```

The DB-backed cases need `HISV3_AI_ROOT` (the checkout of `his-v3-ai`) and the credential file of
the staging schema; both skip with that reason when they are missing.

## The account of the suite

Every flow declares the role it acts as, at the top of its spec file:

```ts
/** The role of this flow: the admission officer of the registration clusters. */
const ROLE = 'admission';
```

- the web cases sign in per test through the real login form (`hisFrontend.signInAsRole(ROLE)`);
- the API cases log in over GraphQL with the same account (`hisBackendEncounter(ROLE)`).

The account lives in `test-data/private/credentials.<TEST_ENV>.json` — one entry per role:

```json
[{ "role": "admission", "username": "qa.admission", "password": "<from the password manager>" }]
```

Without that entry every case that needs a session skips itself with a reason
(`skipUnlessRoleLogin(ROLE)`). The role needs an access group in the target database that
carries the registration menus — a user without it answers `403` instead of registering a patient.

## Known limits

- **The specs have never been executed.** The staging environment of the run machine rejects the
  shared account of `env/.env.staging` (`userLogin` answers "gagal melakukan otentikasi dengan username
  dan password yang diberikan"). Since this change the suite acts with the dedicated `admission`
  account instead, so the remaining dependency is that account: it has to be provisioned in the target
  database (access group + registration menus) and listed in
  `test-data/private/credentials.staging.json`. Until then the rows of the catalog stay `blocked` —
  now on the missing account, not on the rejected shared one. Re-run the suite once the account works,
  then move the rows from `blocked` to `automated`.
- **The web cases need a second validation.** The form reads the patient from the redux store, so
  the journey starts on the outpatient dashboard and walks through the shared patient-search
  dialog. That dialog has never been driven by this suite — the locators of the page objects are
  derived from the source of his-frontend, but they have to be confirmed against a running build.
- **The check-in is out of reach.** `GenerateRegistrationNumberByBarcodeCode` ends in
  `panic("implement me")` (`his-backend/internal/storage/encounter/encounter.go:5685`), so the
  barcode check-in never answers: the cases that need a registration number from the check-in
  (`TC-OUTREG-021`, `TC-OUTREG-022`) stay `blocked` until that function is implemented. The hard
  coded registration number `REG-ABC` of the same path (OR-10) is masked by that panic.
- **The race conditions are not automated.** `OR-03` (two officers, one registration number) and
  `OR-17` (two patients, one queue number) need two truly simultaneous writes against the same
  slot; a deterministic trigger does not exist yet.
