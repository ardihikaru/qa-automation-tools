# TODO for Playwright-Docs Developer - Senior QA Engineer (Documentation)

## Role
**Senior QA Engineer** - Playwright-Docs (Documentation Generator)

## Project
**AI-Powered QA and System Documentation Platform** - playwright-docs (TypeScript + Playwright)

## Context

You will be implementing the Playwright-Docs module for automated system documentation. The system uses Playwright for UI automation and integrates with Minimax AI for narration.

## Current Status
- **Overall Progress**: 60% complete
- **Completed**: Credential management, login flows, screenshot/video capture, role-based flow mapping
- **Not Implemented**: AI narration, auto menu discovery, doc generation, backend integration

## Reference Files
- `proposed-system-architecture-v1-20260416.md` - Full architecture
- `playwright-docs/src/index.ts` - Main entry point
- `playwright-docs/src/flows/flowLoader.ts` - Flow definitions
- `playwright-docs/src/recorder/recordFlow.ts` - Recording logic
- `playwright-docs/src/summarizer/summarizeFlow.ts` - AI summarization
- `config.yaml` - Configuration with credentials

---

## Task 1: AI Narration Integration (P0 - Critical)

### Description
Integrate Minimax AI for generating video narration. After recording a video, generate narration text using AI and optionally combine with audio.

### Implementation Steps

**1. Create Narration Service**
```typescript
// playwright-docs/src/narrator/narrateFlow.ts
import { callMinimax } from '../summarizer/summarizeFlow';

interface NarrationRequest {
  flowName: string;
  flowDescription: string;
  steps: string[];
  screenshots: string[];
}

interface NarrationResult {
  narration: string;
  audioUrl?: string;
  duration: number;
}

export async function generateNarration(
  request: NarrationRequest,
  config: MinimaxConfig
): Promise<NarrationResult> {
  // 1. Generate narration text using Minimax
  const prompt = `
Generate a narration script for this flow.

Flow: ${request.flowName}
Description: ${request.flowDescription}
Steps:
${request.steps.map((s, i) => `${i + 1}. ${s}`).join('\n')}

Generate a clear, concise narration script (2-3 sentences per step).
  `;
  
  const response = await callMinimax({ prompt }, config);
  
  // 2. Convert to speech using TTS (optional)
  // const audioUrl = await textToSpeech(response.text, config);
  
  return {
    narration: response.text,
    // audioUrl,
    duration: request.steps.length * 5, // Estimate 5 seconds per step
  };
}
```

**2. Integrate with RecordFlow**
- After video recording completes, call narration service
- Store narration text/metadata alongside video

**3. Create Narrator Module**
```
playwright-docs/src/narrator/
├── narrateFlow.ts       # Main narration logic
├── textToSpeech.ts      # TTS integration (optional)
└── config.ts            # Narrator configuration
```

### API Endpoint to Use
- Minimax Chat API for text generation
- Minimax TTS API (optional) for audio

### Expected Time: 3 days

---

## Task 2: Auto Menu Discovery (P0 - Critical)

### Description
Implement automatic discovery of available menus/routes after login. Dynamically build route map based on actual menu items visible.

### Implementation Steps

**1. Menu Extractor**
```typescript
// playwright-docs/src/discover/menuExtractor.ts
import { Page } from 'playwright';

interface DiscoveredMenu {
  name: string;
  path: string;
  icon?: string;
  visible: boolean;
}

export async function extractMenus(page: Page): Promise<DiscoveredMenu[]> {
  const menus: DiscoveredMenu[] = [];
  
  // Common menu selectors (adjust based on app)
  const menuSelectors = [
    'nav a',           // Navigation links
    '.sidebar a',     // Sidebar links
    '[class*="menu"] a', // Menu class
    '.nav a',         // Nav class
  ];
  
  for (const selector of menuSelectors) {
    const elements = await page.locator(selector).all();
    for (const el of elements) {
      const text = await el.textContent();
      const href = await el.getAttribute('href');
      
      if (text && href) {
        menus.push({
          name: text.trim(),
          path: href,
          visible: await el.isVisible(),
        });
      }
    }
  }
  
  // Deduplicate and filter
  return [...new Map(menus.map(m => [m.path, m])).values()];
}
```

**2. Route Discovery**
```typescript
// playwright-docs/src/discover/routeDiscovery.ts
export async function discoverRoutes(
  page: Page,
  baseUrl: string
): Promise<Route[]> {
  const routes: Route[] = [];
  
  // 1. Login first
  await login(page, credentials);
  
  // 2. Extract menus from sidebar
  const menus = await extractMenus(page);
  
  // 3. Visit each discovered route
  for (const menu of menus) {
    try {
      await page.goto(`${baseUrl}${menu.path}`);
      await page.waitForTimeout(1000);
      
      // 4. Check if accessible
      const accessible = await checkPageAccessible(page);
      
      routes.push({
        path: menu.path,
        name: menu.name,
        accessible,
      });
    } catch (e) {
      routes.push({
        path: menu.path,
        name: menu.name,
        accessible: false,
        error: e.message,
      });
    }
  }
  
  return routes;
}
```

**3. Create Discovery Module**
```
playwright-docs/src/discover/
├── menuExtractor.ts    # Extract menus from UI
├── routeDiscovery.ts   # Discover all routes
├── pageAnalyzer.ts     # Analyze page content
└── index.ts            # Main export
```

### Expected Time: 3 days

---

## Task 3: Documentation Generation (P0 - Critical)

### Description
Generate markdown documentation from executed flows using the summarizer.

### Implementation Steps

**1. Enhance Summarizer**
```typescript
// playwright-docs/src/summarizer/generateDocs.ts
import { generateDocumentation } from './summarizeFlow';
import * as fs from 'fs';
import * as path from 'path';

interface FlowDoc {
  flowId: string;
  flowName: string;
  role: string;
  steps: Step[];
  screenshots: string[];
  video?: string;
  generatedAt: string;
}

export async function generateMarkdownDocs(
  flow: FlowExecution,
  outputDir: string
): Promise<string> {
  // 1. Call Minimax for summary
  const summary = await generateDocumentation(
    flow.flowName,
    flow.flowDescription,
    flow.role,
    flow.steps.map(s => s.action),
    config,
    outputDir
  );
  
  // 2. Add screenshots and video references
  const enhancedDoc = enhanceWithMedia(summary, flow);
  
  // 3. Save to file
  const fileName = `${flow.role}-${flow.flowName.toLowerCase().replace(/\s+/g, '-')}.md`;
  const filePath = path.join(outputDir, fileName);
  
  fs.writeFileSync(filePath, enhancedDoc, 'utf8');
  
  return filePath;
}
```

**2. Create Documentation Module**
```
playwright-docs/src/docs/
├── generateDocs.ts     # Generate markdown
├── enhanceWithMedia.ts # Add media references
├── templates/          # Doc templates
│   ├── flow.md
│   └── role.md
└── index.ts
```

**3. Integrate with Main Flow**
- After each flow execution, generate documentation
- Save to `output/docs/` directory

### Expected Time: 2 days

---

## Task 4: Submit to Backend Integration (P1 - High)

### Description
Send flow data to ai-backend after execution for storage and later retrieval.

### API Endpoints to Use
- `POST /api/flows` - Create new flow
- `POST /api/docs` - Create documentation

### Implementation Steps

**1. Create Backend Client**
```typescript
// playwright-docs/src/backend/client.ts
import * as fetch from 'node-fetch';

interface BackendConfig {
  baseUrl: string;
  apiKey?: string;
}

export class BackendClient {
  constructor(private config: BackendConfig) {}
  
  async submitFlow(flow: FlowData): Promise<FlowResponse> {
    const response = await fetch(`${this.config.baseUrl}/api/flows`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${this.config.apiKey}`,
      },
      body: JSON.stringify(flow),
    });
    
    return response.json();
  }
  
  async submitDoc(doc: DocData): Promise<DocResponse> {
    const response = await fetch(`${this.config.baseUrl}/api/docs`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(doc),
    });
    
    return response.json();
  }
}
```

**2. Integrate with Index**
- After flow execution, call backend client
- Submit flow data with steps, screenshots, video
- Handle errors gracefully (continue if backend fails)

### Expected Time: 2 days

---

## Task 5: Comprehensive Route Coverage (P1 - High)

### Description
Test all possible routes per role using auto-discovery and manual additions.

### Implementation Steps

**1. Enhance exploreSystem.ts**
- Use auto menu discovery
- Add fallback manual routes
- Handle different page states

**2. Route Categories**
- Authentication routes
- Dashboard routes
- Feature routes (per role)
- Utility routes (profile, settings)

**3. Error Handling**
- Handle 404 errors
- Handle permission errors
- Handle timeout errors

### Expected Time: 2 days

---

## Task 6: Screenshot Gallery (P2 - Medium)

### Description
Better organization and browsing of captured screenshots.

### Implementation Steps

**1. Organize by Structure**
```
output/screenshots/
├── {staging_url}/
│   ├── {role}/
│   │   ├── {flow}/
│   │   │   ├── step-1.png
│   │   │   ├── step-2.png
│   │   │   └── final.png
│   │   └── index.json (metadata)
```

**2. Create Gallery Viewer**
```typescript
// playwright-docs/src/gallery/viewer.ts
interface GalleryImage {
  path: string;
  role: string;
  flow: string;
  step: number;
  timestamp: string;
}

export function createGalleryIndex(outputDir: string): GalleryImage[] {
  // Scan output directory
  // Create index with metadata
}
```

### Expected Time: 1 day

---

## Task 7: Video Playback Enhancement (P2 - Medium)

### Description
Add controls and step timestamps to video output.

### Implementation Steps

**1. Add Step Markers**
- Embed step timestamps in video metadata
- Create JSON sidecar with timestamps

**2. Create Video Player HTML**
```html
<!-- Simple video player with step markers -->
<video id="player" src="flow.webm"></video>
<div id="timeline">
  <!-- Step markers -->
</div>
```

### Expected Time: 1 day

---

## Implementation Guidelines

### Project Structure
```
playwright-docs/
├── src/
│   ├── index.ts              # Main entry
│   ├── exploreSystem.ts     # Route explorer
│   ├── flows/
│   │   └── flowLoader.ts    # Flow definitions
│   ├── recorder/
│   │   └── recordFlow.ts    # Recording
│   ├── summarizer/
│   │   └── summarizeFlow.ts # AI summarization
│   ├── narrator/             # NEW - AI narration
│   │   ├── narrateFlow.ts
│   │   └── textToSpeech.ts
│   ├── discover/             # NEW - Auto discovery
│   │   ├── menuExtractor.ts
│   │   └── routeDiscovery.ts
│   ├── docs/                 # NEW - Doc generation
│   │   └── generateDocs.ts
│   ├── backend/              # NEW - Backend client
│   │   └── client.ts
│   └── gallery/              # NEW - Screenshot gallery
│       └── viewer.ts
├── config/
│   └── credentials.yaml
└── output/
    ├── screenshots/
    ├── videos/
    └── docs/
```

### API Keys Configuration
```yaml
# config.yaml
minimax:
  api_key: "your-api-key"
  base_url: "https://api.minimax.chat/v1"
  
backend:
  url: "https://ai-backend-1087642958941.asia-southeast1.run.app"
```

### Testing
- Test with multiple roles
- Test with different browsers
- Verify video quality
- Verify screenshot resolution

### Error Handling
- Log errors to file
- Continue on error (don't stop whole run)
- Retry failed steps

---

## Dependencies to Install

```bash
# For TTS (optional)
# npm install @minimax/tts

# Already installed
npm install playwright
npm install yaml
npm install dotenv
```

---

## Notes

1. Ensure video recording is enabled in config
2. Use headless mode for production runs
3. Configure appropriate timeouts
4. Handle rate limiting from AI APIs
5. Store credentials securely

---

*Document Version: 1.0*
*Created: 2026-04-17*
*Assigned To: Senior QA Engineer (Playwright-Docs)*