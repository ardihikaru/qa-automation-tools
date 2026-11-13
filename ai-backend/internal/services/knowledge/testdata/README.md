# testdata

`sut-521618a/` is a verbatim copy of the `.md` files under `docs/` of `example-org/automation-test-sut` at commit `521618a`. At that commit `node scripts/test-catalog.mjs --check` prints "Catalog OK" and "444 features, 246 stories (9 unmapped), 1659 test cases"; `TestSUTGolden` asserts those numbers plus 4 parts, 39 modules, 46 epics, 456 automated cases and 200 sub-modules.

Refresh: `git -C <automation-test-sut> archive <sha> docs | tar -x -C sut-<sha>/`, delete non-`.md` files, update the directory name, the test's path and the expected numbers from the script's output.
