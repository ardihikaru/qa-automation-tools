# Tutorial Running Unit Tests

## Scope

This guide is for running **unit tests** in `ai-backend` that do not require a MySQL database or external API connectivity.

## Prerequisites

1. Go installed (project currently tested with Go 1.23.x).
2. Terminal at repo root:

```bash
cd ~/devel/projects/qa-automation-tools
```

## Unit Test Command (Recommended)

Run only unit-style tests in API package by excluding integration test names:

```bash
cd ai-backend
go test ./internal/api -count=1 -run '^(Test(HealthEndpointNoDB|FlowsEndpointNoDB|IssuesEndpointNoDB|SkillsEndpointNoDB|SearchEndpointNoDB|CostOverviewEndpointNoDB|CostTrendEndpointNoDB|CostEfficiencyEndpointNoDB|CostByFeatureEndpointNoDB|NotFoundEndpoint|DocsEndpoint|VideosEndpoint|AnalyticsEndpoint|LearningEndpoint|CORSHeaders|SearchMissingQuery|AdditionalResourceEndpointsNoDB|CreateFlowNoDB|GoalEventsNoDB|GoalEventsPostNoDB|LearningNavigationNoDB|LearningNavigationPostNoDB|LearningSequencesNoDB|LearningSequencesPostNoDB|RolesEndpointNoDB|RolesPostNoDB))$'
```

## Run All Backend Tests

If you want everything (unit + integration) from backend:

```bash
cd ai-backend
go test ./... -count=1
```

## Expected Result

You should see:

```text
ok   github.com/qa-automation/ai-backend/internal/api  <duration>
```

## Common Issues

1. Build cache permission error (sandbox/CI):
- Symptom: read-only write errors in Go build cache.
- Fix: run with proper filesystem permissions in your environment.

2. Tests unexpectedly hitting DB:
- Symptom: connection attempts to MySQL.
- Fix: use the explicit `-run` command above to run unit-only tests.

3. Slow first run:
- Go test cache is disabled by `-count=1`.
- Remove `-count=1` if you want cached runs locally.
