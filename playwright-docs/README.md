# Playwright Documentation Generator

Record flows and generate documentation.

## Structure

| Module | Responsibility |
|--------|-------------|
| `flows/` | Role-based flow definitions (admin.ts, doctor.ts, cashier.ts) |
| `recorder/` | Video and screenshot capture |
| `summarizer/` | Call Minimax for flow description |
| `output/videos/` | Generated MP4 files |
| `output/docs/` | Generated markdown files |
| `config/` | Configuration (credentials.yaml - gitignored) |

## Key Feature

Lightweight - no issue detection, just documentation generation.