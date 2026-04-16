/**
 * HIS V3 System Explorer - Automated exploration of all menus and routes
 * Uses Playwright to systematically explore all available routes for a role
 */

import { chromium, Browser, Page } from 'playwright';
import * as fs from 'fs';
import * as path from 'path';

// Configuration
const config = {
  baseUrl: 'https://stag-app-hisv3.sismedika.online',
  timeout: 30000,
  credentials: {
    username: 'KGDS2025029',
    password: 'Bismillah1!',
  },
  outputDir: 'output/exploration',
};

// Routes to explore based on app.tsx route definitions
// We'll try common routes that a doctor might access
const doctorRoutes = [
  // Auth
  '/auth/login',
  
  // Doctor Queue (likely landing after login)
  '/doctor/queue',
  '/doctor/patient',
  '/doctor/medical-record',
  '/doctor/prescription',
  
  // EMR (Electronic Medical Records)
  '/emr/outpatient',
  '/emr/patient',
  
  // Queue
  '/queue/general',
  '/queue/monitoring',
  
  // Administration
  '/administration/encounter/outpatient-dashboard',
  '/administration/patient-registration',
  '/administration/queue-general',
  
  // Radiology
  '/radiology/encounter',
  '/radiology/registration',
  '/radiology/worklist',
  
  // Laboratory  
  '/laboratory/encounter',
  '/laboratory/registration',
  '/laboratory/worklist',
  
  // Billing
  '/billing/outpatient',
  
  // Emergency (if accessible)
  '/emergency',
  
  // Profile
  '/profile-doctor',
];

// Additional routes to try
const additionalRoutes = [
  '/general/welcome',
  '/emr-reservation',
  '/operation/schedule',
  '/inpatient',
  '/mcu',
  '/hemodialysis',
  '/medical-record/encounter-dashboard',
];

// Combined routes
const allRoutes = [...new Set([...doctorRoutes, ...additionalRoutes])];

/**
 * Ensure directory exists
 */
function ensureDir(dir: string): void {
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

/**
 * Capture screenshot
 */
async function captureScreenshot(page: Page, name: string): Promise<string> {
  ensureDir(config.outputDir);
  const filePath = path.join(config.outputDir, `${name}.png`);
  await page.screenshot({
    path: filePath,
    fullPage: true,
  });
  return filePath;
}

/**
 * Try to login
 */
async function tryLogin(page: Page): Promise<boolean> {
  try {
    console.log('  🔐 Attempting login...');
    await page.goto(`${config.baseUrl}/auth/login`, {
      waitUntil: 'networkidle',
      timeout: config.timeout,
    });
    
    // Wait for login form
    await page.waitForSelector('input[name="username"]', { timeout: 10000 });
    await page.fill('input[name="username"]', config.credentials.username);
    await page.fill('input[name="password"]', config.credentials.password);
    await page.click('button[type="submit"]');
    
    // Wait for redirect after login
    await page.waitForTimeout(5000);
    
    console.log('  ✅ Login successful');
    return true;
  } catch (error) {
    console.log('  ❌ Login failed:', error);
    return false;
  }
}

/**
 * Explore a single route
 */
async function exploreRoute(page: Page, route: string): Promise<void> {
  const routeName = route.replace(/\//g, '-').replace(/^-/, '');
  
  try {
    console.log(`    → Exploring: ${route}`);
    await page.goto(`${config.baseUrl}${route}`, {
      waitUntil: 'domcontentloaded',
      timeout: config.timeout,
    });
    
    // Wait for page to stabilize
    await page.waitForTimeout(2000);
    
    // Capture screenshot
    await captureScreenshot(page, `route-${routeName}`);
    
    // Try to get page title
    const title = await page.title().catch(() => 'No title');
    
    // Check for common elements
    const hasSidebar = await page.locator('.sidebar, [class*="sidebar"], nav').count() > 0;
    const hasHeader = await page.locator('header, [class*="header"]').count() > 0;
    const hasContent = await page.locator('main, [class*="content"]').count() > 0;
    
    console.log(`    ✅ Captured: ${route} (title: ${title})`);
    
    // Log any errors to console
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });
    
  } catch (error: any) {
    console.log(`    ⚠️ Failed: ${route} - ${error.message?.substring(0, 100) || error}`);
    await captureScreenshot(page, `error-${routeName}`);
  }
}

/**
 * Main exploration function
 */
async function main(): Promise<void> {
  console.log('🎯 HIS V3 System Explorer');
  console.log('=========================');
  console.log(`📍 Target: ${config.baseUrl}`);
  console.log(`👤 User: ${config.credentials.username}`);
  console.log(`📁 Output: ${config.outputDir}`);
  console.log('');

  let browser: Browser | null = null;

  try {
    // Launch browser
    console.log('🚀 Launching browser...');
    browser = await chromium.launch({
      headless: true,
      args: ['--no-sandbox', '--disable-setuid-sandbox'],
    });

    const context = await browser.newContext({
      viewport: { width: 1280, height: 720 },
    });

    const page = await context.newPage();

    // Try login first
    const loggedIn = await tryLogin(page);
    
    if (!loggedIn) {
      console.log('⚠️ Could not login, still trying to explore routes...');
    }

    // Explore all routes
    console.log(`\n📋 Exploring ${allRoutes.length} routes...\n`);
    
    for (const route of allRoutes) {
      await exploreRoute(page, route);
    }

    console.log('\n✅ Exploration complete!');
    
    // Generate summary report
    const report = generateReport(allRoutes);
    console.log('\n📊 Summary:');
    console.log(report);

  } catch (error) {
    console.error('❌ Error:', error);
  } finally {
    if (browser) {
      await browser.close();
    }
  }
}

/**
 * Generate exploration report
 */
function generateReport(routes: string[]): string {
  return `
    Total routes explored: ${routes.length}
    Output directory: ${config.outputDir}
    Check screenshots for visual documentation.
  `;
}

// Run the explorer
main().catch(console.error);