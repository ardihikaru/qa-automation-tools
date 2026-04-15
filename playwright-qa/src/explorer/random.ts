/**
 * Random Explorer - Simple random element clicking
 * Updated to use centralized config.yaml
 */

import { chromium, Browser, Page } from 'playwright';
import {
  loadConfig,
  getStaging,
  getCredentials,
  getPlaywrightQA,
  getBackendURL,
  getAllCredentials,
} from '../../lib/config';

/**
 * Main random exploration
 */
async function main(): Promise<void> {
  console.log('🎲 Playwright QA - Random Explorer');
  console.log('=====================================');

  // Load configuration
  const staging = getStaging();
  const qaConfig = getPlaywrightQA();
  const allCreds = getAllCredentials();
  const backendUrl = getBackendURL();

  console.log(`🎯 Using staging: ${staging.url}`);
  console.log(`🔧 Max steps: ${qaConfig.exploration.max_steps}`);
  console.log(`📡 Backend: ${backendUrl}`);

  let browser: Browser | null = null;

  try {
    // Launch browser
    console.log('🚀 Launching browser...');
    browser = await chromium.launch({
      headless: qaConfig.browser.headless,
      args: qaConfig.browser.args || [],
    });

    // Get roles from config
    const roleNames = Object.keys(allCreds);

    for (const roleName of roleNames) {
      const creds = getCredentials(roleName);

      if (!creds) {
        console.log(`⚠️ No credentials for role: ${roleName}`);
        continue;
      }

      console.log(`\n👤 Exploring as ${roleName}...`);

      const context = await browser.newContext({
        viewport: qaConfig.browser.viewport,
      });

      const page = await context.newPage();

      // Login
      const loggedIn = await loginAs(page, staging.url, creds);
      if (!loggedIn) {
        console.log(`  ⚠️ Could not login as ${roleName}`);
        await context.close();
        continue;
      }

      console.log(`  ✅ Logged in as ${roleName}`);

      // Explore randomly
      let steps = 0;
      let lastUrl = page.url();

      while (steps < qaConfig.exploration.max_steps) {
        steps++;

        const moved = await randomStep(page);

        // Check if page changed
        const currentUrl = page.url();
        if (currentUrl !== lastUrl) {
          console.log(`  📄 New page: ${currentUrl.split('/').pop()}`);
          lastUrl = currentUrl;
        }

        // Log to API if available
        if (moved) {
          await logAction(`random_step_${steps}`, page.url(), roleName, backendUrl);
        }

        await page.waitForTimeout(500);
      }

      console.log(`  ✅ Completed ${steps} steps`);

      await context.close();
    }

    console.log('\n✅ Random exploration complete!');

  } catch (error) {
    console.error('❌ Error:', error);
    process.exit(1);
  } finally {
    if (browser) {
      await browser.close();
    }
  }
}

/**
 * Login as a specific role
 */
async function loginAs(
  page: Page,
  baseUrl: string,
  credentials: { username: string; password: string }
): Promise<boolean> {
  try {
    await page.goto(baseUrl + '/login', { timeout: 30000 });

    const emailInput = await page.$(
      'input[name="email"], input[type="email"], #email'
    );
    const passwordInput = await page.$(
      'input[name="password"], input[type="password"], #password'
    );
    const submitButton = await page.$(
      'button[type="submit"], button:has-text("Login")'
    );

    if (emailInput && passwordInput && submitButton) {
      await emailInput.fill(credentials.username);
      await passwordInput.fill(credentials.password);
      await submitButton.click();
      await page.waitForLoadState('networkidle');
      return true;
    }

    return false;
  } catch {
    return false;
  }
}

/**
 * Get all clickable elements
 */
async function getClickableElements(page: Page): Promise<string[]> {
  const selectors = ['button', 'a', '[role="button"]'];

  const elements: string[] = [];

  for (const selector of selectors) {
    try {
      const els = await page.$$(selector);
      for (const el of els) {
        const isVisible = await el.isVisible();
        const isEnabled = await el.isEnabled();
        if (isVisible && isEnabled) {
          const text = await el.innerText().catch(() => '');
          if (text.trim()) {
            elements.push(text.trim());
          }
        }
      }
    } catch {
      // Ignore
    }
  }

  return [...new Set(elements)];
}

/**
 * Random exploration step
 */
async function randomStep(page: Page): Promise<boolean> {
  const elements = await getClickableElements(page);

  if (elements.length === 0) {
    console.log('  ⚠️ No clickable elements found');
    return false;
  }

  const randomIndex = Math.floor(Math.random() * elements.length);
  const selectedText = elements[randomIndex];

  console.log(`  🎯 Clicking: ${selectedText}`);

  try {
    await page.click(`text=${selectedText}`, { timeout: 2000 });
    await page.waitForLoadState('domcontentloaded');
    return true;
  } catch {
    console.log(`  ❌ Failed to click: ${selectedText}`);
    return false;
  }
}

/**
 * Log action to backend API
 */
async function logAction(
  action: string,
  pageUrl: string,
  role: string,
  backendUrl: string
): Promise<void> {
  try {
    await fetch(`${backendUrl}/api/flows`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        role,
        action,
        page: pageUrl,
        timestamp: new Date().toISOString(),
      }),
    });
  } catch {
    // Silently fail - API might not be available
  }
}

// Run if executed directly
main().catch(console.error);

export { main, loginAs, randomStep, getClickableElements };