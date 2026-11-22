# Part A — Core clinic platform (clinic-backend + clinic-web)

Part of the [Acme Clinic module catalogue](README.md). Columns `Epics` … `Automated` are generated.

<a id="a01"></a>

### A01 — Patient Registration & Visits

| ID | Sub-module | Feature | Repo / entry point | Status | Status as of | Evidence | Notes | Epics | Stories | TCs | Automated |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="f-a01-01"></a>A01.01 | Patient Master | Search, create and edit patient records (`/patients`) | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [PATREG](../test-catalog/clinic-backend/patient-registration.md) | 2 | 4 | 2 (50%) |
| <a id="f-a01-02"></a>A01.02 | patient master | Merge duplicate patients and record allergies | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [PATREG](../test-catalog/clinic-backend/patient-registration.md) | 1 | 2 | 0 (0%) |
| <a id="f-a01-03"></a>A01.03 | Patient Master | Temporary identity for walk-in patients | clinic-backend + clinic-web | 🔵 `DEV-DONE` | 2026-11-12 | GIT:staging | waits for the next release | | | | |
| <a id="f-a01-04"></a>A01.04 | Registration | Register a new patient at the front desk | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [PATREG](../test-catalog/clinic-backend/patient-registration.md) | 1 | 2 | 1 (50%) |
| <a id="f-a01-05"></a>A01.05 | Registration | Self check-in kiosk registration | clinic-backend + clinic-web | 🟡 `IN-PROGRESS` | — | S:REG-004 | design review pending | | | | |
| <a id="f-a01-06"></a>A01.06 | Visits | Open, close and cancel a visit | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | | | | |
| <a id="f-a01-07"></a>A01.07 | Visits | Visit check-in from an appointment, change of doctor, follow-up plan | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [APPT](../test-catalog/clinic-backend/appointments.md) | 3 | 6 | 2 (33%) |
| <a id="f-a01-08"></a>A01.08 | | Visit history timeline | clinic-web | ⚪ `NOT-STARTED` | — | | | | | | |

<a id="a02"></a>

### A02 — Appointments

| ID | Sub-module | Feature | Repo / entry point | Status | Status as of | Evidence | Notes | Epics | Stories | TCs | Automated |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="f-a02-01"></a>A02.01 | Booking | Book, reschedule and cancel an appointment | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | [APPT](../test-catalog/clinic-backend/appointments.md) | 3 | 5 | 2 (40%) |
| <a id="f-a02-02"></a>A02.02 | Booking | Reject bookings on a doctor's day off | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | RN:20261101 | | [APPT](../test-catalog/clinic-backend/appointments.md) | 1 | 2 | 1 (50%) |
| <a id="f-a02-03"></a>A02.03 | Doctor Schedule | Weekly doctor schedule and leave calendar | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | | | | |
| <a id="f-a02-04"></a>A02.04 | Reminders | Email reminder one day before the visit (`noreply@example.com`) | clinic-backend | 🔵 `DEV-DONE` | 2026-11-12 | GIT:staging | | | | | |

<a id="a03"></a>

### A03 — Authentication & Accounts

| ID | Sub-module | Feature | Repo / entry point | Status | Status as of | Evidence | Notes | Epics | Stories | TCs | Automated |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| <a id="f-a03-01"></a>A03.01 | Login | Username and password login with lockout | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | | | | |
| <a id="f-a03-02"></a>A03.02 | Login | Password reset by email link | clinic-backend + clinic-web | 🟢 `PROD` | 2026-11-12 | GIT:main | | | | | |
| <a id="f-a03-03"></a>A03.03 | Roles | Role-based menu and permissions | clinic-backend + clinic-web | ❔ `UNKNOWN` | — | | status not confirmed yet | | | | |
