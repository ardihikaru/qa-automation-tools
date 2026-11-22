# Patient registration (clinic-backend)

**Goal:** front-desk staff can find, register and merge patients without creating duplicates.

## User stories

- **US-PATREG-01** ([A01.01](../../modules/A-core.md#f-a01-01)) — As a front-desk clerk I want to search patients by name, birth date or record number so that I reuse the existing record.
- **US-PATREG-02** ([A01.01](../../modules/A-core.md#f-a01-01), [A01.02](../../modules/A-core.md#f-a01-02)) — As a records officer I want to merge two records of the same person so that the visit history is complete.
- **US-PATREG-03** ([A01.04](../../modules/A-core.md#f-a01-04)) — As a front-desk clerk I want to register a new patient with a contact email (e.g. jane.doe@example.com) so that reminders reach them.

## Test cases

| Done | TC ID | Story | Scenario (Given / When / Then) | +/- | Level | Pri | Status | Spec | Reason |
| ---- | ----- | ----- | ------------------------------ | --- | ----- | --- | ------ | ---- | ------ |
| ☑ | TC-PATREG-001 | US-PATREG-01 | Given a patient "Jane Doe", when the clerk searches "doe", then the record is listed | + | API | P0 | automated | `tests/api/patients/search.spec.ts` | |
| ☐ | TC-PATREG-002 | US-PATREG-01 | Given no matching patient, when the clerk searches "zzz", then an empty list is returned | - | API | P1 | planned | | |
| ☑ | TC-PATREG-003 | US-PATREG-02 | Given two duplicate records, when the officer merges them, then one record keeps both visit histories | + | WEB | P1 | automated | [spec](https://git.example.test/acme/clinic-qa/blob/main/tests/web/patients/merge.spec.ts) `tests/web/patients/merge.spec.ts` | |
| ☐ | TC-PATREG-004 | US-PATREG-02 | Given a record with an open invoice, when the officer merges it, then the merge is refused | - | API | P2 | blocked | | defect:BUG-101 merge ignores open invoices |
| ☑ | TC-PATREG-005 | US-PATREG-03 | Given a valid form, when the clerk registers the patient, then a record number is issued | + | E2E | P0 | automated | `tests/e2e/registration/new-patient.spec.ts` | |
| ☐ | TC-PATREG-006 | US-PATREG-03 | Given an invalid email, when the clerk registers the patient, then the form shows a field error | - | API | P1 | planned | | deferred:after the form redesign |
