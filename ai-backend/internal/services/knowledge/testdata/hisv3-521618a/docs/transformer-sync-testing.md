# Transformer check-sync-data (regpas & medrec) — cara menjalankan

Spec: `tests/api/rest/his-data-integration/transformer-sync*.spec.ts` · katalog: [test-catalog/transformer-sync.md](test-catalog/data-integration/transformer-sync.md) · task: HISv3-IMPROVEMENT-00145.

Backend yang diuji adalah build lokal his-data-integration yang menunjuk ke DB staging (read-only untuk spec ini), dengan Redis/RabbitMQ/MinIO **lokal**. Harness: `his-v3-ai/.claude/skills/backend-rest-e2e-common` (lihat SKILL.md-nya).

```bash
# 1. infra + build HDI (cabang yang memuat kode transformer yang diuji; his-backend di-replace secara lokal, jangan di-commit)
S=<his-v3-ai>/.claude/skills/backend-rest-e2e-common/scripts; RUN=T145
$S/stack.sh up && node $S/stub-server.mjs --port 19300 --log ~/.cache/hisv3-e2e/$RUN/stub.log &
$S/instance.sh up his-data-integration $RUN 18087 <his-v3-ai>/.claude/skills/data-integration-e2e/overrides.json cmd/restapi <branch>
python3 $S/seed-apiclients.py --run $RUN --repo hdi --config ~/.cache/hisv3-e2e/$RUN/wt-his-data-integration/config.yaml

# 2. spec (env/.env.local: HIS_DATA_INTEGRATION_URL=http://127.0.0.1:18087/api/v1)
VENDOR_E2E_RUN=$RUN HISV3_AI_ROOT=<his-v3-ai> TEST_ENV=local \
  npx playwright test tests/api/rest/his-data-integration --no-deps

# 3. SELALU matikan setelah selesai
python3 $S/teardown.py --run $RUN          # hapus baris api_client QA (ledger) + cek residu
$S/instance.sh down his-data-integration $RUN
$S/stack.sh down; pkill -f stub-server.mjs
```

Catatan:

- `--no-deps` melewati project setup/teardown vendor-backend (butuh his-vendor-backend di :18081). Jangan jalankan teardown harness di tengah sesi: ia menghapus api_client yang diseed.
- Kredensial Basic dibaca dari `~/.cache/hisv3-e2e/<RUN>/secrets.json` (0600) oleh `transformer.auth.ts`; tidak ada rahasia di repo.
- Urutan yang diharapkan dihitung ulang oleh SELECT read-only di `transformer.db.ts` (guard `db.py`).
- Satu test (diagnosa poli DPJP yang berbeda dari diagnosa paling awal) skip bila staging tidak punya datanya.
