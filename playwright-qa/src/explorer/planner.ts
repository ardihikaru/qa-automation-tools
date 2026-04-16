/**
 * Planning Explorer - Multi-step planning agent
 * Plans sequences of actions to achieve goals
 * Updated to use centralized config.yaml
 */

import { chromium, Browser, Page } from 'playwright';
import * as fs from 'fs';
import * as yaml from 'yaml';
import * as path from 'path';

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
  planDepth: number;
  timeout: number;
}

// Planning types
interface Action {
  text: string;
  selector: string;
  intent: string;
}

interface PlanStep {
  action: string;
  score: number;
  depth: number;
}

interface Plan {
  steps: PlanStep[];
  totalScore: number;
  goal: string;
}

// Goal definitions
interface Goal {
  name: string;
  signals: string[];
  priority: number;
}

// Known goals
const KNOWN_GOALS: Goal[] = [
  {
    name: 'create_user',
    signals: ['User created', 'Success', 'created'],
    priority: 1,
  },
  {
    name: 'create_invoice',
    signals: ['Invoice created', 'Success', 'created'],
    priority: 1,
  },
  {
    name: 'login',
    signals: ['Dashboard', 'Welcome', 'logged'],
    priority: 2,
  },
  {
    name: 'save_form',
    signals: ['Success', 'Saved', 'created'],
    priority: 1,
  },
  {
    name: 'search',
    signals: ['Results', 'found', 'Search'],
    priority: 2,
  },
];

/**
 * Load configuration from config.yaml
 */
function loadConfig(): Config {
  // Default values from environment variables
  const defaults = {
    stagingUrl: process.env.STAGING_URL || 'https://stag-app.mfardiansyah.id',
    credentials: {
      admin: {
        username: process.env.ADMIN_USER || 'USER2024001',
        password: process.env.ADMIN_PASS || '<staging-password>',
      },
      doctor: {
        username: process.env.DOCTOR_USER || 'USER2024029',
        password: process.env.DOCTOR_PASS || '<staging-password>',
      },
      cashier: {
        username: process.env.CASHIER_USER || 'USER2024055',
        password: process.env.CASHIER_PASS || '<staging-password>',
      },
    },
    apiUrl: process.env.API_URL || 'http://localhost:8080',
    maxSteps: parseInt(process.env.MAX_STEPS || '30'),
    planDepth: parseInt(process.env.PLAN_DEPTH || '3'),
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
 * Get all clickable elements
 */
async function getClickableElements(page: Page): Promise<Action[]> {
  const selectors = ['button', 'a', '[role="button"]'];
  
  const elements: Action[] = [];
  
  for (const selector of selectors) {
    try {
      const els = await page.$$(selector);
      for (const el of els) {
        const isVisible = await el.isVisible().catch(() => false);
        const isEnabled = await el.isEnabled().catch(() => false);
        
        if (isVisible && isEnabled) {
          const text = await el.innerText().catch(() => '');
          const trimmed = text.trim();
          if (trimmed && trimmed.length < 50) {
            elements.push({
              text: trimmed,
              selector,
              intent: trimmed.toLowerCase(),
            });
          }
        }
      }
    } catch {
      // Ignore
    }
  }
  
  return elements;
}

/**
 * Generate possible actions from current page
 */
async function generatePossibleActions(page: Page): Promise<Action[]> {
  return getClickableElements(page);
}

/**
 * Check if goal is achieved
 */
async function checkGoalAchievement(page: Page, goal: string): Promise<{achieved: boolean; confidence: number}> {
  const url = page.url();
  const content = await page.content();
  
  // Check URL
  if (url.includes(goal.replace(' ', ''))) {
    return { achieved: true, confidence: 0.8 };
  }
  
  // Check page content for goal signals
  const goalDef = KNOWN_GOALS.find(g => g.name === goal);
  if (!goalDef) {
    return { achieved: false, confidence: 0 };
  }
  
  let signalMatches = 0;
  for (const signal of goalDef.signals) {
    if (content.toLowerCase().includes(signal.toLowerCase())) {
      signalMatches++;
    }
  }
  
  const confidence = signalMatches / goalDef.signals.length;
  
  return {
    achieved: signalMatches > 0,
    confidence,
  };
}

/**
 * Simulate a plan - try action and check resulting state
 */
async function simulateAction(
  page: Page,
  action: Action,
  depth: number
): Promise<{newPage: Page | null; score: number}> {
  try {
    await page.click(`text=${action.text}`, { timeout: 1000 });
    await page.waitForTimeout(300);
    
    // Check if we're still on same page
    const url = page.url();
    
    return {
      newPage: page,
      score: url.includes('error') ? 0 : 0.5, // Simple scoring
    };
  } catch {
    return {
      newPage: null,
      score: 0,
    };
  }
}

/**
 * Generate action tree with scores
 */
async function generateActionTree(
  page: Page,
  goal: string,
  depth: number
): Promise<Plan> {
  const actions = await generatePossibleActions(page);
  
  if (actions.length === 0) {
    return {
      steps: [],
      totalScore: 0,
      goal,
    };
  }
  
  // Score each action
  const scoredActions: PlanStep[] = [];
  
  for (const action of actions) {
    // Simple heuristic scoring based on action text matching goal
    let score = 0.5;
    
    // Boost if action text relates to goal
    const goalKeywords = goal.split('_');
    for (const keyword of goalKeywords) {
      if (action.intent.includes(keyword.toLowerCase())) {
        score += 0.2;
      }
    }
    
    // Cap score
    score = Math.min(score, 1.0);
    
    scoredActions.push({
      action: action.text,
      score,
      depth,
    });
  }
  
  // Sort by score
  scoredActions.sort((a, b) => b.score - a.score);
  
  return {
    steps: scoredActions.slice(0, depth),
    totalScore: scoredActions[0]?.score || 0,
    goal,
  };
}

/**
 * Execute the first step of a plan
 */
async function executePlan(
  page: Page,
  plan: Plan
): Promise<{action: string; success: boolean}> {
  if (plan.steps.length === 0) {
    return { action: 'none', success: false };
  }
  
  const bestAction = plan.steps[0].action;
  
  console.log(`  📋 Executing: ${bestAction}`);
  
  try {
    await page.click(`text=${bestAction}`, { timeout: 2000 });
    await page.waitForLoadState('domcontentloaded');
    
    return { action: bestAction, success: true };
  } catch (error) {
    console.log(`  ❌ Failed: ${error}`);
    return { action: bestAction, success: false };
  }
}

/**
 * Decide when to use planning vs guided
 */
function usePlanningMode(page: Page): boolean {
  // Use planning for complex pages (forms, multiple actions)
  // Use guided for simple navigation pages
  const url = page.url();
  
  const complexPatterns = ['/new', '/create', '/edit', '/form', '/add'];
  for (const pattern of complexPatterns) {
    if (url.includes(pattern)) {
      return true;
    }
  }
  
  return Math.random() < 0.3; // 30% random planning
}

/**
 * Login as role
 * Uses the correct selectors for SUT frontend login form
 */
async function loginAs(page: Page, role: 'admin' | 'doctor' | 'cashier', config: Config): Promise<boolean> {
  const creds = config.credentials[role];
  
  try {
    // Navigate to login page - using correct path for SUT frontend
    await page.goto(config.stagingUrl + '/auth/login', { timeout: config.timeout, waitUntil: 'networkidle' });
    
    // Wait for the login form to be visible
    await page.waitForSelector('input[name="username"], input[name="password"]', { timeout: 5000 });
    
    // Wait a bit for JavaScript to initialize
    await page.waitForTimeout(500);
    
    // Find input fields - using the correct selectors for SUT frontend
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
 * Determine current goal from page
 */
async function determineGoal(page: Page): Promise<string> {
  const url = page.url();
  const content = await page.content();
  
  // Check URL patterns
  if (url.includes('/user')) return 'create_user';
  if (url.includes('/invoice')) return 'create_invoice';
  if (url.includes('/patient')) return 'view_patient';
  if (url.includes('/settings')) return 'save_settings';
  if (url.includes('/billing')) return 'view_billing';
  
  // Default goal
  return 'explore';
}

/**
 * Main planning exploration
 */
async function main(): Promise<void> {
  console.log('📋 Playwright QA - Planning Explorer');
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
      console.log(`\n👤 Planning exploration as ${role}...`);
      
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
      
      while (steps < config.maxSteps) {
        steps++;
        
        // Determine current goal
        const goal = await determineGoal(page);
        console.log(`\n  🎯 Goal: ${goal}`);
        
        // Decide mode: planning or guided
        const usePlanning = usePlanningMode(page);
        
        if (usePlanning) {
          console.log(`  📋 Using planning mode (depth ${config.planDepth})`);
          
          // Generate plan
          const plan = await generateActionTree(page, goal, config.planDepth);
          
          if (plan.steps.length > 0) {
            // Execute first step
            const result = await executePlan(page, plan);
            console.log(`  ${result.success ? '✅' : '❌'} ${result.action}`);
          } else {
            console.log('  ⚠️ No actions available');
          }
        } else {
          console.log('  🎯 Using guided mode');
          // Fall back to guided (would import guided.ts in production)
          const elements = await getClickableElements(page);
          if (elements.length > 0) {
            const random = elements[Math.floor(Math.random() * elements.length)];
            console.log(`  🎲 Random: ${random.text}`);
            
            try {
              await page.click(`text=${random.text}`, { timeout: 2000 });
              await page.waitForLoadState('domcontentloaded');
            } catch {
              // Ignore
            }
          }
        }
        
        // Check goal achievement
        const goalCheck = await checkGoalAchievement(page, goal);
        if (goalCheck.confidence > 0.5) {
          console.log(`  🎉 Goal potentially achieved! (confidence: ${goalCheck.confidence.toFixed(2)})`);
        }
        
        await page.waitForTimeout(500);
      }
      
      console.log(`  ✅ Completed ${steps} planning steps`);
      
      await context.close();
    }
    
    console.log('\n✅ Planning exploration complete!');
    
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

export { 
  main, 
  loadConfig, 
  generateActionTree, 
  executePlan, 
  checkGoalAchievement,
  determineGoal,
  usePlanningMode,
  KNOWN_GOALS,
  Goal,
  Plan,
  PlanStep,
  Action
};