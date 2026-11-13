# Transformer check-sync-data — user stories, acceptance criteria & scenarios

Task: [HISv3-IMPROVEMENT-00145](https://taskmanager.apps.sismedika.com/tasks/HISv3-IMPROVEMENT-00145) · endpoints: `POST /api/v1/transformer/check-sync-data/regpas` and `/medrec` ·
code: his-backend [#4818](https://github.com/developersismedika/his-backend/pull/4818), docs HDI [#120](https://github.com/developersismedika/his-data-integration/pull/120) ·
test cases: [test-catalog/transformer-sync.md](test-catalog/data-integration/transformer-sync.md) (`TC-TRSYNC-nnn`) · how to run: [transformer-sync-testing.md](transformer-sync-testing.md).

## 1. Original checklist → story

| Checklist item (HISv3-IMPROVEMENT-00145)                        | Story        | Where it is tested                              |
| --------------------------------------------------------------- | ------------ | ----------------------------------------------- |
| `last_id: PKG32610300001` must give the data that follows (BUG) | US-TRSYNC-01 | `transformer-sync-last-id.spec.ts`              |
| `/regpas`: `last_id` is null or a RegID, anything else rejected | US-TRSYNC-01 | `transformer-sync*.spec.ts`                     |
| `/medrec`: `last_id` is null or a MEDRECID, else rejected       | US-TRSYNC-02 | `transformer-sync*.spec.ts`                     |
| `/medrec`: ignore `_sync` and `_sync_date`                      | US-TRSYNC-04 | `transformer-sync.spec.ts`                      |
| `/medrec`: order by last-updated date                           | US-TRSYNC-02 | `transformer-sync.spec.ts` (DB oracle)          |
| `/regpas`: order by date (`closed_dttm`, Pasien Pulang)         | US-TRSYNC-01 | `transformer-sync.spec.ts` (DB oracle)          |
| `regpas_diagnosa` from the DPJP poli                            | US-TRSYNC-03 | `transformer-sync-contract.spec.ts`             |
| `DepartemenID2` / `DokterID2` as `""` instead of null           | US-TRSYNC-03 | `transformer-sync.spec.ts`, `-contract.spec.ts` |

## 2. User stories and acceptance criteria

**US-TRSYNC-01 — regpas resume by RegID.** As a Transformer consumer I want to page `regpas` in discharge order and resume from the RegID I last processed, so that I never re-read or miss a registration after a restart.

- Order is `COALESCE(encounter_account.closed_dttm, encounter.updated_dttm)`, then RegID; the first page equals that order read straight from the DB.
- `last_id` = `null`, omitted, or `null` with `cursor_value: null` → the first page.
- `last_id` = a RegID the consumer already processed → that record is **skipped**; the page starts at the next one (works without `cursor_value`).
- `last_id` that is not an existing RegID (typo, suffix, prefix, space, a MEDRECID, a UUID, empty, `0`) → HTTP 422, `errors` has only `last_id`, no data.
- `meta.last_id` of one response fed into the next call continues without gap or duplicate; the last record gives an empty page with `has_more=false`.
- `cursor_by=created` keeps the legacy created_dttm walk, with the same `last_id` validation.

**US-TRSYNC-02 — medrec resume by MEDRECID.** Same criteria with MEDRECID (`patient.patient_mrn`) and the patient sync key: the later of the patient's latest encounter `closed_dttm` and their own last update.

**US-TRSYNC-03 — regpas poli, doctor, diagnosis.** `DepartemenID2` / `DokterID2` hold the second poli and its doctor, or `""` (never null); `regpas_diagnosa` is the earliest diagnosis made in the DPJP's (first) poli, falling back to the earliest diagnosis.

**US-TRSYNC-04 — no `_sync`.** Every `medrec` record, also the `medrec` nested in `regpas`, has `_sync = 0` and `_sync_date = ""`.

**US-TRSYNC-05 — request contract.** `limit` outside 1..200 → 422; `X-Timezone` shifts `last_cursor` by the zone offset (default Asia/Jakarta) and an unknown zone → 422; a `cursor_value` that is not `YYYY-MM-DD HH:MM:SS` → 422; a cursor past every record → empty page; no / unscoped credential → 401 / 403; envelope-only resources and unknown resources keep their behaviour.

**US-TRSYNC-06 — open points.** Cases that need a decision, a staging write or production-size data are tracked as `planned` / `manual-only` (`TC-TRSYNC-035..038`).

## 3. Scenarios on real staging data (`last_id`)

The expected sequence is read from staging by independent read-only SELECTs (`transformer.db.ts`) when the test runs, so the specs never hard-code a key. The values below are a snapshot of 2026-11-11 (staging keeps growing). Sync keys are UTC. Patient MEDRECIDs are masked in docs.

**regpas** (681 registrations): #1 `PKG32608250001` (2026-08-25 13:18:51), #2 `PKG32608260001` (08-26 08:07:05), #3 `PKG32608270001` (08-27 01:18:57), #4 `PKG32608270004` (08-27 02:48:03), #5 `PKG32608270007`, #6 `PKG32608270002`; #41 `PKG32608280006`; last `PKG32611110011`.
**medrec** (631 patients): #1 `MR00005…593` (2026-08-26 07:47:48), #2 …598, #3 …600, #4 …591, #5 …599; #41 …667; last …796.

| Scenario                                 | What the caller sends                                                                 | Expected result (regpas example; medrec follows the same pattern)                                                        | TC range          |
| ---------------------------------------- | ------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | ----------------- |
| 1. `last_id` wrong                       | `PKG32608250001-X`, `xPKG32608250001`, a leading space, `…0`, `0`, a UUID, a MEDRECID | 422 for every value; `errors` = only `last_id`; no data                                                                  | TC-TRSYNC-039/040 |
| 2. `last_id` null                        | `limit 3`, with `last_id` missing / `null` / `null` + `cursor_value: null`            | #1, #2, #3 = `PKG32608250001`, `PKG32608260001`, `PKG32608270001`                                                        | TC-TRSYNC-041/042 |
| 3. `last_id` = #1 (consumer finished #1) | `limit 3`, `last_id: "PKG32608250001"` after page [#1,#2,#3]                          | **#1 skipped** → #2, #3, #4 = `PKG32608260001`, `PKG32608270001`, `PKG32608270004`; `meta.last_id` = #4; `has_more` true | TC-TRSYNC-043/044 |
| 4. Checkpoint moves forward              | `last_id` = #1, #2, #3, #4, #5 in turn                                                | each time the 3 records right after that key (e.g. `last_id` #3 → #4, #5, #6)                                            | TC-TRSYNC-045/046 |
| 5. Feed `meta.last_id` back              | 3 calls of `limit 3`, `last_id` = previous `meta.last_id`                             | together exactly #1..#9 of the DB order, no skip, no duplicate                                                           | TC-TRSYNC-047/048 |
| 6. Checkpoint deep in the data           | `last_id` = #41 `PKG32608280006`                                                      | #42, #43, #44 as the DB orders them                                                                                      | TC-TRSYNC-049/050 |
| 7. `last_id` older than `cursor_value`   | `cursor_value` = key of #3 + `last_id` = #1                                           | the cursor wins: starts at #4; #1 not returned                                                                           | TC-TRSYNC-051/052 |
| 8. `last_id` = last record               | `last_id: "PKG32611110011"`                                                           | `data: []`, `has_more: false`                                                                                            | TC-TRSYNC-053/054 |

Known behaviour: `last_id` matching is **not case-sensitive** (DB collation `utf8mb4_general_ci`): `pkg32608250001` is accepted as `PKG32608250001` and continues from the same record.

## 4. Sample payloads and responses

Captured from the local build against staging (record fields trimmed to the key ones, `…` = other fields omitted; credential is a placeholder).

**Request — scenario 3, `last_id` = #1**

```http
POST /api/v1/transformer/check-sync-data/regpas
Authorization: Basic <api_client with scope transformer:read>
Content-Type: application/json

{"limit": 3, "last_id": "PKG32608250001"}
```

**Response — 200**

```json
{
  "data": [
    {
      "RegID": "PKG32608260001",
      "Tanggal": "2026-08-26 00:06:46",
      "DepartemenID": "IGD",
      "DepartemenID2": "DEP-ID-B015",
      "…": "…"
    },
    {
      "RegID": "PKG32608270001",
      "Tanggal": "2026-08-27 01:18:57",
      "DepartemenID": "DEP-ID-F008",
      "DepartemenID2": "",
      "DokterID2": "",
      "…": "…"
    },
    { "RegID": "PKG32608270004", "…": "…" }
  ],
  "meta": {
    "resource": "regpas",
    "cursor_field": "SyncDttm",
    "cursor_type": "datetime",
    "taken": 3,
    "total": 681,
    "last_cursor": "2026-08-27 09:48:03",
    "last_id": "PKG32608270004",
    "has_more": true,
    "server_time": "2026-11-11T14:30:28+07:00"
  }
}
```

(`last_cursor` is in the `X-Timezone` zone, default Asia/Jakarta: UTC 02:48:03 → 09:48:03.)

**Request — scenario 1, wrong `last_id`**

```http
POST /api/v1/transformer/check-sync-data/regpas
Content-Type: application/json

{"limit": 3, "last_id": "WRONG-VALUE"}
```

**Response — 422**

```json
{
  "message": "must be null or an existing RegID (use meta.last_id)",
  "errors": { "last_id": ["must be null or an existing RegID (use meta.last_id)"] }
}
```

For `/medrec` the same calls apply with `MEDRECID` keys; the message says `existing MEDRECID`, and every record carries `"_sync": 0, "_sync_date": ""`.

The complete captured set (null, #1 skip, three wrong-value variants, for both resources) is in the evidence comment on the PR.
