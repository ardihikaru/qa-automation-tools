# playwright-qa — agent instructions

**Deprecated.** All Playwright execution now lives in [../playwright-executor](../playwright-executor/CLAUDE.md). Make changes there, not here. This package stays only for reference until a cleanup PR deletes it. The contract (frozen) is in [README.md](README.md). The parent rules are in [../CLAUDE.md](../CLAUDE.md).

- Don't add features, and don't wire it into the backend or deploy.
- `src/explorer/guided.ts` and `planner.ts` contain hard-coded staging fallback credentials. They weren't ported, so don't copy them anywhere.
- Only HISv3 **staging** may be crawled. Write-actions against production HISv3 are forbidden.
