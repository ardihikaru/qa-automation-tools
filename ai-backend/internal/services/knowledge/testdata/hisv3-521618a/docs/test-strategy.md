# Test strategy and QA concepts

Which QA concepts this repository follows, how test cases are documented, and which agent helps at which test level.
The checklist itself is the [test catalog](test-catalog/README.md). The rules for writing code are in [../AGENTS.md](../AGENTS.md).

## 1. Test levels (pyramid for services)

| Level           | What it checks                                                                   | Where it lives                                                                                | Speed |
| --------------- | -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | ----- |
| **Unit**        | One function/class in isolation, dependencies mocked                             | **Product repos** (not in this repo)                                                          | ms    |
| **Integration** | One service with its real collaborators (DB, queue, auth) through its public API | Product repos for deep integration; **`tests/api/**` here** for single-service contract tests | s     |
| **E2E**         | A user journey across services and/or the UI, real backend                       | **`tests/web/**`, cross-service specs here**                                                  | s–min |

Rules of thumb: test each rule at the **lowest level that can show it**; keep E2E for journeys and contracts between
services. This repository is E2E and API-level only. It has no unit-test runner; adding one needs human approval (AGENTS.md §6.4).

## 2. Concepts used

- **Risk-based testing.** Every case gets a priority: `P0` revenue/security critical → `@smoke`, `P1` important → `@regression`,
  `P2` normal, `P3` edge. Automate P0/P1 first.
- **Traceability.** Module feature → Epic → User story → Scenario → Test case → Spec. The product map (Part → Module →
  Sub-module → Feature, with release status) is [docs/modules/](modules/README.md); every story names the feature(s) it serves,
  so features without automation show up as the backlog. Stories follow INVEST ("As a … I want … so that …").
- **Scenarios in Given / When / Then.** Used as documentation in one line per case; there is no Cucumber/Gherkin runner.
- **Positive and negative cases.** Every behaviour needs `+` (it works) and `-` (it is refused correctly). Negative cases are
  derived with these techniques:
  - _Equivalence partitioning_ — one case per class of invalid input (missing, wrong type, empty, too long).
  - _Boundary value analysis_ — values at and around limits (quantity 0/1, max length, price 0/0.01).
  - _Decision / state transition_ — role × action matrix; allowed and forbidden state changes (cancel a cancelled order).
  - _Error guessing_ — duplicates, expired tokens, double submits, unknown ids.
  - REST checklist per endpoint (AGENTS.md §5.3): 401, 403, 400, 404, 409/422.
- **Independent, data-owning tests.** Each test creates and cleans its own data (AGENTS.md §2).
- **Shift-left.** Cases are written as catalog rows when the story is refined, before the spec exists.

## 3. IDs and statuses

| Item       | Pattern                        | Example       |
| ---------- | ------------------------------ | ------------- |
| Feature    | `<Part><NN>.<NN>[.<NN>]`       | `A01.07`      |
| Epic code  | 2–5 capital letters            | `AUTH`, `CP`  |
| User story | `US-<EPIC>-NN`                 | `US-AUTH-01`  |
| Test case  | `TC-<EPIC>-NNN` (never reused) | `TC-AUTH-002` |

Status lifecycle: `planned` → `automated`; or `blocked` (needs a ticket), `manual-only`, `wont` (decided not to test). Every non-automatable
row carries a `Reason` (`defect`, `not-needed`, `blocked`, `unsafe`, `manual-only`, `deferred`, `other`) — see
[test-catalog/README.md](test-catalog/README.md#not-every-case-is-automated). A case is **covered** only when it is `automated`, its spec exists and passes.

## 4. Working with the catalog

1. **Plan:** pick the feature in [docs/modules/](modules/README.md) (the `DEV-DONE` / `PROD` backlog first), add the story with
   its feature id(s) and its `+`/`-` rows as `planned` (use the `test-case-planner` agent).
2. **Automate:** write the spec following the AGENTS.md recipes; put the TC ID in the PR description. Spec titles stay
   behaviour sentences.
3. **Close:** set the row to `automated`, tick `Done`, link the spec path.
4. **Verify:** `npm run catalog` (structure, duplicate IDs, spec files exist, feature ids and release statuses) and
   `npm run catalog:report` (coverage table, story links, module columns, backlog, `docs/modules/traceability.md`).

Relation to tags: `P0` → `Tag.smoke`, `P1` → `Tag.regression` (AGENTS.md §5.9).

## 5. Agents

Defined in `.claude/agents/` (Claude Code). Other tools can follow the same playbooks by reading the agent files; the rules always
come from AGENTS.md. They do not clash with the `playwright-test-*` agents created by `npx playwright init-agents`.

| Agent                      | Use it to                                                                                           | Writes code?        |
| -------------------------- | --------------------------------------------------------------------------------------------------- | ------------------- |
| `test-case-planner`        | turn a story/ticket/API spec into catalog rows (+/-, priority, technique)                           | no (docs)           |
| `e2e-test-engineer`        | write REST/GraphQL/web specs in this repo's structure and update the catalog                        | yes                 |
| `integration-test-advisor` | decide what an integration test is for a service; here = `tests/api/**`; guidance for product repos | only API specs here |
| `unit-test-advisor`        | guide unit tests in the product repos (what to unit-test, mocking boundaries, naming)               | no                  |
