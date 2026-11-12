# learning

## Overview

Package `learning` stores and ranks what the QA explorer learns as it runs. It keeps four MySQL 8 stores, all accessed through the `*db.DB` wrapper (see [db](../../db/README.md)):

- `learning_cache`: intent/action/page records with a score.
- `navigation_edges`: page-to-page transitions with success rate and average duration.
- `action_sequences`: goal to step-list records.
- `context_cache`: context key/value to best action.

It also exports small hashing and timing helpers. It is a single file, `learningService.go`, and makes no LLM calls.

Wiring: `cmd/server/main.go` builds the service with `learning.NewService(database)` only after the DB connects. `internal/api/router.go` (see [api](../../api/README.md)) calls `GetLearningStats`, `GetNavigationEdges`, `AddNavigationEdge`, `GetActionSequences`, `AddActionSequence`, `GetContextRecord`, and `AddContextRecord`.

## Interface

- `learningService.go:Service`, `learningService.go:NewService(database *db.DB) *Service`
- Learning cache:
  - `learningService.go:AddToCache(ctx, intent, action, page string, success bool) error`
  - `learningService.go:Search(ctx, intent string, limit int) ([]models.LearningRecord, error)`
  - `learningService.go:GetBestAction(ctx, intent, page string) (*models.LearningRecord, error)`
  - `learningService.go:ApplyDecay(ctx) error`
  - `learningService.go:GetStats(ctx) (map[string]interface{}, error)`
  - `learningService.go:ClearOldRecords(ctx, days int) error`
- Navigation edges:
  - `learningService.go:AddNavigationEdge(ctx, sourcePage, targetPage, action string, success bool, durationMs int) error`
  - `learningService.go:GetNavigationEdges(ctx, sourcePage) ([]models.NavigationEdge, error)`
  - `learningService.go:GetBestNextPage(ctx, currentPage) (*models.NavigationEdge, error)`
- Action sequences:
  - `learningService.go:AddActionSequence(ctx, goal, steps string, success bool, durationMs int) error`
  - `learningService.go:GetActionSequences(ctx, goal) ([]models.ActionSequence, error)`
  - `learningService.go:GetBestActionSequence(ctx, goal) (*models.ActionSequence, error)`
- Context cache:
  - `learningService.go:AddContextRecord(ctx, contextKey, contextValue, bestAction string, success bool) error`
  - `learningService.go:GetContextRecord(ctx, contextKey, contextValue) (*models.ContextRecord, error)`
- Aggregate: `learningService.go:GetLearningStats(ctx) (map[string]interface{}, error)` returns the keys `learning_cache`, `navigation_edges`, and `action_sequences`.
- Helpers:
  - `learningService.go:HashInput(input string) string` returns SHA-256 as hex.
  - `learningService.go:HashToJSON(v interface{}) string`
  - `learningService.go:TimeTrack(start time.Time, name string)`
- Sibling contracts: [skills](../skills/README.md), [ai](../ai/README.md).

## Invariants

- **Upserts are not atomic.** `AddToCache`, `AddNavigationEdge`, `AddActionSequence`, and `AddContextRecord` run a `SELECT id` first, then `UPDATE` on a hit or `INSERT` on `sql.ErrNoRows`. None of the four tables has a UNIQUE key on the upsert columns (DDL in `internal/db/migrations.go`), so concurrent calls can insert duplicate rows.
- **Score bounds** (`learningService.go:AddToCache`, `learningService.go:AddContextRecord`):
  - A new row starts at `success_score` 1.0 on success or 0.5 on failure.
  - An existing row moves by ±0.1, clamped to [0.0, 1.0] with `LEAST`/`GREATEST`.
  - `last_success` is updated only on success.
- `learningService.go:AddNavigationEdge` computes `success_rate` and a running `avg_duration_ms` inside the same `UPDATE`, after incrementing the counts. This depends on MySQL evaluating single-table `SET` assignments left to right, so the column order in that statement matters. New edges are inserted with `success_rate` 0.5.
- `learningService.go:AddActionSequence` sets `is_valid` to the latest outcome (`boolToInt(success)`). One failure therefore hides the sequence from `GetActionSequences` and `GetBestActionSequence`, which both filter `is_valid = 1`.
- `learningService.go:GetBestAction` accepts `intent` but filters only by `page`. The `intent` argument is unused.
- **Defaults:**
  - `learningService.go:Search` uses `limit` 10 when `limit <= 0`. It matches by substring (`LIKE CONCAT('%', ?, '%')`).
  - `learningService.go:ClearOldRecords` uses `days` 30 when `days <= 0`. It deletes only rows with `usage_count < 3`.
- `learningService.go:ApplyDecay` multiplies `success_score` by 0.95 only for rows with `updated_at` older than 7 days and `success_score > 0.1`.
- **Empty results:** `GetNavigationEdges` and `GetActionSequences` return an empty slice, never `nil`. `Search` can return `nil`.
- **Stats:**
  - `learningService.go:GetStats` and `learningService.go:GetLearningStats` cast averages to `UNSIGNED` and scan them into `int`, so fractional scores are rounded to whole numbers.
  - In `GetStats`, the `SUM(...)` for `high_success` is not wrapped in `COALESCE`. On an empty table that scan can fail.
  - `GetLearningStats` ignores per-query errors. It leaves the failed key out and still returns a `nil` error.
- `learningService.go:HashToJSON` ignores `json.Marshal` errors.

## Side Effects

- MySQL writes (INSERT/UPDATE/DELETE):
  - `learning_cache`: `AddToCache`, `ApplyDecay`, `ClearOldRecords`
  - `navigation_edges`: `AddNavigationEdge`
  - `action_sequences`: `AddActionSequence`
  - `context_cache`: `AddContextRecord`
- MySQL reads: every `Get*` method and `Search`.
- Stdout: `learningService.go:TimeTrack` writes a timing line with `fmt.Printf`.
- No network or LLM calls.

## Verification

- Run: `cd ai-backend && go test -mod=mod ./internal/services/learning/...`
- This package has no `_test.go` files, so the command reports `[no test files]`. The router-facing methods are exercised by the integration tests in `ai-backend/internal/api`, which need a live database.
