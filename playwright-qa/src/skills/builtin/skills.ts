/**
 * QA Skills - Built-in and learned skills for bug detection
 */

import { Page } from 'playwright';

// Skill interfaces
export interface Skill {
  id: string;
  name: string;
  description: string;
  checkType: 'selector' | 'validation' | 'data';
  checkQuery: string;
  severity: 'low' | 'medium' | 'high';
  isBuiltin: boolean;
}

export interface SkillResult {
  skill: string;
  passed: boolean;
  message: string;
  severity: string;
  elements: string[];
}

// Built-in QA Skills
export const BUILTIN_SKILLS: Skill[] = [
  {
    id: 'max_length',
    name: 'max_length',
    description: 'Check if textarea has proper maxlength attribute',
    checkType: 'selector',
    checkQuery: 'textarea:not([maxlength]), input[type="text"]:not([maxlength])',
    severity: 'high',
    isBuiltin: true,
  },
  {
    id: 'required_field',
    name: 'required_field',
    description: 'Check if required fields can be submitted empty',
    checkType: 'validation',
    checkQuery: 'input[required], textarea[required], select[required]',
    severity: 'high',
    isBuiltin: true,
  },
  {
    id: 'data_persistence',
    name: 'data_persistence',
    description: 'Check if form data persists after page reload',
    checkType: 'data',
    checkQuery: 'form[data-persist]',
    severity: 'medium',
    isBuiltin: true,
  },
  {
    id: 'multi_tab_save',
    name: 'multi_tab_save',
    description: 'Check if tab data is lost when switching without save',
    checkType: 'selector',
    checkQuery: '.tab-content, [data-tab], [role="tablist"]',
    severity: 'medium',
    isBuiltin: true,
  },
  {
    id: 'input_type',
    name: 'input_type',
    description: 'Check for proper input types',
    checkType: 'selector',
    checkQuery: 'input:not([type]), input[type="text"]',
    severity: 'low',
    isBuiltin: true,
  },
  {
    id: 'label_association',
    name: 'label_association',
    description: 'Check if form inputs have associated labels',
    checkType: 'selector',
    checkQuery: 'input:not([aria-label]):not([aria-labelledby]), textarea:not([aria-label]):not([aria-labelledby])',
    severity: 'medium',
    isBuiltin: true,
  },
];

/**
 * SkillRunner - executes skills against a page
 */
export class SkillRunner {
  private page: Page;
  private results: SkillResult[] = [];
  
  constructor(page: Page) {
    this.page = page;
  }
  
  /**
   * Run a specific skill
   */
  async runSkill(skill: Skill): Promise<SkillResult> {
    console.log(`  🔍 Running skill: ${skill.name}`);
    
    try {
      switch (skill.checkType) {
        case 'selector':
          return this.runSelectorCheck(skill);
        case 'validation':
          return this.runValidationCheck(skill);
        case 'data':
          return this.runDataCheck(skill);
        default:
          return {
            skill: skill.name,
            passed: true,
            message: 'Unknown check type',
            severity: skill.severity,
            elements: [],
          };
      }
    } catch (error) {
      return {
        skill: skill.name,
        passed: false,
        message: `Error: ${error}`,
        severity: skill.severity,
        elements: [],
      };
    }
  }
  
  /**
   * Run selector-based check
   */
  private async runSelectorCheck(skill: Skill): Promise<SkillResult> {
    const elements = await this.page.$$(skill.checkQuery);
    
    if (elements.length === 0) {
      return {
        skill: skill.name,
        passed: true,
        message: 'No elements found',
        severity: skill.severity,
        elements: [],
      };
    }
    
    // For max_length, we FAIL if no maxlength
    if (skill.name === 'max_length') {
      const withoutMaxLength = await this.page.$$(
        'textarea:not([maxlength]), input[type="text"]:not([maxlength])'
      );
      
      return {
        skill: skill.name,
        passed: withoutMaxLength.length === 0,
        message: withoutMaxLength.length > 0
          ? `Found ${withoutMaxLength.length} elements without maxlength`
          : 'All elements have maxlength',
        severity: skill.severity,
        elements: withoutMaxLength.map(el => el.toString()),
      };
    }
    
    // For multi_tab_save, check if tabs exist
    if (skill.name === 'multi_tab_save') {
      const tabs = await this.page.$$('[role="tablist"] [role="tab"]');
      const content = await this.page.$$('.tab-content, [data-tab]');
      
      return {
        skill: skill.name,
        passed: tabs.length <= 1 || content.length >= tabs.length,
        message: tabs.length > 1 && content.length < tabs.length
          ? 'Possible tab data loss issue'
          : 'No tab issues detected',
        severity: skill.severity,
        elements: tabs.map(el => el.toString()),
      };
    }
    
    return {
      skill: skill.name,
      passed: true,
      message: `Found ${elements.length} matching elements`,
      severity: skill.severity,
      elements: elements.map(el => el.toString()),
    };
  }
  
  /**
   * Run validation-based check
   */
  private async runValidationCheck(skill: Skill): Promise<SkillResult> {
    if (skill.name === 'required_field') {
      // Try to submit a form with empty required fields
      const requiredFields = await this.page.$$('input[required], textarea[required]');
      
      if (requiredFields.length === 0) {
        return {
          skill: skill.name,
          passed: true,
          message: 'No required fields found',
          severity: skill.severity,
          elements: [],
        };
      }
      
      // Get the form
      const form = await this.page.$('form');
      const submitBtn = await this.page.$('button[type="submit"]');
      
      if (!form || !submitBtn) {
        return {
          skill: skill.name,
          passed: true,
          message: 'No form or submit button found',
          severity: skill.severity,
          elements: [],
        };
      }
      
      // Try to submit without filling required fields
      await submitBtn.click();
      await this.page.waitForTimeout(500);
      
      // Check for validation error
      const errorMessage = await this.page.$('.error, .text-red, [aria-invalid="true"]');
      const errorText = errorMessage
        ? await errorMessage.innerText().catch(() => '')
        : '';
      
      const hasValidation = errorText.length > 0 || 
        (await this.page.$('[aria-invalid="true"]')) !== null;
      
      return {
        skill: skill.name,
        passed: hasValidation,
        message: hasValidation
          ? 'Validation working correctly'
          : 'Required field can be bypassed!',
        severity: skill.severity,
        elements: requiredFields.map(el => el.toString()),
      };
    }
    
    return {
      skill: skill.name,
      passed: true,
      message: 'Check completed',
      severity: skill.severity,
      elements: [],
    };
  }
  
  /**
   * Run data-based check
   */
  private async runDataCheck(skill: Skill): Promise<SkillResult> {
    if (skill.name === 'data_persistence') {
      // Fill a form field
      const input = await this.page.$('input[type="text"], textarea');
      
      if (!input) {
        return {
          skill: skill.name,
          passed: true,
          message: 'No input fields found',
          severity: skill.severity,
          elements: [],
        };
      }
      
      const testValue = 'test_persistence_' + Date.now();
      await input.fill(testValue);
      
      // Reload page
      await this.page.reload();
      await this.page.waitForLoadState('domcontentloaded');
      
      // Check if value persists
      const valueAfter = await input.inputValue().catch(() => '');
      
      return {
        skill: skill.name,
        passed: valueAfter === '',
        message: valueAfter === '' 
          ? 'Data NOT persisted after reload!'
          : 'Data persisted correctly',
        severity: skill.severity,
        elements: [input.toString()],
      };
    }
    
    return {
      skill: skill.name,
      passed: true,
      message: 'Check completed',
      severity: skill.severity,
      elements: [],
    };
  }
  
  /**
   * Run all built-in skills
   */
  async runAllBuiltin(): Promise<SkillResult[]> {
    console.log('🔍 Running all built-in skills...');
    
    const results: SkillResult[] = [];
    
    for (const skill of BUILTIN_SKILLS) {
      const result = await this.runSkill(skill);
      results.push(result);
      
      console.log(
        `  ${result.passed ? '✅' : '❌'} ${skill.name}: ${result.message}`
      );
    }
    
    this.results = results;
    return results;
  }
  
  /**
   * Run specific skills by name
   */
  async runByName(names: string[]): Promise<SkillResult[]> {
    const results: SkillResult[] = [];
    
    for (const name of names) {
      const skill = BUILTIN_SKILLS.find(s => s.name === name);
      if (skill) {
        const result = await this.runSkill(skill);
        results.push(result);
      }
    }
    
    this.results = results;
    return results;
  }
  
  /**
   * Get failed results
   */
  getFailed(): SkillResult[] {
    return this.results.filter(r => !r.passed);
  }
  
  /**
   * Get passed results
   */
  getPassed(): SkillResult[] {
    return this.results.filter(r => r.passed);
  }
  
  /**
   * Get all results
   */
  getAll(): SkillResult[] {
    return this.results;
  }
}

/**
 * Run skills and report results
 */
export async function runSkills(page: Page): Promise<SkillResult[]> {
  const runner = new SkillRunner(page);
  return runner.runAllBuiltin();
}

/**
 * Get built-in skills
 */
export function getBuiltinSkills(): Skill[] {
  return BUILTIN_SKILLS;
}

/**
 * Get skill by name
 */
export function getSkillByName(name: string): Skill | undefined {
  return BUILTIN_SKILLS.find(s => s.name === name);
}

export { Skill, SkillResult };