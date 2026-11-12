/**
 * Flow recorder - Record video and screenshots for each flow
 * Updated to use centralized config
 */

import { Page } from 'playwright';
import * as fs from 'fs';
import * as path from 'path';

export interface Credential {
  username: string;
  password: string;
  role_code: string;
}

// Updated to use config types
export interface FlowConfig {
  staging: {
    url: string;
    timeout: number;
  };
  api: {
    backend_url: string;
    /** @deprecated kept for compatibility; use ai */
    minimax_api_key: string;
    ai?: import('../../../lib/config').AIConfig;
  };
}

// Import types from flowLoader
import { FlowDefinition, FlowStep } from '../flows/flowLoader';

interface RecorderOptions {
  outputDir?: string;
  videoDir?: string;
  screenshotDir?: string;
  defaultWait?: number;
}

const defaultOptions: RecorderOptions = {
  outputDir: 'output',
  videoDir: 'videos',
  screenshotDir: 'screenshots',
  defaultWait: 2000,
};

/**
 * Capture screenshot
 */
async function captureScreenshot(
  page: Page,
  name: string,
  dir: string
): Promise<string> {
  // Ensure directory exists
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }

  const filePath = path.join(dir, `${name}.png`);
  await page.screenshot({
    path: filePath,
    fullPage: false,
  });
  return filePath;
}

/**
 * Replace placeholders in values
 */
function replacePlaceholders(value: string, credentials: Credential): string {
  return value
    .replace('{{username}}', credentials.username)
    .replace('{{password}}', credentials.password)
    .replace('{{role_code}}', credentials.role_code);
}

/**
 * Wait for element to be visible before interaction
 */
async function waitForElementVisible(
  page: Page,
  selector: string,
  timeout: number = 10000
): Promise<boolean> {
  try {
    await page.waitForSelector(selector, {
      state: 'visible',
      timeout,
    });
    return true;
  } catch {
    console.log(`    ⚠️ Element not found: ${selector}`);
    return false;
  }
}

/**
 * Try to fill using multiple selector strategies
 */
async function tryFillWithFallback(
  page: Page,
  selectors: string,
  value: string
): Promise<boolean> {
  const selectorList = selectors.split(',').map(s => s.trim());
  
  for (const selector of selectorList) {
    try {
      if (await page.locator(selector).first().isVisible({ timeout: 2000 })) {
        await page.fill(selector, value);
        console.log(`    ✅ Filled: ${selector}`);
        return true;
      }
    } catch {
      continue;
    }
  }
  
  console.log(`    ❌ Could not fill any selector: ${selectors}`);
  return false;
}

/**
 * Try to click using multiple selector strategies
 */
async function tryClickWithFallback(
  page: Page,
  selectors: string
): Promise<boolean> {
  const selectorList = selectors.split(',').map(s => s.trim());
  
  for (const selector of selectorList) {
    try {
      if (await page.locator(selector).first().isVisible({ timeout: 2000 })) {
        await page.click(selector);
        console.log(`    ✅ Clicked: ${selector}`);
        return true;
      }
    } catch {
      continue;
    }
  }
  
  console.log(`    ❌ Could not click any selector: ${selectors}`);
  return false;
}

/**
 * Record a single flow
 */
export async function recordFlow(
  page: Page,
  flow: FlowDefinition,
  credentials: Credential,
  config: FlowConfig,
  options: RecorderOptions = defaultOptions
): Promise<void> {
  const outputDir = options.outputDir || 'output';
  
  // Organize screenshots by staging URL and role_code
  const stagingUrl = config.staging.url.replace(/https?:\/\//, '');
  const screenshotDir = path.join(outputDir, options.screenshotDir || 'screenshots', stagingUrl, credentials.role_code);
  const defaultWait = options.defaultWait || 2000;
  
  // Ensure directories exist
  if (!fs.existsSync(screenshotDir)) {
    fs.mkdirSync(screenshotDir, { recursive: true });
  }

  // Get staging URL
  const baseUrl = config.staging.url;

  console.log(`    🌐 Base URL: ${baseUrl}`);

  // Execute flow steps
  for (let i = 0; i < flow.steps.length; i++) {
    const step = flow.steps[i];

    try {
      console.log(`    Step ${i + 1}: ${step.action} ${step.selector || ''}`);
      
      switch (step.action) {
        case 'navigate':
          const fullUrl = baseUrl.replace(/\/$/, '') + (step.selector || '');
          console.log(`    → Navigating to: ${fullUrl}`);
          await page.goto(fullUrl, {
            waitUntil: 'domcontentloaded',
            timeout: config.staging.timeout,
          });
          // Wait for page to stabilize
          await page.waitForTimeout(defaultWait);
          break;

        case 'click':
          if (step.selector) {
            await tryClickWithFallback(page, step.selector);
          }
          break;

        case 'type':
          const value = step.value
            ? replacePlaceholders(step.value, credentials)
            : '';
          console.log(`    → Typing: ${value}`);
          if (step.selector) {
            // Clear field first, then fill using fallback
            try {
              await page.locator(step.selector).first().clear();
            } catch {}
            await tryFillWithFallback(page, step.selector, value);
          }
          break;

        case 'select':
          // Wait for element to be visible first
          if (step.value && step.selector && await waitForElementVisible(page, step.selector, 10000)) {
            await page.selectOption(step.selector, step.value);
          }
          break;

        case 'wait':
          if (step.wait) {
            console.log(`    → Waiting ${step.wait}ms`);
            await page.waitForTimeout(step.wait);
          }
          break;
      }

      // Small delay after each action for stability
      await page.waitForTimeout(500);

      // Capture screenshot after each step
      const screenshotName = `${flow.id}-step-${i + 1}`;
      await captureScreenshot(page, screenshotName, screenshotDir);
    } catch (error) {
      console.error(`  ❌ Step ${i + 1} failed:`, error);
      // Try to capture screenshot on failure
      try {
        await captureScreenshot(page, `${flow.id}-error-step-${i + 1}`, screenshotDir);
      } catch {}
    }
  }

  // Final screenshot
  await captureScreenshot(page, `${flow.id}-final`, screenshotDir);

  console.log(`  📸 Recorded ${flow.steps.length + 1} screenshots`);
}

/**
 * Wait for element
 */
async function waitForElement(
  page: Page,
  selector: string,
  timeout: number = 5000
): Promise<boolean> {
  try {
    await page.waitForSelector(selector, { timeout });
    return true;
  } catch {
    return false;
  }
}

export { RecorderOptions };