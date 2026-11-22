# Partner API access and health (partner-api)

**Goal:** partner systems authenticate with API keys and are rate limited fairly.

## User stories

- **US-VSYS-01** ([C01.01](../../modules/C-partner-api.md#f-c01-01)) — As a partner integrator I want requests signed with a valid API key accepted and others refused so that only known partners get in.
- **US-VSYS-02** ([C01.02](../../modules/C-partner-api.md#f-c01-02)) — As an operator I want each partner rate limited so that one partner cannot starve the others.
- **US-VSYS-03** (unmapped: the health endpoint is infrastructure, not a product feature) — As an operator I want GET /health to answer without a key so that monitoring works.

## Test cases

| Done | TC ID | Story | Scenario (Given / When / Then) | +/- | Level | Pri | Status | Spec | Reason |
| ---- | ----- | ----- | ------------------------------ | --- | ----- | --- | ------ | ---- | ------ |
| ☑ | TC-VSYS-001 | US-VSYS-01 | Given a valid key, when the partner calls GET /v1/clinics, then the response is 200 | + | API | P0 | automated | `tests/api/partner/auth.spec.ts` | |
| ☑ | TC-VSYS-002 | US-VSYS-01 | Given a revoked key, when the partner calls GET /v1/clinics, then the response is 401 | - | API | P0 | automated | `tests/api/partner/auth.spec.ts` | |
| ☐ | TC-VSYS-003 | US-VSYS-01 | Given a malformed Authorization header, when the partner calls any route, then the response is 401 | - | API | P1 | planned | | |
| ☑ | TC-VSYS-004 | US-VSYS-02 | Given a partner over its limit, when it sends one more request, then the response is 429 | - | API | P1 | automated | `tests/api/partner/rate-limit.spec.ts` | |
| ☐ | TC-VSYS-005 | US-VSYS-02 | Given a partner under its limit, when it sends a burst, then every request succeeds | + | API | P2 | planned | | |
| ☑ | TC-VSYS-006 | US-VSYS-03 | Given no key, when GET /health is called on api.example.test, then the response is 200 | + | API | P3 | automated | `tests/api/partner/health.spec.ts` | |
