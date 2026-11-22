## What

<!-- What does this PR change? -->

## Why

<!-- The problem or goal. Link the issue: "Closes #123". -->

## How was it tested

- [ ] `cd ai-backend && go build -mod=mod ./... && go test -mod=mod ./...`
- [ ] `cd dashboard && npm run build`
- [ ] `cd playwright-executor && npm test`
- [ ] Other (describe):

## Checklist

- [ ] Conventional commit title (`feat:`, `fix:`, `docs:`, `chore:` …)
- [ ] README contract of each changed directory updated if its interface or invariants changed
- [ ] Schema changes appended to `ai-backend/internal/db/migrations.go` (idempotent)
- [ ] No secrets, real host names, or internal domains in the diff
