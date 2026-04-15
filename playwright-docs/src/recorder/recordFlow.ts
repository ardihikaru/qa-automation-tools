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
    minimax_api_key: string;
  };
}

// Import types from flowLoader
import { FlowDefinition, FlowStep } from '../flows/flowLoader';

interface RecorderOptions {
  outputDir?: string;
  videoDir?: string;
  screenshotDir?: string;
}

const defaultOptions: RecorderOptions = {
  outputDir: 'output',
  videoDir: 'videos',
  screenshotDir: 'screenshots',
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
  const screenshotDir = path.join(outputDir, options.screenshotDir || 'screenshots');

  // Ensure directories exist
  if (!fs.existsSync(screenshotDir)) {
    fs.mkdirSync(screenshotDir, { recursive: true });
  }

  // Get staging URL
  const baseUrl = config.staging.url;

  // Execute flow steps
  for (let i = 0; i < flow.steps.length; i++) {
    const step = flow.steps[i];

    try {
      switch (step.action) {
        case 'navigate':
          await page.goto(baseUrl + step.selector, {
            waitUntil: 'domcontentloaded',
            timeout: config.staging.timeout,
          });
          break;

        case 'click':
          await page.click(step.selector, { timeout: 5000 });
          break;

        case 'type':
          const value = step.value
            ? replacePlaceholders(step.value, credentials)
            : '';
          await page.fill(step.selector, value);
          break;

        case 'select':
          await page.selectOption(step.selector, step.value);
          break;

        case 'wait':
          if (step.wait) {
            await page.waitForTimeout(step.wait);
          }
          break;
      }

      // Capture screenshot after each step
      const screenshotName = `${flow.id}-step-${i + 1}`;
      await captureScreenshot(page, screenshotName, screenshotDir);
    } catch (error) {
      console.error(`  ❌ Step ${i + 1} failed:`, error);
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

export { RecorderOptions, FlowConfig, Credential, replacePlaceholders };