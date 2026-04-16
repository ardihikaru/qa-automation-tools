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

## Test Scenarios

### Production API

Production API: https://ai-backend-1087642958941.asia-southeast1.run.app

See `TEST-SCENARIO.md.example` for complete test scenarios.

### Documentation

Run recorder to capture flows:

```bash
cd playwright-docs
npm install
npm run recorder
```

Output files:
- `output/videos/*.mp4` - Recorded videos
- `output/docs/*.md` - Generated documentation