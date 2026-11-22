# Appointments and visit check-in (clinic-backend)

**Goal:** patients can book appointments and are checked in to a visit on arrival.

## User stories

- **US-APPT-01** ([A02.01](../../modules/A-core.md#f-a02-01)) — As a patient I want to book an appointment online so that I do not wait at the desk.
- **US-APPT-02** ([A02.01](../../modules/A-core.md#f-a02-01), [A02.02](../../modules/A-core.md#f-a02-02)) — As a scheduler I want bookings on a doctor's day off rejected so that no patient is turned away.
- **US-APPT-03** ([A01.07](../../modules/A-core.md#f-a01-07), [A02.01](../../modules/A-core.md#f-a02-01)) — As a front-desk clerk I want to check in a booked patient so that a visit opens from the appointment.
- **US-APPT-04** ([A01.07](../../modules/A-core.md#f-a01-07)) — As a nurse I want to change the doctor of an open visit so that the patient is seen today.
- **US-APPT-05** ([A01.07](../../modules/A-core.md#f-a01-07)) — As a doctor I want to record a follow-up plan at the end of a visit so that the next appointment is suggested.

## Test cases

| Done | TC ID | Story | Scenario (Given / When / Then) | +/- | Level | Pri | Status | Spec | Reason |
| ---- | ----- | ----- | ------------------------------ | --- | ----- | --- | ------ | ---- | ------ |
| ☑ | TC-APPT-001 | US-APPT-01 | Given a free slot, when the patient books it, then the appointment is confirmed | + | API | P0 | automated | `tests/api/appointments/book.spec.ts` | |
| ☐ | TC-APPT-002 | US-APPT-01 | Given a taken slot, when the patient books it, then the booking is refused | - | API | P1 | planned | | |
| ☑ | TC-APPT-003 | US-APPT-02 | Given the doctor is on leave, when a booking is made for that day, then it is rejected | - | API | P1 | automated | `tests/api/appointments/day-off.spec.ts` | |
| ☑ | TC-APPT-004 | US-APPT-03 | Given a booked patient arrives, when the clerk checks them in, then a visit opens | + | WEB | P0 | automated | `tests/web/visits/check-in.spec.ts` | |
| ☐ | TC-APPT-005 | US-APPT-03 | Given a cancelled appointment, when the clerk checks the patient in, then check-in is refused | - | API | P1 | planned | | |
| ☐ | TC-APPT-006 | US-APPT-04 | Given an open visit, when the nurse changes the doctor, then the queue of the new doctor shows the patient | + | API | P2 | manual-only | | manual-only:needs two signed-in doctor accounts |
| ☑ | TC-APPT-007 | US-APPT-05 | Given an open visit, when the doctor saves a follow-up plan, then a draft appointment is created | + | API | P1 | automated | `tests/api/visits/follow-up.spec.ts` | |
| ☐ | TC-APPT-008 | US-APPT-05 | Given a closed visit, when the doctor saves a follow-up plan, then the request is rejected | - | GQL | P2 | planned | | |
| ☐ | TC-APPT-009 | US-APPT-05 | Given a follow-up plan, when the patient opens the portal, then the suggestion is shown | + | E2E | P3 | wont | | not-needed:covered by TC-APPT-007 |
