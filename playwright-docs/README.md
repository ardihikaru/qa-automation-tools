# Playwright Documentation Generator

Record browser flows and generate documentation automatically using Playwright.

## Requirements

- **Node.js**: >= 18.0.0 (LTS recommended)
- **npm**: >= 9.0.0

## Installation

```bash
# Install dependencies
npm install

# Install Playwright browsers
npx playwright install chromium
```

> Note: This project uses a centralized configuration from the parent directory's `config.yaml` file. Ensure the parent directory has the required node_modules (yaml, dotenv).

## Running

```bash
# Run using npx ts-node (recommended)
npx ts-node src/index.ts

# Or use the npm script
npm run dev
```

## Configuration

The project uses the centralized `config.yaml` from the parent directory. Update the following sections:

- **staging.url**: Base URL for the application
- **credentials**: Role-based login credentials
- **playwright_docs.browser**: Browser settings (headless, viewport, etc.)
- **playwright_docs.flows.roles**: Roles to generate docs for

## Output

Generated documentation is saved to:
- `output/screenshots/`: Screenshot images
- `output/docs/`: Documentation files
- `output/videos/`: Recorded videos (if enabled)

## Project Structure

```
playwright-docs/
├── src/
│   ├── index.ts          # Main entry point
│   ├── flows/
│   │   └── flowLoader.ts # Flow definitions
│   ├── recorder/
│   │   └── recordFlow.ts# Flow recorder
│   └── summarizer/
│       └── summarizeFlow.ts
├── lib/
│   └── config.ts        # Shared config loader (parent directory)
├── output/             # Generated documentation
├── config/            # Local config files
├── package.json
└── tsconfig.json
```

## Dependencies

- **playwright**: Browser automation
- **yaml**: YAML parsing
- **dotenv**: Environment variables
- **@types/node**: Node.js types
- **@types/yaml**: YAML types
- **ts-node**: TypeScript execution
- **typescript**: TypeScript compiler