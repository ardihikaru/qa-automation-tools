# Testing: laporan SIP bulanan (cron his-backend + email-publisher-service)

Katalog: [test-catalog/sip-report-monthly.md](test-catalog/his-backend/sip-report-monthly.md). Spec:

| Spec                                                   | Target                                                    | Syarat                              |
| ------------------------------------------------------ | --------------------------------------------------------- | ----------------------------------- |
| `tests/mq/his-backend/sipreport-monthly.spec.ts`       | binary `sipreportmonthly` (dry-run) terhadap DB staging   | `SIPREPORT_CRON_BIN`, `_CWD`        |
| `tests/mq/email-publisher-service/attachments.spec.ts` | satu e-mail dengan dua lampiran (PDF + XLSX), stack lokal | `RABBITMQ_MGMT_*`, `MAILPIT_URL`, … |

Tanpa variabel di atas, spec di-skip dengan pesan yang jelas (bukan gagal).

## Cron (dry-run, DB staging)

```bash
# di his-backend (branch yang memuat cmd/_cronlike/sipreportmonthly)
go build -o /abs/sipreportmonthly ./cmd/_cronlike/sipreportmonthly
# folder yang berisi config.yaml menuju staging, dengan rabbitMq.subscriberEnabled: false
export SIPREPORT_CRON_BIN=/abs/sipreportmonthly SIPREPORT_CRON_CWD=/abs/dir-with-config
npx playwright test tests/mq/his-backend --project=mq
```

Spec selalu memakai `SIPREPORT_DRY_RUN=true` dan waktu acuan tetap (`1 Nov 2026 08:30 WIB` → bulan Oktober 2026), jadi
angka stabil. Jangan pernah menjalankan binary tanpa dry-run dari test: e-mail akan terkirim ke penerima di `master_config`.
Angka dari binary dibandingkan dengan query independen lewat `dbSelect` (skrip `db.py` milik his-v3-ai, hanya SELECT).

Kasus dalam satu describe berjalan berurutan dalam satu worker (`mode: 'default'`, bukan `serial`) karena self-test Redis
saat start his-backend memakai satu key bersama pada versi lama; versi his-backend yang memuat key unik per proses tidak
punya batasan ini.

## email-publisher-service (stack lokal)

Butuh RabbitMQ (management API), mailpit, MinIO, serta consumer `email-publisher-service` yang berjalan terhadap stack itu
(lihat README repo tersebut, bagian _End-to-end verification_). Isi `RABBITMQ_MGMT_URL/USER/PASSWORD`,
`EMAIL_PUBLISHER_EXCHANGE` (`<market>_<buildMode>_his-v3-exchanger`), `EMAIL_PUBLISHER_QUEUE` (`<market>.<buildMode>.email`),
`MAILPIT_URL`, `MINIO_URL/ACCESS_KEY/SECRET_KEY/BUCKET` di `env/.env.local` (lihat `env/.env.local.example`).
Tidak ada dependency npm baru: publish lewat HTTP API RabbitMQ, upload ke MinIO dengan AWS SigV4 (`MinioClient`).

```bash
npx playwright test tests/mq/email-publisher-service --project=mq --repeat-each=3
```

Yang diperiksa spec ini: satu e-mail, dua lampiran byte-per-byte, `Content-Type` tiap lampiran sesuai file (xlsx harus
`application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`, bukan `application/zip`), dan baris base64 paling
panjang ≤ 76 karakter (RFC 2045 §6.8). Dua pemeriksaan terakhir gagal bila consumer memakai `common-go-modules` yang
belum memuat perbaikan `gomailer` (PR #235), jadi spec ini sekaligus penjaga pin modul.

### Menjalankan di port lain (bila port bawaan dipakai sesi lain)

Semua alamat berasal dari env, tidak ada port yang di-hardcode. Contoh: RabbitMQ AMQP `41672` / management `41673`, MinIO
`41900`, mailpit SMTP `41025` / HTTP `41825`, Redis `41379`, MySQL `41306`. Arahkan `config.yaml` consumer ke port itu
dan isi `RABBITMQ_MGMT_URL`, `MAILPIT_URL`, `MINIO_URL` sesuai. Buat bucket MinIO lebih dulu
(`curl -X PUT --aws-sigv4 aws:amz:us-east-1:s3 --user KEY:SECRET http://host:port/<bucket>`).

**Satu consumer saja pada satu antrean.** Consumer sisa dari run sebelumnya (misalnya binary versi lama) ikut mengambil
pesan dari antrean yang sama, sehingga hasil spec menjadi acak (sebagian lolos, sebagian gagal). Cek dengan
`GET /api/queues` pada management API: `consumers` harus `1`.

## `doctorSipStatusGet` (GraphQL, staging atau backend lokal)

Katalog: [test-catalog/sip-status.md](test-catalog/his-backend/sip-status.md). Spec: `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts`
(project `api-graphql`).

```bash
npx playwright test tests/api/graphql/his-backend/practitioner --project=api-graphql --repeat-each=3
```

- **Akun.** `author` = `HIS_AUTH_*` dan harus anggota grup Administrator atau Super Admin. User non-admin untuk kasus
  "ditolak" diambil dari `CHARTMGMT_VIEWER_*` bila diisi; bila tidak, spec memilih praktisi non-admin lewat SELECT dan
  bertindak sebagai dia dengan `userImpersonatedLogin` (akun `HIS_AUTH_*` yang menjamin), mencoba beberapa kandidat karena
  tidak semua akun bisa diimpersonasi.
- **Data.** Nilai yang diharapkan berasal dari SELECT independen di DB staging (`HISV3_AI_ROOT` → `db.py`), hari dihitung
  pada tanggal kalender WIB. Kasus tanpa data di lingkungan itu (mis. tidak ada dokter dengan SIP < 1 bulan) di-skip dengan
  alasannya.
- **Belum terdeploy.** Bila backend target belum punya query-nya, seluruh spec di-skip dengan alasan "not exposed by this
  backend yet" (dicek lewat introspeksi), lalu otomatis berjalan penuh setelah deploy. Untuk menguji PR sebelum deploy,
  jalankan backend lokal dari branch PR dengan DB staging (skill `backend-pr-runner`) dan arahkan
  `HIS_BACKEND_GRAPHQL_URL` ke sana.
- **Nilai yang tidak boleh bocor.** Nomor SIP bukan bagian kontrak: spec memeriksa field `DoctorSipStatus` lewat introspeksi.
