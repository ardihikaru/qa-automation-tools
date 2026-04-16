/**
 * Guided Explorer - Learning-based action selection
 * Uses learning cache to pick best actions
 */

import { chromium, Browser, Page } from 'playwright';
import * as fs from 'fs';
import * as yaml from 'yaml';
import * as path from 'path';

/**
 * Guided Explorer - Learning-based action selection
 * Uses learning cache to pick best actions
 */

// Configuration interface
interface Config {
  stagingUrl: string;
  credentials: {
    admin: { username: string; password: string };
    doctor: { username: string; password: string };
    cashier: { username: string; password: string };
  };
  apiUrl: string;
  maxSteps: number;
  timeout: number;
}

interface LearningRecord {
  id: number;
  intent: string;
  action: string;
  page: string;
  success_score: number;
  usage_count: number;
}

// Scoring weights
const SCORING_WEIGHTS = {
  SIMILARITY: 0.5,
  SUCCESS_SCORE: 0.3,
  USAGE_COUNT: 0.1,
  EXPLORATION: 0.1,
};

/**
 * Load configuration from config.yaml
 */
function loadConfig(): Config {
  // Default values from environment variables
  const defaults = {
    stagingUrl: process.env.STAGING_URL || 'https://stag-app-hisv3.sismedika.online',
    credentials: {
      admin: {
        username: process.env.ADMIN_USER || 'KG2024001',
        password: process.env.ADMIN_PASS || 'Bismillah1!',
      },
      doctor: {
        username: process.env.DOCTOR_USER || 'KG2024029',
        password: process.env.DOCTOR_PASS || 'Bismillah1!',
      },
      cashier: {
        username: process.env.CASHIER_USER || 'KG2024055',
        password: process.env.CASHIER_PASS || 'Bismillah1!',
      },
    },
    apiUrl: process.env.API_URL || 'http://localhost:8080',
    maxSteps: parseInt(process.env.MAX_STEPS || '50'),
    timeout: parseInt(process.env.TIMEOUT || '30000'),
  };

  // Try to load from config.yaml
  try {
    const configPath = path.join(process.cwd(), '..', 'config.yaml');
    if (fs.existsSync(configPath)) {
      const configFile = fs.readFileSync(configPath, 'utf8');
      const configData = yaml.parse(configFile);
      
      if (configData?.staging?.url) {
        defaults.stagingUrl = configData.staging.url;
      }
      
      if (configData?.credentials?.admin) {
        defaults.credentials.admin = {
          username: configData.credentials.admin.username,
          password: configData.credentials.admin.password,
        };
      }
      
      if (configData?.credentials?.doctor_gp) {
        defaults.credentials.doctor = {
          username: configData.credentials.doctor_gp.username,
          password: configData.credentials.doctor_gp.password,
        };
      }
      
      if (configData?.credentials?.cashier) {
        defaults.credentials.cashier = {
          username: configData.credentials.cashier.username,
          password: configData.credentials.cashier.password,
        };
      }
    }
  } catch (e) {
    console.warn('Could not load config.yaml, using defaults:', e);
  }

  return defaults;
}

/**
 * Load learning cache from backend
 */
async function loadLearningCache(): Promise<LearningRecord[]> {
  const config = loadConfig();
  
  try {
    const response = await fetch(`${config.apiUrl}/api/learning`);
    const data = await response.json();
    return data.learning_records || [];
  } catch {
    return [];
  }
}

/**
 * Query learning cache for similar actions
 */
async function queryLearningCache(intent: string): Promise<LearningRecord[]> {
  const config = loadConfig();
  
  try {
    const response = await fetch(`${config.apiUrl}/api/search?q=${encodeURIComponent(intent)}`);
    const data = await response.json();
    return data || [];
  } catch {
    return [];
  }
}

/**
 * Record action result to learning cache
 */
async function recordToLearningCache(
  intent: string,
  action: string,
  page: string,
  success: boolean
): Promise<void> {
  const config = loadConfig();
  
  try {
    await fetch(`${config.apiUrl}/api/learning`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        intent,
        action,
        page,
        success,
      }),
    });
  } catch {
    // Silently fail
  }
}

/**
 * Score an action based on learning cache
 */
function scoreAction(record: LearningRecord, explorationBonus: boolean = false): number {
  const similarity = record.success_score; // Using success_score as proxy
  const successScore = record.success_score;
  const usageLog = Math.log(record.usage_count + 1);
  const exploration = explorationBonus ? SCORING_WEIGHTS.EXPLORATION : 0;
  
  return (
    similarity * SCORING_WEIGHTS.SIMILARITY +
    successScore * SCORING_WEIGHTS.SUCCESS_SCORE +
    usageLog * SCORING_WEIGHTS.USAGE_COUNT +
    exploration
  );
}

/**
 * Get all clickable elements with their text
 */
async function getClickableElements(page: Page): Promise<Array<{text: string; selector: string}>> {
  const selectors = [
    { selector: 'button', text: 'innerText' },
    { selector: 'a', text: 'innerText' },
    { selector: '[role="button"]', text: 'innerText' },
  ];
  
  const elements: Array<{text: string; selector: string}> = [];
  
  for (const sel of selectors) {
    try {
      const els = await page.$$(sel.selector);
      for (const el of els) {
        const isVisible = await el.isVisible().catch(() => false);
        const isEnabled = await el.isEnabled().catch(() => false);
        
        if (isVisible && isEnabled) {
          const text = await el.innerText().catch(() => '');
          const trimmed = text.trim();
          if (trimmed) {
            elements.push({
              text: trimmed,
              selector: sel.selector,
            });
          }
        }
      }
    } catch {
      // Ignore errors
    }
  }
  
  // Deduplicate
  const unique = new Map<string, {text: string; selector: string}>();
  for (const el of elements) {
    unique.set(el.text, el);
  }
  
  return Array.from(unique.values());
}

/**
 * Login as a specific role
 * Uses the correct selectors for HIS frontend login form
 */
async function loginAs(page: Page, role: 'admin' | 'doctor' | 'cashier', config: Config): Promise<boolean> {
  const creds = config.credentials[role];
  
  try {
    // Navigate to login page
    await page.goto(config.stagingUrl + '/auth/login', { timeout: config.timeout, waitUntil: 'networkidle' });
    
    // Wait for the login form to be visible
    await page.waitForSelector('input[name="username"], input[name="password"]', { timeout: 5000 });
    
    // Wait a bit for JavaScript to initialize
    await page.waitForTimeout(500);
    
    // Find input fields - using the correct selectors for HIS frontend
    // Username field uses name="username", NOT name="email"
    const usernameInput = await page.$('input[name="username"]');
    const passwordInput = await page.$('input[name="password"]');
    const submitButton = await page.$('button[type="submit"], button:has-text("Login")');
    
    if (usernameInput && passwordInput && submitButton) {
      // Clear any existing values first
      await usernameInput.fill('');
      await passwordInput.fill('');
      
      // Fill in credentials with small delay between each
      await usernameInput.fill(creds.username);
      await page.waitForTimeout(100); // Small delay
      await passwordInput.fill(creds.password);
      await page.waitForTimeout(100); // Small delay
      
      // Click submit
      await submitButton.click();
      
      // Wait for navigation or response
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000); // Additional wait for auth to process
      
      return true;
    }
    
    console.log('  ⚠️ Login form elements not found');
    return false;
  } catch (error) {
    console.log('  ❌ Login error:', error);
    return false;
  }
}

/**
 * Decide which action to take - exploration vs exploitation
 * 20% exploration, 80% exploitation
 */
function shouldExplore(): boolean {
  return Math.random() < 0.2;
}

/**
 * Execute a guided step
 */
async function guidedStep(page: Page, config: Config): Promise<{action: string; success: boolean}> {
  // Get all clickable elements
  const elements = await getClickableElements(page);
  
  if (elements.length === 0) {
    return { action: 'none', success: false };
  }
  
  const currentPage = page.url();
  const intent = `page_${currentPage.split('/').pop()}`;
  
  // Decide exploration vs exploitation
  if (shouldExplore()) {
    // Exploration: pick random element
    const randomIndex = Math.floor(Math.random() * elements.length);
    const selected = elements[randomIndex];
    
    console.log(`  🔍 Exploring: ${selected.text}`);
    
    try {
      await page.click(`text=${selected.text}`, { timeout: 2000 });
      await page.waitForLoadState('domcontentloaded');
      
      // Record to learning cache
      await recordToLearningCache(intent, selected.text, currentPage, true);
      
      return { action: selected.text, success: true };
    } catch {
      await recordToLearningCache(intent, selected.text, currentPage, false);
      return { action: selected.text, success: false };
    }
  } else {
    // Exploitation: use learning cache
    const similarActions = await queryLearningCache(intent);
    
    if (similarActions.length === 0) {
      // No learning - fallback to random
      const randomIndex = Math.floor(Math.random() * elements.length);
      const selected = elements[randomIndex];
      
      console.log(`  🎲 No learning, random: ${selected.text}`);
      
      try {
        await page.click(`text=${selected.text}`, { timeout: 2000 });
        await page.waitForLoadState('domcontentloaded');
        return { action: selected.text, success: true };
      } catch {
        return { action: selected.text, success: false };
      }
    }
    
    // Score each element
    const scoredElements = elements.map(el => {
      const matchingAction = similarActions.find(
        a => a.action.toLowerCase() === el.text.toLowerCase()
      );
      
      const record: LearningRecord = matchingAction || {
        id: 0,
        intent: '',
        action: el.text,
        page: currentPage,
        success_score: 0.5,
        usage_count: 1,
      };
      
      return {
        element: el,
        score: scoreAction(record, false),
      };
    });
    
    // Sort by score descending
    scoredElements.sort((a, b) => b.score - a.score);
    
    const best = scoredElements[0];
    console.log(`  🎯 Best: ${best.element.text} (score: ${best.score.toFixed(2)})`);
    
    try {
      await page.click(`text=${best.element.text}`, { timeout: 2000 });
      await page.waitForLoadState('domcontentloaded');
      
      // Record success
      await recordToLearningCache(intent, best.element.text, currentPage, true);
      
      return { action: best.element.text, success: true };
    } catch {
      await recordToLearningCache(intent, best.element.text, currentPage, false);
      return { action: best.element.text, success: false };
    }
  }
}

/**
 * Main guided exploration
 */
async function main(): Promise<void> {
  console.log('🧠 Playwright QA - Guided Explorer');
  console.log('====================================');
  
  const config = loadConfig();
  let browser: Browser | null = null;
  
  try {
    console.log('🚀 Launching browser...');
    browser = await chromium.launch({
      headless: true,
      args: ['--no-sandbox'],
    });
    
    const roles: ('admin' | 'doctor' | 'cashier')[] = ['admin', 'doctor', 'cashier'];
    
    for (const role of roles) {
      console.log(`\n👤 Guided exploration as ${role}...`);
      
      const context = await browser.newContext({
        viewport: { width: 1280, height: 720 },
      });
      
      const page = await context.newPage();
      
      const loggedIn = await loginAs(page, role, config);
      if (!loggedIn) {
        console.log(`  ⚠️ Could not login as ${role}`);
        await context.close();
        continue;
      }
      
      console.log(`  ✅ Logged in as ${role}`);
      
      let steps = 0;
      let lastUrl = page.url();
      
      while (steps < config.maxSteps) {
        steps++;
        
        const result = await guidedStep(page, config);
        
        const currentUrl = page.url();
        if (currentUrl !== lastUrl) {
          console.log(`  📄 Page changed: ${currentUrl.split('/').pop()}`);
          lastUrl = currentUrl;
        }
        
        await page.waitForTimeout(500);
      }
      
      console.log(`  ✅ Completed ${steps} guided steps`);
      
      await context.close();
    }
    
    console.log('\n✅ Guided exploration complete!');
    
  } catch (error) {
    console.error('❌ Error:', error);
    process.exit(1);
  } finally {
    if (browser) {
      await browser.close();
    }
  }
}

// Run if executed directly
main().catch(console.error);

export { main, guidedStep, loadLearningCache, scoreAction, shouldExplore, SCORING_WEIGHTS };