/**
 * Logger - Send logs and issues to backend API
 */

import { Page } from 'playwright';

// Log entry types
export type LogLevel = 'debug' | 'info' | 'warn' | 'error';
export type LogTarget = 'flow' | 'skill' | 'issue' | 'goal' | 'exploration';

// Core log entry
export interface LogEntry {
  id?: number;
  timestamp: string;
  level: LogLevel;
  target: LogTarget;
  action: string;
  page: string;
  details?: string;
  metadata?: Record<string, any>;
  success?: boolean;
  duration?: number;
}

// Flow log
export interface FlowLog extends LogEntry {
  target: 'flow';
  role: string;
  flowName: string;
  steps: number;
  status: 'completed' | 'failed' | 'skipped';
}

// Skill log
export interface SkillLog extends LogEntry {
  target: 'skill';
  skillName: string;
  skillResult: 'pass' | 'fail';
  errorMessage?: string;
}

// Issue log
export interface IssueLog extends LogEntry {
  target: 'issue';
  errorType: string;
  errorText: string;
  category?: 'frontend' | 'backend' | 'unknown';
  severity?: 'low' | 'medium' | 'high';
}

// Goal log
export interface GoalLog extends LogEntry {
  target: 'goal';
  goalName: string;
  confidence: number;
  achieved: boolean;
}

// Logger configuration
interface LoggerConfig {
  apiUrl: string;
  batchSize: number;
  flushInterval: number;
  retryCount: number;
  retryDelay: number;
}

/**
 * Create logger configuration
 */
function createConfig(): LoggerConfig {
  return {
    apiUrl: process.env.API_URL || 'http://localhost:8080',
    batchSize: parseInt(process.env.LOG_BATCH_SIZE || '10'),
    flushInterval: parseInt(process.env.LOG_FLUSH_INTERVAL || '5000'),
    retryCount: parseInt(process.env.LOG_RETRY_COUNT || '3'),
    retryDelay: parseInt(process.env.LOG_RETRY_DELAY || '1000'),
  };
}

/**
 * APILogger - handles sending logs to backend
 */
export class APILogger {
  private config: LoggerConfig;
  private queue: LogEntry[] = [];
  private flushing: boolean = false;
  private flushTimer?: NodeJS.Timeout;
  
  constructor(config?: Partial<LoggerConfig>) {
    this.config = { ...createConfig(), ...config };
    this.startFlushTimer();
  }
  
  /**
   * Start automatic flush timer
   */
  private startFlushTimer(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
    }
    
    this.flushTimer = setInterval(() => {
      this.flush();
    }, this.config.flushInterval);
  }
  
  /**
   * Add log entry to queue
   */
  log(entry: LogEntry): void {
    // Add timestamp if not present
    if (!entry.timestamp) {
      entry.timestamp = new Date().toISOString();
    }
    
    this.queue.push(entry);
    
    // Flush if batch size reached
    if (this.queue.length >= this.config.batchSize) {
      this.flush();
    }
  }
  
  /**
   * Log flow execution
   */
  logFlow(
    action: string,
    page: string,
    role: string,
    flowName: string,
    steps: number,
    status: 'completed' | 'failed' | 'skipped',
    duration?: number
  ): void {
    this.log({
      timestamp: new Date().toISOString(),
      level: status === 'failed' ? 'error' : 'info',
      target: 'flow',
      action,
      page,
      success: status === 'completed',
      duration,
      metadata: { role, flowName, steps, status },
    } as FlowLog);
  }
  
  /**
   * Log skill execution
   */
  logSkill(
    action: string,
    page: string,
    skillName: string,
    passed: boolean,
    errorMessage?: string
  ): void {
    this.log({
      timestamp: new Date().toISOString(),
      level: passed ? 'info' : 'warn',
      target: 'skill',
      action,
      page,
      success: passed,
      metadata: { skillName, skillResult: passed ? 'pass' : 'fail', errorMessage },
    } as SkillLog);
  }
  
  /**
   * Log issue detection
   */
  logIssue(
    action: string,
    page: string,
    errorType: string,
    errorText: string,
    category?: 'frontend' | 'backend' | 'unknown',
    severity: 'low' | 'medium' | 'high' = 'medium'
  ): void {
    this.log({
      timestamp: new Date().toISOString(),
      level: severity === 'high' ? 'error' : 'warn',
      target: 'issue',
      action,
      page,
      success: false,
      metadata: { errorType, errorText, category, severity },
    } as IssueLog);
  }
  
  /**
   * Log goal detection
   */
  logGoal(
    action: string,
    page: string,
    goalName: string,
    confidence: number,
    achieved: boolean
  ): void {
    this.log({
      timestamp: new Date().toISOString(),
      level: achieved ? 'info' : 'debug',
      target: 'goal',
      action,
      page,
      success: achieved,
      metadata: { goalName, confidence, achieved },
    } as GoalLog);
  }
  
  /**
   * Log exploration step
   */
  logExploration(
    action: string,
    page: string,
    mode: 'random' | 'guided' | 'planning',
    success: boolean,
    details?: string
  ): void {
    this.log({
      timestamp: new Date().toISOString(),
      level: success ? 'info' : 'debug',
      target: 'exploration',
      action,
      page,
      success,
      details,
      metadata: { mode },
    });
  }
  
  /**
   * Flush queue to API
   */
  async flush(): Promise<void> {
    if (this.flushing || this.queue.length === 0) {
      return;
    }
    
    this.flushing = true;
    
    const entries = [...this.queue];
    this.queue = [];
    
    // Determine endpoint based on first entry target
    const endpoint = this.getEndpoint(entries[0].target);
    
    let success = false;
    let attempts = 0;
    
    while (!success && attempts < this.config.retryCount) {
      attempts++;
      
      try {
        const response = await fetch(`${this.config.apiUrl}${endpoint}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(entries),
        });
        
        if (response.ok) {
          success = true;
        } else {
          await this.delay(this.config.retryDelay);
        }
      } catch {
        await this.delay(this.config.retryDelay);
      }
    }
    
    // Re-add to queue if failed
    if (!success) {
      this.queue.push(...entries);
    }
    
    this.flushing = false;
  }
  
  /**
   * Get correct endpoint for target
   */
  private getEndpoint(target: LogTarget): string {
    switch (target) {
      case 'flow':
        return '/api/flows';
      case 'skill':
        return '/api/skills';
      case 'issue':
        return '/api/issues';
      case 'goal':
        return '/api/analytics';
      case 'exploration':
        return '/api/learning';
      default:
        return '/api/flows';
    }
  }
  
  /**
   * Delay helper
   */
  private delay(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
  
  /**
   * Close logger (flush and stop timer)
   */
  async close(): Promise<void> {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
    }
    await this.flush();
  }
}

/**
 * Create default logger instance
 */
export function createLogger(): APILogger {
  return new APILogger();
}

/**
 * Log from Playwright page context
 */
export async function logPageState(
  logger: APILogger,
  page: Page,
  action: string,
  mode: 'random' | 'guided' | 'planning'
): Promise<void> {
  const url = page.url();
  
  try {
    // Get page title
    const title = await page.title().catch(() => '');
    
    // Get visible text for context
    const body = await page.$('body');
    const text = body
      ? await body.innerText().catch(() => '')
      : '';
    
    // Determine if action succeeded (simplified)
    const success = !url.includes('error') && !text.includes('error');
    
    logger.logExploration(action, url, mode, success, text.substring(0, 100));
  } catch (error) {
    logger.logExploration(action, url, mode, false, String(error));
  }
}

export { LogEntry, LogLevel, LogTarget, FlowLog, SkillLog, IssueLog, GoalLog, APILogger, createLogger };