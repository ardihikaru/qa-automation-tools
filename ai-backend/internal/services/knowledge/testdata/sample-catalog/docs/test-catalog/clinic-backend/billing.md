# Billing (clinic-backend)

**Goal:** cashiers bill visit services correctly and take payments.

## User stories

- **US-BILL-01** ([B01.001](../../modules/B-billing.md#f-b01-001)) — As a cashier I want an invoice built from the services of a visit so that nothing is billed by hand.
- **US-BILL-02** ([B01.001](../../modules/B-billing.md#f-b01-001), [B01.002](../../modules/B-billing.md#f-b01-002)) — As a cashier I want to apply an approved discount so that the invoice total is correct.
- **US-BILL-03** ([B01.003](../../modules/B-billing.md#f-b01-003)) — As a cashier I want to take a card payment and print a receipt so that the visit can be closed.
- **US-BILL-04** (unmapped: the end-of-day cash count is a back-office routine with no product feature) — As a head cashier I want to reconcile the cash drawer so that the day can be closed.

## Test cases

| Done | TC ID | Story | Scenario (Given / When / Then) | +/- | Level | Pri | Status | Spec | Reason |
| ---- | ----- | ----- | ------------------------------ | --- | ----- | --- | ------ | ---- | ------ |
| ☑ | TC-BILL-001 | US-BILL-01 | Given a visit with two services, when the invoice is built, then it has two lines and the right total | + | API | P0 | automated | `tests/api/billing/invoice.spec.ts` | |
| ☐ | TC-BILL-002 | US-BILL-01 | Given a visit with no services, when the invoice is built, then an empty invoice is refused | - | API | P1 | planned | | |
| ☐ | TC-BILL-003 | US-BILL-02 | Given an approved 10% discount, when it is applied, then the total drops by 10% | + | API | P1 | planned | | |
| ☐ | TC-BILL-004 | US-BILL-02 | Given no approval, when a discount is applied, then it is rejected | - | API | P2 | blocked | | blocked:the discount-approver role is missing on staging |
| ☑ | TC-BILL-005 | US-BILL-03 | Given an open invoice, when a card payment settles it, then a receipt is printed | + | WEB | P1 | automated | `tests/web/billing/payment.spec.ts` | |
| ☐ | TC-BILL-006 | US-BILL-04 | Given a day of payments, when the head cashier counts the drawer, then the difference is zero | + | E2E | P2 | manual-only | | manual-only:needs a physical cash drawer |
