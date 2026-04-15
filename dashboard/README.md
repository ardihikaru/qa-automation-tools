# Dashboard (React)

UI for exploring docs, viewing issues, tracking fixes.

## Structure

| View | Responsibility |
|------|-------------|
| `src/views/` | Page views (Docs, QA, Analytics, Settings) |
| `src/components/` | Reusable UI components |
| `src/hooks/` | Custom React hooks |
| `src/utils/` | Utility functions |

## Views

| View | Description |
|------|-------------|
| `Docs View` | Browse videos/docs |
| `QA Insights` | Skills failures summary |
| `AI Debugger` | Issues with suggestions + patches |
| `Cost Analytics` | AI usage costs |
| `Fix Tracking` | is_fixed checkboxes + notes |

## Tech Stack

- React
- Tailwind CSS
- Recharts (charts)
- React Router