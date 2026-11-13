# Doctor SIP status by username (his-backend GraphQL `doctorSipStatusGet`)

**Goal:** an administrator or a doctor can see, for one user, whether the SIP (surat izin praktik) is expired or about to expire, its date range and the days left, without exposing the SIP number, and a plain user can never read someone else's status.

**Docs:** module features: [module catalogue](../../modules/README.md) (linked on every story).

Surface under test: `doctorSipStatusGet(username: String)` of the his-backend `practitioner` module (`…/practitioner/query`). `username` is optional (defaults to the access token's user); another user's username needs the Administrator or Super Admin group.

The spec skips (with the reason) on a backend that does not expose the query yet, and per case when the environment has no data for it (for example no doctor whose SIP ends within a month). Expected values come from an independent read-only SELECT on the staging DB, and the days are recomputed on the WIB calendar date.

Accounts: `author` (`HIS_AUTH_*`) must be in Administrator or Super Admin; `viewer` (`CHARTMGMT_VIEWER_*`) must be in neither.

## User stories

- **US-SIPSTAT-01** ([A13.06](../../modules/A-core.md#f-a13-06), [A11.05](../../modules/A-core.md#f-a11-05)) — As an administrator I want the SIP status, date range and days left of a doctor so that I can chase renewals before the doctor must stop practising.
- **US-SIPSTAT-02** ([A13.06](../../modules/A-core.md#f-a13-06), [A11.05](../../modules/A-core.md#f-a11-05)) — As a user I want to see my own SIP status without typing a username, while nobody but an Administrator or Super Admin can look at other people.
- **US-SIPSTAT-03** ([A13.06](../../modules/A-core.md#f-a13-06), [A11.05](../../modules/A-core.md#f-a11-05)) — As a security owner I want the SIP number never to leave the backend through this query.

## Test cases

| Done | TC ID          | Story         | Scenario (Given / When / Then)                                                                                                                                                          | +/- | Level | Pri | Status    | Spec                                                                   | Reason |
| ---- | -------------- | ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --- | ----- | --- | --------- | ---------------------------------------------------------------------- | ------ |
| ☑    | TC-SIPSTAT-001 | US-SIPSTAT-01 | Given a doctor whose SIP already ended, when an administrator asks for the status, then it is EXPIRED, isExpired is true and daysLeft is the negative number of days since the end date | +   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-002 | US-SIPSTAT-01 | Given a doctor whose SIP ends within 30 days (today included), when asked, then it is UNDER_1_MONTH with daysLeft 0..30                                                                 | +   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-003 | US-SIPSTAT-01 | Given a doctor whose SIP ends in 31 to 180 days, when asked, then it is UNDER_6_MONTHS                                                                                                  | +   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-004 | US-SIPSTAT-01 | Given a doctor whose SIP ends in more than 180 days, when asked, then it is NORMAL                                                                                                      | +   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-005 | US-SIPSTAT-03 | Given the DoctorSipStatus type, when its fields are introspected, then the SIP number is not among them                                                                                 | +   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-006 | US-SIPSTAT-01 | Given a doctor username in another letter case, when asked, then the same user is found                                                                                                 | +   | API   | P1  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-007 | US-SIPSTAT-01 | Given a doctor without any SIP row, when asked, then it is NO_SIP and has no dates                                                                                                      | +   | API   | P1  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-008 | US-SIPSTAT-01 | Given a doctor whose latest SIP has no end date, when asked, then it is NO_SIP and has no daysLeft                                                                                      | +   | API   | P1  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-009 | US-SIPSTAT-01 | Given an account that is not a practitioner (administrator, finance ...), when asked, then it is NOT_APPLICABLE with status true                                                        | +   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-010 | US-SIPSTAT-01 | Given a practitioner who is not a doctor (nurse ...), when asked, then it is NOT_APPLICABLE with status true                                                                            | +   | API   | P1  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-011 | US-SIPSTAT-01 | Given a username that does not exist, when asked, then it is NOT_APPLICABLE, not an error                                                                                               | -   | API   | P1  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-012 | US-SIPSTAT-02 | Given no username (omitted, null, empty or blank), when asked, then the user of the access token is evaluated                                                                           | +   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-013 | US-SIPSTAT-02 | Given a plain (non-admin) user, when they ask for their own username in any letter case, then it is allowed                                                                             | +   | API   | P1  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-014 | US-SIPSTAT-02 | Given a plain (non-admin) user, when they ask for another user, then status is false, there is no data and the message names Administrator / Super Admin                                | -   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |
| ☑    | TC-SIPSTAT-015 | US-SIPSTAT-02 | Given a request without an access token, when the query is sent, then it is rejected                                                                                                    | -   | API   | P0  | automated | `tests/api/graphql/his-backend/practitioner/doctor-sip-status.spec.ts` |        |

## Not covered by this epic

- Other medical roles (nurse, midwife, pharmacist) have their own licences; for now they are `NOT_APPLICABLE`.
- A frontend consumer of the query does not exist yet.
