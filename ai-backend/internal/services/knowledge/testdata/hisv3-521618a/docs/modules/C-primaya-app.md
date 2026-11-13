# Part C — Primaya mobile app integration (service-hisv3-for-primayaapp)

Part of the [HISv3 module catalogue](README.md) — the single source of truth for modules, sub-modules and features.
Legend, status rules and the automation backlog: [README.md](README.md).

Columns `ID` … `Notes` are the product map (edit by hand when the product changes; feature names stay as reverse-engineered,
in Bahasa). `Status as of` = the date the status was last confirmed: for `PROD` "in production as of", for `DEV-DONE`
"development done (not yet used in production) as of". Columns `Epics` … `Automated` are **generated** by
`npm run catalog:report` from the user stories in `docs/test-catalog/` — never edit them by hand.

<a id="c01"></a>

### C01 — API Aplikasi Mobile Primaya (`/app`)

_BRD:_ `BRD:D1` · _Features:_ 7

| ID                          | Sub-module       | Feature                                          | Repo / entry point           | Status           | Status as of | Evidence               | Notes                           | Epics | Stories | TCs | Automated |
| --------------------------- | ---------------- | ------------------------------------------------ | ---------------------------- | ---------------- | ------------ | ---------------------- | ------------------------------- | ----- | ------- | --- | --------- |
| <a id="f-c01-01"></a>C01.01 | Pasien / Medrec  | POST medrec, POST check-medrec, GET show-profile | service-hisv3-for-primayaapp | 🟢 `PROD`        | 2026-11-12   | GIT:main               |                                 |       |         |     |           |
| <a id="f-c01-02"></a>C01.02 | Dokter & Jadwal  | GET doctors, GET list-schedules                  | service-hisv3-for-primayaapp | 🟢 `PROD`        | 2026-11-12   | GIT:main               |                                 |       |         |     |           |
| <a id="f-c01-03"></a>C01.03 | Antrian          | GET list-antrian                                 | service-hisv3-for-primayaapp | 🟢 `PROD`        | 2026-11-12   | GIT:main               |                                 |       |         |     |           |
| <a id="f-c01-04"></a>C01.04 | Billing          | GET list-billings, POST callback-billing         | service-hisv3-for-primayaapp | 🟢 `PROD`        | 2026-11-12   | GIT:main               |                                 |       |         |     |           |
| <a id="f-c01-05"></a>C01.05 | Harga Konsultasi | POST consultation-price, consultation-price2     | service-hisv3-for-primayaapp | 🟢 `PROD`        | 2026-11-12   | GIT:main               |                                 |       |         |     |           |
| <a id="f-c01-06"></a>C01.06 | Layanan          | GET services-detail, GET service-code            | service-hisv3-for-primayaapp | 🟢 `PROD`        | 2026-11-12   | GIT:main               |                                 |       |         |     |           |
| <a id="f-c01-07"></a>C01.07 | JKN Mobile       | Integrasi antrian HIS untuk JKN Mobile           | service-hisv3-for-primayaapp | 🟡 `IN-PROGRESS` | —            | S:BPJS/JKN Mobile (0%) | milik BPJS; ongoing development |       |         |     |           |
