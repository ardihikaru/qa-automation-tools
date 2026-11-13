# testdata

`hisv3-521618a/` is a verbatim copy of the `.md` files under `docs/` of `developersismedika/automation-test-hisv3` at commit `521618a`. At that commit `node scripts/test-catalog.mjs --check` prints "Catalog OK" and "444 features, 246 stories (9 unmapped), 1659 test cases"; `TestHISv3Golden` asserts those numbers plus 4 parts, 39 modules, 46 epics, 456 automated cases and 200 sub-modules.

Refresh: `git -C <automation-test-hisv3> archive <sha> docs | tar -x -C hisv3-<sha>/`, delete non-`.md` files, update the directory name, the test's path and the expected numbers from the script's output.
