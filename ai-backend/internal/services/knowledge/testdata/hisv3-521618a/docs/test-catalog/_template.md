# <Epic name> (<service or app>)

<!-- Copy into docs/test-catalog/<service>/[<area>/]<epic>.md; relative links below assume one folder deep (add ../ per extra level). -->

**Goal:** one sentence — what value this area gives and who needs it.

**Docs:** test doc [<name>-testing.md](../../<name>-testing.md) · module features: [module catalogue](../../modules/README.md) (linked on every story).

## User stories

- **US-<EPIC>-01** (<feature id from ../modules/, e.g. A01.07>) — As a <role> I want <capability> so that <benefit>.

## Test cases

One row per test case. `Done` is ☑ exactly when Status is `automated`. Scenario = Given / When / Then in one line.
Cover happy paths (+) and negative paths (-); see ../test-strategy.md for the techniques.

| Done | TC ID         | Story        | Scenario (Given / When / Then) | +/- | Level | Pri | Status  | Spec | Reason |
| ---- | ------------- | ------------ | ------------------------------ | --- | ----- | --- | ------- | ---- | ------ |
| ☐    | TC-<EPIC>-001 | US-<EPIC>-01 | Given …, when …, then …        | +   | API   | P0  | planned |      |
