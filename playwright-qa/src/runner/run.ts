/**
 * Main QA Runner - Ties all explorers, skills, and logger together
 */

import { chromium, Browser, Page } from 'playwright';
import { loadConfig, Config } from '../explorer/random';
import { loginAs } from '../explorer/random';
import { runSkills, getBuiltinSkills, SkillResult } from '../skills/builtin/skills';
import { APILogger, createLogger, LogEntry } from '../logger/api';

// Exploration mode
export type ExplorationMode = 'random' | 'guided' | 'planning';

// Runner configuration
interface RunnerConfig extends Config {
  mode: ExplorationMode;
  runSkills: boolean;
  maxIterations: number;
}

/**
 * Load runner configuration
 */
function loadRunnerConfig(): RunnerConfig {
  return {
    ...loadConfig(),
    mode: (process.env.MODE as ExplorationMode) || 'random',
    runSkills: process.env.RUN_SKILLS !== 'false',
    maxIterations: parseInt(process.env.MAX_ITERATIONS || '10'),
  };
}

/**
 * Main QA runner
 */
class QARunner {
  private config: RunnerConfig;
  private logger: APILogger;
  private browser: Browser | null = null;
  
  constructor(config?: Partial<RunnerConfig>) {
    this.config = { ...loadRunnerConfig(), ...config };
    this.logger = createLogger();
  }
  
  /**
   * Start the runner
   */
  async start(): Promise<void> {
    console.log('🎯 Playwright QA Runner');
    console.log('mode:', this.config.mode);
    console.log('runSkills:', this.config.runSkills);
    console.log('================================');
    
    try {
      // Launch browser
      this.browser = await chromium.launch({
        headless: true,
        args: ['--no-sandbox'],
      });
      
      // Run for each role
      const roles = ['admin', 'doctor', 'cashier'] as const;
      
      for (const role of roles) {
        await this.runForRole(role);
      }
      
      console.log('\n✅ QA run complete!');
      
    } catch (error) {
      console.error('❌ Error:', error);
    } finally {
      await this.cleanup();
    }
  }
  
  /**
   * Run for a specific role
   */
  private async runForRole(role: 'admin' | 'doctor' | 'cashier'): Promise<void> {
    console.log(`\n👤 Running as ${role}...`);
    
    const context = await this.browser!.newContext({
      viewport: { width: 1280, height: 720 },
    });
    
    const page = await context.newPage();
    
    // Login
    const loggedIn = await loginAs(page, role, this.config);
    if (!loggedIn) {
      console.log(`  ⚠️ Could not login as ${role}`);
      await context.close();
      return;
    }
    
    console.log(`  ✅ Logged in as ${role}`);
    
    // Run exploration iterations
    let iterations = 0;
    
    while (iterations < this.config.maxIterations) {
      iterations++;
      
      console.log(`\n  🔄 Iteration ${iterations}/${this.config.maxIterations}`);
      
      // Run exploration based on mode
      await this.runExploration(page, role, iterations);
      
      // Run skills if enabled
      if (this.config.runSkills) {
        await this.runSkills(page, role);
      }
      
      // Small delay
      await page.waitForTimeout(500);
    }
    
    await context.close();
  }
  
  /**
   * Run exploration step
   */
  private async runExploration(page: Page, role: string, iteration: number): Promise<void> {
    const url = page.url();
    
    switch (this.config.mode) {
      case 'random':
        // Import and run random explorer
        // const { randomStep } = await import('../explorer/random');
        // await randomStep(page, this.config);
        await this.randomStep(page);
        break;
        
      case 'guided':
        // Import and run guided explorer
        // const { guidedStep } = await import('../explorer/guided');
        // await guidedStep(page, this.config);
        await this.guidedStep(page);
        break;
        
      case 'planning':
        // Import and run planning explorer
        // const { executePlan } = await import('../explorer/planner');
        // await executePlan(page, plan);
        await this.planningStep(page);
        break;
    }
    
    // Log exploration
    this.logger.logExploration(
      `iteration_${iteration}`,
      url,
      this.config.mode,
      true,
      `Run ${iteration} for ${role}`
    );
  }
  
  /**
   * Simple random step (inline implementation)
   */
  private async randomStep(page: Page): Promise<void> {
    const elements = await page.$$('button, a, [role="button"]');
    
    if (elements.length === 0) {
      console.log('  ⚠️ No clickable elements');
      return;
    }
    
    const randomIndex = Math.floor(Math.random() * elements.length);
    const selected = elements[randomIndex];
    const text = await selected.innerText().catch(() => 'unknown');
    
    console.log(`  🎲 Random: ${text.trim() || 'element'}`);
    
    try {
      await selected.click();
      await page.waitForLoadState('domcontentloaded');
    } catch {
      // Ignore click errors
    }
  }
  
  /**
   * Simple guided step (inline implementation)
   */
  private async guidedStep(page: Page): Promise<void> {
    // In production, this would use learning cache
    // For now, fallback to random
    await this.randomStep(page);
  }
  
  /**
   * Simple planning step (inline implementation)
   */
  private async planningStep(page: Page): Promise<void> {
    // In production, this would generate a plan
    // For now, fallback to guided
    await this.guidedStep(page);
  }
  
  /**
   * Run QA skills on page
   */
  private async runSkills(page: Page, role: string): Promise<void> {
    console.log('  🔍 Running QA skills...');
    
    try {
      const results = await runSkills(page);
      
      for (const result of results) {
        if (!result.passed) {
          console.log(`  ❌ Skill failed: ${result.skill} - ${result.message}`);
          
          this.logger.logIssue(
            result.skill,
            page.url(),
            result.skill,
            result.message,
            'frontend',
            result.severity as 'low' | 'medium' | 'high'
          );
        } else {
          console.log(`  ✅ Skill passed: ${result.skill}`);
        }
      }
    } catch (error) {
      console.log(`  ⚠️ Skill error: ${error}`);
    }
  }
  
  /**
   * Cleanup resources
   */
  private async cleanup(): Promise<void> {
    if (this.browser) {
      await this.browser.close();
    }
    await this.logger.close();
  }
}

/**
 * Main entry point
 */
async function main(): Promise<void> {
  const runner = new QARunner();
  await runner.start();
}

// Run if executed directly
main().catch(console.error);

export { QARunner, main, ExplorationMode, RunnerConfig };