# Part C — Partner API (partner-api)

Part of the [Acme Clinic module catalogue](README.md). REST API used by partner systems at `https://api.example.test`.

### C01 — Partner Access

| ID | Sub-module | Feature | Repo / entry point | Status | Status as of | Evidence | Notes | Epics | Stories | TCs | Automated |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="f-c01-01"></a>C01.01 | API Keys | Issue, rotate and revoke partner API keys | partner-api | 🟢 `PROD` | 2026-11-12 | GIT:main | | [VSYS](../test-catalog/vendor-api/system.md) | 1 | 3 | 2 (67%) |
| <a id="f-c01-02"></a>C01.02 | Gateway | Rate limiting per partner | partner-api | 🟢 `PROD` | 2026-11-12 | GIT:main | | [VSYS](../test-catalog/vendor-api/system.md) | 1 | 2 | 1 (50%) |

### C02 — Partner Claims

| ID | Sub-module | Feature | Repo / entry point | Status | Status as of | Evidence | Notes | Epics | Stories | TCs | Automated |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="f-c02-01"></a>C02.01 | Claims | Submit an insurance claim for a settled invoice | partner-api | 🟢 `PROD` | 2026-11-12 | GIT:main | | [VCLAIM](../test-catalog/vendor-api/partners/claims.md) | 2 | 4 | 1 (25%) |
| <a id="f-c02-02"></a>C02.02 | Claims | Claim status callback | partner-api | 🔵 `DEV-DONE` | 2026-11-12 | GIT:staging | | [VCLAIM](../test-catalog/vendor-api/partners/claims.md) | 1 | 1 | 0 (0%) |
