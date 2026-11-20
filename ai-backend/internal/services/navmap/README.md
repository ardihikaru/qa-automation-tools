# services/navmap

## Overview
Pure builder of a product's route and page inventory for the navigation map ([NAVIGATION_MAP.md](../../../../docs/designs/NAVIGATION_MAP.md)). No database, clock or I/O; it does not import `knowledge` (the sync converts its snapshot to `Input`).

## Interface
- `Build(Input) Inventory` (`navmap.go`): routes (`surface` web|api, `app`, normalised `path`, methods, sources, page objects), links (`feature`/`case`/`spec` → route, with a source and method) and problems. `Version` changes whenever the output for the same input changes.
- `NormPath`, `Match`, `RouteKey`, `Segments`, `IsParam` (`path.go`).
- `KeepRepoFile` (`repo.go`): the repo files the scan reads.
- `CaseSets`, `Coverage`, `Next` (`coverage.go`): per-route case sets (with wildcard and prefix roll-up), status (`failing`, `passing`, `automated`, `planned`, `untested`) and the "next to automate" tiers.

## Invariants
- Deterministic: outputs are sorted; input order does not matter (tested by shuffling).
- Sources: feature `repo_entry`/name backticks (catalog), the **first** `METHOD /path` of a case scenario (scenario), page objects / app members / fixtures / REST client JSDoc and calls / spec test blocks (repo, spec). A route known only from negative cases' scenarios is dropped.
- api routes get their app's mount (`/api/vN`, most frequent among all its routes) when they do not start with `/api/`.
- `NormPath` drops scheme, host, query and fragment; turns `<x>`, `{x}`, `${…}` into parameters and id-like segments into `:id`; rejects `..`, whitespace, quotes and parameter-only paths.
- Only `scenario` and `spec` links (and observations, added by the API) are evidence; `feature` links are context.
- A file over 256 KiB is skipped with a problem.

## Side Effects
None.

## Verification
`go test -mod=mod ./internal/services/navmap/`. Against a local SUT checkout: `NAV_LOCAL_REPO=… go test -mod=mod -run TestNavLocalRepo ./internal/services/knowledge/ -v`.
