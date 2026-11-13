# Part B — Omni-channel & vendor integration (his-vendor-backend)

Part of the [HISv3 module catalogue](README.md) — the single source of truth for modules, sub-modules and features.
Legend, status rules and the automation backlog: [README.md](README.md).

Columns `ID` … `Notes` are the product map (edit by hand when the product changes; feature names stay as reverse-engineered,
in Bahasa). `Status as of` = the date the status was last confirmed: for `PROD` "in production as of", for `DEV-DONE`
"development done (not yet used in production) as of". Columns `Epics` … `Automated` are **generated** by
`npm run catalog:report` from the user stories in `docs/test-catalog/` — never edit them by hand.

<a id="b01"></a>

### B01 — Omnichannel — Inquiry & Registrasi

_BRD:_ `BRD:D1` · _Features:_ 2

| ID                          | Sub-module | Feature                                                                                      | Repo / entry point                | Status    | Status as of | Evidence | Notes      | Epics                                                                | Stories                       | TCs | Automated |
| --------------------------- | ---------- | -------------------------------------------------------------------------------------------- | --------------------------------- | --------- | ------------ | -------- | ---------- | -------------------------------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-b01-01"></a>B01.01 | Inquiry    | rs, jadwal, dokter, jadwaldokter, jadwaldoktertersedia, jadwaldokter_periodetanggal, profile | his-vendor-backend `/omnichannel` | 🟢 `PROD` | 2026-11-12   | GIT:main | tanpa auth | [VOCINQ](../test-catalog/vendor-backend/omnichannel/inquiry.md)      | [7](traceability.md#f-b01-01) | 57  | 46 (81%)  |
| <a id="f-b01-02"></a>B01.02 | Registrasi | medrec, medrec_nama (cari pasien: MRN / nama / NIK / telepon)                                | his-vendor-backend `/omnichannel` | 🟢 `PROD` | 2026-11-12   | GIT:main |            | [VOCREG](../test-catalog/vendor-backend/omnichannel/registration.md) | [2](traceability.md#f-b01-02) | 23  | 17 (74%)  |

<a id="b02"></a>

### B02 — Omnichannel — Booking

_BRD:_ `BRD:D1` · _Features:_ 3

| ID                          | Sub-module     | Feature                                                               | Repo / entry point                | Status    | Status as of | Evidence | Notes                | Epics                                                                                                                                   | Stories                       | TCs | Automated |
| --------------------------- | -------------- | --------------------------------------------------------------------- | --------------------------------- | --------- | ------------ | -------- | -------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-b02-01"></a>B02.01 | Booking        | cek_gantung, cek_slot, booking, booking_baru                          | his-vendor-backend `/omnichannel` | 🟢 `PROD` | 2026-11-12   | GIT:main |                      | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md)                                                                        | [4](traceability.md#f-b02-01) | 47  | 35 (74%)  |
| <a id="f-b02-02"></a>B02.02 | Booking        | booking_bayar, summary, booking_edit, booking_document, booking_batal | his-vendor-backend `/omnichannel` | 🟢 `PROD` | 2026-11-12   | GIT:main |                      | [VOCBOOK](../test-catalog/vendor-backend/omnichannel/booking.md)                                                                        | [5](traceability.md#f-b02-02) | 32  | 24 (75%)  |
| <a id="f-b02-03"></a>B02.03 | Contact Center | list_departemen, list_appointment, info_kamar, validasi_pasien        | his-vendor-backend `/omnichannel` | 🟢 `PROD` | 2026-11-12   | GIT:main | tidak ada di swagger | [VOCCC](../test-catalog/vendor-backend/omnichannel/callcenter.md), [VOCREG](../test-catalog/vendor-backend/omnichannel/registration.md) | [4](traceability.md#f-b02-03) | 35  | 24 (69%)  |

<a id="b03"></a>

### B03 — Integrasi Asuransi

_BRD:_ `BRD:D20` · _Features:_ 2

| ID                          | Sub-module         | Feature                                                                                  | Repo / entry point | Status    | Status as of | Evidence      | Notes                                                                                                                         | Epics                                                          | Stories                        | TCs | Automated |
| --------------------------- | ------------------ | ---------------------------------------------------------------------------------------- | ------------------ | --------- | ------------ | ------------- | ----------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ------------------------------ | --- | --------- |
| <a id="f-b03-01"></a>B03.01 | Garda Medika       | garda_dischargeresult, garda_claimpaymentresult, garda_uploadinvoiceresult               | his-vendor-backend | 🟢 `PROD` | 2026-11-12   | S:GAM-001/002 | handler hanya validasi + balas OK:1, tidak persist                                                                            | [VOCGARDA](../test-catalog/vendor-backend/insurance/garda.md)  | [3](traceability.md#f-b03-01)  | 33  | 26 (79%)  |
| <a id="f-b03-02"></a>B03.02 | Sinarmas (inbound) | jawabanasuransi, jawabpersetujuantindakan, biayaakhir, permintaanasuransi, getbillingreg | his-vendor-backend | 🟢 `PROD` | 2026-11-12   | RN:20260731   | sudah dipakai di production (konfirmasi owner); aktif jika sinarmasIntegration.baseURL di-set; tulis tabel sinarmas_* + MinIO | [VOCSIN](../test-catalog/vendor-backend/insurance/sinarmas.md) | [17](traceability.md#f-b03-02) | 49  | 0 (0%)    |

<a id="b04"></a>

### B04 — Kiosk HIS v2

_BRD:_ `BRD:D1` · _Features:_ 9

| ID                          | Sub-module           | Feature                                                                         | Repo / entry point                 | Status           | Status as of | Evidence                | Notes                                                                                                           | Epics                                                                                                                      | Stories                       | TCs | Automated |
| --------------------------- | -------------------- | ------------------------------------------------------------------------------- | ---------------------------------- | ---------------- | ------------ | ----------------------- | --------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-b04-01"></a>B04.01 | Konten & Pengaturan  | settings, packages, excellence, network, info, assets, home/stats, smart-search | his-vendor-backend `/kiosk-his-v2` | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-001..006 (blue) | QA/QC KIOSK-019 0%                                                                                              | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md)                                                                  | [8](traceability.md#f-b04-01) | 42  | 0 (0%)    |
| <a id="f-b04-02"></a>B04.02 | Pasien & Kunjungan   | patients (register/search/get), visits, appointments, checkin, tickets          | his-vendor-backend `/kiosk-his-v2` | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-010,011         |                                                                                                                 | [VKAPPT](../test-catalog/vendor-backend/kiosk/appointment.md), [VKPAT](../test-catalog/vendor-backend/kiosk/patient.md)    | [7](traceability.md#f-b04-02) | 67  | 0 (0%)    |
| <a id="f-b04-03"></a>B04.03 | Dokter & Jadwal      | doctors, slots, dates, on-leave                                                 | his-vendor-backend `/kiosk-his-v2` | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-007             |                                                                                                                 | [VKSCHED](../test-catalog/vendor-backend/kiosk/schedule.md)                                                                | [4](traceability.md#f-b04-03) | 21  | 0 (0%)    |
| <a id="f-b04-04"></a>B04.04 | BPJS / VClaim        | rujukan, cari-nik, surat-kontrol, vclaim/check                                  | his-vendor-backend `/kiosk-his-v2` | 🟡 `IN-PROGRESS` | —            | S:KIOSK-005 (mock)      | mock sampai integrasi BPJS selesai (BPJS under development); selain BPJS kiosk dev-done tapi belum dipakai user | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md), [VKPAT](../test-catalog/vendor-backend/kiosk/patient.md)           | [4](traceability.md#f-b04-04) | 23  | 0 (0%)    |
| <a id="f-b04-05"></a>B04.05 | Farmasi              | pharmacy/status (real); pharmacy/delivery (mock)                                | his-vendor-backend `/kiosk-his-v2` | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-013,014         | delivery masih mock                                                                                             | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md), [VKREAD](../test-catalog/vendor-backend/kiosk/clinical-read.md)    | [2](traceability.md#f-b04-05) | 24  | 0 (0%)    |
| <a id="f-b04-06"></a>B04.06 | Billing & Pembayaran | billing/search, billing/{regId}; payment/create/check/callback (mock Duitku)    | his-vendor-backend `/kiosk-his-v2` | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-004,012         | pembayaran mock                                                                                                 | [VKMOCK](../test-catalog/vendor-backend/kiosk/mock.md), [VKREAD](../test-catalog/vendor-backend/kiosk/clinical-read.md)    | [4](traceability.md#f-b04-06) | 33  | 0 (0%)    |
| <a id="f-b04-07"></a>B04.07 | Klinis Read          | lab/results (lab & rad), queue/live                                             | his-vendor-backend `/kiosk-his-v2` | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-009,015         |                                                                                                                 | [VKCONT](../test-catalog/vendor-backend/kiosk/content.md), [VKREAD](../test-catalog/vendor-backend/kiosk/clinical-read.md) | [2](traceability.md#f-b04-07) | 15  | 0 (0%)    |
| <a id="f-b04-08"></a>B04.08 | Officer Call         | POST/GET/PATCH officer-calls, events (scope officer-call:operate)               | his-vendor-backend `/kiosk-his-v2` | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-017             |                                                                                                                 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md), [VKOFF](../test-catalog/vendor-backend/kiosk/officer-call.md)      | [9](traceability.md#f-b04-08) | 59  | 0 (0%)    |
| <a id="f-b04-09"></a>B04.09 | Keamanan             | JWT Bearer scope kiosk:invoke                                                   | his-vendor-backend                 | 🔵 `DEV-DONE`    | 2026-11-12   | S:KIOSK-016             |                                                                                                                 | [VKAUTH](../test-catalog/vendor-backend/kiosk/auth.md)                                                                     | [6](traceability.md#f-b04-09) | 45  | 0 (0%)    |

<a id="b05"></a>

### B05 — VTT — Dictation to EMR

_BRD:_ `BRD:D6` · _Features:_ 5

| ID                          | Sub-module      | Feature                                                         | Repo / entry point        | Status           | Status as of | Evidence                | Notes                     | Epics                                                   | Stories                       | TCs | Automated |
| --------------------------- | --------------- | --------------------------------------------------------------- | ------------------------- | ---------------- | ------------ | ----------------------- | ------------------------- | ------------------------------------------------------- | ----------------------------- | --- | --------- |
| <a id="f-b05-01"></a>B05.01 | Lookup          | patients, practitioners, master-item (staging)                  | his-vendor-backend `/vtt` | 🔵 `DEV-DONE`    | 2026-11-12   | S:VTT-001; GIT:stg      | master-item hanya staging | [VTTREAD](../test-catalog/vendor-backend/vtt/lookup.md) | [5](traceability.md#f-b05-01) | 34  | 0 (0%)    |
| <a id="f-b05-02"></a>B05.02 | SOAP            | POST/GET soap, audio stream (retensi 3 hari)                    | his-vendor-backend `/vtt` | 🔵 `DEV-DONE`    | 2026-11-12   | S:VTT-001..003          |                           | [VTTSOAP](../test-catalog/vendor-backend/vtt/soap.md)   | [4](traceability.md#f-b05-02) | 30  | 0 (0%)    |
| <a id="f-b05-03"></a>B05.03 | EMR Update      | soap/emr/{emrID}, emr-with-actions (buat/hapus service request) | his-vendor-backend `/vtt` | 🔵 `DEV-DONE`    | 2026-11-12   | S:VTT-004..008; GIT:stg |                           | [VTTEMR](../test-catalog/vendor-backend/vtt/emr.md)     | [3](traceability.md#f-b05-03) | 19  | 0 (0%)    |
| <a id="f-b05-04"></a>B05.04 | Service Request | general-dropdown, requests (POST/GET)                           | his-vendor-backend `/vtt` | 🔵 `DEV-DONE`    | 2026-11-12   | S:VTT-004..008          |                           | [VTTEMR](../test-catalog/vendor-backend/vtt/emr.md)     | [2](traceability.md#f-b05-04) | 18  | 0 (0%)    |
| <a id="f-b05-05"></a>B05.05 | Daftar Obat RS  | REST API daftar obat di RS                                      | his-vendor-backend        | ⚪ `NOT-STARTED` | —            | S:VTT-009 (0%)          |                           |                                                         |                               |     |           |

<a id="b06"></a>

### B06 — Teknis & Monitoring (his-vendor-backend)

_BRD:_ `BRD:D24` · _Features:_ 3

| ID                          | Sub-module      | Feature                                                                                                  | Repo / entry point                          | Status    | Status as of | Evidence | Notes                                                   | Epics                                            | Stories                       | TCs | Automated |
| --------------------------- | --------------- | -------------------------------------------------------------------------------------------------------- | ------------------------------------------- | --------- | ------------ | -------- | ------------------------------------------------------- | ------------------------------------------------ | ----------------------------- | --- | --------- |
| <a id="f-b06-01"></a>B06.01 | Liveness        | `GET /service-status` tanpa auth: HTTP 200, JSON, tanpa stack trace / detail internal                    | his-vendor-backend `/api/v1/service-status` | 🟢 `PROD` | 2026-11-12   | GIT:main | fitur teknis (probe load balancer / orchestrator)       | [VSYS](../test-catalog/vendor-backend/system.md) | [1](traceability.md#f-b06-01) | 2   | 0 (0%)    |
| <a id="f-b06-02"></a>B06.02 | Dokumentasi API | Swagger UI `/swagger/*` (index.html, swagger.json); path tak dikenal → 404 tanpa panic                   | his-vendor-backend `/api/v1/swagger/*`      | 🟢 `PROD` | 2026-11-12   | GIT:main | fitur teknis (untuk developer / integrator)             | [VSYS](../test-catalog/vendor-backend/system.md) | [1](traceability.md#f-b06-02) | 2   | 0 (0%)    |
| <a id="f-b06-03"></a>B06.03 | Router          | Route atau method tidak valid → 404/405 bersih tanpa bocoran internal (panic, goroutine, SQL, path file) | his-vendor-backend (router)                 | 🟢 `PROD` | 2026-11-12   | GIT:main | fitur teknis (kontrak error untuk integrator eksternal) | [VSYS](../test-catalog/vendor-backend/system.md) | [1](traceability.md#f-b06-03) | 4   | 0 (0%)    |
