# Partner insurance claims (partner-api)

**Goal:** insurer partners receive claims for settled invoices and report their status back.

## User stories

- **US-VCLAIM-01** ([C02.01](../../../modules/C-partner-api.md#f-c02-01)) — As an insurer partner I want to receive a claim when an invoice is settled so that I can process it.
- **US-VCLAIM-02** ([C02.01](../../../modules/C-partner-api.md#f-c02-01), [C02.02](../../../modules/C-partner-api.md#f-c02-02)) — As a cashier I want the claim status updated from the insurer's callback so that I know when it is paid.

## Test cases

| Done | TC ID | Story | Scenario (Given / When / Then) | +/- | Level | Pri | Status | Spec | Reason |
| ---- | ----- | ----- | ------------------------------ | --- | ----- | --- | ------ | ---- | ------ |
| ☑ | TC-VCLAIM-001 | US-VCLAIM-01 | Given a settled invoice, when the claim job runs, then the insurer receives one claim | + | API | P0 | automated | `tests/api/partner/claims-submit.spec.ts` | |
| ☐ | TC-VCLAIM-002 | US-VCLAIM-01 | Given an unsettled invoice, when the claim job runs, then no claim is sent | - | API | P1 | planned | | |
| ☐ | TC-VCLAIM-003 | US-VCLAIM-02 | Given a sent claim, when the insurer posts status "paid", then the claim shows paid | + | API | P1 | planned | | |
| ☐ | TC-VCLAIM-004 | US-VCLAIM-02 | Given an unknown claim id, when the insurer posts a status, then the callback is retried | - | API | P2 | wont | | not-needed:the partner retries on its own |
