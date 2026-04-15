/**
 * Learned Skills - AI-generated skills from observed issues
 */

import { Page } from 'playwright';

// Learned skill interface
export interface LearnedSkill {
  id: string;
  name: string;
  description: string;
  pattern: string;
  checkType: string;
  checkQuery: string;
  frequency: number;
  createdAt: string;
}

// Skill result with issues
export interface IssueMatch {
  skill: LearnedSkill;
  selector: string;
  matched: boolean;
  details: string;
}

/**
 * Generate a new skill from observed issues using AI
 * This would call the backend AI service in production
 */
export async function generateSkillFromIssues(
  issues: Array<{page: string; action: string; error: string}>
): Promise<LearnedSkill | null> {
  if (issues.length === 0) {
    return null;
  }
  
  // Simple pattern detection (in production, call AI backend)
  const patterns = detectPatterns(issues);
  
  if (patterns.length === 0) {
    return null;
  }
  
  const pattern = patterns[0];
  
  return {
    id: `learned_${Date.now()}`,
    name: pattern.name,
    description: pattern.description,
    pattern: pattern.pattern,
    checkType: pattern.checkType,
    checkQuery: pattern.query,
    frequency: issues.length,
    createdAt: new Date().toISOString(),
  };
}

/**
 * Detect common patterns from issues
 */
function detectPatterns(
  issues: Array<{page: string; action: string; error: string}>
): Array<{name: string; description: string; pattern: string; checkType: string; query: string}> {
  const patterns: Array<{name: string; description: string; pattern: string; checkType: string; query: string}> = [];
  
  // Check for similar error types
  const errorTypes = new Map<string, number>();
  
  for (const issue of issues) {
    const error = issue.error?.toLowerCase() || '';
    
    if (error.includes('max') || error.includes('length') || error.includes('too long')) {
      const existing = patterns.find(p => p.name === 'max_length_detected');
      if (existing) {
        existing.query = 'textarea:not([maxlength])';
      } else {
        patterns.push({
          name: 'max_length_detected',
          description: 'Auto-detected max length issue',
          pattern: 'max.*length',
          checkType: 'selector',
          query: 'textarea:not([maxlength])',
        });
      }
    }
    
    if (error.includes('required') || error.includes('empty')) {
      const existing = patterns.find(p => p.name === 'required_field_detected');
      if (existing) {
        existing.query = 'input[required]';
      } else {
        patterns.push({
          name: 'required_field_detected',
          description: 'Auto-detected required field issue',
          pattern: 'required.*empty',
          checkType: 'validation',
          query: 'input[required]',
        });
      }
    }
    
    if (error.includes('500') || error.includes('internal error')) {
      const existing = patterns.find(p => p.name === 'backend_error_detected');
      if (existing) {
        existing.query = 'form';
      } else {
        patterns.push({
          name: 'backend_error_detected',
          description: 'Auto-detected backend error',
          pattern: '500.*error',
          checkType: 'data',
          query: 'form',
        });
      }
    }
  }
  
  return patterns;
}

/**
 * LearnedSkillRunner - executes learned skills
 */
export class LearnedSkillRunner {
  private page: Page;
  private learnedSkills: LearnedSkill[] = [];
  
  constructor(page: Page) {
    this.page = page;
  }
  
  /**
   * Add a learned skill
   */
  addSkill(skill: LearnedSkill): void {
    this.learnedSkills.push(skill);
  }
  
  /**
   * Load skills from backend
   */
  async loadFromBackend(): Promise<void> {
    try {
      const response = await fetch(`${process.env.API_URL || 'http://localhost:8080'}/api/skills?builtin=false`);
      const data = await response.json();
      this.learnedSkills = data.map((s: any) => ({
        id: s.id,
        name: s.name,
        description: s.description,
        pattern: s.pattern,
        checkType: s.check_type,
        checkQuery: s.check_query,
        frequency: s.frequency,
        createdAt: s.created_at,
      }));
    } catch {
      // Backend might not have skills yet
    }
  }
  
  /**
   * Run a specific learned skill
   */
  async runSkill(skill: LearnedSkill): Promise<IssueMatch[]> {
    const results: IssueMatch[] = [];
    
    try {
      const elements = await this.page.$$(skill.checkQuery);
      
      for (const el of elements) {
        const isVisible = await el.isVisible().catch(() => false);
        if (!isVisible) continue;
        
        let matched = false;
        let details = '';
        
        // Check based on skill type
        switch (skill.checkType) {
          case 'selector':
            // Just check if selector exists
            matched = true;
            details = `Found ${elements.length} elements`;
            break;
            
          case 'validation':
            // Try to trigger validation
            matched = true;
            details = 'Validation check needed';
            break;
            
          case 'data':
            // Check for data issues
            const value = await el.inputValue().catch(() => '');
            if (value.length > 1000) {
              matched = true;
              details = `Large value: ${value.length} chars`;
            }
            break;
        }
        
        results.push({
          skill,
          selector: skill.checkQuery,
          matched,
          details,
        });
      }
    } catch (error) {
      results.push({
        skill,
        selector: skill.checkQuery,
        matched: false,
        details: `Error: ${error}`,
      });
    }
    
    return results;
  }
  
  /**
   * Run all learned skills
   */
  async runAll(): Promise<IssueMatch[]> {
    const results: IssueMatch[] = [];
    
    for (const skill of this.learnedSkills) {
      const skillResults = await this.runSkill(skill);
      results.push(...skillResults);
    }
    
    return results;
  }
  
  /**
   * Get all skills
   */
  getSkills(): LearnedSkill[] {
    return this.learnedSkills;
  }
  
  /**
   * Get failed checks
   */
  getFailed(results: IssueMatch[]): IssueMatch[] {
    return results.filter(r => r.matched);
  }
}

/**
 * Analyze page for learned skill patterns
 */
export async function analyzePageForPatterns(page: Page): Promise<LearnedSkill[]> {
  const patterns: LearnedSkill[] = [];
  
  // Collect potential issues
  const issues: string[] = [];
  
  // Check for long inputs
  const longInputs = await page.$$('input:not([maxlength]), textarea:not([maxlength])');
  if (longInputs.length > 0) {
    issues.push(' Possible long input without maxlength');
  }
  
  // Check for inputs without labels
  const unlabeled = await page.$$('input:not([aria-label]):not([aria-labelledby]):not([id])');
  if (unlabeled.length > 0) {
    issues.push(' Possible unlabeled input');
  }
  
  // Check for forms without CSRF
  const forms = await page.$$('form:not([action])');
  if (forms.length > 0) {
    issues.push(' Possible form without action');
  }
  
  // Convert to learned skills if issues found
  if (issues.length > 0) {
    patterns.push({
      id: `auto_${Date.now()}`,
      name: 'auto_detected_patterns',
      description: `Auto-detected: ${issues.join(', ')}`,
      pattern: 'auto_detection',
      checkType: 'selector',
      checkQuery: 'input, textarea, form',
      frequency: issues.length,
      createdAt: new Date().toISOString(),
    });
  }
  
  return patterns;
}

export { LearnedSkill, IssueMatch, generateSkillFromIssues };