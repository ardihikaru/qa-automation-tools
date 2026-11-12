# skills

## Overview

Package `skills` manages QA "skills": named DOM/data checks (a CSS selector or storage hint plus a severity) that are stored in the MySQL 8 `skills` table, together with an execution log in `skill_logs`. It seeds four built-in skills when the server starts and can create a new skill from observed issues through the AI facade in [services/ai](../ai/README.md). It is a single file, `skillsService.go`.

Wiring: `cmd/server/main.go` builds the service with `skills.NewService(database, aiService)` only after the DB connects, then calls `EnsureBuiltinSkills`. The instance is passed to `api.NewRouter` (see [api](../../api/README.md)), but `internal/api/router.go` currently calls none of its methods.

## Interface

- `skillsService.go:Service`: struct holding `*db.DB` (see [db](../../db/README.md)) and `*ai.Service`.
- `skillsService.go:NewService(database *db.DB, aiService *ai.Service) *Service`
- `skillsService.go:BuiltinSkills() []models.Skill`: returns the built-ins `max_length`, `required_field`, `data_persistence`, `multi_tab_save`.
- `skillsService.go:EnsureBuiltinSkills(ctx) error`
- `skillsService.go:GetSkills(ctx) ([]models.Skill, error)`: active skills only, ordered `is_builtin DESC, name`.
- `skillsService.go:GetSkillByName(ctx, name) (*models.Skill, error)`
- `skillsService.go:LogSkillResult(ctx, log models.SkillLog) error`
- `skillsService.go:GetSkillLogs(ctx, skillID, limit) ([]models.SkillLog, error)`: newest first.
- `skillsService.go:GetSkillStats(ctx) (map[string]interface{}, error)`: keys `total_runs`, `passed`, `failed`, `pass_rate`.
- `skillsService.go:GenerateSkill(ctx, issues []models.RawIssue) (*models.Skill, error)`
- `skillsService.go:ActivateSkill(ctx, name) error` / `skillsService.go:DeactivateSkill(ctx, name) error`
- Types come from `internal/models` (`models.Skill`, `models.SkillLog`, `models.RawIssue`).
- Sibling contracts: [ai](../ai/README.md), [learning](../learning/README.md).

## Invariants

- All SQL uses MySQL `?` placeholders. Booleans are written as `0`/`1` through `skillsService.go:boolToInt` because the columns are `TINYINT(1)`.
- `skillsService.go:EnsureBuiltinSkills` uses `INSERT IGNORE` keyed on the `skills.name UNIQUE` column (DDL in `internal/db/migrations.go`). Re-running it is idempotent and never overwrites an existing row, including a built-in that was deactivated.
- `skillsService.go:GetSkills` filters `is_active = 1`. `skillsService.go:GetSkillByName` does not, so it also returns inactive skills.
- `skillsService.go:GetSkillLogs` defaults `limit` to 50 when `limit <= 0`.
- `skillsService.go:GetSkillStats` sets `pass_rate` to `0.0` when there are no logs, so it never divides by zero.
- `skillsService.go:GenerateSkill` returns an error when `issues` is empty. The AI response (`skillJSON`) is discarded (`_ = skillJSON`). The stored row is a placeholder named `generated_<len(issues)>` with `check_query` `.generated`. It uses a plain `INSERT` (not `INSERT IGNORE`), so a second call with the same issue count fails on the `name` UNIQUE constraint.
- `skillsService.go:GetSkills` and `skillsService.go:GetSkillLogs` return a `nil` slice (not an empty slice) when no rows match.
- `skillsService.go:GenerateSkill` dereferences `aiService`. Integration tests build the service with a `nil` AI service (`internal/api/crud_integration_test.go`), so `GenerateSkill` must not be called in that setup.

## Side Effects

- MySQL writes to `skills`: `skillsService.go:EnsureBuiltinSkills` (INSERT IGNORE), `skillsService.go:GenerateSkill` (INSERT), `skillsService.go:ActivateSkill` / `skillsService.go:DeactivateSkill` (UPDATE `is_active`).
- MySQL writes to `skill_logs`: `skillsService.go:LogSkillResult` (INSERT).
- MySQL reads: `skillsService.go:GetSkills`, `GetSkillByName`, `GetSkillLogs`, `GetSkillStats`.
- LLM network call: `skillsService.go:GenerateSkill` calls `ai.Service.GenerateSkill`, which sends the prompt through `ai.Service.Chat` and the configured `ChatProvider` (ai-manager by default, MiniMax fallback). See [ai](../ai/README.md).

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/services/skills/...`
- This package has no `_test.go` files, so the command reports `[no test files]`. The service is exercised indirectly by the integration tests in `ai-backend/internal/api` (`crud_integration_test.go`, `search_integration_test.go`), which need a live database.
