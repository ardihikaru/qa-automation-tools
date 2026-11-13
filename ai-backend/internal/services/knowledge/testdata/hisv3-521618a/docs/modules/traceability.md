# Traceability — feature → epic → user story → test cases

> **Generated** by `npm run catalog:report` from [the module catalogue](README.md) and the
> [test catalog](../test-catalog/README.md). Do not edit by hand. Only features with at least one user story are listed;
> the full feature list (with release status) is in the module files.

## A01 — Registrasi, Pasien & Kunjungan

<a id="f-a01-01"></a>

### [A01.01](A-core.md#f-a01-01) — Master Pasien: Pencarian, buat & ubah data pasien (`/master-patient`)

Release status `PROD` (as of 2026-11-12) · 6 stories · 34 test cases · 11 automated (32%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-PATREG-01 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want to register a new patient with the complete data so that the patient gets a … |     6 |         0 |       0 |               6 |
| US-PATREG-02 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want the system to tell me when an identity is already registered so that no two … |     8 |         5 |       0 |               3 |
| US-PATREG-09 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a medical records officer I want to correct a patient and activate a draft so that the master data stays u… |     4 |         2 |       0 |               2 |
| US-PATREG-11 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a counter officer I want to find a patient I just registered so that I can continue to check-in.            |     5 |         3 |       0 |               2 |
| US-PATREG-13 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a security officer I want every surface that writes or reads patient data to require authentication so tha… |     7 |         1 |       0 |               6 |
| US-PATREG-17 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As an auditor I want to know who registered, changed, activated and merged a patient so that every master-dat… |     4 |         0 |       0 |               4 |

<a id="f-a01-02"></a>

### [A01.02](A-core.md#f-a01-02) — Master Pasien: Merge pasien, alergi, general consent, membership

Release status `PROD` (as of 2026-11-12) · 2 stories · 7 test cases · 1 automated (14%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-PATREG-10 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a medical records officer I want to merge two patient records so that the clinical history is not split.    |     3 |         1 |       0 |               2 |
| US-PATREG-17 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As an auditor I want to know who registered, changed, activated and merged a patient so that every master-dat… |     4 |         0 |       0 |               4 |

<a id="f-a01-03"></a>

### [A01.03](A-core.md#f-a01-03) — Master Pasien: Pasien darurat / identitas sementara (`/master-patient-emergency`)

Release status `PROD` (as of 2026-11-12) · 1 stories · 5 test cases · 4 automated (80%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-PATREG-07 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As an ER officer I want to register an emergency patient quickly so that treatment can start before the MRN e… |     5 |         4 |       0 |               1 |

<a id="f-a01-04"></a>

### [A01.04](A-core.md#f-a01-04) — Master Pasien: NIK ibu sebagai jenis identitas pasien (bayi baru lahir)

Release status `PROD` (as of 2026-11-12) · 1 stories · 1 test cases · 0 automated (0%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-PATREG-08 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a midwife I want to register a newborn with the birth encounter so that the baby record is linked to the d… |     1 |         0 |       0 |               1 |

<a id="f-a01-05"></a>

### [A01.05](A-core.md#f-a01-05) — Pendaftaran: Registrasi pasien baru & pasien bayi baru lahir

Release status `PROD` (as of 2026-11-12) · 12 stories · 54 test cases · 9 automated (17%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DONEPG-02 | [DONEPG](../test-catalog/data-integration/onepage.md)         | Buat pasien baru (source: `US-D-ONEPAGE-02`)                                                                   |     5 |         0 |       3 |               2 |
| US-PATREG-01 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want to register a new patient with the complete data so that the patient gets a … |     6 |         0 |       0 |               6 |
| US-PATREG-02 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want the system to tell me when an identity is already registered so that no two … |     8 |         5 |       0 |               3 |
| US-PATREG-03 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want the medical record number to be unique and sequential so that no two patient… |     4 |         1 |       0 |               3 |
| US-PATREG-05 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want a national patient to be registered in SATUSEHAT so that the record is inter… |     2 |         0 |       0 |               2 |
| US-PATREG-06 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want the phone number verified over WhatsApp so that the hospital can reach the p… |     2 |         0 |       0 |               2 |
| US-PATREG-08 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a midwife I want to register a newborn with the birth encounter so that the baby record is linked to the d… |     1 |         0 |       0 |               1 |
| US-PATREG-12 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As an integrator I want the creation to be atomic and retry-safe so that there is no half-created or duplicat… |     3 |         0 |       0 |               3 |
| US-PATREG-13 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a security officer I want every surface that writes or reads patient data to require authentication so tha… |     7 |         1 |       0 |               6 |
| US-PATREG-14 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want everything I typed to be stored so that I do not have to type it twice.       |     4 |         0 |       0 |               4 |
| US-PATREG-15 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As an officer I want an actionable failure message so that I know what to correct.                             |     6 |         2 |       0 |               4 |
| US-PATREG-16 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As an integrator I want consistent numbering and data quality so that reports and third-party integrations ar… |     6 |         0 |       0 |               6 |

<a id="f-a01-06"></a>

### [A01.06](A-core.md#f-a01-06) — Pendaftaran: Pasien triage IGD + verifikasi WhatsApp otomatis (BUG-00232)

Release status `DEV-DONE` (as of 2026-11-12) · 1 stories · 2 test cases · 0 automated (0%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-PATREG-06 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want the phone number verified over WhatsApp so that the hospital can reach the p… |     2 |         0 |       0 |               2 |

<a id="f-a01-07"></a>

### [A01.07](A-core.md#f-a01-07) — Kunjungan (Encounter): Encounter RJ/IGD/RI/MCU, status, ganti dokter, konsultasi, rencana tindak lanjut

Release status `PROD` (as of 2026-11-12) · 21 stories · 47 test cases · 0 automated (0%)

| Story        | Epic                                                             | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-OUTREG-01 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want to create an outpatient visit for a registered patient so that the patient c… |     4 |         0 |       0 |               4 |
| US-OUTREG-02 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want to pick the right payor and coverage so that the visit is billed correctly.   |     6 |         0 |       0 |               6 |
| US-OUTREG-03 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want to pick the poli, the doctor and the time slot so that the visit is schedule… |     2 |         0 |       0 |               2 |
| US-OUTREG-04 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want a warning when the patient already has an active outpatient visit so that no… |     0 |         0 |       0 |               0 |
| US-OUTREG-05 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want a registration number at check-in so that the queue is unambiguous.                   |     2 |         0 |       0 |               2 |
| US-OUTREG-06 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want the outpatient visit to carry a registration number from the start so that r… |     1 |         0 |       0 |               1 |
| US-OUTREG-07 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want the queue to show a unique number and a barcode so that every patient is called unam… |     1 |         0 |       0 |               1 |
| US-OUTREG-08 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want to give the patient a printable proof of registration so that the patient ha… |     2 |         0 |       0 |               2 |
| US-OUTREG-09 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As an IT officer I want every surface that writes an outpatient encounter to be authenticated so that patient… |     3 |         0 |       0 |               3 |
| US-OUTREG-10 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want an actionable error message so that I know what to correct.                   |     3 |         0 |       0 |               3 |
| US-OUTREG-11 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a data manager I want the outpatient visit to carry a registration number since it is created so that the … |     1 |         0 |       0 |               1 |
| US-OUTREG-12 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want the registration number to be free of race conditions so that two patients n… |     1 |         0 |       0 |               1 |
| US-OUTREG-13 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want one active outpatient visit per patient so that no bill is duplicated.        |     1 |         0 |       0 |               1 |
| US-OUTREG-14 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want the barcode check-in to produce a correct registration number so that the barcode ca… |     0 |         0 |       0 |               0 |
| US-OUTREG-15 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As an IT officer I want the REST encounter surface to be authenticated so that no encounter and no bill can b… |     2 |         0 |       0 |               2 |
| US-OUTREG-16 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want the queue number to be free of collisions so that two patients are never called with… |     1 |         0 |       0 |               1 |
| US-OUTREG-17 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want an honest failure report so that I do not repeat a registration that already… |     3 |         0 |       0 |               3 |
| US-OUTREG-18 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want the poli, the doctor and the slot to stay mandatory so that the visit cannot… |     5 |         0 |       0 |               5 |
| US-OUTREG-19 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a registration officer I want the patient data to be stored consistently so that the downstream services r… |     4 |         0 |       0 |               4 |
| US-OUTREG-20 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As an auditor I want the encounter write to carry the identity of the caller so that every change is traceabl… |     1 |         0 |       0 |               1 |
| US-PATREG-04 | [PATREG](../test-catalog/his-backend/patient-registration.md)    | As a counter officer I want a registration number at check-in so that the queue is unambiguous.                |     4 |         0 |       0 |               4 |

## A02 — Antrian & Tampilan Publik

<a id="f-a02-01"></a>

### [A02.01](A-core.md#f-a02-01) — Antrian: Antrian umum, per ruang/dokter, monitoring antrian, scanner

Release status `PROD` (as of 2026-11-12) · 5 stories · 8 test cases · 0 automated (0%)

| Story        | Epic                                                             | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-OUTREG-05 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want a registration number at check-in so that the queue is unambiguous.                   |     2 |         0 |       0 |               2 |
| US-OUTREG-07 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want the queue to show a unique number and a barcode so that every patient is called unam… |     1 |         0 |       0 |               1 |
| US-OUTREG-14 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want the barcode check-in to produce a correct registration number so that the barcode ca… |     0 |         0 |       0 |               0 |
| US-OUTREG-16 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a poli officer I want the queue number to be free of collisions so that two patients are never called with… |     1 |         0 |       0 |               1 |
| US-PATREG-04 | [PATREG](../test-catalog/his-backend/patient-registration.md)    | As a counter officer I want a registration number at check-in so that the queue is unambiguous.                |     4 |         0 |       0 |               4 |

## A06 — Billing, Kasir, Paket & Payor

<a id="f-a06-08"></a>

### [A06.08](A-core.md#f-a06-08) — Diskon: Approval diskon kasir berjenjang (global & per item)

Release status `PROD` (as of 2026-11-12) · 2 stories · 8 test cases · 0 automated (0%)

| Story        | Epic                                                  | Story text                                                  | Cases | Automated | Planned | Blocked / other |
| ------------ | ----------------------------------------------------- | ----------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DONEPG-03 | [DONEPG](../test-catalog/data-integration/onepage.md) | Approve / tolak diskon global (source: `US-D-ONEPAGE-03`)   |     5 |         0 |       4 |               1 |
| US-DONEPG-04 | [DONEPG](../test-catalog/data-integration/onepage.md) | Approve / tolak diskon per item (source: `US-D-ONEPAGE-04`) |     3 |         0 |       2 |               1 |

<a id="f-a06-14"></a>

### [A06.14](A-core.md#f-a06-14) — Paket Layanan: Approval berjenjang master paket

Release status `PROD` (as of 2026-11-12) · 1 stories · 3 test cases · 0 automated (0%)

| Story        | Epic                                                  | Story text                                                | Cases | Automated | Planned | Blocked / other |
| ------------ | ----------------------------------------------------- | --------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DONEPG-05 | [DONEPG](../test-catalog/data-integration/onepage.md) | Approve / tolak paket layanan (source: `US-D-ONEPAGE-05`) |     3 |         0 |       2 |               1 |

## A07 — Laboratorium

<a id="f-a07-04"></a>

### [A07.04](A-core.md#f-a07-04) — QC Lab: Kontrol, Levey-Jennings, lock analyzer, laporan QC

Release status `PROD` (as of 2026-11-12) · 1 stories · 1 test cases · 0 automated (0%)

| Story        | Epic                                                       | Story text                                         | Cases | Automated | Planned | Blocked / other |
| ------------ | ---------------------------------------------------------- | -------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DHWLAB-02 | [DHWLAB](../test-catalog/data-integration/hardware-lab.md) | Submit Data QC Analyzer (source: `US-D-HW-LAB-02`) |     1 |         0 |       1 |               0 |

<a id="f-a07-06"></a>

### [A07.06](A-core.md#f-a07-06) — Outref: Rujukan lab keluar (Westerindo) kirim/terima

Release status `PROD` (as of 2026-11-12) · 3 stories · 13 test cases · 0 automated (0%)

| Story         | Epic                                                      | Story text                                                                   | Cases | Automated | Planned | Blocked / other |
| ------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DLABREF-01 | [DLABREF](../test-catalog/data-integration/lab-outref.md) | Re-fetch Hasil Rujukan dengan Token Valid (source: `US-D-LABOUTREF-01`)      |     2 |         0 |       2 |               0 |
| US-DLABREF-02 | [DLABREF](../test-catalog/data-integration/lab-outref.md) | Penolakan Request Tanpa Token atau Token Salah (source: `US-D-LABOUTREF-02`) |     7 |         0 |       5 |               2 |
| US-DLABREF-03 | [DLABREF](../test-catalog/data-integration/lab-outref.md) | Error Handling Saat Westerindo Tidak Tersedia (source: `US-D-LABOUTREF-03`)  |     4 |         0 |       2 |               2 |

## A10 — Approval / Persetujuan

<a id="f-a10-02"></a>

### [A10.02](A-core.md#f-a10-02) — Approval by Email / WA: Approval via email/WhatsApp untuk approval berjenjang

Release status `PROD` (as of 2026-11-12) · 4 stories · 15 test cases · 0 automated (0%)

| Story       | Epic                                                  | Story text                                                                       | Cases | Automated | Planned | Blocked / other |
| ----------- | ----------------------------------------------------- | -------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DAPPR-01 | [DAPPR](../test-catalog/data-integration/approval.md) | Approve permintaan via link (source: `US-D-APPROVAL-01`)                         |     6 |         0 |       3 |               3 |
| US-DAPPR-02 | [DAPPR](../test-catalog/data-integration/approval.md) | Reject permintaan via link dengan catatan (source: `US-D-APPROVAL-02`)           |     4 |         0 |       3 |               1 |
| US-DAPPR-03 | [DAPPR](../test-catalog/data-integration/approval.md) | Tampilkan form rejection (source: `US-D-APPROVAL-03`)                            |     3 |         0 |       3 |               0 |
| US-DAPPR-04 | [DAPPR](../test-catalog/data-integration/approval.md) | Keamanan — endpoint approval tidak boleh tanpa auth (source: `US-D-APPROVAL-04`) |     2 |         0 |       0 |               2 |

## A11 — Mutu, Laporan & Analitik

<a id="f-a11-01"></a>

### [A11.01](A-core.md#f-a11-01) — Mutu: Laporan mutu (data pasien, departemen, payor)

Release status `PROD` (as of 2026-11-12) · 4 stories · 17 test cases · 0 automated (0%)

| Story       | Epic                                              | Story text                                                                 | Cases | Automated | Planned | Blocked / other |
| ----------- | ------------------------------------------------- | -------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DMUTU-01 | [DMUTU](../test-catalog/data-integration/mutu.md) | Data pasien untuk pelaporan mutu (source: `US-D-MUTU-01`)                  |     6 |         0 |       5 |               1 |
| US-DMUTU-02 | [DMUTU](../test-catalog/data-integration/mutu.md) | Detail data pasien untuk audit mutu (source: `US-D-MUTU-02`)               |     4 |         0 |       4 |               0 |
| US-DMUTU-03 | [DMUTU](../test-catalog/data-integration/mutu.md) | Data departemen untuk filter laporan (source: `US-D-MUTU-03`)              |     4 |         0 |       4 |               0 |
| US-DMUTU-04 | [DMUTU](../test-catalog/data-integration/mutu.md) | Data pembayar pasien untuk analisis keuangan mutu (source: `US-D-MUTU-04`) |     3 |         0 |       2 |               1 |

<a id="f-a11-04"></a>

### [A11.04](A-core.md#f-a11-04) — Chart Management: Dashboard dan chart builder / management (SQL terjaga, validasi dua mata, grup penonton)

Release status `DEV-DONE` (as of 2026-11-12) · 4 stories · 112 test cases · 103 automated (92%)

| Story        | Epic                                                      | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-CHMGMT-01 | [CHMGMT](../test-catalog/his-backend/chart-management.md) | As an Administrator I want to build a SQL-driven chart with verified safe SQL so that I can track operational… |    19 |        18 |       0 |               1 |
| US-CHMGMT-02 | [CHMGMT](../test-catalog/his-backend/chart-management.md) | As an Administrator I want to assemble charts into a dashboard with viewer-group access control so that only … |    14 |        12 |       0 |               2 |
| US-CHMGMT-03 | [CHMGMT](../test-catalog/his-backend/chart-management.md) | As a viewer I want to open a dashboard and see rendered charts so that I can read aggregate operational data … |    12 |        12 |       0 |               0 |
| US-CHMGMT-04 | [CHMGMT](../test-catalog/his-backend/chart-management.md) | As a system I want to block suspicious SQL, enforce chunk-transport integrity, and suppress small cells autom… |    67 |        61 |       0 |               6 |

<a id="f-a11-05"></a>

### [A11.05](A-core.md#f-a11-05) — Laporan SIP Bulanan: Email laporan SIP bulanan + `doctorSipStatusGet` (FEATURE-00240)

Release status `DEV-DONE` (as of 2026-11-12) · 6 stories · 25 test cases · 21 automated (84%)

| Story         | Epic                                                        | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------- | ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-SIPRPT-01  | [SIPRPT](../test-catalog/his-backend/sip-report-monthly.md) | As a hospital director I want a monthly list of doctors whose SIP is expired or expiring so that I can have t… |     2 |         2 |       0 |               0 |
| US-SIPRPT-02  | [SIPRPT](../test-catalog/his-backend/sip-report-monthly.md) | As a hospital director I want the PDF briefing and the Excel list in the same e-mail so that I can read the s… |     5 |         4 |       1 |               0 |
| US-SIPRPT-03  | [SIPRPT](../test-catalog/his-backend/sip-report-monthly.md) | As an operator I want the cron to fail loudly or skip safely (no recipients, disabled, already sent) so that … |     3 |         0 |       0 |               3 |
| US-SIPSTAT-01 | [SIPSTAT](../test-catalog/his-backend/sip-status.md)        | As an administrator I want the SIP status, date range and days left of a doctor so that I can chase renewals … |    10 |        10 |       0 |               0 |
| US-SIPSTAT-02 | [SIPSTAT](../test-catalog/his-backend/sip-status.md)        | As a user I want to see my own SIP status without typing a username, while nobody but an Administrator or Sup… |     4 |         4 |       0 |               0 |
| US-SIPSTAT-03 | [SIPSTAT](../test-catalog/his-backend/sip-status.md)        | As a security owner I want the SIP number never to leave the backend through this query.                       |     1 |         1 |       0 |               0 |

## A13 — Akses, Hak Akses & Pengguna

<a id="f-a13-01"></a>

### [A13.01](A-core.md#f-a13-01) — User Management: Master user/akun (dokter, non-dokter, spesialis), peran, jadwal, cuti

Release status `PROD` (as of 2026-11-12) · 2 stories · 2 test cases · 1 automated (50%)

| Story      | Epic                                        | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ---------- | ------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-AUTH-07 | [AUTH](../test-catalog/his-backend/auth.md) | As a supervisor I want to sign in on behalf of a practitioner so that I can complete administration for the p… |     1 |         1 |       0 |               0 |
| US-AUTH-10 | [AUTH](../test-catalog/his-backend/auth.md) | As a security officer I want the practitioner list to verify the caller identity so that it is not readable w… |     1 |         0 |       1 |               0 |

<a id="f-a13-02"></a>

### [A13.02](A-core.md#f-a13-02) — Access Group / Menu / ACL: Menu tree, ACL per acg_code; drag-and-drop master access group

Release status `PROD` (as of 2026-11-12) · 1 stories · 9 test cases · 8 automated (89%)

| Story      | Epic                                        | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ---------- | ------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-AUTH-01 | [AUTH](../test-catalog/his-backend/auth.md) | As a hospital staff member I want to sign in with my User ID and password so that I can work with the menus m… |     9 |         8 |       1 |               0 |

<a id="f-a13-03"></a>

### [A13.03](A-core.md#f-a13-03) — Sesi: Session management & monitoring

Release status `PROD` (as of 2026-11-12) · 7 stories · 26 test cases · 22 automated (85%)

| Story      | Epic                                        | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ---------- | ------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-AUTH-01 | [AUTH](../test-catalog/his-backend/auth.md) | As a hospital staff member I want to sign in with my User ID and password so that I can work with the menus m… |     9 |         8 |       1 |               0 |
| US-AUTH-02 | [AUTH](../test-catalog/his-backend/auth.md) | As a signed-in staff member I want my session to survive the next page so that I do not have to sign in again… |     7 |         6 |       0 |               1 |
| US-AUTH-03 | [AUTH](../test-catalog/his-backend/auth.md) | As a staff member who mistypes the password I want a clear message so that I know what to correct (D-HISV3-AU… |     3 |         3 |       0 |               0 |
| US-AUTH-04 | [AUTH](../test-catalog/his-backend/auth.md) | As a staff member who finished working I want to sign out so that the next person cannot use my access on a s… |     4 |         4 |       0 |               0 |
| US-AUTH-07 | [AUTH](../test-catalog/his-backend/auth.md) | As a supervisor I want to sign in on behalf of a practitioner so that I can complete administration for the p… |     1 |         1 |       0 |               0 |
| US-AUTH-08 | [AUTH](../test-catalog/his-backend/auth.md) | As a security officer I want the REST login endpoint to verify the credentials so that no token is issued wit… |     1 |         0 |       1 |               0 |
| US-AUTH-10 | [AUTH](../test-catalog/his-backend/auth.md) | As a security officer I want the practitioner list to verify the caller identity so that it is not readable w… |     1 |         0 |       1 |               0 |

<a id="f-a13-06"></a>

### [A13.06](A-core.md#f-a13-06) — SIP Dokter: `doctorSipStatusGet` admin-only

Release status `DEV-DONE` (as of 2026-11-12) · 3 stories · 15 test cases · 15 automated (100%)

| Story         | Epic                                                 | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------- | ---------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-SIPSTAT-01 | [SIPSTAT](../test-catalog/his-backend/sip-status.md) | As an administrator I want the SIP status, date range and days left of a doctor so that I can chase renewals … |    10 |        10 |       0 |               0 |
| US-SIPSTAT-02 | [SIPSTAT](../test-catalog/his-backend/sip-status.md) | As a user I want to see my own SIP status without typing a username, while nobody but an Administrator or Sup… |     4 |         4 |       0 |               0 |
| US-SIPSTAT-03 | [SIPSTAT](../test-catalog/his-backend/sip-status.md) | As a security owner I want the SIP number never to leave the backend through this query.                       |     1 |         1 |       0 |               0 |

## A14 — Integrasi Eksternal (sisi core)

<a id="f-a14-01"></a>

### [A14.01](A-core.md#f-a14-01) — SATUSEHAT: Pengiriman FHIR, dashboard, summary, utility, detail encounter

Release status `PROD` (as of 2026-11-12) · 1 stories · 2 test cases · 0 automated (0%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-PATREG-05 | [PATREG](../test-catalog/his-backend/patient-registration.md) | As a registration officer I want a national patient to be registered in SATUSEHAT so that the record is inter… |     2 |         0 |       0 |               2 |

<a id="f-a14-05"></a>

### [A14.05](A-core.md#f-a14-05) — E-Sign: Dashboard e-sign, verifikasi dokumen

Release status `PROD` (as of 2026-11-12) · 2 stories · 7 test cases · 0 automated (0%)

| Story        | Epic                                                | Story text                                              | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------- | ------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DESIGN-01 | [DESIGN](../test-catalog/data-integration/esign.md) | Proses callback webhook eSIGN (source: `US-D-ESIGN-01`) |     5 |         0 |       5 |               0 |
| US-DESIGN-02 | [DESIGN](../test-catalog/data-integration/esign.md) | Keamanan webhook eSIGN (source: `US-D-ESIGN-02`)        |     2 |         0 |       1 |               1 |

## B01 — Omnichannel — Inquiry & Registrasi

<a id="f-b01-01"></a>

### [B01.01](B-vendor.md#f-b01-01) — Inquiry: rs, jadwal, dokter, jadwaldokter, jadwaldoktertersedia, jadwaldokter_periodetanggal, profile

Release status `PROD` (as of 2026-11-12) · 7 stories · 57 test cases · 46 automated (81%)

| Story        | Epic                                                            | Story text                                                                 | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------------------- | -------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCINQ-01 | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md) | Daftar rumah sakit (rs) (source: `US-V-OC-INQ-01`)                         |     6 |         5 |       0 |               1 |
| US-VOCINQ-02 | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md) | Jadwal praktek di poli pada tanggal tertentu (source: `US-V-OC-INQ-02`)    |    10 |         7 |       0 |               3 |
| US-VOCINQ-03 | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md) | Daftar dokter di poli (source: `US-V-OC-INQ-03`)                           |     6 |         6 |       0 |               0 |
| US-VOCINQ-04 | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md) | Slot waktu dokter tertentu (source: `US-V-OC-INQ-04`)                      |     6 |         6 |       0 |               0 |
| US-VOCINQ-05 | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md) | Ketersediaan slot dokter (jadwaldoktertersedia) (source: `US-V-OC-INQ-05`) |    11 |        10 |       0 |               1 |
| US-VOCINQ-06 | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md) | Jadwal dokter untuk periode tanggal (source: `US-V-OC-INQ-06`)             |     9 |         7 |       0 |               2 |
| US-VOCINQ-07 | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md) | Profil dokter lengkap (source: `US-V-OC-INQ-07`)                           |     9 |         5 |       0 |               4 |

<a id="f-b01-02"></a>

### [B01.02](B-vendor.md#f-b01-02) — Registrasi: medrec, medrec_nama (cari pasien: MRN / nama / NIK / telepon)

Release status `PROD` (as of 2026-11-12) · 2 stories · 23 test cases · 17 automated (74%)

| Story        | Epic                                                                 | Story text                                                                 | Cases | Automated | Planned | Blocked / other |
| ------------ | -------------------------------------------------------------------- | -------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCREG-01 | [VOCREG](../test-catalog/vendor-backend/omnichannel/registration.md) | Cari MRN berdasarkan identitas lengkap (medrec) (source: `US-V-OC-REG-01`) |    12 |         9 |       0 |               3 |
| US-VOCREG-02 | [VOCREG](../test-catalog/vendor-backend/omnichannel/registration.md) | Verifikasi MRN dengan nama (medrec_nama) (source: `US-V-OC-REG-02`)        |    11 |         8 |       0 |               3 |

## B02 — Omnichannel — Booking

<a id="f-b02-01"></a>

### [B02.01](B-vendor.md#f-b02-01) — Booking: cek_gantung, cek_slot, booking, booking_baru

Release status `PROD` (as of 2026-11-12) · 4 stories · 47 test cases · 35 automated (74%)

| Story         | Epic                                                             | Story text                                                                       | Cases | Automated | Planned | Blocked / other |
| ------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCBOOK-01 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Pengecekan Tagihan Gantung (source: `US-V-OC-BOOK-01`)                           |    10 |         6 |       0 |               4 |
| US-VOCBOOK-02 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Pengecekan Ketersediaan Slot (source: `US-V-OC-BOOK-02`)                         |    10 |         9 |       0 |               1 |
| US-VOCBOOK-03 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Booking Pasien Lama (source: `US-V-OC-BOOK-03`)                                  |    16 |        11 |       0 |               5 |
| US-VOCBOOK-04 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Booking Pasien Baru (Registrasi + Booking Sekaligus) (source: `US-V-OC-BOOK-04`) |    11 |         9 |       0 |               2 |

<a id="f-b02-02"></a>

### [B02.02](B-vendor.md#f-b02-02) — Booking: booking_bayar, summary, booking_edit, booking_document, booking_batal

Release status `PROD` (as of 2026-11-12) · 5 stories · 32 test cases · 24 automated (75%)

| Story         | Epic                                                             | Story text                                                | Cases | Automated | Planned | Blocked / other |
| ------------- | ---------------------------------------------------------------- | --------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCBOOK-05 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Konfirmasi Pembayaran Booking (source: `US-V-OC-BOOK-05`) |     6 |         3 |       0 |               3 |
| US-VOCBOOK-06 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Melihat Ringkasan Booking (source: `US-V-OC-BOOK-06`)     |     6 |         4 |       0 |               2 |
| US-VOCBOOK-07 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Reschedule Booking (source: `US-V-OC-BOOK-07`)            |     6 |         6 |       0 |               0 |
| US-VOCBOOK-08 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Upload Dokumen Booking (source: `US-V-OC-BOOK-08`)        |     7 |         6 |       0 |               1 |
| US-VOCBOOK-09 | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md) | Pembatalan Booking (source: `US-V-OC-BOOK-09`)            |     7 |         5 |       0 |               2 |

<a id="f-b02-03"></a>

### [B02.03](B-vendor.md#f-b02-03) — Contact Center: list_departemen, list_appointment, info_kamar, validasi_pasien

Release status `PROD` (as of 2026-11-12) · 4 stories · 35 test cases · 24 automated (69%)

| Story        | Epic                                                                 | Story text                                                                       | Cases | Automated | Planned | Blocked / other |
| ------------ | -------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCCC-01  | [VOCCC](../test-catalog/vendor-backend/omnichannel/callcenter.md)    | Daftar departemen/poliklinik (source: `US-V-OC-CC-01`)                           |     6 |         4 |       0 |               2 |
| US-VOCCC-02  | [VOCCC](../test-catalog/vendor-backend/omnichannel/callcenter.md)    | Daftar appointment berdasarkan filter (source: `US-V-OC-CC-02`)                  |    10 |         5 |       0 |               5 |
| US-VOCCC-03  | [VOCCC](../test-catalog/vendor-backend/omnichannel/callcenter.md)    | Daftar kamar rawat inap (source: `US-V-OC-CC-03`)                                |     8 |         7 |       0 |               1 |
| US-VOCREG-03 | [VOCREG](../test-catalog/vendor-backend/omnichannel/registration.md) | Verifikasi pasien di contact center (validasi_pasien) (source: `US-V-OC-REG-03`) |    11 |         8 |       0 |               3 |

## B03 — Integrasi Asuransi

<a id="f-b03-01"></a>

### [B03.01](B-vendor.md#f-b03-01) — Garda Medika: garda_dischargeresult, garda_claimpaymentresult, garda_uploadinvoiceresult

Release status `PROD` (as of 2026-11-12) · 3 stories · 33 test cases · 26 automated (79%)

| Story          | Epic                                                          | Story text                                                              | Cases | Automated | Planned | Blocked / other |
| -------------- | ------------------------------------------------------------- | ----------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCGARDA-01 | [VOCGARDA](../test-catalog/vendor-backend/insurance/garda.md) | Terima hasil discharge dari Sinarmas Garda (source: `US-V-OC-GARDA-01`) |    15 |        12 |       0 |               3 |
| US-VOCGARDA-02 | [VOCGARDA](../test-catalog/vendor-backend/insurance/garda.md) | Terima konfirmasi pembayaran klaim (source: `US-V-OC-GARDA-02`)         |     9 |         7 |       0 |               2 |
| US-VOCGARDA-03 | [VOCGARDA](../test-catalog/vendor-backend/insurance/garda.md) | Terima konfirmasi upload invoice (source: `US-V-OC-GARDA-03`)           |     9 |         7 |       0 |               2 |

<a id="f-b03-02"></a>

### [B03.02](B-vendor.md#f-b03-02) — Sinarmas (inbound): jawabanasuransi, jawabpersetujuantindakan, biayaakhir, permintaanasuransi, getbillingreg

Release status `PROD` (as of 2026-11-12) · 17 stories · 49 test cases · 0 automated (0%)

| Story        | Epic                                                           | Story text                                                                                    | Cases | Automated | Planned | Blocked / other |
| ------------ | -------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCSIN-01 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Keputusan otorisasi asuransi diterima dan disimpan (source: `US-V-OC-SINARMAS-01`)            |     5 |         0 |       0 |               5 |
| US-VOCSIN-02 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Dokumen pendukung otorisasi diarsip ke MinIO (source: `US-V-OC-SINARMAS-02`)                  |     3 |         0 |       0 |               3 |
| US-VOCSIN-03 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Payload tidak valid ditolak dengan kode yang sesuai (source: `US-V-OC-SINARMAS-03`)           |     5 |         0 |       0 |               5 |
| US-VOCSIN-04 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Replay webhook bersifat idempotent (source: `US-V-OC-SINARMAS-04`)                            |     1 |         0 |       0 |               1 |
| US-VOCSIN-05 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Endpoint Sinarmas memerlukan autentikasi (source: `US-V-OC-SINARMAS-05`)                      |     1 |         0 |       0 |               1 |
| US-VOCSIN-06 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Tindakan medis disetujui/ditolak per item (source: `US-V-OC-SINARMAS-06`)                     |     3 |         0 |       0 |               3 |
| US-VOCSIN-07 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Validasi payload JawabPersetujuanTindakan (source: `US-V-OC-SINARMAS-07`)                     |     5 |         0 |       0 |               5 |
| US-VOCSIN-08 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Isolasi registrasi pada tindakan medis (source: `US-V-OC-SINARMAS-08`)                        |     1 |         0 |       0 |               1 |
| US-VOCSIN-09 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Replay JawabPersetujuanTindakan bersifat idempotent (source: `US-V-OC-SINARMAS-09`)           |     1 |         0 |       0 |               1 |
| US-VOCSIN-10 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Billing akhir rawat inap disimpan dengan is_latest yang benar (source: `US-V-OC-SINARMAS-10`) |     4 |         0 |       0 |               4 |
| US-VOCSIN-11 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Validasi BiayaAkhir (source: `US-V-OC-SINARMAS-11`)                                           |     3 |         0 |       0 |               3 |
| US-VOCSIN-12 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Permohonan otorisasi inbound dari Sinarmas tersimpan (source: `US-V-OC-SINARMAS-12`)          |     2 |         0 |       0 |               2 |
| US-VOCSIN-13 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Validasi PermintaanAsuransi (source: `US-V-OC-SINARMAS-13`)                                   |     4 |         0 |       0 |               4 |
| US-VOCSIN-14 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Rincian billing rawat inap tersedia untuk Sinarmas (source: `US-V-OC-SINARMAS-14`)            |     1 |         0 |       0 |               1 |
| US-VOCSIN-15 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | getbillingreg menolak noreg tidak valid (source: `US-V-OC-SINARMAS-15`)                       |     6 |         0 |       0 |               6 |
| US-VOCSIN-16 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Format tanggal dalam response getbillingreg benar (source: `US-V-OC-SINARMAS-16`)             |     1 |         0 |       0 |               1 |
| US-VOCSIN-17 | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | Keamanan getbillingreg — autentikasi dan validasi rs (source: `US-V-OC-SINARMAS-17`)          |     3 |         0 |       0 |               3 |

## B04 — Kiosk HIS v2

<a id="f-b04-01"></a>

### [B04.01](B-vendor.md#f-b04-01) — Konten & Pengaturan: settings, packages, excellence, network, info, assets, home/stats, smart-search

Release status `DEV-DONE` (as of 2026-11-12) · 8 stories · 42 test cases · 0 automated (0%)

| Story        | Epic                                                      | Story text                                                       | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------------- | ---------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKCONT-01 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Pengaturan sistem kiosk (source: `US-V-KIOSK-CONTENT-01`)        |     2 |         0 |       2 |               0 |
| US-VKCONT-02 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Paket layanan berdasarkan tipe (source: `US-V-KIOSK-CONTENT-02`) |     9 |         0 |       9 |               0 |
| US-VKCONT-03 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Konten excellence center (source: `US-V-KIOSK-CONTENT-03`)       |     6 |         0 |       5 |               1 |
| US-VKCONT-04 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Network images (source: `US-V-KIOSK-CONTENT-04`)                 |     6 |         0 |       6 |               0 |
| US-VKCONT-05 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Informasi hak pasien (source: `US-V-KIOSK-CONTENT-05`)           |     5 |         0 |       5 |               0 |
| US-VKCONT-06 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Statistik home dan live queue (source: `US-V-KIOSK-CONTENT-06`)  |     4 |         0 |       4 |               0 |
| US-VKCONT-07 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Smart search keyword (source: `US-V-KIOSK-CONTENT-07`)           |     6 |         0 |       6 |               0 |
| US-VKCONT-08 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md) | Embedded static assets (source: `US-V-KIOSK-CONTENT-08`)         |     4 |         0 |       4 |               0 |

<a id="f-b04-02"></a>

### [B04.02](B-vendor.md#f-b04-02) — Pasien & Kunjungan: patients (register/search/get), visits, appointments, checkin, tickets

Release status `DEV-DONE` (as of 2026-11-12) · 7 stories · 67 test cases · 0 automated (0%)

| Story        | Epic                                                          | Story text                                                                    | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | ----------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKAPPT-01 | [VKAPPT](../test-catalog/vendor-backend/kiosk/appointment.md) | Walk-in visit (source: `US-V-KIOSK-APPT-01`)                                  |    13 |         0 |      12 |               1 |
| US-VKAPPT-02 | [VKAPPT](../test-catalog/vendor-backend/kiosk/appointment.md) | Booking appointment advance (source: `US-V-KIOSK-APPT-02`)                    |    12 |         0 |      11 |               1 |
| US-VKAPPT-03 | [VKAPPT](../test-catalog/vendor-backend/kiosk/appointment.md) | Check-in dengan booking code (source: `US-V-KIOSK-APPT-03`)                   |     8 |         0 |       8 |               0 |
| US-VKAPPT-04 | [VKAPPT](../test-catalog/vendor-backend/kiosk/appointment.md) | Lihat tiket appointment pasien (source: `US-V-KIOSK-APPT-04`)                 |     5 |         0 |       3 |               2 |
| US-VKPAT-01  | [VKPAT](../test-catalog/vendor-backend/kiosk/patient.md)      | Pendaftaran pasien baru (source: `US-V-KIOSK-PATIENT-01`)                     |    16 |         0 |      16 |               0 |
| US-VKPAT-02  | [VKPAT](../test-catalog/vendor-backend/kiosk/patient.md)      | Pencarian pasien untuk verifikasi identitas (source: `US-V-KIOSK-PATIENT-02`) |     9 |         0 |       9 |               0 |
| US-VKPAT-03  | [VKPAT](../test-catalog/vendor-backend/kiosk/patient.md)      | Fetch profil lengkap pasien by MRN (source: `US-V-KIOSK-PATIENT-03`)          |     4 |         0 |       4 |               0 |

<a id="f-b04-03"></a>

### [B04.03](B-vendor.md#f-b04-03) — Dokter & Jadwal: doctors, slots, dates, on-leave

Release status `DEV-DONE` (as of 2026-11-12) · 4 stories · 21 test cases · 0 automated (0%)

| Story         | Epic                                                        | Story text                                                | Cases | Automated | Planned | Blocked / other |
| ------------- | ----------------------------------------------------------- | --------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKSCHED-01 | [VKSCHED](../test-catalog/vendor-backend/kiosk/schedule.md) | Daftar dokter tersedia (source: `US-V-KIOSK-SCHED-01`)    |     7 |         0 |       6 |               1 |
| US-VKSCHED-02 | [VKSCHED](../test-catalog/vendor-backend/kiosk/schedule.md) | Slot waktu dokter (source: `US-V-KIOSK-SCHED-02`)         |     8 |         0 |       7 |               1 |
| US-VKSCHED-03 | [VKSCHED](../test-catalog/vendor-backend/kiosk/schedule.md) | Kalender tersedia 15 hari (source: `US-V-KIOSK-SCHED-03`) |     4 |         0 |       4 |               0 |
| US-VKSCHED-04 | [VKSCHED](../test-catalog/vendor-backend/kiosk/schedule.md) | Dokter cuti hari ini (source: `US-V-KIOSK-SCHED-04`)      |     2 |         0 |       2 |               0 |

<a id="f-b04-04"></a>

### [B04.04](B-vendor.md#f-b04-04) — BPJS / VClaim: rujukan, cari-nik, surat-kontrol, vclaim/check

Release status `IN-PROGRESS` · 4 stories · 23 test cases · 0 automated (0%)

| Story        | Epic                                                     | Story text                                                              | Cases | Automated | Planned | Blocked / other |
| ------------ | -------------------------------------------------------- | ----------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKMOCK-02 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md)   | BPJS rujukan mock (source: `US-V-KIOSK-MOCK-02`)                        |     4 |         0 |       4 |               0 |
| US-VKMOCK-03 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md)   | BPJS cari NIK mock (source: `US-V-KIOSK-MOCK-03`)                       |     5 |         0 |       5 |               0 |
| US-VKMOCK-04 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md)   | BPJS surat kontrol dan VClaim check mock (source: `US-V-KIOSK-MOCK-04`) |     9 |         0 |       9 |               0 |
| US-VKPAT-04  | [VKPAT](../test-catalog/vendor-backend/kiosk/patient.md) | Pendaftaran BPJS (source: `US-V-KIOSK-PATIENT-04`)                      |     5 |         0 |       4 |               1 |

<a id="f-b04-05"></a>

### [B04.05](B-vendor.md#f-b04-05) — Farmasi: pharmacy/status (real); pharmacy/delivery (mock)

Release status `DEV-DONE` (as of 2026-11-12) · 2 stories · 24 test cases · 0 automated (0%)

| Story        | Epic                                                            | Story text                                                 | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------------------- | ---------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKREAD-01 | [VKREAD](../test-catalog/vendor-backend/kiosk/clinical-read.md) | Status apotek (source: `US-V-KIOSK-CLINREAD-01`)           |    19 |         0 |      18 |               1 |
| US-VKMOCK-07 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md)          | Pharmacy delivery mock echo (source: `US-V-KIOSK-MOCK-07`) |     5 |         0 |       5 |               0 |

<a id="f-b04-06"></a>

### [B04.06](B-vendor.md#f-b04-06) — Billing & Pembayaran: billing/search, billing/{regId}; payment/create/check/callback (mock Duitku)

Release status `DEV-DONE` (as of 2026-11-12) · 4 stories · 33 test cases · 0 automated (0%)

| Story        | Epic                                                            | Story text                                                                                                 | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKREAD-02 | [VKREAD](../test-catalog/vendor-backend/kiosk/clinical-read.md) | Cari dan lihat tagihan (source: `US-V-KIOSK-CLINREAD-02`)                                                  |    15 |         0 |      11 |               4 |
| US-VKMOCK-01 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md)          | Duitku callback merchantCode guard (source: `US-V-KIOSK-MOCK-01`)                                          |     8 |         0 |       8 |               0 |
| US-VKMOCK-05 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md)          | Payment create mock (503 hanya untuk REG-DUITKU-DOWN; 422 bila total salah) (source: `US-V-KIOSK-MOCK-05`) |     6 |         0 |       6 |               0 |
| US-VKMOCK-06 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md)          | Payment check mock (502 hanya untuk MOID-GATEWAY-DOWN) (source: `US-V-KIOSK-MOCK-06`)                      |     4 |         0 |       4 |               0 |

<a id="f-b04-07"></a>

### [B04.07](B-vendor.md#f-b04-07) — Klinis Read: lab/results (lab & rad), queue/live

Release status `DEV-DONE` (as of 2026-11-12) · 2 stories · 15 test cases · 0 automated (0%)

| Story        | Epic                                                            | Story text                                                      | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------------------- | --------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKREAD-03 | [VKREAD](../test-catalog/vendor-backend/kiosk/clinical-read.md) | Lihat hasil laboratorium (source: `US-V-KIOSK-CLINREAD-03`)     |    11 |         0 |       7 |               4 |
| US-VKCONT-06 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md)       | Statistik home dan live queue (source: `US-V-KIOSK-CONTENT-06`) |     4 |         0 |       4 |               0 |

<a id="f-b04-08"></a>

### [B04.08](B-vendor.md#f-b04-08) — Officer Call: POST/GET/PATCH officer-calls, events (scope officer-call:operate)

Release status `DEV-DONE` (as of 2026-11-12) · 9 stories · 59 test cases · 0 automated (0%)

| Story        | Epic                                                          | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKAUTH-05 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md)        | Isolasi scope kiosk vs officer (source: `US-V-KIOSK-AUTH-05`)                                                  |    11 |         0 |       6 |               5 |
| US-VKOFF-01  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | Membuat officer call dari kiosk (source: `US-V-KIOSK-OFFICER-01`)                                              |    11 |         0 |       9 |               2 |
| US-VKOFF-02  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | Melihat antrian panggilan (officer) (source: `US-V-KIOSK-OFFICER-02`)                                          |     6 |         0 |       6 |               0 |
| US-VKOFF-03  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | State machine officer call (source: `US-V-KIOSK-OFFICER-03`)                                                   |    12 |         0 |      12 |               0 |
| US-VKOFF-04  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | Concurrent claim race condition (source: `US-V-KIOSK-OFFICER-04`)                                              |     4 |         0 |       4 |               0 |
| US-VKOFF-05  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | Cancel panggilan (source: `US-V-KIOSK-OFFICER-05`)                                                             |     2 |         0 |       2 |               0 |
| US-VKOFF-06  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | Kiosk melihat call sendiri (source: `US-V-KIOSK-OFFICER-06`)                                                   |     6 |         0 |       6 |               0 |
| US-VKOFF-07  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | Event outbox polling (source: `US-V-KIOSK-OFFICER-07`)                                                         |     6 |         0 |       6 |               0 |
| US-VKOFF-08  | [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md) | Not described in USER-STORIES.md of the source cluster; referenced by 1 case(s), e.g. V-KIOSK-OFFICER-patch-N… |     1 |         0 |       1 |               0 |

<a id="f-b04-09"></a>

### [B04.09](B-vendor.md#f-b04-09) — Keamanan: JWT Bearer scope kiosk:invoke

Release status `DEV-DONE` (as of 2026-11-12) · 6 stories · 45 test cases · 0 automated (0%)

| Story        | Epic                                                   | Story text                                                                       | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------ | -------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VKAUTH-01 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md) | Kiosk gate — akses sukses dengan credential valid (source: `US-V-KIOSK-AUTH-01`) |     7 |         0 |       6 |               1 |
| US-VKAUTH-02 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md) | Kiosk gate — penolakan credential rusak (source: `US-V-KIOSK-AUTH-02`)           |    21 |         0 |      21 |               0 |
| US-VKAUTH-03 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md) | Route publik tidak memerlukan credential (source: `US-V-KIOSK-AUTH-03`)          |     3 |         0 |       3 |               0 |
| US-VKAUTH-04 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md) | 404/405 tidak bocorkan gate auth (source: `US-V-KIOSK-AUTH-04`)                  |     2 |         0 |       2 |               0 |
| US-VKAUTH-05 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md) | Isolasi scope kiosk vs officer (source: `US-V-KIOSK-AUTH-05`)                    |    11 |         0 |       6 |               5 |
| US-VKAUTH-06 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md) | Legacy v0 token compatibility (source: `US-V-KIOSK-AUTH-06`)                     |     1 |         0 |       1 |               0 |

## B05 — VTT — Dictation to EMR

<a id="f-b05-01"></a>

### [B05.01](B-vendor.md#f-b05-01) — Lookup: patients, practitioners, master-item (staging)

Release status `DEV-DONE` (as of 2026-11-12) · 5 stories · 34 test cases · 0 automated (0%)

| Story         | Epic                                                    | Story text                                                            | Cases | Automated | Planned | Blocked / other |
| ------------- | ------------------------------------------------------- | --------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VTTREAD-01 | [VTTREAD](../test-catalog/vendor-backend/vtt/lookup.md) | Cari pasien berdasarkan nomor registrasi (source: `US-V-VTT-READ-01`) |    10 |         0 |      10 |               0 |
| US-VTTREAD-02 | [VTTREAD](../test-catalog/vendor-backend/vtt/lookup.md) | Cari pasien dengan filter dokter (source: `US-V-VTT-READ-02`)         |     4 |         0 |       4 |               0 |
| US-VTTREAD-03 | [VTTREAD](../test-catalog/vendor-backend/vtt/lookup.md) | Cari practitioner aktif (source: `US-V-VTT-READ-03`)                  |     8 |         0 |       7 |               1 |
| US-VTTREAD-04 | [VTTREAD](../test-catalog/vendor-backend/vtt/lookup.md) | Cari master item farmasi (source: `US-V-VTT-READ-04`)                 |     9 |         0 |       9 |               0 |
| US-VTTREAD-05 | [VTTREAD](../test-catalog/vendor-backend/vtt/lookup.md) | Cari item berdasarkan kode item (source: `US-V-VTT-READ-05`)          |     3 |         0 |       3 |               0 |

<a id="f-b05-02"></a>

### [B05.02](B-vendor.md#f-b05-02) — SOAP: POST/GET soap, audio stream (retensi 3 hari)

Release status `DEV-DONE` (as of 2026-11-12) · 4 stories · 30 test cases · 0 automated (0%)

| Story         | Epic                                                  | Story text                                                             | Cases | Automated | Planned | Blocked / other |
| ------------- | ----------------------------------------------------- | ---------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VTTSOAP-01 | [VTTSOAP](../test-catalog/vendor-backend/vtt/soap.md) | Submit dikte SOAP dengan audio (source: `US-V-VTT-SOAP-01`)            |     8 |         0 |       7 |               1 |
| US-VTTSOAP-02 | [VTTSOAP](../test-catalog/vendor-backend/vtt/soap.md) | Submit dikte SOAP tanpa audio (text only) (source: `US-V-VTT-SOAP-02`) |     5 |         0 |       5 |               0 |
| US-VTTSOAP-03 | [VTTSOAP](../test-catalog/vendor-backend/vtt/soap.md) | Lihat riwayat submission SOAP (source: `US-V-VTT-SOAP-03`)             |     9 |         0 |       8 |               1 |
| US-VTTSOAP-04 | [VTTSOAP](../test-catalog/vendor-backend/vtt/soap.md) | Stream audio submission (206 Range) (source: `US-V-VTT-SOAP-04`)       |     8 |         0 |       8 |               0 |

<a id="f-b05-03"></a>

### [B05.03](B-vendor.md#f-b05-03) — EMR Update: soap/emr/{emrID}, emr-with-actions (buat/hapus service request)

Release status `DEV-DONE` (as of 2026-11-12) · 3 stories · 19 test cases · 0 automated (0%)

| Story        | Epic                                                | Story text                                                                                  | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------- | ------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VTTEMR-01 | [VTTEMR](../test-catalog/vendor-backend/vtt/emr.md) | Update bagian EMR SOAP (draft) (source: `US-V-VTT-EMR-01`)                                  |     7 |         0 |       7 |               0 |
| US-VTTEMR-02 | [VTTEMR](../test-catalog/vendor-backend/vtt/emr.md) | EMR tidak boleh dioverwrite setelah final (DEF-350) (source: `US-V-VTT-EMR-02`)             |     5 |         0 |       2 |               3 |
| US-VTTEMR-03 | [VTTEMR](../test-catalog/vendor-backend/vtt/emr.md) | Update EMR + service requests dalam satu operasi (with-actions) (source: `US-V-VTT-EMR-03`) |     7 |         0 |       6 |               1 |

<a id="f-b05-04"></a>

### [B05.04](B-vendor.md#f-b05-04) — Service Request: general-dropdown, requests (POST/GET)

Release status `DEV-DONE` (as of 2026-11-12) · 2 stories · 18 test cases · 0 automated (0%)

| Story        | Epic                                                | Story text                                                                   | Cases | Automated | Planned | Blocked / other |
| ------------ | --------------------------------------------------- | ---------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VTTEMR-04 | [VTTEMR](../test-catalog/vendor-backend/vtt/emr.md) | Lihat dropdown layanan untuk kunjungan (source: `US-V-VTT-EMR-04`)           |     8 |         0 |       8 |               0 |
| US-VTTEMR-05 | [VTTEMR](../test-catalog/vendor-backend/vtt/emr.md) | Buat dan lihat service requests (tindakan medis) (source: `US-V-VTT-EMR-05`) |    10 |         0 |       7 |               3 |

## B06 — Teknis & Monitoring (his-vendor-backend)

<a id="f-b06-01"></a>

### [B06.01](B-vendor.md#f-b06-01) — Liveness: `GET /service-status` tanpa auth: HTTP 200, JSON, tanpa stack trace / detail internal

Release status `PROD` (as of 2026-11-12) · 1 stories · 2 test cases · 0 automated (0%)

| Story      | Epic                                             | Story text                                            | Cases | Automated | Planned | Blocked / other |
| ---------- | ------------------------------------------------ | ----------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VSYS-01 | [VSYS](../test-catalog/vendor-backend/system.md) | Liveness probe service-status (source: `US-V-SYS-01`) |     2 |         0 |       2 |               0 |

<a id="f-b06-02"></a>

### [B06.02](B-vendor.md#f-b06-02) — Dokumentasi API: Swagger UI `/swagger/*` (index.html, swagger.json); path tak dikenal → 404 tanpa panic

Release status `PROD` (as of 2026-11-12) · 1 stories · 2 test cases · 0 automated (0%)

| Story      | Epic                                             | Story text                                                  | Cases | Automated | Planned | Blocked / other |
| ---------- | ------------------------------------------------ | ----------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VSYS-02 | [VSYS](../test-catalog/vendor-backend/system.md) | Swagger UI tersedia untuk developer (source: `US-V-SYS-02`) |     2 |         0 |       2 |               0 |

<a id="f-b06-03"></a>

### [B06.03](B-vendor.md#f-b06-03) — Router: Route atau method tidak valid → 404/405 bersih tanpa bocoran internal (panic, goroutine, SQL, path file)

Release status `PROD` (as of 2026-11-12) · 1 stories · 4 test cases · 0 automated (0%)

| Story      | Epic                                             | Story text                                                                           | Cases | Automated | Planned | Blocked / other |
| ---------- | ------------------------------------------------ | ------------------------------------------------------------------------------------ | ----: | --------: | ------: | --------------: |
| US-VSYS-03 | [VSYS](../test-catalog/vendor-backend/system.md) | Router mengembalikan 404/405 yang bersih untuk route invalid (source: `US-V-SYS-03`) |     4 |         0 |       4 |               0 |

## D01 — CIS & CCM (master korporat → HIS)

<a id="f-d01-01"></a>

### [D01.01](D-corporate.md#f-d01-01) — CIS: 33 aksi `?name=` + REST `/{slug}/{verb}` (care-class, department, service-group/type, service, module, menu, access-right, user-group, user, formula)

Release status `IN-PROGRESS` · 3 stories · 101 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                                | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | --------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DCIS-01 | [DCIS](../test-catalog/data-integration/cis.md) | Siklus Hidup Entitas CIS per Slug (source: `US-D-CIS-01`) |    86 |         0 |      24 |              62 |
| US-DCIS-03 | [DCIS](../test-catalog/data-integration/cis.md) | Legacy Dispatcher ?name= (source: `US-D-CIS-03`)          |     9 |         0 |       3 |               6 |
| US-DCIS-04 | [DCIS](../test-catalog/data-integration/cis.md) | Verifikasi Token (source: `US-D-CIS-04`)                  |     6 |         0 |       1 |               5 |

<a id="f-d01-02"></a>

### [D01.02](D-corporate.md#f-d01-02) — CIS: Rejection departemen/layanan/pengajuan

Release status `IN-PROGRESS` · 1 stories · 7 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                 | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | ------------------------------------------ | ----: | --------: | ------: | --------------: |
| US-DCIS-02 | [DCIS](../test-catalog/data-integration/cis.md) | Rejection Endpoint (source: `US-D-CIS-02`) |     7 |         0 |       2 |               5 |

<a id="f-d01-03"></a>

### [D01.03](D-corporate.md#f-d01-03) — Master Lab: specimen-type, analyzer, method, material, specimen, lab-service

Release status `IN-PROGRESS` · 3 stories · 48 test cases · 0 automated (0%)

| Story         | Epic                                                   | Story text                                                    | Cases | Automated | Planned | Blocked / other |
| ------------- | ------------------------------------------------------ | ------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DCISLAB-01 | [DCISLAB](../test-catalog/data-integration/cis-lab.md) | Siklus Hidup Entitas Lab per Slug (source: `US-D-CIS-LAB-01`) |    35 |         0 |       7 |              28 |
| US-DCISLAB-02 | [DCISLAB](../test-catalog/data-integration/cis-lab.md) | Verifikasi Token Lab (source: `US-D-CIS-LAB-02`)              |     6 |         0 |       2 |               4 |
| US-DCISLAB-03 | [DCISLAB](../test-catalog/data-integration/cis-lab.md) | Legacy Dispatcher ?name= Lab (source: `US-D-CIS-LAB-03`)      |     7 |         0 |       2 |               5 |

<a id="f-d01-04"></a>

### [D01.04](D-corporate.md#f-d01-04) — Master Lookup: list, one, dropdown, doctor autocomplete, user check/save

Release status `IN-PROGRESS` · 4 stories · 50 test cases · 0 automated (0%)

| Story        | Epic                                                     | Story text                                                                      | Cases | Automated | Planned | Blocked / other |
| ------------ | -------------------------------------------------------- | ------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DCISMS-01 | [DCISMS](../test-catalog/data-integration/cis-master.md) | Baca Daftar Master Entitas via list/one/dropdown (source: `US-D-CIS-MASTER-01`) |    23 |         0 |       5 |              18 |
| US-DCISMS-02 | [DCISMS](../test-catalog/data-integration/cis-master.md) | Autocomplete Dokter (source: `US-D-CIS-MASTER-02`)                              |     4 |         0 |       1 |               3 |
| US-DCISMS-03 | [DCISMS](../test-catalog/data-integration/cis-master.md) | Cek dan Simpan User CIS (source: `US-D-CIS-MASTER-03`)                          |    10 |         0 |       3 |               7 |
| US-DCISMS-04 | [DCISMS](../test-catalog/data-integration/cis-master.md) | Legacy Dispatcher ?name= (master_cis.php) (source: `US-D-CIS-MASTER-04`)        |    13 |         0 |       4 |               9 |

<a id="f-d01-05"></a>

### [D01.05](D-corporate.md#f-d01-05) — CCM: Channel `/channel/{verb}` + deploy_api.php; ±97 REST API CCM

Release status `IN-PROGRESS` · 2 stories · 26 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                       | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | ------------------------------------------------ | ----: | --------: | ------: | --------------: |
| US-DCCM-01 | [DCCM](../test-catalog/data-integration/ccm.md) | Siklus Hidup Channel CCM (source: `US-D-CCM-01`) |    20 |         0 |       4 |              16 |
| US-DCCM-02 | [DCCM](../test-catalog/data-integration/ccm.md) | Verifikasi Token Channel (source: `US-D-CCM-02`) |     6 |         0 |       2 |               4 |

## D02 — CCP (Central Purchasing)

<a id="f-d02-01"></a>

### [D02.01](D-corporate.md#f-d02-01) — Master Item CCP: deploy/item/{drug,bmhp,alkes}, deploy/mou

Release status `PROD` (as of 2026-11-12) · 2 stories · 19 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                                 | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | ---------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DCCP-01 | [DCCP](../test-catalog/data-integration/ccp.md) | Deploy Master Item Obat/BMHP/Alkes (source: `US-D-CCP-01`) |    15 |         0 |       8 |               7 |
| US-DCCP-02 | [DCCP](../test-catalog/data-integration/ccp.md) | Deploy MOU (source: `US-D-CCP-02`)                         |     4 |         0 |       4 |               0 |

<a id="f-d02-02"></a>

### [D02.02](D-corporate.md#f-d02-02) — Status: download/item/status, pengajuan/item/status

Release status `PROD` (as of 2026-11-12) · 1 stories · 13 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                                   | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | ------------------------------------------------------------ | ----: | --------: | ------: | --------------: |
| US-DCCP-03 | [DCCP](../test-catalog/data-integration/ccp.md) | Update Status Download dan Pengajuan (source: `US-D-CCP-03`) |    13 |         0 |      10 |               3 |

<a id="f-d02-03"></a>

### [D02.03](D-corporate.md#f-d02-03) — Stub: /ccp/apifile/ccpapi.php, /ccp/poapi/poapi.php

Release status `PROD` (as of 2026-11-12) · 1 stories · 4 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                                     | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | -------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DCCP-04 | [DCCP](../test-catalog/data-integration/ccp.md) | Stub Endpoints (ccpapi.php, poapi.php) (source: `US-D-CCP-04`) |     4 |         0 |       4 |               0 |

## D03 — CRM (Primaya LMS)

<a id="f-d03-01"></a>

### [D03.01](D-corporate.md#f-d03-01) — Perusahaan / Payor: Deploy master perusahaan, payor, dan asuransi (upsert: kode, jenis, penjamin, diskon, PPN, kontak, bank)

Release status `PROD` (as of 2026-11-12) · 3 stories · 11 test cases · 0 automated (0%)

| Story        | Epic                                            | Story text                                                                        | Cases | Automated | Planned | Blocked / other |
| ------------ | ----------------------------------------------- | --------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCCRM-01 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Data perusahaan penjamin baru dideploy dari LMS ke HIS (source: `US-V-OC-CRM-01`) |     1 |         0 |       0 |               1 |
| US-VOCCRM-02 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Redeploy perusahaan bersifat idempotent (update mode) (source: `US-V-OC-CRM-02`)  |     1 |         0 |       0 |               1 |
| US-VOCCRM-03 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Validasi crm_deployperusahaan (source: `US-V-OC-CRM-03`)                          |     9 |         0 |       0 |               9 |

<a id="f-d03-02"></a>

### [D03.02](D-corporate.md#f-d03-02) — Tarif Perusahaan: Deploy kontrak tarif perusahaan (periode berlaku, kelas, cito, cost sharing, diskon, SOC)

Release status `PROD` (as of 2026-11-12) · 3 stories · 11 test cases · 0 automated (0%)

| Story        | Epic                                            | Story text                                                             | Cases | Automated | Planned | Blocked / other |
| ------------ | ----------------------------------------------- | ---------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCCRM-04 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Tarif kontrak baru dideploy dari LMS ke HIS (source: `US-V-OC-CRM-04`) |     1 |         0 |       0 |               1 |
| US-VOCCRM-05 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Redeploy tarif bersifat idempotent (source: `US-V-OC-CRM-05`)          |     1 |         0 |       0 |               1 |
| US-VOCCRM-06 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Validasi crm_deploytarif (source: `US-V-OC-CRM-06`)                    |     9 |         0 |       0 |               9 |

<a id="f-d03-04"></a>

### [D03.04](D-corporate.md#f-d03-04) — Paket: Deploy master paket (layanan, item, kamar, operasi, pembulatan)

Release status `DEV-DONE` (as of 2026-11-12) · 3 stories · 10 test cases · 0 automated (0%)

| Story        | Epic                                            | Story text                                                             | Cases | Automated | Planned | Blocked / other |
| ------------ | ----------------------------------------------- | ---------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-VOCCRM-07 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Paket layanan baru dideploy dari LMS ke HIS (source: `US-V-OC-CRM-07`) |     1 |         0 |       0 |               1 |
| US-VOCCRM-08 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Redeploy paket bersifat idempotent (source: `US-V-OC-CRM-08`)          |     1 |         0 |       0 |               1 |
| US-VOCCRM-09 | [VOCCRM](../test-catalog/vendor-backend/crm.md) | Validasi crm_deploypaket (source: `US-V-OC-CRM-09`)                    |     8 |         0 |       0 |               8 |

## D04 — EIS

<a id="f-d04-01"></a>

### [D04.01](D-corporate.md#f-d04-01) — Laporan Jadwal Cuti Dokter: `/eis/laporan-jadwal-cuti-dokter` (JSON/Excel/HTML, x-secret-key)

Release status `DEV-DONE` (as of 2026-11-12) · 2 stories · 17 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                                      | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | --------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DEIS-01 | [DEIS](../test-catalog/data-integration/eis.md) | Laporan jadwal cuti dokter multi-format (source: `US-D-EIS-01`) |    13 |         0 |       0 |              13 |
| US-DEIS-02 | [DEIS](../test-catalog/data-integration/eis.md) | Keamanan akses laporan dokter (source: `US-D-EIS-02`)           |     4 |         0 |       4 |               0 |

## D06 — AIS (Akuntansi)

<a id="f-d06-01"></a>

### [D06.01](D-corporate.md#f-d06-01) — AIS: recalculate, recalculate/bulk, hondok (publish ke RabbitMQ)

Release status `PROD` (as of 2026-11-12) · 3 stories · 26 test cases · 0 automated (0%)

| Story      | Epic                                            | Story text                                                   | Cases | Automated | Planned | Blocked / other |
| ---------- | ----------------------------------------------- | ------------------------------------------------------------ | ----: | --------: | ------: | --------------: |
| US-DAIS-01 | [DAIS](../test-catalog/data-integration/ais.md) | Recalculate AIS untuk satu encounter (source: `US-D-AIS-01`) |     9 |         0 |       6 |               3 |
| US-DAIS-02 | [DAIS](../test-catalog/data-integration/ais.md) | Bulk recalculate AIS (source: `US-D-AIS-02`)                 |     7 |         0 |       4 |               3 |
| US-DAIS-03 | [DAIS](../test-catalog/data-integration/ais.md) | Update billing hondok (honor dokter) (source: `US-D-AIS-03`) |    10 |         0 |       7 |               3 |

## D07 — Hardware & Alat Lab

<a id="f-d07-01"></a>

### [D07.01](D-corporate.md#f-d07-01) — Mindray: search-patient, store-mindray, store-mindray-intensive (vital sign)

Release status `PROD` (as of 2026-11-12) · 3 stories · 13 test cases · 0 automated (0%)

| Story        | Epic                                                         | Story text                                                                  | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------ | --------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DHWVIT-01 | [DHWVIT](../test-catalog/data-integration/hardware-vital.md) | Pencarian Pasien oleh Perangkat Mindray (source: `US-D-HW-VITAL-01`)        |     6 |         0 |       2 |               4 |
| US-DHWVIT-02 | [DHWVIT](../test-catalog/data-integration/hardware-vital.md) | Penyimpanan Data Mindray (PHI Disclosure Risk) (source: `US-D-HW-VITAL-02`) |     1 |         0 |       0 |               1 |
| US-DHWVIT-03 | [DHWVIT](../test-catalog/data-integration/hardware-vital.md) | Penyimpanan Vital Sign Mindray Rawat Inap (source: `US-D-HW-VITAL-03`)      |     6 |         0 |       0 |               6 |

<a id="f-d07-02"></a>

### [D07.02](D-corporate.md#f-d07-02) — Medscape: store-vital-sign

Release status `PROD` (as of 2026-11-12) · 1 stories · 6 test cases · 0 automated (0%)

| Story        | Epic                                                         | Story text                                                          | Cases | Automated | Planned | Blocked / other |
| ------------ | ------------------------------------------------------------ | ------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DHWVIT-04 | [DHWVIT](../test-catalog/data-integration/hardware-vital.md) | Penyimpanan Vital Sign Medscape via QR (source: `US-D-HW-VITAL-04`) |     6 |         0 |       2 |               4 |

<a id="f-d07-03"></a>

### [D07.03](D-corporate.md#f-d07-03) — Lab Analyzer: result, raw, order, update_order

Release status `PROD` (as of 2026-11-12) · 5 stories · 23 test cases · 0 automated (0%)

| Story        | Epic                                                       | Story text                                                                        | Cases | Automated | Planned | Blocked / other |
| ------------ | ---------------------------------------------------------- | --------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DHWLAB-01 | [DHWLAB](../test-catalog/data-integration/hardware-lab.md) | Submit Hasil Laboratorium dari Analyzer (Patient Path) (source: `US-D-HW-LAB-01`) |    15 |         0 |      10 |               5 |
| US-DHWLAB-02 | [DHWLAB](../test-catalog/data-integration/hardware-lab.md) | Submit Data QC Analyzer (source: `US-D-HW-LAB-02`)                                |     1 |         0 |       1 |               0 |
| US-DHWLAB-03 | [DHWLAB](../test-catalog/data-integration/hardware-lab.md) | Query Order Lab oleh Analyzer (source: `US-D-HW-LAB-03`)                          |     3 |         0 |       2 |               1 |
| US-DHWLAB-04 | [DHWLAB](../test-catalog/data-integration/hardware-lab.md) | Submit Raw Data Analyzer (source: `US-D-HW-LAB-04`)                               |     2 |         0 |       2 |               0 |
| US-DHWLAB-05 | [DHWLAB](../test-catalog/data-integration/hardware-lab.md) | Acknowledge Update Order Status (Stub) (source: `US-D-HW-LAB-05`)                 |     2 |         0 |       1 |               1 |

## D08 — Westerindo dan Sistem Approval

<a id="f-d08-01"></a>

### [D08.01](D-corporate.md#f-d08-01) — Westerindo Lab Outref: trigger-fetch hasil lab rujukan keluar

Release status `PROD` (as of 2026-11-12) · 3 stories · 13 test cases · 0 automated (0%)

| Story         | Epic                                                      | Story text                                                                   | Cases | Automated | Planned | Blocked / other |
| ------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DLABREF-01 | [DLABREF](../test-catalog/data-integration/lab-outref.md) | Re-fetch Hasil Rujukan dengan Token Valid (source: `US-D-LABOUTREF-01`)      |     2 |         0 |       2 |               0 |
| US-DLABREF-02 | [DLABREF](../test-catalog/data-integration/lab-outref.md) | Penolakan Request Tanpa Token atau Token Salah (source: `US-D-LABOUTREF-02`) |     7 |         0 |       5 |               2 |
| US-DLABREF-03 | [DLABREF](../test-catalog/data-integration/lab-outref.md) | Error Handling Saat Westerindo Tidak Tersedia (source: `US-D-LABOUTREF-03`)  |     4 |         0 |       2 |               2 |

<a id="f-d08-02"></a>

### [D08.02](D-corporate.md#f-d08-02) — Approval: Halaman setuju/tolak melalui email; persetujuan diskon global, diskon per item, dan paket

Release status `PROD` (as of 2026-11-12) · 7 stories · 26 test cases · 0 automated (0%)

| Story        | Epic                                                  | Story text                                                                       | Cases | Automated | Planned | Blocked / other |
| ------------ | ----------------------------------------------------- | -------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DAPPR-01  | [DAPPR](../test-catalog/data-integration/approval.md) | Approve permintaan via link (source: `US-D-APPROVAL-01`)                         |     6 |         0 |       3 |               3 |
| US-DAPPR-02  | [DAPPR](../test-catalog/data-integration/approval.md) | Reject permintaan via link dengan catatan (source: `US-D-APPROVAL-02`)           |     4 |         0 |       3 |               1 |
| US-DAPPR-03  | [DAPPR](../test-catalog/data-integration/approval.md) | Tampilkan form rejection (source: `US-D-APPROVAL-03`)                            |     3 |         0 |       3 |               0 |
| US-DAPPR-04  | [DAPPR](../test-catalog/data-integration/approval.md) | Keamanan — endpoint approval tidak boleh tanpa auth (source: `US-D-APPROVAL-04`) |     2 |         0 |       0 |               2 |
| US-DONEPG-03 | [DONEPG](../test-catalog/data-integration/onepage.md) | Approve / tolak diskon global (source: `US-D-ONEPAGE-03`)                        |     5 |         0 |       4 |               1 |
| US-DONEPG-04 | [DONEPG](../test-catalog/data-integration/onepage.md) | Approve / tolak diskon per item (source: `US-D-ONEPAGE-04`)                      |     3 |         0 |       2 |               1 |
| US-DONEPG-05 | [DONEPG](../test-catalog/data-integration/onepage.md) | Approve / tolak paket layanan (source: `US-D-ONEPAGE-05`)                        |     3 |         0 |       2 |               1 |

## D09 — Transformer & Single Medrec (Datalake)

<a id="f-d09-01"></a>

### [D09.01](D-corporate.md#f-d09-01) — Transformer API: check-sync-data, data/{res}, regpas (by id/medrec/nik/ihs), resource-options

Release status `PROD` (as of 2026-11-12) · 13 stories · 94 test cases · 50 automated (53%)

| Story        | Epic                                                             | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ------------ | ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-TRSYNC-01 | [TRSYNC](../test-catalog/data-integration/transformer-sync.md)   | As a Transformer consumer I want to page `regpas` in discharge order and resume from the RegID I last process… |    17 |        17 |       0 |               0 |
| US-TRSYNC-02 | [TRSYNC](../test-catalog/data-integration/transformer-sync.md)   | As a Transformer consumer I want to page `medrec` in discharge order and resume from the MEDRECID I last proc… |    18 |        18 |       0 |               0 |
| US-TRSYNC-03 | [TRSYNC](../test-catalog/data-integration/transformer-sync.md)   | As a Transformer consumer I want `regpas` to carry the correct second poli, its doctor and the DPJP poli's di… |     4 |         4 |       0 |               0 |
| US-TRSYNC-04 | [TRSYNC](../test-catalog/data-integration/transformer-sync.md)   | As a Transformer consumer I want `_sync` and `_sync_date` of `medrec` to stay at their defaults, so that lega… |     1 |         1 |       0 |               0 |
| US-TRSYNC-05 | [TRSYNC](../test-catalog/data-integration/transformer-sync.md)   | As an integration engineer I want a predictable request contract (limit, `X-Timezone`, `cursor_value`, tail p… |    10 |        10 |       0 |               0 |
| US-TRSYNC-06 | [TRSYNC](../test-catalog/data-integration/transformer-sync.md)   | As a QA lead I want the open points (demographic edits, duplicate RegID, fallback keys, production-size perfo… |     4 |         0 |       3 |               1 |
| US-DTRANS-01 | [DTRANS](../test-catalog/data-integration/transformer.md)        | Sync incremental regpas (source: `US-D-TRANSFORMER-01`)                                                        |    12 |         0 |      12 |               0 |
| US-DTRANS-02 | [DTRANS](../test-catalog/data-integration/transformer.md)        | Sync incremental medrec (source: `US-D-TRANSFORMER-02`)                                                        |     2 |         0 |       2 |               0 |
| US-DTRANS-03 | [DTRANS](../test-catalog/data-integration/transformer.md)        | Fetch batch data regpas via keys (source: `US-D-TRANSFORMER-03`)                                               |     8 |         0 |       8 |               0 |
| US-DTRANS-04 | [DTRANS](../test-catalog/data-integration/transformer.md)        | Lookup regpas by reg_id (GET) (source: `US-D-TRANSFORMER-04`)                                                  |     4 |         0 |       4 |               0 |
| US-DTRANS-05 | [DTRANS](../test-catalog/data-integration/transformer.md)        | Lookup regpas by medrec / NIK / IHS (source: `US-D-TRANSFORMER-05`)                                            |     9 |         0 |       9 |               0 |
| US-DTRANS-06 | [DTRANS](../test-catalog/data-integration/transformer.md)        | List resource options (source: `US-D-TRANSFORMER-06`)                                                          |     4 |         0 |       4 |               0 |
| US-OUTREG-11 | [OUTREG](../test-catalog/his-backend/outpatient-registration.md) | As a data manager I want the outpatient visit to carry a registration number since it is created so that the … |     1 |         0 |       0 |               1 |

## D10 — Mutu, Jadwal Dokter, Master Data, Public

<a id="f-d10-01"></a>

### [D10.01](D-corporate.md#f-d10-01) — Mutu: data-patient, data-patient-detail, data-department, data-payor-patient

Release status `PROD` (as of 2026-11-12) · 4 stories · 17 test cases · 0 automated (0%)

| Story       | Epic                                              | Story text                                                                 | Cases | Automated | Planned | Blocked / other |
| ----------- | ------------------------------------------------- | -------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DMUTU-01 | [DMUTU](../test-catalog/data-integration/mutu.md) | Data pasien untuk pelaporan mutu (source: `US-D-MUTU-01`)                  |     6 |         0 |       5 |               1 |
| US-DMUTU-02 | [DMUTU](../test-catalog/data-integration/mutu.md) | Detail data pasien untuk audit mutu (source: `US-D-MUTU-02`)               |     4 |         0 |       4 |               0 |
| US-DMUTU-03 | [DMUTU](../test-catalog/data-integration/mutu.md) | Data departemen untuk filter laporan (source: `US-D-MUTU-03`)              |     4 |         0 |       4 |               0 |
| US-DMUTU-04 | [DMUTU](../test-catalog/data-integration/mutu.md) | Data pembayar pasien untuk analisis keuangan mutu (source: `US-D-MUTU-04`) |     3 |         0 |       2 |               1 |

<a id="f-d10-02"></a>

### [D10.02](D-corporate.md#f-d10-02) — Jadwal Dokter: daftar-poli, daftar-dokter, jadwal-dokter (+ alias /api.php)

Release status `PROD` (as of 2026-11-12) · 4 stories · 20 test cases · 0 automated (0%)

| Story         | Epic                                                           | Story text                                   | Cases | Automated | Planned | Blocked / other |
| ------------- | -------------------------------------------------------------- | -------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DJADWAL-01 | [DJADWAL](../test-catalog/data-integration/doctor-schedule.md) | Daftar Poliklinik (source: `US-D-JADWAL-01`) |     5 |         0 |       4 |               1 |
| US-DJADWAL-02 | [DJADWAL](../test-catalog/data-integration/doctor-schedule.md) | Daftar Dokter (source: `US-D-JADWAL-02`)     |     5 |         0 |       3 |               2 |
| US-DJADWAL-03 | [DJADWAL](../test-catalog/data-integration/doctor-schedule.md) | Jadwal Dokter (source: `US-D-JADWAL-03`)     |     4 |         0 |       2 |               2 |
| US-DJADWAL-04 | [DJADWAL](../test-catalog/data-integration/doctor-schedule.md) | API Legacy Alias (source: `US-D-JADWAL-04`)  |     6 |         0 |       5 |               1 |

<a id="f-d10-03"></a>

### [D10.03](D-corporate.md#f-d10-03) — Master Data: holiday (Basic auth, scope master-data:holiday:read)

Release status `DEV-DONE` (as of 2026-11-12) · 1 stories · 12 test cases · 0 automated (0%)

| Story         | Epic                                                       | Story text                                       | Cases | Automated | Planned | Blocked / other |
| ------------- | ---------------------------------------------------------- | ------------------------------------------------ | ----: | --------: | ------: | --------------: |
| US-DMASTER-01 | [DMASTER](../test-catalog/data-integration/master-data.md) | Daftar Hari Libur (source: `US-D-MASTERDATA-01`) |    12 |         0 |      12 |               0 |

<a id="f-d10-04"></a>

### [D10.04](D-corporate.md#f-d10-04) — Public: terminology, regional

Release status `PROD` (as of 2026-11-12) · 2 stories · 15 test cases · 0 automated (0%)

| Story         | Epic                                                  | Story text                                              | Cases | Automated | Planned | Blocked / other |
| ------------- | ----------------------------------------------------- | ------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DPUBLIC-01 | [DPUBLIC](../test-catalog/data-integration/public.md) | Pencarian Terminologi Klinis (source: `US-D-PUBLIC-01`) |     8 |         0 |       3 |               5 |
| US-DPUBLIC-02 | [DPUBLIC](../test-catalog/data-integration/public.md) | Pencarian Data Wilayah (source: `US-D-PUBLIC-02`)       |     7 |         0 |       6 |               1 |

<a id="f-d10-05"></a>

### [D10.05](D-corporate.md#f-d10-05) — Monitoring: service-status, swagger, WhatsApp 5xx alert

Release status `DEV-DONE` (as of 2026-11-12) · 1 stories · 4 test cases · 0 automated (0%)

| Story      | Epic                                               | Story text                                                                                                     | Cases | Automated | Planned | Blocked / other |
| ---------- | -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DSYS-01 | [DSYS](../test-catalog/data-integration/system.md) | Sebagai operator infrastruktur, saya ingin memanggil endpoint /service-status tanpa credential, (source: `US-… |     4 |         0 |       4 |               0 |

## D12 — OnePage App (healthcare-one-page-app → his-data-integration)

<a id="f-d12-01"></a>

### [D12.01](D-corporate.md#f-d12-01) — Verifikasi Kontak: Verifikasi nomor kontak pasien dengan token yang dikirim via social media

Release status `PROD` (as of 2026-11-12) · 1 stories · 4 test cases · 0 automated (0%)

| Story        | Epic                                                  | Story text                                          | Cases | Automated | Planned | Blocked / other |
| ------------ | ----------------------------------------------------- | --------------------------------------------------- | ----: | --------: | ------: | --------------: |
| US-DONEPG-01 | [DONEPG](../test-catalog/data-integration/onepage.md) | Verifikasi social media (source: `US-D-ONEPAGE-01`) |     4 |         0 |       1 |               3 |
