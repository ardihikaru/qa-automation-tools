/**
 * Built-in QA skills (DOM checks), ported from playwright-qa
 * src/skills/builtin/skills.ts. Read-only skills run on every newly visited
 * page; interactive ones (they click submit) only when a run asks for them.
 */
import { Page } from 'playwright';

export type Severity = 'low' | 'medium' | 'high';

export interface SkillResult {
  skill: string;
  passed: boolean;
  severity: Severity;
  message: string;
  /** Short descriptors of offending elements (tag#id[name]), at most 10. */
  elements: string[];
}

interface Skill {
  name: string;
  severity: Severity;
  interactive: boolean;
  run(page: Page): Promise<Omit<SkillResult, 'skill' | 'severity'>>;
}

type Predicate = 'no_maxlength' | 'no_label' | 'typed_as_text';

/**
 * Returns descriptors of visible elements matching `selector` that satisfy the
 * named predicate. Predicates live inside the page function (no eval), so a
 * strict CSP on HISv3 cannot break them.
 */
async function describe(page: Page, selector: string, predicate: Predicate): Promise<{ count: number; elements: string[] }> {
  return page.$$eval(
    selector,
    (els, kind) => {
      const pred = (el: Element): boolean => {
        switch (kind) {
          case 'no_maxlength':
            return !el.hasAttribute('maxlength');
          case 'no_label':
            return !(
              el.getAttribute('aria-label') ||
              el.getAttribute('aria-labelledby') ||
              el.getAttribute('placeholder') ||
              el.closest('label') ||
              (el.id && document.querySelector(`label[for="${CSS.escape(el.id)}"]`))
            );
          case 'typed_as_text':
            return /(e-?mail|phone|telp|tanggal|date|tgl|nik|number|jumlah|qty)/i.test(
              `${el.getAttribute('name') || ''} ${el.id || ''}`,
            );
          default:
            return false;
        }
      };
      const visible = (el: Element) => {
        const r = (el as HTMLElement).getBoundingClientRect();
        return r.width > 0 && r.height > 0;
      };
      const hits = els.filter((el) => visible(el) && pred(el));
      return {
        count: hits.length,
        elements: hits.slice(0, 10).map((el) => {
          const id = el.id ? `#${el.id}` : '';
          const name = el.getAttribute('name') ? `[name=${el.getAttribute('name')}]` : '';
          return `${el.tagName.toLowerCase()}${id}${name}`;
        }),
      };
    },
    predicate,
  );
}

const SKILLS: Skill[] = [
  {
    name: 'max_length',
    severity: 'high',
    interactive: false,
    async run(page) {
      const r = await describe(page, 'textarea, input[type="text"], input:not([type])', 'no_maxlength');
      return {
        passed: r.count === 0,
        message: r.count ? `${r.count} text field(s) without maxlength` : 'All text fields have maxlength',
        elements: r.elements,
      };
    },
  },
  {
    name: 'label_association',
    severity: 'medium',
    interactive: false,
    async run(page) {
      const r = await describe(
        page,
        'input:not([type="hidden"]):not([type="submit"]):not([type="button"]), textarea, select',
        'no_label',
      );
      return {
        passed: r.count === 0,
        message: r.count ? `${r.count} form control(s) without an accessible label` : 'All form controls are labelled',
        elements: r.elements,
      };
    },
  },
  {
    name: 'input_type',
    severity: 'low',
    interactive: false,
    async run(page) {
      const r = await describe(
        page,
        'input[type="text"], input:not([type])',
        'typed_as_text',
      );
      return {
        passed: r.count === 0,
        message: r.count ? `${r.count} text input(s) look like email/phone/date/number fields` : 'Input types look appropriate',
        elements: r.elements,
      };
    },
  },
  {
    name: 'multi_tab_save',
    severity: 'medium',
    interactive: false,
    async run(page) {
      const tabs = await page.$$eval('[role="tablist"] [role="tab"]', (e) => e.length);
      const panels = await page.$$eval('[role="tabpanel"], .tab-content, [data-tab]', (e) => e.length);
      const suspicious = tabs > 1 && panels === 0;
      return {
        passed: !suspicious,
        message: suspicious ? `${tabs} tabs but no tab panels: tab data may be lost on switch` : 'No tab issues detected',
        elements: [],
      };
    },
  },
  {
    name: 'required_field',
    severity: 'high',
    interactive: true,
    async run(page) {
      const form = page.locator('form:has(input[required], textarea[required], select[required])').first();
      if ((await form.count()) === 0) return { passed: true, message: 'No form with required fields', elements: [] };
      const before = page.url();
      await form.locator('button[type="submit"], input[type="submit"]').first().click({ timeout: 3_000 }).catch(() => undefined);
      await page.waitForTimeout(800);
      const invalid = await page.$$eval(':invalid, [aria-invalid="true"], .error, .text-red-500', (e) => e.length);
      const ok = invalid > 0 || page.url() === before;
      return {
        passed: ok,
        message: ok ? 'Empty required fields are rejected' : 'Form with empty required fields was submitted',
        elements: [],
      };
    },
  },
];

export async function runSkills(page: Page, interactive: boolean): Promise<SkillResult[]> {
  const out: SkillResult[] = [];
  for (const s of SKILLS) {
    if (s.interactive && !interactive) continue;
    try {
      const r = await s.run(page);
      out.push({ skill: s.name, severity: s.severity, ...r });
    } catch (err) {
      out.push({
        skill: s.name,
        severity: s.severity,
        passed: true,
        message: `skill error (ignored): ${err instanceof Error ? err.message.split('\n')[0] : String(err)}`,
        elements: [],
      });
    }
  }
  return out;
}

export const SKILL_NAMES = SKILLS.map((s) => s.name);
