# Test catalog

The checklist of **what the product should be tested for** and how much of it is automated. Organized by epic (one file
per epic), traceable from user story to spec. Concepts and rules: [../test-strategy.md](../test-strategy.md).

Every user story names the HISv3 feature(s) it serves from the [module catalogue](../modules/README.md) — the single
source of what the product does and of its release status (`PROD`, `DEV-DONE`, …). Write the ids on the story line,
`- **US-X-01** (A01.07, A02.01) — As a …`, or `(unmapped: <why>)`; `npm run catalog:report` turns them into links and
refreshes the per-feature coverage, the [traceability](../modules/traceability.md) and the automation backlog.

Not every case is automated at the start — `planned` rows are the backlog. A case counts as covered only when
its status is `automated` and the linked spec exists and passes.

## Coverage

<!-- coverage:start -->

| Epic                                                                                                                                                        | Cases |   + |    - | automated | planned | blocked | manual-only | wont | Automated % |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ----: | --: | ---: | --------: | ------: | ------: | ----------: | ---: | ----------: |
| [AIS recalculate and hondok (his-data-integration)](data-integration/ais.md)                                                                                |    26 |   7 |   19 |         0 |      17 |       9 |           0 |    0 |          0% |
| [Approval by e-mail link (his-data-integration)](data-integration/approval.md)                                                                              |    15 |   5 |   10 |         0 |       9 |       6 |           0 |    0 |          0% |
| [CCM channel deploy (his-data-integration)](data-integration/ccm.md)                                                                                        |    26 |   9 |   17 |         0 |       6 |      20 |           0 |    0 |          0% |
| [CCP master item, MOU and status (his-data-integration)](data-integration/ccp.md)                                                                           |    36 |  12 |   24 |         0 |      26 |      10 |           0 |    0 |          0% |
| [CIS lab master deploy (his-data-integration)](data-integration/cis-lab.md)                                                                                 |    48 |  23 |   25 |         0 |      11 |      37 |           0 |    0 |          0% |
| [CIS master lookup and user check (his-data-integration)](data-integration/cis-master.md)                                                                   |    50 |  24 |   26 |         0 |      13 |      37 |           0 |    0 |          0% |
| [CIS entity deploy and rejection (his-data-integration)](data-integration/cis.md)                                                                           |   108 |  38 |   70 |         0 |      30 |      78 |           0 |    0 |          0% |
| [Public doctor schedule (his-data-integration)](data-integration/doctor-schedule.md)                                                                        |    20 |   9 |   11 |         0 |      14 |       6 |           0 |    0 |          0% |
| [EIS doctor leave schedule report (his-data-integration)](data-integration/eis.md)                                                                          |    17 |   6 |   11 |         0 |       4 |      13 |           0 |    0 |          0% |
| [eSIGN callback webhook (his-data-integration)](data-integration/esign.md)                                                                                  |     7 |   2 |    5 |         0 |       6 |       1 |           0 |    0 |          0% |
| [Lab analyzer interface (his-data-integration)](data-integration/hardware-lab.md)                                                                           |    23 |   7 |   16 |         0 |      16 |       7 |           0 |    0 |          0% |
| [Mindray and Medscape vital signs (his-data-integration)](data-integration/hardware-vital.md)                                                               |    19 |   5 |   14 |         0 |       4 |      15 |           0 |    0 |          0% |
| [Westerindo lab out-referral fetch (his-data-integration)](data-integration/lab-outref.md)                                                                  |    13 |   2 |   11 |         0 |       9 |       4 |           0 |    0 |          0% |
| [Master data holidays (his-data-integration)](data-integration/master-data.md)                                                                              |    12 |   7 |    5 |         0 |      12 |       0 |           0 |    0 |          0% |
| [Quality (mutu) reporting data (his-data-integration)](data-integration/mutu.md)                                                                            |    17 |   7 |   10 |         0 |      15 |       2 |           0 |    0 |          0% |
| [One-page app (verification, new patient, approvals) (his-data-integration)](data-integration/onepage.md)                                                   |    20 |   6 |   14 |         0 |      12 |       8 |           0 |    0 |          0% |
| [Public terminology and region lookup (his-data-integration)](data-integration/public.md)                                                                   |    15 |   7 |    8 |         0 |       9 |       6 |           0 |    0 |          0% |
| [Data integration service status (his-data-integration)](data-integration/system.md)                                                                        |     4 |   2 |    2 |         0 |       4 |       0 |           0 |    0 |          0% |
| [Transformer check-sync-data — regpas & medrec (his-data-integration → his-backend)](data-integration/transformer-sync.md)                                  |    54 |  37 |   17 |        50 |       3 |       0 |           1 |    0 |         93% |
| [Transformer API (regpas, medrec, resources) (his-data-integration)](data-integration/transformer.md)                                                       |    39 |  16 |   23 |         0 |      39 |       0 |           0 |    0 |          0% |
| [Authentication & session (his-frontend + his-backend GraphQL)](his-backend/auth.md)                                                                        |    26 |   7 |   19 |        22 |       3 |       1 |           0 |    0 |         85% |
| [Chart management (his-backend `chartmanagement` + his-frontend `/chart-management`)](his-backend/chart-management.md)                                      |   112 |  36 |   76 |       103 |       0 |       8 |           1 |    0 |         92% |
| [Outpatient registration (his-backend `encounter` GraphQL + REST, his-frontend encounter form)](his-backend/outpatient-registration.md)                     |    43 |  21 |   22 |         0 |       0 |      43 |           0 |    0 |          0% |
| [Patient registration (his-backend `patient` + `encounter` GraphQL, his-backend REST, his-frontend registration form)](his-backend/patient-registration.md) |    74 |  42 |   32 |        19 |       0 |      55 |           0 |    0 |         26% |
| [Report Builder (his-backend `reportbuilder` + his-frontend `/report-builder`)](his-backend/report-builder.md)                                              |    49 |  19 |   30 |        41 |       0 |       7 |           0 |    1 |         84% |
| [Monthly SIP report e-mail (his-backend cron + email-publisher-service)](his-backend/sip-report-monthly.md)                                                 |    10 |   6 |    4 |         6 |       1 |       3 |           0 |    0 |         60% |
| [Doctor SIP status by username (his-backend GraphQL `doctorSipStatusGet`)](his-backend/sip-status.md)                                                       |    15 |  12 |    3 |        15 |       0 |       0 |           0 |    0 |        100% |
| [CRM/LMS deploy (company, tariff, package) (his-vendor-backend)](vendor-backend/crm.md)                                                                     |    32 |   7 |   25 |         0 |       0 |      32 |           0 |    0 |          0% |
| [Garda Medika inbound results (his-vendor-backend)](vendor-backend/insurance/garda.md)                                                                      |    33 |   7 |   26 |        26 |       0 |       7 |           0 |    0 |         79% |
| [Sinarmas insurance webhooks (his-vendor-backend)](vendor-backend/insurance/sinarmas.md)                                                                    |    49 |  13 |   36 |         0 |       0 |      49 |           0 |    0 |          0% |
| [Kiosk visits, appointments and check-in (his-vendor-backend)](vendor-backend/kiosk/appointment.md)                                                         |    38 |   4 |   34 |         0 |      34 |       4 |           0 |    0 |          0% |
| [Kiosk API authentication gate (his-vendor-backend)](vendor-backend/kiosk/auth.md)                                                                          |    45 |  10 |   35 |         0 |      39 |       6 |           0 |    0 |          0% |
| [Kiosk pharmacy status, billing and lab results (his-vendor-backend)](vendor-backend/kiosk/clinical-read.md)                                                |    45 |  13 |   32 |         0 |      36 |       9 |           0 |    0 |          0% |
| [Kiosk content and settings (his-vendor-backend)](vendor-backend/kiosk/content.md)                                                                          |    42 |  16 |   26 |         0 |      41 |       1 |           0 |    0 |          0% |
| [Kiosk mocked BPJS, payment and delivery (his-vendor-backend)](vendor-backend/kiosk/mock.md)                                                                |    41 |  14 |   27 |         0 |      41 |       0 |           0 |    0 |          0% |
| [Kiosk officer calls (his-vendor-backend)](vendor-backend/kiosk/officer-call.md)                                                                            |    48 |  19 |   29 |         0 |      46 |       2 |           0 |    0 |          0% |
| [Kiosk patient registration and lookup (his-vendor-backend)](vendor-backend/kiosk/patient.md)                                                               |    34 |   7 |   27 |         0 |      33 |       1 |           0 |    0 |          0% |
| [Kiosk doctors, slots and leave (his-vendor-backend)](vendor-backend/kiosk/schedule.md)                                                                     |    21 |   9 |   12 |         0 |      19 |       2 |           0 |    0 |          0% |
| [Omnichannel booking lifecycle (his-vendor-backend)](vendor-backend/omnichannel/booking.md)                                                                 |    79 |  12 |   67 |        59 |       0 |      20 |           0 |    0 |         75% |
| [Contact-center lookups (his-vendor-backend)](vendor-backend/omnichannel/callcenter.md)                                                                     |    24 |  12 |   12 |        16 |       0 |       8 |           0 |    0 |         67% |
| [Omnichannel inquiry (hospital, schedule, profile) (his-vendor-backend)](vendor-backend/omnichannel/inquiry.md)                                             |    57 |  21 |   36 |        46 |       0 |      11 |           0 |    0 |         81% |
| [Omnichannel patient registration lookup (his-vendor-backend)](vendor-backend/omnichannel/registration.md)                                                  |    34 |   9 |   25 |        25 |       0 |       9 |           0 |    0 |         74% |
| [Vendor backend service status and routing (his-vendor-backend)](vendor-backend/system.md)                                                                  |     8 |   3 |    5 |         0 |       8 |       0 |           0 |    0 |          0% |
| [VTT EMR update and service requests (his-vendor-backend)](vendor-backend/vtt/emr.md)                                                                       |    37 |  11 |   26 |         0 |      30 |       7 |           0 |    0 |          0% |
| [VTT patient, practitioner and item lookup (his-vendor-backend)](vendor-backend/vtt/lookup.md)                                                              |    34 |  20 |   14 |         0 |      33 |       1 |           0 |    0 |          0% |
| [VTT SOAP dictation and audio (his-vendor-backend)](vendor-backend/vtt/soap.md)                                                                             |    30 |  10 |   20 |         0 |      28 |       2 |           0 |    0 |          0% |
| **Total**                                                                                                                                                   |  1629 | 591 | 1038 |       428 |     651 |     547 |           2 |    1 |         26% |

<!-- coverage:end -->

Refresh with `npm run catalog:report`; validate with `npm run catalog`.

## Not every case is automated

A case may be **listed** in the catalog without being **implemented**. Any row whose Status is `blocked`, `manual-only` or `wont`
MUST fill the `Reason` column as `<category>:<detail>`; `npm run catalog` rejects the row otherwise.

| Category      | Use when                                                                   | Detail required        |
| ------------- | -------------------------------------------------------------------------- | ---------------------- |
| `defect`      | the product is broken today (case would fail / asserts known-wrong result) | defect id, `DEF-152`   |
| `not-needed`  | out of scope for now, duplicate coverage, or low value                     | one-line justification |
| `blocked`     | a prerequisite is missing (seed data, dependency case, environment)        | what is missing        |
| `unsafe`      | would mutate shared/production-like data with no safe cleanup              | why and what is needed |
| `manual-only` | cannot be automated reliably (hardware, third-party UI, human judgement)   | why                    |
| `deferred`    | wanted, but scheduled for a later phase                                    | target phase           |
| `other`       | none of the above                                                          | free text              |

Rules: (1) the reviewer decides; an agent never silently drops or weakens a case — it sets Status + Reason instead. (2) A `defect`
case is re-checked when the defect is fixed and then moves to `planned`. (3) `planned`/`automated` rows keep `Reason` empty.

## Legend

| Field  | Values                                                                                                       |
| ------ | ------------------------------------------------------------------------------------------------------------ |
| +/-    | `+` happy path, `-` negative path (invalid input, missing auth, wrong role, not found, business rule)        |
| Level  | `API` REST, `GQL` GraphQL, `WEB` single-app UI, `E2E` journey across services/apps                           |
| Pri    | `P0` critical path → `@smoke`; `P1` important → `@regression`; `P2` normal; `P3` edge/nice-to-have           |
| Status | `planned`, `automated`, `blocked` (needs a ticket), `manual-only` (reason in the scenario), `wont` (decided) |
| Reason | required for `blocked`/`manual-only`/`wont`, see above                                                       |

## Folders

One folder per **service under test**; inside a service, a sub-folder per area when there are many epics. File names drop the
service prefix (`vendor-backend/kiosk/auth.md`, not `vendor-backend-kiosk-auth.md`).

| Folder                        | Holds                                                                                                                  |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `his-backend/`                | his-backend GraphQL / REST / cron, with the his-frontend UI that drives it (auth, chart management, registration, SIP) |
| `vendor-backend/omnichannel/` | his-vendor-backend omnichannel inquiry, registration lookup, booking, contact center                                   |
| `vendor-backend/kiosk/`       | his-vendor-backend `/kiosk-his-v2`                                                                                     |
| `vendor-backend/vtt/`         | his-vendor-backend `/vtt` (dictation to EMR)                                                                           |
| `vendor-backend/insurance/`   | his-vendor-backend inbound insurer webhooks (Garda Medika, Sinarmas)                                                   |
| `vendor-backend/`             | other his-vendor-backend surfaces (CRM deploy, system routes)                                                          |
| `data-integration/`           | his-data-integration (CIS/CCM, CCP, AIS, hardware, transformer, public, …)                                             |

A new service gets a new folder; `npm run catalog` rejects an epic file placed directly in `docs/test-catalog/`.

## Epics

<!-- epics:start -->

### `data-integration/`

| Epic                                                                                                                       | Code      | Cases | Automated |
| -------------------------------------------------------------------------------------------------------------------------- | --------- | ----: | --------: |
| [AIS recalculate and hondok (his-data-integration)](data-integration/ais.md)                                               | `DAIS`    |    26 |        0% |
| [Approval by e-mail link (his-data-integration)](data-integration/approval.md)                                             | `DAPPR`   |    15 |        0% |
| [CCM channel deploy (his-data-integration)](data-integration/ccm.md)                                                       | `DCCM`    |    26 |        0% |
| [CCP master item, MOU and status (his-data-integration)](data-integration/ccp.md)                                          | `DCCP`    |    36 |        0% |
| [CIS lab master deploy (his-data-integration)](data-integration/cis-lab.md)                                                | `DCISLAB` |    48 |        0% |
| [CIS master lookup and user check (his-data-integration)](data-integration/cis-master.md)                                  | `DCISMS`  |    50 |        0% |
| [CIS entity deploy and rejection (his-data-integration)](data-integration/cis.md)                                          | `DCIS`    |   108 |        0% |
| [Public doctor schedule (his-data-integration)](data-integration/doctor-schedule.md)                                       | `DJADWAL` |    20 |        0% |
| [EIS doctor leave schedule report (his-data-integration)](data-integration/eis.md)                                         | `DEIS`    |    17 |        0% |
| [eSIGN callback webhook (his-data-integration)](data-integration/esign.md)                                                 | `DESIGN`  |     7 |        0% |
| [Lab analyzer interface (his-data-integration)](data-integration/hardware-lab.md)                                          | `DHWLAB`  |    23 |        0% |
| [Mindray and Medscape vital signs (his-data-integration)](data-integration/hardware-vital.md)                              | `DHWVIT`  |    19 |        0% |
| [Westerindo lab out-referral fetch (his-data-integration)](data-integration/lab-outref.md)                                 | `DLABREF` |    13 |        0% |
| [Master data holidays (his-data-integration)](data-integration/master-data.md)                                             | `DMASTER` |    12 |        0% |
| [Quality (mutu) reporting data (his-data-integration)](data-integration/mutu.md)                                           | `DMUTU`   |    17 |        0% |
| [One-page app (verification, new patient, approvals) (his-data-integration)](data-integration/onepage.md)                  | `DONEPG`  |    20 |        0% |
| [Public terminology and region lookup (his-data-integration)](data-integration/public.md)                                  | `DPUBLIC` |    15 |        0% |
| [Data integration service status (his-data-integration)](data-integration/system.md)                                       | `DSYS`    |     4 |        0% |
| [Transformer check-sync-data — regpas & medrec (his-data-integration → his-backend)](data-integration/transformer-sync.md) | `TRSYNC`  |    54 |       93% |
| [Transformer API (regpas, medrec, resources) (his-data-integration)](data-integration/transformer.md)                      | `DTRANS`  |    39 |        0% |

### `his-backend/`

| Epic                                                                                                                                                        | Code      | Cases | Automated |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- | ----: | --------: |
| [Authentication & session (his-frontend + his-backend GraphQL)](his-backend/auth.md)                                                                        | `AUTH`    |    26 |       85% |
| [Chart management (his-backend `chartmanagement` + his-frontend `/chart-management`)](his-backend/chart-management.md)                                      | `CHMGMT`  |   112 |       92% |
| [Outpatient registration (his-backend `encounter` GraphQL + REST, his-frontend encounter form)](his-backend/outpatient-registration.md)                     | `OUTREG`  |    43 |        0% |
| [Patient registration (his-backend `patient` + `encounter` GraphQL, his-backend REST, his-frontend registration form)](his-backend/patient-registration.md) | `PATREG`  |    74 |       26% |
| [Report Builder (his-backend `reportbuilder` + his-frontend `/report-builder`)](his-backend/report-builder.md)                                              | `RB`      |    49 |       84% |
| [Monthly SIP report e-mail (his-backend cron + email-publisher-service)](his-backend/sip-report-monthly.md)                                                 | `SIPRPT`  |    10 |       60% |
| [Doctor SIP status by username (his-backend GraphQL `doctorSipStatusGet`)](his-backend/sip-status.md)                                                       | `SIPSTAT` |    15 |      100% |

### `vendor-backend/`

| Epic                                                                                       | Code     | Cases | Automated |
| ------------------------------------------------------------------------------------------ | -------- | ----: | --------: |
| [CRM/LMS deploy (company, tariff, package) (his-vendor-backend)](vendor-backend/crm.md)    | `VOCCRM` |    32 |        0% |
| [Vendor backend service status and routing (his-vendor-backend)](vendor-backend/system.md) | `VSYS`   |     8 |        0% |

### `vendor-backend/insurance/`

| Epic                                                                                     | Code       | Cases | Automated |
| ---------------------------------------------------------------------------------------- | ---------- | ----: | --------: |
| [Garda Medika inbound results (his-vendor-backend)](vendor-backend/insurance/garda.md)   | `VOCGARDA` |    33 |       79% |
| [Sinarmas insurance webhooks (his-vendor-backend)](vendor-backend/insurance/sinarmas.md) | `VOCSIN`   |    49 |        0% |

### `vendor-backend/kiosk/`

| Epic                                                                                                         | Code      | Cases | Automated |
| ------------------------------------------------------------------------------------------------------------ | --------- | ----: | --------: |
| [Kiosk visits, appointments and check-in (his-vendor-backend)](vendor-backend/kiosk/appointment.md)          | `VKAPPT`  |    38 |        0% |
| [Kiosk API authentication gate (his-vendor-backend)](vendor-backend/kiosk/auth.md)                           | `VKAUTH`  |    45 |        0% |
| [Kiosk pharmacy status, billing and lab results (his-vendor-backend)](vendor-backend/kiosk/clinical-read.md) | `VKREAD`  |    45 |        0% |
| [Kiosk content and settings (his-vendor-backend)](vendor-backend/kiosk/content.md)                           | `VKCONT`  |    42 |        0% |
| [Kiosk mocked BPJS, payment and delivery (his-vendor-backend)](vendor-backend/kiosk/mock.md)                 | `VKMOCK`  |    41 |        0% |
| [Kiosk officer calls (his-vendor-backend)](vendor-backend/kiosk/officer-call.md)                             | `VKOFF`   |    48 |        0% |
| [Kiosk patient registration and lookup (his-vendor-backend)](vendor-backend/kiosk/patient.md)                | `VKPAT`   |    34 |        0% |
| [Kiosk doctors, slots and leave (his-vendor-backend)](vendor-backend/kiosk/schedule.md)                      | `VKSCHED` |    21 |        0% |

### `vendor-backend/omnichannel/`

| Epic                                                                                                            | Code      | Cases | Automated |
| --------------------------------------------------------------------------------------------------------------- | --------- | ----: | --------: |
| [Omnichannel booking lifecycle (his-vendor-backend)](vendor-backend/omnichannel/booking.md)                     | `VOCBOOK` |    79 |       75% |
| [Contact-center lookups (his-vendor-backend)](vendor-backend/omnichannel/callcenter.md)                         | `VOCCC`   |    24 |       67% |
| [Omnichannel inquiry (hospital, schedule, profile) (his-vendor-backend)](vendor-backend/omnichannel/inquiry.md) | `VOCINQ`  |    57 |       81% |
| [Omnichannel patient registration lookup (his-vendor-backend)](vendor-backend/omnichannel/registration.md)      | `VOCREG`  |    34 |       74% |

### `vendor-backend/vtt/`

| Epic                                                                                           | Code      | Cases | Automated |
| ---------------------------------------------------------------------------------------------- | --------- | ----: | --------: |
| [VTT EMR update and service requests (his-vendor-backend)](vendor-backend/vtt/emr.md)          | `VTTEMR`  |    37 |        0% |
| [VTT patient, practitioner and item lookup (his-vendor-backend)](vendor-backend/vtt/lookup.md) | `VTTREAD` |    34 |        0% |
| [VTT SOAP dictation and audio (his-vendor-backend)](vendor-backend/vtt/soap.md)                | `VTTSOAP` |    30 |        0% |

<!-- epics:end -->

New epic: copy [_template.md](_template.md) into the service folder, run `npm run catalog:report` (the index above and the
coverage table are generated). Epics of the backend REST e2e
assessment clusters are seeded with `node scripts/import-qa-cases.mjs` (never overwrites an existing epic without `--force`).
