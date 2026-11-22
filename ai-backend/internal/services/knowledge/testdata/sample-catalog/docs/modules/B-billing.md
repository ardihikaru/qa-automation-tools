# Part B — Billing & Reports

Part of the [Acme Clinic module catalogue](README.md).

### B01 — Billing

| ID | Sub-module | Feature | Repo / entry point | Status | Status as of | Evidence | Notes | Epics | Stories | TCs | Automated |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="f-b01-001"></a>B01.001 | Invoices | Build an invoice from visit services | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [BILL](../test-catalog/clinic-backend/billing.md) | 2 | 4 | 2 (50%) |
| <a id="f-b01-002"></a>B01.002 | Invoices | Discounts and invoice corrections | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [BILL](../test-catalog/clinic-backend/billing.md) | 1 | 2 | 0 (0%) |
| <a id="f-b01-003"></a>B01.003 | Payments | Cash and card payments, receipts | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [BILL](../test-catalog/clinic-backend/billing.md) | 1 | 2 | 1 (50%) |
| <a id="f-b01-003-01"></a>B01.003.01 | Payments | Refund of a settled payment | clinic-backend | 🟡 `IN-PROGRESS` | — | S:PAY-010 | | | | | |

### B02 — Reports

| ID | Sub-module | Feature | Repo / entry point | Status | Status as of | Evidence | Notes | Epics | Stories | TCs | Automated |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="f-b02-01"></a>B02.01 | Daily Reports | Daily visit and revenue summary | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | | | | |
| <a id="f-b02-02"></a>B02.02 | Daily Reports | Export a report to CSV | clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | | | | |
| <a id="f-b02-03"></a>B02.03 | | Monthly statistics dashboard | clinic-web | ⚪ `NOT-STARTED` | — | | | | | | |
