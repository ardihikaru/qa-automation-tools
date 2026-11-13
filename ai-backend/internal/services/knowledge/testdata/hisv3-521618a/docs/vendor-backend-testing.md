# his-vendor-backend — automation test

Katalog `docs/test-catalog/vendor-backend/**/*.md` (sumber: e2e assessment run `20261104T1430v`,
`his-v3-ai/services-impl/202611/backend-rest-e2e-qa-assessment/`) dijalankan sebagai spec Playwright **data-driven**.

## Target

- **Staging yang sudah ter-deploy**: `HIS_VENDOR_BACKEND_URL=https://stag-api-vendor-his-v3.sismedika.online/api/v1`
  (`env/.env.staging`, git-ignored; template: `env/.env.staging.example`). Tidak ada build lokal / stub outbound.
- Preflight: `GET /service-status` (base URL dan `/health` mengembalikan 404).
- Endpoint omnichannel `POST /omnichannel/<endpoint>?formType=json` tidak memakai autentikasi (DEF-100).

## Layout data-driven

| Bagian                                                | Isi                                                                                                     |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `test-data/static/vendor-backend/<CLUSTER>.json`      | Tabel case per cluster (hanya baris katalog `planned`/`automated`). Dibuat oleh `export-vendor-cases`   |
| `src/data/vendor-cases/`                              | `types` (zod, key tak dikenal = gagal), `render` (token), `evaluate`, `fixtures`, `harness`, `run-case` |
| `tests/api/rest/his-vendor-backend/<cluster>.spec.ts` | Satu `test()` per case: judul `<TC id> · <judul sumber>`, annotation `case` = id sumber                 |
| `tests/setup/vendor-e2e.{setup,teardown}.ts`          | Project `vendor-e2e-setup` (preflight + seed) dan `vendor-e2e-teardown` (teardown + cek residu)         |

**Service-driven:** spec tidak membuat request mentah. Tiap endpoint punya satu method bertipe di `HisVendorBackendClient`
(`vendorApi.rs(...)`, `vendorApi.bookingBaru(...)`, …, 25 endpoint omnichannel; daftar: `omnichannelOperations`). Runner memilih method dari
`request.path` lewat `src/data/vendor-cases/invoke.ts` (record eksaustif atas `OmnichannelOperation`); tabel case hanya berisi data
(body `json`/`form`/`raw`, query). Endpoint baru = tambah method di client + satu baris di `invoke.ts`; case untuk endpoint yang belum punya
method gagal saat tabel dimuat. Sinarmas/CRM/kiosk/VTT menambah method + operasinya saat clusternya diotomasi.

Token yang didukung (sama dengan harness): `{{fx.x}} {{cap.x}} {{run.marker}} {{run.id}} {{sql:…}} {{date:+N:FMT}}` (UTC+7)
`{{rand:hex|digits:N}} {{nik:qa}} {{phone:qa}}`. Assertion: `status`, `json` (partial), `jsonPath` (eq/ne/contains/regex/exists/absent/gt/gte/lt/lte/len/lenGte/type/in),
`headers`, `bodyContains`, `bodyNotContains` (default `goroutine `, `runtime error`, `panic:`), `db` (polled dengan `expect.poll`).

## Menjalankan

```bash
export TEST_ENV=staging
# prasyarat: python3 (+bcrypt/jwt/yaml), mysql CLI, file cred staging di ~/.config/staging-his-v3-db/ (dibaca skrip, tidak pernah dicetak),
# checkout his-v3-ai (HISV3_AI_ROOT, default ../his-v3-ai) dan WORKSPACE_ROOT_HISV3AI di env/.env.staging
npx playwright test --project=api-rest                         # seed → semua spec → teardown
VENDOR_E2E_CLUSTERS=V-OC-INQ,V-OC-REG npx playwright test --project=api-rest   # seed hanya cluster tertentu
npx playwright test --project=api-rest tests/api/rest/his-vendor-backend/inquiry.spec.ts --repeat-each=3
```

- Run id (`VENDOR_E2E_RUN`, default dibuat otomatis `pw<ts>`); semua baris tulis memakai marker `QA-E2E-<run>`.
- Seed/ledger/cleanup/snapshot/teardown dijalankan skrip harness `backend-rest-e2e-common` (his-v3-ai): `db.py` (guard schema staging,
  write hanya by marker/key literal), `seed.py`, `teardown.py`. Artefak privat: `~/.cache/hisv3-e2e/<run>/` (`teardown-report.json`).
- Run **gagal** kalau teardown tidak melapor `clean: true`. Jangan melemahkan expectation untuk meluluskan test; laporkan kegagalan.
- **Tanpa file cred staging, setup dan teardown di-skip (bukan gagal).** Seed butuh DB staging; kalau file cred
  `~/.config/staging-his-v3-db/his_v3_production_20260508.json` tidak ada, `vendor-e2e-setup` dan `vendor-e2e-teardown`
  melompat dengan alasannya, dan case data-driven ikut melompat sendiri karena fixture-nya tidak bisa dibaca. Project
  dependen (`api-rest`) tetap jalan — jadi spec REST lain (mis. patient registration) tidak ikut terblokir.
  `db.py` membaca `E2E_STAGING_CRED` lebih dulu (lalu jatuh ke path default); guard repo memirror yang sama, jadi
  `E2E_STAGING_CRED=/path/to/cred.json` juga berlaku.

## Booking (chain / serial)

`booking.spec.ts` menjalankan seluruh tabel BOOK berurutan dalam satu worker (`describe.configure({ mode: 'default' })`: berurutan, kegagalan
tidak melewati sisanya). `depends` dihormati (case di-skip, bukan lulus, bila dependensi tidak lulus di run ini); `capture` disimpan ke
`~/.cache/hisv3-e2e/<run>/captures.json` agar selamat dari restart worker. `cleanup` **tidak** dijalankan per test (langkah chain berikutnya
masih butuh barisnya, tag `cleanup-by:`), melainkan diantrikan ke `cleanup.jsonl` dan dieksekusi teardown. Pasien QA berasal dari `seed.sql`.
Case mutating dijalankan **sekali** per run (slot terpakai); subset read-only boleh `--repeat-each=3`.

Race double-booking (`TC-VOCBOOK-055`, DEF-707) berstatus `blocked` / `defect:DEF-707`, sehingga tidak diekspor ke spec: pecundang race
nondeterministik (socket reset lokal, HTTP 502 di staging ter-deploy), jadi `test.fail()` pun akan flaky saat xpass. Dibuka kembali (→ `planned`)
setelah DEF-707 diperbaiki.

## Arsip run (git-ignored, tidak dihapus otomatis)

Setiap run menyimpan semuanya di `.runs/<run id>/` (di-ignore git; dibuat 0700/0600; hapus manual bila perlu):

| File / folder              | Isi                                                                                                              |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `exchanges.jsonl`          | Satu baris per case: request **persis seperti dikirim** (payload), response (status, header, body), `failures`   |
| `results.json`             | Verdict Playwright per test (JSON reporter)                                                                      |
| `report/`, `test-results/` | HTML report dan trace/lampiran Playwright (`npm run report [-- <run id>]` membuka run terbaru atau yang dipilih) |
| `run.json`                 | Run id, waktu mulai, `TEST_ENV`, base URL, cluster yang di-seed                                                  |
| `teardown-report.json`     | Hasil teardown (jumlah cleanup/ledger, residu, `clean`)                                                          |

Payload tidak di-redact karena arsip lokal dan di-ignore; jangan dibagikan/di-upload tanpa menyaring (artifact CI mengecualikan `exchanges.jsonl`).
Artefak privat harness (ledger, `fixtures.json`, `captures.json`, bukti cleanup) tetap di `~/.cache/hisv3-e2e/<run>/`. Memakai `VENDOR_E2E_RUN`
yang sama dua kali menambahkan ke arsip yang sama.

## Lokasi skrip

| Skrip                                                  | Repo                                | Fungsi                                                                                                           |
| ------------------------------------------------------ | ----------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `scripts/export-vendor-cases.mjs`                      | repo ini                            | Menghasilkan `test-data/static/vendor-backend/*.json` dari sumber assessment + katalog                           |
| `scripts/import-qa-cases.mjs`                          | repo ini                            | Seed katalog satu kali dari run assessment vendor + HDI (file epic yang sudah ada tidak ditimpa tanpa `--force`) |
| `scripts/show-report.mjs`                              | repo ini                            | `npm run report`: buka HTML report run terarsip                                                                  |
| `scripts/test-catalog.mjs`                             | repo ini                            | `npm run catalog` / `catalog:report`: validasi id, status, alasan, link spec                                     |
| `src/data/vendor-cases/harness.ts`                     | repo ini                            | Wrapper Node (`execFile`, tanpa shell) atas skrip Python harness di bawah                                        |
| `tests/setup/vendor-e2e.{setup,teardown}.ts`           | repo ini                            | Project Playwright: preflight + seed, lalu teardown + cek residu                                                 |
| `.claude/skills/backend-rest-e2e-common/scripts/db.py` | `his-v3-ai` (dipakai, tidak diubah) | Akses MySQL ber-guard (staging read/write terbatas; prod SELECT-only)                                            |
| `.../seed.py`, `.../teardown.py`                       | `his-v3-ai` (dipakai, tidak diubah) | Seed `seed.sql` per cluster dengan ledger; teardown (cleanup → restore → ledger → residu)                        |

Skrip harness sengaja tidak disalin ke repo ini (satu sumber kebenaran untuk guard); lokasinya lewat `HISV3_AI_ROOT`.

## Aturan dokumentasi

Setiap perubahan kode di area ini **harus** memperbarui dokumen dalam PR yang sama: file ini (layout, perintah, aturan), baris katalog
(Status/Reason/Spec + `npm run catalog:report`), dan `.env.*.example` bila ada variabel baru. Perilaku khusus (mis. skip, chain, defect) dicatat
sebagai aturan di sini, bukan sebagai pengecualian tersembunyi di kode.

## Regenerasi tabel case

```bash
HISV3_AI_ROOT=../his-v3-ai node scripts/export-vendor-cases.mjs   # deterministik; hanya baris katalog planned/automated (blocked tidak pernah diekspor)
```

## Pemetaan Status/Reason katalog → spec

| Katalog                             | Spec                                                                                                                                                                                                             |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `planned`                           | tulis test; jadi `automated` setelah spec ada dan lulus 3×                                                                                                                                                       |
| `blocked` + `defect:DEF-nnn`        | tidak diekspor, jadi tidak ada di spec; jangan dihapus dari katalog. Untuk defect stabil boleh diotomasi `test.fail()` + annotation DEF-nnn (deteksi _xpass_); defect nondeterministik (DEF-707) tetap `blocked` |
| `blocked` + `blocked:…` / `other:…` | `test.skip` dengan alasan, sampai prasyarat tersedia                                                                                                                                                             |
| `blocked` + `deferred:…`            | cluster CRM (`V-OC-CRM`) dan Sinarmas (`V-OC-SINARMAS`) ditunda (keputusan owner 2026-11-06)                                                                                                                     |

Aturan "tidak semua case diimplementasi": [test-catalog/README.md](test-catalog/README.md#not-every-case-is-automated).
