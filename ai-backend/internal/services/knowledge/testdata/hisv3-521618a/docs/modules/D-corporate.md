# Part D — Corporate & web platform (his-data-integration, CCP, CRM, CIS, CCM, …)

Part of the [HISv3 module catalogue](README.md) — the single source of truth for modules, sub-modules and features.
Legend, status rules and the automation backlog: [README.md](README.md).

Columns `ID` … `Notes` are the product map (edit by hand when the product changes; feature names stay as reverse-engineered,
in Bahasa). `Status as of` = the date the status was last confirmed: for `PROD` "in production as of", for `DEV-DONE`
"development done (not yet used in production) as of". Columns `Epics` … `Automated` are **generated** by
`npm run catalog:report` from the user stories in `docs/test-catalog/` — never edit them by hand.

<a id="d01"></a>

### D01 — CIS & CCM (master korporat → HIS)

_BRD:_ `BRD:D20` · _Features:_ 6

| ID                          | Sub-module    | Feature                                                                                                                                               | Repo / entry point                | Status           | Status as of | Evidence                          | Notes                                                                                 | Epics                                                    | Stories                       | TCs | Automated |
| --------------------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | ---------------- | ------------ | --------------------------------- | ------------------------------------------------------------------------------------- | -------------------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-d01-01"></a>D01.01 | CIS           | 33 aksi `?name=` + REST `/{slug}/{verb}` (care-class, department, service-group/type, service, module, menu, access-right, user-group, user, formula) | his-data-integration `/ccm/poapi` | 🟡 `IN-PROGRESS` | —            | GIT:main/stg; S:CIS-001..003 (0%) | ongoing; /ccm & /cis digabung ke /ccm/poapi (HDI #114) — uji integrasi DB belum jalan | [DCIS](../test-catalog/data-integration/cis.md)          | [3](traceability.md#f-d01-01) | 101 | 0 (0%)    |
| <a id="f-d01-02"></a>D01.02 | CIS           | Rejection departemen/layanan/pengajuan                                                                                                                | his-data-integration              | 🟡 `IN-PROGRESS` | —            | GIT:main                          |                                                                                       | [DCIS](../test-catalog/data-integration/cis.md)          | [1](traceability.md#f-d01-02) | 7   | 0 (0%)    |
| <a id="f-d01-03"></a>D01.03 | Master Lab    | specimen-type, analyzer, method, material, specimen, lab-service                                                                                      | his-data-integration `/lab/*`     | 🟡 `IN-PROGRESS` | —            | GIT:main                          |                                                                                       | [DCISLAB](../test-catalog/data-integration/cis-lab.md)   | [3](traceability.md#f-d01-03) | 48  | 0 (0%)    |
| <a id="f-d01-04"></a>D01.04 | Master Lookup | list, one, dropdown, doctor autocomplete, user check/save                                                                                             | his-data-integration `/master/*`  | 🟡 `IN-PROGRESS` | —            | GIT:main                          |                                                                                       | [DCISMS](../test-catalog/data-integration/cis-master.md) | [4](traceability.md#f-d01-04) | 50  | 0 (0%)    |
| <a id="f-d01-05"></a>D01.05 | CCM           | Channel `/channel/{verb}` + deploy_api.php; ±97 REST API CCM                                                                                          | his-data-integration              | 🟡 `IN-PROGRESS` | —            | S:CCM-001..003 (0%); GIT:main     | ongoing; cakupan ±97 API belum terverifikasi; mapping 'JSON-only' = TODO              | [DCCM](../test-catalog/data-integration/ccm.md)          | [2](traceability.md#f-d01-05) | 26  | 0 (0%)    |
| <a id="f-d01-06"></a>D01.06 | Logging       | cis_ccm_api_log untuk setiap POST                                                                                                                     | his-data-integration              | 🟡 `IN-PROGRESS` | —            | GIT:main                          |                                                                                       |                                                          |                               |     |           |

<a id="d02"></a>

### D02 — CCP (Central Purchasing)

_BRD:_ `BRD:D17,D20` · _Features:_ 4

| ID                          | Sub-module      | Feature                                       | Repo / entry point          | Status           | Status as of | Evidence                    | Notes                                       | Epics                                           | Stories                       | TCs | Automated |
| --------------------------- | --------------- | --------------------------------------------- | --------------------------- | ---------------- | ------------ | --------------------------- | ------------------------------------------- | ----------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-d02-01"></a>D02.01 | Master Item CCP | deploy/item/{drug,bmhp,alkes}, deploy/mou     | his-data-integration `/ccp` | 🟢 `PROD`        | 2026-11-12   | S:CCP-001..003; RN:20260901 |                                             | [DCCP](../test-catalog/data-integration/ccp.md) | [2](traceability.md#f-d02-01) | 19  | 0 (0%)    |
| <a id="f-d02-02"></a>D02.02 | Status          | download/item/status, pengajuan/item/status   | his-data-integration `/ccp` | 🟢 `PROD`        | 2026-11-12   | S:CCP-001..003              | branch release/pengajuan-ccp ada di staging | [DCCP](../test-catalog/data-integration/ccp.md) | [1](traceability.md#f-d02-02) | 13  | 0 (0%)    |
| <a id="f-d02-03"></a>D02.03 | Stub            | /ccp/apifile/ccpapi.php, /ccp/poapi/poapi.php | his-data-integration        | 🟢 `PROD`        | 2026-11-12   | GIT:main                    | stub                                        | [DCCP](../test-catalog/data-integration/ccp.md) | [1](traceability.md#f-d02-03) | 4   | 0 (0%)    |
| <a id="f-d02-04"></a>D02.04 | PIP             | Pembelian internal Primaya                    | his-data-integration        | ⚪ `NOT-STARTED` | —            | S:PIP (TBD, 0%)             |                                             |                                                 |                               |     |           |

<a id="d03"></a>

### D03 — CRM (Primaya LMS)

_BRD:_ `BRD:D20` · _Features:_ 9

| ID                          | Sub-module          | Feature                                                                                                  | Repo / entry point                        | Status           | Status as of | Evidence            | Notes                                                                                 | Epics                                           | Stories                       | TCs | Automated |
| --------------------------- | ------------------- | -------------------------------------------------------------------------------------------------------- | ----------------------------------------- | ---------------- | ------------ | ------------------- | ------------------------------------------------------------------------------------- | ----------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-d03-01"></a>D03.01 | Perusahaan / Payor  | Deploy master perusahaan, payor, dan asuransi (upsert: kode, jenis, penjamin, diskon, PPN, kontak, bank) | his-data-integration + his-vendor-backend | 🟢 `PROD`        | 2026-11-12   | GIT:main            | aktif bila crmIntegration.enabled; hanya deploy (upsert), tanpa undeploy              | [VOCCRM](../test-catalog/vendor-backend/crm.md) | [3](traceability.md#f-d03-01) | 11  | 0 (0%)    |
| <a id="f-d03-02"></a>D03.02 | Tarif Perusahaan    | Deploy kontrak tarif perusahaan (periode berlaku, kelas, cito, cost sharing, diskon, SOC)                | his-data-integration + his-vendor-backend | 🟢 `PROD`        | 2026-11-12   | GIT:main            |                                                                                       | [VOCCRM](../test-catalog/vendor-backend/crm.md) | [3](traceability.md#f-d03-02) | 11  | 0 (0%)    |
| <a id="f-d03-03"></a>D03.03 | Tarif Perusahaan    | Detail tarif per layanan, item, kamar, dan margin progresif (ada di spec, belum diimplementasikan)       | his-data-integration + his-vendor-backend | ⚪ `NOT-STARTED` | —            | spec CRM            | DTO vendor belum memuat blok detail                                                   |                                                 |                               |     |           |
| <a id="f-d03-04"></a>D03.04 | Paket               | Deploy master paket (layanan, item, kamar, operasi, pembulatan)                                          | his-data-integration + his-vendor-backend | 🔵 `DEV-DONE`    | 2026-11-12   | GIT:main; S:CRM-003 | kode ada di main; marketing belum memakai                                             | [VOCCRM](../test-catalog/vendor-backend/crm.md) | [3](traceability.md#f-d03-04) | 10  | 0 (0%)    |
| <a id="f-d03-05"></a>D03.05 | Paket               | Pembagian share paket: RS, dokter, eksternal (ada di spec, belum diimplementasikan)                      | his-data-integration + his-vendor-backend | ⚪ `NOT-STARTED` | —            | spec CRM            |                                                                                       |                                                 |                               |     |           |
| <a id="f-d03-06"></a>D03.06 | Paket               | Marketing submit/deploy master paket baru (alur dari CRM)                                                | his-data-integration + his-vendor-backend | ⚪ `NOT-STARTED` | —            | S:CRM-003 (0%)      | belum ada desakan bisnis                                                              |                                                 |                               |     |           |
| <a id="f-d03-07"></a>D03.07 | Undeploy / Redeploy | Undeploy dan redeploy master perusahaan, tarif, dan paket                                                | his-data-integration + his-vendor-backend | ⚪ `NOT-STARTED` | —            | spec CRM            | belum ada pada kontrak CRM; deploy/undeploy grup layanan dan layanan ada di CIS (D01) |                                                 |                               |     |           |
| <a id="f-d03-08"></a>D03.08 | Laporan CRM         | Revenue paket untuk CRM (crm_revenuepaket; ada di spec, belum ada route)                                 | his-data-integration + his-vendor-backend | ⚪ `NOT-STARTED` | —            | spec CRM            |                                                                                       |                                                 |                               |     |           |
| <a id="f-d03-09"></a>D03.09 | Audit               | Log sinkronisasi masuk CRM (status received/processing/success/failed, payload, deployer)                | his-data-integration + his-vendor-backend | 🟢 `PROD`        | 2026-11-12   | GIT:main            |                                                                                       |                                                 |                               |     |           |

<a id="d04"></a>

### D04 — EIS

_BRD:_ `BRD:D20` · _Features:_ 1

| ID                          | Sub-module                 | Feature                                                           | Repo / entry point          | Status        | Status as of | Evidence             | Notes                                                | Epics                                           | Stories                       | TCs | Automated |
| --------------------------- | -------------------------- | ----------------------------------------------------------------- | --------------------------- | ------------- | ------------ | -------------------- | ---------------------------------------------------- | ----------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-d04-01"></a>D04.01 | Laporan Jadwal Cuti Dokter | `/eis/laporan-jadwal-cuti-dokter` (JSON/Excel/HTML, x-secret-key) | his-data-integration `/eis` | 🔵 `DEV-DONE` | 2026-11-12   | S:EIS-001,002 (blue) | sheet: mock response + static key; route ada di main | [DEIS](../test-catalog/data-integration/eis.md) | [2](traceability.md#f-d04-01) | 17  | 0 (0%)    |

<a id="d05"></a>

### D05 — ESI (CCD) & CCA — Corporate Platform

_BRD:_ `BRD:D20` · _Features:_ 2

| ID                          | Sub-module | Feature                                | Repo / entry point   | Status           | Status as of | Evidence       | Notes                                                                                     | Epics | Stories | TCs | Automated |
| --------------------------- | ---------- | -------------------------------------- | -------------------- | ---------------- | ------------ | -------------- | ----------------------------------------------------------------------------------------- | ----- | ------- | --- | --------- |
| <a id="f-d05-01"></a>D05.01 | ESI (CCD)  | Cron harian (±01:00–02:00) sinkron ESI | his-data-integration | ⚪ `NOT-STARTED` | —            | S:ESI-001 (0%) | belum start development (konfirmasi owner)                                                |       |         |     |           |
| <a id="f-d05-02"></a>D05.02 | CCA        | Export data & tabel CCA (view-only)    | his-backend          | ⚪ `NOT-STARTED` | —            | S:CCA-001      | belum start development (konfirmasi owner); beda dengan modul CCA view-only di core (A14) |       |         |     |           |

<a id="d06"></a>

### D06 — AIS (Akuntansi)

_BRD:_ `BRD:D20` · _Features:_ 1

| ID                          | Sub-module | Feature                                                     | Repo / entry point          | Status    | Status as of | Evidence    | Notes                  | Epics                                           | Stories                       | TCs | Automated |
| --------------------------- | ---------- | ----------------------------------------------------------- | --------------------------- | --------- | ------------ | ----------- | ---------------------- | ----------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-d06-01"></a>D06.01 | AIS        | recalculate, recalculate/bulk, hondok (publish ke RabbitMQ) | his-data-integration `/ais` | 🟢 `PROD` | 2026-11-12   | RN:20260901 | AIS Hondok V2 ✅ di RN | [DAIS](../test-catalog/data-integration/ais.md) | [3](traceability.md#f-d06-01) | 26  | 0 (0%)    |

<a id="d07"></a>

### D07 — Hardware & Alat Lab

_BRD:_ `BRD:D20,D24` · _Features:_ 3

| ID                          | Sub-module   | Feature                                                             | Repo / entry point               | Status    | Status as of | Evidence | Notes      | Epics                                                        | Stories                       | TCs | Automated |
| --------------------------- | ------------ | ------------------------------------------------------------------- | -------------------------------- | --------- | ------------ | -------- | ---------- | ------------------------------------------------------------ | ----------------------------- | --- | --------- |
| <a id="f-d07-01"></a>D07.01 | Mindray      | search-patient, store-mindray, store-mindray-intensive (vital sign) | his-data-integration `/hardware` | 🟢 `PROD` | 2026-11-12   | GIT:main | tanpa auth | [DHWVIT](../test-catalog/data-integration/hardware-vital.md) | [3](traceability.md#f-d07-01) | 13  | 0 (0%)    |
| <a id="f-d07-02"></a>D07.02 | Medscape     | store-vital-sign                                                    | his-data-integration `/hardware` | 🟢 `PROD` | 2026-11-12   | GIT:main |            | [DHWVIT](../test-catalog/data-integration/hardware-vital.md) | [1](traceability.md#f-d07-02) | 6   | 0 (0%)    |
| <a id="f-d07-03"></a>D07.03 | Lab Analyzer | result, raw, order, update_order                                    | his-data-integration `/hardware` | 🟢 `PROD` | 2026-11-12   | GIT:main |            | [DHWLAB](../test-catalog/data-integration/hardware-lab.md)   | [5](traceability.md#f-d07-03) | 23  | 0 (0%)    |

<a id="d08"></a>

### D08 — Westerindo dan Sistem Approval

_BRD:_ `BRD:D20` · _Features:_ 2

| ID                          | Sub-module            | Feature                                                                                   | Repo / entry point   | Status    | Status as of | Evidence              | Notes              | Epics                                                                                                        | Stories                       | TCs | Automated |
| --------------------------- | --------------------- | ----------------------------------------------------------------------------------------- | -------------------- | --------- | ------------ | --------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------ | ----------------------------- | --- | --------- |
| <a id="f-d08-01"></a>D08.01 | Westerindo Lab Outref | trigger-fetch hasil lab rujukan keluar                                                    | his-data-integration | 🟢 `PROD` | 2026-11-12   | RN:20260901           | PDF hasil didukung | [DLABREF](../test-catalog/data-integration/lab-outref.md)                                                    | [3](traceability.md#f-d08-01) | 13  | 0 (0%)    |
| <a id="f-d08-02"></a>D08.02 | Approval              | Halaman setuju/tolak melalui email; persetujuan diskon global, diskon per item, dan paket | his-data-integration | 🟢 `PROD` | 2026-11-12   | GIT:main; RN:20260901 |                    | [DAPPR](../test-catalog/data-integration/approval.md), [DONEPG](../test-catalog/data-integration/onepage.md) | [7](traceability.md#f-d08-02) | 26  | 0 (0%)    |

<a id="d09"></a>

### D09 — Transformer & Single Medrec (Datalake)

_BRD:_ `BRD:D2` · _Features:_ 1

| ID                          | Sub-module      | Feature                                                                      | Repo / entry point                  | Status    | Status as of | Evidence                                | Notes                                                                                                                      | Epics                                                                                                                                                                                       | Stories                        | TCs | Automated |
| --------------------------- | --------------- | ---------------------------------------------------------------------------- | ----------------------------------- | --------- | ------------ | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------ | --- | --------- |
| <a id="f-d09-01"></a>D09.01 | Transformer API | check-sync-data, data/{res}, regpas (by id/medrec/nik/ihs), resource-options | his-data-integration `/transformer` | 🟢 `PROD` | 2026-11-12   | S:SMR-001..009; S:TRS (Datalake) 3 task | sync regpas & medrec sudah prod (konfirmasi owner); transformer dipindah ke his-backend (RN 09-15), sebagian masih staging | [DTRANS](../test-catalog/data-integration/transformer.md), [OUTREG](../test-catalog/his-backend/outpatient-registration.md), [TRSYNC](../test-catalog/data-integration/transformer-sync.md) | [13](traceability.md#f-d09-01) | 94  | 50 (53%)  |

<a id="d10"></a>

### D10 — Mutu, Jadwal Dokter, Master Data, Public

_BRD:_ `BRD:D18,D23` · _Features:_ 5

| ID                          | Sub-module    | Feature                                                                | Repo / entry point                   | Status        | Status as of | Evidence        | Notes                         | Epics                                                          | Stories                       | TCs | Automated |
| --------------------------- | ------------- | ---------------------------------------------------------------------- | ------------------------------------ | ------------- | ------------ | --------------- | ----------------------------- | -------------------------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-d10-01"></a>D10.01 | Mutu          | data-patient, data-patient-detail, data-department, data-payor-patient | his-data-integration `/mutu`         | 🟢 `PROD`     | 2026-11-12   | S:Mutu (3 task) | auth failure → 500            | [DMUTU](../test-catalog/data-integration/mutu.md)              | [4](traceability.md#f-d10-01) | 17  | 0 (0%)    |
| <a id="f-d10-02"></a>D10.02 | Jadwal Dokter | daftar-poli, daftar-dokter, jadwal-dokter (+ alias /api.php)           | his-data-integration `/jadwaldokter` | 🟢 `PROD`     | 2026-11-12   | GIT:main        | tanpa auth                    | [DJADWAL](../test-catalog/data-integration/doctor-schedule.md) | [4](traceability.md#f-d10-02) | 20  | 0 (0%)    |
| <a id="f-d10-03"></a>D10.03 | Master Data   | holiday (Basic auth, scope master-data:holiday:read)                   | his-data-integration `/master-data`  | 🔵 `DEV-DONE` | 2026-11-12   | GIT:stg         |                               | [DMASTER](../test-catalog/data-integration/master-data.md)     | [1](traceability.md#f-d10-03) | 12  | 0 (0%)    |
| <a id="f-d10-04"></a>D10.04 | Public        | terminology, regional                                                  | his-data-integration `/public`       | 🟢 `PROD`     | 2026-11-12   | GIT:main        |                               | [DPUBLIC](../test-catalog/data-integration/public.md)          | [2](traceability.md#f-d10-04) | 15  | 0 (0%)    |
| <a id="f-d10-05"></a>D10.05 | Monitoring    | service-status, swagger, WhatsApp 5xx alert                            | his-data-integration                 | 🔵 `DEV-DONE` | 2026-11-12   | GIT:stg         | alert middleware staging-only | [DSYS](../test-catalog/data-integration/system.md)             | [1](traceability.md#f-d10-05) | 4   | 0 (0%)    |

<a id="d11"></a>

### D11 — BPJS

_BRD:_ `BRD:D20` · _Features:_ 4

| ID                          | Sub-module | Feature                                       | Repo / entry point                   | Status           | Status as of | Evidence               | Notes                             | Epics | Stories | TCs | Automated |
| --------------------------- | ---------- | --------------------------------------------- | ------------------------------------ | ---------------- | ------------ | ---------------------- | --------------------------------- | ----- | ------- | --- | --------- |
| <a id="f-d11-01"></a>D11.01 | PCare      | Semua proses PCare (mulai SEP); disarankan Go | belum ada (tidak ditemukan di BE/FE) | 🟡 `IN-PROGRESS` | —            | S:BPJS/PCARE (0%)      | under development (belum di main) |       |         |     |           |
| <a id="f-d11-02"></a>D11.02 | Vklaim     | Upload dokumen klaim, baca dokumen otomatis   | belum ada                            | 🟡 `IN-PROGRESS` | —            | S:BPJS/Vklaim (0%)     | under development (belum di main) |       |         |     |           |
| <a id="f-d11-03"></a>D11.03 | Eklaim     | Klaim rawat inap BPJS (SEP, INA-CBG)          | belum ada di main                    | 🟡 `IN-PROGRESS` | —            | owner 2026-11-10       | under development                 |       |         |     |           |
| <a id="f-d11-04"></a>D11.04 | JKN Mobile | Integrasi antrian HIS                         | belum ada                            | 🟡 `IN-PROGRESS` | —            | S:BPJS/JKN Mobile (0%) | milik BPJS; ongoing development   |       |         |     |           |

<a id="d12"></a>

### D12 — OnePage App (healthcare-one-page-app → his-data-integration)

_BRD:_ `BRD:D2,D20` · _Features:_ 1

| ID                          | Sub-module        | Feature                                                                   | Repo / entry point                                                | Status    | Status as of | Evidence | Notes                                            | Epics                                                 | Stories                       | TCs | Automated |
| --------------------------- | ----------------- | ------------------------------------------------------------------------- | ----------------------------------------------------------------- | --------- | ------------ | -------- | ------------------------------------------------ | ----------------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-d12-01"></a>D12.01 | Verifikasi Kontak | Verifikasi nomor kontak pasien dengan token yang dikirim via social media | his-data-integration `POST /onepageapp/social-media/verification` | 🟢 `PROD` | 2026-11-12   | GIT:main | fitur teknis; ValidateToken masih stub (DEF-456) | [DONEPG](../test-catalog/data-integration/onepage.md) | [1](traceability.md#f-d12-01) | 4   | 0 (0%)    |
